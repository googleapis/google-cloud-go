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

package spansql

import (
	"fmt"
	"math"
	"reflect"
	"testing"
)

func TestParseQuery(t *testing.T) {
	tests := []struct {
		in   string
		want Query
	}{
		{`SELECT 17`, Query{Select: Select{List: []Expr{IntegerLiteral(17)}}}},
		{`SELECT Alias AS aka FROM Characters WHERE Age < @ageLimit AND Alias IS NOT NULL ORDER BY Age DESC LIMIT @limit OFFSET 3` + "\n\t",
			Query{
				Select: Select{
					List: []Expr{ID("Alias")},
					From: []SelectFrom{{
						Table: "Characters",
					}},
					Where: LogicalOp{
						Op: And,
						LHS: ComparisonOp{
							LHS: ID("Age"),
							Op:  Lt,
							RHS: Param("ageLimit"),
						},
						RHS: IsOp{
							LHS: ID("Alias"),
							Neg: true,
							RHS: Null,
						},
					},
					ListAliases: []string{"aka"},
				},
				Order: []Order{{
					Expr: ID("Age"),
					Desc: true,
				}},
				Limit:  Param("limit"),
				Offset: IntegerLiteral(3),
			},
		},
		{`SELECT COUNT(*) FROM Packages`,
			Query{
				Select: Select{
					List: []Expr{
						Func{
							Name: "COUNT",
							Args: []Expr{Star},
						},
					},
					From: []SelectFrom{{Table: "Packages"}},
				},
			},
		},
		{`SELECT * FROM Packages`,
			Query{
				Select: Select{
					List: []Expr{Star},
					From: []SelectFrom{{Table: "Packages"}},
				},
			},
		},
		{`SELECT SUM(PointsScored) AS total_points, FirstName, LastName AS surname FROM PlayerStats GROUP BY FirstName, LastName`,
			Query{
				Select: Select{
					List: []Expr{
						Func{Name: "SUM", Args: []Expr{ID("PointsScored")}},
						ID("FirstName"),
						ID("LastName"),
					},
					From:        []SelectFrom{{Table: "PlayerStats"}},
					GroupBy:     []Expr{ID("FirstName"), ID("LastName")},
					ListAliases: []string{"total_points", "", "surname"},
				},
			},
		},
	}
	for _, test := range tests {
		got, err := ParseQuery(test.in)
		if err != nil {
			t.Errorf("ParseQuery(%q): %v", test.in, err)
			continue
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("ParseQuery(%q) incorrect.\n got %#v\nwant %#v", test.in, got, test.want)
		}
	}
}

