// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and

package main

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestBumpSemverPatch(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{
			name: "full",
			in:   "v1.2.3",
			want: "v1.2.4",
		},
		{
			name: "minor",
			in:   "v0.1.2",
			want: "v0.1.3",
		},
		{
			name: "patch",
			in:   "v0.0.1",
			want: "v0.0.2",
		},
		{
			name: "prefix",
			in:   "foo/v1.2.3",
			want: "foo/v1.2.4",
		},
		{
			name: "longer prefix",
			in:   "foo/bar/v1.2.3",
			want: "foo/bar/v1.2.4",
		},
		{
			name: "release candidate",
			in:   "v1.2.3-rc1",
			want: "v1.2.4",
		},
		{
			name:    "invalid input major",
			in:      "vs.0.1",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := bumpSemverPatch(tt.in)
			if tt.wantErr && err == nil {
				t.Fatalf("bumpSemverPatch(%q) = nil, want error", tt.in)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("bumpSemverPatch(%q) = %v, wantErr false", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("bumpSemverPatch(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParsePkgName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "subpackage",
			in:   "cloud.google.com/go/asset",
			want: "asset",
		},
		{
			name: "nested package",
			in:   "cloud.google.com/go/dialogflow/cx",
			want: "cx",
		},
		{
			name: "root",
			in:   "cloud.google.com/go",
			want: "go",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mi := &modInfo{importPath: tt.in}
			got := mi.PkgName()
			if got != tt.want {
				t.Fatalf("&modInfo{importPath: %q}PkgName() = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSortTags(t *testing.T) {
	in := []string{"v0.12.1", "v1.3.2", "v0.1.3-rc1", "v0.1.3", "v1.1.2-rc1", "v2.1.3-rc1"}
	sortTags(in)
	want := []string{"v1.3.2", "v2.1.3-rc1", "v0.12.1", "v0.1.3", "v1.1.2-rc1", "v0.1.3-rc1"}
	if diff := cmp.Diff(want, in); diff != "" {
		t.Errorf("sortTags() mismatch (-want +got):\n%s", diff)
	}
}

func TestParseMetadata(t *testing.T) {
	data := `{
	"cloud.google.com/go/foo/apiv1": {
	  "description": "Foo API"
	},
	"cloud.google.com/go/foo/bar/apiv1beta": {
	  "description": "FooBar API"
	},
	"cloud.google.com/go/baz": {
		"description": "Baz API"
	}
}`
	m, err := parseMetadata(strings.NewReader(data))
	if err != nil {
		t.Fatalf("parseMetadata() = %v, want nil", err)
	}
	if key, want := "cloud.google.com/go/foo", "Foo API"; m[key] != want {
		t.Fatalf("m[%q] = %q, want %q", key, m[key], want)
	}
	if key, want := "cloud.google.com/go/foo/bar", "FooBar API"; m[key] != want {
		t.Fatalf("m[%q] = %q, want %q", key, m[key], want)
	}
	if key, want := "cloud.google.com/go/baz", "Baz API"; m[key] != want {
		t.Fatalf("m[%q] = %q, want %q", key, m[key], want)
	}
}
