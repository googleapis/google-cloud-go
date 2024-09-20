// Copyright 2024 Google LLC
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

package dataflux

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestNewRangeSplitter(t *testing.T) {
	testCases := []struct {
		desc     string
		alphabet string
		wantErr  bool
	}{
		{
			desc:     "Valid alphabet",
			alphabet: "0123456789",
			wantErr:  false,
		},
		{
			desc:     "Empty alphabet",
			alphabet: "",
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			_, gotErr := newRangeSplitter(tc.alphabet)
			if (gotErr != nil) != tc.wantErr {
				t.Errorf("NewRangeSplitter(%q) got error = %v, want error = %v", tc.alphabet, gotErr, tc.wantErr)
			}
		})
	}
}

func TestSplitRange(t *testing.T) {

	testAlphabet := "0123456789"

	// We use the numbers as the base alphabet for testing purposes.
	rangeSplitter, err := newRangeSplitter(testAlphabet)
	if err != nil {
		t.Fatalf("NewRangeSplitter(%q) got error = %v, want error = nil", testAlphabet, err)
	}

	testCases := []struct {
		desc            string
		startRange      string
		endRange        string
		numSplits       int
		wantErr         bool
		wantSplitPoints []string
	}{
		// Tests for checking invalid arguments are properly handled.
		{
			desc:            "Number of Splits Less Than One",
			startRange:      "123",
			endRange:        "456",
			numSplits:       0,
			wantErr:         true,
			wantSplitPoints: nil,
		},
		{
			desc:            "End Range Lexicographically Smaller Than Start Range",
			startRange:      "456",
			endRange:        "123",
			numSplits:       2,
			wantErr:         true,
			wantSplitPoints: nil,
		},
		// Test for unsplittable cases.
		{
			desc:            "Unsplittable with Empty Start Range",
			startRange:      "",
			endRange:        "0",
			numSplits:       100,
			wantErr:         false,
			wantSplitPoints: nil,
		},
		{
			desc:            "Unsplittable with Non Empty Ranges",
			startRange:      "9",
			endRange:        "90",
			numSplits:       100,
			wantErr:         false,
			wantSplitPoints: nil,
		},
		// Test for splittable cases.
		{
			desc:            "Split Entire Bucket Namespace",
			startRange:      "",
			endRange:        "",
			numSplits:       24,
			wantErr:         false,
			wantSplitPoints: []string{"03", "07", "11", "15", "19", "23", "27", "31", "35", "39", "43", "47", "51", "55", "59", "63", "67", "71", "75", "79", "83", "87", "91", "95"},
		},
		{
			desc:            "Split with Only Start Range",
			startRange:      "5555",
			endRange:        "",
			numSplits:       4,
			wantErr:         false,
			wantSplitPoints: []string{"63", "72", "81", "90"},
		},
		{
			desc:            "Split Large Distance with Few Split Points",
			startRange:      "0",
			endRange:        "9",
			numSplits:       3,
			wantErr:         false,
			wantSplitPoints: []string{"2", "4", "6"},
		},
		{
			desc:            "Split with Prefix, Distance at Index 5 > 1",
			startRange:      "0123455111",
			endRange:        "012347",
			numSplits:       1,
			wantErr:         false,
			wantSplitPoints: []string{"012346"},
		},
		{
			desc:            "Split with Prefix, Distance at Index 6 > 1",
			startRange:      "00005699",
			endRange:        "00006",
			numSplits:       3,
			wantErr:         false,
			wantSplitPoints: []string{"000057", "000058", "000059"},
		},
		{
			desc:            "Split into Half with Small Range",
			startRange:      "199999",
			endRange:        "2",
			numSplits:       1,
			wantErr:         false,
			wantSplitPoints: []string{"1999995"},
		},
		{
			desc:            "Split into Multuple Pieces with Small Range",
			startRange:      "011",
			endRange:        "022",
			numSplits:       5,
			wantErr:         false,
			wantSplitPoints: []string{"012", "014", "016", "018", "020"},
		},
		{
			desc:            "Split towards End Range",
			startRange:      "8999",
			endRange:        "",
			numSplits:       4,
			wantErr:         false,
			wantSplitPoints: []string{"91", "93", "95", "97"},
		},
		{
			desc:            "Split with Sequence of Adjacent Characters",
			startRange:      "12345",
			endRange:        "23456",
			numSplits:       4,
			wantErr:         false,
			wantSplitPoints: []string{"14", "16", "18", "20"},
		},
		{
			desc:            "Split into Adjenct Split Points",
			startRange:      "0999998",
			endRange:        "1000002",
			numSplits:       3,
			wantErr:         false,
			wantSplitPoints: []string{"0999999", "1000000", "1000001"},
		},
		{
			desc:            "End Range Contains new Character",
			startRange:      "123",
			endRange:        "xyz",
			numSplits:       2,
			wantErr:         false,
			wantSplitPoints: []string{"4", "7"},
		},
		{
			desc:            "Start Range Contains new Character",
			startRange:      "abc",
			endRange:        "xyz",
			numSplits:       2,
			wantErr:         false,
			wantSplitPoints: []string{"b", "c"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			gotSplitPoints, gotErr := rangeSplitter.splitRange(tc.startRange, tc.endRange, tc.numSplits)
			if (gotErr != nil) != tc.wantErr {
				t.Errorf("SplitRange(%q, %q, %d) got error = %v, want error = %v",
					tc.startRange, tc.endRange, tc.numSplits, gotErr, tc.wantErr)
			}

			if diff := cmp.Diff(tc.wantSplitPoints, gotSplitPoints); diff != "" {
				t.Errorf("SplitRange(%q, %q, %d) returned unexpected diff (-want +got):\n%s",
					tc.startRange, tc.endRange, tc.numSplits, diff)
			}
		})
	}
}

