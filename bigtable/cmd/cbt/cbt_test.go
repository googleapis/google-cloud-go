// Copyright 2016 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/bigtable"
	"cloud.google.com/go/bigtable/bttest"
	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		in string
		// out or fail are mutually exclusive
		out  time.Duration
		fail bool
	}{
		{in: "10ms", out: 10 * time.Millisecond},
		{in: "3s", out: 3 * time.Second},
		{in: "60m", out: 60 * time.Minute},
		{in: "12h", out: 12 * time.Hour},
		{in: "7d", out: 168 * time.Hour},

		{in: "", fail: true},
		{in: "0", fail: true},
		{in: "7ns", fail: true},
		{in: "14mo", fail: true},
		{in: "3.5h", fail: true},
		{in: "106752d", fail: true}, // overflow
	}
	for _, tc := range tests {
		got, err := parseDuration(tc.in)
		if !tc.fail && err != nil {
			t.Errorf("parseDuration(%q) unexpectedly failed: %v", tc.in, err)
			continue
		}
		if tc.fail && err == nil {
			t.Errorf("parseDuration(%q) did not fail", tc.in)
			continue
		}
		if tc.fail {
			continue
		}
		if got != tc.out {
			t.Errorf("parseDuration(%q) = %v, want %v", tc.in, got, tc.out)
		}
	}
}

func TestParseArgs(t *testing.T) {
	got, err := parseArgs([]string{"a=1", "b=2"}, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"a": "1", "b": "2"}
	if !testutil.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	if _, err := parseArgs([]string{"a1"}, []string{"a1"}); err == nil {
		t.Error("malformed: got nil, want error")
	}
	if _, err := parseArgs([]string{"a=1"}, []string{"b"}); err == nil {
		t.Error("invalid: got nil, want error")
	}
}

func TestParseColumnsFilter(t *testing.T) {
	tests := []struct {
		in   string
		out  bigtable.Filter
		fail bool
	}{
		{
			in:  "columnA",
			out: bigtable.ColumnFilter("columnA"),
		},
		{
			in:  "familyA:columnA",
			out: bigtable.ChainFilters(bigtable.FamilyFilter("familyA"), bigtable.ColumnFilter("columnA")),
		},
		{
			in:  "columnA,columnB",
			out: bigtable.InterleaveFilters(bigtable.ColumnFilter("columnA"), bigtable.ColumnFilter("columnB")),
		},
		{
			in: "familyA:columnA,columnB",
			out: bigtable.InterleaveFilters(
				bigtable.ChainFilters(bigtable.FamilyFilter("familyA"), bigtable.ColumnFilter("columnA")),
				bigtable.ColumnFilter("columnB"),
			),
		},
		{
			in: "columnA,familyB:columnB",
			out: bigtable.InterleaveFilters(
				bigtable.ColumnFilter("columnA"),
				bigtable.ChainFilters(bigtable.FamilyFilter("familyB"), bigtable.ColumnFilter("columnB")),
			),
		},
		{
			in: "familyA:columnA,familyB:columnB",
			out: bigtable.InterleaveFilters(
				bigtable.ChainFilters(bigtable.FamilyFilter("familyA"), bigtable.ColumnFilter("columnA")),
				bigtable.ChainFilters(bigtable.FamilyFilter("familyB"), bigtable.ColumnFilter("columnB")),
			),
		},
		{
			in:  "familyA:",
			out: bigtable.FamilyFilter("familyA"),
		},
		{
			in:  ":columnA",
			out: bigtable.ColumnFilter("columnA"),
		},
		{
			in: ",:columnA,,familyB:columnB,",
			out: bigtable.InterleaveFilters(
				bigtable.ColumnFilter("columnA"),
				bigtable.ChainFilters(bigtable.FamilyFilter("familyB"), bigtable.ColumnFilter("columnB")),
			),
		},
		{
			in:   "familyA:columnA:cellA",
			fail: true,
		},
		{
			in:   "familyA::columnA",
			fail: true,
		},
	}

	for _, tc := range tests {
		got, err := parseColumnsFilter(tc.in)

		if !tc.fail && err != nil {
			t.Errorf("parseColumnsFilter(%q) unexpectedly failed: %v", tc.in, err)
			continue
		}
		if tc.fail && err == nil {
			t.Errorf("parseColumnsFilter(%q) did not fail", tc.in)
			continue
		}
		if tc.fail {
			continue
		}

		var cmpOpts cmp.Options
		cmpOpts =
			append(
				cmpOpts,
				cmp.AllowUnexported(bigtable.ChainFilters([]bigtable.Filter{}...)),
				cmp.AllowUnexported(bigtable.InterleaveFilters([]bigtable.Filter{}...)))

		if !cmp.Equal(got, tc.out, cmpOpts) {
			t.Errorf("parseColumnsFilter(%q) = %v, want %v", tc.in, got, tc.out)
		}
	}
}

