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

// This file holds the type definitions for the SQL dialect.

import (
	"fmt"
	"math"
	"sort"
)

// TODO: More Position fields throughout; maybe in Query/Select.
// TODO: Perhaps identifiers in the AST should be ID-typed.

// CreateTable represents a CREATE TABLE statement.
// https://cloud.google.com/spanner/docs/data-definition-language#create_table
type CreateTable struct {
	Name        string
	Columns     []ColumnDef
	Constraints []TableConstraint
	PrimaryKey  []KeyPart
	Interleave  *Interleave

	Position Position // position of the "CREATE" token
}

func (ct *CreateTable) String() string { return fmt.Sprintf("%#v", ct) }
func (*CreateTable) isDDLStmt()        {}
func (ct *CreateTable) Pos() Position  { return ct.Position }
func (ct *CreateTable) clearOffset() {
	for i := range ct.Columns {
		// Mutate in place.
		ct.Columns[i].clearOffset()
	}
	for i := range ct.Constraints {
		// Mutate in place.
		ct.Constraints[i].clearOffset()
	}
	ct.Position.Offset = 0
}

// TableConstraint represents a constraint on a table.
type TableConstraint struct {
	Name       string // may be empty
	ForeignKey ForeignKey

	Position Position // position of the "CONSTRAINT" or "FOREIGN" token
}

func (tc TableConstraint) Pos() Position { return tc.Position }
func (tc *TableConstraint) clearOffset() {
	tc.Position.Offset = 0
	tc.ForeignKey.clearOffset()
}

// Interleave represents an interleave clause of a CREATE TABLE statement.
type Interleave struct {
	Parent   string
	OnDelete OnDelete
}

// CreateIndex represents a CREATE INDEX statement.
// https://cloud.google.com/spanner/docs/data-definition-language#create-index
type CreateIndex struct {
	Name    string
	Table   string
	Columns []KeyPart

	Unique       bool
	NullFiltered bool

	Storing    []string
	Interleave string

	Position Position // position of the "CREATE" token
}

func (ci *CreateIndex) String() string { return fmt.Sprintf("%#v", ci) }
func (*CreateIndex) isDDLStmt()        {}
func (ci *CreateIndex) Pos() Position  { return ci.Position }
func (ci *CreateIndex) clearOffset()   { ci.Position.Offset = 0 }

// DropTable represents a DROP TABLE statement.
// https://cloud.google.com/spanner/docs/data-definition-language#drop_table
type DropTable struct {
	Name string

	Position Position // position of the "DROP" token
}

func (dt *DropTable) String() string { return fmt.Sprintf("%#v", dt) }
func (*DropTable) isDDLStmt()        {}
func (dt *DropTable) Pos() Position  { return dt.Position }
func (dt *DropTable) clearOffset()   { dt.Position.Offset = 0 }

// DropIndex represents a DROP INDEX statement.
// https://cloud.google.com/spanner/docs/data-definition-language#drop-index
type DropIndex struct {
	Name string

	Position Position // position of the "DROP" token
}

func (di *DropIndex) String() string { return fmt.Sprintf("%#v", di) }
func (*DropIndex) isDDLStmt()        {}
func (di *DropIndex) Pos() Position  { return di.Position }
func (di *DropIndex) clearOffset()   { di.Position.Offset = 0 }

// AlterTable represents an ALTER TABLE statement.
// https://cloud.google.com/spanner/docs/data-definition-language#alter_table
type AlterTable struct {
	Name       string
	Alteration TableAlteration

	Position Position // position of the "ALTER" token
}

func (at *AlterTable) String() string { return fmt.Sprintf("%#v", at) }
func (*AlterTable) isDDLStmt()        {}
func (at *AlterTable) Pos() Position  { return at.Position }
func (at *AlterTable) clearOffset() {
	switch alt := at.Alteration.(type) {
	case AddColumn:
		alt.Def.clearOffset()
		at.Alteration = alt
	case AddConstraint:
		alt.Constraint.clearOffset()
		at.Alteration = alt
	case AlterColumn:
		alt.Def.clearOffset()
		at.Alteration = alt
	}
	at.Position.Offset = 0
}

