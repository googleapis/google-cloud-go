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

package git

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

func TestFormatChanges(t *testing.T) {
	tests := []struct {
		name       string
		changes    []*ChangeInfo
		onlyGapics bool
		want       string
	}{
		{
			name:    "basic",
			changes: []*ChangeInfo{{Title: "fix: foo", Body: "bar"}},
			want:    "\nChanges:\n\nfix: foo\n  bar\n\n",
		},
		{
			name:    "breaking change",
			changes: []*ChangeInfo{{Title: "feat!: breaking change", Body: "BREAKING CHANGE: The world is breaking."}},
			want:    "\nChanges:\n\nfeat!: breaking change\n  BREAKING CHANGE: The world is breaking.\n\n",
		},
		{
			name:    "multi-lined body indented",
			changes: []*ChangeInfo{{Title: "fix: foo", Body: "bar\nbaz"}},
			want:    "\nChanges:\n\nfix: foo\n  bar\n  baz\n\n",
		},
		{
			name:    "multi-lined body indented, multiple changes",
			changes: []*ChangeInfo{{Title: "fix: foo", Body: "bar\nbaz"}, {Title: "fix: baz", Body: "foo\nbar"}},
			want:    "\nChanges:\n\nfix: foo\n  bar\n  baz\n\nfix: baz\n  foo\n  bar\n\n",
		},
		{
			name:       "no package, filtered",
			changes:    []*ChangeInfo{{Title: "fix: foo", Body: "bar"}},
			onlyGapics: true,
			want:       "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatChanges(tc.changes, tc.onlyGapics); got != tc.want {
				t.Errorf("%s: FormatChanges() = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestFormatChanges_MaxChangesLen(t *testing.T) {
	tests := []struct {
		name          string
		changes       []*ChangeInfo
		maxChangesLen int
		want          string
	}{
		{
			name:          "truncate long title",
			changes:       []*ChangeInfo{{Title: "fix: Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod", Body: "tempor incididunt ut\nPiperOrigin-RevId: bar"}},
			maxChangesLen: 100,
			want:          "\nChanges:\n\nfix: Lorem ipsum dolor...\n  PiperOrigin-RevId: bar\n\n",
		},
		{
			name:          "no truncate short title",
			changes:       []*ChangeInfo{{Title: "fix: Lorem ipsum dolor", Body: "tempor incididunt ut\n PiperOrigin-RevId: bar"}},
			maxChangesLen: 50,
			want:          "\nChanges:\n\nfix: Lorem ipsum dolor\n  PiperOrigin-RevId: bar\n\n",
		},
	}

	for _, tc := range tests {
		maxChangesLen = tc.maxChangesLen
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatChanges(tc.changes, false); got != tc.want {
				t.Errorf("%s: FormatChanges() = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}
