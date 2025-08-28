// Copyright 2022 Google LLC
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

package aliasgen

import (
	"fmt"
	"go/doc"
	"go/types"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"
)

const (
	softLineBreak = 77
)

// Run generators aliases from the srcDir into the destDir and tidies required
// files.
func Run(srcDir, destDir string) error {
	if err := cleanDir(destDir); err != nil {
		return err
	}
	am, err := createMappings(srcDir)
	if err != nil {
		return err
	}
	if err := am.WriteAliases(destDir); err != nil {
		return err
	}
	if err := goImports(destDir); err != nil {
		return err
	}
	if err := goModTidy(destDir); err != nil {
		return err
	}
	return nil
}

func cleanDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}

	return nil
}

// Loads information about a Go package in the specified directory and returns
// the information required to properly create aliases for the public surface.
func createMappings(dir string) (*aliasGenerator, error) {
	log.Printf("creating mappings for: %q", dir)
	conf := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedDeps | packages.NeedSyntax,
		Dir:  dir,
	}

	// Load all package info.
	pkgs, err := packages.Load(conf)
	if err != nil {
		return nil, err
	}
	if len(pkgs) != 1 {
		return nil, fmt.Errorf("found %d packages is %s, expected 1", len(pkgs), dir)
	}
	pkg := pkgs[0]
	am := &aliasGenerator{
		importPath: pkg.PkgPath,
		pkg:        strings.TrimSuffix(pkg.Name, "pb"),
	}

	// Load corresponding documentation.
	docPkg, err := doc.NewFromFiles(pkg.Fset, pkg.Syntax, pkg.PkgPath)
	if err != nil {
		return nil, err
	}
	identToDoc := make(map[string]string, len(docPkg.Types))
	for _, t := range docPkg.Types {
		identToDoc[t.Name] = t.Doc
	}

	// Copy information over for all public members.
	for _, name := range pkg.Types.Scope().Names() {
		obj := pkg.Types.Scope().Lookup(name)
		if !obj.Exported() {
			continue
		}
		switch obj.(type) {
		case *types.Var:
			am.vars = append(am.vars, obj.Name())
		case *types.Const:
			am.consts = append(am.consts, obj.Name())
		case *types.TypeName:
			am.typeNames = append(am.typeNames, &namedType{
				name: obj.Name(),
				doc:  identToDoc[obj.Name()],
			})
		case *types.Func:
			f, err := processFunction(obj.(*types.Func))
			if err != nil {
				return nil, err
			}
			am.funcs = append(am.funcs, f)
		default:
			return nil, fmt.Errorf("unable to associate %q with type %T", obj.Name(), obj)
		}
	}
	return am, nil
}

// processFunction parses types information from a function signature.
func processFunction(f *types.Func) (*function, error) {
	fn := &function{
		name: f.Name(),
	}
	sig, ok := f.Type().(*types.Signature)
	if !ok {
		return nil, fmt.Errorf("unexpected type %+v", f.Type())
	}
	params, err := processTuple(sig.Params())
	if err != nil {
		return nil, err
	}
	fn.params = append(fn.params, params...)
	returns, err := processTuple(sig.Results())
	if err != nil {
		return nil, err
	}
	fn.returns = append(fn.returns, returns...)
	return fn, nil
}

func processTuple(t *types.Tuple) ([]*typeInfo, error) {
	var tis []*typeInfo
	for i := 0; i < t.Len(); i++ {
		ti := &typeInfo{}
		v := t.At(i)
		ti.name = v.Name()
		obj, isPtr, err := getTypeNameForFn(v.Type(), false)
		if err != nil {
			return nil, err
		}
		ti.typeName = obj.Name()
		ti.pkg = obj.Pkg().Name()
		ti.isPtr = isPtr
		tis = append(tis, ti)
	}
	return tis, nil
}

// getTypeNameForFn recursively extracts information for function parameter and
// return values.
func getTypeNameForFn(t types.Type, isPtr bool) (*types.TypeName, bool, error) {
	if n, ok := t.(*types.Named); ok {
		return n.Obj(), isPtr, nil
	} else if p, ok := t.(*types.Pointer); ok {
		return getTypeNameForFn(p.Elem(), true)
	}
	return nil, false, fmt.Errorf("unexpected type %+v", t)
}

// aliasGenerator contains the information about a package required to generate
// aliases for its types.
type aliasGenerator struct {
	importPath string
	pkg        string
	typeNames  []*namedType
	vars       []string
	consts     []string
	funcs      []*function
}

type namedType struct {
	name string
	doc  string
}

