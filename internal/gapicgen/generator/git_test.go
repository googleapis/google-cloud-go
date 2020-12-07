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

func TestParseConventionalCommitPkg(t *testing.T) {
	tests := []struct {
		name       string
		importPath string
		want       string
	}{
		{name: "one path element", importPath: "cloud.google.com/go/foo/apiv1", want: "foo"},
		{name: "two path elements", importPath: "cloud.google.com/go/foo/bar/apiv1", want: "foo/bar"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseConventionalCommitPkg(tc.importPath); got != tc.want {
				t.Errorf("parseConventionalCommitPkg(%q) = %q, want %q", tc.importPath, got, tc.want)
			}
		})
	}
}