// Check if we get a substring of the expected error.
// Returns "" if so, else returns the expected substring and error.
func matchesExpectedError(want string, err error) string {
	if err != nil {
		got := err.Error()
		if want == "" || !strings.Contains(got, want) {
			return fmt.Sprintf("expected error substr:%s, got:%s", want, got)
		}
	} else if want != "" {
		return fmt.Sprintf("expected error substr:%s", want)
	}
	return ""
}

func TestCsvImporterArgs(t *testing.T) {
	tests := []struct {
		in  []string
		out importerArgs
		err string
	}{
		{in: []string{"my-table", "my-file.csv"}, out: importerArgs{"", "", 500, 1}},
		{in: []string{"my-table", "my-file.csv", "app-profile="}, out: importerArgs{"", "", 500, 1}},
		{in: []string{"my-table", "my-file.csv", "app-profile=my-ap", "column-family=my-family", "batch-size=100", "workers=20"},
			out: importerArgs{"my-ap", "my-family", 100, 20}},

		{in: []string{}, err: "usage: cbt import <table-id> <input-file> [app-profile=<app-profile-id>] [column-family=<family-name>] [batch-size=<500>] [workers=<1>]"},
		{in: []string{"my-table", "my-file.csv", "column-family="}, err: "column-family cannot be ''"},
		{in: []string{"my-table", "my-file.csv", "batch-size=-5"}, err: "batch-size must be > 0 and <= 100000"},
		{in: []string{"my-table", "my-file.csv", "batch-size=5000000"}, err: "batch-size must be > 0 and <= 100000"},
		{in: []string{"my-table", "my-file.csv", "batch-size=nan"}, err: "batch-size must be > 0 and <= 100000"},
		{in: []string{"my-table", "my-file.csv", "batch-size="}, err: "batch-size must be > 0 and <= 100000"},
		{in: []string{"my-table", "my-file.csv", "workers=0"}, err: "workers must be > 0, err:%!s(<nil>)"},
		{in: []string{"my-table", "my-file.csv", "workers=nan"}, err: "workers must be > 0, err:strconv.Atoi: parsing \"nan\": invalid syntax"},
		{in: []string{"my-table", "my-file.csv", "workers="}, err: "workers must be > 0, err:strconv.Atoi: parsing \"\": invalid syntax"},
	}
	for _, tc := range tests {
		got, err := parseImporterArgs(context.Background(), tc.in)
		if e := matchesExpectedError(tc.err, err); e != "" {
			t.Errorf("%s", e)
			continue
		}
		if tc.err != "" {
			continue // received expected error, do not parse below
		}
		if got.appProfile != tc.out.appProfile ||
			got.fam != tc.out.fam ||
			got.sz != tc.out.sz ||
			got.workers != tc.out.workers {
			t.Errorf("parseImportArgs(%q) did not fail, out: %q", tc.in, got)
		}
	}
}