// TableAlteration is satisfied by AddColumn, DropColumn and SetOnDelete.
type TableAlteration interface {
	isTableAlteration()
	SQL() string
}

func (AddColumn) isTableAlteration()      {}
func (DropColumn) isTableAlteration()     {}
func (AddConstraint) isTableAlteration()  {}
func (DropConstraint) isTableAlteration() {}
func (SetOnDelete) isTableAlteration()    {}
func (AlterColumn) isTableAlteration()    {}

type AddColumn struct{ Def ColumnDef }
type DropColumn struct{ Name string }
type AddConstraint struct{ Constraint TableConstraint }
type DropConstraint struct{ Name string }
type SetOnDelete struct{ Action OnDelete }
type AlterColumn struct{ Def ColumnDef }

type OnDelete int

const (
	NoActionOnDelete OnDelete = iota
	CascadeOnDelete
)

// Delete represents a DELETE statement.
// https://cloud.google.com/spanner/docs/dml-syntax#delete-statement
type Delete struct {
	Table string
	Where BoolExpr

	// TODO: Alias
}

func (d *Delete) String() string { return fmt.Sprintf("%#v", d) }
func (*Delete) isDMLStmt()       {}

// TODO: Insert, Update.

// ColumnDef represents a column definition as part of a CREATE TABLE
// or ALTER TABLE statement.
type ColumnDef struct {
	Name    string
	Type    Type
	NotNull bool

	// AllowCommitTimestamp represents a column OPTIONS.
	// `true` if query is `OPTIONS (allow_commit_timestamp = true)`
	// `false` if query is `OPTIONS (allow_commit_timestamp = null)`
	// `nil` if there are no OPTIONS
	AllowCommitTimestamp *bool

	Position Position // position of the column name
}

func (cd ColumnDef) Pos() Position { return cd.Position }
func (cd *ColumnDef) clearOffset() { cd.Position.Offset = 0 }

// ForeignKey represents a foreign key definition as part of a CREATE TABLE
// or ALTER TABLE statement.
type ForeignKey struct {
	Columns    []string
	RefTable   string
	RefColumns []string

	Position Position // position of the "FOREIGN" token
}

func (fk ForeignKey) Pos() Position { return fk.Position }
func (fk *ForeignKey) clearOffset() { fk.Position.Offset = 0 }

// Type represents a column type.
type Type struct {
	Array bool
	Base  TypeBase // Bool, Int64, Float64, String, Bytes, Date, Timestamp
	Len   int64    // if Base is String or Bytes; may be MaxLen
}

// MaxLen is a sentinel for Type's Len field, representing the MAX value.
const MaxLen = math.MaxInt64

type TypeBase int

const (
	Bool TypeBase = iota
	Int64
	Float64
	String
	Bytes
	Date
	Timestamp
)

// KeyPart represents a column specification as part of a primary key or index definition.
type KeyPart struct {
	Column string
	Desc   bool
}

// Query represents a query statement.
// https://cloud.google.com/spanner/docs/query-syntax#sql-syntax
type Query struct {
	Select Select
	Order  []Order

	Limit, Offset LiteralOrParam
}

// Select represents a SELECT statement.
// https://cloud.google.com/spanner/docs/query-syntax#select-list
type Select struct {
	Distinct bool
	List     []Expr
	From     []SelectFrom
	Where    BoolExpr
	GroupBy  []Expr
	// TODO: Having

	// If the SELECT list has explicit aliases ("AS alias"),
	// ListAliases will be populated 1:1 with List;
	// aliases that are present will be non-empty.
	ListAliases []string
}

type SelectFrom struct {
	// This only supports a FROM clause directly from a table.
	Table       string
	TableSample *TableSample
}

type Order struct {
	Expr Expr
	Desc bool
}

