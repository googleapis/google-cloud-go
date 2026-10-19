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

package spanner

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// goList runs go list with the build tags and returns the fields of its
// output.
func goList(t *testing.T, tags string, args ...string) []string {
	t.Helper()
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("go tool not found: %v", err)
	}
	args = append([]string{"list", "-tags=" + tags}, args...)
	cmd := exec.CommandContext(t.Context(), goTool, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.Fields(string(out))
}

// TestVTProtoDependencies verifies that vtprotobuf code is only linked into
// the spanner package when the spanner_vtproto build tag is set.
func TestVTProtoDependencies(t *testing.T) {
	for _, tc := range []struct {
		tags string
		want bool
	}{
		{tags: "", want: false},
		{tags: "spanner_vtproto", want: true},
	} {
		t.Run("tags="+tc.tags, func(t *testing.T) {
			deps := goList(t, tc.tags, "-deps", ".")
			for _, prefix := range []string{"github.com/planetscale/vtprotobuf/", "cloud.google.com/go/spanner/internal/vtpb"} {
				if got := slices.ContainsFunc(deps, func(dep string) bool { return strings.HasPrefix(dep, prefix) }); got != tc.want {
					t.Errorf("dependency on %q = %v, want %v", prefix, got, tc.want)
				}
			}
		})
	}
}

// TestVTProtoPublicAPI verifies that the files that only one of the builds
// with and without the spanner_vtproto build tag selects export nothing.
func TestVTProtoPublicAPI(t *testing.T) {
	const files = "{{range .GoFiles}}{{.}} {{end}}"
	untagged := goList(t, "", "-f", files, ".")
	tagged := goList(t, "spanner_vtproto", "-f", files, ".")
	for _, onlyIn := range [][]string{without(tagged, untagged), without(untagged, tagged)} {
		if len(onlyIn) == 0 {
			t.Fatal("the build tag selects no files")
		}
		for _, name := range onlyIn {
			if exported := exportedNames(t, name); len(exported) > 0 {
				t.Errorf("%s exports %v, want nothing", name, exported)
			}
		}
	}
}

// exportedNames returns the exported package level identifiers of a file,
// and the exported methods of its exported types.
func exportedNames(t *testing.T, filename string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for name := range f.Scope.Objects {
		if ast.IsExported(name) {
			names = append(names, name)
		}
	}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || !fn.Name.IsExported() {
			continue
		}
		recv := fn.Recv.List[0].Type
		if star, ok := recv.(*ast.StarExpr); ok {
			recv = star.X
		}
		if id, ok := recv.(*ast.Ident); !ok || id.IsExported() {
			names = append(names, fn.Name.Name)
		}
	}
	slices.Sort(names)
	return names
}

// without returns the elements of a that are not in b.
func without(a, b []string) []string {
	var d []string
	for _, s := range a {
		if !slices.Contains(b, s) {
			d = append(d, s)
		}
	}
	return d
}
