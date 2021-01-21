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

// +build go1.15

package main

import (
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	_ "cloud.google.com/go/bigquery" // Implicitly required by test.
	_ "cloud.google.com/go/storage"  // Implicitly required by test.
)

var updateGoldens bool

func TestMain(m *testing.M) {
	flag.BoolVar(&updateGoldens, "update-goldens", false, "Update the golden files")
	flag.Parse()
	os.Exit(m.Run())
}

func TestParse(t *testing.T) {
	mod := "cloud.google.com/go/bigquery"
	r, err := parse(mod+"/...", ".", []string{"README.md"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := len(r.toc), 1; got != want {
		t.Fatalf("Parse got len(toc) = %d, want %d", got, want)
	}
	if got, want := len(r.pages), 10; got != want {
		t.Errorf("Parse got len(pages) = %d, want %d", got, want)
	}
	if got := r.module.Path; got != mod {
		t.Fatalf("Parse got module = %q, want %q", got, mod)
	}

	page := r.pages[mod]

	// Check invariants for every item.
	for _, item := range page.Items {
		if got := item.UID; got == "" {
			t.Errorf("Parse found missing UID: %v", item)
		}

		if got, want := item.Langs, []string{"go"}; len(got) != 1 || got[0] != want[0] {
			t.Errorf("Parse %v got langs = %v, want %v", item.UID, got, want)
		}
	}

	// Check there is at least one type, const, variable, function, and method.
	wants := []string{"type", "const", "variable", "function", "method"}
	for _, want := range wants {
		found := false
		for _, c := range page.Items {
			if c.Type == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Parse got no %q, want at least one", want)
		}
	}

	foundREADME := false
	foundUnnested := false
	for _, item := range r.toc[0].Items {
		if item.Name == "README" {
			foundREADME = true
		}
		if len(item.Items) == 0 && len(item.UID) > 0 && len(item.Name) > 0 {
			foundUnnested = true
		}
	}
	if !foundREADME {
		t.Errorf("Parse didn't find a README in TOC")
	}
	if !foundUnnested {
		t.Errorf("Parse didn't find an unnested element in TOC (e.g. datatransfer/apiv1)")
	}
}

func TestGoldens(t *testing.T) {
	gotDir := "testdata/out"
	goldenDir := "testdata/golden"
	extraFiles := []string{"README.md"}

	testPath := "cloud.google.com/go/storage"
	r, err := parse(testPath, ".", extraFiles)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	ignoreFiles := map[string]bool{"docs.metadata": true}

	if updateGoldens {
		os.RemoveAll(goldenDir)

		if err := write(goldenDir, r); err != nil {
			t.Fatalf("write: %v", err)
		}

		for ignore := range ignoreFiles {
			if err := os.Remove(filepath.Join(goldenDir, ignore)); err != nil {
				t.Fatalf("Remove: %v", err)
			}
		}

		t.Logf("Successfully updated goldens in %s", goldenDir)

		return
	}

	if err := write(gotDir, r); err != nil {
		t.Fatalf("write: %v", err)
	}

	gotFiles, err := ioutil.ReadDir(gotDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	goldens, err := ioutil.ReadDir(goldenDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	if got, want := len(gotFiles)-len(ignoreFiles), len(goldens); got != want {
		t.Fatalf("parse & write got %d files in %s, want %d ignoring %v", got, gotDir, want, ignoreFiles)
	}

	for _, golden := range goldens {
		if golden.IsDir() {
			continue
		}
		gotPath := filepath.Join(gotDir, golden.Name())
		goldenPath := filepath.Join(goldenDir, golden.Name())

		gotContent, err := ioutil.ReadFile(gotPath)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}

		goldenContent, err := ioutil.ReadFile(goldenPath)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}

		if string(gotContent) != string(goldenContent) {
			t.Errorf("got %s is different from expected %s", gotPath, goldenPath)
		}
	}
}
