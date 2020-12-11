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

// This file holds SQL methods for rendering the types in types.go
// as the SQL dialect that this package parses.
//
// Every exported type has an SQL method that returns a string.
// Some also have an addSQL method that efficiently builds that string
// in a provided strings.Builder.

import (
	"fmt"
	"strconv"
	"strings"
)

func buildSQL(x interface{ addSQL(*strings.Builder) }) string {
	var sb strings.Builder
	x.addSQL(&sb)
	return sb.String()
}

func (ct CreateTable) SQL() string {
	str := "CREATE TABLE " + ct.Name.SQL() + " (\n"
	for _, c := range ct.Columns {
		str += "  " + c.SQL() + ",\n"
	}
	for _, tc := range ct.Constraints {
		str += "  " + tc.SQL() + ",\n"
	}
	str += ") PRIMARY KEY("
	for i, c := range ct.PrimaryKey {
		if i > 0 {
			str += ", "
		}
		str += c.SQL()
	}
	str += ")"
	if il := ct.Interleave; il != nil {
		str += ",\n  INTERLEAVE IN PARENT " + il.Parent.SQL() + " ON DELETE " + il.OnDelete.SQL()
	}
	return str
}

func (ci CreateIndex) SQL() string {
	str := "CREATE"
	if ci.Unique {
		str += " UNIQUE"
	}
	if ci.NullFiltered {
		str += " NULL_FILTERED"
	}
	str += " INDEX " + ci.Name.SQL() + " ON " + ci.Table.SQL() + "("
	for i, c := range ci.Columns {
		if i > 0 {
			str += ", "
		}
		str += c.SQL()
	}
	str += ")"
	if len(ci.Storing) > 0 {
		str += " STORING (" + idList(ci.Storing, ", ") + ")"
	}
	if ci.Interleave != "" {
		str += ", INTERLEAVE IN " + ci.Interleave.SQL()
	}
	return str
}

func (dt DropTable) SQL() string {
	return "DROP TABLE " + dt.Name.SQL()
}

func (di DropIndex) SQL() string {
	return "DROP INDEX " + di.Name.SQL()
}

func (at AlterTable) SQL() string {
	return "ALTER TABLE " + at.Name.SQL() + " " + at.Alteration.SQL()
}

func (ac AddColumn) SQL() string {
	return "ADD COLUMN " + ac.Def.SQL()
}

func (dc DropColumn) SQL() string {
	return "DROP COLUMN " + dc.Name.SQL()
}

func (ac AddConstraint) SQL() string {
	return "ADD " + ac.Constraint.SQL()
}

func (dc DropConstraint) SQL() string {
	return "DROP CONSTRAINT " + dc.Name.SQL()
}

func (sod SetOnDelete) SQL() string {
	return "SET ON DELETE " + sod.Action.SQL()
}

func (od OnDelete) SQL() string {
	switch od {
	case NoActionOnDelete:
		return "NO ACTION"
	case CascadeOnDelete:
		return "CASCADE"
	}
	panic("unknown OnDelete")
}

func (ac AlterColumn) SQL() string {
	return "ALTER COLUMN " + ac.Name.SQL() + " " + ac.Alteration.SQL()
}

func (sct SetColumnType) SQL() string {
	str := sct.Type.SQL()
	if sct.NotNull {
		str += " NOT NULL"
	}
	return str
}

func (sco SetColumnOptions) SQL() string {
	// TODO: not clear what to do for no options.
	return "SET " + sco.Options.SQL()
}

func (co ColumnOptions) SQL() string {
	str := "OPTIONS ("
	if co.AllowCommitTimestamp != nil {
		if *co.AllowCommitTimestamp {
			str += "allow_commit_timestamp = true"
		} else {
			str += "allow_commit_timestamp = null"
		}
	}
	str += ")"
	return str
}

func (d *Delete) SQL() string {
	return "DELETE FROM " + d.Table.SQL() + " WHERE " + d.Where.SQL()
}

