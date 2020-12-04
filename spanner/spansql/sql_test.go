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
	"reflect"
	"testing"
)

func boolAddr(b bool) *bool {
	return &b
}

func TestSQL(t *testing.T) {
	reparseDDL := func(s string) (interface{}, error) {
		ddl, err := ParseDDLStmt(s)
		if err != nil {
			return nil, err
		}
		ddl.clearOffset()
		return ddl, nil
	}
	reparseDML := func(s string) (interface{}, error) {
		dml, err := ParseDMLStmt(s)
		if err != nil {
			return nil, err
		}
		return dml, nil
	}
	reparseQuery := func(s string) (interface{}, error) {
		q, err := ParseQuery(s)
		return q, err
	}
	reparseExpr := func(s string) (interface{}, error) {
		e, pe := newParser("f-expr", s).parseExpr()
		if pe != nil {
			return nil, pe
		}
		return e, nil
	}

	line := func(n int) Position { return Position{Line: n} }
	tests := []struct {
		data    interface{ SQL() string }
		sql     string
		reparse func(string) (interface{}, error)
	}{
		{
			&CreateTable{
				Name: "Ta",
				Columns: []ColumnDef{
					{Name: "Ca", Type: Type{Base: Bool}, NotNull: true, Position: line(2)},
					{Name: "Cb", Type: Type{Base: Int64}, Position: line(3)},
					{Name: "Cc", Type: Type{Base: Float64}, Position: line(4)},
					{Name: "Cd", Type: Type{Base: String, Len: 17}, Position: line(5)},
					{Name: "Ce", Type: Type{Base: String, Len: MaxLen}, Position: line(6)},
					{Name: "Cf", Type: Type{Base: Bytes, Len: 4711}, Position: line(7)},
					{Name: "Cg", Type: Type{Base: Bytes, Len: MaxLen}, Position: line(8)},
					{Name: "Ch", Type: Type{Base: Date}, Position: line(9)},
					{Name: "Ci", Type: Type{Base: Timestamp}, Options: ColumnOptions{AllowCommitTimestamp: boolAddr(true)}, Position: line(10)},
					{Name: "Cj", Type: Type{Array: true, Base: Int64}, Position: line(11)},
					{Name: "Ck", Type: Type{Array: true, Base: String, Len: MaxLen}, Position: line(12)},
					{Name: "Cl", Type: Type{Base: Timestamp}, Options: ColumnOptions{AllowCommitTimestamp: boolAddr(false)}, Position: line(13)},
					{Name: "Cm", Type: Type{Base: Int64}, Generated: Func{Name: "CHAR_LENGTH", Args: []Expr{ID("Ce")}}, Position: line(14)},
				},
				PrimaryKey: []KeyPart{
					{Column: "Ca"},
					{Column: "Cb", Desc: true},
				},
				Position: line(1),
			},
			`CREATE TABLE Ta (
  Ca BOOL NOT NULL,
  Cb INT64,
  Cc FLOAT64,
  Cd STRING(17),
  Ce STRING(MAX),
  Cf BYTES(4711),
  Cg BYTES(MAX),
  Ch DATE,
  Ci TIMESTAMP OPTIONS (allow_commit_timestamp = true),
  Cj ARRAY<INT64>,
  Ck ARRAY<STRING(MAX)>,
  Cl TIMESTAMP OPTIONS (allow_commit_timestamp = null),
  Cm INT64 AS (CHAR_LENGTH(Ce)) STORED,
) PRIMARY KEY(Ca, Cb DESC)`,
			reparseDDL,
		},
		{
			&CreateTable{
				Name: "Tsub",
				Columns: []ColumnDef{
					{Name: "SomeId", Type: Type{Base: Int64}, NotNull: true, Position: line(2)},
					{Name: "OtherId", Type: Type{Base: Int64}, NotNull: true, Position: line(3)},
					// This column name uses a reserved keyword.
					{Name: "Hash", Type: Type{Base: Bytes, Len: 32}, Position: line(4)},
				},
				PrimaryKey: []KeyPart{
					{Column: "SomeId"},
					{Column: "OtherId"},
				},
				Interleave: &Interleave{
					Parent:   "Ta",
					OnDelete: CascadeOnDelete,
				},
				Position: line(1),
			},
			`CREATE TABLE Tsub (
  SomeId INT64 NOT NULL,
  OtherId INT64 NOT NULL,
  ` + "`Hash`" + ` BYTES(32),
) PRIMARY KEY(SomeId, OtherId),
  INTERLEAVE IN PARENT Ta ON DELETE CASCADE`,
			reparseDDL,
		},
		{
			&DropTable{
				Name:     "Ta",
				Position: line(1),
			},
			"DROP TABLE Ta",
			reparseDDL,
		},
		{
			&CreateIndex{
				Name:  "Ia",
				Table: "Ta",
				Columns: []KeyPart{
					{Column: "Ca"},
					{Column: "Cb", Desc: true},
				},
				Position: line(1),
			},
			"CREATE INDEX Ia ON Ta(Ca, Cb DESC)",
			reparseDDL,
		},
		{
			&DropIndex{
				Name:     "Ia",
				Position: line(1),
			},
			"DROP INDEX Ia",
			reparseDDL,
		},
		{
			&AlterTable{
				Name:       "Ta",
				Alteration: AddColumn{Def: ColumnDef{Name: "Ca", Type: Type{Base: Bool}, Position: line(1)}},
				Position:   line(1),
			},
			"ALTER TABLE Ta ADD COLUMN Ca BOOL",
			reparseDDL,
		},
		{
			&AlterTable{
				Name:       "Ta",
				Alteration: DropColumn{Name: "Ca"},
				Position:   line(1),
			},
			"ALTER TABLE Ta DROP COLUMN Ca",
			reparseDDL,
		},
		{
			&AlterTable{
				Name:       "Ta",
				Alteration: SetOnDelete{Action: NoActionOnDelete},
				Position:   line(1),
			},
			"ALTER TABLE Ta SET ON DELETE NO ACTION",
			reparseDDL,
		},
		{
			&AlterTable{
				Name:       "Ta",
				Alteration: SetOnDelete{Action: CascadeOnDelete},
				Position:   line(1),
			},
			"ALTER TABLE Ta SET ON DELETE CASCADE",
			reparseDDL,
		},
		{
			&AlterTable{
				Name: "Ta",
				Alteration: AlterColumn{
					Name: "Cg",
					Alteration: SetColumnType{
						Type: Type{Base: String, Len: MaxLen},
					},
				},
				Position: line(1),
			},
			"ALTER TABLE Ta ALTER COLUMN Cg STRING(MAX)",
			reparseDDL,
		},
		{
			&AlterTable{
				Name: "Ta",
				Alteration: AlterColumn{
					Name: "Ci",
					Alteration: SetColumnOptions{
						Options: ColumnOptions{
							AllowCommitTimestamp: boolAddr(false),
						},
					},
				},
				Position: line(1),
			},
			"ALTER TABLE Ta ALTER COLUMN Ci SET OPTIONS (allow_commit_timestamp = null)",
			reparseDDL,
		},
		{
			&Delete{
				Table: "Ta",
				Where: ComparisonOp{
					LHS: ID("C"),
					Op:  Gt,
					RHS: IntegerLiteral(2),
				},
			},
			"DELETE FROM Ta WHERE C > 2",
			reparseDML,
		},
		{
			&Update{
				Table: "Ta",
				Items: []UpdateItem{
					{Column: "Cb", Value: IntegerLiteral(4)},
					{Column: "Ce", Value: StringLiteral("wow")},
					{Column: "Cf", Value: ID("Cg")},
					{Column: "Cg", Value: Null},
					{Column: "Ch", Value: nil},
				},
				Where: ID("Ca"),
			},
			`UPDATE Ta SET Cb = 4, Ce = "wow", Cf = Cg, Cg = NULL, Ch = DEFAULT WHERE Ca`,
			reparseDML,
		},
		{
			Query{
				Select: Select{
					List: []Expr{ID("A"), ID("B")},
					From: []SelectFrom{SelectFromTable{Table: "Table"}},
					Where: LogicalOp{
						LHS: ComparisonOp{
							LHS: ID("C"),
							Op:  Lt,
							RHS: StringLiteral("whelp"),
						},
						Op: And,
						RHS: IsOp{
							LHS: ID("D"),
							Neg: true,
							RHS: Null,
						},
					},
					ListAliases: []ID{"", "banana"},
				},
				Order: []Order{{Expr: ID("OCol"), Desc: true}},
				Limit: IntegerLiteral(1000),
			},
			`SELECT A, B AS banana FROM Table WHERE C < "whelp" AND D IS NOT NULL ORDER BY OCol DESC LIMIT 1000`,
			reparseQuery,
		},
		{
			Query{
				Select: Select{
					List: []Expr{IntegerLiteral(7)},
				},
			},
			`SELECT 7`,
			reparseQuery,
		},
		{
			ComparisonOp{LHS: ID("X"), Op: NotBetween, RHS: ID("Y"), RHS2: ID("Z")},
			`X NOT BETWEEN Y AND Z`,
			reparseExpr,
		},
		{
			Query{
				Select: Select{
					List: []Expr{
						ID("Desc"),
					},
				},
			},
			"SELECT `Desc`",
			reparseQuery,
		},
	}
	for _, test := range tests {
		sql := test.data.SQL()
		if sql != test.sql {
			t.Errorf("%v.SQL() wrong.\n got %s\nwant %s", test.data, sql, test.sql)
			continue
		}

		// As a confidence check, confirm that parsing the SQL produces the original input.
		data, err := test.reparse(sql)
		if err != nil {
			t.Errorf("Reparsing %q: %v", sql, err)
			continue
		}
		if !reflect.DeepEqual(data, test.data) {
			t.Errorf("Reparsing %q wrong.\n got %v\nwant %v", sql, data, test.data)
		}
	}
}
