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

// Package filter provides a parser that creates an ASTNode
// from a filter string, which can then be used to evaluate
// attributes. See comment in [ParseFilter] for example.
package filter

import (
	"fmt"
	"strings"
)

const (
	attributesStr = "attributes"
	hasPrefixStr  = "hasPrefix"
)

// ASTNode represents a node in the Abstract Syntax Tree of a filter.
type ASTNode interface {
}

type identNode struct {
	name string
}

type stringNode struct {
	val string
}

type opNode struct {
	op    string // ":", "=", "!=", "AND", "OR", "NOT", "-"
	left  ASTNode
	right ASTNode
}

type funcNode struct {
	name string
	args []ASTNode
}

type tokenType int

const (
	tokEOF tokenType = iota
	tokIdent
	tokOp
	tokString
	tokLParen
	tokRParen
	tokComma
	tokInvalid
)

type token struct {
	typ tokenType
	val string
}

type lexer struct {
	input string
	pos   int
}

func (l *lexer) nextToken() token {
	for l.pos < len(l.input) && isWhitespace(l.input[l.pos]) {
		l.pos++
	}

	if l.pos >= len(l.input) {
		return token{typ: tokEOF}
	}

	ch := l.input[l.pos]

	switch ch {
	case '(':
		l.pos++
		return token{typ: tokLParen, val: "("}
	case ')':
		l.pos++
		return token{typ: tokRParen, val: ")"}
	case ',':
		l.pos++
		return token{typ: tokComma, val: ","}
	case ':':
		l.pos++
		return token{typ: tokOp, val: ":"}
	case '=':
		l.pos++
		return token{typ: tokOp, val: "="}
	case '!':
		if l.pos+1 < len(l.input) && l.input[l.pos+1] == '=' {
			l.pos += 2
			return token{typ: tokOp, val: "!="}
		}
	case '-':
		l.pos++
		return token{typ: tokOp, val: "-"}
	}

	if ch == '"' {
		return l.lexString()
	}

	if isAlphaNumeric(ch) || ch == '_' || ch == '-' || ch == '.' {
		return l.lexIdent()
	}

	// Handle invalid character or fallback
	l.pos++
	return token{typ: tokInvalid, val: string(ch)}
}

func (l *lexer) lexString() token {
	l.pos++ // skip opening quote
	start := l.pos
	for l.pos < len(l.input) {
		if l.input[l.pos] == '\\' && l.pos+1 < len(l.input) {
			l.pos += 2
			continue
		}
		if l.input[l.pos] == '"' {
			break
		}
		l.pos++
	}
	val := l.input[start:l.pos]
	// Simple unescape for \" and \\
	val = strings.ReplaceAll(val, "\\\"", "\"")
	val = strings.ReplaceAll(val, "\\\\", "\\")
	if l.pos < len(l.input) {
		l.pos++ // skip closing quote
	}
	return token{typ: tokString, val: val}
}

func (l *lexer) lexIdent() token {
	start := l.pos
	for l.pos < len(l.input) && (isAlphaNumeric(l.input[l.pos]) || l.input[l.pos] == '_' || l.input[l.pos] == '-' || l.input[l.pos] == '.') {
		l.pos++
	}
	val := l.input[start:l.pos]

	// Check if it's an operator
	switch val {
	case "AND", "OR", "NOT":
		return token{typ: tokOp, val: val}
	}

	return token{typ: tokIdent, val: val}
}

func isWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func isAlphaNumeric(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
}

type parser struct {
	lexer *lexer
	curr  token
}

func (p *parser) next() {
	p.curr = p.lexer.nextToken()
}

func (p *parser) parse() (ASTNode, error) {
	p.next()
	node, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.curr.typ != tokEOF {
		return nil, fmt.Errorf("unexpected trailing tokens: %v", p.curr)
	}
	return node, nil
}

func (p *parser) parseExpr() (ASTNode, error) {
	return p.parseOr()
}

