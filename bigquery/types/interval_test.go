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
			wantInterval: &IntervalValue{Years: 1, Months: 2, Days: 3, Hours: 4, Mins: 5, Seconds: 6, SubSeconds: 0},
		},
		{
			inputStr:     "1-2 3 4:5:6.777",
			wantInterval: &IntervalValue{Years: 1, Months: 2, Days: 3, Hours: 4, Mins: 5, Seconds: 6, SubSeconds: 777},
		},
		{
			inputStr:     "-1-2 -3 -4:5:6",
			wantInterval: &IntervalValue{Years: -1, Months: 2, Days: -3, Hours: -4, Mins: 5, Seconds: 6, SubSeconds: 0},
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
