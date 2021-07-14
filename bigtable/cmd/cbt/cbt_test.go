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
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"cloud.google.com/go/bigtable"
	"cloud.google.com/go/bigtable/internal/cbtconfig"
	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
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

func TestGetDataFilter(t *testing.T) {
	valid := []string{"columns", "cells-per-column", "app-profile", "keys-only"}
	cmpOpts := cmp.Options{
		cmp.AllowUnexported(bigtable.ChainFilters([]bigtable.Filter{}...)),
		cmp.AllowUnexported(
			bigtable.RowFilter(
				bigtable.ChainFilters([]bigtable.Filter{}...))),
	}
	type result struct {
		Opt      bigtable.ReadOption
		KeysOnly bool
		Err      string
	}

	tests := []struct {
		args   []string
		result result
	}{
		{[]string{}, result{nil, false, "<nil>"}},
		{[]string{"columns=columnA"},
			result{
				bigtable.RowFilter(
					bigtable.ColumnFilter("columnA")),
				false, "<nil>"}},
		{[]string{"columns=columnA", "keys-only=f"},
			result{
				bigtable.RowFilter(
					bigtable.ColumnFilter("columnA")),
				false, "<nil>"}},
		{[]string{"columns=columnA", "keys-only=fff"},
			result{nil, false, "Bad value for keys-only: fff"}},
		{[]string{"columns=columnA", "keys-only=t"},
			result{
				bigtable.RowFilter(
					bigtable.ChainFilters(
						bigtable.StripValueFilter(),
						bigtable.ColumnFilter("columnA"),
					),
				),
				true, "<nil>"}},
		{[]string{"columns=columnA", "keys-only=t", "cells-per-column=42"},
			result{
				bigtable.RowFilter(
					bigtable.ChainFilters(
						bigtable.LatestNFilter(42),
						bigtable.StripValueFilter(),
						bigtable.ColumnFilter("columnA"),
					),
				),
				true, "<nil>"}},
		{[]string{"columns=columnA", "keys-only=t", "cells-per-column=f"},
			result{nil, false,
				"Bad number of cells per column \"f\":" +
					" strconv.Atoi: parsing \"f\": invalid syntax"}},
	}

	for _, test := range tests {
		parsed, err := parseArgs(test.args, valid)
		assertNoError(t, err)
		opt, keysOnly, err := getDataFilter(parsed)
		assertEqual(t, result{opt, keysOnly, fmt.Sprint(err)}, test.result, cmpOpts...)
	}
}

type filterTable struct {
	Opts []bigtable.ReadOption
}

var filterTableRows = []bigtable.Row{
	{
		"f1": {
			bigtable.ReadItem{
				Row:    "r1",
				Column: "c1",
				Value:  []byte("Hello!"),
			},
			bigtable.ReadItem{
				Row:    "r2",
				Column: "c2",
				Value:  []byte{1, 2},
			},
		},
	},
	{
		"f1": {
			bigtable.ReadItem{
				Row:    "r2",
				Column: "c1",
				Value:  []byte("Hi!"),
			},
		},
	},
}

func (table *filterTable) ReadRows(
	ctx context.Context,
	rs bigtable.RowSet,
	f func(bigtable.Row) bool,
	opts ...bigtable.ReadOption,
) error {
	table.Opts = opts

	for _, row := range filterTableRows {
		f(row)
	}

	return nil
}

func (table *filterTable) ReadRow(
	ctx context.Context,
	row string,
	opts ...bigtable.ReadOption,
) (bigtable.Row, error) {
	table.Opts = opts
	return filterTableRows[0], nil
}

var timestampsRE = regexp.MustCompile("[ ]+@ [^ \t\n]+")

func stripTimestamps(s string) string {
	return string(timestampsRE.ReplaceAll([]byte(s), []byte("")))
}

func TestDoLookup(t *testing.T) {
	config := cbtconfig.Config{Project: "p", Instance: "i", Creds: "c"}
	cmpOpts := cmp.Options{
		cmp.AllowUnexported(bigtable.ChainFilters([]bigtable.Filter{}...)),
		cmp.AllowUnexported(
			bigtable.RowFilter(
				bigtable.StripValueFilter())),
	}

	ft := &filterTable{}
	table = ft
	defer func() { table = nil }()

	out := captureStdout(func() {
		doMain(&config, []string{"lookup", "mytable", "r"})
	})

	assertEqual(t, stripTimestamps(out),
		"----------------------------------------\n"+
			"r1\n"+
			"  c1\n"+
			"    \"Hello!\"\n"+
			"  c2\n"+
			"    \"\\x01\\x02\"\n")

	var inopts []bigtable.ReadOption
	expectOpts := func(opts ...bigtable.ReadOption) []bigtable.ReadOption {
		return opts
	}(inopts...)

	assertEqual(t, ft.Opts, expectOpts)

	ft = &filterTable{}
	table = ft

	out = captureStdout(func() {
		doMain(&config, []string{"lookup", "mytable", "r", "keys-only=t"})
	})

	assertEqual(t, stripTimestamps(out),
		"----------------------------------------\n"+
			"r1\n"+
			"  c1\n"+
			"  c2\n")

	assertEqual(t, ft.Opts, []bigtable.ReadOption{
		bigtable.RowFilter(bigtable.StripValueFilter()),
	}, cmpOpts...)
}

func TestDoRead(t *testing.T) {
	config := cbtconfig.Config{Project: "p", Instance: "i", Creds: "c"}
	cmpOpts := cmp.Options{
		cmp.AllowUnexported(bigtable.ChainFilters([]bigtable.Filter{}...)),
		cmp.AllowUnexported(
			bigtable.RowFilter(
				bigtable.StripValueFilter())),
	}

	ft := &filterTable{}
	table = ft
	defer func() { table = nil }()

	out := captureStdout(func() {
		doMain(&config, []string{"read", "mytable"})
	})

	assertEqual(t, stripTimestamps(out),
		"----------------------------------------\n"+
			"r1\n"+
			"  c1\n"+
			"    \"Hello!\"\n"+
			"  c2\n"+
			"    \"\\x01\\x02\"\n"+
			"----------------------------------------\n"+
			"r2\n"+
			"  c1\n"+
			"    \"Hi!\"\n")

	var inopts []bigtable.ReadOption
	expectOpts := func(opts ...bigtable.ReadOption) []bigtable.ReadOption {
		return opts
	}(inopts...)

	assertEqual(t, ft.Opts, expectOpts)

	ft = &filterTable{}
	table = ft

	out = captureStdout(func() {
		doMain(&config, []string{"read", "mytable", "keys-only=t"})
	})

	assertEqual(t, stripTimestamps(out),
		"----------------------------------------\n"+
			"r1\n"+
			"  c1\n"+
			"  c2\n"+
			"----------------------------------------\n"+
			"r2\n"+
			"  c1\n")

	assertEqual(t, ft.Opts, []bigtable.ReadOption{
		bigtable.RowFilter(bigtable.StripValueFilter()),
	}, cmpOpts...)
}
