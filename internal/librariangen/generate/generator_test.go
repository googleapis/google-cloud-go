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

package generate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/config"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
)

// testEnv encapsulates a temporary test environment.
type testEnv struct {
	tmpDir       string
	librarianDir string
	sourceDir    string
	outputDir    string
}

// newTestEnv creates a new test environment.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "generator-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	e := &testEnv{tmpDir: tmpDir}
	e.librarianDir = filepath.Join(tmpDir, "librarian")
	e.sourceDir = filepath.Join(tmpDir, "source")
	e.outputDir = filepath.Join(tmpDir, "output")
	generatorInputDir := filepath.Join(e.librarianDir, config.GeneratorInputDir)
	for _, dir := range []string{e.librarianDir, e.sourceDir, e.outputDir, generatorInputDir} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatalf("failed to create dir %s: %v", dir, err)
		}
	}

	// An empty config file is valid, and any modules that are requested will
	// just get default values.
	configFile := filepath.Join(generatorInputDir, config.RepoConfigFile)
	err = os.WriteFile(configFile, make([]byte, 0), 0644)
	if err != nil {
		t.Fatalf("failed to create file %s: %v", configFile, err)
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

// writeRequestFile writes a generate-request.json file.
func (e *testEnv) writeRequestFile(t *testing.T, content string) {
	t.Helper()
	p := filepath.Join(e.librarianDir, "generate-request.json")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write request file: %v", err)
	}
}

// writeBazelFile writes a BUILD.bazel file.
func (e *testEnv) writeBazelFile(t *testing.T, apiPath, content string) {
	t.Helper()
	apiDir := filepath.Join(e.sourceDir, apiPath)
	if err := os.MkdirAll(apiDir, 0755); err != nil {
		t.Fatalf("failed to create api dir: %v", err)
	}
	// Create a fake .proto file, which is required by the protoc command builder.
	if err := os.WriteFile(filepath.Join(apiDir, "fake.proto"), nil, 0644); err != nil {
		t.Fatalf("failed to write fake proto file: %v", err)
	}
	p := filepath.Join(apiDir, "BUILD.bazel")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write bazel file: %v", err)
	}
}

// writeServiceYAML writes a service.yaml file.
func (e *testEnv) writeServiceYAML(t *testing.T, apiPath, title string) {
	t.Helper()
	apiDir := filepath.Join(e.sourceDir, apiPath)
	content := "title: " + title
	p := filepath.Join(apiDir, "service.yaml")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write service yaml: %v", err)
	}
}

type postProcessRecorder struct {
	called bool
}

func (r *postProcessRecorder) record(ctx context.Context, req *request.Library, outputDir, moduleDir string, moduleConfig *config.ModuleConfig) error {
	r.called = true
	return nil
}