func transformToCsvBuffer(data [][]string) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("Data cannot be empty")
	}
	var buf bytes.Buffer
	csvWriter := csv.NewWriter(&buf)
	if err := csvWriter.WriteAll(data); err != nil {
		return nil, err
	}
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func TestCsvHeaderParser(t *testing.T) {
	tests := []struct {
		label    string
		iData    [][]string
		iFam     string
		oFams    []string
		oCols    []string
		nextLine []string
		err      string
	}{
		{label: "extend-family-gap",
			iData:    [][]string{{"", "my-family", "", "my-family-2"}, {"", "col-1", "col-2", "col-3"}, {"rk-1", "A", "", ""}},
			iFam:     "",
			oFams:    []string{"", "my-family", "my-family", "my-family-2"},
			oCols:    []string{"", "col-1", "col-2", "col-3"},
			nextLine: []string{"rk-1", "A", "", ""}},
		{label: "handle-family-arg",
			iData:    [][]string{{"", "col-1", "col-2"}, {"rk-1", "A", ""}},
			iFam:     "arg-family",
			oFams:    []string{"", "arg-family", "arg-family"},
			oCols:    []string{"", "col-1", "col-2"},
			nextLine: []string{"rk-1", "A", ""}},

		{label: "eof-header-family",
			iData: [][]string{{""}},
			iFam:  "",
			err:   "family header reader error:EOF"},
		{label: "eof-header-column",
			iData: [][]string{{""}, {""}},
			iFam:  "arg-family",
			err:   "columns header reader error:EOF"},
		{label: "rowkey-in-header-row",
			iData: [][]string{{"ABC", "my-family", ""}},
			iFam:  "arg-family",
			err:   "the first column must be empty for column-family and column name rows"},
		{label: "blank-first-headers",
			iData: [][]string{{"", "", ""}},
			iFam:  "arg-family",
			err:   "the second column (first data column) must have values for column family and column name rows if present"},
	}

	for _, tc := range tests {
		// create in memory csv like file
		byteData, err := transformToCsvBuffer(tc.iData)
		if err != nil {
			t.Fatal(err)
		}
		reader := csv.NewReader(bytes.NewReader(byteData))

		fams, cols, err := parseCsvHeaders(reader, tc.iFam)
		if e := matchesExpectedError(tc.err, err); e != "" {
			t.Errorf("%s %s", tc.label, e)
			continue
		}
		if tc.err != "" {
			continue // received expected error, do not parse below
		}

		line, _ := reader.Read()
		if err != nil {
			t.Errorf("Next line for reader error, got: %q, expect: %q, error:%s", line, tc.nextLine, err)
			continue
		}
		if len(fams) != len(tc.oFams) ||
			len(cols) != len(tc.oCols) ||
			len(line) != len(tc.nextLine) {
			t.Errorf("parseCsvHeaders() did not fail, incorrect output sizes found, fams: %d, cols:%d, line:%d", len(fams), len(cols), len(line))
			continue
		}
		for i, f := range fams {
			if f != tc.oFams[i] {
				t.Errorf("Incorrect column families idx:%d, got: %q, want %q", i, fams[i], tc.oFams[i])
				continue
			}
		}
		for i, c := range cols {
			if c != tc.oCols[i] {
				t.Errorf("parseCsvHeaders() did not fail for column names idx:%d, got: %q, want %q", i, cols[i], tc.oCols[i])
				continue
			}
		}
		for i, v := range line {
			if v != tc.nextLine[i] {
				t.Errorf("parseCsvHeaders() did not fail for next line idx:%d, got: %q, want %q", i, cols[i], tc.oCols[i])
				continue
			}
		}
	}
}