func TestParseExpr(t *testing.T) {
	tests := []struct {
		in   string
		want Expr
	}{
		{`17`, IntegerLiteral(17)},
		{`-1`, IntegerLiteral(-1)},
		{fmt.Sprintf(`%d`, math.MaxInt64), IntegerLiteral(math.MaxInt64)},
		{fmt.Sprintf(`%d`, math.MinInt64), IntegerLiteral(math.MinInt64)},
		{"1.797693134862315708145274237317043567981e+308", FloatLiteral(math.MaxFloat64)},
		{`4.940656458412465441765687928682213723651e-324`, FloatLiteral(math.SmallestNonzeroFloat64)},
		{`0xf00d`, IntegerLiteral(0xf00d)},
		{`-0xbeef`, IntegerLiteral(-0xbeef)},
		{`0XabCD`, IntegerLiteral(0xabcd)},
		{`-0XBEEF`, IntegerLiteral(-0xbeef)},
		{`123.456e-67`, FloatLiteral(123.456e-67)},
		{`-123.456e-67`, FloatLiteral(-123.456e-67)},
		{`.1E4`, FloatLiteral(0.1e4)},
		{`58.`, FloatLiteral(58)},
		{`4e2`, FloatLiteral(4e2)},
		{`X + Y * Z`, ArithOp{LHS: ID("X"), Op: Add, RHS: ArithOp{LHS: ID("Y"), Op: Mul, RHS: ID("Z")}}},
		{`X + Y + Z`, ArithOp{LHS: ArithOp{LHS: ID("X"), Op: Add, RHS: ID("Y")}, Op: Add, RHS: ID("Z")}},
		{`X * -Y`, ArithOp{LHS: ID("X"), Op: Mul, RHS: ArithOp{Op: Neg, RHS: ID("Y")}}},
		{`ID&0x3fff`, ArithOp{LHS: ID("ID"), Op: BitAnd, RHS: IntegerLiteral(0x3fff)}},
		{`SHA1("Hello" || " " || "World")`, Func{Name: "SHA1", Args: []Expr{ArithOp{LHS: ArithOp{LHS: StringLiteral("Hello"), Op: Concat, RHS: StringLiteral(" ")}, Op: Concat, RHS: StringLiteral("World")}}}},
		{`Count > 0`, ComparisonOp{LHS: ID("Count"), Op: Gt, RHS: IntegerLiteral(0)}},
		{`Name LIKE "Eve %"`, ComparisonOp{LHS: ID("Name"), Op: Like, RHS: StringLiteral("Eve %")}},
		{`Speech NOT LIKE "_oo"`, ComparisonOp{LHS: ID("Speech"), Op: NotLike, RHS: StringLiteral("_oo")}},
		{`A AND NOT B`, LogicalOp{LHS: ID("A"), Op: And, RHS: LogicalOp{Op: Not, RHS: ID("B")}}},
		{`X BETWEEN Y AND Z`, ComparisonOp{LHS: ID("X"), Op: Between, RHS: ID("Y"), RHS2: ID("Z")}},
		{`@needle IN UNNEST(@haystack)`, InOp{LHS: Param("needle"), RHS: []Expr{Param("haystack")}, Unnest: true}},

		// String literal:
		// Accept double quote and single quote.
		{`"hello"`, StringLiteral("hello")},
		{`'hello'`, StringLiteral("hello")},
		// Accept triple-quote.
		{`""" "hello" "world" """`, StringLiteral(` "hello" "world" `)},
		{"''' 'hello'\n'world' '''", StringLiteral(" 'hello'\n'world' ")},
		// Simple escape sequence
		{`"\a\b\f\n\r\t\v\\\?\"\'"`, StringLiteral("\a\b\f\n\r\t\v\\?\"'")},
		{`'\a\b\f\n\r\t\v\\\?\"\''`, StringLiteral("\a\b\f\n\r\t\v\\?\"'")},
		{"'\\`'", StringLiteral("`")},
		// Hex and unicode escape sequence
		{`"\060\x30\X30\u0030\U00000030"`, StringLiteral("00000")},
		{`'\060\x30\X30\u0030\U00000030'`, StringLiteral("00000")},
		{`"\uBEAF\ubeaf"`, StringLiteral("\ubeaf\ubeaf")},
		{`'\uBEAF\ubeaf'`, StringLiteral("\ubeaf\ubeaf")},
		// Escape sequence in triple quote is allowed.
		{`"""\u0030"""`, StringLiteral("0")},
		{`'''\u0030'''`, StringLiteral("0")},
		// Raw string literal
		{`R"\\"`, StringLiteral("\\\\")},
		{`R'\\'`, StringLiteral("\\\\")},
		{`r"\\"`, StringLiteral("\\\\")},
		{`r'\\'`, StringLiteral("\\\\")},
		{`R"\\\""`, StringLiteral("\\\\\\\"")},
		{`R"""\\//\\//"""`, StringLiteral("\\\\//\\\\//")},
		{"R'''\\\\//\n\\\\//'''", StringLiteral("\\\\//\n\\\\//")},

		// Bytes literal:
		{`B"hello"`, BytesLiteral("hello")},
		{`B'hello'`, BytesLiteral("hello")},
		{`b"hello"`, BytesLiteral("hello")},
		{`b'hello'`, BytesLiteral("hello")},
		{`B""" "hello" "world" """`, BytesLiteral(` "hello" "world" `)},
		{`B''' 'hello' 'world' '''`, BytesLiteral(` 'hello' 'world' `)},
		{`B"\a\b\f\n\r\t\v\\\?\"\'"`, BytesLiteral("\a\b\f\n\r\t\v\\?\"'")},
		{`B'\a\b\f\n\r\t\v\\\?\"\''`, BytesLiteral("\a\b\f\n\r\t\v\\?\"'")},
		{"B'''\n'''", BytesLiteral("\n")},
		{`br"\\"`, BytesLiteral("\\\\")},
		{`br'\\'`, BytesLiteral("\\\\")},
		{`rb"\\"`, BytesLiteral("\\\\")},
		{`rb'\\'`, BytesLiteral("\\\\")},
		{`RB"\\"`, BytesLiteral("\\\\")},
		{`RB'\\'`, BytesLiteral("\\\\")},
		{`BR"\\"`, BytesLiteral("\\\\")},
		{`BR'\\'`, BytesLiteral("\\\\")},
		{`RB"""\\//\\//"""`, BytesLiteral("\\\\//\\\\//")},
		{"RB'''\\\\//\n\\\\//'''", BytesLiteral("\\\\//\n\\\\//")},

		// OR is lower precedence than AND.
		{`A AND B OR C`, LogicalOp{LHS: LogicalOp{LHS: ID("A"), Op: And, RHS: ID("B")}, Op: Or, RHS: ID("C")}},
		{`A OR B AND C`, LogicalOp{LHS: ID("A"), Op: Or, RHS: LogicalOp{LHS: ID("B"), Op: And, RHS: ID("C")}}},
		// Parens to override normal precedence.
		{`A OR (B AND C)`, LogicalOp{LHS: ID("A"), Op: Or, RHS: Paren{Expr: LogicalOp{LHS: ID("B"), Op: And, RHS: ID("C")}}}},

		// This is the same as the WHERE clause from the test in ParseQuery.
		{`Age < @ageLimit AND Alias IS NOT NULL`,
			LogicalOp{
				LHS: ComparisonOp{LHS: ID("Age"), Op: Lt, RHS: Param("ageLimit")},
				Op:  And,
				RHS: IsOp{LHS: ID("Alias"), Neg: true, RHS: Null},
			},
		},

		// This used to be broken because the lexer didn't reset the token type.
		{`C < "whelp" AND D IS NOT NULL`,
			LogicalOp{
				LHS: ComparisonOp{LHS: ID("C"), Op: Lt, RHS: StringLiteral("whelp")},
				Op:  And,
				RHS: IsOp{LHS: ID("D"), Neg: true, RHS: Null},
			},
		},

		// Reserved keywords.
		{`TRUE AND FALSE`, LogicalOp{LHS: True, Op: And, RHS: False}},
		{`NULL`, Null},
	}
	for _, test := range tests {
		p := newParser("test-file", test.in)
		got, err := p.parseExpr()
		if err != nil {
			t.Errorf("[%s]: %v", test.in, err)
			continue
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("[%s]: incorrect parse\n got <%T> %#v\nwant <%T> %#v", test.in, got, got, test.want, test.want)
		}
		if p.s != "" {
			t.Errorf("[%s]: Unparsed [%s]", test.in, p.s)
		}
	}
}

