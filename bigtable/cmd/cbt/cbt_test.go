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
	"errors"
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

func TestImporterArgs(t *testing.T) {
	tests := []struct {
		in   []string
		out  importerArgs
		fail bool
	}{
		{in: []string{"my-table", "my-file.csv"}, out: importerArgs{"", []string{""}, 500, 1}},
		{in: []string{"my-table", "my-file.csv", "app-profile="}, out: importerArgs{"", []string{""}, 500, 1}},
		{in: []string{"my-table", "my-file.csv", "column-family="}, out: importerArgs{"", []string{"", ""}, 500, 1}},
		{in: []string{"my-table", "my-file.csv", "app-profile=my-ap", "column-family=my-family", "batch-size=100", "workers=20"},
			out: importerArgs{"my-ap", []string{"", "my-family"}, 100, 20}},

		{in: []string{}, fail: true},
		{in: []string{"my-table", "my-file.csv", "batch-size=-5"}, fail: true},
		{in: []string{"my-table", "my-file.csv", "batch-size=5000000"}, fail: true},
		{in: []string{"my-table", "my-file.csv", "batch-size=nan"}, fail: true},
		{in: []string{"my-table", "my-file.csv", "batch-size="}, fail: true},
		{in: []string{"my-table", "my-file.csv", "workers=0"}, fail: true},
		{in: []string{"my-table", "my-file.csv", "workers=nan"}, fail: true},
		{in: []string{"my-table", "my-file.csv", "workers="}, fail: true},
	}
	for _, tc := range tests {
		got, err := parseImporterArgs(context.Background(), tc.in)
		if !tc.fail && err != nil {
			t.Errorf("parseImportArgs(%q) unexpectedly failed: %v", tc.in, err)
			continue
		}
		if tc.fail && err == nil {
			t.Errorf("parseImportArgs(%q) did not fail, out: %q", tc.in, got)
			continue
		}
		if tc.fail {
			continue
		}
		if got.appProfile != tc.out.appProfile ||
			len(got.fams) != len(tc.out.fams) ||
			got.sz != tc.out.sz ||
			got.workers != tc.out.workers {
			t.Errorf("parseImportArgs(%q) did not fail, out: %q", tc.in, got)
			continue
		}
		for i, f := range got.fams {
			if f != tc.out.fams[i] {
				t.Errorf("parseImportArgs(%q) incorrect column families, out: %q", tc.in, got)
				continue
			}
		}
	}
}

