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
	"time"

	"cloud.google.com/go/civil"
)

func TestParseQuery(t *testing.T) {
	tests := []struct {
		in   string
		want Query
	}{
		{`SELECT 17`, Query{Select: Select{List: []Expr{IntegerLiteral(17)}}}},
		{`SELECT Alias AS aka From Characters WHERE Age < @ageLimit AND Alias IS NOT NULL ORDER BY Age DESC LIMIT @limit OFFSET 3` + "\n\t",
			Query{
				Select: Select{
					List: []Expr{ID("Alias")},
					From: []SelectFrom{SelectFromTable{
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
					ListAliases: []ID{"aka"},
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
					From: []SelectFrom{SelectFromTable{Table: "Packages"}},
				},
			},
		},
		{`SELECT * FROM Packages`,
			Query{
				Select: Select{
					List: []Expr{Star},
					From: []SelectFrom{SelectFromTable{Table: "Packages"}},
				},
			},
		},
		{`SELECT date, timestamp as timestamp FROM Packages WHERE date = DATE '2014-09-27' AND timestamp = TIMESTAMP '2014-09-27 12:30:00'`,
			Query{
				Select: Select{
					List: []Expr{ID("date"), ID("timestamp")},
					From: []SelectFrom{SelectFromTable{Table: "Packages"}},
					Where: LogicalOp{
						Op: And,
						LHS: ComparisonOp{
							Op:  Eq,
							LHS: ID("date"),
							RHS: DateLiteral{Year: 2014, Month: 9, Day: 27},
						},
						RHS: ComparisonOp{
							Op:  Eq,
							LHS: ID("timestamp"),
							RHS: TimestampLiteral(timef(t, "2006-01-02 15:04:05", "2014-09-27 12:30:00")),
						},
					},
					ListAliases: []ID{"", "timestamp"},
				},
			},
		},
		{`SELECT UNIX_DATE(DATE "2008-12-25")`,
			Query{
				Select: Select{
					List: []Expr{Func{Name: "UNIX_DATE", Args: []Expr{DateLiteral{Year: 2008, Month: 12, Day: 25}}}},
				},
			},
		},
		{`SELECT * FROM Foo WHERE STARTS_WITH(Bar, 'B')`,
			Query{
				Select: Select{
					List:  []Expr{Star},
					From:  []SelectFrom{SelectFromTable{Table: "Foo"}},
					Where: Func{Name: "STARTS_WITH", Args: []Expr{ID("Bar"), StringLiteral("B")}},
				},
			},
		},
		{`SELECT * FROM Foo WHERE CAST(Bar AS STRING)='Bar'`,
			Query{
				Select: Select{
					List: []Expr{Star},
					From: []SelectFrom{SelectFromTable{Table: "Foo"}},
					Where: ComparisonOp{
						Op:  Eq,
						LHS: Func{Name: "CAST", Args: []Expr{TypedExpr{Expr: ID("Bar"), Type: Type{Base: String}}}},
						RHS: StringLiteral("Bar"),
					},
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
					From:        []SelectFrom{SelectFromTable{Table: "PlayerStats"}},
					GroupBy:     []Expr{ID("FirstName"), ID("LastName")},
					ListAliases: []ID{"total_points", "", "surname"},
				},
			},
		},
		// https://github.com/googleapis/google-cloud-go/issues/1973
		{`SELECT COUNT(*) AS count FROM Lists AS l WHERE l.user_id=@userID`,
			Query{
				Select: Select{
					List: []Expr{
						Func{Name: "COUNT", Args: []Expr{Star}},
					},
					From: []SelectFrom{SelectFromTable{Table: "Lists", Alias: "l"}},
					Where: ComparisonOp{
						Op:  Eq,
						LHS: PathExp{"l", "user_id"},
						RHS: Param("userID"),
					},
					ListAliases: []ID{"count"},
				},
			},
		},
		// with single table hint
		{`SELECT * FROM Packages@{FORCE_INDEX=PackagesIdx} WHERE package_idx=@packageIdx`,
			Query{
				Select: Select{
					List: []Expr{Star},
					From: []SelectFrom{SelectFromTable{Table: "Packages", Hints: map[string]string{"FORCE_INDEX": "PackagesIdx"}}},
					Where: ComparisonOp{
						Op:  Eq,
						LHS: ID("package_idx"),
						RHS: Param("packageIdx"),
					},
				},
			},
		},
		// with multiple table hints
		{`SELECT * FROM Packages@{ FORCE_INDEX=PackagesIdx, GROUPBY_SCAN_OPTIMIZATION=TRUE } WHERE package_idx=@packageIdx`,
			Query{
				Select: Select{
					List: []Expr{Star},
					From: []SelectFrom{SelectFromTable{Table: "Packages", Hints: map[string]string{"FORCE_INDEX": "PackagesIdx", "GROUPBY_SCAN_OPTIMIZATION": "TRUE"}}},
					Where: ComparisonOp{
						Op:  Eq,
						LHS: ID("package_idx"),
						RHS: Param("packageIdx"),
					},
				},
			},
		},
		{`SELECT * FROM A INNER JOIN B ON A.w = B.y`,
			Query{
				Select: Select{
					List: []Expr{Star},
					From: []SelectFrom{SelectFromJoin{
						Type: InnerJoin,
						LHS:  SelectFromTable{Table: "A"},
						RHS:  SelectFromTable{Table: "B"},
						On: ComparisonOp{
							Op:  Eq,
							LHS: PathExp{"A", "w"},
							RHS: PathExp{"B", "y"},
						},
					}},
				},
			},
		},
		{`SELECT * FROM A INNER JOIN B USING (x)`,
			Query{
				Select: Select{
					List: []Expr{Star},
					From: []SelectFrom{SelectFromJoin{
						Type:  InnerJoin,
						LHS:   SelectFromTable{Table: "A"},
						RHS:   SelectFromTable{Table: "B"},
						Using: []ID{"x"},
					}},
				},
			},
		},
		{`SELECT Roster . LastName, TeamMascot.Mascot FROM Roster JOIN TeamMascot ON Roster.SchoolID = TeamMascot.SchoolID`,
			Query{
				Select: Select{
					List: []Expr{
						PathExp{"Roster", "LastName"},
						PathExp{"TeamMascot", "Mascot"},
					},
					From: []SelectFrom{SelectFromJoin{
						Type: InnerJoin,
						LHS:  SelectFromTable{Table: "Roster"},
						RHS:  SelectFromTable{Table: "TeamMascot"},
						On: ComparisonOp{
							Op:  Eq,
							LHS: PathExp{"Roster", "SchoolID"},
							RHS: PathExp{"TeamMascot", "SchoolID"},
						},
					}},
				},
			},
		},
		// Joins with hints.
		{`SELECT * FROM A HASH JOIN B USING (x)`,
			Query{
				Select: Select{
					List: []Expr{Star},
					From: []SelectFrom{SelectFromJoin{
						Type:  InnerJoin,
						LHS:   SelectFromTable{Table: "A"},
						RHS:   SelectFromTable{Table: "B"},
						Using: []ID{"x"},
						Hints: map[string]string{"JOIN_METHOD": "HASH_JOIN"},
					}},
				},
			},
		},
		{`SELECT * FROM A JOIN @{ JOIN_METHOD=HASH_JOIN } B USING (x)`,
			Query{
				Select: Select{
					List: []Expr{Star},
					From: []SelectFrom{SelectFromJoin{
						Type:  InnerJoin,
						LHS:   SelectFromTable{Table: "A"},
						RHS:   SelectFromTable{Table: "B"},
						Using: []ID{"x"},
						Hints: map[string]string{"JOIN_METHOD": "HASH_JOIN"},
					}},
				},
			},
		},
		{`SELECT * FROM UNNEST ([1, 2, 3]) AS data`,
			Query{
				Select: Select{
					List: []Expr{Star},
					From: []SelectFrom{SelectFromUnnest{
						Expr: Array{
							IntegerLiteral(1),
							IntegerLiteral(2),
							IntegerLiteral(3),
						},
						Alias: ID("data"),
					}},
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
		{`+X * -Y`, ArithOp{LHS: ArithOp{Op: Plus, RHS: ID("X")}, Op: Mul, RHS: ArithOp{Op: Neg, RHS: ID("Y")}}},
		// Don't require space around +/- operators.
		{`ID+100`, ArithOp{LHS: ID("ID"), Op: Add, RHS: IntegerLiteral(100)}},
		{`ID-100`, ArithOp{LHS: ID("ID"), Op: Sub, RHS: IntegerLiteral(100)}},
		{`ID&0x3fff`, ArithOp{LHS: ID("ID"), Op: BitAnd, RHS: IntegerLiteral(0x3fff)}},
		{`SHA1("Hello" || " " || "World")`, Func{Name: "SHA1", Args: []Expr{ArithOp{LHS: ArithOp{LHS: StringLiteral("Hello"), Op: Concat, RHS: StringLiteral(" ")}, Op: Concat, RHS: StringLiteral("World")}}}},
		{`Count > 0`, ComparisonOp{LHS: ID("Count"), Op: Gt, RHS: IntegerLiteral(0)}},
		{`Name LIKE "Eve %"`, ComparisonOp{LHS: ID("Name"), Op: Like, RHS: StringLiteral("Eve %")}},
		{`Speech NOT LIKE "_oo"`, ComparisonOp{LHS: ID("Speech"), Op: NotLike, RHS: StringLiteral("_oo")}},
		{`A AND NOT B`, LogicalOp{LHS: ID("A"), Op: And, RHS: LogicalOp{Op: Not, RHS: ID("B")}}},
		{`X BETWEEN Y AND Z`, ComparisonOp{LHS: ID("X"), Op: Between, RHS: ID("Y"), RHS2: ID("Z")}},
		{`@needle IN UNNEST(@haystack)`, InOp{LHS: Param("needle"), RHS: []Expr{Param("haystack")}, Unnest: true}},
		{`@needle NOT IN UNNEST(@haystack)`, InOp{LHS: Param("needle"), Neg: true, RHS: []Expr{Param("haystack")}, Unnest: true}},

		// Functions
		{`STARTS_WITH(Bar, 'B')`, Func{Name: "STARTS_WITH", Args: []Expr{ID("Bar"), StringLiteral("B")}}},
		{`CAST(Bar AS STRING)`, Func{Name: "CAST", Args: []Expr{TypedExpr{Expr: ID("Bar"), Type: Type{Base: String}}}}},
		{`SAFE_CAST(Bar AS INT64)`, Func{Name: "SAFE_CAST", Args: []Expr{TypedExpr{Expr: ID("Bar"), Type: Type{Base: Int64}}}}},

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

		// Date and timestamp literals:
		{`DATE '2014-09-27'`, DateLiteral(civil.Date{Year: 2014, Month: time.September, Day: 27})},
		{`TIMESTAMP '2014-09-27 12:30:00'`, TimestampLiteral(timef(t, "2006-01-02 15:04:05", "2014-09-27 12:30:00"))},

		// date and timestamp funclit
		{`DATE('2014-09-27')`, Func{Name: "DATE", Args: []Expr{StringLiteral("2014-09-27")}}},
		{`TIMESTAMP('2014-09-27 12:30:00')`, Func{Name: "TIMESTAMP", Args: []Expr{StringLiteral("2014-09-27 12:30:00")}}},
		// date and timestamp identifier
		{`DATE = '2014-09-27'`, ComparisonOp{LHS: ID("DATE"), Op: Eq, RHS: StringLiteral("2014-09-27")}},
		{`TIMESTAMP = '2014-09-27 12:30:00'`, ComparisonOp{LHS: ID("TIMESTAMP"), Op: Eq, RHS: StringLiteral("2014-09-27 12:30:00")}},
		// Array literals:
		// https://cloud.google.com/spanner/docs/lexical#array_literals
		{`[1, 2, 3]`, Array{IntegerLiteral(1), IntegerLiteral(2), IntegerLiteral(3)}},
		{`['x', 'y', 'xy']`, Array{StringLiteral("x"), StringLiteral("y"), StringLiteral("xy")}},
		{`ARRAY[1, 2, 3]`, Array{IntegerLiteral(1), IntegerLiteral(2), IntegerLiteral(3)}},

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
		if rem := p.Rem(); rem != "" {
			t.Errorf("[%s]: Unparsed [%s]", test.in, rem)
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
			CONSTRAINT Con4 CHECK (System != ""),
			CHECK (RepoPath != ""),
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
			-- leading multi comment immediately after inline comment
			BCol BOOL,
			Names ARRAY<STRING(MAX)>,
		) PRIMARY KEY (Dummy);

		-- Table with generated column.
		CREATE TABLE GenCol (
			Name STRING(MAX) NOT NULL,
			NameLen INT64 AS (char_length(Name)) STORED,
		) PRIMARY KEY (Name);

		-- Table with row deletion policy.
		CREATE TABLE WithRowDeletionPolicy (
			Name STRING(MAX) NOT NULL,
			DelTimestamp TIMESTAMP NOT NULL,
		) PRIMARY KEY (Name)
		, ROW DELETION POLICY ( OLDER_THAN ( DelTimestamp, INTERVAL 30 DAY ));

		ALTER TABLE WithRowDeletionPolicy DROP ROW DELETION POLICY;
		ALTER TABLE WithRowDeletionPolicy ADD ROW DELETION POLICY ( OLDER_THAN ( DelTimestamp, INTERVAL 30 DAY ));
		ALTER TABLE WithRowDeletionPolicy REPLACE ROW DELETION POLICY ( OLDER_THAN ( DelTimestamp, INTERVAL 30 DAY ));

		CREATE VIEW SingersView
		SQL SECURITY INVOKER
		AS SELECT SingerId, FullName
		FROM Singers
		ORDER BY LastName, FirstName;

		CREATE TABLE users (
		  user_id      STRING(36) NOT NULL,
		  some_string  STRING(16) NOT NULL,
		  number_key   INT64 AS (SAFE_CAST(SUBSTR(some_string, 2) AS INT64)) STORED,
		) PRIMARY KEY(user_id);

		-- Trailing comment at end of file.
		`, &DDL{Filename: "filename", List: []DDLStmt{
			&CreateTable{
				Name: "FooBar",
				Columns: []ColumnDef{
					{Name: "System", Type: Type{Base: String, Len: MaxLen}, NotNull: true, Position: line(2)},
					{Name: "RepoPath", Type: Type{Base: String, Len: MaxLen}, NotNull: true, Position: line(3)},
					{Name: "Count", Type: Type{Base: Int64}, Position: line(4)},
					{Name: "UpdatedAt", Type: Type{Base: Timestamp}, Options: ColumnOptions{AllowCommitTimestamp: boolAddr(true)}, Position: line(6)},
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
				Storing:    []ID{"Count"},
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
						Constraint: ForeignKey{
							Columns:    []ID{"System"},
							RefTable:   "FooBar",
							RefColumns: []ID{"System"},
							Position:   line(13),
						},
						Position: line(13),
					},
					{
						Constraint: ForeignKey{
							Columns:    []ID{"System", "RepoPath"},
							RefTable:   "Stranger",
							RefColumns: []ID{"Sys", "RPath"},
							Position:   line(15),
						},
						Position: line(15),
					},
					{
						Name: "Con4",
						Constraint: Check{
							Expr:     ComparisonOp{LHS: ID("System"), Op: Ne, RHS: StringLiteral("")},
							Position: line(18),
						},
						Position: line(18),
					},
					{
						Constraint: Check{
							Expr:     ComparisonOp{LHS: ID("RepoPath"), Op: Ne, RHS: StringLiteral("")},
							Position: line(19),
						},
						Position: line(19),
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
				Alteration: AddColumn{Def: ColumnDef{Name: "TZ", Type: Type{Base: Bytes, Len: 20}, Position: line(23)}},
				Position:   line(23),
			},
			&AlterTable{
				Name:       "FooBar",
				Alteration: DropColumn{Name: "TZ"},
				Position:   line(24),
			},
			&AlterTable{
				Name: "FooBar",
				Alteration: AddConstraint{Constraint: TableConstraint{
					Name: "Con2",
					Constraint: ForeignKey{
						Columns:    []ID{"RepoPath"},
						RefTable:   "Repos",
						RefColumns: []ID{"RPath"},
						Position:   line(25),
					},
					Position: line(25),
				}},
				Position: line(25),
			},
			&AlterTable{
				Name:       "FooBar",
				Alteration: DropConstraint{Name: "Con3"},
				Position:   line(26),
			},
			&AlterTable{
				Name:       "FooBar",
				Alteration: SetOnDelete{Action: NoActionOnDelete},
				Position:   line(27),
			},
			&AlterTable{
				Name: "FooBar",
				Alteration: AlterColumn{
					Name: "Author",
					Alteration: SetColumnType{
						Type:    Type{Base: String, Len: MaxLen},
						NotNull: true,
					},
				},
				Position: line(28),
			},
			&DropIndex{Name: "MyFirstIndex", Position: line(30)},
			&DropTable{Name: "FooBar", Position: line(31)},
			&CreateTable{
				Name: "NonScalars",
				Columns: []ColumnDef{
					{Name: "Dummy", Type: Type{Base: Int64}, NotNull: true, Position: line(36)},
					{Name: "Ids", Type: Type{Array: true, Base: Int64}, Position: line(37)},
					{Name: "BCol", Type: Type{Base: Bool}, Position: line(39)},
					{Name: "Names", Type: Type{Array: true, Base: String, Len: MaxLen}, Position: line(40)},
				},
				PrimaryKey: []KeyPart{{Column: "Dummy"}},
				Position:   line(35),
			},
			&CreateTable{
				Name: "GenCol",
				Columns: []ColumnDef{
					{Name: "Name", Type: Type{Base: String, Len: MaxLen}, NotNull: true, Position: line(45)},
					{
						Name: "NameLen", Type: Type{Base: Int64},
						Generated: Func{Name: "CHAR_LENGTH", Args: []Expr{ID("Name")}},
						Position:  line(46),
					},
				},
				PrimaryKey: []KeyPart{{Column: "Name"}},
				Position:   line(44),
			},
			&CreateTable{
				Name: "WithRowDeletionPolicy",
				Columns: []ColumnDef{
					{Name: "Name", Type: Type{Base: String, Len: MaxLen}, NotNull: true, Position: line(51)},
					{Name: "DelTimestamp", Type: Type{Base: Timestamp}, NotNull: true, Position: line(52)},
				},
				PrimaryKey: []KeyPart{{Column: "Name"}},
				RowDeletionPolicy: &RowDeletionPolicy{
					Column:  ID("DelTimestamp"),
					NumDays: 30,
				},
				Position: line(50),
			},
			&AlterTable{
				Name:       "WithRowDeletionPolicy",
				Alteration: DropRowDeletionPolicy{},
				Position:   line(56),
			},
			&AlterTable{
				Name: "WithRowDeletionPolicy",
				Alteration: AddRowDeletionPolicy{
					RowDeletionPolicy: RowDeletionPolicy{
						Column:  ID("DelTimestamp"),
						NumDays: 30,
					},
				},
				Position: line(57),
			},
			&AlterTable{
				Name: "WithRowDeletionPolicy",
				Alteration: ReplaceRowDeletionPolicy{
					RowDeletionPolicy: RowDeletionPolicy{
						Column:  ID("DelTimestamp"),
						NumDays: 30,
					},
				},
				Position: line(58),
			},
			&CreateView{
				Name:      "SingersView",
				OrReplace: false,
				Query: Query{
					Select: Select{
						List: []Expr{ID("SingerId"), ID("FullName")},
						From: []SelectFrom{SelectFromTable{
							Table: "Singers",
						}},
					},
					Order: []Order{
						{Expr: ID("LastName")},
						{Expr: ID("FirstName")},
					},
				},
				Position: line(60),
			},

			//	CREATE TABLE users (
			//	user_id      STRING(36) NOT NULL,
			//	some_string  STRING(16) NOT NULL,
			//	number_key   INT64 AS (SAFE_CAST(SUBSTR(some_string, 2) AS INT64)) STORED,
			//) PRIMARY KEY(user_id);
			&CreateTable{
				Name: "users",
				Columns: []ColumnDef{
					{Name: "user_id", Type: Type{Base: String, Len: 36}, NotNull: true, Position: line(67)},
					{Name: "some_string", Type: Type{Base: String, Len: 16}, NotNull: true, Position: line(68)},
					{
						Name: "number_key", Type: Type{Base: Int64},
						Generated: Func{Name: "SAFE_CAST", Args: []Expr{
							TypedExpr{Expr: Func{Name: "SUBSTR", Args: []Expr{ID("some_string"), IntegerLiteral(2)}}, Type: Type{Base: Int64}},
						}},
						Position: line(69),
					},
				},
				PrimaryKey: []KeyPart{{Column: "user_id"}},
				Position:   line(66),
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
			{Marker: "--", Isolated: true, Start: line(33), End: line(34),
				Text: []string{"This table has some commentary", "that spans multiple lines."}},
			// These comments shouldn't get combined:
			{Marker: "--", Start: line(36), End: line(36), Text: []string{"dummy comment"}},
			{Marker: "--", Start: line(37), End: line(37), Text: []string{"comment on ids"}},
			{Marker: "--", Isolated: true, Start: line(38), End: line(38), Text: []string{"leading multi comment immediately after inline comment"}},

			{Marker: "--", Isolated: true, Start: line(43), End: line(43), Text: []string{"Table with generated column."}},
			{Marker: "--", Isolated: true, Start: line(49), End: line(49), Text: []string{"Table with row deletion policy."}},

			// Comment after everything else.
			{Marker: "--", Isolated: true, Start: line(72), End: line(72), Text: []string{"Trailing comment at end of file."}},
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
		{`ALTER DATABASE dbname SET OPTIONS (optimizer_version=2, version_retention_period='7d', enable_key_visualizer=true)`,
			&DDL{Filename: "filename", List: []DDLStmt{
				&AlterDatabase{
					Name: "dbname",
					Alteration: SetDatabaseOptions{
						Options: DatabaseOptions{
							OptimizerVersion:       func(i int) *int { return &i }(2),
							VersionRetentionPeriod: func(s string) *string { return &s }("7d"),
							EnableKeyVisualizer:    func(b bool) *bool { return &b }(true),
						},
					},
					Position: line(1),
				},
			},
			}},
		{`ALTER DATABASE dbname SET OPTIONS (optimizer_version=null, version_retention_period=null, enable_key_visualizer=null)`,
			&DDL{Filename: "filename", List: []DDLStmt{
				&AlterDatabase{
					Name: "dbname",
					Alteration: SetDatabaseOptions{
						Options: DatabaseOptions{
							OptimizerVersion:       func(i int) *int { return &i }(0),
							VersionRetentionPeriod: func(s string) *string { return &s }(""),
							EnableKeyVisualizer:    func(b bool) *bool { return &b }(false),
						},
					},
					Position: line(1),
				},
			},
			}},
		{"CREATE OR REPLACE VIEW `SingersView` SQL SECURITY INVOKER AS SELECT SingerId, FullName, Picture FROM Singers ORDER BY LastName, FirstName",
			&DDL{Filename: "filename", List: []DDLStmt{
				&CreateView{
					Name:      "SingersView",
					OrReplace: true,
					Query: Query{
						Select: Select{
							List: []Expr{ID("SingerId"), ID("FullName"), ID("Picture")},
							From: []SelectFrom{SelectFromTable{
								Table: "Singers",
							}},
						},
						Order: []Order{
							{Expr: ID("LastName")},
							{Expr: ID("FirstName")},
						},
					},
					Position: line(1),
				},
			},
			}},
		{"DROP VIEW `SingersView`",
			&DDL{Filename: "filename", List: []DDLStmt{
				&DropView{
					Name:     "SingersView",
					Position: line(1),
				},
			},
			}},
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
	// There are no leading comments on the columns of NonScalars (except for BCol),
	// even though there's often a comment on the previous line.
	for _, cd := range tableByName(t, ddl, "NonScalars").Columns {
		if cd.Name == "BCol" {
			continue
		}
		if com := ddl.LeadingComment(cd); com != nil {
			t.Errorf("Leading comment found for NonScalars.%s: %v", cd.Name, com)
		}
	}
}

func tableByName(t *testing.T, ddl *DDL, name ID) *CreateTable {
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
		if _, pe := p.parseExpr(); pe != nil {
			return pe
		}
		return nil
	}
	query := func(p *parser) error {
		if _, pe := p.parseQuery(); pe != nil {
			return pe
		}
		return nil
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
		// Found by fuzzing.
		// https://github.com/googleapis/google-cloud-go/issues/2196
		{query, `/*/*/`, "invalid comment termination"},
	}
	for _, test := range tests {
		p := newParser("f", test.in)
		err := test.f(p)
		if err == nil && p.Rem() == "" {
			t.Errorf("%s: parsing [%s] succeeded, should have failed", test.desc, test.in)
		}
	}
}

func timef(t *testing.T, format, s string) time.Time {
	ti, err := time.ParseInLocation(format, string(s), defaultLocation)
	if err != nil {
		t.Errorf("parsing %s [%s] time.ParseInLocation failed.", s, format)
	}
	return ti
}