func TestParseDDL(t *testing.T) {
	line := func(n int) Position { return Position{Line: n} }
	tests := []struct {
		in   string
		want *DDL
	}{
		{`CREATE TABLE FooBar (
			System STRING(MAX) NOT NULL,  # This is a comment.
			RepoPath STRING(MAX) NOT NULL,  -- This is another comment.
			Count INT64, /* This is a
						  * multiline comment. */
			UpdatedAt TIMESTAMP OPTIONS (allow_commit_timestamp = true),
		) PRIMARY KEY(System, RepoPath);
		CREATE UNIQUE INDEX MyFirstIndex ON FooBar (
			Count DESC
		) STORING (Count), INTERLEAVE IN SomeTable;
		CREATE TABLE FooBarAux (
			System STRING(MAX) NOT NULL,
			CONSTRAINT Con1 FOREIGN KEY (System) REFERENCES FooBar (System),
			RepoPath STRING(MAX) NOT NULL,
			FOREIGN KEY (System, RepoPath) REFERENCES Stranger (Sys, RPath), -- unnamed foreign key
			Author STRING(MAX) NOT NULL,
			CONSTRAINT BOOL,  -- not a constraint
		) PRIMARY KEY(System, RepoPath, Author),
		  INTERLEAVE IN PARENT FooBar ON DELETE CASCADE;

		ALTER TABLE FooBar ADD COLUMN TZ BYTES(20);
		ALTER TABLE FooBar DROP COLUMN TZ;
		ALTER TABLE FooBar ADD CONSTRAINT Con2 FOREIGN KEY (RepoPath) REFERENCES Repos (RPath);
		ALTER TABLE FooBar DROP CONSTRAINT Con3;
		ALTER TABLE FooBar SET ON DELETE NO ACTION;
		ALTER TABLE FooBar ALTER COLUMN Author STRING(MAX) NOT NULL;

		DROP INDEX MyFirstIndex;
		DROP TABLE FooBar;

		-- This table has some commentary
		-- that spans multiple lines.
		CREATE TABLE NonScalars (
			Dummy INT64 NOT NULL, -- dummy comment
			Ids ARRAY<INT64>, -- comment on ids
			Names ARRAY<STRING(MAX)>,
		) PRIMARY KEY (Dummy);
		`, &DDL{Filename: "filename", List: []DDLStmt{
			&CreateTable{
				Name: "FooBar",
				Columns: []ColumnDef{
					{Name: "System", Type: Type{Base: String, Len: MaxLen}, NotNull: true, Position: line(2)},
					{Name: "RepoPath", Type: Type{Base: String, Len: MaxLen}, NotNull: true, Position: line(3)},
					{Name: "Count", Type: Type{Base: Int64}, Position: line(4)},
					{Name: "UpdatedAt", Type: Type{Base: Timestamp}, AllowCommitTimestamp: boolAddr(true), Position: line(6)},
				},
				PrimaryKey: []KeyPart{
					{Column: "System"},
					{Column: "RepoPath"},
				},
				Position: line(1),
			},
			&CreateIndex{
				Name:       "MyFirstIndex",
				Table:      "FooBar",
				Columns:    []KeyPart{{Column: "Count", Desc: true}},
				Unique:     true,
				Storing:    []string{"Count"},
				Interleave: "SomeTable",
				Position:   line(8),
			},
			&CreateTable{
				Name: "FooBarAux",
				Columns: []ColumnDef{
					{Name: "System", Type: Type{Base: String, Len: MaxLen}, NotNull: true, Position: line(12)},
					{Name: "RepoPath", Type: Type{Base: String, Len: MaxLen}, NotNull: true, Position: line(14)},
					{Name: "Author", Type: Type{Base: String, Len: MaxLen}, NotNull: true, Position: line(16)},
					{Name: "CONSTRAINT", Type: Type{Base: Bool}, Position: line(17)},
				},
				Constraints: []TableConstraint{
					{
						Name: "Con1",
						ForeignKey: ForeignKey{
							Columns:    []string{"System"},
							RefTable:   "FooBar",
							RefColumns: []string{"System"},
							Position:   line(13),
						},
						Position: line(13),
					},
					{
						ForeignKey: ForeignKey{
							Columns:    []string{"System", "RepoPath"},
							RefTable:   "Stranger",
							RefColumns: []string{"Sys", "RPath"},
							Position:   line(15),
						},
						Position: line(15),
					},
				},
				PrimaryKey: []KeyPart{
					{Column: "System"},
					{Column: "RepoPath"},
					{Column: "Author"},
				},
				Interleave: &Interleave{
					Parent:   "FooBar",
					OnDelete: CascadeOnDelete,
				},
				Position: line(11),
			},
			&AlterTable{
				Name:       "FooBar",
				Alteration: AddColumn{Def: ColumnDef{Name: "TZ", Type: Type{Base: Bytes, Len: 20}, Position: line(21)}},
				Position:   line(21),
			},
			&AlterTable{
				Name:       "FooBar",
				Alteration: DropColumn{Name: "TZ"},
				Position:   line(22),
			},
			&AlterTable{
				Name: "FooBar",
				Alteration: AddConstraint{Constraint: TableConstraint{
					Name: "Con2",
					ForeignKey: ForeignKey{
						Columns:    []string{"RepoPath"},
						RefTable:   "Repos",
						RefColumns: []string{"RPath"},
						Position:   line(23),
					},
					Position: line(23),
				}},
				Position: line(23),
			},
			&AlterTable{
				Name:       "FooBar",
				Alteration: DropConstraint{Name: "Con3"},
				Position:   line(24),
			},
			&AlterTable{
				Name:       "FooBar",
				Alteration: SetOnDelete{Action: NoActionOnDelete},
				Position:   line(25),
			},
			&AlterTable{
				Name: "FooBar",
				Alteration: AlterColumn{Def: ColumnDef{
					Name:     "Author",
					Type:     Type{Base: String, Len: MaxLen},
					NotNull:  true,
					Position: line(26),
				}},
				Position: line(26),
			},
			&DropIndex{Name: "MyFirstIndex", Position: line(28)},
			&DropTable{Name: "FooBar", Position: line(29)},
			&CreateTable{
				Name: "NonScalars",
				Columns: []ColumnDef{
					{Name: "Dummy", Type: Type{Base: Int64}, NotNull: true, Position: line(34)},
					{Name: "Ids", Type: Type{Array: true, Base: Int64}, Position: line(35)},
					{Name: "Names", Type: Type{Array: true, Base: String, Len: MaxLen}, Position: line(36)},
				},
				PrimaryKey: []KeyPart{{Column: "Dummy"}},
				Position:   line(33),
			},
		}, Comments: []*Comment{
			{Marker: "#", Start: line(2), End: line(2),
				Text: []string{"This is a comment."}},
			{Marker: "--", Start: line(3), End: line(3),
				Text: []string{"This is another comment."}},
			{Marker: "/*", Start: line(4), End: line(5),
				Text: []string{" This is a", "\t\t\t\t\t\t  * multiline comment."}},
			{Marker: "--", Start: line(15), End: line(15),
				Text: []string{"unnamed foreign key"}},
			{Marker: "--", Start: line(17), End: line(17),
				Text: []string{"not a constraint"}},
			{Marker: "--", Isolated: true, Start: line(31), End: line(32),
				Text: []string{"This table has some commentary", "that spans multiple lines."}},
			// These comments shouldn't get combined:
			{Marker: "--", Start: line(34), End: line(34), Text: []string{"dummy comment"}},
			{Marker: "--", Start: line(35), End: line(35), Text: []string{"comment on ids"}},
		}}},
		// No trailing comma:
		{`ALTER TABLE T ADD COLUMN C2 INT64`, &DDL{Filename: "filename", List: []DDLStmt{
			&AlterTable{
				Name:       "T",
				Alteration: AddColumn{Def: ColumnDef{Name: "C2", Type: Type{Base: Int64}, Position: line(1)}},
				Position:   line(1),
			},
		}}},
		// Table and column names using reserved keywords.
		{`CREATE TABLE ` + "`enum`" + ` (
			` + "`With`" + ` STRING(MAX) NOT NULL,
		) PRIMARY KEY(` + "`With`" + `);
		`, &DDL{Filename: "filename", List: []DDLStmt{
			&CreateTable{
				Name: "enum",
				Columns: []ColumnDef{
					{Name: "With", Type: Type{Base: String, Len: MaxLen}, NotNull: true, Position: line(2)},
				},
				PrimaryKey: []KeyPart{
					{Column: "With"},
				},
				Position: line(1),
			},
		}}},
	}
	for _, test := range tests {
		got, err := ParseDDL("filename", test.in)
		if err != nil {
			t.Errorf("ParseDDL(%q): %v", test.in, err)
			continue
		}
		got.clearOffset()
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("ParseDDL(%q) incorrect.\n got %v\nwant %v", test.in, got, test.want)

			// Also log the specific elements that don't match to make it easier to debug
			// especially the large DDLs.
			for i := range got.List {
				if !reflect.DeepEqual(got.List[i], test.want.List[i]) {
					t.Errorf("\tstatement %d mismatch:\n\t got %v\n\twant %v", i, got.List[i], test.want.List[i])
				}
			}
			for i := range got.Comments {
				if !reflect.DeepEqual(got.Comments[i], test.want.Comments[i]) {
					t.Errorf("\tcomment %d mismatch:\n\t got %v\n\twant %v", i, got.Comments[i], test.want.Comments[i])
				}
			}
		}
	}

	// Check the comment discovey helpers on the first DDL.
	// Reparse it first so we get full position information.
	ddl, err := ParseDDL("filename", tests[0].in)
	if err != nil {
		t.Fatal(err)
	}
	// The CreateTable for NonScalars has a leading comment.
	com := ddl.LeadingComment(tableByName(t, ddl, "NonScalars"))
	if com == nil {
		t.Errorf("No leading comment found for NonScalars")
	} else if com.Text[0] != "This table has some commentary" {
		t.Errorf("LeadingComment returned the wrong comment for NonScalars")
	}
	// Second field of FooBar (RepoPath) has an inline comment.
	cd := tableByName(t, ddl, "FooBar").Columns[1]
	if com := ddl.InlineComment(cd); com == nil {
		t.Errorf("No inline comment found for FooBar.RepoPath")
	} else if com.Text[0] != "This is another comment." {
		t.Errorf("InlineComment returned the wrong comment (%q) for FooBar.RepoPath", com.Text[0])
	}
	// There are no leading comments on the columns of NonScalars,
	// even though there's often a comment on the previous line.
	for _, cd := range tableByName(t, ddl, "NonScalars").Columns {
		if com := ddl.LeadingComment(cd); com != nil {
			t.Errorf("Leading comment found for NonScalars.%s: %v", cd.Name, com)
		}
	}
}

