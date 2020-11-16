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

func TestHasPrefix(t *testing.T) {
	tests := []struct {
		s        string
		prefixes []string
		want     bool
	}{
		{
			s:        "abc",
			prefixes: []string{"a"},
			want:     true,
		},
		{
			s:        "abc",
			prefixes: []string{"ab"},
			want:     true,
		},
		{
			s:        "abc",
			prefixes: []string{"abc"},
			want:     true,
		},
		{
			s:        "google.golang.org/genproto/googleapis/ads/googleads/v1/common",
			prefixes: []string{"google.golang.org/genproto/googleapis/ads"},
			want:     true,
		},
		{
			s:        "abc",
			prefixes: []string{"zzz"},
			want:     false,
		},
		{
			s:        "",
			prefixes: []string{"zzz"},
			want:     false,
		},
		{
			s:    "abc",
			want: false,
		},
	}
	for _, test := range tests {
		if got := hasPrefix(test.s, test.prefixes); got != test.want {
			t.Errorf("hasPrefix(%q, %q) got %v, want %v", test.s, test.prefixes, got, test.want)
		}
	}
}
