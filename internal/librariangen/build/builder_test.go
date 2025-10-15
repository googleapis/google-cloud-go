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

package build

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// testEnv encapsulates a temporary test environment.
type testEnv struct {
	tmpDir       string
	librarianDir string
	repoDir      string
}

func TestBuild(t *testing.T) {
	singleAPIRequest := `{"id": "foo", "apis": [{"path": "api/v1"}]}`
	tests := []struct {
		name           string
		setup          func(e *testEnv, t *testing.T)
		buildErr       error
		testErr        error
		wantErr        bool
		wantExecvCount int
	}{
		{
			name: "happy path",
			setup: func(e *testEnv, t *testing.T) {
				e.writeRequestFile(t, singleAPIRequest)
			},
			wantErr:        false,
			wantExecvCount: 2,
		},
		{
			name:    "missing request file",
			wantErr: true,
		},
		{
			name: "go build fails",
			setup: func(e *testEnv, t *testing.T) {
				e.writeRequestFile(t, singleAPIRequest)
			},
			buildErr:       errors.New("build failed"),
			wantErr:        true,
			wantExecvCount: 1,
		},
		{
			name: "go test fails",
			setup: func(e *testEnv, t *testing.T) {
				e.writeRequestFile(t, singleAPIRequest)
			},
			testErr:        errors.New("test failed"),
			wantErr:        true,
			wantExecvCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEnv(t)
			defer e.cleanup(t)

			if tt.setup != nil {
				tt.setup(e, t)
			}

			var execvCount int
			execvRun = func(ctx context.Context, args []string, dir string) error {
				execvCount++
				want := filepath.Join(e.repoDir, "foo")
				if dir != want {
					t.Errorf("execv called with wrong working directory %s; want %s", dir, want)
				}
				switch {
				case slices.Equal(args, []string{"go", "build", "./..."}):
					return tt.buildErr
				case slices.Equal(args, []string{"go", "test", "./...", "-short"}):
					return tt.testErr
				default:
					t.Errorf("execv called with unexpected args %v", args)
					return nil
				}
			}

			cfg := &Config{
				LibrarianDir: e.librarianDir,
				RepoDir:      e.repoDir,
			}

			if err := Build(context.Background(), cfg); (err != nil) != tt.wantErr {
				t.Errorf("Build() error = %v, wantErr %v", err, tt.wantErr)
			}

			if execvCount != tt.wantExecvCount {
				t.Errorf("execv called = %v; want %v", execvCount, tt.wantExecvCount)
			}
		})
	}
}

// cleanup removes the temporary directory.
func (e *testEnv) cleanup(t *testing.T) {
	t.Helper()
	if err := os.RemoveAll(e.tmpDir); err != nil {
		t.Fatalf("failed to remove temp dir: %v", err)
	}
}

// writeRequestFile writes a builf-request.json file.
func (e *testEnv) writeRequestFile(t *testing.T, content string) {
	t.Helper()
	p := filepath.Join(e.librarianDir, "build-request.json")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write request file: %v", err)
	}
}

// newTestEnv creates a new test environment.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "builder-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	e := &testEnv{tmpDir: tmpDir}
	e.librarianDir = filepath.Join(tmpDir, "librarian")
	e.repoDir = filepath.Join(tmpDir, "repo")
	for _, dir := range []string{e.librarianDir, e.repoDir} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatalf("failed to create dir %s: %v", dir, err)
		}
	}

	return e
}
