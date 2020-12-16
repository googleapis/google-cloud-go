/*
Copyright 2019 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

/*
Package spansql contains types and a parser for the Cloud Spanner SQL dialect.

To parse, use one of the Parse functions (ParseDDL, ParseDDLStmt, ParseQuery, etc.).

Sources:
	https://cloud.google.com/spanner/docs/lexical
	https://cloud.google.com/spanner/docs/query-syntax
	https://cloud.google.com/spanner/docs/data-definition-language
*/
package spansql

/*
This file is structured as follows:

- There are several exported ParseFoo functions that accept an input string
  and return a type defined in types.go. This is the principal API of this package.
  These functions are implemented as wrappers around the lower-level functions,
  with additional checks to ensure things such as input exhaustion.
- The token and parser types are defined. These constitute the lexical token
  and parser machinery. parser.next is the main way that other functions get
  the next token, with parser.back providing a single token rewind, and
  parser.sniff, parser.eat and parser.expect providing lookahead helpers.
- The parseFoo methods are defined, matching the SQL grammar. Each consumes its
  namesake production from the parser. There are also some fooParser helper vars
  defined that abbreviate the parsing of some of the regular productions.
*/

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

const debug = false

func debugf(format string, args ...interface{}) {
	if !debug {
		return
	}
	fmt.Fprintf(os.Stderr, "spansql debug: "+format+"\n", args...)
}

// ParseDDL parses a DDL file.
//
// The provided filename is used for error reporting and will
// appear in the returned structure.
func ParseDDL(filename, s string) (*DDL, error) {
	p := newParser(filename, s)

	ddl := &DDL{
		Filename: filename,
	}
	for {
		p.skipSpace()
		if p.done {
			break
		}

		stmt, err := p.parseDDLStmt()
		if err != nil {
			return nil, err
		}
		ddl.List = append(ddl.List, stmt)

		tok := p.next()
		if tok.err == eof {
			break
		} else if tok.err != nil {
			return nil, tok.err
		}
		if tok.value == ";" {
			continue
		} else {
			return nil, p.errorf("unexpected token %q", tok.value)
		}
	}
	if p.Rem() != "" {
		return nil, fmt.Errorf("unexpected trailing contents %q", p.Rem())
	}

	// Handle comments.
	for _, com := range p.comments {
		c := &Comment{
			Marker:   com.marker,
			Isolated: com.isolated,
			Start:    com.start,
			End:      com.end,
			Text:     com.text,
		}

		// Strip common whitespace prefix and any whitespace suffix.
		// TODO: This is a bodgy implementation of Longest Common Prefix,
		// and also doesn't do tabs vs. spaces well.
		var prefix string
		for i, line := range c.Text {
			line = strings.TrimRight(line, " \b\t")
			c.Text[i] = line
			trim := len(line) - len(strings.TrimLeft(line, " \b\t"))
			if i == 0 {
				prefix = line[:trim]
			} else {
				// Check how much of prefix is in common.
				for !strings.HasPrefix(line, prefix) {
					prefix = prefix[:len(prefix)-1]
				}
			}
			if prefix == "" {
				break
			}
		}
		if prefix != "" {
			for i, line := range c.Text {
				c.Text[i] = strings.TrimPrefix(line, prefix)
			}
		}

		ddl.Comments = append(ddl.Comments, c)
	}

	return ddl, nil
}

// ParseDDLStmt parses a single DDL statement.
func ParseDDLStmt(s string) (DDLStmt, error) {
	p := newParser("-", s)
	stmt, err := p.parseDDLStmt()
	if err != nil {
		return nil, err
	}
	if p.Rem() != "" {
		return nil, fmt.Errorf("unexpected trailing contents %q", p.Rem())
	}
	return stmt, nil
}

// ParseDMLStmt parses a single DML statement.
func ParseDMLStmt(s string) (DMLStmt, error) {
	p := newParser("-", s)
	stmt, err := p.parseDMLStmt()
	if err != nil {
		return nil, err
	}
	if p.Rem() != "" {
		return nil, fmt.Errorf("unexpected trailing contents %q", p.Rem())
	}
	return stmt, nil
}

// ParseQuery parses a query string.
func ParseQuery(s string) (Query, error) {
	p := newParser("-", s)
	q, err := p.parseQuery()
	if err != nil {
		return Query{}, err
	}
	if p.Rem() != "" {
		return Query{}, fmt.Errorf("unexpected trailing query contents %q", p.Rem())
	}
	return q, nil
}

type token struct {
	value        string
	err          *parseError
	line, offset int

	typ     tokenType
	float64 float64
	string  string // unquoted form for stringToken/bytesToken/quotedID

	// int64Token is parsed as a number only when it is known to be a literal.
	// This permits correct handling of operators preceding such a token,
	// which cannot be identified as part of the int64 until later.
	int64Base int
}

type tokenType int

const (
	unknownToken tokenType = iota
	int64Token
	float64Token
	stringToken
	bytesToken
	unquotedID
	quotedID
)

func (t *token) String() string {
	if t.err != nil {
		return fmt.Sprintf("parse error: %v", t.err)
	}
	return strconv.Quote(t.value)
}

type parseError struct {
	message  string
	filename string
	line     int // 1-based line number
	offset   int // 0-based byte offset from start of input
}

func (pe *parseError) Error() string {
	if pe == nil {
		return "<nil>"
	}
	if pe.line == 1 {
		return fmt.Sprintf("%s:1.%d: %v", pe.filename, pe.offset, pe.message)
	}
	return fmt.Sprintf("%s:%d: %v", pe.filename, pe.line, pe.message)
}

var eof = &parseError{message: "EOF"}

type parser struct {
	s      string // Remaining input.
	done   bool   // Whether the parsing is finished (success or error).
	backed bool   // Whether back() was called.
	cur    token

	filename     string
	line, offset int // updated by places that shrink s

	comments []comment // accumulated during parse
}

type comment struct {
	marker     string // "#" or "--" or "/*"
	isolated   bool   // if it starts on its own line
	start, end Position
	text       []string
}

// Pos reports the position of the current token.
func (p *parser) Pos() Position { return Position{Line: p.cur.line, Offset: p.cur.offset} }

func newParser(filename, s string) *parser {
	return &parser{
		s: s,

		cur: token{line: 1},

		filename: filename,
		line:     1,
	}
}

// Rem returns the unparsed remainder, ignoring space.
func (p *parser) Rem() string {
	rem := p.s
	if p.backed {
		rem = p.cur.value + rem
	}
	i := 0
	for ; i < len(rem); i++ {
		if !isSpace(rem[i]) {
			break
		}
	}
	return rem[i:]
}

func (p *parser) String() string {
	if p.backed {
		return fmt.Sprintf("next tok: %s (rem: %q)", &p.cur, p.s)
	}
	return fmt.Sprintf("rem: %q", p.s)
}

func (p *parser) errorf(format string, args ...interface{}) *parseError {
	pe := &parseError{
		message:  fmt.Sprintf(format, args...),
		filename: p.filename,
		line:     p.cur.line,
		offset:   p.cur.offset,
	}
	p.cur.err = pe
	p.done = true
	return pe
}

func isInitialIdentifierChar(c byte) bool {
	// https://cloud.google.com/spanner/docs/lexical#identifiers
	switch {
	case 'A' <= c && c <= 'Z':
		return true
	case 'a' <= c && c <= 'z':
		return true
	case c == '_':
		return true
	}
	return false
}

func isIdentifierChar(c byte) bool {
	// https://cloud.google.com/spanner/docs/lexical#identifiers
	// This doesn't apply the restriction that an identifier cannot start with [0-9],
	// nor does it check against reserved keywords.
	switch {
	case 'A' <= c && c <= 'Z':
		return true
	case 'a' <= c && c <= 'z':
		return true
	case '0' <= c && c <= '9':
		return true
	case c == '_':
		return true
	}
	return false
}