func (u *Update) SQL() string {
	str := "UPDATE " + u.Table.SQL() + " SET "
	for i, item := range u.Items {
		if i > 0 {
			str += ", "
		}
		str += item.Column.SQL() + " = "
		if item.Value != nil {
			str += item.Value.SQL()
		} else {
			str += "DEFAULT"
		}
	}
	str += " WHERE " + u.Where.SQL()
	return str
}

func (cd ColumnDef) SQL() string {
	str := cd.Name.SQL() + " " + cd.Type.SQL()
	if cd.NotNull {
		str += " NOT NULL"
	}
	if cd.Generated != nil {
		str += " AS (" + cd.Generated.SQL() + ") STORED"
	}
	if cd.Options != (ColumnOptions{}) {
		str += " " + cd.Options.SQL()
	}
	return str
}

func (tc TableConstraint) SQL() string {
	var str string
	if tc.Name != "" {
		str += "CONSTRAINT " + tc.Name.SQL() + " "
	}
	str += tc.Constraint.SQL()
	return str
}

func (fk ForeignKey) SQL() string {
	str := "FOREIGN KEY (" + idList(fk.Columns, ", ")
	str += ") REFERENCES " + fk.RefTable.SQL() + " ("
	str += idList(fk.RefColumns, ", ") + ")"
	return str
}

func (c Check) SQL() string {
	return "CHECK (" + c.Expr.SQL() + ")"
}

func (t Type) SQL() string {
	str := t.Base.SQL()
	if t.Base == String || t.Base == Bytes {
		str += "("
		if t.Len == MaxLen {
			str += "MAX"
		} else {
			str += strconv.FormatInt(t.Len, 10)
		}
		str += ")"
	}
	if t.Array {
		str = "ARRAY<" + str + ">"
	}
	return str
}

func (tb TypeBase) SQL() string {
	switch tb {
	case Bool:
		return "BOOL"
	case Int64:
		return "INT64"
	case Float64:
		return "FLOAT64"
	case Numeric:
		return "NUMERIC"
	case String:
		return "STRING"
	case Bytes:
		return "BYTES"
	case Date:
		return "DATE"
	case Timestamp:
		return "TIMESTAMP"
	}
	panic("unknown TypeBase")
}

func (kp KeyPart) SQL() string {
	str := kp.Column.SQL()
	if kp.Desc {
		str += " DESC"
	}
	return str
}

func (q Query) SQL() string { return buildSQL(q) }
func (q Query) addSQL(sb *strings.Builder) {
	q.Select.addSQL(sb)
	if len(q.Order) > 0 {
		sb.WriteString(" ORDER BY ")
		for i, o := range q.Order {
			if i > 0 {
				sb.WriteString(", ")
			}
			o.addSQL(sb)
		}
	}
	if q.Limit != nil {
		sb.WriteString(" LIMIT ")
		sb.WriteString(q.Limit.SQL())
		if q.Offset != nil {
			sb.WriteString(" OFFSET ")
			sb.WriteString(q.Offset.SQL())
		}
	}
}

func (sel Select) SQL() string { return buildSQL(sel) }
func (sel Select) addSQL(sb *strings.Builder) {
	sb.WriteString("SELECT ")
	if sel.Distinct {
		sb.WriteString("DISTINCT ")
	}
	for i, e := range sel.List {
		if i > 0 {
			sb.WriteString(", ")
		}
		e.addSQL(sb)
		if len(sel.ListAliases) > 0 {
			alias := sel.ListAliases[i]
			if alias != "" {
				sb.WriteString(" AS ")
				sb.WriteString(alias.SQL())
			}
		}
	}
	if len(sel.From) > 0 {
		sb.WriteString(" FROM ")
		for i, f := range sel.From {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(f.SQL())
		}
	}
	if sel.Where != nil {
		sb.WriteString(" WHERE ")
		sel.Where.addSQL(sb)
	}
	if len(sel.GroupBy) > 0 {
		sb.WriteString(" GROUP BY ")
		addExprList(sb, sel.GroupBy, ", ")
	}
}

func (sft SelectFromTable) SQL() string {
	str := sft.Table.SQL()
	if sft.Alias != "" {
		str += " AS " + sft.Alias.SQL()
	}
	return str
}