func TestGenerate(t *testing.T) {
	singleAPIRequest := `{"id": "foo", "apis": [{"path": "api/v1"}]}`
	multiAPIRequest := `{"id": "foo", "apis": [{"path": "api/v1"}, {"path": "api/v2"}]}`
	validBazel := `
go_gapic_library(
    name = "v1_gapic",
    importpath = "path/to/v1;v1",
    grpc_service_config = "service_config.json",
    service_yaml = "service.yaml",
    transport = "grpc",
)
`
	invalidBazel := `
go_gapic_library(
    name = "v1_gapic",
    importpath = "path/to/v1;v1",
)
`
	tests := []struct {
		name               string
		setup              func(e *testEnv, t *testing.T)
		protocErr          error
		wantErr            bool
		wantProtocRunCount int
	}{
		{
			name: "happy path",
			setup: func(e *testEnv, t *testing.T) {
				e.writeRequestFile(t, singleAPIRequest)
				e.writeBazelFile(t, "api/v1", validBazel)
				e.writeServiceYAML(t, "api/v1", "My API")
			},
			wantErr:            false,
			wantProtocRunCount: 1,
		},
		{
			name: "multi-api request uses first api config",
			setup: func(e *testEnv, t *testing.T) {
				e.writeRequestFile(t, multiAPIRequest)
				e.writeBazelFile(t, "api/v1", validBazel)
				e.writeServiceYAML(t, "api/v1", "First API")
				e.writeBazelFile(t, "api/v2", validBazel)
				e.writeServiceYAML(t, "api/v2", "Second API")
			},
			wantErr:            false,
			wantProtocRunCount: 2,
		},
		{
			name: "missing request file",
			setup: func(e *testEnv, t *testing.T) {
				e.writeBazelFile(t, "api/v1", validBazel)
			},
			wantErr: true,
		},
		{
			name: "missing bazel file",
			setup: func(e *testEnv, t *testing.T) {
				e.writeRequestFile(t, singleAPIRequest)
			},
			wantErr: true,
		},
		{
			name: "invalid bazel config",
			setup: func(e *testEnv, t *testing.T) {
				e.writeRequestFile(t, singleAPIRequest)
				e.writeBazelFile(t, "api/v1", invalidBazel)
			},
			wantErr: true,
		},
		{
			name: "protoc fails",
			setup: func(e *testEnv, t *testing.T) {
				e.writeRequestFile(t, singleAPIRequest)
				e.writeBazelFile(t, "api/v1", validBazel)
				e.writeServiceYAML(t, "api/v1", "My API")
			},
			protocErr:          errors.New("protoc failed"),
			wantErr:            true,
			wantProtocRunCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEnv(t)
			defer e.cleanup(t)

			tt.setup(e, t)

			var protocRunCount int
			execvRun = func(ctx context.Context, args []string, dir string) error {
				want := "protoc"
				if args[0] != want {
					t.Errorf("protocRun called with %s; want %s", args[0], want)
				}
				if tt.protocErr == nil {
					// Simulate protoc creating the nested directory.
					if err := os.MkdirAll(filepath.Join(e.outputDir, "cloud.google.com", "go"), 0755); err != nil {
						t.Fatalf("failed to create nested dir: %v", err)
					}
				}
				protocRunCount++
				return tt.protocErr
			}
			recorder := &postProcessRecorder{}
			postProcess = recorder.record

			cfg := &Config{
				LibrarianDir:         e.librarianDir,
				InputDir:             "fake-input",
				OutputDir:            e.outputDir,
				SourceDir:            e.sourceDir,
				DisablePostProcessor: tt.name != "happy path" && tt.name != "multi-api request uses first api config",
			}

			if err := Generate(context.Background(), cfg); (err != nil) != tt.wantErr {
				t.Errorf("Generate() error = %v, wantErr %v", err, tt.wantErr)
			}

			if protocRunCount != tt.wantProtocRunCount {
				t.Errorf("protocRun called = %v; want %v", protocRunCount, tt.wantProtocRunCount)
			}
		})
	}
}

