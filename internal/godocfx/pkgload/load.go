// Copyright 2021 Google LLC
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

package pkgload

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Info holds info about a package.
type Info struct {
	Pkg  *packages.Package
	Doc  *doc.Package
	Fset *token.FileSet
	// ImportRenames is a map from package path to local name or "".
	ImportRenames map[string]string
	Status        string
}

// Load parses the given glob and returns info for the matching packages.
// The workingDir is used for module detection.
// Packages that match the filter are ignored.
func Load(glob, workingDir string, filter []string) ([]Info, error) {
	config := &packages.Config{
		Mode:  packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedModule | packages.NeedImports | packages.NeedDeps,
		Tests: true,
		Dir:   workingDir,
	}

	allPkgs, err := packages.Load(config, glob)
	if err != nil {
		return nil, fmt.Errorf("packages.Load: %v", err)
	}
	packages.PrintErrors(allPkgs) // Don't fail everything because of one package.

	if len(allPkgs) == 0 {
		return nil, fmt.Errorf("pattern %q matched 0 packages", glob)
	}

	module := allPkgs[0].Module
	skippedModules := map[string]struct{}{}

	// First, collect all of the files grouped by package, including test
	// packages.
	pkgFiles := map[string][]string{}

	idToPkg := map[string]*packages.Package{}
	pkgNames := []string{}
	for _, pkg := range allPkgs {
		// Ignore filtered packages.
		if hasPrefix(pkg.PkgPath, filter) {
			continue
		}

		id := pkg.ID
		// See https://pkg.go.dev/golang.org/x/tools/go/packages#Config.
		// The uncompiled test package shows up as "foo_test [foo.test]".
		if strings.HasSuffix(id, ".test") ||
			strings.Contains(id, "internal") ||
			strings.Contains(id, "third_party") ||
			(strings.Contains(id, " [") && !strings.Contains(id, "_test [")) {
			continue
		}
		if strings.Contains(id, "_test") {
			id = id[0:strings.Index(id, "_test [")]
		} else if pkg.Module != nil {
			idToPkg[pkg.PkgPath] = pkg
			pkgNames = append(pkgNames, pkg.PkgPath)
			// The test package doesn't have Module set.
			if pkg.Module.Path != module.Path {
				skippedModules[pkg.Module.Path] = struct{}{}
				continue
			}
		}
		for _, f := range pkg.Syntax {
			name := pkg.Fset.File(f.Pos()).Name()
			if strings.HasSuffix(name, ".go") {
				pkgFiles[id] = append(pkgFiles[id], name)
			}
		}
	}

	sort.Strings(pkgNames)

	result := []Info{}

	for _, pkgPath := range pkgNames {
		// Check if pkgPath has prefix of skipped module.
		skip := false
		for skipModule := range skippedModules {
			if strings.HasPrefix(pkgPath, skipModule) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		parsedFiles := []*ast.File{}
		fset := token.NewFileSet()
		for _, f := range pkgFiles[pkgPath] {
			pf, err := parser.ParseFile(fset, f, nil, parser.ParseComments)
			if err != nil {
				return nil, fmt.Errorf("ParseFile: %v", err)
			}
			parsedFiles = append(parsedFiles, pf)
		}

		// Parse out GoDoc.
		docPkg, err := doc.NewFromFiles(fset, parsedFiles, pkgPath)
		if err != nil {
			return nil, fmt.Errorf("doc.NewFromFiles: %v", err)
		}

		// Extra filter in case the file filtering didn't catch everything.
		if !strings.HasPrefix(docPkg.ImportPath, module.Path) {
			continue
		}

		imports := map[string]string{}
		for _, f := range parsedFiles {
			for _, i := range f.Imports {
				name := ""
				// i.Name is nil for imports that aren't renamed.
				if i.Name != nil {
					name = i.Name.Name
				}
				iPath, err := strconv.Unquote(i.Path.Value)
				if err != nil {
					return nil, fmt.Errorf("strconv.Unquote: %v", err)
				}
				imports[iPath] = name
			}
		}

		result = append(result, Info{
			Pkg:           idToPkg[pkgPath],
			Doc:           docPkg,
			Fset:          fset,
			ImportRenames: imports,
			Status:        pkgStatus(pkgPath, docPkg.Doc, idToPkg[pkgPath].Module.Version),
		})
	}

	return result, nil
}

// pkgStatus returns the status of the given package with the
// given GoDoc.
//
// pkgStatus does not use repo-metadata-full.json because it's
// not available for all modules nor all versions.
func pkgStatus(importPath, doc, version string) string {
	switch {
	case strings.Contains(version, "-"):
		return "preview"

	case strings.Contains(doc, "\nDeprecated:"):
		return "deprecated"
	case strings.Contains(doc, "This package is in alpha"):
		return "alpha"
	case strings.Contains(doc, "This package is in beta"):
		return "beta"

	case strings.Contains(importPath, "alpha"):
		return "alpha"
	case strings.Contains(importPath, "beta"):
		return "beta"
	}

	return ""
}

func hasPrefix(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}
