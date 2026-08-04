/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package database

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestOmniClientAPIIsNotDefinedInGeneratedClient(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "database_admin_client.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parser.ParseFile(database_admin_client.go): %v", err)
	}
	for _, imp := range file.Imports {
		if imp.Path.Value == `"cloud.google.com/go/spanner/omni"` {
			t.Fatal("database_admin_client.go is generated and must not import handwritten Spanner Omni helpers")
		}
	}

	file, err = parser.ParseFile(fset, "database_admin_client.go", nil, 0)
	if err != nil {
		t.Fatalf("parser.ParseFile(database_admin_client.go): %v", err)
	}
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			if decl.Name.Name == "NewDatabaseAdminClientWithConfig" {
				t.Fatal("NewDatabaseAdminClientWithConfig must live in a handwritten file, not generated database_admin_client.go")
			}
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if ok && (typeSpec.Name.Name == "ClientConfig" || typeSpec.Name.Name == "OmniClientConfig") {
					t.Fatal("OmniClientConfig/ClientConfig must live in a handwritten file, not generated database_admin_client.go")
				}
			}
		}
	}
}

func TestNewDatabaseAdminClientWithConfigRejectsNilConfig(t *testing.T) {
	_, err := NewDatabaseAdminClientWithConfig(context.Background(), nil)
	if err == nil {
		t.Fatal("NewDatabaseAdminClientWithConfig() err = nil, want error")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("status.Code(err) = %v, want %v", status.Code(err), codes.InvalidArgument)
	}
}
