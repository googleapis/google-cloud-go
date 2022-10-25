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

package reader

import "testing"

func TestDetectOrderedResults(t *testing.T) {
	testCases := []struct {
		query     string
		isOrdered bool
	}{
		{query: "select 1 as num from a order by num", isOrdered: true},
		{query: "select 1 as num from a", isOrdered: false},
		{query: "select 1 as num from (select x from b order by x)", isOrdered: false},
		{query: "select x as num from (select x from b order by x) a inner join b on b.x = a.x order by x", isOrdered: true},
		{query: "select sum(1) from a", isOrdered: false},
		{query: "select count(a.x) from a order by", isOrdered: true},
		// Invalid queries
		{query: "select 1 as num from a (", isOrdered: false},
		//{query: "select 1 as num from a )", isOrdered: false},
	}

	for _, tc := range testCases {
		got := haveOrderedResults(tc.query)
		if got != tc.isOrdered {
			if tc.isOrdered {
				t.Fatalf("expected query `%s` to be ordered", tc.query)
			} else {
				t.Fatalf("expected query `%s` to not be ordered", tc.query)
			}

		}
	}
}
