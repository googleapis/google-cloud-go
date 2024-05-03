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

//go:build go1.18
// +build go1.18

package aliasfix

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/tools/imports"
)

var (
	fset = token.NewFileSet()
)

// ProcessPath rewrites imports from go-genproto in terms of google-cloud-go
// types.
func ProcessPath(path string) error {
	dir, err := os.Stat(path)
	if err != nil {
		return err
	}
	if dir.IsDir() {
		err := filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if strings.HasSuffix(d.Name(), ".go") {
				return processFile(path, nil)
			}
			return nil
		})
		if err != nil {
			return err
		}
	} else {
		if err := processFile(path, nil); err != nil {
			return err
		}
	}
	return nil
}

// processFile checks to see if the given file needs any imports rewritten and
// does so if needed. Note an io.Writer is injected here for testability.
func processFile(name string, w io.Writer) (err error) {
	var f *ast.File
	f, err = parser.ParseFile(fset, name, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	var modified bool
	for _, imp := range f.Imports {
		var importPath string
		importPath, err = strconv.Unquote(imp.Path.Value)
		if err != nil {
			return err
		}
		if pkg, ok := GenprotoPkgMigration[importPath]; ok && pkg.Status == StatusMigrated {
			oldNamespace := genprotoNamespace(importPath)
			newNamespace := path.Base(pkg.ImportPath)
			if imp.Name == nil && oldNamespace != newNamespace {
				// use old namespace for fewer diffs
				imp.Name = ast.NewIdent(oldNamespace)
			} else if imp.Name != nil && imp.Name.Name == newNamespace {
				// no longer need named import if matching named import
				imp.Name = nil
			}
			imp.EndPos = imp.End()
			imp.Path.Value = strconv.Quote(pkg.ImportPath)
			modified = true
		}
	}
	if !modified {
		return nil
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return err
	}
	b, err := imports.Process(name, buf.Bytes(), nil)
	if err != nil {
		return err
	}

	if w != nil {
		_, err := w.Write(b)
		return err
	}

	backup := name + ".bak"
	if err = os.Rename(name, backup); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.Rename(backup, name)
		} else {
			os.Remove(backup)
		}
	}()

	return os.WriteFile(name, b, 0644)
}

func genprotoNamespace(importPath string) string {
	suffix := path.Base(importPath)
	// if it looks like a version, then use the second from last component.
	if len(suffix) >= 2 && suffix[0] == 'v' && '0' <= suffix[1] && suffix[1] <= '1' {
		return path.Base(path.Dir(importPath))
	}
	return suffix
}
