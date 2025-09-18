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

package module

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/config"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
	"github.com/google/go-cmp/cmp"
)

func TestGenerateInternalVersionFile(t *testing.T) {
	tmpDir := t.TempDir()
	version := "1.2.3"
	if err := GenerateInternalVersionFile(tmpDir, version); err != nil {
		t.Fatalf("GenerateInternalVersionFile() error = %v", err)
	}

	versionFile := filepath.Join(tmpDir, "internal", "version.go")
	if _, err := os.Stat(versionFile); os.IsNotExist(err) {
		t.Errorf("file %s was not created", versionFile)
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, versionFile, nil, parser.ParseComments)
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
				if gotVersion != version {
					t.Errorf("version.go Version = %q, want %q", gotVersion, version)
				}
				return
			}
		}
	}
	t.Errorf("could not find Version constant in version.go")
}

func TestUpdateSnippetsMetadata(t *testing.T) {
	testdata := []struct {
		name         string
		lib          *request.Library
		moduleConfig *config.ModuleConfig
		files        map[string]string
		want         map[string]string
		wantErr      bool
	}{
		{
			name: "version placeholder",
			lib: &request.Library{
				ID:      "secretmanager",
				Version: "2.3.4",
				APIs: []request.API{
					{
						Path: "google/cloud/secretmanager/v1",
					},
				},
			},
			moduleConfig: &config.ModuleConfig{
				Name: "secretmanager",
				APIs: []*config.APIConfig{
					{
						Path:         "google/cloud/secretmanager/v1",
						ProtoPackage: "google.cloud.secretmanager.v1",
					},
				},
			},
			files: map[string]string{
				"internal/generated/snippets/secretmanager/apiv1/snippet_metadata.google.cloud.secretmanager.v1.json": `{"clientLibrary":{"version":"$VERSION"}}`,
			},
			want: map[string]string{
				"internal/generated/snippets/secretmanager/apiv1/snippet_metadata.google.cloud.secretmanager.v1.json": `{"clientLibrary":{"version":"2.3.4"}}`,
			},
		},
		{
			name: "existing version",
			lib: &request.Library{
				ID:      "workflows",
				Version: "5.6.7",
				APIs: []request.API{
					{
						Path: "google/cloud/workflows/v1",
					},
				},
			},
			moduleConfig: &config.ModuleConfig{
				Name: "workflows",
				APIs: []*config.APIConfig{
					{
						Path:         "google/cloud/workflows/v1",
						ProtoPackage: "google.cloud.workflows.v1",
					},
				},
			},
			files: map[string]string{
				"internal/generated/snippets/workflows/apiv1/snippet_metadata.google.cloud.workflows.v1.json": `{"clientLibrary":{"version":"1.2.3"}}`,
			},
			want: map[string]string{
				"internal/generated/snippets/workflows/apiv1/snippet_metadata.google.cloud.workflows.v1.json": `{"clientLibrary":{"version":"5.6.7"}}`,
			},
		},
		{
			name: "file not found",
			lib: &request.Library{
				ID:      "secretmanager",
				Version: "2.3.4",
				APIs: []request.API{
					{
						Path: "google/cloud/secretmanager/v1",
					},
				},
			},
			moduleConfig: &config.ModuleConfig{
				Name: "secretmanager",
				APIs: []*config.APIConfig{
					{
						Path:         "google/cloud/secretmanager/v1",
						ProtoPackage: "google.cloud.secretmanager.v1",
					},
				},
			},
			files: map[string]string{},
			want:  map[string]string{},
		},
		{
			name: "no version string",
			lib: &request.Library{
				ID:      "secretmanager",
				Version: "2.3.4",
				APIs: []request.API{
					{
						Path: "google/cloud/secretmanager/v1",
					},
				},
			},
			moduleConfig: &config.ModuleConfig{
				Name: "secretmanager",
				APIs: []*config.APIConfig{
					{
						Path:         "google/cloud/secretmanager/v1",
						ProtoPackage: "google.cloud.secretmanager.v1",
					},
				},
			},
			files: map[string]string{
				"internal/generated/snippets/secretmanager/apiv1/snippet_metadata.google.cloud.secretmanager.v1.json": `{"clientLibrary":{}}`,
			},
			want:    map[string]string{},
			wantErr: true,
		},
		{
			name: "multiple api versions and a sub-API",
			lib: &request.Library{
				ID:      "secretmanager",
				Version: "1.0.0",
				APIs: []request.API{
					{
						Path: "google/cloud/secretmanager/v1",
					},
					{
						Path: "google/cloud/secretmanager/v2",
					},
					{
						Path: "google/cloud/secretmanager/subapi/v1",
					},
				},
			},
			moduleConfig: &config.ModuleConfig{
				Name: "secretmanager",
				APIs: []*config.APIConfig{
					{
						Path:         "google/cloud/secretmanager/v1",
						ProtoPackage: "google.cloud.secretmanager.v1",
					},
					{
						Path:         "google/cloud/secretmanager/v2",
						ProtoPackage: "google.cloud.secretmanager.v2",
					},
					{
						Path:         "google/cloud/secretmanager/subapi/v1",
						ProtoPackage: "google.cloud.secretmanager.subapi.v1",
					},
				},
			},
			files: map[string]string{
				"internal/generated/snippets/secretmanager/apiv1/snippet_metadata.google.cloud.secretmanager.v1.json":               `{"clientLibrary":{"version":"$VERSION"}}`,
				"internal/generated/snippets/secretmanager/apiv2/snippet_metadata.google.cloud.secretmanager.v2.json":               `{"clientLibrary":{"version":"0.1.0"}}`,
				"internal/generated/snippets/secretmanager/subapi/apiv1/snippet_metadata.google.cloud.secretmanager.subapi.v1.json": `{"clientLibrary":{"version":"0.1.0"}}`,
			},
			want: map[string]string{
				"internal/generated/snippets/secretmanager/apiv1/snippet_metadata.google.cloud.secretmanager.v1.json":               `{"clientLibrary":{"version":"1.0.0"}}`,
				"internal/generated/snippets/secretmanager/apiv2/snippet_metadata.google.cloud.secretmanager.v2.json":               `{"clientLibrary":{"version":"1.0.0"}}`,
				"internal/generated/snippets/secretmanager/subapi/apiv1/snippet_metadata.google.cloud.secretmanager.subapi.v1.json": `{"clientLibrary":{"version":"1.0.0"}}`,
			},
		},
	}

	for _, tc := range testdata {
		t.Run(tc.name, func(t *testing.T) {
			sourceDir := t.TempDir()
			destDir := t.TempDir()

			for path, content := range tc.files {
				fullPath := filepath.Join(sourceDir, path)
				if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
					t.Fatalf("failed to create directory: %v", err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					t.Fatalf("failed to write file: %v", err)
				}
			}

			if err := UpdateSnippetsMetadata(tc.lib, sourceDir, destDir, tc.moduleConfig); (err != nil) != tc.wantErr {
				t.Errorf("UpdateSnippetsMetadata() error = %v, wantErr %v", err, tc.wantErr)
			}

			got := make(map[string]string)
			for path := range tc.want {
				content, err := os.ReadFile(filepath.Join(destDir, path))
				if err != nil {
					// If the file is not found, it's a valid case for some tests.
					if os.IsNotExist(err) {
						continue
					}
					t.Fatalf("failed to read file: %v", err)
				}
				got[path] = string(content)
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("UpdateSnippetsMetadata() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
