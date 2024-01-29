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
	"net/http"
	"net/http/httptest"
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

func fakeMetaServer() *httptest.Server {
	meta := repoMetadata{
		"cloud.google.com/go/storage": repoMetadataItem{
			Description: "Storage API",
		},
		"cloud.google.com/iam/apiv1beta1": repoMetadataItem{
			Description: "IAM",
		},
		"cloud.google.com/go/cloudbuild/apiv1/v2": repoMetadataItem{
			Description: "Cloud Build API",
		},
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(meta)
	}))
}

func TestParse(t *testing.T) {
	mod := "cloud.google.com/go/bigquery"
	metaServer := fakeMetaServer()
	defer metaServer.Close()
	r, err := parse(mod+"/...", ".", []string{"README.md"}, nil, &friendlyAPINamer{metaURL: metaServer.URL})
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
	metaServer := fakeMetaServer()
	defer metaServer.Close()
	namer := &friendlyAPINamer{metaURL: metaServer.URL}
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

		if string(gotContent) != string(goldenContent) {
			t.Errorf("got %s is different from expected %s", gotPath, goldenPath)
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
	metaServer := fakeMetaServer()
	defer metaServer.Close()
	namer := &friendlyAPINamer{metaURL: metaServer.URL}

	tests := []struct {
		importPath string
		want       string
	}{
		{
			importPath: "cloud.google.com/go/storage",
			want:       "Storage API",
		},
		{
			importPath: "cloud.google.com/iam/apiv1beta1",
			want:       "IAM v1beta1",
		},
		{
			importPath: "cloud.google.com/go/cloudbuild/apiv1/v2",
			want:       "Cloud Build API v1",
		},
		{
			importPath: "not found",
			want:       "",
		},
	}

	for _, test := range tests {
		got, err := namer.friendlyAPIName(test.importPath)
		if err != nil {
			t.Errorf("friendlyAPIName(%q) got err: %v", test.importPath, err)
			continue
		}
		if got != test.want {
			t.Errorf("friendlyAPIName(%q) got %q, want %q", test.importPath, got, test.want)
		}
	}
}
