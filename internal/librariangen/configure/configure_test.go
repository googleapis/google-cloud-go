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

package configure

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/execv"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
	"github.com/google/go-cmp/cmp"
)

// testEnv encapsulates a temporary test environment.
type testEnv struct {
	tmpDir       string
	librarianDir string
	inputDir     string
	outputDir    string
	sourceDir    string
	repoDir      string
}

// newTestEnv creates a new test environment.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "configure-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	e := &testEnv{tmpDir: tmpDir}
	e.librarianDir = filepath.Join(tmpDir, "librarian")
	e.inputDir = filepath.Join(tmpDir, "input")
	e.outputDir = filepath.Join(tmpDir, "output")
	e.sourceDir = filepath.Join(tmpDir, "source")
	e.repoDir = filepath.Join(tmpDir, "repo")
	for _, dir := range []string{e.librarianDir, e.inputDir, e.outputDir, e.sourceDir, e.repoDir} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatalf("failed to create dir %s: %v", dir, err)
		}
	}
	return e
}

// cleanup removes the temporary directory.
func (e *testEnv) cleanup(t *testing.T) {
	t.Helper()
	if err := os.RemoveAll(e.tmpDir); err != nil {
		t.Fatalf("failed to remove temp dir: %v", err)
	}
}

// writeRequestFile writes a configure-request.json file.
func (e *testEnv) writeRequestFile(t *testing.T, content string) {
	t.Helper()
	p := filepath.Join(e.librarianDir, "configure-request.json")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write request file: %v", err)
	}
}

// writeConfigFile writes a config.yaml file.
func (e *testEnv) writeConfigFile(t *testing.T) {
	t.Helper()
	// An empty config file is valid.
	p := filepath.Join(e.librarianDir, "config.yaml")
	if err := os.WriteFile(p, nil, 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}
}

// writeServiceYAML writes a service.yaml file.
func (e *testEnv) writeServiceYAML(t *testing.T, apiPath, title string) {
	t.Helper()
	apiDir := filepath.Join(e.sourceDir, apiPath)
	if err := os.MkdirAll(apiDir, 0755); err != nil {
		t.Fatalf("failed to create api dir: %v", err)
	}
	content := "title: " + title
	p := filepath.Join(apiDir, "service.yaml")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write service yaml: %v", err)
	}
}

// writeRepoGoMod writes the go.mod file that is expected to be in the repo.
func (e *testEnv) writeRepoGoMod(t *testing.T) {
	t.Helper()
	repoSnippetsDir := filepath.Join(e.repoDir, "internal", "generated", "snippets")
	if err := os.MkdirAll(repoSnippetsDir, 0755); err != nil {
		t.Fatalf("failed to create repo snippets dir: %v", err)
	}
	p := filepath.Join(repoSnippetsDir, "go.mod")
	if err := os.WriteFile(p, []byte("module fake.repo.go.mod"), 0644); err != nil {
		t.Fatalf("failed to write repo go.mod: %v", err)
	}
}

// assertFileExists checks if a file exists in the output directory.
func (e *testEnv) assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(e.outputDir, path)); err != nil {
		t.Errorf("file %s does not exist", path)
	}
}

// assertFileNotExist checks if a file does not exist in the output directory.
func (e *testEnv) assertFileNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(e.outputDir, path)); !os.IsNotExist(err) {
		t.Errorf("file %s exists but should not", path)
	}
}

func extractPackageName(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "package ") {
			return strings.TrimPrefix(line, "package ")
		}
	}
	return ""
}