// WriteAliases uses the internal state to create a file that contains all the
// alias mappings in the specified directory.
func (am *aliasGenerator) WriteAliases(dir string) error {
	log.Printf("writing aliases to: %q", dir)
	os.MkdirAll(dir, os.ModePerm)
	f, err := os.OpenFile(filepath.Join(dir, "alias.go"), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := am.writeHeader(f); err != nil {
		return err
	}
	if err := am.writeConsts(f); err != nil {
		return err
	}
	if err := am.writeVars(f); err != nil {
		return err
	}
	if err := am.writeTypeNames(f); err != nil {
		return err
	}
	if err := am.writeFuncs(f); err != nil {
		return err
	}
	return nil
}

func (am *aliasGenerator) writeHeader(w io.Writer) error {
	header := fmt.Sprintf(`// Copyright %d Google LLC
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

// Code generated by aliasgen. DO NOT EDIT.

// Package %s aliases all exported identifiers in package
// %q.
//
// Deprecated: Please use types in: %s.
// Please read https://github.com/googleapis/google-cloud-go/blob/main/migration.md
// for more details.
package %s

import (
	src %q
	grpc "google.golang.org/grpc" 
)

`, time.Now().Year(), am.pkg, am.importPath, am.importPath, am.pkg, am.importPath)
	if _, err := io.Copy(w, strings.NewReader(header)); err != nil {
		return err
	}
	return nil
}

func (am *aliasGenerator) writeConsts(w io.Writer) error {
	if len(am.consts) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "// Deprecated: Please use consts in: %s\n", am.importPath); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "const (\n"); err != nil {
		return err
	}
	for _, v := range am.consts {
		if _, err := fmt.Fprintf(w, "\t%s = src.%s\n", v, v); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, ")\n\n"); err != nil {
		return err
	}
	return nil
}

func (am *aliasGenerator) writeVars(w io.Writer) error {
	if len(am.vars) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "// Deprecated: Please use vars in: %s\n", am.importPath); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "var (\n"); err != nil {
		return err
	}
	for _, v := range am.vars {
		if _, err := fmt.Fprintf(w, "\t%s = src.%s\n", v, v); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, ")\n\n"); err != nil {
		return err
	}
	return nil
}

func (am *aliasGenerator) writeTypeNames(w io.Writer) error {
	for _, v := range am.typeNames {
		if v.doc != "" {
			if _, err := fmt.Fprint(w, formatComment(v.doc, am.importPath)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "type %s = src.%s\n", v.name, v.name); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "\n"); err != nil {
		return err
	}
	return nil
}

func (am *aliasGenerator) writeFuncs(w io.Writer) error {
	newpkg := am.importPath[strings.LastIndex(am.importPath, "/")+1:]
	for _, f := range am.funcs {
		if _, err := fmt.Fprintf(w, "// Deprecated: Please use funcs in: %s\n", am.importPath); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "func %s(", f.name); err != nil {
			return err
		}

		// write param info
		for i, p := range f.params {
			if i != 0 {
				if _, err := fmt.Fprintf(w, ", "); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(w, "%s %s", p.name, p.FullType(newpkg)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, ")"); err != nil {
			return err
		}

		// build return info
		if len(f.returns) > 1 {
			return fmt.Errorf("expected max of 1 return value for %q, found: %d", f.name, len(f.returns))
		}
		if len(f.returns) == 1 {
			if _, err := fmt.Fprintf(w, " %s", f.returns[0].FullType(newpkg)); err != nil {
				return err
			}
		}

		// write body
		fmt.Fprintf(w, " { ")
		if len(f.returns) > 0 {
			if _, err := fmt.Fprintf(w, "return "); err != nil {
				return nil
			}
		}
		if _, err := fmt.Fprintf(w, "src.%s(", f.name); err != nil {
			return nil
		}
		for i, p := range f.params {
			if i != 0 {
				if _, err := fmt.Fprintf(w, ", "); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(w, "%s", p.name); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, ") }\n"); err != nil {
			return err
		}
	}
	return nil
}

type function struct {
	name    string
	params  []*typeInfo
	returns []*typeInfo
}

type typeInfo struct {
	pkg      string
	isPtr    bool
	name     string
	typeName string
}

// FullType
func (ti *typeInfo) FullType(pkg string) string {
	var sb strings.Builder
	if ti.isPtr {
		sb.WriteString("*")
	}
	var p string
	if ti.pkg != pkg {
		p = ti.pkg + "."
	}
	sb.WriteString(fmt.Sprintf("%s%s", p, ti.typeName))
	return sb.String()
}

func formatComment(doc, pkg string) string {
	var sb strings.Builder
	ss := strings.Fields(doc)
	var ssi int
	var lineLen int
	for i, str := range ss {
		// Add one to account for spaces between words.
		if (len(str) + lineLen + 1) < softLineBreak {
			lineLen = lineLen + len(str) + 1
		} else if lineLen == 0 {
			sb.WriteString(fmt.Sprintf("// %s\n", str))
			ssi = i + 1
		} else {
			sb.WriteString(fmt.Sprintf("// %s\n", strings.Join(ss[ssi:i], " ")))
			ssi = i
			lineLen = len(str)
		}
	}
	if ssi != len(ss) {
		sb.WriteString(fmt.Sprintf("// %s\n", strings.Join(ss[ssi:], " ")))
	}
	sb.WriteString(fmt.Sprintf("//\n// Deprecated: Please use types in: %s\n", pkg))
	return sb.String()
}