func (p *parser) parseOr() (ASTNode, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.curr.typ == tokOp && p.curr.val == "OR" {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &opNode{op: "OR", left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (ASTNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.curr.typ == tokOp && p.curr.val == "AND" {
		p.next()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &opNode{op: "AND", left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseUnary() (ASTNode, error) {
	if p.curr.typ == tokOp && (p.curr.val == "NOT" || p.curr.val == "-") {
		op := p.curr.val
		p.next()
		expr, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &opNode{op: op, left: expr}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (ASTNode, error) {
	switch p.curr.typ {
	case tokLParen:
		p.next()
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.curr.typ != tokRParen {
			return nil, fmt.Errorf("expected ')'")
		}
		p.next()
		return expr, nil
	case tokIdent:
		name := p.curr.val
		p.next()
		if p.curr.typ == tokLParen {
			// Function call
			p.next()
			var args []ASTNode
			for p.curr.typ != tokRParen {
				arg, err := p.parseExpr()
				if err != nil {
					return nil, err
				}
				args = append(args, arg)
				if p.curr.typ == tokComma {
					p.next()
				}
			}
			p.next() // skip ')'
			if name == hasPrefixStr && len(args) != 2 {
				return nil, fmt.Errorf("hasPrefix requires exactly 2 arguments")
			}
			return &funcNode{name: name, args: args}, nil
		}
		if p.curr.typ == tokOp && (p.curr.val == ":" || p.curr.val == "=" || p.curr.val == "!=") {
			op := p.curr.val
			p.next()
			var right ASTNode
			if p.curr.typ == tokString {
				right = &stringNode{val: p.curr.val}
				p.next()
			} else if p.curr.typ == tokIdent {
				right = &identNode{name: p.curr.val}
				p.next()
			} else {
				return nil, fmt.Errorf("expected string or ident on right of operator")
			}
			return &opNode{op: op, left: &identNode{name: name}, right: right}, nil
		}
		return &identNode{name: name}, nil
	case tokString:
		val := p.curr.val
		p.next()
		return &stringNode{val: val}, nil
	case tokEOF:
		return nil, fmt.Errorf("unexpected EOF")
	case tokInvalid:
		return nil, fmt.Errorf("invalid character: %s", p.curr.val)
	default:
		return nil, fmt.Errorf("unexpected token: %v", p.curr)
	}
}

// ParseFilter validates a filter string and returns an AST node.
//
// Example usage combining parsing and evaluation:
//
//	ast, err := ParseFilter(`attributes.name = "com"`)
//	if err != nil {
//	    // Handle error
//	}
//	attrs := map[string]string{"name": "com"}
//	matched := Evaluate(ast, attrs) // returns true
func ParseFilter(filter string) (ASTNode, error) {
	l := &lexer{input: filter}
	p := &parser{lexer: l}
	return p.parse()
}

// Evaluate applies the AST to message attributes.
func Evaluate(node ASTNode, attrs map[string]string) bool {
	switch n := node.(type) {
	case *identNode:
		return false
	case *stringNode:
		return false
	case *opNode:
		switch n.op {
		case "OR":
			return Evaluate(n.left, attrs) || Evaluate(n.right, attrs)
		case "AND":
			return Evaluate(n.left, attrs) && Evaluate(n.right, attrs)
		case "NOT", "-":
			return !Evaluate(n.left, attrs)
		case ":":
			ident, ok := n.left.(*identNode)
			if !ok || ident.name != attributesStr {
				return false
			}
			key, ok := n.right.(*stringNode)
			if ok {
				_, exists := attrs[key.val]
				return exists
			}
			rightIdent, ok := n.right.(*identNode)
			if ok {
				_, exists := attrs[rightIdent.name]
				return exists
			}
			return false
		case "=":
			ident, ok := n.left.(*identNode)
			if !ok {
				return false
			}
			if !strings.HasPrefix(ident.name, attributesStr+".") {
				return false
			}
			key := ident.name[len(attributesStr)+1:]
			valNode, ok := n.right.(*stringNode)
			if !ok {
				return false
			}
			v, exists := attrs[key]
			return exists && v == valNode.val
		case "!=":
			ident, ok := n.left.(*identNode)
			if !ok {
				return false
			}
			if !strings.HasPrefix(ident.name, attributesStr+".") {
				return false
			}
			key := ident.name[len(attributesStr)+1:]
			valNode, ok := n.right.(*stringNode)
			if !ok {
				return false
			}
			v, exists := attrs[key]
			return !exists || v != valNode.val
		}
	case *funcNode:
		if n.name == hasPrefixStr {
			if len(n.args) != 2 {
				return false
			}
			ident, ok := n.args[0].(*identNode)
			if !ok || !strings.HasPrefix(ident.name, attributesStr+".") {
				return false
			}
			key := ident.name[len(attributesStr)+1:]
			prefixNode, ok := n.args[1].(*stringNode)
			if !ok {
				return false
			}
			v, exists := attrs[key]
			return exists && strings.HasPrefix(v, prefixNode.val)
		}
	}
	return false
}