func setupEmulator(t *testing.T, tables, families []string) (context.Context, *bigtable.Client) {
	srv, err := bttest.NewServer("localhost:0")
	if err != nil {
		t.Fatalf("Error starting bttest server: %s", err)
	}

	ctx := context.Background()

	conn, err := grpc.Dial(srv.Addr, grpc.WithInsecure())
	if err != nil {
		t.Fatalf("Error %s", err)
	}

	proj, instance := "proj", "instance"
	adminClient, err := bigtable.NewAdminClient(ctx, proj, instance, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("Error %s", err)
	}

	for _, ta := range tables {
		if err = adminClient.CreateTable(ctx, ta); err != nil {
			t.Fatalf("Error %s", err)
		}
		for _, f := range families {
			if err = adminClient.CreateColumnFamily(ctx, ta, f); err != nil {
				t.Fatalf("Error %s", err)
			}
		}
	}

	client, err := bigtable.NewClient(ctx, proj, instance, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("Error %s", err)
	}

	return ctx, client
}

func validateData(ctx context.Context, tbl *bigtable.Table, fams, cols []string, rowData [][]string) error {
	// vaildate table entries, valMap["rowkey:family:column"] = mutation value
	valMap := make(map[string]string)
	for _, row := range rowData {
		for i, val := range row {
			if i > 0 && val != "" {
				valMap[row[0]+":"+fams[i]+":"+cols[i]] = val
			}
		}
	}
	for _, data := range rowData {
		row, err := tbl.ReadRow(ctx, data[0])
		if err != nil {
			return err
		}
		for _, cf := range row {
			for _, column := range cf {
				k := data[0] + ":" + string(column.Column)
				v, ok := valMap[k]
				if ok && v == string(column.Value) {
					delete(valMap, k)
				}
			}
		}
	}
	if len(valMap) != 0 {
		return fmt.Errorf("Data didn't match after read, not found %v", valMap)
	}
	return nil
}

func TestCsvParseAndWrite(t *testing.T) {
	ctx, client := setupEmulator(t, []string{"my-table"}, []string{"my-family", "my-family-2"})

	tbl := client.Open("my-table")
	fams := []string{"", "my-family", "my-family-2"}
	cols := []string{"", "col-1", "col-2"}
	rowData := [][]string{
		{"rk-0", "A", "B"},
		{"rk-1", "", "C"},
	}

	byteData, err := transformToCsvBuffer(rowData)
	if err != nil {
		t.Fatal(err)
	}
	reader := csv.NewReader(bytes.NewReader(byteData))

	sr := safeReader{r: reader}
	if err = sr.parseAndWrite(ctx, tbl, fams, cols, 1, 1, 1); err != nil {
		t.Fatalf("parseAndWrite() failed unexpectedly, error:%s", err)
	}

	if err := validateData(ctx, tbl, fams, cols, rowData); err != nil {
		t.Fatalf("Read back validation error:%s", err)
	}
}

func TestCsvParseAndWriteBadFamily(t *testing.T) {
	ctx, client := setupEmulator(t, []string{"my-table"}, []string{"my-family"})

	tbl := client.Open("my-table")
	fams := []string{"", "my-family", "not-my-family"}
	cols := []string{"", "col-1", "col-2"}
	rowData := [][]string{
		{"rk-0", "A", "B"},
		{"rk-1", "", "C"},
	}

	byteData, err := transformToCsvBuffer(rowData)
	if err != nil {
		t.Fatal(err)
	}
	reader := csv.NewReader(bytes.NewReader(byteData))

	sr := safeReader{r: reader}
	if err = sr.parseAndWrite(ctx, tbl, fams, cols, 1, 1, 1); err == nil {
		t.Fatalf("parseAndWrite() should have failed with non-existant column family")
	}
}

