// Copyright 2020 Google LLC
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

package generator

import "testing"

func TestFilterPackages(t *testing.T) {
	in := map[string][]string{
		"google.golang.org/genproto/googleapis/api/distribution":  {"foo.proto"},
		"google.golang.org/genproto/googleapis/type/date_range":   {"foo.proto"},
		"google.golang.org/genproto/googleapis/bigtable/admin/v2": {"foo.proto"},
		// Should be excluded.
		"google.golang.org/genproto/do/not/generate/me": {"foo.proto"},
	}
	want := map[string][]string{
		"google.golang.org/genproto/googleapis/api/distribution":  {"foo.proto"},
		"google.golang.org/genproto/googleapis/type/date_range":   {"foo.proto"},
		"google.golang.org/genproto/googleapis/bigtable/admin/v2": {"foo.proto"},
	}
	out, err := filterPackages(in)
	if err != nil {
		t.Fatal(err)
	}

	if len(out) != len(want) {
		t.Fatalf("expected %d packages got %d packages", len(want), len(out))
	}
	for p := range out {
		if _, ok := want[p]; !ok {
			t.Errorf("retained package that should have been removed: %q", p)
		}
	}

}