type TableSample struct {
	Method   TableSampleMethod
	Size     Expr
	SizeType TableSampleSizeType
}

type TableSampleMethod int

const (
	Bernoulli TableSampleMethod = iota
	Reservoir
)

type TableSampleSizeType int

const (
	PercentTableSample TableSampleSizeType = iota
	RowsTableSample
)

type BoolExpr interface {
	isBoolExpr()
	Expr
}

type Expr interface {
	isExpr()
	SQL() string
}

// LiteralOrParam is implemented by integer literal and parameter values.
type LiteralOrParam interface {
	isLiteralOrParam()
	SQL() string
}

type ArithOp struct {
	Op       ArithOperator
	LHS, RHS Expr // only RHS is set for Neg, BitNot
}

func (ArithOp) isExpr() {}

type ArithOperator int

const (
	Neg    ArithOperator = iota // unary -
	BitNot                      // unary ~
	Mul                         // *
	Div                         // /
	Concat                      // ||
	Add                         // +
	Sub                         // -
	BitShl                      // <<
	BitShr                      // >>
	BitAnd                      // &
	BitXor                      // ^
	BitOr                       // |
)

type LogicalOp struct {
	Op       LogicalOperator
	LHS, RHS BoolExpr // only RHS is set for Neg, BitNot
}

func (LogicalOp) isBoolExpr() {}
func (LogicalOp) isExpr()     {}

type LogicalOperator int

const (
	And LogicalOperator = iota
	Or
	Not
)

type ComparisonOp struct {
	LHS, RHS Expr
	Op       ComparisonOperator

	// RHS2 is the third operand for BETWEEN.
	// "<LHS> BETWEEN <RHS> AND <RHS2>".
	RHS2 Expr
}

func (ComparisonOp) isBoolExpr() {}
func (ComparisonOp) isExpr()     {}

type ComparisonOperator int

const (
	Lt ComparisonOperator = iota
	Le
	Gt
	Ge
	Eq
	Ne // both "!=" and "<>"
	Like
	NotLike
	Between
	NotBetween
)

type IsOp struct {
	LHS Expr
	Neg bool
	RHS IsExpr
}

func (IsOp) isBoolExpr() {}
func (IsOp) isExpr()     {}

type IsExpr interface {
	isIsExpr()
	isExpr()
	SQL() string
}

// Func represents a function call.
type Func struct {
	Name string
	Args []Expr

	// TODO: various functions permit as-expressions, which might warrant different types in here.
}

func (Func) isBoolExpr() {} // possibly bool
func (Func) isExpr()     {}

// Paren represents a parenthesised expression.
type Paren struct {
	Expr Expr
}

func (Paren) isBoolExpr() {} // possibly bool
func (Paren) isExpr()     {}

// ID represents an identifier.
type ID string

func (ID) isBoolExpr() {} // possibly bool
func (ID) isExpr()     {}

// Param represents a query parameter.
type Param string

func (Param) isBoolExpr()       {} // possibly bool
func (Param) isExpr()           {}
func (Param) isLiteralOrParam() {}

type BoolLiteral bool

const (
	True  = BoolLiteral(true)
	False = BoolLiteral(false)
)

func (BoolLiteral) isBoolExpr() {}
func (BoolLiteral) isIsExpr()   {}
func (BoolLiteral) isExpr()     {}

type NullLiteral int

const Null = NullLiteral(0)

func (NullLiteral) isIsExpr() {}
func (NullLiteral) isExpr()   {}

// IntegerLiteral represents an integer literal.
// https://cloud.google.com/spanner/docs/lexical#integer-literals
type IntegerLiteral int64

func (IntegerLiteral) isLiteralOrParam() {}
func (IntegerLiteral) isExpr()           {}

// FloatLiteral represents a floating point literal.
// https://cloud.google.com/spanner/docs/lexical#floating-point-literals
type FloatLiteral float64

func (FloatLiteral) isExpr() {}

