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

//go:build go1.15
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
	"go/format"
	"go/printer"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	goldmarkcodeblock "cloud.google.com/go/internal/godocfx/goldmark-codeblock"
	"cloud.google.com/go/internal/godocfx/pkgload"
	"cloud.google.com/go/third_party/go/doc"
	"cloud.google.com/go/third_party/pkgsite"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"
	"golang.org/x/tools/go/packages"
)

// tableOfContents represents a TOC.
type tableOfContents []*tocItem

// tocItem is an item in a TOC.
type tocItem struct {
	UID    string     `yaml:"uid,omitempty"`
	Name   string     `yaml:"name,omitempty"`
	Items  []*tocItem `yaml:"items,omitempty"`
	Href   string     `yaml:"href,omitempty"`
	Status string     `yaml:"status,omitempty"`
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
	Status   string    `yaml:"status,omitempty"`
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
func parse(glob string, workingDir string, optionalExtraFiles []string, filter []string) (*result, error) {
	pages := map[string]*page{}

	pkgInfos, err := pkgload.Load(glob, workingDir, filter)
	if err != nil {
		return nil, err
	}
	module := pkgInfos[0].Pkg.Module

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
		link := newLinker(pi)
		topLevelDecls := pkgsite.TopLevelDecls(pi.Doc)
		pkgItem := &item{
			UID:      pi.Doc.ImportPath,
			Name:     pi.Doc.ImportPath,
			ID:       pi.Doc.Name,
			Summary:  toHTML(pi.Doc.Doc),
			Langs:    onlyGo,
			Type:     "package",
			Examples: processExamples(pi.Doc.Examples, pi.Fset),
			AltLink:  "https://pkg.go.dev/" + pi.Doc.ImportPath,
			Status:   pi.Status,
		}
		pkgPage := &page{Items: []*item{pkgItem}}
		pages[pi.Doc.ImportPath] = pkgPage

		for _, c := range pi.Doc.Consts {
			name := strings.Join(c.Names, ", ")
			id := strings.Join(c.Names, ",")
			uid := pi.Doc.ImportPath + "." + id
			pkgItem.addChild(child(uid))
			pkgPage.addItem(&item{
				UID:     uid,
				Name:    name,
				ID:      id,
				Parent:  pi.Doc.ImportPath,
				Type:    "const",
				Summary: c.Doc,
				Langs:   onlyGo,
				Syntax:  syntax{Content: pkgsite.PrintType(pi.Fset, c.Decl, link.toURL, topLevelDecls)},
				Status:  getStatus(c.Doc),
			})
		}
		for _, v := range pi.Doc.Vars {
			name := strings.Join(v.Names, ", ")
			id := strings.Join(v.Names, ",")
			uid := pi.Doc.ImportPath + "." + id
			pkgItem.addChild(child(uid))
			pkgPage.addItem(&item{
				UID:     uid,
				Name:    name,
				ID:      id,
				Parent:  pi.Doc.ImportPath,
				Type:    "variable",
				Summary: v.Doc,
				Langs:   onlyGo,
				Syntax:  syntax{Content: pkgsite.PrintType(pi.Fset, v.Decl, link.toURL, topLevelDecls)},
				Status:  getStatus(v.Doc),
			})
		}
		for _, t := range pi.Doc.Types {
			uid := pi.Doc.ImportPath + "." + t.Name
			pkgItem.addChild(child(uid))
			typeItem := &item{
				UID:      uid,
				Name:     t.Name,
				ID:       t.Name,
				Parent:   pi.Doc.ImportPath,
				Type:     "type",
				Summary:  t.Doc,
				Langs:    onlyGo,
				Syntax:   syntax{Content: pkgsite.PrintType(pi.Fset, t.Decl, link.toURL, topLevelDecls)},
				Examples: processExamples(t.Examples, pi.Fset),
				Status:   getStatus(t.Doc),
			}
			// Note: items are added as page.Children, rather than
			// typeItem.Children, as a workaround for the DocFX template.
			pkgPage.addItem(typeItem)
			for _, c := range t.Consts {
				name := strings.Join(c.Names, ", ")
				id := strings.Join(c.Names, ",")
				cUID := pi.Doc.ImportPath + "." + id
				pkgItem.addChild(child(cUID))
				pkgPage.addItem(&item{
					UID:     cUID,
					Name:    name,
					ID:      id,
					Parent:  uid,
					Type:    "const",
					Summary: c.Doc,
					Langs:   onlyGo,
					Syntax:  syntax{Content: pkgsite.PrintType(pi.Fset, c.Decl, link.toURL, topLevelDecls)},
					Status:  getStatus(c.Doc),
				})
			}
			for _, v := range t.Vars {
				name := strings.Join(v.Names, ", ")
				id := strings.Join(v.Names, ",")
				cUID := pi.Doc.ImportPath + "." + id
				pkgItem.addChild(child(cUID))
				pkgPage.addItem(&item{
					UID:     cUID,
					Name:    name,
					ID:      id,
					Parent:  uid,
					Type:    "variable",
					Summary: v.Doc,
					Langs:   onlyGo,
					Syntax:  syntax{Content: pkgsite.PrintType(pi.Fset, v.Decl, link.toURL, topLevelDecls)},
					Status:  getStatus(v.Doc),
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
					Syntax:   syntax{Content: pkgsite.Synopsis(pi.Fset, fn.Decl, link.linkify)},
					Examples: processExamples(fn.Examples, pi.Fset),
					Status:   getStatus(fn.Doc),
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
					Syntax:   syntax{Content: pkgsite.Synopsis(pi.Fset, fn.Decl, link.linkify)},
					Examples: processExamples(fn.Examples, pi.Fset),
					Status:   getStatus(fn.Doc),
				})
			}
		}
		for _, fn := range pi.Doc.Funcs {
			uid := pi.Doc.ImportPath + "." + fn.Name
			pkgItem.addChild(child(uid))
			pkgPage.addItem(&item{
				UID:      uid,
				Name:     fmt.Sprintf("func %s\n", fn.Name),
				ID:       fn.Name,
				Parent:   pi.Doc.ImportPath,
				Type:     "function",
				Summary:  fn.Doc,
				Langs:    onlyGo,
				Syntax:   syntax{Content: pkgsite.Synopsis(pi.Fset, fn.Decl, link.linkify)},
				Examples: processExamples(fn.Examples, pi.Fset),
				Status:   getStatus(fn.Doc),
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

// getStatus returns a possibly empty status string for the given
// docs.
func getStatus(doc string) string {
	deprecated := "\nDeprecated:"
	if strings.Contains(doc, deprecated) {
		return "deprecated"
	}
	return ""
}

type linker struct {
	// imports is a map from local package name to import path.
	// Behavior is undefined when a single import has different names in
	// different files.
	imports map[string]string

	// idToAnchor is a map from package path to a map from ID to the anchor for
	// that ID.
	idToAnchor map[string]map[string]string

	// sameDomainModules is a map from package path to module for every imported
	// package that should cross link on the same domain.
	sameDomainModules map[string]*packages.Module
}

func newLinker(pi pkgload.Info) *linker {
	sameDomainPrefixes := []string{"cloud.google.com/go"}

	imports := map[string]string{}
	sameDomainModules := map[string]*packages.Module{}
	idToAnchor := map[string]map[string]string{}

	for path, pkg := range pi.Pkg.Imports {
		name := pkg.Name
		if rename := pi.ImportRenames[path]; rename != "" {
			name = rename
		}
		imports[name] = path

		// TODO: Consider documenting internal packages so we don't have to link
		// out.
		if pkg.Module != nil && hasPrefix(pkg.PkgPath, sameDomainPrefixes) && !strings.Contains(pkg.PkgPath, "internal") {
			sameDomainModules[path] = pkg.Module

			docPkg, _ := doc.NewFromFiles(pkg.Fset, pkg.Syntax, path)
			idToAnchor[path] = buildIDToAnchor(docPkg)
		}
	}

	idToAnchor[""] = buildIDToAnchor(pi.Doc)

	return &linker{imports: imports, idToAnchor: idToAnchor, sameDomainModules: sameDomainModules}
}

// nonWordRegex is based on
// https://github.com/googleapis/doc-templates/blob/70eba5908e7b9aef5525d0f1f24194ae750f267e/third_party/docfx/templates/devsite/common.js#L27-L30.
var nonWordRegex = regexp.MustCompile(`\W`)

func buildIDToAnchor(pkg *doc.Package) map[string]string {
	idToAnchor := map[string]string{}
	idToAnchor[pkg.ImportPath] = pkg.ImportPath

	for _, c := range pkg.Consts {
		commaID := strings.Join(c.Names, ",")
		uid := pkg.ImportPath + "." + commaID
		for _, name := range c.Names {
			idToAnchor[name] = uid
		}
	}
	for _, v := range pkg.Vars {
		commaID := strings.Join(v.Names, ",")
		uid := pkg.ImportPath + "." + commaID
		for _, name := range v.Names {
			idToAnchor[name] = uid
		}
	}
	for _, f := range pkg.Funcs {
		uid := pkg.ImportPath + "." + f.Name
		idToAnchor[f.Name] = uid
	}
	for _, t := range pkg.Types {
		uid := pkg.ImportPath + "." + t.Name
		idToAnchor[t.Name] = uid
		for _, c := range t.Consts {
			commaID := strings.Join(c.Names, ",")
			uid := pkg.ImportPath + "." + commaID
			for _, name := range c.Names {
				idToAnchor[name] = uid
			}
		}
		for _, v := range t.Vars {
			commaID := strings.Join(v.Names, ",")
			uid := pkg.ImportPath + "." + commaID
			for _, name := range v.Names {
				idToAnchor[name] = uid
			}
		}
		for _, f := range t.Funcs {
			uid := pkg.ImportPath + "." + t.Name + "." + f.Name
			idToAnchor[f.Name] = uid
		}
		for _, m := range t.Methods {
			uid := pkg.ImportPath + "." + t.Name + "." + m.Name
			idToAnchor[m.Name] = uid
		}
	}

	for id, anchor := range idToAnchor {
		idToAnchor[id] = nonWordRegex.ReplaceAllString(anchor, "_")
	}

	return idToAnchor
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
				return href(l.toURL("builtin", s), s)
			}
			return fmt.Sprintf("%s%s", prefix, s)
		}
		return fmt.Sprintf("%s%s", prefix, href(l.toURL("", s), s))
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
	return fmt.Sprintf("%s%s.%s", prefix, href(l.toURL(pkgPath, ""), pkg), href(l.toURL(pkgPath, name), name))
}

func (l *linker) toURL(pkg, name string) string {
	if pkg == "" {
		if anchor := l.idToAnchor[""][name]; anchor != "" {
			name = anchor
		}
		return fmt.Sprintf("#%s", name)
	}
	if mod, ok := l.sameDomainModules[pkg]; ok {
		pkgRemainder := ""
		if pkg != mod.Path {
			pkgRemainder = pkg[len(mod.Path)+1:] // +1 to skip slash.
		}
		// Note: we always link to latest. One day, we'll link to mod.Version.
		// Also, other packages may have different paths.
		baseURL := fmt.Sprintf("/go/docs/reference/%v/latest/%v", mod.Path, pkgRemainder)
		if anchor := l.idToAnchor[pkg][name]; anchor != "" {
			return fmt.Sprintf("%s#%s", baseURL, anchor)
		}
		return baseURL
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

func buildTOC(mod string, pis []pkgload.Info, extraFiles []extraFile) tableOfContents {
	toc := tableOfContents{}

	// If all of the packages have the same status, only put the status on
	// the module instead of all of the individual packages.
	uniqueStatuses := map[string]struct{}{}
	for _, pi := range pis {
		uniqueStatuses[pi.Status] = struct{}{}
	}
	modStatus := ""
	if len(uniqueStatuses) == 1 {
		for status := range uniqueStatuses {
			modStatus = status
		}
	}

	modTOC := &tocItem{
		UID:    mod,
		Name:   mod,
		Status: modStatus,
	}

	for _, ef := range extraFiles {
		modTOC.addItem(&tocItem{
			Href: ef.dstRelativePath,
			Name: ef.name,
		})
	}

	toc = append(toc, modTOC)

	trimmedPkgs := []string{}
	statuses := map[string]string{}
	for _, pi := range pis {
		importPath := pi.Doc.ImportPath
		if importPath == mod {
			// Add the module root package immediately with the full name.
			rootPkgStatus := pi.Status
			if modStatus != "" {
				rootPkgStatus = ""
			}
			modTOC.addItem(&tocItem{
				UID:    mod,
				Name:   mod,
				Status: rootPkgStatus,
			})
			continue
		}
		if !strings.HasPrefix(importPath, mod) {
			panic(fmt.Sprintf("Package %q does not start with %q, should never happen", importPath, mod))
		}
		trimmed := strings.TrimPrefix(importPath, mod+"/")
		trimmedPkgs = append(trimmedPkgs, trimmed)
		if modStatus == "" {
			statuses[trimmed] = pi.Status
		}
	}

	sort.Strings(trimmedPkgs)

	for _, trimmed := range trimmedPkgs {
		uid := mod + "/" + trimmed
		pkgTOCItem := &tocItem{
			UID:    uid,
			Name:   trimmed,
			Status: statuses[trimmed],
		}
		modTOC.addItem(pkgTOCItem)
	}

	return toc
}

func toHTML(s string) string {
	buf := &bytes.Buffer{}
	// First, convert to Markdown.
	doc.ToMarkdown(buf, s, nil)

	// Then, handle Markdown stuff, like lists and links.
	md := goldmark.New(goldmark.WithRendererOptions(html.WithUnsafe()), goldmark.WithExtensions(goldmarkcodeblock.CodeBlock))
	mdBuf := &bytes.Buffer{}
	if err := md.Convert(buf.Bytes(), mdBuf); err != nil {
		panic(err)
	}

	// Replace * with &#42; to avoid confusing the DocFX Markdown processor,
	// which sometimes interprets * as <em>.
	result := string(bytes.ReplaceAll(mdBuf.Bytes(), []byte("*"), []byte("&#42;")))

	return result
}

func hasPrefix(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}
