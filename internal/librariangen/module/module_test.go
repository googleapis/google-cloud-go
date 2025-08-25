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
	tests := []struct {
		name           string
		initialContent string
		version        string
		wantContent    string
	}{
		{
			name:           "replace $VERSION",
			initialContent: `{"clientLibrary": {"version": "$VERSION"}}`,
			version:        "2.0.0",
			wantContent:    `{"clientLibrary": {"version": "2.0.0"}}`,
		},
		{
			name:           "replace semver",
			initialContent: `{"clientLibrary": {"version": "1.15.0"}}`,
			version:        "1.16.0",
			wantContent:    `{"clientLibrary": {"version": "1.16.0"}}`,
		},
		{
			name:           "no replacement",
			initialContent: `{"clientLibrary": {"version": "NA"}}`,
			version:        "1.0.0",
			wantContent:    `{"clientLibrary": {"version": "NA"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputDir := t.TempDir()
			snippetsDir := filepath.Join(outputDir, "internal", "generated", "snippets", "mymodule")
			if err := os.MkdirAll(snippetsDir, 0755); err != nil {
				t.Fatalf("failed to create snippetsDir: %v", err)
			}
			metadataFile := filepath.Join(snippetsDir, "snippet_metadata.google.cloud.mymodule.v1.json")
			if err := os.WriteFile(metadataFile, []byte(tt.initialContent), 0644); err != nil {
				t.Fatalf("failed to write initial metadata file: %v", err)
			}

			if err := UpdateSnippetsMetadata(outputDir, "mymodule", tt.version); err != nil {
				t.Fatalf("UpdateSnippetsMetadata() error = %v", err)
			}

			content, err := os.ReadFile(metadataFile)
			if err != nil {
				t.Fatalf("failed to read metadata file: %v", err)
			}
			if string(content) != tt.wantContent {
				t.Errorf("file content = %q, want %q", string(content), tt.wantContent)
			}
		})
	}
}