func (sfj SelectFromJoin) SQL() string {
	// TODO: The grammar permits arbitrary nesting. Does this need to add parens?
	str := sfj.LHS.SQL() + " " + joinTypes[sfj.Type] + " JOIN "
	// TODO: hints go here
	str += sfj.RHS.SQL()
	if sfj.On != nil {
		str += " " + sfj.On.SQL()
	} else if len(sfj.Using) > 0 {
		str += " USING (" + idList(sfj.Using, ", ") + ")"
	}
	return str
}

var joinTypes = map[JoinType]string{
	InnerJoin: "INNER",
	CrossJoin: "CROSS",
	FullJoin:  "FULL",
	LeftJoin:  "LEFT",
	RightJoin: "RIGHT",
}

func (sfu SelectFromUnnest) SQL() string {
	str := "UNNEST(" + sfu.Expr.SQL() + ")"
	if sfu.Alias != "" {
		str += " AS " + sfu.Alias.SQL()
	}
	return str
}

func (o Order) SQL() string { return buildSQL(o) }
func (o Order) addSQL(sb *strings.Builder) {
	o.Expr.addSQL(sb)
	if o.Desc {
		sb.WriteString(" DESC")
	}
}

var arithOps = map[ArithOperator]string{
	// Binary operators only; unary operators are handled first.
	Mul:    "*",
	Div:    "/",
	Concat: "||",
	Add:    "+",
	Sub:    "-",
	BitShl: "<<",
	BitShr: ">>",
	BitAnd: "&",
	BitXor: "^",
	BitOr:  "|",
}

func (ao ArithOp) SQL() string { return buildSQL(ao) }
func (ao ArithOp) addSQL(sb *strings.Builder) {
	// Extra parens inserted to ensure the correct precedence.

	switch ao.Op {
	case Neg:
		sb.WriteString("-(")
		ao.RHS.addSQL(sb)
		sb.WriteString(")")
		return
	case Plus:
		sb.WriteString("+(")
		ao.RHS.addSQL(sb)
		sb.WriteString(")")
		return
	case BitNot:
		sb.WriteString("~(")
		ao.RHS.addSQL(sb)
		sb.WriteString(")")
		return
	}
	op, ok := arithOps[ao.Op]
	if !ok {
		panic("unknown ArithOp")
	}
	sb.WriteString("(")
	ao.LHS.addSQL(sb)
	sb.WriteString(")")
	sb.WriteString(op)
	sb.WriteString("(")
	ao.RHS.addSQL(sb)
	sb.WriteString(")")
}

func (lo LogicalOp) SQL() string { return buildSQL(lo) }
func (lo LogicalOp) addSQL(sb *strings.Builder) {
	switch lo.Op {
	default:
		panic("unknown LogicalOp")
	case And:
		lo.LHS.addSQL(sb)
		sb.WriteString(" AND ")
	case Or:
		lo.LHS.addSQL(sb)
		sb.WriteString(" OR ")
	case Not:
		sb.WriteString("NOT ")
	}
	lo.RHS.addSQL(sb)
}

var compOps = map[ComparisonOperator]string{
	Lt:         "<",
	Le:         "<=",
	Gt:         ">",
	Ge:         ">=",
	Eq:         "=",
	Ne:         "!=",
	Like:       "LIKE",
	NotLike:    "NOT LIKE",
	Between:    "BETWEEN",
	NotBetween: "NOT BETWEEN",
}

func (co ComparisonOp) SQL() string { return buildSQL(co) }
func (co ComparisonOp) addSQL(sb *strings.Builder) {
	op, ok := compOps[co.Op]
	if !ok {
		panic("unknown ComparisonOp")
	}
	co.LHS.addSQL(sb)
	sb.WriteString(" ")
	sb.WriteString(op)
	sb.WriteString(" ")
	co.RHS.addSQL(sb)
	if co.Op == Between || co.Op == NotBetween {
		sb.WriteString(" AND ")
		co.RHS2.addSQL(sb)
	}
}

func (io InOp) SQL() string { return buildSQL(io) }
func (io InOp) addSQL(sb *strings.Builder) {
	io.LHS.addSQL(sb)
	if io.Neg {
		sb.WriteString(" NOT")
	}
	sb.WriteString(" IN ")
	if io.Unnest {
		sb.WriteString("UNNEST")
	}
	sb.WriteString("(")
	addExprList(sb, io.RHS, ", ")
	sb.WriteString(")")
}

