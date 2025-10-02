// Copyright 2025 Google LLC
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

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestLoadVersion(t *testing.T) {
	tmpDir := t.TempDir()
	internalDir := filepath.Join(tmpDir, "internal")
	if err := os.Mkdir(internalDir, 0755); err != nil {
		t.Fatal(err)
	}
	versionFile := filepath.Join(internalDir, "version.go")
	versionContents := `
package internal

const Version = "1.2.3"
`
	if err := os.WriteFile(versionFile, []byte(versionContents), 0644); err != nil {
		t.Fatal(err)
	}
	version, err := loadVersion(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if version != "1.2.3" {
		t.Errorf("got %q, want %q", version, "1.2.3")
	}
}

func TestFindLatestGoogleapisCommit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"commit": {"sha": "my-sha"}}`)
	}))
	defer server.Close()
	googleapisURL = server.URL
	commit, err := findLatestGoogleapisCommit()
	if err != nil {
		t.Fatal(err)
	}
	if commit != "my-sha" {
		t.Errorf("got %q, want %q", commit, "my-sha")
	}
}

func TestAddModule(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "foo", "apiv1"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "foo", "internal"), 0755); err != nil {
		t.Fatal(err)
	}
	versionFile := filepath.Join(tmpDir, "foo", "internal", "version.go")
	versionContents := `
package internal

const Version = "1.2.3"
`
	if err := os.WriteFile(versionFile, []byte(versionContents), 0644); err != nil {
		t.Fatal(err)
	}
	ppc := &postProcessorConfig{
		ServiceConfigs: []*serviceConfigEntry{
			{
				ImportPath:     "cloud.google.com/go/foo/apiv1",
				InputDirectory: "google/cloud/foo/v1",
			},
		},
	}
	state := &LibrarianState{}
	if err := addModule(tmpDir, ppc, state, "foo", "my-commit"); err != nil {
		t.Fatal(err)
	}
	if len(state.Libraries) != 1 {
		t.Fatalf("got %d libraries, want 1", len(state.Libraries))
	}
	lib := state.Libraries[0]
	want := &LibraryState{
		ID:                  "foo",
		Version:             "1.2.3",
		LastGeneratedCommit: "my-commit",
		APIs: []*API{
			{
				Path: "google/cloud/foo/v1",
			},
		},
		SourceRoots: []string{
			"foo",
			"internal/generated/snippets/foo",
		},
		RemoveRegex: []string{
			"^internal/generated/snippets/foo/",
			"^foo/apiv1/[^/]*_client\\.go$",
			"^foo/apiv1/[^/]*_client_example_go123_test\\.go$",
			"^foo/apiv1/[^/]*_client_example_test\\.go$",
			"^foo/apiv1/auxiliary\\.go$",
			"^foo/apiv1/auxiliary_go123\\.go$",
			"^foo/apiv1/doc\\.go$",
			"^foo/apiv1/gapic_metadata\\.json$",
			"^foo/apiv1/helpers\\.go$",
			"^foo/apiv1/foopb/.*$",
		},
		ReleaseExcludePaths: []string{
			"internal/generated/snippets/foo/",
		},
		TagFormat: "{id}/v{version}",
	}
	// Don't compare RemoveRegex because the order is not guaranteed.
	removeRegex := lib.RemoveRegex
	lib.RemoveRegex = nil
	want.RemoveRegex = nil
	if diff := cmp.Diff(want, lib); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
	wantRemoveRegex := []string{
		"^internal/generated/snippets/foo/",
		"^foo/apiv1/[^/]*_client\\.go$",
		"^foo/apiv1/[^/]*_client_example_go123_test\\.go$",
		"^foo/apiv1/[^/]*_client_example_test\\.go$",
		"^foo/apiv1/auxiliary\\.go$",
		"^foo/apiv1/auxiliary_go123\\.go$",
		"^foo/apiv1/doc\\.go$",
		"^foo/apiv1/gapic_metadata\\.json$",
		"^foo/apiv1/helpers\\.go$",
		"^foo/apiv1/foopb/.*$",
	}
	if diff := cmp.Diff(wantRemoveRegex, removeRegex); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