func TestFixPermissions(t *testing.T) {
	// Create a temporary directory for the test.
	tmpDir, err := os.MkdirTemp("", "permissions-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a subdirectory to test recursive behavior.
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	// Create test files with incorrect permissions.
	goFile1 := filepath.Join(tmpDir, "file1.go")
	goFile2 := filepath.Join(subDir, "file2.go")
	otherFile := filepath.Join(tmpDir, "other.txt")

	if err := os.WriteFile(goFile1, []byte("package main"), 0777); err != nil {
		t.Fatalf("failed to write goFile1: %v", err)
	}
	if err := os.WriteFile(goFile2, []byte("package main"), 0777); err != nil {
		t.Fatalf("failed to write goFile2: %v", err)
	}
	if err := os.WriteFile(otherFile, []byte("some text"), 0777); err != nil {
		t.Fatalf("failed to write otherFile: %v", err)
	}

	// Run the function to fix permissions.
	if err := fixPermissions(tmpDir); err != nil {
		t.Fatalf("fixPermissions() failed: %v", err)
	}

	// Check permissions of the .go files.
	for _, f := range []string{goFile1, goFile2} {
		info, err := os.Stat(f)
		if err != nil {
			t.Fatalf("failed to stat %s: %v", f, err)
		}
		if info.Mode().Perm() != 0644 {
			t.Errorf("permission of %s is %v, want 0644", f, info.Mode().Perm())
		}
	}

	// Check that the permission of the other file is unchanged.
	info, err := os.Stat(otherFile)
	if err != nil {
		t.Fatalf("failed to stat %s: %v", otherFile, err)
	}
	if info.Mode().Perm() == 0644 {
		t.Errorf("permission of %s was changed, should not have been", otherFile)
	}
}

func TestFlattenOutput(t *testing.T) {
	// Create a temporary directory for the test.
	tmpDir, err := os.MkdirTemp("", "flatten-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create the nested directory structure.
	goDir := filepath.Join(tmpDir, "cloud.google.com", "go")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatalf("failed to create goDir: %v", err)
	}

	// Create a file to be moved.
	filePath := filepath.Join(goDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Run the flatten function.
	if err := flattenOutput(tmpDir); err != nil {
		t.Fatalf("flattenOutput() failed: %v", err)
	}

	// Check that the file was moved to the top level.
	newFilePath := filepath.Join(tmpDir, "file.txt")
	if _, err := os.Stat(newFilePath); os.IsNotExist(err) {
		t.Errorf("file was not moved to the top level")
	}

	// Check that the old directory was removed.
	if _, err := os.Stat(filepath.Join(tmpDir, "cloud.google.com")); !os.IsNotExist(err) {
		t.Errorf("old directory was not removed")
	}
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
			},
			wantErr: false,
		},
		{
			name: "missing librarian dir",
			cfg: &Config{
				InputDir:  "b",
				OutputDir: "c",
				SourceDir: "d",
			},
			wantErr: true,
		},
		{
			name: "missing input dir",
			cfg: &Config{
				LibrarianDir: "a",
				OutputDir:    "c",
				SourceDir:    "d",
			},
			wantErr: true,
		},
		{
			name: "missing output dir",
			cfg: &Config{
				LibrarianDir: "a",
				InputDir:     "b",
				SourceDir:    "d",
			},
			wantErr: true,
		},
		{
			name: "missing source dir",
			cfg: &Config{
				LibrarianDir: "a",
				InputDir:     "b",
				OutputDir:    "c",
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

func TestApplyModuleVersion(t *testing.T) {
	tests := []struct {
		name        string
		libraryID   string
		moduleName  string
		modulePath  string
		setupFiles  []string
		wantErr     bool
		wantPresent []string
		wantAbsent  []string
	}{
		{
			name:        "v1",
			libraryID:   "functions",
			moduleName:  "functions",
			modulePath:  "cloud.google.com/go/functions",
			setupFiles:  []string{"functions/apiv1/client.go", "internal/generated/snippets/functions/apiv1/snippets.go"},
			wantPresent: []string{"functions/apiv1/client.go", "internal/generated/snippets/functions/apiv1/snippets.go"},
		},
		{
			name:        "v2",
			libraryID:   "functions",
			moduleName:  "functions",
			modulePath:  "cloud.google.com/go/functions/v2",
			setupFiles:  []string{"functions/v2/apiv1/client.go", "internal/generated/snippets/functions/v2/apiv1/snippets.go"},
			wantPresent: []string{"functions/apiv1/client.go", "internal/generated/snippets/functions/apiv1/snippets.go"},
			wantAbsent:  []string{"functions/v2/apiv1/client.go", "internal/generated/snippets/functions/v2/apiv1/snippets.go"},
		},
		{
			name:        "v2 in subdirectory",
			libraryID:   "functions/v2",
			moduleName:  "functions",
			modulePath:  "cloud.google.com/go/functions/v2",
			setupFiles:  []string{"functions/v2/apiv1/client.go", "internal/generated/snippets/functions/v2/apiv1/snippets.go"},
			wantPresent: []string{"functions/v2/apiv1/client.go", "internal/generated/snippets/functions/v2/apiv1/snippets.go"},
			wantAbsent:  []string{"functions/apiv1/client.go", "internal/generated/snippets/functions/apiv1/snippets.go"},
		},
		{
			name:       "unexpected module path format",
			libraryID:  "bogus",
			moduleName: "bogus",
			modulePath: "cloud.google.com/go/bogus/v1/v2",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "apply-module-version-test")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			for _, file := range tt.setupFiles {
				fullFile := filepath.Join(tmpDir, file)
				fullDir := filepath.Dir(fullFile)
				if err := os.MkdirAll(fullDir, 0755); err != nil {
					t.Fatalf("failed to create dir %s: %v", fullDir, err)
				}
				if err := os.WriteFile(fullFile, []byte("test"), 0644); err != nil {
					t.Fatalf("failed to write file %s: %v", fullFile, err)
				}
			}

			if err := applyModuleVersion(tmpDir, tt.libraryID, tt.modulePath); (err != nil) != tt.wantErr {
				t.Errorf("applyModuleVersion() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			for _, wantPresent := range tt.wantPresent {
				if _, err := os.Stat(filepath.Join(tmpDir, wantPresent)); err != nil {
					t.Errorf("wanted present, was absent: %s", wantPresent)
				}
			}
			for _, wantAbsent := range tt.wantAbsent {
				if _, err := os.Stat(filepath.Join(tmpDir, wantAbsent)); !os.IsNotExist(err) {
					t.Errorf("wanted absent, was missing: %s", wantAbsent)
				}
			}
		})
	}
}
