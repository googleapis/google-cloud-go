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
	// 1. Setup initial state from source file
	state, err := parseLibrarianState("testdata/source/.librarian/state.yaml")
	if err != nil {
		t.Fatal(err)
	}

	// 2. Setup dummy module dir and files for "apihub"
	tmpDir := t.TempDir()
	apihubRoot := filepath.Join(tmpDir, "apihub")
	if err := os.MkdirAll(filepath.Join(apihubRoot, "apiv1"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(apihubRoot, "internal"), 0755); err != nil {
		t.Fatal(err)
	}
	versionFile := filepath.Join(apihubRoot, "internal", "version.go")
	versionContents := `
package internal

const Version = "0.2.0"
`
	if err := os.WriteFile(versionFile, []byte(versionContents), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Setup post-processor config for apihub
	ppc := &postProcessorConfig{
		ServiceConfigs: []*serviceConfigEntry{
			{
				ImportPath:     "cloud.google.com/go/apihub/apiv1",
				InputDirectory: "google/cloud/apihub/v1",
				ServiceConfig:  "apihub_v1.yaml",
			},
		},
	}

	// 4. Add the module
	if err := addModule(tmpDir, ppc, state, "apihub", "063f9e19c5890182920980ced75828fd7c0588a5"); err != nil {
		t.Fatal(err)
	}

	// 5. The save function sorts, so we sort here to compare.
	sortStateLibraries(state)

	// 6. Load golden file as "want"
	wantState, err := parseLibrarianState("testdata/golden/apihub/.librarian/state.yaml")
	if err != nil {
		t.Fatal(err)
	}

	// 7. Compare
	if diff := cmp.Diff(wantState.Libraries, state.Libraries); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
