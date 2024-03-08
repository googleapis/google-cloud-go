// Copyright 2024 Google LLC
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

package main

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var (
	update = flag.Bool("update", false, "update test goldens")
	keep   = flag.Bool("keep", false, "do not remove generated files")
)

func TestGeneration(t *testing.T) {
	ctx := context.Background()
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			t.Run(e.Name(), func(t *testing.T) {
				dir := filepath.Join("testdata", e.Name())
				configFile := filepath.Join(dir, "config.yaml")
				goldenFile := filepath.Join(dir, "golden")
				outFile := filepath.Join(dir, e.Name()+"_veneer.gen.go")
				if *keep {
					t.Logf("keeping %s", outFile)
				} else {
					defer os.Remove(outFile)
				}
				if err := run(ctx, configFile, dir, dir); err != nil {
					t.Fatal(err)
				}
				if *update {
					if err := os.Remove(goldenFile); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(outFile, goldenFile); err != nil {
						t.Fatal(err)
					}
					t.Logf("updated golden")
				} else {
					if diff := diffFiles(goldenFile, outFile); diff != "" {
						t.Errorf("diff (-want, +got):\n%s", diff)
					}
				}
			})
		}
	}
}

func diffFiles(wantFile, gotFile string) string {
	want, err := os.ReadFile(wantFile)
	if err != nil {
		return err.Error()
	}
	got, err := os.ReadFile(gotFile)
	if err != nil {
		return err.Error()
	}
	return cmp.Diff(string(want), string(got))
}

func TestCamelToUpperSnakeCase(t *testing.T) {
	for _, test := range []struct {
		in, want string
	}{
		{"foo", "FOO"},
		{"fooBar", "FOO_BAR"},
		{"aBC", "A_B_C"},
		{"ABC", "A_B_C"},
	} {
		got := camelToUpperSnakeCase(test.in)
		if got != test.want {
			t.Errorf("%q: got %q, want %q", test.in, got, test.want)
		}
	}
}

func TestAdjustDoc(t *testing.T) {
	const protoName = "PName"
	const veneerName = "VName"
	for i, test := range []struct {
		origDoc string
		verb    string
		newDoc  string
		want    string
	}{
		{
			origDoc: "",
			verb:    "foo",
			newDoc:  "",
			want:    "",
		},
		{
			origDoc: "",
			verb:    "",
			newDoc:  "is new doc.",
			want:    "VName is new doc.",
		},
		{
			origDoc: "The harm category is dangerous content.",
			verb:    "means",
			want:    "VName means the harm category is dangerous content.",
		},
		{
			origDoc: "URI for the file.",
			verb:    "is the",
			want:    "VName is the URI for the file.",
		},
		{
			origDoc: "PName is a thing.",
			newDoc:  "contains something else.",
			want:    "VName contains something else.",
		},
		{
			origDoc: "PName is a thing.",
			verb:    "ignored",
			want:    "VName is a thing.",
		},
	} {
		got := adjustDoc(test.origDoc, protoName, veneerName, test.verb, test.newDoc)
		if got != test.want {
			t.Errorf("#%d: got %q, want %q", i, got, test.want)
		}
	}
}
