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

// TODO:
//   IDs for const/var groups have every name, not just the one to link to.
//   Preserve IDs when sanitizing then use the right ID for linking.
//   Link to different domains by pattern (e.g. for cloud.google.com/go).
//   Make sure dot imports work (those identifiers aren't in the current package).

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
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	AltLink  string    `yaml:"alt_link,omitempty"`
}

func (p *page) addItem(i *item) {
	p.Items = append(p.Items, i)
}

func (i *item) addChild(c child) {
	i.Children = append(i.Children, c)
}

var onlyGo = []string{"go"}

type extraFile struct{ srcRelativePath, dstRelativePath, name string }

type result struct {
	pages      map[string]*page
	toc        tableOfContents
	module     *packages.Module
	extraFiles []extraFile
}

// parse parses the directory into a map of import path -> page and a TOC.
//
// glob is the path to parse, usually ending in `...`. glob is passed directly
// to packages.Load as-is.
//
// workingDir is the directory to use to run go commands.
//
// optionalExtraFiles is a list of paths relative to the module root to include.
func parse(glob string, workingDir string, optionalExtraFiles []string) (*result, error) {
	pages := map[string]*page{}

	pkgInfos, err := loadPackages(glob, workingDir)
	if err != nil {
		return nil, err
	}
	module := pkgInfos[0].pkg.Module

	// Filter out extra files that don't exist because some modules don't have a
	// README.
	extraFiles := []extraFile{}
	for _, f := range optionalExtraFiles {
		if _, err := os.Stat(filepath.Join(module.Dir, f)); err == nil {
			dst := f
			dir := filepath.Dir(f)
			base := filepath.Base(f)
			name := strings.TrimSuffix(base, filepath.Ext(base))
			name = strings.Title(name)
			if name == "README" {
				dst = filepath.Join(dir, "pkg-readme.md")
			}
			extraFiles = append(extraFiles, extraFile{
				srcRelativePath: f,
				dstRelativePath: dst,
				name:            name,
			})
		}
	}

	toc := buildTOC(module.Path, pkgInfos, extraFiles)

	// Once the files are grouped by package, process each package
	// independently.
	for _, pi := range pkgInfos {
		link := newLinker(pi.pkg.Imports, pi.importRenames)
		topLevelDecls := pkgsite.TopLevelDecls(pi.doc)
		pkgItem := &item{
			UID:      pi.doc.ImportPath,
			Name:     pi.doc.ImportPath,
			ID:       pi.doc.Name,
			Summary:  toHTML(pi.doc.Doc),
			Langs:    onlyGo,
			Type:     "package",
			Examples: processExamples(pi.doc.Examples, pi.fset),
			AltLink:  "https://pkg.go.dev/" + pi.doc.ImportPath,
		}
		pkgPage := &page{Items: []*item{pkgItem}}
		pages[pi.doc.ImportPath] = pkgPage

		for _, c := range pi.doc.Consts {
			name := strings.Join(c.Names, ", ")
			id := strings.Join(c.Names, ",")
			uid := pi.doc.ImportPath + "." + id
			pkgItem.addChild(child(uid))
			pkgPage.addItem(&item{
				UID:     uid,
				Name:    name,
				ID:      id,
				Parent:  pi.doc.ImportPath,
				Type:    "const",
				Summary: c.Doc,
				Langs:   onlyGo,
				Syntax:  syntax{Content: pkgsite.PrintType(pi.fset, c.Decl, toURL, topLevelDecls)},
			})
		}
		for _, v := range pi.doc.Vars {
			name := strings.Join(v.Names, ", ")
			id := strings.Join(v.Names, ",")
			uid := pi.doc.ImportPath + "." + id
			pkgItem.addChild(child(uid))
			pkgPage.addItem(&item{
				UID:     uid,
				Name:    name,
				ID:      id,
				Parent:  pi.doc.ImportPath,
				Type:    "variable",
				Summary: v.Doc,
				Langs:   onlyGo,
				Syntax:  syntax{Content: pkgsite.PrintType(pi.fset, v.Decl, toURL, topLevelDecls)},
			})
		}
		for _, t := range pi.doc.Types {
			uid := pi.doc.ImportPath + "." + t.Name
			pkgItem.addChild(child(uid))
			typeItem := &item{
				UID:      uid,
				Name:     t.Name,
				ID:       t.Name,
				Parent:   pi.doc.ImportPath,
				Type:     "type",
				Summary:  t.Doc,
				Langs:    onlyGo,
				Syntax:   syntax{Content: pkgsite.PrintType(pi.fset, t.Decl, toURL, topLevelDecls)},
				Examples: processExamples(t.Examples, pi.fset),
			}
			// Note: items are added as page.Children, rather than
			// typeItem.Children, as a workaround for the DocFX template.
			pkgPage.addItem(typeItem)
			for _, c := range t.Consts {
				name := strings.Join(c.Names, ", ")
				id := strings.Join(c.Names, ",")
				cUID := pi.doc.ImportPath + "." + id
				pkgItem.addChild(child(cUID))
				pkgPage.addItem(&item{
					UID:     cUID,
					Name:    name,
					ID:      id,
					Parent:  uid,
					Type:    "const",
					Summary: c.Doc,
					Langs:   onlyGo,
					Syntax:  syntax{Content: pkgsite.PrintType(pi.fset, c.Decl, toURL, topLevelDecls)},
				})
			}
			for _, v := range t.Vars {
				name := strings.Join(v.Names, ", ")
				id := strings.Join(v.Names, ",")
				cUID := pi.doc.ImportPath + "." + id
				pkgItem.addChild(child(cUID))
				pkgPage.addItem(&item{
					UID:     cUID,
					Name:    name,
					ID:      id,
					Parent:  uid,
					Type:    "variable",
					Summary: v.Doc,
					Langs:   onlyGo,
					Syntax:  syntax{Content: pkgsite.PrintType(pi.fset, v.Decl, toURL, topLevelDecls)},
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
					Syntax:   syntax{Content: pkgsite.Synopsis(pi.fset, fn.Decl, link.linkify)},
					Examples: processExamples(fn.Examples, pi.fset),
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
					Syntax:   syntax{Content: pkgsite.Synopsis(pi.fset, fn.Decl, link.linkify)},
					Examples: processExamples(fn.Examples, pi.fset),
				})
			}
		}
		for _, fn := range pi.doc.Funcs {
			uid := pi.doc.ImportPath + "." + fn.Name
			pkgItem.addChild(child(uid))
			pkgPage.addItem(&item{
				UID:      uid,
				Name:     fmt.Sprintf("func %s\n", fn.Name),
				ID:       fn.Name,
				Parent:   pi.doc.ImportPath,
				Type:     "function",
				Summary:  fn.Doc,
				Langs:    onlyGo,
				Syntax:   syntax{Content: pkgsite.Synopsis(pi.fset, fn.Decl, link.linkify)},
				Examples: processExamples(fn.Examples, pi.fset),
			})
		}
	}

	return &result{
		pages:      pages,
		toc:        toc,
		module:     module,
		extraFiles: extraFiles,
	}, nil
}