func (io IsOp) SQL() string { return buildSQL(io) }
func (io IsOp) addSQL(sb *strings.Builder) {
	io.LHS.addSQL(sb)
	sb.WriteString(" IS ")
	if io.Neg {
		sb.WriteString("NOT ")
	}
	io.RHS.addSQL(sb)
}

func (f Func) SQL() string { return buildSQL(f) }
func (f Func) addSQL(sb *strings.Builder) {
	sb.WriteString(f.Name)
	sb.WriteString("(")
	addExprList(sb, f.Args, ", ")
	sb.WriteString(")")
}

func idList(l []ID, join string) string {
	var ss []string
	for _, s := range l {
		ss = append(ss, s.SQL())
	}
	return strings.Join(ss, join)
}

func addExprList(sb *strings.Builder, l []Expr, join string) {
	for i, s := range l {
		if i > 0 {
			sb.WriteString(join)
		}
		s.addSQL(sb)
	}
}

func addIDList(sb *strings.Builder, l []ID, join string) {
	for i, s := range l {
		if i > 0 {
			sb.WriteString(join)
		}
		s.addSQL(sb)
	}
}

func (pe PathExp) SQL() string { return buildSQL(pe) }
func (pe PathExp) addSQL(sb *strings.Builder) {
	addIDList(sb, []ID(pe), ".")
}

func (p Paren) SQL() string { return buildSQL(p) }
func (p Paren) addSQL(sb *strings.Builder) {
	sb.WriteString("(")
	p.Expr.addSQL(sb)
	sb.WriteString(")")
}

func (a Array) SQL() string { return buildSQL(a) }
func (a Array) addSQL(sb *strings.Builder) {
	sb.WriteString("[")
	addExprList(sb, []Expr(a), ", ")
	sb.WriteString("]")
}

func (id ID) SQL() string { return buildSQL(id) }
func (id ID) addSQL(sb *strings.Builder) {
	// https://cloud.google.com/spanner/docs/lexical#identifiers

	// TODO: If there are non-letters/numbers/underscores then this also needs quoting.

	if IsKeyword(string(id)) {
		// TODO: Escaping may be needed here.
		sb.WriteString("`")
		sb.WriteString(string(id))
		sb.WriteString("`")
		return
	}

	sb.WriteString(string(id))
}

func (p Param) SQL() string { return buildSQL(p) }
func (p Param) addSQL(sb *strings.Builder) {
	sb.WriteString("@")
	sb.WriteString(string(p))
}

func (b BoolLiteral) SQL() string { return buildSQL(b) }
func (b BoolLiteral) addSQL(sb *strings.Builder) {
	if b {
		sb.WriteString("TRUE")
	} else {
		sb.WriteString("FALSE")
	}
}

func (NullLiteral) SQL() string                { return buildSQL(NullLiteral(0)) }
func (NullLiteral) addSQL(sb *strings.Builder) { sb.WriteString("NULL") }

func (StarExpr) SQL() string                { return buildSQL(StarExpr(0)) }
func (StarExpr) addSQL(sb *strings.Builder) { sb.WriteString("*") }

func (il IntegerLiteral) SQL() string                { return buildSQL(il) }
func (il IntegerLiteral) addSQL(sb *strings.Builder) { fmt.Fprintf(sb, "%d", il) }

func (fl FloatLiteral) SQL() string                { return buildSQL(fl) }
func (fl FloatLiteral) addSQL(sb *strings.Builder) { fmt.Fprintf(sb, "%g", fl) }

// TODO: provide correct string quote method and use it.

func (sl StringLiteral) SQL() string                { return buildSQL(sl) }
func (sl StringLiteral) addSQL(sb *strings.Builder) { fmt.Fprintf(sb, "%q", sl) }

func (bl BytesLiteral) SQL() string                { return buildSQL(bl) }
func (bl BytesLiteral) addSQL(sb *strings.Builder) { fmt.Fprintf(sb, "B%q", bl) }
