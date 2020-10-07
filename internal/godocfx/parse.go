// Copyright 2020 Google LLC
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

// +build go1.15

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/doc"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"path/filepath"
	"sort"
	"strings"

	"cloud.google.com/go/third_party/pkgsite"
	"golang.org/x/tools/go/packages"
)

// tableOfContents represents a TOC.
type tableOfContents []*tocItem

// tocItem is an item in a TOC.
type tocItem struct {
	UID   string     `yaml:"uid,omitempty"`
	Name  string     `yaml:"name,omitempty"`
	Items []*tocItem `yaml:"items,omitempty"`
	Href  string     `yaml:"href,omitempty"`
}

func (t *tocItem) addItem(i *tocItem) {
	t.Items = append(t.Items, i)
}

// page represents a single DocFX page.
//
// There is one page per package.
type page struct {
	Items      []*item `yaml:"items"`
	References []*item `yaml:"references,omitempty"`
}

// child represents an item child.
type child string

// syntax represents syntax.
type syntax struct {
	Content string `yaml:"content,omitempty"`
}

type example struct {
	Content string `yaml:"content,omitempty"`
	Name    string `yaml:"name,omitempty"`
}

// item represents a DocFX item.
type item struct {
	UID      string    `yaml:"uid"`
	Name     string    `yaml:"name,omitempty"`
	ID       string    `yaml:"id,omitempty"`
	Summary  string    `yaml:"summary,omitempty"`
	Parent   string    `yaml:"parent,omitempty"`
	Type     string    `yaml:"type,omitempty"`
	Langs    []string  `yaml:"langs,omitempty"`
	Syntax   syntax    `yaml:"syntax,omitempty"`
	Examples []example `yaml:"codeexamples,omitempty"`
	Children []child   `yaml:"children,omitempty"`
}

func (p *page) addItem(i *item) {
	p.Items = append(p.Items, i)
}

func (i *item) addChild(c child) {
	i.Children = append(i.Children, c)
}

var onlyGo = []string{"go"}

type result struct {
	pages  map[string]*page
	toc    tableOfContents
	module *packages.Module
}

