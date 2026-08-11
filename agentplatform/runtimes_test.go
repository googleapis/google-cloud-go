// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package agentplatform

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"cloud.google.com/go/agentplatform/types"
)

func TestArchiveSourceDir_Success(t *testing.T) {
	// Temporary source directory creation
	tempDir := t.TempDir()

	mainCode := "package main\nfunc main() {}"
	if err := os.WriteFile(filepath.Join(tempDir, "main.go"), []byte(mainCode), 0644); err != nil {
		t.Fatalf("failed to create main.go: %v", err)
	}

	archiveBytes, err := archiveSourceDir(tempDir)
	if err != nil {
		t.Fatalf("archiveSourceDir failed: %v", err)
	}
	if len(archiveBytes) == 0 {
		t.Fatal("expected non-empty archive bytes")
	}
	gr, err := gzip.NewReader(bytes.NewReader(archiveBytes))

	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	foundMain := false

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("error reading tar: %v", err)
		}

		if header.Name == "main.go" {
			foundMain = true
			var content bytes.Buffer
			if _, err := io.Copy(&content, tr); err != nil {
				t.Fatalf("failed to copy content for main.go: %v", err)
			}
			if content.String() != mainCode {
				t.Errorf("content mismatch: got %q, want %q", content.String(), mainCode)
			}
		}
	}

	if !foundMain {
		t.Errorf("main.go was not found inside the generated archive")
	}
}

func TestArchiveSourceDir_NonExistentDirectory(t *testing.T) {
	// Non-existent directory error validation
	_, err := archiveSourceDir("/non/existent/path/987654321")
	if err == nil {
		t.Fatal("expected error for non-existent directory, got nil")
	}
}

func TestArchiveSourceDir_PathIsFileNotDir(t *testing.T) {
	// Single regular file creation
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "single_file.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0644); err != nil {
		t.Fatalf("failed to create dummy file: %v", err)
	}

	// File path validation error check
	_, err := archiveSourceDir(filePath)
	if err == nil {
		t.Fatal("expected error when passing a file path instead of a directory, got nil")
	}
}

func TestRuntimes_Create_LocalSourcePackages(t *testing.T) {
	if *mode != apiMode && *mode != unitMode {
		t.Skipf("Skipping %s. [%s] mode is not supported for this test.", t.Name(), *mode)
	}

	client, mockServer := createClient(t)

	// Temporary source directory creation
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create dummy source file: %v", err)
	}

	// Mock server response configuration
	mockServer.AddResponses(
		&MockResponse{
			StatusCode: http.StatusOK,
			Body: &types.RuntimeOperation{
				Name: "projects/test-project/locations/global/reasoningEngines/test-runtime/operations/op1",
				Done: true,
			},
		},
	)

	// Runtime creation request with local source packages
	config := &types.CreateRuntimeConfig{
		DisplayName:    "TestRuntimeSourcePackages",
		SourcePackages: []string{tempDir},
	}

	op, err := client.Runtimes.Create(t.Context(), config)
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	if op == nil || op.Name == "" {
		t.Errorf("Create() returned empty operation")
	}

	// Inline source archive payload verification
	if config.Spec == nil || config.Spec.SourceCodeSpec == nil || config.Spec.SourceCodeSpec.InlineSource == nil {
		t.Fatal("expected inline source spec hierarchy initialization")
	}

	if len(config.Spec.SourceCodeSpec.InlineSource.SourceArchive) == 0 {
		t.Errorf("expected non-empty SourceArchive payload")
	}
}

func TestRuntimes_Create_InvalidSourcePackages(t *testing.T) {
	client, _ := createClient(t)

	// Runtime creation request with non-existent source directory
	config := &types.CreateRuntimeConfig{
		DisplayName:    "TestRuntimeInvalidDir",
		SourcePackages: []string{"/non/existent/test/directory/path/12345"},
	}

	_, err := client.Runtimes.Create(t.Context(), config)
	if err == nil {
		t.Fatal("expected error for non-existent source directory, got nil")
	}
}

func TestRuntimes_Create_Declarative(t *testing.T) {
	if *mode != apiMode && *mode != unitMode {
		t.Skipf("Skipping %s. [%s] mode is not supported for this test.", t.Name(), *mode)
	}

	client, mockServer := createClient(t)

	// Mock server response configuration
	mockServer.AddResponses(
		&MockResponse{
			StatusCode: http.StatusOK,
			Body: &types.RuntimeOperation{
				Name: "projects/test-project/locations/global/reasoningEngines/test-runtime/operations/op2",
				Done: true,
			},
		},
	)

	// Declarative runtime creation request without source packages
	config := &types.CreateRuntimeConfig{
		DisplayName: "TestRuntimeDeclarative",
		Description: "Declarative runtime without source packages",
	}

	op, err := client.Runtimes.Create(t.Context(), config)
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	if op == nil || op.Name == "" {
		t.Errorf("Create() returned empty operation")
	}
}

func TestRuntimes_Create_MultipleSourcePackages(t *testing.T) {
	client, _ := createClient(t)

	// Runtime creation request with multiple source packages
	config := &types.CreateRuntimeConfig{
		DisplayName:    "TestRuntimeMultipleSourcePackages",
		SourcePackages: []string{"./dir1", "./dir2"},
	}

	_, err := client.Runtimes.Create(t.Context(), config)
	if err == nil {
		t.Fatal("expected error for multiple source packages, got nil")
	}
}
