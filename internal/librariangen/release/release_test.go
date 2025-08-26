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

package release

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestDirs(t *testing.T, initialRepoContent map[string]string, requestJSON string) (librarianDir, repoDir, outputDir string) {
	t.Helper()
	tmpDir := t.TempDir()

	librarianDir = filepath.Join(tmpDir, "librarian")
	repoDir = filepath.Join(tmpDir, "repo")
	outputDir = filepath.Join(tmpDir, "output")
	for _, dir := range []string{librarianDir, repoDir, outputDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
	}

	if err := os.WriteFile(filepath.Join(librarianDir, "release-init-request.json"), []byte(requestJSON), 0644); err != nil {
		t.Fatalf("failed to write request file: %v", err)
	}

	for path, content := range initialRepoContent {
		fullPath := filepath.Join(repoDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed to create repo content dir: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write repo content: %v", err)
		}
	}
	return
}

func assertVersion(t *testing.T, versionGoPath, wantVersion string) {
	t.Helper()
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, versionGoPath, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse version.go file: %v", err)
	}
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if valueSpec.Names[0].Name == "Version" {
				gotVersion := valueSpec.Values[0].(*ast.BasicLit).Value
				// trim quotes
				gotVersion = gotVersion[1 : len(gotVersion)-1]
				if gotVersion != wantVersion {
					t.Errorf("version.go Version = %q, want %q", gotVersion, wantVersion)
				}
				return
			}
		}
	}
	t.Errorf("could not find Version constant in version.go")
}

func TestInit(t *testing.T) {
	tests := []struct {
		name                string
		requestJSON         string
		initialRepoContent  map[string]string
		wantChangelogSubstr string
		wantVersion         string
		wantSnippetVersion  string
		wantErr             bool
		releaseNotTriggered bool
		alreadyUpToDate     bool
	}{
		{
			name: "success",
			requestJSON: `{ 
				"libraries": [{ 
					"id": "secretmanager", "version": "1.16.0", "release_triggered": true,
					"source_roots": ["secretmanager", "internal/generated/snippets/secretmanager"],
					"changes": [
						{"type": "feat", "subject": "add new GetSecret API"},
						{"type": "fix", "subject": "correct typo in documentation"}
					]
				}]
			}`,
			initialRepoContent: map[string]string{
				"secretmanager/CHANGES.md":                                           "# Changes\n\n## [1.15.0]\n- Old stuff.",
				"secretmanager/internal/version.go":                                  `package internal; const Version = "1.15.0"`,
				"internal/generated/snippets/secretmanager/snippet_metadata.v1.json": `{"version": "1.15.0"}`,
			},
			wantChangelogSubstr: "### 1.16.0",
			wantVersion:         "1.16.0",
			wantSnippetVersion:  `"version": "1.16.0"`,
		},
		{
			name:                "release not triggered",
			requestJSON:         `{ "libraries": [{"id": "secretmanager", "version": "1.16.0", "release_triggered": false}] }`,
			releaseNotTriggered: true,
		},
		{
			name:        "changelog already up-to-date",
			requestJSON: `{ "libraries": [ { "id": "secretmanager", "version": "1.16.0", "release_triggered": true, "source_roots": ["secretmanager", "internal/generated/snippets/secretmanager"], "changes": [{"type": "feat", "subject": "add new GetSecret API"}] } ] }`,
			initialRepoContent: map[string]string{
				"secretmanager/CHANGES.md": "# Changes\n\n### 1.16.0\n- Already there.",
				"internal/generated/snippets/secretmanager/snippet_metadata.v1.json": `{"version": "1.15.0"}`,
			},
			alreadyUpToDate: true,
		},
		{
			name:        "malformed json",
			requestJSON: `{"libraries": [}`,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			librarianDir, repoDir, outputDir := setupTestDirs(t, tt.initialRepoContent, tt.requestJSON)
			cfg := &Config{
				LibrarianDir: librarianDir,
				RepoDir:      repoDir,
				OutputDir:    outputDir,
			}

			err := Init(context.Background(), cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Init() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if tt.releaseNotTriggered {
				files, _ := os.ReadDir(outputDir)
				if len(files) > 0 {
					t.Errorf("outputDir should be empty, but got %d files", len(files))
				}
				return
			}

			if tt.alreadyUpToDate {
				content, err := os.ReadFile(filepath.Join(outputDir, "secretmanager/CHANGES.md"))
				if err != nil {
					t.Fatalf("failed to read changelog: %v", err)
				}
				if !strings.Contains(string(content), "- Already there.") {
					t.Errorf("changelog content was modified unexpectedly: got %q", string(content))
				}
				return
			}

			changelog, err := os.ReadFile(filepath.Join(outputDir, "secretmanager/CHANGES.md"))
			if err != nil {
				t.Fatalf("failed to read changelog: %v", err)
			}
			if !strings.Contains(string(changelog), tt.wantChangelogSubstr) {
				t.Errorf("changelog content = %q, want contains %q", string(changelog), tt.wantChangelogSubstr)
			}

			assertVersion(t, filepath.Join(outputDir, "secretmanager/internal/version.go"), tt.wantVersion)

			snippet, err := os.ReadFile(filepath.Join(outputDir, "internal/generated/snippets/secretmanager/snippet_metadata.v1.json"))
			if err != nil {
				t.Fatalf("failed to read snippet: %v", err)
			}
			if !strings.Contains(string(snippet), tt.wantSnippetVersion) {
				t.Errorf("snippet content = %q, want contains %q", string(snippet), tt.wantSnippetVersion)
			}
		})
	}
}
