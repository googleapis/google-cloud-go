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

package postprocessor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/config"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/configure"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
)

func TestPostProcess(t *testing.T) {
	tests := []struct {
		name                     string
		mockexecvRun             func(ctx context.Context, args []string, dir string) error
		wantErr                  bool
		noVersion                bool
		singleNewAPI             bool
		snippetFiles             []string
		wantModifiedSnippetFiles []string
	}{
		{
			name: "success",
			mockexecvRun: func(ctx context.Context, args []string, dir string) error {
				baseDir := filepath.Base(dir)
				wantBaseDir := "output"
				if args[0] == "goimports" && baseDir != wantBaseDir {
					return fmt.Errorf("goimports failed, baseDir: got: %q, want: %q", baseDir, wantBaseDir)
				}
				return nil
			},
			wantErr: false,
			snippetFiles: []string{
				"apiv1/snippet_metadata.google.cloud.chronicle.v1.json",
				"apiv2/snippet_metadata.google.cloud.chronicle.v2.json",
				"sublevel1/sublevel2/apiv1/snippet_metadata.google.cloud.chronicle.sublevel1.sublevel2.v1.json",
				// This is *not* part of the request, so won't be modified.
				"apiv3/snippet_metadata.google.cloud.chronicle.v3.json",
			},
			wantModifiedSnippetFiles: []string{
				"apiv1/snippet_metadata.google.cloud.chronicle.v1.json",
				"apiv2/snippet_metadata.google.cloud.chronicle.v2.json",
				"sublevel1/sublevel2/apiv1/snippet_metadata.google.cloud.chronicle.sublevel1.sublevel2.v1.json",
			},
		},
		{
			name: "goimports fails (fatal)",
			mockexecvRun: func(ctx context.Context, args []string, dir string) error {
				if args[0] == "goimports" {
					return errors.New("goimports failed")
				}
				return nil
			},
			wantErr: true,
		},
		{
			name:      "fail without version",
			noVersion: true,
			wantErr:   true,
		},
		{
			name: "success with go mod init and tidy",
			mockexecvRun: func(ctx context.Context, args []string, dir string) error {
				return nil
			},
			wantErr:      false,
			singleNewAPI: true,
			snippetFiles: []string{
				"apiv1/snippet_metadata.google.cloud.chronicle.v1.json",
			},
			wantModifiedSnippetFiles: []string{
				"apiv1/snippet_metadata.google.cloud.chronicle.v1.json",
			},
		},
		{
			name: "go mod init fails",
			mockexecvRun: func(ctx context.Context, args []string, dir string) error {
				if len(args) > 2 && args[1] == "mod" && args[2] == "init" {
					return errors.New("go mod init failed")
				}
				return nil
			},
			wantErr:      true,
			singleNewAPI: true,
		},
		{
			name: "go mod tidy fails",
			mockexecvRun: func(ctx context.Context, args []string, dir string) error {
				if len(args) > 2 && args[1] == "mod" && args[2] == "tidy" {
					return errors.New("go mod tidy failed")
				}
				return nil
			},
			wantErr:      true,
			singleNewAPI: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			outputDir := filepath.Join(t.TempDir(), "output")
			moduleDir := filepath.Join(outputDir, "chronicle")
			if err := os.MkdirAll(moduleDir, 0755); err != nil {
				t.Fatalf("failed to create directory %s: %v", moduleDir, err)
				return
			}

			snippetsDir := filepath.Join(outputDir, "internal", "generated", "snippets", "chronicle")

			if err := createDirectories(t, moduleDir, snippetsDir, filepath.Join(snippetsDir, "sublevel1/sublevel2")); err != nil {
				// Specific failure will have been logged already.
				return
			}

			for _, snippetFile := range tt.snippetFiles {
				fullPath := filepath.Join(snippetsDir, snippetFile)
				specificSnippetDir := filepath.Dir(fullPath)
				if err := os.MkdirAll(specificSnippetDir, 0755); err != nil {
					t.Fatalf("failed to create directory %s: %v", moduleDir, err)
					return
				}
				content := "x\ny\nversion: $VERSION\na\nb\n"
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					t.Fatalf("failed to write file %s: %v", fullPath, err)
					return
				}
			}

			if err := os.MkdirAll(moduleDir, 0755); err != nil {
				t.Fatalf("failed to create moduleDir %v", err)
				return
			}

			var goModInitCalled, goModTidyCalled bool
			execvRun = func(ctx context.Context, args []string, dir string) error {
				if len(args) > 3 && args[1] == "mod" && args[2] == "init" && strings.HasPrefix(args[3], "cloud.google.com/go") {
					goModInitCalled = true
				}
				if len(args) > 2 && args[1] == "mod" && args[2] == "tidy" {
					goModTidyCalled = true
				}
				return tt.mockexecvRun(ctx, args, dir)
			}

			req := &request.Library{
				ID: "chronicle",
				APIs: []request.API{
					{Path: "google/cloud/chronicle/v1"},
					{Path: "google/cloud/chronicle/v2"},
					{Path: "google/cloud/chronicle/sublevel1/sublevel2/v1"},
				},
				Version: "1.0.0",
			}
			if tt.singleNewAPI {
				req = &request.Library{
					ID: "chronicle",
					APIs: []request.API{
						{Path: "google/cloud/chronicle/v1", Status: configure.NewAPIStatus},
					},
					Version: "0.0.0",
				}
			}

			if tt.noVersion {
				req.Version = ""
			}

			moduleConfig := &config.ModuleConfig{
				Name: "chronicle",
			}
			if err := PostProcess(context.Background(), req, outputDir, moduleDir, moduleConfig); (err != nil) != tt.wantErr {
				t.Fatalf("PostProcess() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.singleNewAPI && !tt.wantErr {
				if !goModInitCalled {
					t.Error("go mod init was not called")
				}
				if !goModTidyCalled {
					t.Error("go mod tidy was not called")
				}
			}
			if !tt.singleNewAPI {
				if goModInitCalled {
					t.Error("go mod init was called unexpectedly")
				}
				if goModTidyCalled {
					t.Error("go mod tidy was called unexpectedly")
				}
			}

			if tt.wantErr {
				return
			}

			// Determine which files should have been modified based on the request.
			for _, snippetFile := range tt.snippetFiles {
				path := filepath.Join(snippetsDir, snippetFile)
				read, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("Couldn't read snippet metadata file %s: %v", snippetFile, err)
				}
				wantModified := slices.Contains(tt.wantModifiedSnippetFiles, snippetFile)
				gotModified := strings.Contains(string(read), req.Version)
				if wantModified != gotModified {
					t.Errorf("incorrect snippet metadata modification for %s; got = %v; want = %v", snippetFile, gotModified, wantModified)
				}
			}
		})
	}
}

func createDirectories(t *testing.T, directories ...string) error {
	for _, directory := range directories {
		if err := os.MkdirAll(directory, 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", directory, err)
			return err
		}
	}
	return nil
}
