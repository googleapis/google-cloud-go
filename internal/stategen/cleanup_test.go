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
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCleanupLegacyConfigs(t *testing.T) {
	t.Parallel()
	testModuleNames := []string{
		// Test using `ai` because it is a substring of `aiplatform`.
		"ai",
		// Test an individually-released module in release-please-config-individual.json, etc.
		"auth",
		"cloudtasks",
		// Test datastore which has google/datastore/v1 and google/datastore/admin/v1
		"datastore",
	}
	for _, testModuleName := range testModuleNames {
		// Create a temporary directory for the test repo.
		repoRoot := t.TempDir()

		// Set up the initial directory structure and copy testdata files.
		files := []string{
			".github/.OwlBot.yaml",
			"internal/postprocessor/config.yaml",
			"release-please-config-individual.json",
			"release-please-config-yoshi-submodules.json",
			".release-please-manifest-individual.json",
			".release-please-manifest-submodules.json",
		}

		for _, f := range files {
			content, err := os.ReadFile("testdata/source/" + f)
			if err != nil {
				t.Fatalf("Failed to read testdata file %s: %v", f, err)
			}
			destPath := filepath.Join(repoRoot, f)
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				t.Fatalf("Failed to create directory for %s: %v", f, err)
			}
			if err := os.WriteFile(destPath, content, 0644); err != nil {
				t.Fatalf("Failed to write initial file %s: %v", f, err)
			}
		}

		if err := cleanupLegacyConfigs(repoRoot, testModuleName); err != nil {
			t.Fatalf("cleanupLegacyConfigs failed: %v", err)
		}

		for _, f := range files {
			got, err := os.ReadFile(filepath.Join(repoRoot, f))
			if err != nil {
				t.Fatalf("Failed to read modified file %s: %v", f, err)
			}
			gf := fmt.Sprintf("testdata/golden/%s/%s", testModuleName, f)
			want, err := os.ReadFile(gf)
			if err != nil {
				t.Fatalf("Failed to read golden file %s: %v", gf, err)
			}
			if diff := cmp.Diff(string(want), string(got)); diff != "" {
				t.Errorf("File %s mismatch for %q (-want +got):\n%s", f, testModuleName, diff)
			}
		}
	}
}
