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
	"testing"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
)

func TestPostProcess(t *testing.T) {
	tests := []struct {
		name                string
		newModule           bool
		mockexecvRun        func(ctx context.Context, args []string, dir string) error
		wantFilesCreated    []string
		wantFilesNotCreated []string
		wantGoModInitCalled bool
		wantGoModTidyCalled bool
		wantErr             bool
		noVersion           bool
	}{
		{
			name:      "new module success",
			newModule: true,
			mockexecvRun: func(ctx context.Context, args []string, dir string) error {
				return nil
			},
			wantFilesCreated: []string{
				"CHANGES.md",
				"README.md",
				"apiv1/version.go",
				"apiv2/version.go",
			},
			wantGoModInitCalled: true,
			wantGoModTidyCalled: true,
			wantErr:             false,
		},
		{
			name:      "existing module success",
			newModule: false,
			mockexecvRun: func(ctx context.Context, args []string, dir string) error {
				return nil
			},
			wantFilesCreated: []string{
				"README.md",
				"apiv1/version.go",
				"apiv2/version.go",
			},
			wantFilesNotCreated: []string{
				"go.mod",
				"CHANGES.md",
			},
			wantGoModInitCalled: true,
			wantGoModTidyCalled: true,
			wantErr:             false,
		},
		{
			name:      "goimports fails (fatal)",
			newModule: false,
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
			name:      "go mod init fails (fatal)",
			newModule: true,
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
			name:      "go mod tidy fails (fatal)",
			newModule: false,
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
				t.Fatalf("failed to create moduleDir %v", err)
				return
			}
			// Create the snippets dir that UpdateSnippetsMetadata expects.
			snippetsDir := filepath.Join(outputDir, "internal", "generated", "snippets", "chronicle")
			if err := os.MkdirAll(snippetsDir, 0755); err != nil {
				t.Fatalf("failed to create snippetsDir %v", err)
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
				},
				Version: "1.0.0",
			}

			if tt.noVersion {
				req.Version = ""
			}

			if err := PostProcess(context.Background(), req, outputDir, moduleDir, tt.newModule, "Chronicle API"); (err != nil) != tt.wantErr {
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

			for _, file := range tt.wantFilesCreated {
				if _, err := os.Stat(filepath.Join(moduleDir, file)); os.IsNotExist(err) {
					t.Errorf("file %s was not created", file)
				}
			}

			for _, file := range tt.wantFilesNotCreated {
				if _, err := os.Stat(filepath.Join(moduleDir, file)); !os.IsNotExist(err) {
					t.Errorf("file %s was created, but should not have been", file)
				}
			}
		})
	}
}