func TestConfigure(t *testing.T) {
	newLibraryRequest := `{
		"libraries": [{
			"id": "capacityplanner",
			"apis": [{
				"path": "google/cloud/capacityplanner/v1beta",
				"service_config": "service.yaml",
				"status": "new"
			}]
		}]
	}`
	updateLibraryRequest := `{
		"libraries": [{
			"id": "secretmanager",
			"apis": [
				{"path": "google/cloud/secretmanager/v1", "status": "existing"},
				{"path": "google/cloud/secretmanager/v1beta2", "service_config": "service.yaml", "status": "new"}
			]
		}]
	}`
	wantNewLibraryResponse := &request.Library{
		ID:      "capacityplanner",
		Version: "0.0.0",
		APIs: []request.API{
			{Path: "google/cloud/capacityplanner/v1beta", ServiceConfig: "service.yaml", Status: "new"},
		},
		SourcePaths: []string{"capacityplanner", "internal/generated/snippets/capacityplanner"},
		RemoveRegex: []string{
			"^internal/generated/snippets/capacityplanner/",
			"^capacityplanner/apiv1beta/[^/]*_client\\.go$",
			"^capacityplanner/apiv1beta/[^/]*_client_example_go123_test\\.go$",
			"^capacityplanner/apiv1beta/[^/]*_client_example_test\\.go$",
			"^capacityplanner/apiv1beta/auxiliary\\.go$",
			"^capacityplanner/apiv1beta/auxiliary_go123\\.go$",
			"^capacityplanner/apiv1beta/doc\\.go$",
			"^capacityplanner/apiv1beta/gapic_metadata\\.json$",
			"^capacityplanner/apiv1beta/helpers\\.go$",
			"^capacityplanner/apiv1beta/\\.repo-metadata\\.json$",
			"^capacityplanner/apiv1beta/capacityplannerpb/.*$",
		},
		TagFormat: "{id}/v{version}",
	}
	wantUpdateLibraryResponse := &request.Library{
		ID: "secretmanager",
		APIs: []request.API{
			{Path: "google/cloud/secretmanager/v1", Status: "existing"},
			{Path: "google/cloud/secretmanager/v1beta2", ServiceConfig: "service.yaml", Status: "new"},
		},
		RemoveRegex: []string{
			"^secretmanager/apiv1beta2/[^/]*_client\\.go$",
			"^secretmanager/apiv1beta2/[^/]*_client_example_go123_test\\.go$",
			"^secretmanager/apiv1beta2/[^/]*_client_example_test\\.go$",
			"^secretmanager/apiv1beta2/auxiliary\\.go$",
			"^secretmanager/apiv1beta2/auxiliary_go123\\.go$",
			"^secretmanager/apiv1beta2/doc\\.go$",
			"^secretmanager/apiv1beta2/gapic_metadata\\.json$",
			"^secretmanager/apiv1beta2/helpers\\.go$",
			"^secretmanager/apiv1beta2/\\.repo-metadata\\.json$",
			"^secretmanager/apiv1beta2/secretmanagerpb/.*$",
		},
	}

	tests := []struct {
		name                 string
		clientPath           string
		setup                func(e *testEnv, t *testing.T)
		execvErr             error
		wantErr              bool
		wantExecvRunCount    int
		wantResponse         *request.Library
		wantFiles            []string
		wantNotFiles         []string
		wantVersionGoPackage string
	}{
		{
			name: "happy path new library",
			setup: func(e *testEnv, t *testing.T) {
				e.writeRequestFile(t, newLibraryRequest)
				e.writeConfigFile(t)
				e.writeServiceYAML(t, "google/cloud/capacityplanner/v1beta", "Capacity Planner API")
				e.writeRepoGoMod(t)
			},
			wantErr:           false,
			wantExecvRunCount: 1,
			wantResponse:      wantNewLibraryResponse,
			wantFiles: []string{
				"capacityplanner/README.md",
				"capacityplanner/CHANGES.md",
				"capacityplanner/internal/version.go",
				"capacityplanner/apiv1beta/version.go",
				"internal/generated/snippets/go.mod",
			},
		},
		{
			name: "happy path update library",
			setup: func(e *testEnv, t *testing.T) {
				e.writeRequestFile(t, updateLibraryRequest)
				e.writeConfigFile(t)
				e.writeServiceYAML(t, "google/cloud/secretmanager/v1beta2", "Secret Manager API")
			},
			wantErr:           false,
			wantExecvRunCount: 0,
			wantResponse:      wantUpdateLibraryResponse,
			wantFiles: []string{
				"secretmanager/apiv1beta2/version.go",
			},
			wantNotFiles: []string{
				"secretmanager/README.md",
				"secretmanager/CHANGES.md",
				"secretmanager/internal/version.go",
			},
		},
		{
			name:       "happy path new API with complex client directory",
			clientPath: "saasplatform/saasservicemgmt/apiv1beta1",
			setup: func(e *testEnv, t *testing.T) {
				e.writeRequestFile(t, `{
					"libraries": [{
						"id": "saasplatform",
						"apis": [
							{"path": "google/cloud/saasplatform/saasservicemgmt/v1beta1", "service_config": "service.yaml", "status": "new"}
						]
					}]
				}`)
				e.writeConfigFile(t)
				e.writeServiceYAML(t, "google/cloud/saasplatform/saasservicemgmt/v1beta1", "SaaS Service Management API")
				e.writeRepoGoMod(t)
			},
			wantErr:           false,
			wantExecvRunCount: 1,
			wantResponse: &request.Library{
				ID:      "saasplatform",
				Version: "0.0.0",
				APIs: []request.API{
					{Path: "google/cloud/saasplatform/saasservicemgmt/v1beta1", ServiceConfig: "service.yaml", Status: "new"},
				},
				SourcePaths: []string{"saasplatform", "internal/generated/snippets/saasplatform"},
				RemoveRegex: []string{
					"^internal/generated/snippets/saasplatform/",
					"^saasplatform/saasservicemgmt/apiv1beta1/[^/]*_client\\.go$",
					"^saasplatform/saasservicemgmt/apiv1beta1/[^/]*_client_example_go123_test\\.go$",
					"^saasplatform/saasservicemgmt/apiv1beta1/[^/]*_client_example_test\\.go$",
					"^saasplatform/saasservicemgmt/apiv1beta1/auxiliary\\.go$",
					"^saasplatform/saasservicemgmt/apiv1beta1/auxiliary_go123\\.go$",
					"^saasplatform/saasservicemgmt/apiv1beta1/doc\\.go$",
					"^saasplatform/saasservicemgmt/apiv1beta1/gapic_metadata\\.json$",
					"^saasplatform/saasservicemgmt/apiv1beta1/helpers\\.go$",
					"^saasplatform/saasservicemgmt/apiv1beta1/\\.repo-metadata\\.json$",
					"^saasplatform/saasservicemgmt/apiv1beta1/saasservicemgmtpb/.*$",
				},
				TagFormat: "{id}/v{version}",
			},
			wantFiles: []string{
				"saasplatform/README.md",
				"saasplatform/CHANGES.md",
				"saasplatform/internal/version.go",
				"saasplatform/saasservicemgmt/apiv1beta1/version.go",
				"internal/generated/snippets/go.mod",
			},
			wantNotFiles:         []string{},
			wantVersionGoPackage: "saasservicemgmt",
		}, {
			name: "execv fails",
			setup: func(e *testEnv, t *testing.T) {
				e.writeRequestFile(t, newLibraryRequest)
				e.writeConfigFile(t)
				e.writeServiceYAML(t, "google/cloud/capacityplanner/v1beta", "Capacity Planner API")
				e.writeRepoGoMod(t)
			},
			execvErr:          errors.New("go mod edit failed"),
			wantErr:           true,
			wantExecvRunCount: 1,
		},
		{
			name: "missing service yaml",
			setup: func(e *testEnv, t *testing.T) {
				e.writeRequestFile(t, newLibraryRequest)
				e.writeConfigFile(t)
				e.writeRepoGoMod(t)
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEnv(t)
			defer e.cleanup(t)

			tt.setup(e, t)

			var execvRunCount int
			execvRun = func(ctx context.Context, args []string, dir string) error {
				execvRunCount++
				return tt.execvErr
			}
			t.Cleanup(func() { execvRun = execv.Run })

			var gotResponse *request.Library
			responseSave = func(resp *request.Library, path string) error {
				gotResponse = resp
				// Write a dummy file to satisfy the test script's file check.
				return os.WriteFile(path, []byte("{}"), 0644)
			}
			t.Cleanup(func() { responseSave = saveResponseImpl })

			// The real parse function is used, as its behavior is simple.
			requestParse = Parse
			t.Cleanup(func() { requestParse = Parse })

			cfg := &Config{
				LibrarianDir: e.librarianDir,
				InputDir:     e.inputDir,
				OutputDir:    e.outputDir,
				SourceDir:    e.sourceDir,
				RepoDir:      e.repoDir,
			}

			if err := Configure(context.Background(), cfg); (err != nil) != tt.wantErr {
				t.Fatalf("Configure() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			if execvRunCount != tt.wantExecvRunCount {
				t.Errorf("execvRun called = %d; want %d", execvRunCount, tt.wantExecvRunCount)
			}

			if diff := cmp.Diff(tt.wantResponse, gotResponse); diff != "" {
				t.Fatalf("Configure() response mismatch (-want +got):\n%s", diff)
			}

			if tt.wantVersionGoPackage != "" {
				versionGoPath := filepath.Join(e.outputDir, tt.clientPath, "version.go")
				content, err := os.ReadFile(versionGoPath)
				if err != nil {
					t.Fatalf("failed to read version.go: %v", err)
				}
				gotPackage := extractPackageName(string(content))
				if gotPackage != tt.wantVersionGoPackage {
					t.Errorf("version.go package mismatch: got %q, want %q", gotPackage, tt.wantVersionGoPackage)
				}
			}

			for _, file := range tt.wantFiles {
				e.assertFileExists(t, file)
			}
			for _, file := range tt.wantNotFiles {
				e.assertFileNotExist(t, file)
			}
		})
	}
}

// saveResponseImpl is the real implementation of saving a response, captured
// here so it can be restored in test cleanup.
func saveResponseImpl(resp *request.Library, path string) error {
	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid",
			cfg: &Config{
				LibrarianDir: "a",
				InputDir:     "b",
				OutputDir:    "c",
				SourceDir:    "d",
				RepoDir:      "e",
			},
			wantErr: false,
		},
		{
			name: "missing librarian dir",
			cfg: &Config{
				InputDir:  "b",
				OutputDir: "c",
				SourceDir: "d",
				RepoDir:   "e",
			},
			wantErr: true,
		},
		{
			name: "missing input dir",
			cfg: &Config{
				LibrarianDir: "a",
				OutputDir:    "c",
				SourceDir:    "d",
				RepoDir:      "e",
			},
			wantErr: true,
		},
		{
			name: "missing output dir",
			cfg: &Config{
				LibrarianDir: "a",
				InputDir:     "b",
				SourceDir:    "d",
				RepoDir:      "e",
			},
			wantErr: true,
		},
		{
			name: "missing source dir",
			cfg: &Config{
				LibrarianDir: "a",
				InputDir:     "b",
				OutputDir:    "c",
				RepoDir:      "e",
			},
			wantErr: true,
		},
		{
			name: "missing repo dir",
			cfg: &Config{
				LibrarianDir: "a",
				InputDir:     "b",
				OutputDir:    "c",
				SourceDir:    "d",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFindLibraryAndAPIToConfigure(t *testing.T) {
	tests := []struct {
		name     string
		req      *Request
		wantID   string
		wantPath string
		wantErr  bool
	}{
		{
			name: "valid new library",
			req: &Request{
				Libraries: []*request.Library{
					{
						ID: "old1",
						APIs: []request.API{
							{
								Path: "old1",
							},
						},
					},
					{
						ID: "new",
						APIs: []request.API{
							{
								Path:   "a/b/c",
								Status: NewAPIStatus,
							},
						},
					},
					{
						ID: "old2",
						APIs: []request.API{
							{
								Path: "old2",
							},
						},
					},
				},
			},
			wantID:   "new",
			wantPath: "a/b/c",
		},
		{
			name: "valid updated library",
			req: &Request{
				Libraries: []*request.Library{
					{
						ID: "old1",
						APIs: []request.API{
							{
								Path: "old1",
							},
						},
					},
					{
						ID: "updated",
						APIs: []request.API{
							{
								Path: "a/b/c",
							},
							{
								Path:   "e/f/g",
								Status: NewAPIStatus,
							},
							{
								Path: "old",
							},
						},
					},
					{
						ID: "old2",
						APIs: []request.API{
							{
								Path: "old2",
							},
						},
					},
				},
			},
			wantID:   "updated",
			wantPath: "e/f/g",
		},
		{
			name: "invalid no new APIs",
			req: &Request{
				Libraries: []*request.Library{
					{
						ID: "old1",
						APIs: []request.API{
							{
								Path: "old1",
							},
						},
					},
					{
						ID: "old2",
						APIs: []request.API{
							{
								Path: "old2",
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "multiple new libraries",
			req: &Request{
				Libraries: []*request.Library{
					{
						ID: "new1",
						APIs: []request.API{
							{
								Path:   "new1",
								Status: NewAPIStatus,
							},
						},
					},
					{
						ID: "new1",
						APIs: []request.API{
							{
								Path:   "new2",
								Status: NewAPIStatus,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "multiple new APIs in one library",
			req: &Request{
				Libraries: []*request.Library{
					{
						ID: "new1",
						APIs: []request.API{
							{
								Path:   "new1",
								Status: NewAPIStatus,
							},
							{
								Path:   "new2",
								Status: NewAPIStatus,
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lib, api, err := findLibraryAndAPIToConfigure(tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("findLibraryToConfigure error = %v, wantErr %v", err, tt.wantErr)
			}
			// We assume that if the ID is correct, the rest is right too (i.e. we're just
			// picking the right struct).
			if tt.wantID != "" && lib.ID != tt.wantID {
				t.Errorf("mismatched ID, got=%s, want=%s", lib.ID, tt.wantID)
			}
			if tt.wantPath != "" && api.Path != tt.wantPath {
				t.Errorf("mismatched API path, got=%s, want=%s", api.Path, tt.wantPath)
			}
		})
	}
}