func TestSortAlphabet(t *testing.T) {
	testCases := []struct {
		desc             string
		unsortedAlphabet []rune
		wantAphabet      *[]rune
	}{
		{
			desc:             "unsorted array",
			unsortedAlphabet: []rune{'8', '9', '7'},
			wantAphabet:      &[]rune{'7', '8', '9'},
		},
		{
			desc:             "one alphabet",
			unsortedAlphabet: []rune{'7'},
			wantAphabet:      &[]rune{'7'},
		},
		{
			desc:             "empty array",
			unsortedAlphabet: []rune{},
			wantAphabet:      &[]rune{},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			got := sortAlphabet(tc.unsortedAlphabet)
			if diff := cmp.Diff(tc.wantAphabet, got); diff != "" {
				t.Errorf("sortAlphabet(%q) returned unexpected diff (-want +got):\n%s", tc.unsortedAlphabet, diff)
			}
		})
	}
}

func TestConstructAlphabetMap(t *testing.T) {
	testCases := []struct {
		desc           string
		sortedAlphabet *[]rune
		wantMap        map[rune]int
	}{
		{
			desc:           "sorted array",
			sortedAlphabet: &[]rune{'7', '8', '9'},
			wantMap:        map[rune]int{'7': 0, '8': 1, '9': 2},
		},
		{
			desc:           "unsorted array",
			sortedAlphabet: &[]rune{'7', '9', '8'},
			wantMap:        map[rune]int{'7': 0, '9': 1, '8': 2},
		},
		{
			desc:           "one alphabet",
			sortedAlphabet: &[]rune{'7'},
			wantMap:        map[rune]int{'7': 0},
		},
		{
			desc:           "empty array",
			sortedAlphabet: &[]rune{},
			wantMap:        map[rune]int{},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			got := constructAlphabetMap(tc.sortedAlphabet)
			if diff := cmp.Diff(tc.wantMap, got); diff != "" {
				t.Errorf("constructAlphabetMap(%q) returned unexpected diff (-want +got):\n%s", tc.sortedAlphabet, diff)
			}
		})
	}
}

func TestCharPosition(t *testing.T) {
	testCases := []struct {
		desc      string
		character rune
		wantErr   bool
		wantPos   int
	}{
		{
			desc:      "no error",
			character: '7',
			wantErr:   false,
			wantPos:   0,
		},
		{
			desc:      "character not present",
			character: '6',
			wantErr:   true,
			wantPos:   -1,
		},
	}
	rs, err := newRangeSplitter("78898")
	if err != nil {
		t.Fatalf("Failed to initialize range splitter, err: %v", err)
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := rs.charPosition(tc.character)
			if (err != nil) != tc.wantErr {
				t.Errorf("charPosition(%q) got error = %v, want error = %v", tc.character, err, tc.wantErr)
			}
			if got != tc.wantPos {
				t.Errorf("charPosition(%q) got = %v, want = %v", tc.character, got, tc.wantPos)
			}
		})
	}
}
