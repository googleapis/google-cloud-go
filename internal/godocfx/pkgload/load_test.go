// Copyright 2021 Google LLC
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

package pkgload

import (
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestPkgStatus(t *testing.T) {
	tests := []struct {
		importPath string
		doc        string
		version    string
		want       string
	}{
		{
			importPath: "cloud.google.com/go",
			want:       "",
		},
		{
			importPath: "cloud.google.com/go/storage/v1alpha1",
			want:       "alpha",
		},
		{
			importPath: "cloud.google.com/go/storage/v2beta2",
			want:       "beta",
		},
		{
			doc:  "NOTE: This package is in beta. It is not stable, and may be subject to changes.",
			want: "beta",
		},
		{
			doc:  "NOTE: This package is in alpha. It is not stable, and is likely to change.",
			want: "alpha",
		},
		{
			doc:  "Package foo is great\nDeprecated: not anymore",
			want: "deprecated",
		},
		{
			importPath: "cloud.google.com/go/storage/v1alpha1",
			doc:        "Package foo is great\nDeprecated: not anymore",
			want:       "deprecated", // Deprecated comes before alpha and beta.
		},
		{
			importPath: "cloud.google.com/go/storage/v1beta1",
			doc:        "Package foo is great\nDeprecated: not anymore",
			want:       "deprecated", // Deprecated comes before alpha and beta.
		},
		{
			version: "v0.1.0",
			want:    "",
		},
		{
			version: "v2.1.0-alpha",
			want:    "preview", // Preview comes before alpha and beta.
		},
	}
	for _, test := range tests {
		if got := pkgStatus(test.importPath, test.doc, test.version); got != test.want {
			t.Errorf("pkgStatus(%q, %q, %q) got %q, want %q", test.importPath, test.doc, test.version, got, test.want)
		}
	}
}

func TestGetModulePath(t *testing.T) {
	tests := []struct {
		name string
		pkgs []*packages.Package
		want string
	}{
		{
			name: "empty",
			pkgs: []*packages.Package{},
			want: "",
		},
		{
			name: "no module",
			pkgs: []*packages.Package{
				{Module: nil},
			},
			want: "",
		},
		{
			name: "module no path",
			pkgs: []*packages.Package{
				{Module: &packages.Module{}},
			},
			want: "",
		},
		{
			name: "one with path",
			pkgs: []*packages.Package{
				{Module: &packages.Module{Path: "example.com/foo"}},
			},
			want: "example.com/foo",
		},
		{
			name: "many",
			pkgs: []*packages.Package{
				{Module: nil},
				{Module: &packages.Module{}},
				{Module: &packages.Module{Path: "example.com/bar"}},
			},
			want: "example.com/bar",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getModulePath(tt.pkgs); got != tt.want {
				t.Errorf("getModulePath() = %v, want %v", got, tt.want)
			}
		})
	}
}