type linker struct {
	// imports is a map from local package name to import path.
	// Behavior is undefined when a single import has different names in
	// different files.
	imports map[string]string
}

func newLinker(rawImports map[string]*packages.Package, importSyntax map[string]string) *linker {
	imports := map[string]string{}
	for path, pkg := range rawImports {
		name := pkg.Name
		if rename := importSyntax[path]; rename != "" {
			name = rename
		}
		imports[name] = path
	}
	return &linker{imports: imports}
}

func (l *linker) linkify(s string) string {
	prefix := ""
	if strings.HasPrefix(s, "...") {
		s = s[3:]
		prefix = "..."
	}
	if s[0] == '*' {
		s = s[1:]
		prefix += "*"
	}

	if !strings.Contains(s, ".") {
		// If s is not exported, it's probably a builtin.
		if !token.IsExported(s) {
			if doc.IsPredeclared(s) {
				return href(toURL("builtin", s), s)
			}
			return fmt.Sprintf("%s%s", prefix, s)
		}
		return fmt.Sprintf("%s%s", prefix, href(toURL("", s), s))
	}
	// Otherwise, it's in another package.
	split := strings.Split(s, ".")
	if len(split) != 2 {
		// Don't know how to link this.
		return fmt.Sprintf("%s%s", prefix, s)
	}

	pkg := split[0]
	pkgPath, ok := l.imports[pkg]
	if !ok {
		// Don't know how to link this.
		return fmt.Sprintf("%s%s", prefix, s)
	}
	name := split[1]
	return fmt.Sprintf("%s%s.%s", prefix, href(toURL(pkgPath, ""), pkg), href(toURL(pkgPath, name), name))
}

// TODO: link to the right baseURL, with the right module name and version
// pattern.
func toURL(pkg, name string) string {
	if pkg == "" {
		return fmt.Sprintf("#%s", strings.ToLower(name))
	}
	baseURL := "https://pkg.go.dev"
	if name == "" {
		return fmt.Sprintf("%s/%s", baseURL, pkg)
	}
	return fmt.Sprintf("%s/%s#%s", baseURL, pkg, name)
}

func href(url, text string) string {
	return fmt.Sprintf(`<a href="%s">%s</a>`, url, text)
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

func buildTOC(mod string, pis []pkgInfo, extraFiles []extraFile) tableOfContents {
	toc := tableOfContents{}

	modTOC := &tocItem{
		UID:  mod,
		Name: mod,
	}

	// Assume the module root has a package.
	modTOC.addItem(&tocItem{
		UID:  mod,
		Name: mod,
	})

	for _, ef := range extraFiles {
		modTOC.addItem(&tocItem{
			Href: ef.dstRelativePath,
			Name: ef.name,
		})
	}

	toc = append(toc, modTOC)

	if len(pis) == 1 {
		// The module only has one package.
		return toc
	}

	trimmedPkgs := []string{}
	for _, pi := range pis {
		importPath := pi.doc.ImportPath
		if importPath == mod {
			continue
		}
		if !strings.HasPrefix(importPath, mod) {
			panic(fmt.Sprintf("Package %q does not start with %q, should never happen", importPath, mod))
		}
		trimmed := strings.TrimPrefix(importPath, mod+"/")
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

func toHTML(s string) string {
	buf := &bytes.Buffer{}
	doc.ToHTML(buf, s, nil)
	return buf.String()
}

type pkgInfo struct {
	pkg  *packages.Package
	doc  *doc.Package
	fset *token.FileSet
	// importRenames is a map from package path to local name or "".
	importRenames map[string]string
}

func loadPackages(glob, workingDir string) ([]pkgInfo, error) {
	config := &packages.Config{
		Mode:  packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedModule | packages.NeedImports,
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
		} else {
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

	result := []pkgInfo{}

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

		result = append(result, pkgInfo{
			pkg:           idToPkg[pkgPath],
			doc:           docPkg,
			fset:          fset,
			importRenames: imports,
		})
	}

	return result, nil
}