func tableByName(t *testing.T, ddl *DDL, name string) *CreateTable {
	t.Helper()
	for _, stmt := range ddl.List {
		if ct, ok := stmt.(*CreateTable); ok && ct.Name == name {
			return ct
		}
	}
	t.Fatalf("no table with name %q", name)
	panic("unreachable")
}

func TestParseFailures(t *testing.T) {
	expr := func(p *parser) error {
		_, err := p.parseExpr()
		return err
	}

	tests := []struct {
		f    func(p *parser) error
		in   string
		desc string
	}{
		{expr, `0b337`, "binary literal"},
		{expr, `"foo\`, "unterminated string"},
		{expr, `"\i"`, "invalid escape sequence"},
		{expr, `"\0"`, "invalid escape sequence"},
		{expr, `"\099"`, "invalid escape sequence"},
		{expr, `"\400"`, "invalid escape sequence: octal digits overflow"},
		{expr, `"\x"`, "invalid escape sequence"},
		{expr, `"\xFZ"`, "invalid escape sequence"},
		{expr, `"\u"`, "invalid escape sequence"},
		{expr, `"\uFFFZ"`, "invalid escape sequence"},
		{expr, `"\uD800"`, "invalid unicode character (surrogate)"},
		{expr, `"\U"`, "invalid escape sequence"},
		{expr, `"\UFFFFFFFZ"`, "invalid escape sequence"},
		{expr, `"\U00110000"`, "invalid unicode character (out of range)"},
		{expr, "\"\n\"", "unterminated string by newline (double quote)"},
		{expr, "'\n'", "unterminated string by newline (single quote)"},
		{expr, "R\"\n\"", "unterminated raw string by newline (double quote)"},
		{expr, "R'\n'", "unterminated raw string by newline (single quote)"},
		{expr, `B"\u0030"`, "\\uXXXX sequence is not supported in bytes literal (double quote)"},
		{expr, `B'\u0030'`, "\\uXXXX sequence is not supported in bytes literal (double quote)"},
		{expr, `B"\U00000030"`, "\\UXXXXXXXX sequence is not supported in bytes literal (double quote)"},
		{expr, `B'\U00000030'`, "\\UXXXXXXXX sequence is not supported in bytes literal (double quote)"},
		{expr, `BB""`, "invalid string-like literal prefix"},
		{expr, `rr""`, "invalid string-like literal prefix"},
		{expr, `"""\"""`, "unterminated triple-quoted string by last backslash (double quote)"},
		{expr, `'''\'''`, "unterminated triple-quoted string by last backslash (single quote)"},
		{expr, `"foo" AND "bar"`, "logical operation on string literals"},
	}
	for _, test := range tests {
		p := newParser("f", test.in)
		err := test.f(p)
		if err == nil && p.Rem() == "" {
			t.Errorf("%s: parsing [%s] succeeded, should have failed", test.desc, test.in)
		}
	}
}
