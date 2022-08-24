// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build go1.18
// +build go1.18

package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var updateGoldens bool

func TestMain(m *testing.M) {
	flag.BoolVar(&updateGoldens, "update-goldens", false, "Update the golden files")
	flag.Parse()
	os.Exit(m.Run())
}

func TestGolden(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		modified bool
	}{
		{
			name:     "replace single import",
			fileName: "input1",
			modified: true,
		},
		{
			name:     "replace multi-import",
			fileName: "input2",
			modified: true,
		},
		{
			name:     "no replaces",
			fileName: "input3",
			modified: false,
		},
		{
			name:     "replace single, renamed matching new namespace",
			fileName: "input4",
			modified: true,
		},
		{
			name:     "replace multi-import, renamed non-matching",
			fileName: "input5",
			modified: true,
		},
		{
			name:     "not-migrated",
			fileName: "input6",
			modified: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m["example.com/old/foo"] = pkg{
				importPath: "example.com/new/foopb",
				migrated:   tc.modified,
			}
			var w bytes.Buffer
			if updateGoldens {
				if err := processFile(filepath.Join("testdata", tc.fileName), nil); err != nil {
					t.Fatal(err)
				}
				return
			}
			if err := processFile(filepath.Join("testdata", tc.fileName), &w); err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", "golden", tc.fileName))
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if !tc.modified {
				if len(w.Bytes()) != 0 {
					t.Fatalf("source modified:\n%s", w.Bytes())
				}
				return
			}
			if diff := cmp.Diff(want, w.Bytes()); diff != "" {
				t.Errorf("bytes mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