// parse parses the directory into a map of import path -> page and a TOC.
//
// glob is the path to parse, usually ending in `...`. glob is passed directly
// to packages.Load as-is.
//
// extraFiles is a list of paths relative to the module root to include.
func parse(glob string, extraFiles []string) (*result, error) {
	config := &packages.Config{
		Mode:  packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedModule,
		Tests: true,
	}

	pkgs, err := packages.Load(config, glob)
	if err != nil {
		return nil, fmt.Errorf("packages.Load: %v", err)
	}
	packages.PrintErrors(pkgs) // Don't fail everything because of one package.

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("pattern %q matched 0 packages", glob)
	}

	pages := map[string]*page{}

	module := pkgs[0].Module
	skippedModules := map[string]struct{}{}

	log.Printf("Processing %s@%s", module.Path, module.Version)

	// First, collect all of the files grouped by package, including test
	// packages.
	goPkgFiles := map[string][]string{}
	for _, pkg := range pkgs {
		id := pkg.ID
		// See https://pkg.go.dev/golang.org/x/tools/go/packages#Config.
		// The uncompiled test package shows up as "foo_test [foo.test]".
		if strings.HasSuffix(id, ".test") ||
			strings.Contains(id, "internal") ||
			(strings.Contains(id, " [") && !strings.Contains(id, "_test [")) {
			continue
		}
		if strings.Contains(id, "_test") {
			id = id[0:strings.Index(id, "_test [")]
		} else {
			// The test package doesn't have Module set.
			if pkg.Module.Path != module.Path {
				skippedModules[pkg.Module.Path] = struct{}{}
				continue
			}
		}
		for _, f := range pkg.Syntax {
			name := pkg.Fset.File(f.Pos()).Name()
			if strings.HasSuffix(name, ".go") {
				goPkgFiles[id] = append(goPkgFiles[id], name)
			}
		}
	}

	// Test files don't have Module set. Filter out packages in skipped modules.
	pkgFiles := map[string][]string{}
	pkgNames := []string{}
	for pkgPath, files := range goPkgFiles {
		skip := false
		for skipped := range skippedModules {
			if strings.HasPrefix(pkgPath, skipped) {
				skip = true
				break
			}
		}
		if !skip {
			pkgFiles[pkgPath] = files
			pkgNames = append(pkgNames, pkgPath)
		}
	}
	sort.Strings(pkgNames)

	toc := buildTOC(module.Path, pkgNames, extraFiles)

	// Once the files are grouped by package, process each package
	// independently.
	for _, pkgPath := range pkgNames {
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

		pkgItem := &item{
			UID:      docPkg.ImportPath,
			Name:     docPkg.ImportPath,
			ID:       docPkg.Name,
			Summary:  docPkg.Doc,
			Langs:    onlyGo,
			Type:     "package",
			Examples: processExamples(docPkg.Examples, fset),
		}
		pkgPage := &page{Items: []*item{pkgItem}}
		pages[pkgPath] = pkgPage

		for _, c := range docPkg.Consts {
			name := strings.Join(c.Names, ", ")
			id := strings.Join(c.Names, ",")
			uid := docPkg.ImportPath + "." + id
			pkgItem.addChild(child(uid))
			pkgPage.addItem(&item{
				UID:     uid,
				Name:    name,
				ID:      id,
				Parent:  docPkg.ImportPath,
				Type:    "const",
				Summary: c.Doc,
				Langs:   onlyGo,
				Syntax:  syntax{Content: pkgsite.PrintType(fset, c.Decl)},
			})
		}
		for _, v := range docPkg.Vars {
			name := strings.Join(v.Names, ", ")
			id := strings.Join(v.Names, ",")
			uid := docPkg.ImportPath + "." + id
			pkgItem.addChild(child(uid))
			pkgPage.addItem(&item{
				UID:     uid,
				Name:    name,
				ID:      id,
				Parent:  docPkg.ImportPath,
				Type:    "variable",
				Summary: v.Doc,
				Langs:   onlyGo,
				Syntax:  syntax{Content: pkgsite.PrintType(fset, v.Decl)},
			})
		}
		for _, t := range docPkg.Types {
			uid := docPkg.ImportPath + "." + t.Name
			pkgItem.addChild(child(uid))
			typeItem := &item{
				UID:      uid,
				Name:     t.Name,
				ID:       t.Name,
				Parent:   docPkg.ImportPath,
				Type:     "type",
				Summary:  t.Doc,
				Langs:    onlyGo,
				Syntax:   syntax{Content: pkgsite.PrintType(fset, t.Decl)},
				Examples: processExamples(t.Examples, fset),
			}
			// Note: items are added as page.Children, rather than
			// typeItem.Children, as a workaround for the DocFX template.
			pkgPage.addItem(typeItem)
			for _, c := range t.Consts {
				name := strings.Join(c.Names, ", ")
				id := strings.Join(c.Names, ",")
				cUID := docPkg.ImportPath + "." + id
				pkgItem.addChild(child(cUID))
				pkgPage.addItem(&item{
					UID:     cUID,
					Name:    name,
					ID:      id,
					Parent:  uid,
					Type:    "const",
					Summary: c.Doc,
					Langs:   onlyGo,
					Syntax:  syntax{Content: pkgsite.PrintType(fset, c.Decl)},
				})
			}
			for _, v := range t.Vars {
				name := strings.Join(v.Names, ", ")
				id := strings.Join(v.Names, ",")
				cUID := docPkg.ImportPath + "." + id
				pkgItem.addChild(child(cUID))
				pkgPage.addItem(&item{
					UID:     cUID,
					Name:    name,
					ID:      id,
					Parent:  uid,
					Type:    "variable",
					Summary: v.Doc,
					Langs:   onlyGo,
					Syntax:  syntax{Content: pkgsite.PrintType(fset, v.Decl)},
				})
			}

			for _, fn := range t.Funcs {
				fnUID := uid + "." + fn.Name
				pkgItem.addChild(child(fnUID))
				pkgPage.addItem(&item{
					UID:      fnUID,
					Name:     fmt.Sprintf("func %s\n", fn.Name),
					ID:       fn.Name,
					Parent:   uid,
					Type:     "function",
					Summary:  fn.Doc,
					Langs:    onlyGo,
					Syntax:   syntax{Content: pkgsite.Synopsis(fset, fn.Decl)},
					Examples: processExamples(fn.Examples, fset),
				})
			}
			for _, fn := range t.Methods {
				fnUID := uid + "." + fn.Name
				pkgItem.addChild(child(fnUID))
				pkgPage.addItem(&item{
					UID:      fnUID,
					Name:     fmt.Sprintf("func (%s) %s\n", fn.Recv, fn.Name),
					ID:       fn.Name,
					Parent:   uid,
					Type:     "method",
					Summary:  fn.Doc,
					Langs:    onlyGo,
					Syntax:   syntax{Content: pkgsite.Synopsis(fset, fn.Decl)},
					Examples: processExamples(fn.Examples, fset),
				})
			}
		}
		for _, fn := range docPkg.Funcs {
			uid := docPkg.ImportPath + "." + fn.Name
			pkgItem.addChild(child(uid))
			pkgPage.addItem(&item{
				UID:      uid,
				Name:     fmt.Sprintf("func %s\n", fn.Name),
				ID:       fn.Name,
				Parent:   docPkg.ImportPath,
				Type:     "function",
				Summary:  fn.Doc,
				Langs:    onlyGo,
				Syntax:   syntax{Content: pkgsite.Synopsis(fset, fn.Decl)},
				Examples: processExamples(fn.Examples, fset),
			})
		}
	}
	if len(skippedModules) > 0 {
		skipped := []string{}
		for s := range skippedModules {
			skipped = append(skipped, "* "+s)
		}
		sort.Strings(skipped)
		log.Printf("Warning: Only processed %s@%s, skipped:\n%s\n", module.Path, module.Version, strings.Join(skipped, "\n"))
	}

	return &result{
		pages:  pages,
		toc:    toc,
		module: module,
	}, nil
}

