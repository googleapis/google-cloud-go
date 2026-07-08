// Copyright 2026 Google LLC
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

package bigquery

import "testing"

func TestRandomID(t *testing.T) {
	// A very simplistic collision test.
	colMap := make(map[string]struct{})
	for i := 0; i < 50000; i++ {
		id := randomID()
		if len(id) != 32 {
			t.Fatalf("anomalous ID len (%d): %q", len(id), id)
		}
		if _, ok := colMap[id]; ok {
			t.Fatalf("collision on id: %q", id)
		}
		colMap[id] = struct{}{}
	}
}
