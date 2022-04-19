// Copyright 2022 Google LLC
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

package types

import (
	"testing"

	"cloud.google.com/go/internal/testutil"
)

func TestParseInterval(t *testing.T) {
	testcases := []struct {
		inputStr     string
		wantInterval *IntervalValue
		wantErr      bool
	}{
		{
			inputStr: "",
			wantErr:  true,
		},
		{
			inputStr: "1-2 3",
			wantErr:  true,
		},
		{
			inputStr:     "1-2 3 4:5:6",
			wantInterval: &IntervalValue{Years: 1, Months: 2, Days: 3, Hours: 4, Minutes: 5, Seconds: 6, SubSeconds: 0},
		},
		{
			inputStr:     "1-2 3 4:5:6.777",
			wantInterval: &IntervalValue{Years: 1, Months: 2, Days: 3, Hours: 4, Minutes: 5, Seconds: 6, SubSeconds: 777},
		},
		{
			inputStr:     "-1-2 -3 -4:5:6",
			wantInterval: &IntervalValue{Years: -1, Months: 2, Days: -3, Hours: -4, Minutes: 5, Seconds: 6, SubSeconds: 0},
		},
	}

	for _, tc := range testcases {
		gotInterval, err := ParseInterval(tc.inputStr)
		if tc.wantErr {
			if err != nil {
				continue
			}
			t.Errorf("input %s: wanted err, got success", tc.inputStr)
		}
		if err != nil {
			t.Errorf("input %s got err: %v", tc.inputStr, err)
		}
		if diff := testutil.Diff(gotInterval, tc.wantInterval); diff != "" {
			t.Errorf("input %s: got=-, want=+:\n%s", tc.inputStr, diff)
		}
	}
}

func TestCanonicalInterval(t *testing.T) {
	testcases := []struct {
		description   string
		input         *IntervalValue
		wantCanonical *IntervalValue
		wantString    string
	}{
		{
			description:   "already canonical",
			input:         &IntervalValue{Years: 1, Months: 2, Days: 3, Hours: 4, Minutes: 5, Seconds: 6, SubSeconds: 0},
			wantCanonical: &IntervalValue{Years: 1, Months: 2, Days: 3, Hours: 4, Minutes: 5, Seconds: 6, SubSeconds: 0},
			wantString:    "1-2 3 4:5:6",
		},
		{
			description:   "mixed Y-M",
			input:         &IntervalValue{Years: -1, Months: 28},
			wantCanonical: &IntervalValue{Years: 1, Months: 4, Days: 0, Hours: 0, Minutes: 0, Seconds: 0, SubSeconds: 0},
			wantString:    "1-4 0 0:0:0",
		},
		{
			description:   "mixed Y-M",
			input:         &IntervalValue{Years: -1, Months: 28},
			wantCanonical: &IntervalValue{Years: 1, Months: 4, Days: 0, Hours: 0, Minutes: 0, Seconds: 0, SubSeconds: 0},
			wantString:    "1-4 0 0:0:0",
		},
		{
			description:   "big month Y-M",
			input:         &IntervalValue{Years: 0, Months: -13},
			wantCanonical: &IntervalValue{Years: -1, Months: -1, Days: 0, Hours: 0, Minutes: 0, Seconds: 0, SubSeconds: 0},
			wantString:    "-1-1 0 0:0:0",
		},
		{
			description:   "big days not normalized",
			input:         &IntervalValue{Days: 1000},
			wantCanonical: &IntervalValue{Years: 0, Months: 0, Days: 1000, Hours: 0, Minutes: 0, Seconds: 0, SubSeconds: 0},
			wantString:    "0-0 1000 0:0:0",
		},
	}

	for _, tc := range testcases {
		gotCanonical := tc.input.Canonicalize()

		if diff := testutil.Diff(gotCanonical, tc.wantCanonical); diff != "" {
			t.Errorf("%s: got=-, want=+:\n%s", tc.description, diff)
		}

		gotStr := tc.input.String()
		if gotStr != tc.wantString {
			t.Errorf("%s mismatched strings. got %s want %s", tc.description, gotStr, tc.wantString)
		}
	}
}
