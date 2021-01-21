// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.15

// Package pkgsite is not for external use. May change at any time without
// warning.
//
// Copied from
// https://github.com/golang/pkgsite/blob/ff1e697b104e751da362159cf6c7743898eea3fe/internal/fetch/dochtml/internal/render/
// and
// https://github.com/golang/pkgsite/tree/88f8a28ab2102416529d05d11e8135a43e146d46/internal/fetch/dochtml.
package pkgsite

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"strconv"
	"strings"
)

// PrintType returns a string representation of the decl.
func PrintType(fset *token.FileSet, decl ast.Decl) string {
	v := &declVisitor{}
	ast.Walk(v, decl)

	var b bytes.Buffer
	p := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 4}
	p.Fprint(&b, fset, &printer.CommentedNode{Node: decl, Comments: v.Comments})
	return b.String()
}

// declVisitor is used to walk over the AST and trim large string
// literals and arrays before package documentation is rendered.
// Comments are added to Comments to indicate that a part of the
// original code is not displayed.
type declVisitor struct {
	Comments []*ast.CommentGroup
}

// Visit implements ast.Visitor.
func (v *declVisitor) Visit(n ast.Node) ast.Visitor {
	switch n := n.(type) {
	case *ast.BasicLit:
		if n.Kind == token.STRING && len(n.Value) > 128 {
			v.Comments = append(v.Comments,
				&ast.CommentGroup{List: []*ast.Comment{{
					Slash: n.Pos(),
					Text:  stringBasicLitSize(n.Value),
				}}})
			n.Value = `""`
		}
	case *ast.CompositeLit:
		if len(n.Elts) > 100 {
			v.Comments = append(v.Comments,
				&ast.CommentGroup{List: []*ast.Comment{{
					Slash: n.Lbrace,
					Text:  fmt.Sprintf("/* %d elements not displayed */", len(n.Elts)),
				}}})
			n.Elts = n.Elts[:0]
		}
	}
	return v
}

// stringBasicLitSize computes the number of bytes in the given string basic literal.
//
// See noder.basicLit and syntax.StringLit cases in cmd/compile/internal/gc/noder.go.
func stringBasicLitSize(s string) string {
	if len(s) > 0 && s[0] == '`' {
		// strip carriage returns from raw string
		s = strings.ReplaceAll(s, "\r", "")
	}
	u, err := strconv.Unquote(s)
	if err != nil {
		return fmt.Sprintf("/* invalid %d byte string literal not displayed */", len(s))
	}
	return fmt.Sprintf("/* %d byte string literal not displayed */", len(u))
}
