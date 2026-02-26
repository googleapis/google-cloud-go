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

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "cloud.google.com/go/bigquery" // Implicitly required by test.
	"github.com/google/go-cmp/cmp"
	"golang.org/x/tools/go/packages"
)

var updateGoldens bool

func TestMain(m *testing.M) {
	flag.BoolVar(&updateGoldens, "update-goldens", false, "Update the golden files")
	flag.Parse()
	os.Exit(m.Run())
}

func TestParse(t *testing.T) {
	mod := "cloud.google.com/go/bigquery"
	r, err := parse(mod+"/...", ".", []string{"README.md"}, nil, &friendlyAPINamer{
		Fallbacks: map[string]string{
			"cloud.google.com/go/bigquery": "BigQuery API",
		},
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := len(r.toc), 1; got != want {
		t.Fatalf("Parse got len(toc) = %d, want %d", got, want)
	}
	if got, want := len(r.pages), 33; got < want {
		t.Errorf("Parse got len(pages) = %d, want at least %d", got, want)
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
	if updateGoldens {
		os.RemoveAll(gotDir)
		os.RemoveAll(goldenDir)
		gotDir = goldenDir
	}
	extraFiles := []string{"README.md"}

	testMod := indexEntry{Path: "cloud.google.com/go/storage", Version: "v1.33.0"}
	namer := &friendlyAPINamer{
		Fallbacks: map[string]string{
			"cloud.google.com/go/storage": "Storage API",
		},
	}
	ok := processMods([]indexEntry{testMod}, gotDir, namer, extraFiles, false)
	if !ok {
		t.Fatalf("failed to process modules")
	}

	ignoreGoldens := []string{fmt.Sprintf("%s@%s/docs.metadata", testMod.Path, testMod.Version)}

	if updateGoldens {
		for _, ignore := range ignoreGoldens {
			if err := os.Remove(filepath.Join(goldenDir, ignore)); err != nil {
				t.Fatalf("Remove: %v", err)
			}
		}
		t.Logf("Successfully updated goldens in %s", goldenDir)
		return
	}

	goldenCount := 0
	err := filepath.WalkDir(goldenDir, func(goldenPath string, d fs.DirEntry, err error) error {
		goldenCount++
		if d.IsDir() {
			return nil
		}
		if err != nil {
			return err
		}

		gotPath := filepath.Join(gotDir, goldenPath[len(goldenDir):])

		gotContent, err := os.ReadFile(gotPath)
		if err != nil {
			t.Fatalf("failed to read got: %v", err)
		}

		goldenContent, err := os.ReadFile(goldenPath)
		if err != nil {
			t.Fatalf("failed to read golden: %v", err)
		}

		if diff := cmp.Diff(goldenContent, gotContent); diff != "" {
			t.Errorf("diff with golden (%q,%q) (-want +got):\n\n%s", goldenPath, gotPath, diff)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed to compare goldens: %v", err)
	}
	gotCount := 0
	err = filepath.WalkDir(gotDir, func(_ string, _ fs.DirEntry, _ error) error {
		gotCount++
		return nil
	})
	if err != nil {
		t.Fatalf("failed to count got: %v", err)
	}
	if gotCount-len(ignoreGoldens) != goldenCount {
		t.Fatalf("processMods got %d files in %s, want %d ignoring %v", gotCount, gotDir, goldenCount, ignoreGoldens)
	}
}

func TestHasPrefix(t *testing.T) {
	tests := []struct {
		s        string
		prefixes []string
		want     bool
	}{
		{
			s:        "abc",
			prefixes: []string{"1", "a"},
			want:     true,
		},
		{
			s:        "abc",
			prefixes: []string{"1"},
			want:     false,
		},
		{
			s:        "abc",
			prefixes: []string{"1", "2"},
			want:     false,
		},
	}

	for _, test := range tests {
		if got := hasPrefix(test.s, test.prefixes); got != test.want {
			t.Errorf("hasPrefix(%q, %q) got %v, want %v", test.s, test.prefixes, got, test.want)
		}
	}
}

func TestWriteMetadata(t *testing.T) {
	now := time.Now()

	want := fmt.Sprintf(`update_time {
	seconds: %d
	nanos: %d
}
name: "cloud.google.com/go"
version: "100.0.0"
language: "go"
`, now.Unix(), now.Nanosecond())

	wantAppEngine := fmt.Sprintf(`update_time {
	seconds: %d
	nanos: %d
}
name: "google.golang.org/appengine/v2"
version: "2.0.0"
language: "go"
stem: "/appengine/docs/standard/go/reference/services/bundled"
`, now.Unix(), now.Nanosecond())

	tests := []struct {
		path    string
		version string
		want    string
	}{
		{
			path:    "cloud.google.com/go",
			version: "100.0.0",
			want:    want,
		},
		{
			path:    "google.golang.org/appengine/v2",
			version: "2.0.0",
			want:    wantAppEngine,
		},
	}
	for _, test := range tests {
		var buf bytes.Buffer
		module := &packages.Module{
			Path:    test.path,
			Version: test.version,
		}
		writeMetadata(&buf, now, module)
		if diff := cmp.Diff(test.want, buf.String()); diff != "" {
			t.Errorf("writeMetadata(%q) got unexpected diff (-want +got):\n\n%s", test.path, diff)
		}
	}
}

func TestGetStatus(t *testing.T) {
	tests := []struct {
		doc  string
		want string
	}{
		{
			doc: `Size returns the size of the object in bytes.
The returned value is always the same and is not affected by
calls to Read or Close.

Deprecated: use Reader.Attrs.Size.`,
			want: "deprecated",
		},
		{
			doc:  `This will never be deprecated!`,
			want: "",
		},
	}

	for _, test := range tests {
		if got := getStatus(test.doc); got != test.want {
			t.Errorf("getStatus(%v) got %q, want %q", test.doc, got, test.want)
		}
	}
}

func TestFriendlyAPIName(t *testing.T) {
	// 1. Root metadata
	storageDir := t.TempDir()
	meta := repoMetadata{Description: "Storage API"}
	b, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(storageDir, ".repo-metadata.json"), b, 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Sub-package metadata only
	subModuleDir := t.TempDir()
	subPkgDir := filepath.Join(subModuleDir, "apiv1")
	if err := os.MkdirAll(subPkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	subMeta := repoMetadata{Description: "Sub API"}
	subB, _ := json.Marshal(subMeta)
	if err := os.WriteFile(filepath.Join(subPkgDir, ".repo-metadata.json"), subB, 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Malformed metadata
	badDir := t.TempDir()
	badSubDir := filepath.Join(badDir, "apiv1")
	if err := os.MkdirAll(badSubDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badSubDir, ".repo-metadata.json"), []byte("invalid json"), 0644); err != nil {
		t.Fatal(err)
	}

	// 4. Submodule avoidance
	pubsubDir := t.TempDir()
	pubsubV2Dir := filepath.Join(pubsubDir, "v2")
	if err := os.MkdirAll(pubsubV2Dir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create go.mod in v2 to mark it as a separate module.
	if err := os.WriteFile(filepath.Join(pubsubV2Dir, "go.mod"), []byte("module cloud.google.com/go/pubsub/v2"), 0644); err != nil {
		t.Fatal(err)
	}
	v2Meta := repoMetadata{Description: "PubSub V2 API"}
	v2B, _ := json.Marshal(v2Meta)
	if err := os.WriteFile(filepath.Join(pubsubV2Dir, ".repo-metadata.json"), v2B, 0644); err != nil {
		t.Fatal(err)
	}

	namer := &friendlyAPINamer{}

	tests := []struct {
		importPath string
		module     *packages.Module
		want       string
		wantErr    bool
	}{
		{
			importPath: "cloud.google.com/go/storage",
			module: &packages.Module{
				Path: "cloud.google.com/go/storage",
				Dir:  storageDir,
			},
			want: "Storage API",
		},
		{
			importPath: "cloud.google.com/go/sub",
			module: &packages.Module{
				Path: "cloud.google.com/go/sub",
				Dir:  subModuleDir,
			},
			want: "Sub API",
		},
		{
			importPath: "cloud.google.com/go/storage/apiv1",
			module: &packages.Module{
				Path: "cloud.google.com/go/storage",
				Dir:  storageDir,
			},
			want: "Storage API v1",
		},
		{
			importPath: "cloud.google.com/go/bad",
			module: &packages.Module{
				Path: "cloud.google.com/go/bad",
				Dir:  badDir,
			},
			wantErr: true,
		},
		{
			importPath: "cloud.google.com/go/pubsub",
			module: &packages.Module{
				Path: "cloud.google.com/go/pubsub",
				Dir:  pubsubDir,
			},
			wantErr: true, // Should NOT find v2's metadata.
		},
		{
			importPath: "not found",
			module:     nil,
			wantErr:    true,
		},
	}

	for _, test := range tests {
		got, err := namer.friendlyAPIName(test.importPath, test.module)
		if (err != nil) != test.wantErr {
			t.Errorf("friendlyAPIName(%q) got err: %v, wantErr %v", test.importPath, err, test.wantErr)
			continue
		}
		if got != test.want {
			t.Errorf("friendlyAPIName(%q) got %q, want %q", test.importPath, got, test.want)
		}
	}
}
