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

package main

import (
	"fmt"
	"go/ast"
	"go/doc"
	"log"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// tableOfContents represents a TOC.
type tableOfContents []*tocItem

// tocItem is an item in a TOC.
type tocItem struct {
	UID   string     `yaml:"uid,omitempty"`
	Name  string     `yaml:"name,omitempty"`
	Items []*tocItem `yaml:"items,omitempty"`
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
	Content string
}

// item represents a DocFX item.
type item struct {
	UID      string   `yaml:"uid"`
	Name     string   `yaml:"name,omitempty"`
	ID       string   `yaml:"id,omitempty"`
	Summary  string   `yaml:"summary,omitempty"`
	Parent   string   `yaml:"parent,omitempty"`
	Type     string   `yaml:"type,omitempty"`
	Langs    []string `yaml:"langs,omitempty"`
	Syntax   syntax   `yaml:"syntax,omitempty"`
	Children []child  `yaml:"children,omitempty"`
}

func (p *page) addItem(i *item) {
	p.Items = append(p.Items, i)
}

func (i *item) addChild(c child) {
	i.Children = append(i.Children, c)
}

var onlyGo = []string{"go"}

// parse parses the directory into a map of import path -> page and a TOC.
//
// glob is the path to parse, usually ending in `...`. glob is passed directly
// to packages.Load as-is.
func parse(glob string) (map[string]*page, tableOfContents, *packages.Module, error) {
	config := &packages.Config{
		Mode:  packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedModule,
		Tests: true,
	}

	pkgs, err := packages.Load(config, glob)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("packages.Load: %v", err)
	}
	packages.PrintErrors(pkgs) // Don't fail everything because of one package.

	if len(pkgs) == 0 {
		return nil, nil, nil, fmt.Errorf("pattern %q matched 0 packages", glob)
	}

	pages := map[string]*page{}
	toc := tableOfContents{}

	module := pkgs[0].Module
	skippedModules := map[string]struct{}{}

	log.Printf("Processing %s@%s", module.Path, module.Version)

	for _, pkg := range pkgs {
		if pkg == nil || pkg.Module == nil {
			continue
		}
		if pkg.Module.Path != module.Path {
			skippedModules[pkg.Module.Path] = struct{}{}
			continue
		}
		// Don't generate docs for tests or internal.
		switch {
		case strings.HasSuffix(pkg.ID, ".test"),
			strings.HasSuffix(pkg.ID, ".test]"),
			strings.Contains(pkg.ID, "internal"):
			continue
		}

		// Collect all .go files.
		files := []*ast.File{}
		for _, f := range pkg.Syntax {
			tf := pkg.Fset.File(f.Pos())
			if strings.HasSuffix(tf.Name(), ".go") {
				files = append(files, f)
			}
		}

		// Parse out GoDoc.
		docPkg, err := doc.NewFromFiles(pkg.Fset, files, pkg.PkgPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("doc.NewFromFiles: %v", err)
		}

		toc = append(toc, &tocItem{
			UID:  pkg.ID,
			Name: pkg.PkgPath,
		})

		pkgItem := &item{
			UID:     pkg.ID,
			Name:    pkg.PkgPath,
			ID:      pkg.Name,
			Summary: docPkg.Doc,
			Langs:   onlyGo,
			Type:    "package",
		}
		pkgPage := &page{Items: []*item{pkgItem}}
		pages[pkg.PkgPath] = pkgPage

		for _, c := range docPkg.Consts {
			name := strings.Join(c.Names, ", ")
			id := strings.Join(c.Names, ",")
			uid := pkg.PkgPath + "." + id
			pkgItem.addChild(child(uid))
			pkgPage.addItem(&item{
				UID:     uid,
				Name:    name,
				ID:      id,
				Parent:  pkg.PkgPath,
				Type:    "const",
				Summary: c.Doc,
				Langs:   onlyGo,
				Syntax:  syntax{Content: printType(pkg.Fset, c.Decl)},
			})
		}
		for _, v := range docPkg.Vars {
			name := strings.Join(v.Names, ", ")
			id := strings.Join(v.Names, ",")
			uid := pkg.PkgPath + "." + id
			pkgItem.addChild(child(uid))
			pkgPage.addItem(&item{
				UID:     uid,
				Name:    name,
				ID:      id,
				Parent:  pkg.PkgPath,
				Type:    "variable",
				Summary: v.Doc,
				Langs:   onlyGo,
				Syntax:  syntax{Content: printType(pkg.Fset, v.Decl)},
			})
		}
		for _, t := range docPkg.Types {
			uid := pkg.PkgPath + "." + t.Name
			pkgItem.addChild(child(uid))
			typeItem := &item{
				UID:     uid,
				Name:    t.Name,
				ID:      t.Name,
				Parent:  pkg.PkgPath,
				Type:    "type",
				Summary: t.Doc,
				Langs:   onlyGo,
				Syntax:  syntax{Content: printType(pkg.Fset, t.Decl)},
			}
			// TODO: items are added as page.Children, rather than
			// typeItem.Children, as a workaround for the DocFX template.
			// That also means methods are called functions.
			pkgPage.addItem(typeItem)
			for _, c := range t.Consts {
				name := strings.Join(c.Names, ", ")
				id := strings.Join(c.Names, ",")
				cUID := pkg.PkgPath + "." + id
				pkgItem.addChild(child(cUID))
				pkgPage.addItem(&item{
					UID:     cUID,
					Name:    name,
					ID:      id,
					Parent:  uid,
					Type:    "const",
					Summary: c.Doc,
					Langs:   onlyGo,
					Syntax:  syntax{Content: printType(pkg.Fset, c.Decl)},
				})
			}
			for _, v := range t.Vars {
				name := strings.Join(v.Names, ", ")
				id := strings.Join(v.Names, ",")
				cUID := pkg.PkgPath + "." + id
				pkgItem.addChild(child(cUID))
				pkgPage.addItem(&item{
					UID:     cUID,
					Name:    name,
					ID:      id,
					Parent:  uid,
					Type:    "variable",
					Summary: v.Doc,
					Langs:   onlyGo,
					Syntax:  syntax{Content: printType(pkg.Fset, v.Decl)},
				})
			}

			for _, fn := range t.Funcs {
				fnUID := uid + "." + fn.Name
				pkgItem.addChild(child(fnUID))
				s := Synopsis(pkg.Fset, fn.Decl)
				pkgPage.addItem(&item{
					UID:     fnUID,
					Name:    s,
					ID:      fn.Name,
					Parent:  uid,
					Type:    "function",
					Summary: fn.Doc,
					Langs:   onlyGo,
					// Note: Name has the syntax already.
					// Syntax:  Syntax{Content: s},
				})
			}
			for _, fn := range t.Methods {
				fnUID := uid + "." + fn.Name
				pkgItem.addChild(child(fnUID))
				s := Synopsis(pkg.Fset, fn.Decl)
				pkgPage.addItem(&item{
					UID:     fnUID,
					Name:    s,
					ID:      fn.Name,
					Parent:  uid,
					Type:    "function",
					Summary: fn.Doc,
					Langs:   onlyGo,
					// Note: Name has the syntax already.
					// Syntax:  Syntax{Content: s},
				})
			}
		}
		for _, fn := range docPkg.Funcs {
			uid := pkg.PkgPath + "." + fn.Name
			pkgItem.addChild(child(uid))
			s := Synopsis(pkg.Fset, fn.Decl)
			pkgPage.addItem(&item{
				UID:     uid,
				Name:    s,
				ID:      fn.Name,
				Parent:  pkg.PkgPath,
				Type:    "function",
				Summary: fn.Doc,
				Langs:   onlyGo,
				// Note: Name has the syntax already.
				// Syntax:  Syntax{Content: s},
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
	return pages, toc, module, nil
}
