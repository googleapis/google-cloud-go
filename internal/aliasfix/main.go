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

package main

import (
	"bytes"
	"flag"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/tools/imports"
)

var (
	fset = token.NewFileSet()
)

func main() {
	flag.Parse()
	path := flag.Arg(0)
	if path == "" {
		log.Fatalf("expected one argument -- path to the directory needing updates")
	}
	if err := processPath(path); err != nil {
		log.Fatal(err)
	}
}

func processPath(path string) error {
	dir, err := os.Stat(path)
	if err != nil {
		return err
	}
	if dir.IsDir() {
		filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".go") {
				err = processFile(path, nil)
			}
			if err != nil {
				return err
			}
			return nil
		})
	} else {
		if err := processFile(path, nil); err != nil {
			return err
		}
	}
	return nil
}

// processFile checks to see if the given file needs any imports rewritten and
// does so if needed. Note an io.Writer is injected here for testability.
func processFile(name string, w io.Writer) error {
	if w == nil {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer file.Close()
		w = file
	}
	f, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	var modified bool
	for _, imp := range f.Imports {
		importPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return err
		}
		if pkg, ok := m[importPath]; ok && pkg.migrated {
			oldNamespace := importPath[strings.LastIndex(importPath, "/")+1:]
			newNamespace := pkg.importPath[strings.LastIndex(pkg.importPath, "/")+1:]
			if imp.Name == nil && oldNamespace != newNamespace {
				// use old namespace for fewer diffs
				imp.Name = ast.NewIdent(oldNamespace)
			} else if imp.Name != nil && imp.Name.Name == newNamespace {
				// no longer need named import if matching named import
				imp.Name = nil
			}
			imp.EndPos = imp.End()
			imp.Path.Value = strconv.Quote(pkg.importPath)
			modified = true
		}
	}
	if modified {
		var buf bytes.Buffer
		if err := format.Node(&buf, fset, f); err != nil {
			return err
		}
		b, err := imports.Process(name, buf.Bytes(), nil)
		if err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
	}
	return nil
}