func TestCsvParseAndWriteDuplicateRowkeys(t *testing.T) {
	ctx, client := setupEmulator(t, []string{"my-table"}, []string{"my-family"})

	tbl := client.Open("my-table")
	fams := []string{"", "my-family", "my-family"}
	cols := []string{"", "col-1", "col-2"}
	rowData := [][]string{
		{"rk-0", "A", ""},
		{"rk-0", "", "B"},
		{"rk-0", "C", ""},
	}

	byteData, err := transformToCsvBuffer(rowData)
	if err != nil {
		t.Fatal(err)
	}
	reader := csv.NewReader(bytes.NewReader(byteData))

	sr := safeReader{r: reader}
	if err = sr.parseAndWrite(ctx, tbl, fams, cols, 1, 1, 1); err != nil {
		t.Fatalf("parseAndWrite() should not have failed for duplicate rowkeys: %s", err)
	}

	// the "A" not present result is expected, the emulator only keeps 1 version
	valMap := map[string]bool{"rk-0:my-family:col-2:B": true, "rk-0:my-family:col-1:C": true}
	row, err := tbl.ReadRow(ctx, "rk-0")
	if err != nil {
		t.Errorf("error %s", err)
	}
	for _, cf := range row { // each column family in row
		for _, column := range cf { // each cf:column, aka each mutation
			k := "rk-0:" + string(column.Column) + ":" + string(column.Value)
			_, ok := valMap[k]
			if ok {
				delete(valMap, k)
				continue
			}
			t.Errorf("row data not found for %s\n", k)
		}
	}

	if len(valMap) != 0 {
		t.Fatalf("values were not present in table: %v", valMap)
	}
}

func TestCsvToCbt(t *testing.T) {
	tests := []struct {
		label        string
		ia           importerArgs
		csvData      [][]string
		expectedFams []string
		dataStartIdx int
	}{
		{
			label: "has-column-families",
			ia:    importerArgs{fam: "", sz: 1, workers: 1},
			csvData: [][]string{
				{"", "my-family", ""},
				{"", "col-1", "col-2"},
				{"rk-0", "A", ""},
				{"rk-1", "", "B"},
				{"rk-2", "", ""},
				{"rk-3", "C", ""},
			},
			expectedFams: []string{"", "my-family", "my-family"},
			dataStartIdx: 2,
		},
		{
			label: "no-column-families",
			ia:    importerArgs{fam: "arg-family", sz: 1, workers: 1},
			csvData: [][]string{
				{"", "col-1", "col-2"},
				{"rk-0", "A", ""},
				{"rk-1", "", "B"},
				{"rk-2", "", ""},
				{"rk-3", "C", "D"},
			},
			expectedFams: []string{"", "arg-family", "arg-family"},
			dataStartIdx: 1,
		},
		{
			label: "larger-batches",
			ia:    importerArgs{fam: "arg-family", sz: 100, workers: 1},
			csvData: [][]string{
				{"", "col-1", "col-2"},
				{"rk-0", "A", ""},
				{"rk-1", "", "B"},
				{"rk-2", "", ""},
				{"rk-3", "C", "D"},
			},
			expectedFams: []string{"", "arg-family", "arg-family"},
			dataStartIdx: 1,
		},
		{
			label: "many-workers",
			ia:    importerArgs{fam: "arg-family", sz: 1, workers: 20},
			csvData: [][]string{
				{"", "col-1", "col-2"},
				{"rk-0", "A", ""},
				{"rk-1", "", "B"},
				{"rk-2", "", ""},
				{"rk-3", "C", "D"},
			},
			expectedFams: []string{"", "arg-family", "arg-family"},
			dataStartIdx: 1,
		},
	}

	for _, tc := range tests {
		ctx, client := setupEmulator(t, []string{"my-table"}, []string{"my-family", "arg-family"})
		tbl := client.Open("my-table")

		byteData, err := transformToCsvBuffer(tc.csvData)
		if err != nil {
			t.Fatal(err)
		}
		reader := csv.NewReader(bytes.NewReader(byteData))

		importCSV(ctx, tbl, reader, tc.ia)

		if err := validateData(ctx, tbl, tc.expectedFams, tc.csvData[tc.dataStartIdx-1], tc.csvData[tc.dataStartIdx:]); err != nil {
			t.Fatalf("Read back validation error: %s", err)
		}
	}
}