// processExamples converts the examples to []example.
//
// Surrounding braces and indentation is removed.
func processExamples(exs []*doc.Example, fset *token.FileSet) []example {
	result := []example{}
	for _, ex := range exs {
		buf := &bytes.Buffer{}
		var node interface{} = &printer.CommentedNode{
			Node:     ex.Code,
			Comments: ex.Comments,
		}
		if ex.Play != nil {
			node = ex.Play
		}
		if err := format.Node(buf, fset, node); err != nil {
			log.Fatal(err)
		}
		s := buf.String()
		if strings.HasPrefix(s, "{\n") && strings.HasSuffix(s, "\n}") {
			lines := strings.Split(s, "\n")
			builder := strings.Builder{}
			for _, line := range lines[1 : len(lines)-1] {
				builder.WriteString(strings.TrimPrefix(line, "\t"))
				builder.WriteString("\n")
			}
			s = builder.String()
		}
		result = append(result, example{
			Content: s,
			Name:    ex.Suffix,
		})
	}
	return result
}

func buildTOC(mod string, pkgs []string, extraFiles []string) tableOfContents {
	toc := tableOfContents{}

	modTOC := &tocItem{
		UID:  mod, // Assume the module root has a package.
		Name: mod,
	}
	for _, path := range extraFiles {
		base := filepath.Base(path)
		name := strings.TrimSuffix(base, filepath.Ext(base))
		name = strings.Title(name)
		modTOC.addItem(&tocItem{
			Href: path,
			Name: name,
		})
	}

	toc = append(toc, modTOC)

	if len(pkgs) == 1 {
		// The module only has one package.
		return toc
	}

	trimmedPkgs := []string{}
	for _, pkg := range pkgs {
		if pkg == mod {
			continue
		}
		if !strings.HasPrefix(pkg, mod) {
			panic(fmt.Sprintf("Package %q does not start with %q, should never happen", pkg, mod))
		}
		trimmed := strings.TrimPrefix(pkg, mod+"/")
		trimmedPkgs = append(trimmedPkgs, trimmed)
	}

	sort.Strings(trimmedPkgs)

	for _, trimmed := range trimmedPkgs {
		uid := mod + "/" + trimmed
		pkgTOCItem := &tocItem{
			UID:  uid,
			Name: trimmed,
		}
		modTOC.addItem(pkgTOCItem)
	}

	return toc
}
