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