// StringLiteral represents a string literal.
// https://cloud.google.com/spanner/docs/lexical#string-and-bytes-literals
type StringLiteral string

func (StringLiteral) isExpr() {}

// BytesLiteral represents a bytes literal.
// https://cloud.google.com/spanner/docs/lexical#string-and-bytes-literals
type BytesLiteral string

func (BytesLiteral) isExpr() {}

type StarExpr int

// Star represents a "*" in an expression.
const Star = StarExpr(0)

func (StarExpr) isExpr() {}

// DDL
// https://cloud.google.com/spanner/docs/data-definition-language#ddl_syntax

// DDL represents a Data Definition Language (DDL) file.
type DDL struct {
	List []DDLStmt

	Filename string // if known at parse time

	Comments []*Comment // all comments, sorted by position
}

func (d *DDL) clearOffset() {
	for _, stmt := range d.List {
		stmt.clearOffset()
	}
	for _, c := range d.Comments {
		c.clearOffset()
	}
}

// DDLStmt is satisfied by a type that can appear in a DDL.
type DDLStmt interface {
	isDDLStmt()
	clearOffset()
	SQL() string
	Node
}

// DMLStmt is satisfied by a type that is a DML statement.
type DMLStmt interface {
	isDMLStmt()
	SQL() string
}

// Comment represents a comment.
type Comment struct {
	Marker   string // Opening marker; one of "#", "--", "/*".
	Isolated bool   // Whether this comment is on its own line.
	// Start and End are the position of the opening and terminating marker.
	Start, End Position
	Text       []string
}

func (c *Comment) String() string { return fmt.Sprintf("%#v", c) }
func (c *Comment) Pos() Position  { return c.Start }
func (c *Comment) clearOffset()   { c.Start.Offset, c.End.Offset = 0, 0 }

// Node is implemented by concrete types in this package that represent things
// appearing in a DDL file.
type Node interface {
	Pos() Position
	// clearOffset() is not included here because some types like ColumnDef
	// have the method on their pointer type rather than their natural value type.
	// This method is only invoked from within this package, so it isn't
	// important to enforce such things.
}

// Position describes a source position in an input DDL file.
// It is only valid if the line number is positive.
type Position struct {
	Line   int // 1-based line number
	Offset int // 0-based byte offset
}

func (pos Position) IsValid() bool { return pos.Line > 0 }
func (pos Position) String() string {
	if pos.Line == 0 {
		return ":<invalid>"
	}
	return fmt.Sprintf(":%d", pos.Line)
}

// LeadingComment returns the comment that immediately precedes a node,
// or nil if there's no such comment.
func (ddl *DDL) LeadingComment(n Node) *Comment {
	// Get the comment whose End position is on the previous line.
	lineEnd := n.Pos().Line - 1
	ci := sort.Search(len(ddl.Comments), func(i int) bool {
		return ddl.Comments[i].End.Line >= lineEnd
	})
	if ci >= len(ddl.Comments) || ddl.Comments[ci].End.Line != lineEnd {
		return nil
	}
	if !ddl.Comments[ci].Isolated {
		// This is an inline comment for a previous node.
		return nil
	}
	return ddl.Comments[ci]
}

// InlineComment returns the comment on the same line as a node,
// or nil if there's no inline comment.
// The returned comment is guaranteed to be a single line.
func (ddl *DDL) InlineComment(n Node) *Comment {
	// TODO: Do we care about comments like this?
	// 	string name = 1; /* foo
	// 	bar */

	pos := n.Pos()
	ci := sort.Search(len(ddl.Comments), func(i int) bool {
		return ddl.Comments[i].Start.Line >= pos.Line
	})
	if ci >= len(ddl.Comments) {
		return nil
	}
	c := ddl.Comments[ci]
	if c.Start.Line != pos.Line {
		return nil
	}
	if c.Start.Line != c.End.Line || len(c.Text) != 1 {
		// Multi-line comment; don't return it.
		return nil
	}
	return c
}