func writeAsCSV(records [][]string) ([]byte, error) {
	if records == nil || len(records) == 0 {
		return nil, errors.New("Records cannot be nil or empty")
	}
	var buf bytes.Buffer
	csvWriter := csv.NewWriter(&buf)
	err := csvWriter.WriteAll(records)
	if err != nil {
		return nil, err
	}
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func TestCsvParser(t *testing.T) {
	tests := []struct {
		iData    [][]string
		iFams    []string
		oFams    []string
		oCols    []string
		nextLine []string
		fail     bool
	}{
		// extends "my-family" to col-2
		{iData: [][]string{{"", "my-family", "", "my-family-2"}, {"", "col-1", "col-2", "col-3"}, {"rk-1", "A", "", ""}},
			iFams:    []string{""},
			oFams:    []string{"", "my-family", "my-family", "my-family-2"},
			oCols:    []string{"", "col-1", "col-2", "col-3"},
			nextLine: []string{"rk-1", "A", "", ""}},
		// handles column-faimly=arg-family flag
		{iData: [][]string{{"", "col-1", "col-2"}, {"rk-1", "A", ""}},
			iFams:    []string{"", "arg-family"},
			oFams:    []string{"", "arg-family", "arg-family"},
			oCols:    []string{"", "col-1", "col-2"},
			nextLine: []string{"rk-1", "A", ""}},

		// early EOF in headers
		{iData: [][]string{{"", "my-family", ""}},
			iFams: []string{""},
			fail:  true},
		// empty column-family from iFams[1] (ie: empty column-family arg)
		{iData: [][]string{{"", "my-family", ""}, {"", "col-1", "col-2"}},
			iFams: []string{"", ""},
			fail:  true},
	}

	for _, tc := range tests {
		// create in memory csv like file
		byteData, err := writeAsCSV(tc.iData)
		if err != nil {
			t.Fatal(err)
		}
		reader := csv.NewReader(bytes.NewReader(byteData))

		fams, cols, err := parseCsvHeaders(reader, tc.iFams)
		if !tc.fail && err != nil {
			t.Errorf("parseCsvHeaders() failed. input:%+v, error:%s", tc, err)
			continue
		}
		if tc.fail && err == nil {
			t.Errorf("parseImportArgs() did not fail. input:%+v, error:%s", tc, err)
			continue
		}
		if tc.fail {
			continue
		}

		line, _ := reader.Read()
		if err != nil {
			t.Errorf("Next line for reader error, got: %q, expect: %q", line, tc.nextLine)
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
				t.Errorf("parseImportArgs() did not fail for column names idx:%d, got: %q, want %q", i, cols[i], tc.oCols[i])
				continue
			}
		}
		for i, v := range line {
			if v != tc.nextLine[i] {
				t.Errorf("parseImportArgs() did not fail for next line idx:%d, got: %q, want %q", i, cols[i], tc.oCols[i])
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

// test against in-memory bttest emulator
func TestParseAndWrite(t *testing.T) {
	ctx, client := setupEmulator(t, []string{"my-table"}, []string{"my-family", "my-family-2"})

	tbl := client.Open("my-table")
	fams := []string{"", "my-family", "my-family-2"}
	cols := []string{"", "col-1", "col-2"}
	rowData := [][]string{
		{"rk-0", "A", "B"},
		{"rk-1", "", "C"},
	}
	matchCount := 3

	byteData, err := writeAsCSV(rowData)
	if err != nil {
		t.Fatal(err)
	}
	reader := csv.NewReader(bytes.NewReader(byteData))

	sr := safeReader{r: reader}
	if err = sr.parseAndWrite(ctx, tbl, fams, cols, 1, 1, 1); err != nil {
		t.Fatalf("parseAndWrite() failed unexpectedly")
	}

	// vaildate table entries
	colMap := make(map[string][]string)
	for i := range fams {
		var col []string
		for _, r := range rowData {
			col = append(col, r[i])
		}
		colMap[fams[i]+":"+cols[i]] = col
	}
	for rowIdx, data := range rowData {
		if row, err := tbl.ReadRow(ctx, data[0]); err != nil {
			t.Errorf("Error %s", err)
		} else {
			for _, fam := range fams {
				for _, column := range row[fam] {
					colId := string(column.Column)
					col, ok := colMap[colId]
					if ok {
						if string(column.Value) == col[rowIdx] {
							matchCount--
							continue
						}
						t.Errorf("Column data didnt match, colId: %s, got: %s, want %s\n", colId, string(column.Value), col[rowIdx])
					}
				}
			}
		}
	}

	if matchCount != 0 {
		t.Fatalf("Data didn't match after read for %d values", matchCount)
	}
}

// test against in-memory bttest emulator
func TestParseAndWriteBadFamily(t *testing.T) {
	ctx, client := setupEmulator(t, []string{"my-table"}, []string{"my-family"})

	tbl := client.Open("my-table")
	fams := []string{"", "my-family", "not-my-family"}
	cols := []string{"", "col-1", "col-2"}
	rowData := [][]string{
		{"rk-0", "A", "B"},
		{"rk-1", "", "C"},
	}

	byteData, err := writeAsCSV(rowData)
	if err != nil {
		t.Fatal(err)
	}
	reader := csv.NewReader(bytes.NewReader(byteData))

	sr := safeReader{r: reader}
	if err = sr.parseAndWrite(ctx, tbl, fams, cols, 1, 1, 1); err == nil {
		t.Fatalf("parseAndWrite() should have failed with non-existant column family")
	}
}

// test against in-memory bttest emulator
func TestCsvToCbt(t *testing.T) {
	tests := []struct {
		label        string
		ia           importerArgs
		csvData      [][]string
		expectedFams []string
		matchCount   int
		dataStartIdx int
	}{
		{
			label: "has-column-families",
			ia:    importerArgs{fams: []string{""}, sz: 1, workers: 1},
			csvData: [][]string{
				{"", "my-family", ""},
				{"", "col-1", "col-2"},
				{"rk-0", "A", ""},
				{"rk-1", "", "B"},
				{"rk-2", "", ""},
				{"rk-3", "C", ""},
			},
			expectedFams: []string{"", "my-family", "my-family"},
			matchCount:   3,
			dataStartIdx: 2,
		},
		{
			label: "no-column-families",
			ia:    importerArgs{fams: []string{"", "arg-family"}, sz: 1, workers: 1},
			csvData: [][]string{
				{"", "col-1", "col-2"},
				{"rk-0", "A", ""},
				{"rk-1", "", "B"},
				{"rk-2", "", ""},
				{"rk-3", "C", "D"},
			},
			expectedFams: []string{"", "arg-family", "arg-family"},
			matchCount:   4,
			dataStartIdx: 1,
		},
		{
			label: "larger-batches",
			ia:    importerArgs{fams: []string{"", "arg-family"}, sz: 100, workers: 1},
			csvData: [][]string{
				{"", "col-1", "col-2"},
				{"rk-0", "A", ""},
				{"rk-1", "", "B"},
				{"rk-2", "", ""},
				{"rk-3", "C", "D"},
			},
			expectedFams: []string{"", "arg-family", "arg-family"},
			matchCount:   4,
			dataStartIdx: 1,
		},
		{
			label: "many-workers",
			ia:    importerArgs{fams: []string{"", "arg-family"}, sz: 1, workers: 20},
			csvData: [][]string{
				{"", "col-1", "col-2"},
				{"rk-0", "A", ""},
				{"rk-1", "", "B"},
				{"rk-2", "", ""},
				{"rk-3", "C", "D"},
			},
			expectedFams: []string{"", "arg-family", "arg-family"},
			matchCount:   4,
			dataStartIdx: 1,
		},
	}

	for _, tc := range tests {
		ctx, client := setupEmulator(t, []string{"my-table"}, []string{"my-family", "arg-family"})
		tbl := client.Open("my-table")

		byteData, err := writeAsCSV(tc.csvData)
		if err != nil {
			t.Fatal(err)
		}
		reader := csv.NewReader(bytes.NewReader(byteData))

		importCSV(ctx, tbl, reader, tc.ia)

		// created lookup map for expected outputs
		colRow := tc.csvData[tc.dataStartIdx-1]
		colMap := make(map[string][]string)
		for i := range tc.expectedFams {
			var col []string
			for _, r := range tc.csvData[tc.dataStartIdx:] {
				col = append(col, r[i])
			}
			colMap[tc.expectedFams[i]+":"+colRow[i]] = col
		}

		// read rows back and validate mutations
		for rowIdx, data := range tc.csvData[tc.dataStartIdx:] {
			if row, err := tbl.ReadRow(ctx, data[0]); err != nil {
				t.Errorf("%s error %s", tc.label, err)
			} else {
				for _, cf := range row { // each column family in row
					for _, column := range cf { // each cf:column, aka each mutation
						colId := string(column.Column)
						col, ok := colMap[colId]
						if ok {
							if string(column.Value) == col[rowIdx] {
								tc.matchCount--
								continue
							}
							t.Errorf("%s, column data didnt match, colId: %s, got: %s, want %s\n", tc.label, colId, string(column.Value), col[rowIdx])
						}
					}
				}
			}
		}

		if tc.matchCount != 0 {
			t.Fatalf("%s, data didn't match after read for %d values", tc.label, tc.matchCount)
		}
	}
}
