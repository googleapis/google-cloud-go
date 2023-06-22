// Copyright 2023 Google LLC
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

package query

import "testing"

func TestDetectOrderedResults(t *testing.T) {
	testCases := []struct {
		query     string
		isOrdered bool
	}{
		{query: "SELECT 1 AS num FROM a ORDER BY num", isOrdered: true},
		{query: "SELECT 1 AS num FROM a", isOrdered: false},
		{query: "SELECT 1 AS num FROM (SELECT x FROM b ORDER BY x)", isOrdered: false},
		{query: "SELECT x AS num FROM (SELECT x FROM b ORDER BY x) a inner join b on b.x = a.x ORDER BY x", isOrdered: true},
		{query: "SELECT SUM(1) FROM a", isOrdered: false},
		{query: "SELECT COUNT(a.x) FROM a ORDER BY b", isOrdered: true},
		{query: "SELECT FOO(a.x, a.y) FROM a ORDER BY a", isOrdered: true},
		// Invalid queries
		{query: "SELECT 1 AS num FROM a (", isOrdered: false},
		{query: "SELECT 1 AS num FROM a )", isOrdered: false},
		// Script statements
		{query: "CALL foo()", isOrdered: false},
		{query: "DECLARE x INT64", isOrdered: false},
		{query: "SET x = 4;", isOrdered: false},
	}

	for _, tc := range testCases {
		got := HasOrderedResults(tc.query)
		if got != tc.isOrdered {
			if tc.isOrdered {
				t.Fatalf("expected query `%s` to be ordered", tc.query)
			} else {
				t.Fatalf("expected query `%s` to not be ordered", tc.query)
			}

		}
	}
}