func isHexDigit(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

func isOctalDigit(c byte) bool {
	return '0' <= c && c <= '7'
}

func (p *parser) consumeNumber() {
	/*
		int64_value:
			{ decimal_value | hex_value }

		decimal_value:
			[-]0—9+

		hex_value:
			[-]0[xX]{0—9|a—f|A—F}+

		(float64_value is not formally specified)

		float64_value :=
			  [+-]DIGITS.[DIGITS][e[+-]DIGITS]
			| [DIGITS].DIGITS[e[+-]DIGITS]
			| DIGITSe[+-]DIGITS
	*/

	i, neg, base := 0, false, 10
	float, e, dot := false, false, false
	if p.s[i] == '-' {
		neg = true
		i++
	} else if p.s[i] == '+' {
		// This isn't in the formal grammar, but is mentioned informally.
		// https://cloud.google.com/spanner/docs/lexical#integer-literals
		i++
	}
	if strings.HasPrefix(p.s[i:], "0x") || strings.HasPrefix(p.s[i:], "0X") {
		base = 16
		i += 2
	}
	d0 := i
digitLoop:
	for i < len(p.s) {
		switch c := p.s[i]; {
		case '0' <= c && c <= '9':
			i++
		case base == 16 && 'A' <= c && c <= 'F':
			i++
		case base == 16 && 'a' <= c && c <= 'f':
			i++
		case base == 10 && (c == 'e' || c == 'E'):
			if e {
				p.errorf("bad token %q", p.s[:i])
				return
			}
			// Switch to consuming float.
			float, e = true, true
			i++

			if i < len(p.s) && (p.s[i] == '+' || p.s[i] == '-') {
				i++
			}
		case base == 10 && c == '.':
			if dot || e { // any dot must come before E
				p.errorf("bad token %q", p.s[:i])
				return
			}
			// Switch to consuming float.
			float, dot = true, true
			i++
		default:
			break digitLoop
		}
	}
	if d0 == i {
		p.errorf("no digits in numeric literal")
		return
	}
	sign := ""
	if neg {
		sign = "-"
	}
	p.cur.value, p.s = p.s[:i], p.s[i:]
	p.offset += i
	var err error
	if float {
		p.cur.typ = float64Token
		p.cur.float64, err = strconv.ParseFloat(sign+p.cur.value[d0:], 64)
	} else {
		p.cur.typ = int64Token
		p.cur.value = sign + p.cur.value[d0:]
		p.cur.int64Base = base
		// This is parsed on demand.
	}
	if err != nil {
		p.errorf("bad numeric literal %q: %v", p.cur.value, err)
	}
}

func (p *parser) consumeString() {
	// https://cloud.google.com/spanner/docs/lexical#string-and-bytes-literals

	delim := p.stringDelimiter()
	if p.cur.err != nil {
		return
	}

	p.cur.string, p.cur.err = p.consumeStringContent(delim, false, true, "string literal")
	p.cur.typ = stringToken
}

func (p *parser) consumeRawString() {
	// https://cloud.google.com/spanner/docs/lexical#string-and-bytes-literals

	p.s = p.s[1:] // consume 'R'
	delim := p.stringDelimiter()
	if p.cur.err != nil {
		return
	}

	p.cur.string, p.cur.err = p.consumeStringContent(delim, true, true, "raw string literal")
	p.cur.typ = stringToken
}

func (p *parser) consumeBytes() {
	// https://cloud.google.com/spanner/docs/lexical#string-and-bytes-literals

	p.s = p.s[1:] // consume 'B'
	delim := p.stringDelimiter()
	if p.cur.err != nil {
		return
	}

	p.cur.string, p.cur.err = p.consumeStringContent(delim, false, false, "bytes literal")
	p.cur.typ = bytesToken
}

func (p *parser) consumeRawBytes() {
	// https://cloud.google.com/spanner/docs/lexical#string-and-bytes-literals

	p.s = p.s[2:] // consume 'RB'
	delim := p.stringDelimiter()
	if p.cur.err != nil {
		return
	}

	p.cur.string, p.cur.err = p.consumeStringContent(delim, true, false, "raw bytes literal")
	p.cur.typ = bytesToken
}

// stringDelimiter returns the opening string delimiter.
func (p *parser) stringDelimiter() string {
	c := p.s[0]
	if c != '"' && c != '\'' {
		p.errorf("invalid string literal")
		return ""
	}
	// Look for triple.
	if len(p.s) >= 3 && p.s[1] == c && p.s[2] == c {
		return p.s[:3]
	}
	return p.s[:1]
}

// consumeStringContent consumes a string-like literal, including its delimiters.
//
//   - delim is the opening/closing delimiter.
//   - raw is true if consuming a raw string.
//   - unicode is true if unicode escape sequence (\uXXXX or \UXXXXXXXX) are permitted.
//   - name identifies the name of the consuming token.
//
// It is designed for consuming string, bytes literals, and also backquoted identifiers.
func (p *parser) consumeStringContent(delim string, raw, unicode bool, name string) (string, *parseError) {
	// https://cloud.google.com/spanner/docs/lexical#string-and-bytes-literals

	if len(delim) == 3 {
		name = "triple-quoted " + name
	}

	i := len(delim)
	var content []byte

	for i < len(p.s) {
		if strings.HasPrefix(p.s[i:], delim) {
			i += len(delim)
			p.s = p.s[i:]
			p.offset += i
			return string(content), nil
		}

		if p.s[i] == '\\' {
			i++
			if i >= len(p.s) {
				return "", p.errorf("unclosed %s", name)
			}

			if raw {
				content = append(content, '\\', p.s[i])
				i++
				continue
			}

			switch p.s[i] {
			case 'a':
				i++
				content = append(content, '\a')
			case 'b':
				i++
				content = append(content, '\b')
			case 'f':
				i++
				content = append(content, '\f')
			case 'n':
				i++
				content = append(content, '\n')
			case 'r':
				i++
				content = append(content, '\r')
			case 't':
				i++
				content = append(content, '\t')
			case 'v':
				i++
				content = append(content, '\v')
			case '\\':
				i++
				content = append(content, '\\')
			case '?':
				i++
				content = append(content, '?')
			case '"':
				i++
				content = append(content, '"')
			case '\'':
				i++
				content = append(content, '\'')
			case '`':
				i++
				content = append(content, '`')
			case 'x', 'X':
				i++
				if !(i+1 < len(p.s) && isHexDigit(p.s[i]) && isHexDigit(p.s[i+1])) {
					return "", p.errorf("illegal escape sequence: hex escape sequence must be followed by 2 hex digits")
				}
				c, err := strconv.ParseUint(p.s[i:i+2], 16, 64)
				if err != nil {
					return "", p.errorf("illegal escape sequence: invalid hex digits: %q: %v", p.s[i:i+2], err)
				}
				content = append(content, byte(c))
				i += 2
			case 'u', 'U':
				t := p.s[i]
				if !unicode {
					return "", p.errorf("illegal escape sequence: \\%c", t)
				}

				i++
				size := 4
				if t == 'U' {
					size = 8
				}
				if i+size-1 >= len(p.s) {
					return "", p.errorf("illegal escape sequence: \\%c escape sequence must be followed by %d hex digits", t, size)
				}
				for j := 0; j < size; j++ {
					if !isHexDigit(p.s[i+j]) {
						return "", p.errorf("illegal escape sequence: \\%c escape sequence must be followed by %d hex digits", t, size)
					}
				}
				c, err := strconv.ParseUint(p.s[i:i+size], 16, 64)
				if err != nil {
					return "", p.errorf("illegal escape sequence: invalid \\%c digits: %q: %v", t, p.s[i:i+size], err)
				}
				if 0xD800 <= c && c <= 0xDFFF || 0x10FFFF < c {
					return "", p.errorf("illegal escape sequence: invalid codepoint: %x", c)
				}
				var buf [utf8.UTFMax]byte
				n := utf8.EncodeRune(buf[:], rune(c))
				content = append(content, buf[:n]...)
				i += size
			case '0', '1', '2', '3', '4', '5', '6', '7':
				if !(i+2 < len(p.s) && isOctalDigit(p.s[i+1]) && isOctalDigit(p.s[i+2])) {
					return "", p.errorf("illegal escape sequence: octal escape sequence must be followed by 3 octal digits")
				}
				c, err := strconv.ParseUint(p.s[i:i+3], 8, 64)
				if err != nil {
					return "", p.errorf("illegal escape sequence: invalid octal digits: %q: %v", p.s[i:i+3], err)
				}
				if c >= 256 {
					return "", p.errorf("illegal escape sequence: octal digits overflow: %q (%d)", p.s[i:i+3], c)
				}
				content = append(content, byte(c))
				i += 3
			default:
				return "", p.errorf("illegal escape sequence: \\%c", p.s[i])
			}

			continue
		}

		if p.s[i] == '\n' {
			if len(delim) != 3 { // newline is only allowed inside triple-quoted.
				return "", p.errorf("newline forbidden in %s", name)
			}
			p.line++
		}

		content = append(content, p.s[i])
		i++
	}

	return "", p.errorf("unclosed %s", name)
}

var operators = map[string]bool{
	// Arithmetic operators.
	"-":  true, // both unary and binary
	"~":  true,
	"*":  true,
	"/":  true,
	"||": true,
	"+":  true,
	"<<": true,
	">>": true,
	"&":  true,
	"^":  true,
	"|":  true,

	// Comparison operators.
	"<":  true,
	"<=": true,
	">":  true,
	">=": true,
	"=":  true,
	"!=": true,
	"<>": true,
}

func isSpace(c byte) bool {
	// Per https://cloud.google.com/spanner/docs/lexical, informally,
	// whitespace is defined as "space, backspace, tab, newline".
	switch c {
	case ' ', '\b', '\t', '\n':
		return true
	}
	return false
}

// skipSpace skips past any space or comments.
func (p *parser) skipSpace() bool {
	initLine := p.line
	// If we start capturing a comment in this method,
	// this is set to its comment value. Multi-line comments
	// are only joined during a single skipSpace invocation.
	var com *comment

	i := 0
	for i < len(p.s) {
		if isSpace(p.s[i]) {
			if p.s[i] == '\n' {
				p.line++
			}
			i++
			continue
		}
		// Comments.
		marker, term := "", ""
		if p.s[i] == '#' {
			marker, term = "#", "\n"
		} else if i+1 < len(p.s) && p.s[i] == '-' && p.s[i+1] == '-' {
			marker, term = "--", "\n"
		} else if i+1 < len(p.s) && p.s[i] == '/' && p.s[i+1] == '*' {
			marker, term = "/*", "*/"
		}
		if term == "" {
			break
		}
		// Search for the terminator, starting after the marker.
		ti := strings.Index(p.s[i+len(marker):], term)
		if ti < 0 {
			p.errorf("unterminated comment")
			return false
		}
		ti += len(marker) // make ti relative to p.s[i:]
		if com != nil && (com.end.Line+1 < p.line || com.marker != marker) {
			// There's a previous comment, but there's an
			// intervening blank line, or the marker changed.
			// Terminate the previous comment.
			com = nil
		}
		if com == nil {
			// New comment.
			p.comments = append(p.comments, comment{
				marker:   marker,
				isolated: (p.line != initLine) || p.line == 1,
				start: Position{
					Line:   p.line,
					Offset: p.offset + i,
				},
			})
			com = &p.comments[len(p.comments)-1]
		}
		textLines := strings.Split(p.s[i+len(marker):i+ti], "\n")
		com.text = append(com.text, textLines...)
		com.end = Position{
			Line:   p.line + len(textLines) - 1,
			Offset: p.offset + i + ti,
		}
		p.line = com.end.Line
		if term == "\n" {
			p.line++
		}
		i += ti + len(term)

		// A non-isolated comment is always complete and doesn't get
		// combined with any future comment.
		if !com.isolated {
			com = nil
		}
	}
	p.s = p.s[i:]
	p.offset += i
	if p.s == "" {
		p.done = true
	}
	return i > 0
}

// advance moves the parser to the next token, which will be available in p.cur.
func (p *parser) advance() {
	prevID := p.cur.typ == quotedID || p.cur.typ == unquotedID

	p.skipSpace()
	if p.done {
		return
	}

	// If the previous token was an identifier (quoted or unquoted),
	// the next token being a dot means this is a path expression (not a number).
	if prevID && p.s[0] == '.' {
		p.cur.err = nil
		p.cur.line, p.cur.offset = p.line, p.offset
		p.cur.typ = unknownToken
		p.cur.value, p.s = p.s[:1], p.s[1:]
		p.offset++
		return
	}

	p.cur.err = nil
	p.cur.line, p.cur.offset = p.line, p.offset
	p.cur.typ = unknownToken
	// TODO: struct, date, timestamp literals
	switch p.s[0] {
	case ',', ';', '(', ')', '{', '}', '[', ']', '*', '+', '-':
		// Single character symbol.
		p.cur.value, p.s = p.s[:1], p.s[1:]
		p.offset++
		return
	// String literal prefix.
	case 'B', 'b', 'R', 'r', '"', '\'':
		// "B", "b", "BR", "Rb" etc are valid string literal prefix, however "BB", "rR" etc are not.
		raw, bytes := false, false
		for i := 0; i < 4 && i < len(p.s); i++ {
			switch {
			case !raw && (p.s[i] == 'R' || p.s[i] == 'r'):
				raw = true
				continue
			case !bytes && (p.s[i] == 'B' || p.s[i] == 'b'):
				bytes = true
				continue
			case p.s[i] == '"' || p.s[i] == '\'':
				switch {
				case raw && bytes:
					p.consumeRawBytes()
				case raw:
					p.consumeRawString()
				case bytes:
					p.consumeBytes()
				default:
					p.consumeString()
				}
				return
			}
			break
		}
	case '`':
		// Quoted identifier.
		p.cur.string, p.cur.err = p.consumeStringContent("`", false, true, "quoted identifier")
		p.cur.typ = quotedID
		return
	}
	if p.s[0] == '@' || isInitialIdentifierChar(p.s[0]) {
		// Start consuming identifier.
		i := 1
		for i < len(p.s) && isIdentifierChar(p.s[i]) {
			i++
		}
		p.cur.value, p.s = p.s[:i], p.s[i:]
		p.cur.typ = unquotedID
		p.offset += i
		return
	}
	if len(p.s) >= 2 && p.s[0] == '.' && ('0' <= p.s[1] && p.s[1] <= '9') {
		// dot followed by a digit.
		p.consumeNumber()
		return
	}
	if '0' <= p.s[0] && p.s[0] <= '9' {
		p.consumeNumber()
		return
	}

	// Look for operator (two or one bytes).
	for i := 2; i >= 1; i-- {
		if i <= len(p.s) && operators[p.s[:i]] {
			p.cur.value, p.s = p.s[:i], p.s[i:]
			p.offset += i
			return
		}
	}

	p.errorf("unexpected byte %#x", p.s[0])
}

// back steps the parser back one token. It cannot be called twice in succession.
func (p *parser) back() {
	if p.backed {
		panic("parser backed up twice")
	}
	p.done = false
	p.backed = true
	// If an error was being recovered, we wish to ignore the error.
	// Don't do that for eof since that'll be returned next.
	if p.cur.err != eof {
		p.cur.err = nil
	}
}

// next returns the next token.
func (p *parser) next() *token {
	if p.backed || p.done {
		p.backed = false
		return &p.cur
	}
	p.advance()
	if p.done && p.cur.err == nil {
		p.cur.value = ""
		p.cur.err = eof
	}
	debugf("parser·next(): returning [%v] [err: %v] @l%d,o%d", p.cur.value, p.cur.err, p.cur.line, p.cur.offset)
	return &p.cur
}

// sniff reports whether the next N tokens are as specified.
func (p *parser) sniff(want ...string) bool {
	// Store current parser state and restore on the way out.
	orig := *p
	defer func() { *p = orig }()

	for _, w := range want {
		tok := p.next()
		if tok.err != nil || tok.value != w {
			return false
		}
	}
	return true
}

// eat reports whether the next N tokens are as specified,
// then consumes them.
func (p *parser) eat(want ...string) bool {
	// Store current parser state so we can restore if we get a failure.
	orig := *p

	for _, w := range want {
		tok := p.next()
		if tok.err != nil || tok.value != w {
			// Mismatch.
			*p = orig
			return false
		}
	}
	return true
}

func (p *parser) expect(want string) *parseError {
	tok := p.next()
	if tok.err != nil {
		return tok.err
	}
	if tok.value != want {
		return p.errorf("got %q while expecting %q", tok.value, want)
	}
	return nil
}

func (p *parser) parseDDLStmt() (DDLStmt, *parseError) {
	debugf("parseDDLStmt: %v", p)

	/*
		statement:
			{ create_database | create_table | create_index | alter_table | drop_table | drop_index }
	*/

	// TODO: support create_database

	if p.sniff("CREATE", "TABLE") {
		ct, err := p.parseCreateTable()
		return ct, err
	} else if p.sniff("CREATE") {
		// The only other statement starting with CREATE is CREATE INDEX,
		// which can have UNIQUE or NULL_FILTERED as the token after CREATE.
		ci, err := p.parseCreateIndex()
		return ci, err
	} else if p.sniff("ALTER", "TABLE") {
		a, err := p.parseAlterTable()
		return a, err
	} else if p.eat("DROP") {
		pos := p.Pos()
		// These statements are simple.
		//	DROP TABLE table_name
		//	DROP INDEX index_name
		tok := p.next()
		if tok.err != nil {
			return nil, tok.err
		}
		kind := tok.value
		if kind != "TABLE" && kind != "INDEX" {
			return nil, p.errorf("got %q, want TABLE or INDEX", kind)
		}
		name, err := p.parseTableOrIndexOrColumnName()
		if err != nil {
			return nil, err
		}
		if kind == "TABLE" {
			return &DropTable{Name: name, Position: pos}, nil
		}
		return &DropIndex{Name: name, Position: pos}, nil
	}

	return nil, p.errorf("unknown DDL statement")
}

func (p *parser) parseCreateTable() (*CreateTable, *parseError) {
	debugf("parseCreateTable: %v", p)

	/*
		CREATE TABLE table_name(
			[column_def, ...] [ table_constraint, ...] )
			primary_key [, cluster]

		primary_key:
			PRIMARY KEY ( [key_part, ...] )

		cluster:
			INTERLEAVE IN PARENT table_name [ ON DELETE { CASCADE | NO ACTION } ]
	*/

	if err := p.expect("CREATE"); err != nil {
		return nil, err
	}
	pos := p.Pos()
	if err := p.expect("TABLE"); err != nil {
		return nil, err
	}
	tname, err := p.parseTableOrIndexOrColumnName()
	if err != nil {
		return nil, err
	}

	ct := &CreateTable{Name: tname, Position: pos}
	err = p.parseCommaList("(", ")", func(p *parser) *parseError {
		if p.sniffTableConstraint() {
			tc, err := p.parseTableConstraint()
			if err != nil {
				return err
			}
			ct.Constraints = append(ct.Constraints, tc)
			return nil
		}

		cd, err := p.parseColumnDef()
		if err != nil {
			return err
		}
		ct.Columns = append(ct.Columns, cd)
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := p.expect("PRIMARY"); err != nil {
		return nil, err
	}
	if err := p.expect("KEY"); err != nil {
		return nil, err
	}
	ct.PrimaryKey, err = p.parseKeyPartList()
	if err != nil {
		return nil, err
	}

	if p.eat(",", "INTERLEAVE") {
		if err := p.expect("IN"); err != nil {
			return nil, err
		}
		if err := p.expect("PARENT"); err != nil {
			return nil, err
		}
		pname, err := p.parseTableOrIndexOrColumnName()
		if err != nil {
			return nil, err
		}
		ct.Interleave = &Interleave{
			Parent:   pname,
			OnDelete: NoActionOnDelete,
		}
		// The ON DELETE clause is optional; it defaults to NoActionOnDelete.
		if p.eat("ON", "DELETE") {
			od, err := p.parseOnDelete()
			if err != nil {
				return nil, err
			}
			ct.Interleave.OnDelete = od
		}
	}

	return ct, nil
}

func (p *parser) sniffTableConstraint() bool {
	// Unfortunately the Cloud Spanner grammar is LL(3) because
	//	CONSTRAINT BOOL
	// could be the start of a declaration of a column called "CONSTRAINT" of boolean type,
	// or it could be the start of a foreign key constraint called "BOOL".
	// We have to sniff up to the third token to see what production it is.
	// If we have "FOREIGN" and "KEY" (or "CHECK"), this is an unnamed table constraint.
	// If we have "CONSTRAINT", an identifier and "FOREIGN" (or "CHECK"), this is a table constraint.
	// Otherwise, this is a column definition.

	if p.sniff("FOREIGN", "KEY") || p.sniff("CHECK") {
		return true
	}

	// Store parser state, and peek ahead.
	// Restore on the way out.
	orig := *p
	defer func() { *p = orig }()

	if !p.eat("CONSTRAINT") {
		return false
	}
	if _, err := p.parseTableOrIndexOrColumnName(); err != nil {
		return false
	}
	return p.sniff("FOREIGN") || p.sniff("CHECK")
}

func (p *parser) parseCreateIndex() (*CreateIndex, *parseError) {
	debugf("parseCreateIndex: %v", p)

	/*
		CREATE [UNIQUE] [NULL_FILTERED] INDEX index_name
			ON table_name ( key_part [, ...] ) [ storing_clause ] [ , interleave_clause ]

		index_name:
			{a—z|A—Z}[{a—z|A—Z|0—9|_}+]

		storing_clause:
			STORING ( column_name [, ...] )

		interleave_clause:
			INTERLEAVE IN table_name
	*/

	var unique, nullFiltered bool

	if err := p.expect("CREATE"); err != nil {
		return nil, err
	}
	pos := p.Pos()
	if p.eat("UNIQUE") {
		unique = true
	}
	if p.eat("NULL_FILTERED") {
		nullFiltered = true
	}
	if err := p.expect("INDEX"); err != nil {
		return nil, err
	}
	iname, err := p.parseTableOrIndexOrColumnName()
	if err != nil {
		return nil, err
	}
	if err := p.expect("ON"); err != nil {
		return nil, err
	}
	tname, err := p.parseTableOrIndexOrColumnName()
	if err != nil {
		return nil, err
	}
	ci := &CreateIndex{
		Name:  iname,
		Table: tname,

		Unique:       unique,
		NullFiltered: nullFiltered,

		Position: pos,
	}
	ci.Columns, err = p.parseKeyPartList()
	if err != nil {
		return nil, err
	}

	if p.eat("STORING") {
		ci.Storing, err = p.parseColumnNameList()
		if err != nil {
			return nil, err
		}
	}

	if p.eat(",", "INTERLEAVE", "IN") {
		ci.Interleave, err = p.parseTableOrIndexOrColumnName()
		if err != nil {
			return nil, err
		}
	}

	return ci, nil
}

func (p *parser) parseAlterTable() (*AlterTable, *parseError) {
	debugf("parseAlterTable: %v", p)

	/*
		alter_table:
			ALTER TABLE table_name { table_alteration | table_column_alteration }

		table_alteration:
			{ ADD [ COLUMN ] column_def
			| DROP [ COLUMN ] column_name
			| ADD table_constraint
			| DROP CONSTRAINT constraint_name
			| SET ON DELETE { CASCADE | NO ACTION } }

		table_column_alteration:
			ALTER [ COLUMN ] column_name { { scalar_type | array_type } [NOT NULL] | SET options_def }
	*/

	if err := p.expect("ALTER"); err != nil {
		return nil, err
	}
	pos := p.Pos()
	if err := p.expect("TABLE"); err != nil {
		return nil, err
	}
	tname, err := p.parseTableOrIndexOrColumnName()
	if err != nil {
		return nil, err
	}
	a := &AlterTable{Name: tname, Position: pos}

	tok := p.next()
	if tok.err != nil {
		return nil, tok.err
	}
	switch tok.value {
	default:
		return nil, p.errorf("got %q, expected ADD or DROP or SET or ALTER", tok.value)
	case "ADD":
		if p.sniff("CONSTRAINT") || p.sniff("FOREIGN") || p.sniff("CHECK") {
			tc, err := p.parseTableConstraint()
			if err != nil {
				return nil, err
			}
			a.Alteration = AddConstraint{Constraint: tc}
			return a, nil
		}

		// TODO: "COLUMN" is optional.
		if err := p.expect("COLUMN"); err != nil {
			return nil, err
		}
		cd, err := p.parseColumnDef()
		if err != nil {
			return nil, err
		}
		a.Alteration = AddColumn{Def: cd}
		return a, nil
	case "DROP":
		if p.eat("CONSTRAINT") {
			name, err := p.parseTableOrIndexOrColumnName()
			if err != nil {
				return nil, err
			}
			a.Alteration = DropConstraint{Name: name}
			return a, nil
		}

		// TODO: "COLUMN" is optional.
		if err := p.expect("COLUMN"); err != nil {
			return nil, err
		}
		name, err := p.parseTableOrIndexOrColumnName()
		if err != nil {
			return nil, err
		}
		a.Alteration = DropColumn{Name: name}
		return a, nil
	case "SET":
		if err := p.expect("ON"); err != nil {
			return nil, err
		}
		if err := p.expect("DELETE"); err != nil {
			return nil, err
		}
		od, err := p.parseOnDelete()
		if err != nil {
			return nil, err
		}
		a.Alteration = SetOnDelete{Action: od}
		return a, nil
	case "ALTER":
		// TODO: "COLUMN" is optional.
		if err := p.expect("COLUMN"); err != nil {
			return nil, err
		}
		name, err := p.parseTableOrIndexOrColumnName()
		if err != nil {
			return nil, err
		}
		ca, err := p.parseColumnAlteration()
		if err != nil {
			return nil, err
		}
		a.Alteration = AlterColumn{
			Name:       name,
			Alteration: ca,
		}
		return a, nil
	}
}

func (p *parser) parseDMLStmt() (DMLStmt, *parseError) {
	debugf("parseDMLStmt: %v", p)

	/*
		DELETE [FROM] target_name [[AS] alias]
		WHERE condition

		UPDATE target_name [[AS] alias]
		SET update_item [, ...]
		WHERE condition

		update_item: path_expression = expression | path_expression = DEFAULT

		TODO: Insert.
	*/

	if p.eat("DELETE") {
		p.eat("FROM") // optional
		tname, err := p.parseTableOrIndexOrColumnName()
		if err != nil {
			return nil, err
		}
		// TODO: parse alias.
		if err := p.expect("WHERE"); err != nil {
			return nil, err
		}
		where, err := p.parseBoolExpr()
		if err != nil {
			return nil, err
		}
		return &Delete{
			Table: tname,
			Where: where,
		}, nil
	}

	if p.eat("UPDATE") {
		tname, err := p.parseTableOrIndexOrColumnName()
		if err != nil {
			return nil, err
		}
		u := &Update{
			Table: tname,
		}
		// TODO: parse alias.
		if err := p.expect("SET"); err != nil {
			return nil, err
		}
		for {
			ui, err := p.parseUpdateItem()
			if err != nil {
				return nil, err
			}
			u.Items = append(u.Items, ui)
			if p.eat(",") {
				continue
			}
			break
		}
		if err := p.expect("WHERE"); err != nil {
			return nil, err
		}
		where, err := p.parseBoolExpr()
		if err != nil {
			return nil, err
		}
		u.Where = where
		return u, nil
	}

	return nil, p.errorf("unknown DML statement")
}

func (p *parser) parseUpdateItem() (UpdateItem, *parseError) {
	col, err := p.parseTableOrIndexOrColumnName()
	if err != nil {
		return UpdateItem{}, err
	}
	ui := UpdateItem{
		Column: col,
	}
	if err := p.expect("="); err != nil {
		return UpdateItem{}, err
	}
	if p.eat("DEFAULT") {
		return ui, nil
	}
	ui.Value, err = p.parseExpr()
	if err != nil {
		return UpdateItem{}, err
	}
	return ui, nil
}

func (p *parser) parseColumnDef() (ColumnDef, *parseError) {
	debugf("parseColumnDef: %v", p)

	/*
		column_def:
			column_name {scalar_type | array_type} [NOT NULL] [AS ( expression ) STORED] [options_def]
	*/

	name, err := p.parseTableOrIndexOrColumnName()
	if err != nil {
		return ColumnDef{}, err
	}

	cd := ColumnDef{Name: name, Position: p.Pos()}

	cd.Type, err = p.parseType()
	if err != nil {
		return ColumnDef{}, err
	}

	if p.eat("NOT", "NULL") {
		cd.NotNull = true
	}

	if p.eat("AS", "(") {
		cd.Generated, err = p.parseExpr()
		if err != nil {
			return ColumnDef{}, err
		}
		if err := p.expect(")"); err != nil {
			return ColumnDef{}, err
		}
		if err := p.expect("STORED"); err != nil {
			return ColumnDef{}, err
		}
	}

	if p.sniff("OPTIONS") {
		cd.Options, err = p.parseColumnOptions()
		if err != nil {
			return ColumnDef{}, err
		}
	}

	return cd, nil
}

func (p *parser) parseColumnAlteration() (ColumnAlteration, *parseError) {
	debugf("parseColumnAlteration: %v", p)
	/*
		{ data_type } [ NOT NULL ] | SET [ options_def ]
	*/

	if p.eat("SET") {
		co, err := p.parseColumnOptions()
		if err != nil {
			return nil, err
		}
		return SetColumnOptions{Options: co}, nil
	}

	typ, err := p.parseType()
	if err != nil {
		return nil, err
	}
	sct := SetColumnType{Type: typ}

	if p.eat("NOT", "NULL") {
		sct.NotNull = true
	}

	return sct, nil
}

func (p *parser) parseColumnOptions() (ColumnOptions, *parseError) {
	debugf("parseColumnOptions: %v", p)
	/*
		options_def:
			OPTIONS (allow_commit_timestamp = { true | null })
	*/

	if err := p.expect("OPTIONS"); err != nil {
		return ColumnOptions{}, err
	}
	if err := p.expect("("); err != nil {
		return ColumnOptions{}, err
	}

	var co ColumnOptions
	if p.eat("allow_commit_timestamp", "=") {
		tok := p.next()
		if tok.err != nil {
			return ColumnOptions{}, tok.err
		}
		allowCommitTimestamp := new(bool)
		switch tok.value {
		case "true":
			*allowCommitTimestamp = true
		case "null":
			*allowCommitTimestamp = false
		default:
			return ColumnOptions{}, p.errorf("got %q, want true or null", tok.value)
		}
		co.AllowCommitTimestamp = allowCommitTimestamp
	}

	if err := p.expect(")"); err != nil {
		return ColumnOptions{}, err
	}

	return co, nil
}

func (p *parser) parseKeyPartList() ([]KeyPart, *parseError) {
	var list []KeyPart
	err := p.parseCommaList("(", ")", func(p *parser) *parseError {
		kp, err := p.parseKeyPart()
		if err != nil {
			return err
		}
		list = append(list, kp)
		return nil
	})
	return list, err
}

func (p *parser) parseKeyPart() (KeyPart, *parseError) {
	debugf("parseKeyPart: %v", p)

	/*
		key_part:
			column_name [{ ASC | DESC }]
	*/

	name, err := p.parseTableOrIndexOrColumnName()
	if err != nil {
		return KeyPart{}, err
	}

	kp := KeyPart{Column: name}

	tok := p.next()
	if tok.err != nil {
		// End of the key_part.
		p.back()
		return kp, nil
	}
	switch tok.value {
	case "ASC":
	case "DESC":
		kp.Desc = true
	default:
		p.back()
	}

	return kp, nil
}

func (p *parser) parseTableConstraint() (TableConstraint, *parseError) {
	debugf("parseTableConstraint: %v", p)

	/*
		table_constraint:
			[ CONSTRAINT constraint_name ]
			{ check | foreign_key }
	*/

	if p.eat("CONSTRAINT") {
		pos := p.Pos()
		// Named constraint.
		cname, err := p.parseTableOrIndexOrColumnName()
		if err != nil {
			return TableConstraint{}, err
		}
		c, err := p.parseConstraint()
		if err != nil {
			return TableConstraint{}, err
		}
		return TableConstraint{
			Name:       cname,
			Constraint: c,
			Position:   pos,
		}, nil
	}

	// Unnamed constraint.
	c, err := p.parseConstraint()
	if err != nil {
		return TableConstraint{}, err
	}
	return TableConstraint{
		Constraint: c,
		Position:   c.Pos(),
	}, nil
}

func (p *parser) parseConstraint() (Constraint, *parseError) {
	if p.sniff("FOREIGN") {
		fk, err := p.parseForeignKey()
		return fk, err
	}
	c, err := p.parseCheck()
	return c, err
}

func (p *parser) parseForeignKey() (ForeignKey, *parseError) {
	debugf("parseForeignKey: %v", p)

	/*
		foreign_key:
			FOREIGN KEY ( column_name [, ... ] ) REFERENCES ref_table ( ref_column [, ... ] )
	*/

	if err := p.expect("FOREIGN"); err != nil {
		return ForeignKey{}, err
	}
	fk := ForeignKey{Position: p.Pos()}
	if err := p.expect("KEY"); err != nil {
		return ForeignKey{}, err
	}
	var err *parseError
	fk.Columns, err = p.parseColumnNameList()
	if err != nil {
		return ForeignKey{}, err
	}
	if err := p.expect("REFERENCES"); err != nil {
		return ForeignKey{}, err
	}
	fk.RefTable, err = p.parseTableOrIndexOrColumnName()
	if err != nil {
		return ForeignKey{}, err
	}
	fk.RefColumns, err = p.parseColumnNameList()
	if err != nil {
		return ForeignKey{}, err
	}
	return fk, nil
}

func (p *parser) parseCheck() (Check, *parseError) {
	debugf("parseCheck: %v", p)

	/*
		check:
			CHECK ( expression )
	*/

	if err := p.expect("CHECK"); err != nil {
		return Check{}, err
	}
	c := Check{Position: p.Pos()}
	if err := p.expect("("); err != nil {
		return Check{}, err
	}
	var err *parseError
	c.Expr, err = p.parseBoolExpr()
	if err != nil {
		return Check{}, err
	}
	if err := p.expect(")"); err != nil {
		return Check{}, err
	}
	return c, nil
}

func (p *parser) parseColumnNameList() ([]ID, *parseError) {
	var list []ID
	err := p.parseCommaList("(", ")", func(p *parser) *parseError {
		n, err := p.parseTableOrIndexOrColumnName()
		if err != nil {
			return err
		}
		list = append(list, n)
		return nil
	})
	return list, err
}

var baseTypes = map[string]TypeBase{
	"BOOL":      Bool,
	"INT64":     Int64,
	"FLOAT64":   Float64,
	"NUMERIC":   Numeric,
	"STRING":    String,
	"BYTES":     Bytes,
	"DATE":      Date,
	"TIMESTAMP": Timestamp,
}

func (p *parser) parseType() (Type, *parseError) {
	debugf("parseType: %v", p)

	/*
		array_type:
			ARRAY< scalar_type >

		scalar_type:
			{ BOOL | INT64 | FLOAT64 | NUMERIC | STRING( length ) | BYTES( length ) | DATE | TIMESTAMP }
		length:
			{ int64_value | MAX }
	*/

	var t Type

	tok := p.next()
	if tok.err != nil {
		return Type{}, tok.err
	}
	if tok.value == "ARRAY" {
		t.Array = true
		if err := p.expect("<"); err != nil {
			return Type{}, err
		}
		tok = p.next()
		if tok.err != nil {
			return Type{}, tok.err
		}
	}
	base, ok := baseTypes[tok.value]
	if !ok {
		return Type{}, p.errorf("got %q, want scalar type", tok.value)
	}
	t.Base = base

	if t.Base == String || t.Base == Bytes {
		if err := p.expect("("); err != nil {
			return Type{}, err
		}

		tok = p.next()
		if tok.err != nil {
			return Type{}, tok.err
		}
		if tok.value == "MAX" {
			t.Len = MaxLen
		} else if tok.typ == int64Token {
			n, err := strconv.ParseInt(tok.value, tok.int64Base, 64)
			if err != nil {
				return Type{}, p.errorf("%v", err)
			}
			t.Len = n
		} else {
			return Type{}, p.errorf("got %q, want MAX or int64", tok.value)
		}

		if err := p.expect(")"); err != nil {
			return Type{}, err
		}
	}

	if t.Array {
		if err := p.expect(">"); err != nil {
			return Type{}, err
		}
	}

	return t, nil
}

func (p *parser) parseQuery() (Query, *parseError) {
	debugf("parseQuery: %v", p)

	/*
		query_statement:
			[ table_hint_expr ][ join_hint_expr ]
			query_expr

		query_expr:
			{ select | ( query_expr ) | query_expr set_op query_expr }
			[ ORDER BY expression [{ ASC | DESC }] [, ...] ]
			[ LIMIT count [ OFFSET skip_rows ] ]
	*/

	// TODO: hints, sub-selects, etc.

	// TODO: use a case-insensitive select.
	if err := p.expect("SELECT"); err != nil {
		return Query{}, err
	}
	p.back()
	sel, err := p.parseSelect()
	if err != nil {
		return Query{}, err
	}
	q := Query{Select: sel}

	if p.eat("ORDER", "BY") {
		for {
			o, err := p.parseOrder()
			if err != nil {
				return Query{}, err
			}
			q.Order = append(q.Order, o)

			if !p.eat(",") {
				break
			}
		}
	}

	if p.eat("LIMIT") {
		// "only literal or parameter values"
		// https://cloud.google.com/spanner/docs/query-syntax#limit-clause-and-offset-clause

		lim, err := p.parseLiteralOrParam()
		if err != nil {
			return Query{}, err
		}
		q.Limit = lim

		if p.eat("OFFSET") {
			off, err := p.parseLiteralOrParam()
			if err != nil {
				return Query{}, err
			}
			q.Offset = off
		}
	}

	return q, nil
}

func (p *parser) parseSelect() (Select, *parseError) {
	debugf("parseSelect: %v", p)

	/*
		select:
			SELECT  [{ ALL | DISTINCT }]
				{ [ expression. ]* | expression [ [ AS ] alias ] } [, ...]
			[ FROM from_item [ tablesample_type ] [, ...] ]
			[ WHERE bool_expression ]
			[ GROUP BY expression [, ...] ]
			[ HAVING bool_expression ]
	*/
	if err := p.expect("SELECT"); err != nil {
		return Select{}, err
	}

	var sel Select

	if p.eat("ALL") {
		// Nothing to do; this is the default.
	} else if p.eat("DISTINCT") {
		sel.Distinct = true
	}

	// Read expressions for the SELECT list.
	list, aliases, err := p.parseSelectList()
	if err != nil {
		return Select{}, err
	}
	sel.List, sel.ListAliases = list, aliases

	if p.eat("FROM") {
		padTS := func() {
			for len(sel.TableSamples) < len(sel.From) {
				sel.TableSamples = append(sel.TableSamples, nil)
			}
		}

		for {
			from, err := p.parseSelectFrom()
			if err != nil {
				return Select{}, err
			}
			sel.From = append(sel.From, from)

			if p.sniff("TABLESAMPLE") {
				ts, err := p.parseTableSample()
				if err != nil {
					return Select{}, err
				}
				padTS()
				sel.TableSamples[len(sel.TableSamples)-1] = &ts
			}

			if p.eat(",") {
				continue
			}
			break
		}

		if sel.TableSamples != nil {
			padTS()
		}
	}

	if p.eat("WHERE") {
		where, err := p.parseBoolExpr()
		if err != nil {
			return Select{}, err
		}
		sel.Where = where
	}

	if p.eat("GROUP", "BY") {
		list, err := p.parseExprList()
		if err != nil {
			return Select{}, err
		}
		sel.GroupBy = list
	}

	// TODO: HAVING

	return sel, nil
}

func (p *parser) parseSelectList() ([]Expr, []ID, *parseError) {
	var list []Expr
	var aliases []ID // Only set if any aliases are seen.
	padAliases := func() {
		for len(aliases) < len(list) {
			aliases = append(aliases, "")
		}
	}

	for {
		expr, err := p.parseExpr()
		if err != nil {
			return nil, nil, err
		}
		list = append(list, expr)

		// TODO: The "AS" keyword is optional.
		if p.eat("AS") {
			alias, err := p.parseAlias()
			if err != nil {
				return nil, nil, err
			}

			padAliases()
			aliases[len(aliases)-1] = alias
		}

		if p.eat(",") {
			continue
		}
		break
	}
	if aliases != nil {
		padAliases()
	}
	return list, aliases, nil
}

func (p *parser) parseSelectFrom() (SelectFrom, *parseError) {
	debugf("parseSelectFrom: %v", p)

	/*
		from_item: {
			table_name [ table_hint_expr ] [ [ AS ] alias ] |
			join |
			( query_expr ) [ table_hint_expr ] [ [ AS ] alias ] |
			field_path |
			{ UNNEST( array_expression ) | UNNEST( array_path ) | array_path }
				[ table_hint_expr ] [ [ AS ] alias ] [ WITH OFFSET [ [ AS ] alias ] ] |
			with_query_name [ table_hint_expr ] [ [ AS ] alias ]
		}

		join:
			from_item [ join_type ] [ join_method ] JOIN  [ join_hint_expr ] from_item
				[ ON bool_expression | USING ( join_column [, ...] ) ]

		join_type:
			{ INNER | CROSS | FULL [OUTER] | LEFT [OUTER] | RIGHT [OUTER] }
	*/

	if p.eat("UNNEST") {
		if err := p.expect("("); err != nil {
			return nil, err
		}
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if err := p.expect(")"); err != nil {
			return nil, err
		}
		sfu := SelectFromUnnest{Expr: e}
		if p.eat("AS") { // TODO: The "AS" keyword is optional.
			alias, err := p.parseAlias()
			if err != nil {
				return nil, err
			}
			sfu.Alias = alias
		}
		// TODO: hint, offset
		return sfu, nil
	}

	// A join starts with a from_item, so that can't be detected in advance.
	// TODO: Support subquery, field_path, array_path, WITH.
	// TODO: Verify associativity of multile joins.

	tname, err := p.parseTableOrIndexOrColumnName()
	if err != nil {
		return nil, err
	}
	sf := SelectFromTable{Table: tname}

	// TODO: The "AS" keyword is optional.
	if p.eat("AS") {
		alias, err := p.parseAlias()
		if err != nil {
			return nil, err
		}
		sf.Alias = alias
	}

	// Look ahead to see if this is a join.
	tok := p.next()
	if tok.err != nil {
		p.back()
		return sf, nil
	}
	var hashJoin bool // Special case for "HASH JOIN" syntax.
	if tok.value == "HASH" {
		hashJoin = true
		tok = p.next()
		if tok.err != nil {
			return nil, err
		}
	}
	var jt JoinType
	if tok.value == "JOIN" {
		// This is implicitly an inner join.
		jt = InnerJoin
	} else if j, ok := joinKeywords[tok.value]; ok {
		jt = j
		switch jt {
		case FullJoin, LeftJoin, RightJoin:
			// These join types are implicitly "outer" joins,
			// so the "OUTER" keyword is optional.
			p.eat("OUTER")
		}
		if err := p.expect("JOIN"); err != nil {
			return nil, err
		}
	} else {
		p.back()
		return sf, nil
	}

	sfj := SelectFromJoin{
		Type: jt,
		LHS:  sf,
	}
	setHint := func(k, v string) {
		if sfj.Hints == nil {
			sfj.Hints = make(map[string]string)
		}
		sfj.Hints[k] = v
	}
	if hashJoin {
		setHint("JOIN_METHOD", "HASH_JOIN")
	}

	if p.eat("@") {
		if err := p.expect("{"); err != nil {
			return nil, err
		}
		for {
			if p.sniff("}") {
				break
			}
			tok := p.next()
			if tok.err != nil {
				return nil, tok.err
			}
			k := tok.value
			if err := p.expect("="); err != nil {
				return nil, err
			}
			tok = p.next()
			if tok.err != nil {
				return nil, tok.err
			}
			v := tok.value
			setHint(k, v)
			if !p.eat(",") {
				break
			}
		}
		if err := p.expect("}"); err != nil {
			return nil, err
		}
	}

	sfj.RHS, err = p.parseSelectFrom()
	if err != nil {
		return nil, err
	}

	if p.eat("ON") {
		sfj.On, err = p.parseBoolExpr()
		if err != nil {
			return nil, err
		}
	}
	if p.eat("USING") {
		if sfj.On != nil {
			return nil, p.errorf("join may not have both ON and USING clauses")
		}
		sfj.Using, err = p.parseColumnNameList()
		if err != nil {
			return nil, err
		}
	}

	return sfj, nil
}

var joinKeywords = map[string]JoinType{
	"INNER": InnerJoin,
	"CROSS": CrossJoin,
	"FULL":  FullJoin,
	"LEFT":  LeftJoin,
	"RIGHT": RightJoin,
}

func (p *parser) parseTableSample() (TableSample, *parseError) {
	var ts TableSample

	if err := p.expect("TABLESAMPLE"); err != nil {
		return ts, err
	}

	tok := p.next()
	switch {
	case tok.err != nil:
		return ts, tok.err
	case tok.value == "BERNOULLI":
		ts.Method = Bernoulli
	case tok.value == "RESERVOIR":
		ts.Method = Reservoir
	default:
		return ts, p.errorf("got %q, want BERNOULLI or RESERVOIR", tok.value)
	}

	if err := p.expect("("); err != nil {
		return ts, err
	}

	// The docs say "numeric_value_expression" here,
	// but that doesn't appear to be defined anywhere.
	size, err := p.parseExpr()
	if err != nil {
		return ts, err
	}
	ts.Size = size

	tok = p.next()
	switch {
	case tok.err != nil:
		return ts, tok.err
	case tok.value == "PERCENT":
		ts.SizeType = PercentTableSample
	case tok.value == "ROWS":
		ts.SizeType = RowsTableSample
	default:
		return ts, p.errorf("got %q, want PERCENT or ROWS", tok.value)
	}

	if err := p.expect(")"); err != nil {
		return ts, err
	}

	return ts, nil
}

func (p *parser) parseOrder() (Order, *parseError) {
	/*
		expression [{ ASC | DESC }]
	*/

	expr, err := p.parseExpr()
	if err != nil {
		return Order{}, err
	}
	o := Order{Expr: expr}

	tok := p.next()
	switch {
	case tok.err == nil && tok.value == "ASC":
	case tok.err == nil && tok.value == "DESC":
		o.Desc = true
	default:
		p.back()
	}

	return o, nil
}

func (p *parser) parseLiteralOrParam() (LiteralOrParam, *parseError) {
	tok := p.next()
	if tok.err != nil {
		return nil, tok.err
	}
	if tok.typ == int64Token {
		n, err := strconv.ParseInt(tok.value, tok.int64Base, 64)
		if err != nil {
			return nil, p.errorf("%v", err)
		}
		return IntegerLiteral(n), nil
	}
	// TODO: check character sets.
	if strings.HasPrefix(tok.value, "@") {
		return Param(tok.value[1:]), nil
	}
	return nil, p.errorf("got %q, want literal or parameter", tok.value)
}

func (p *parser) parseExprList() ([]Expr, *parseError) {
	var list []Expr
	for {
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		list = append(list, expr)

		if p.eat(",") {
			continue
		}
		break
	}
	return list, nil
}

func (p *parser) parseParenExprList() ([]Expr, *parseError) {
	var list []Expr
	err := p.parseCommaList("(", ")", func(p *parser) *parseError {
		e, err := p.parseExpr()
		if err != nil {
			return err
		}
		list = append(list, e)
		return nil
	})
	return list, err
}

/*
Expressions

Cloud Spanner expressions are not formally specified.
The set of operators and their precedence is listed in
https://cloud.google.com/spanner/docs/functions-and-operators#operators.

parseExpr works as a classical recursive descent parser, splitting
precedence levels into separate methods, where the call stack is in
ascending order of precedence:
	parseExpr
	orParser
	andParser
	parseIsOp
	parseInOp
	parseComparisonOp
	parseArithOp: |, ^, &, << and >>, + and -, * and / and ||
	parseUnaryArithOp: - and ~
	parseLit
*/

func (p *parser) parseExpr() (Expr, *parseError) {
	debugf("parseExpr: %v", p)

	return orParser.parse(p)
}

// binOpParser is a generic meta-parser for binary operations.
// It assumes the operation is left associative.
type binOpParser struct {
	LHS, RHS func(*parser) (Expr, *parseError)
	Op       string
	ArgCheck func(Expr) error
	Combiner func(lhs, rhs Expr) Expr
}

func (bin binOpParser) parse(p *parser) (Expr, *parseError) {
	expr, err := bin.LHS(p)
	if err != nil {
		return nil, err
	}

	for {
		if !p.eat(bin.Op) {
			break
		}
		rhs, err := bin.RHS(p)
		if err != nil {
			return nil, err
		}
		if bin.ArgCheck != nil {
			if err := bin.ArgCheck(expr); err != nil {
				return nil, p.errorf("%v", err)
			}
			if err := bin.ArgCheck(rhs); err != nil {
				return nil, p.errorf("%v", err)
			}
		}
		expr = bin.Combiner(expr, rhs)
	}
	return expr, nil
}

// Break initialisation loop.
func init() { orParser = orParserShim }

var (
	boolExprCheck = func(expr Expr) error {
		if _, ok := expr.(BoolExpr); !ok {
			return fmt.Errorf("got %T, want a boolean expression", expr)
		}
		return nil
	}

	orParser binOpParser

	orParserShim = binOpParser{
		LHS:      andParser.parse,
		RHS:      andParser.parse,
		Op:       "OR",
		ArgCheck: boolExprCheck,
		Combiner: func(lhs, rhs Expr) Expr {
			return LogicalOp{LHS: lhs.(BoolExpr), Op: Or, RHS: rhs.(BoolExpr)}
		},
	}
	andParser = binOpParser{
		LHS:      (*parser).parseLogicalNot,
		RHS:      (*parser).parseLogicalNot,
		Op:       "AND",
		ArgCheck: boolExprCheck,
		Combiner: func(lhs, rhs Expr) Expr {
			return LogicalOp{LHS: lhs.(BoolExpr), Op: And, RHS: rhs.(BoolExpr)}
		},
	}

	bitOrParser  = newBinArithParser("|", BitOr, bitXorParser.parse)
	bitXorParser = newBinArithParser("^", BitXor, bitAndParser.parse)
	bitAndParser = newBinArithParser("&", BitAnd, bitShrParser.parse)
	bitShrParser = newBinArithParser(">>", BitShr, bitShlParser.parse)
	bitShlParser = newBinArithParser("<<", BitShl, subParser.parse)
	subParser    = newBinArithParser("-", Sub, addParser.parse)
	addParser    = newBinArithParser("+", Add, concatParser.parse)
	concatParser = newBinArithParser("||", Concat, divParser.parse)
	divParser    = newBinArithParser("/", Div, mulParser.parse)
	mulParser    = newBinArithParser("*", Mul, (*parser).parseUnaryArithOp)
)

func newBinArithParser(opStr string, op ArithOperator, nextPrec func(*parser) (Expr, *parseError)) binOpParser {
	return binOpParser{
		LHS: nextPrec,
		RHS: nextPrec,
		Op:  opStr,
		// TODO: ArgCheck? numeric inputs only, except for ||.
		Combiner: func(lhs, rhs Expr) Expr {
			return ArithOp{LHS: lhs, Op: op, RHS: rhs}
		},
	}
}

func (p *parser) parseLogicalNot() (Expr, *parseError) {
	if !p.eat("NOT") {
		return p.parseIsOp()
	}
	be, err := p.parseBoolExpr()
	if err != nil {
		return nil, err
	}
	return LogicalOp{Op: Not, RHS: be}, nil
}

func (p *parser) parseIsOp() (Expr, *parseError) {
	debugf("parseIsOp: %v", p)

	expr, err := p.parseInOp()
	if err != nil {
		return nil, err
	}

	tok := p.next()
	if tok.err != nil || tok.value != "IS" {
		p.back()
		return expr, nil
	}

	isOp := IsOp{LHS: expr}
	if p.eat("NOT") {
		isOp.Neg = true
	}

	tok = p.next()
	if tok.err != nil {
		return nil, tok.err
	}
	switch tok.value {
	case "NULL":
		isOp.RHS = Null
	case "TRUE":
		isOp.RHS = True
	case "FALSE":
		isOp.RHS = False
	default:
		return nil, p.errorf("got %q, want NULL or TRUE or FALSE", tok.value)
	}

	return isOp, nil
}

func (p *parser) parseInOp() (Expr, *parseError) {
	debugf("parseInOp: %v", p)

	expr, err := p.parseComparisonOp()
	if err != nil {
		return nil, err
	}

	// TODO: do we need to do lookahead?

	inOp := InOp{LHS: expr}
	if p.eat("NOT") {
		inOp.Neg = true
	}

	if !p.eat("IN") {
		// TODO: push back the "NOT"?
		return expr, nil
	}

	if p.eat("UNNEST") {
		inOp.Unnest = true
	}

	inOp.RHS, err = p.parseParenExprList()
	if err != nil {
		return nil, err
	}
	return inOp, nil
}

var symbolicOperators = map[string]ComparisonOperator{
	"<":  Lt,
	"<=": Le,
	">":  Gt,
	">=": Ge,
	"=":  Eq,
	"!=": Ne,
	"<>": Ne,
}

func (p *parser) parseComparisonOp() (Expr, *parseError) {
	debugf("parseComparisonOp: %v", p)

	expr, err := p.parseArithOp()
	if err != nil {
		return nil, err
	}

	for {
		tok := p.next()
		if tok.err != nil {
			p.back()
			break
		}
		var op ComparisonOperator
		var ok, rhs2 bool
		if tok.value == "NOT" {
			tok := p.next()
			switch {
			case tok.err != nil:
				// TODO: Does this need to push back two?
				return nil, err
			case tok.value == "LIKE":
				op, ok = NotLike, true
			case tok.value == "BETWEEN":
				op, ok, rhs2 = NotBetween, true, true
			default:
				// TODO: Does this need to push back two?
				return nil, p.errorf("got %q, want LIKE or BETWEEN", tok.value)
			}
		} else if tok.value == "LIKE" {
			op, ok = Like, true
		} else if tok.value == "BETWEEN" {
			op, ok, rhs2 = Between, true, true
		} else {
			op, ok = symbolicOperators[tok.value]
		}
		if !ok {
			p.back()
			break
		}

		rhs, err := p.parseArithOp()
		if err != nil {
			return nil, err
		}
		co := ComparisonOp{LHS: expr, Op: op, RHS: rhs}

		if rhs2 {
			if err := p.expect("AND"); err != nil {
				return nil, err
			}
			rhs2, err := p.parseArithOp()
			if err != nil {
				return nil, err
			}
			co.RHS2 = rhs2
		}

		expr = co
	}
	return expr, nil
}

func (p *parser) parseArithOp() (Expr, *parseError) {
	return bitOrParser.parse(p)
}

var unaryArithOperators = map[string]ArithOperator{
	"-": Neg,
	"~": BitNot,
	"+": Plus,
}

func (p *parser) parseUnaryArithOp() (Expr, *parseError) {
	tok := p.next()
	if tok.err != nil {
		return nil, tok.err
	}

	op := tok.value

	if op == "-" || op == "+" {
		// If the next token is a numeric token, combine and parse as a literal.
		ntok := p.next()
		if ntok.err == nil {
			switch ntok.typ {
			case int64Token:
				comb := op + ntok.value
				n, err := strconv.ParseInt(comb, ntok.int64Base, 64)
				if err != nil {
					return nil, p.errorf("%v", err)
				}
				return IntegerLiteral(n), nil
			case float64Token:
				f := ntok.float64
				if op == "-" {
					f = -f
				}
				return FloatLiteral(f), nil
			}
		}
		// It is not possible for the p.back() lower down to fire
		// because - and + are in unaryArithOperators.
		p.back()
	}

	if op, ok := unaryArithOperators[op]; ok {
		e, err := p.parseLit()
		if err != nil {
			return nil, err
		}
		return ArithOp{Op: op, RHS: e}, nil
	}
	p.back()

	return p.parseLit()
}

func (p *parser) parseLit() (Expr, *parseError) {
	tok := p.next()
	if tok.err != nil {
		return nil, tok.err
	}

	switch tok.typ {
	case int64Token:
		n, err := strconv.ParseInt(tok.value, tok.int64Base, 64)
		if err != nil {
			return nil, p.errorf("%v", err)
		}
		return IntegerLiteral(n), nil
	case float64Token:
		return FloatLiteral(tok.float64), nil
	case stringToken:
		return StringLiteral(tok.string), nil
	case bytesToken:
		return BytesLiteral(tok.string), nil
	}

	// Handle parenthesized expressions.
	if tok.value == "(" {
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if err := p.expect(")"); err != nil {
			return nil, err
		}
		return Paren{Expr: e}, nil
	}

	// If the literal was an identifier, and there's an open paren next,
	// this is a function invocation.
	// TODO: Case-insensitivity.
	if name := tok.value; funcs[name] && p.sniff("(") {
		list, err := p.parseParenExprList()
		if err != nil {
			return nil, err
		}
		return Func{
			Name: name,
			Args: list,
		}, nil
	}

	// Handle some reserved keywords and special tokens that become specific values.
	switch tok.value {
	case "TRUE":
		return True, nil
	case "FALSE":
		return False, nil
	case "NULL":
		return Null, nil
	case "*":
		return Star, nil
	default:
		// TODO: Check IsKeyWord(tok.value), and return a distinguished type,
		// then only accept that when parsing. That will also permit
		// case insensitivity for keywords.
	}

	// Handle array literals.
	if tok.value == "ARRAY" || tok.value == "[" {
		p.back()
		return p.parseArrayLit()
	}

	// TODO: more types of literals (struct, date, timestamp).

	// Try a parameter.
	// TODO: check character sets.
	if strings.HasPrefix(tok.value, "@") {
		return Param(tok.value[1:]), nil
	}

	// Only thing left is a path expression or standalone identifier.
	p.back()
	pe, err := p.parsePathExp()
	if err != nil {
		return nil, err
	}
	if len(pe) == 1 {
		return pe[0], nil // identifier
	}
	return pe, nil
}

func (p *parser) parseArrayLit() (Array, *parseError) {
	// ARRAY keyword is optional.
	// TODO: If it is present, consume any <T> after it.
	p.eat("ARRAY")

	var arr Array
	err := p.parseCommaList("[", "]", func(p *parser) *parseError {
		e, err := p.parseLit()
		if err != nil {
			return err
		}
		// TODO: Do type consistency checking here?
		arr = append(arr, e)
		return nil
	})
	return arr, err
}

func (p *parser) parsePathExp() (PathExp, *parseError) {
	var pe PathExp
	for {
		tok := p.next()
		if tok.err != nil {
			return nil, tok.err
		}
		switch tok.typ {
		case quotedID:
			pe = append(pe, ID(tok.string))
		case unquotedID:
			pe = append(pe, ID(tok.value))
		default:
			// TODO: Is this correct?
			return nil, p.errorf("expected identifer")
		}
		if !p.eat(".") {
			break
		}
	}
	return pe, nil
}

func (p *parser) parseBoolExpr() (BoolExpr, *parseError) {
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	be, ok := expr.(BoolExpr)
	if !ok {
		return nil, p.errorf("got non-bool expression %T", expr)
	}
	return be, nil
}

func (p *parser) parseAlias() (ID, *parseError) {
	// The docs don't specify what lexical token is valid for an alias,
	// but it seems likely that it is an identifier.
	return p.parseTableOrIndexOrColumnName()
}

func (p *parser) parseTableOrIndexOrColumnName() (ID, *parseError) {
	/*
		table_name and column_name and index_name:
				{a—z|A—Z}[{a—z|A—Z|0—9|_}+]
	*/

	tok := p.next()
	if tok.err != nil {
		return "", tok.err
	}
	switch tok.typ {
	case quotedID:
		return ID(tok.string), nil
	case unquotedID:
		// TODO: enforce restrictions
		return ID(tok.value), nil
	default:
		return "", p.errorf("expected identifier")
	}
}

func (p *parser) parseOnDelete() (OnDelete, *parseError) {
	/*
		CASCADE
		NO ACTION
	*/

	tok := p.next()
	if tok.err != nil {
		return 0, tok.err
	}
	if tok.value == "CASCADE" {
		return CascadeOnDelete, nil
	}
	if tok.value != "NO" {
		return 0, p.errorf("got %q, want NO or CASCADE", tok.value)
	}
	if err := p.expect("ACTION"); err != nil {
		return 0, err
	}
	return NoActionOnDelete, nil
}

// parseCommaList parses a comma-separated list enclosed by bra and ket,
// delegating to f for the individual element parsing.
func (p *parser) parseCommaList(bra, ket string, f func(*parser) *parseError) *parseError {
	if err := p.expect(bra); err != nil {
		return err
	}
	for {
		if p.eat(ket) {
			return nil
		}

		err := f(p)
		if err != nil {
			return err
		}

		// ket or "," should be next.
		tok := p.next()
		if tok.err != nil {
			return err
		}
		if tok.value == ket {
			return nil
		} else if tok.value == "," {
			continue
		} else {
			return p.errorf(`got %q, want %q or ","`, tok.value, ket)
		}
	}
}
