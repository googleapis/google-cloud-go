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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/config"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
)

func TestPostProcess(t *testing.T) {
	tests := []struct {
		name                string
		mockexecvRun        func(ctx context.Context, args []string, dir string) error
		wantGoModInitCalled bool
		wantGoModTidyCalled bool
		wantErr             bool
		noVersion           bool
	}{
		{
			name: "success",
			mockexecvRun: func(ctx context.Context, args []string, dir string) error {
				return nil
			},
			wantGoModInitCalled: true,
			wantGoModTidyCalled: true,
			wantErr:             false,
		},
		{
			name: "goimports fails (fatal)",
			mockexecvRun: func(ctx context.Context, args []string, dir string) error {
				if args[0] == "goimports" {
					return errors.New("goimports failed")
				}
				return nil
			},
			wantGoModInitCalled: true,
			wantGoModTidyCalled: false,
			wantErr:             true,
		},
		{
			name: "go mod init fails (fatal)",
			mockexecvRun: func(ctx context.Context, args []string, dir string) error {
				if args[0] == "go" && args[1] == "mod" && args[2] == "init" {
					return errors.New("go mod init failed")
				}
				return nil
			},
			wantGoModInitCalled: true,
			wantGoModTidyCalled: false,
			wantErr:             true,
		},
		{
			name: "go mod tidy fails (fatal)",
			mockexecvRun: func(ctx context.Context, args []string, dir string) error {
				if args[0] == "go" && args[1] == "mod" && args[2] == "tidy" {
					return errors.New("go mod tidy failed")
				}
				return nil
			},
			wantGoModInitCalled: true,
			wantGoModTidyCalled: true,
			wantErr:             true,
		},
		{
			name:                "fail without version",
			noVersion:           true,
			wantGoModInitCalled: false,
			wantGoModTidyCalled: false,
			wantErr:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputDir := t.TempDir()
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

			snippetMetadataFiles := []string{
				"apiv1/snippet_metadata.google.cloud.chronicle.v1.json",
				"apiv2/snippet_metadata.google.cloud.chronicle.v2.json",
				"sublevel1/sublevel2/apiv1/snippet_metadata.google.cloud.chronicle.sublevel1.sublevel2.v1.json",
				// This is *not* part of the request, so won't be modified.
				"apiv3/snippet_metadata.google.cloud.chronicle.v3.json",
			}
			for _, snippetMetadataFile := range snippetMetadataFiles {
				fullPath := filepath.Join(snippetsDir, snippetMetadataFile)
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
				if len(args) > 2 && args[0] == "go" && args[1] == "mod" && args[2] == "init" {
					goModInitCalled = true
				}
				if len(args) > 2 && args[0] == "go" && args[1] == "mod" && args[2] == "tidy" {
					goModTidyCalled = true
				}
				return tt.mockexecvRun(ctx, args, dir)
			}

			req := &request.Request{
				ID: "chronicle",
				APIs: []request.API{
					{Path: "google/cloud/chronicle/v1"},
					{Path: "google/cloud/chronicle/v2"},
					{Path: "google/cloud/chronicle/sublevel1/sublevel2/v1"},
				},
				Version: "1.0.0",
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

			if tt.wantErr {
				return
			}

			if goModInitCalled != tt.wantGoModInitCalled {
				t.Errorf("goModInitCalled = %v; want %v", goModInitCalled, tt.wantGoModInitCalled)
			}
			if goModTidyCalled != tt.wantGoModTidyCalled {
				t.Errorf("goModTidyCalled = %v; want %v", goModTidyCalled, tt.wantGoModTidyCalled)
			}

			for _, snippetMetadataFile := range snippetMetadataFiles {
				path := filepath.Join(snippetsDir, snippetMetadataFile)
				read, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("Couldn't read snippet metadata file %s: %v", snippetMetadataFile, err)
				}
				wantModified := !strings.Contains(snippetMetadataFile, "v3")
				gotModified := strings.Contains(string(read), req.Version)
				if wantModified != gotModified {
					t.Errorf("incorrect snippet metadata modification for %s; got = %v; want = %v", snippetMetadataFile, gotModified, wantModified)
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
