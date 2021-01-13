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

package spannertest

// This file contains the part of the Spanner fake that evaluates expressions.

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner/spansql"
)

// evalContext represents the context for evaluating an expression.
type evalContext struct {
	// cols and row are set during expr evaluation.
	cols []colInfo
	row  row

	// If there are visible aliases, they are populated here.
	aliases map[spansql.ID]spansql.Expr

	params queryParams
}

// coercedValue represents a literal value that has been coerced to a different type.
// This never leaves this package, nor is persisted.
type coercedValue struct {
	spansql.Expr             // not a real Expr
	val          interface{} // internal representation
	// TODO: type?
	orig spansql.Expr
}

func (cv coercedValue) SQL() string { return cv.orig.SQL() }

func (ec evalContext) evalExprList(list []spansql.Expr) ([]interface{}, error) {
	var out []interface{}
	for _, e := range list {
		x, err := ec.evalExpr(e)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, nil
}

func (ec evalContext) evalBoolExpr(be spansql.BoolExpr) (*bool, error) {
	switch be := be.(type) {
	default:
		return nil, fmt.Errorf("unhandled BoolExpr %T", be)
	case spansql.BoolLiteral:
		b := bool(be)
		return &b, nil
	case spansql.ID, spansql.Param, spansql.Paren, spansql.InOp: // InOp is a bit weird.
		e, err := ec.evalExpr(be)
		if err != nil {
			return nil, err
		}
		if e == nil {
			return nil, nil // preserve NULLs
		}
		b, ok := e.(bool)
		if !ok {
			return nil, fmt.Errorf("got %T, want bool", e)
		}
		return &b, nil
	case spansql.LogicalOp:
		var lhs, rhs *bool
		var err error
		if be.LHS != nil {
			lhs, err = ec.evalBoolExpr(be.LHS)
			if err != nil {
				return nil, err
			}
		}
		rhs, err = ec.evalBoolExpr(be.RHS)
		if err != nil {
			return nil, err
		}
		// https://cloud.google.com/spanner/docs/operators#logical_operators
		switch be.Op {
		case spansql.And:
			if lhs != nil {
				if *lhs {
					// TRUE AND x => x
					return rhs, nil
				}
				// FALSE AND x => FALSE
				return lhs, nil
			}
			// NULL AND FALSE => FALSE
			if rhs != nil && !*rhs {
				return rhs, nil
			}
			// NULL AND TRUE|NULL => NULL
			return nil, nil
		case spansql.Or:
			if lhs != nil {
				if *lhs {
					// TRUE OR x => TRUE
					return lhs, nil
				}
				// FALSE OR x => x
				return rhs, nil
			}
			// NULL OR TRUE => TRUE
			if rhs != nil && *rhs {
				return rhs, nil
			}
			// NULL OR FALSE|NULL => NULL
			return nil, nil
		case spansql.Not:
			if rhs == nil {
				return nil, nil
			}
			b := !*rhs
			return &b, nil
		default:
			return nil, fmt.Errorf("unhandled LogicalOp %d", be.Op)
		}
	case spansql.ComparisonOp:
		// Per https://cloud.google.com/spanner/docs/operators#comparison_operators,
		// "Cloud Spanner SQL will generally coerce literals to the type of non-literals, where present".
		// Before evaluating be.LHS and be.RHS, do any necessary coercion.
		be, err := ec.coerceComparisonOpArgs(be)
		if err != nil {
			return nil, err
		}

		lhs, err := ec.evalExpr(be.LHS)
		if err != nil {
			return nil, err
		}
		rhs, err := ec.evalExpr(be.RHS)
		if err != nil {
			return nil, err
		}
		if lhs == nil || rhs == nil {
			// https://cloud.google.com/spanner/docs/operators#comparison_operators says
			// "any operation with a NULL input returns NULL."
			return nil, nil
		}
		var b bool
		switch be.Op {
		default:
			return nil, fmt.Errorf("TODO: ComparisonOp %d", be.Op)
		case spansql.Lt:
			b = compareVals(lhs, rhs) < 0
		case spansql.Le:
			b = compareVals(lhs, rhs) <= 0
		case spansql.Gt:
			b = compareVals(lhs, rhs) > 0
		case spansql.Ge:
			b = compareVals(lhs, rhs) >= 0
		case spansql.Eq:
			b = compareVals(lhs, rhs) == 0
		case spansql.Ne:
			b = compareVals(lhs, rhs) != 0
		case spansql.Like, spansql.NotLike:
			left, ok := lhs.(string)
			if !ok {
				// TODO: byte works here too?
				return nil, fmt.Errorf("LHS of LIKE is %T, not string", lhs)
			}
			right, ok := rhs.(string)
			if !ok {
				// TODO: byte works here too?
				return nil, fmt.Errorf("RHS of LIKE is %T, not string", rhs)
			}

			b = evalLike(left, right)
			if be.Op == spansql.NotLike {
				b = !b
			}
		case spansql.Between, spansql.NotBetween:
			rhs2, err := ec.evalExpr(be.RHS2)
			if err != nil {
				return nil, err
			}
			b = compareVals(rhs, lhs) <= 0 && compareVals(lhs, rhs2) <= 0
			if be.Op == spansql.NotBetween {
				b = !b
			}
		}
		return &b, nil
	case spansql.IsOp:
		lhs, err := ec.evalExpr(be.LHS)
		if err != nil {
			return nil, err
		}
		var b bool
		switch rhs := be.RHS.(type) {
		default:
			return nil, fmt.Errorf("unhandled IsOp %T", rhs)
		case spansql.BoolLiteral:
			if lhs == nil {
				// For `X IS TRUE`, X being NULL is okay, and this evaluates
				// to false. Same goes for `X IS FALSE`.
				lhs = !bool(rhs)
			}
			lhsBool, ok := lhs.(bool)
			if !ok {
				return nil, fmt.Errorf("non-bool value %T on LHS for %s", lhs, be.SQL())
			}
			b = (lhsBool == bool(rhs))
		case spansql.NullLiteral:
			b = (lhs == nil)
		}
		if be.Neg {
			b = !b
		}
		return &b, nil
	}
}

func (ec evalContext) evalArithOp(e spansql.ArithOp) (interface{}, error) {
	switch e.Op {
	case spansql.Neg:
		rhs, err := ec.evalExpr(e.RHS)
		if err != nil {
			return nil, err
		}
		switch rhs := rhs.(type) {
		case float64:
			return -rhs, nil
		case int64:
			return -rhs, nil
		}
		return nil, fmt.Errorf("RHS of %s evaluates to %T, want FLOAT64 or INT64", e.SQL(), rhs)
	case spansql.BitNot:
		rhs, err := ec.evalExpr(e.RHS)
		if err != nil {
			return nil, err
		}
		switch rhs := rhs.(type) {
		case int64:
			return ^rhs, nil
		case []byte:
			b := append([]byte(nil), rhs...) // deep copy
			for i := range b {
				b[i] = ^b[i]
			}
			return b, nil
		}
		return nil, fmt.Errorf("RHS of %s evaluates to %T, want INT64 or BYTES", e.SQL(), rhs)
	case spansql.Div:
		lhs, err := ec.evalFloat64(e.LHS)
		if err != nil {
			return nil, err
		}
		rhs, err := ec.evalFloat64(e.RHS)
		if err != nil {
			return nil, err
		}
		if rhs == 0 {
			// TODO: Does real Spanner use a specific error code here?
			return nil, fmt.Errorf("divide by zero")
		}
		return lhs / rhs, nil
	case spansql.Add, spansql.Sub, spansql.Mul:
		lhs, err := ec.evalExpr(e.LHS)
		if err != nil {
			return nil, err
		}
		rhs, err := ec.evalExpr(e.RHS)
		if err != nil {
			return nil, err
		}
		i1, ok1 := lhs.(int64)
		i2, ok2 := rhs.(int64)
		if ok1 && ok2 {
			switch e.Op {
			case spansql.Add:
				return i1 + i2, nil
			case spansql.Sub:
				return i1 - i2, nil
			case spansql.Mul:
				return i1 * i2, nil
			}
		}
		f1, err := asFloat64(e.LHS, lhs)
		if err != nil {
			return nil, err
		}
		f2, err := asFloat64(e.RHS, rhs)
		if err != nil {
			return nil, err
		}
		switch e.Op {
		case spansql.Add:
			return f1 + f2, nil
		case spansql.Sub:
			return f1 - f2, nil
		case spansql.Mul:
			return f1 * f2, nil
		}
	case spansql.BitAnd, spansql.BitXor, spansql.BitOr:
		lhs, err := ec.evalExpr(e.LHS)
		if err != nil {
			return nil, err
		}
		rhs, err := ec.evalExpr(e.RHS)
		if err != nil {
			return nil, err
		}
		i1, ok1 := lhs.(int64)
		i2, ok2 := rhs.(int64)
		if ok1 && ok2 {
			switch e.Op {
			case spansql.BitAnd:
				return i1 & i2, nil
			case spansql.BitXor:
				return i1 ^ i2, nil
			case spansql.BitOr:
				return i1 | i2, nil
			}
		}
		b1, ok1 := lhs.([]byte)
		b2, ok2 := rhs.([]byte)
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("arguments of %s evaluate to (%T, %T), want (INT64, INT64) or (BYTES, BYTES)", e.SQL(), lhs, rhs)
		}
		if len(b1) != len(b2) {
			return nil, fmt.Errorf("arguments of %s evaluate to BYTES of unequal lengths (%d vs %d)", e.SQL(), len(b1), len(b2))
		}
		var f func(x, y byte) byte
		switch e.Op {
		case spansql.BitAnd:
			f = func(x, y byte) byte { return x & y }
		case spansql.BitXor:
			f = func(x, y byte) byte { return x ^ y }
		case spansql.BitOr:
			f = func(x, y byte) byte { return x | y }
		}
		b := make([]byte, len(b1))
		for i := range b1 {
			b[i] = f(b1[i], b2[i])
		}
		return b, nil
	}
	// TODO: Concat, BitShl, BitShr
	return nil, fmt.Errorf("TODO: evalArithOp(%s %v)", e.SQL(), e.Op)
}

// evalFloat64 evaluates an expression and returns its FLOAT64 value.
// If the expression does not yield a FLOAT64 or INT64 it returns an error.
func (ec evalContext) evalFloat64(e spansql.Expr) (float64, error) {
	v, err := ec.evalExpr(e)
	if err != nil {
		return 0, err
	}
	return asFloat64(e, v)
}

func asFloat64(e spansql.Expr, v interface{}) (float64, error) {
	switch v := v.(type) {
	default:
		return 0, fmt.Errorf("expression %s evaluates to %T, want FLOAT64 or INT64", e.SQL(), v)
	case float64:
		return v, nil
	case int64:
		return float64(v), nil
	}
}

func (ec evalContext) evalExpr(e spansql.Expr) (interface{}, error) {
	// Several cases below are handled by this.
	// It evaluates a BoolExpr (which returns *bool for a tri-state BOOL)
	// and converts it to true/false/nil.
	evalBool := func(be spansql.BoolExpr) (interface{}, error) {
		b, err := ec.evalBoolExpr(be)
		if err != nil {
			return nil, err
		}
		if b == nil {
			return nil, nil // (*bool)(nil) -> interface nil
		}
		return *b, nil
	}

	switch e := e.(type) {
	default:
		return nil, fmt.Errorf("TODO: evalExpr(%s %T)", e.SQL(), e)
	case coercedValue:
		return e.val, nil
	case spansql.PathExp:
		return ec.evalPathExp(e)
	case spansql.ID:
		return ec.evalID(e)
	case spansql.Param:
		qp, ok := ec.params[string(e)]
		if !ok {
			return 0, fmt.Errorf("unbound param %s", e.SQL())
		}
		return qp.Value, nil
	case spansql.IntegerLiteral:
		return int64(e), nil
	case spansql.FloatLiteral:
		return float64(e), nil
	case spansql.StringLiteral:
		return string(e), nil
	case spansql.BytesLiteral:
		return []byte(e), nil
	case spansql.NullLiteral:
		return nil, nil
	case spansql.BoolLiteral:
		return bool(e), nil
	case spansql.Paren:
		return ec.evalExpr(e.Expr)
	case spansql.Array:
		var arr []interface{}
		for _, elt := range e {
			v, err := ec.evalExpr(elt)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		// TODO: enforce or coerce to consistent types.
		return arr, nil
	case spansql.ArithOp:
		return ec.evalArithOp(e)
	case spansql.LogicalOp:
		return evalBool(e)
	case spansql.ComparisonOp:
		return evalBool(e)
	case spansql.InOp:
		// This is implemented here in evalExpr instead of evalBoolExpr
		// because it can return FALSE/TRUE/NULL.
		// The docs are a bit confusing here, so there's probably some bugs here around NULL handling.
		// TODO: Can this now simplify using evalBool?

		if len(e.RHS) == 0 {
			// "IN with an empty right side expression is always FALSE".
			return e.Neg, nil
		}
		lhs, err := ec.evalExpr(e.LHS)
		if err != nil {
			return false, err
		}
		if lhs == nil {
			// "IN with a NULL left side expression and a non-empty right side expression is always NULL".
			return nil, nil
		}
		var b bool
		for _, rhse := range e.RHS {
			rhs, err := ec.evalExpr(rhse)
			if err != nil {
				return false, err
			}
			if !e.Unnest {
				if lhs == rhs {
					b = true
				}
			} else {
				if rhs == nil {
					// "IN UNNEST(<NULL array>) returns FALSE (not NULL)".
					return e.Neg, nil
				}
				arr, ok := rhs.([]interface{})
				if !ok {
					return nil, fmt.Errorf("UNNEST argument evaluated as %T, want array", rhs)
				}
				for _, rhs := range arr {
					// == isn't okay here.
					if compareVals(lhs, rhs) == 0 {
						b = true
					}
				}
			}
		}
		if e.Neg {
			b = !b
		}
		return b, nil
	case spansql.IsOp:
		return evalBool(e)
	case aggSentinel:
		// Match up e.AggIndex with the column.
		// They might have been reordered.
		ci := -1
		for i, col := range ec.cols {
			if col.AggIndex == e.AggIndex {
				ci = i
				break
			}
		}
		if ci < 0 {
			return 0, fmt.Errorf("internal error: did not find aggregate column %d", e.AggIndex)
		}
		return ec.row[ci], nil
	}
}

// resolveColumnIndex turns an ID or PathExp into a table column index.
func (ec evalContext) resolveColumnIndex(e spansql.Expr) (int, error) {
	switch e := e.(type) {
	case spansql.ID:
		for i, col := range ec.cols {
			if col.Name == e {
				return i, nil
			}
		}
	case spansql.PathExp:
		for i, col := range ec.cols {
			if pathExpEqual(e, col.Alias) {
				return i, nil
			}
		}
	}
	return 0, fmt.Errorf("couldn't resolve [%s] as a table column", e.SQL())
}

func (ec evalContext) evalPathExp(pe spansql.PathExp) (interface{}, error) {
	// TODO: support more than only naming an aliased table column.
	if i, err := ec.resolveColumnIndex(pe); err == nil {
		return ec.row.copyDataElem(i), nil
	}
	return nil, fmt.Errorf("couldn't resolve path expression %s", pe.SQL())
}

func (ec evalContext) evalID(id spansql.ID) (interface{}, error) {
	if i, err := ec.resolveColumnIndex(id); err == nil {
		return ec.row.copyDataElem(i), nil
	}
	if e, ok := ec.aliases[id]; ok {
		// Make a copy of the context without this alias
		// to prevent an evaluation cycle.
		innerEC := ec
		innerEC.aliases = make(map[spansql.ID]spansql.Expr)
		for alias, e := range ec.aliases {
			if alias != id {
				innerEC.aliases[alias] = e
			}
		}
		return innerEC.evalExpr(e)
	}
	return nil, fmt.Errorf("couldn't resolve identifier %s", id)
}

func (ec evalContext) coerceComparisonOpArgs(co spansql.ComparisonOp) (spansql.ComparisonOp, error) {
	// https://cloud.google.com/spanner/docs/operators#comparison_operators

	if co.RHS2 != nil {
		// TODO: Handle co.RHS2 for BETWEEN. The rules for that aren't clear.
		return co, nil
	}

	// Look for a string literal on LHS or RHS.
	var err error
	if slit, ok := co.LHS.(spansql.StringLiteral); ok {
		co.LHS, err = ec.coerceString(co.RHS, slit)
		return co, err
	}
	if slit, ok := co.RHS.(spansql.StringLiteral); ok {
		co.RHS, err = ec.coerceString(co.LHS, slit)
		return co, err
	}

	// TODO: Other coercion literals. The int64/float64 code elsewhere may be able to be simplified.

	return co, nil
}

// coerceString converts a string literal into something compatible with the target expression.
func (ec evalContext) coerceString(target spansql.Expr, slit spansql.StringLiteral) (spansql.Expr, error) {
	ci, err := ec.colInfo(target)
	if err != nil {
		return nil, err
	}
	if ci.Type.Array {
		return nil, fmt.Errorf("unable to coerce string literal %q to match array type", slit)
	}
	switch ci.Type.Base {
	case spansql.String:
		return slit, nil
	case spansql.Date:
		d, err := parseAsDate(string(slit))
		if err != nil {
			return nil, fmt.Errorf("coercing string literal %q to DATE: %v", slit, err)
		}
		return coercedValue{
			val:  d,
			orig: slit,
		}, nil
	case spansql.Timestamp:
		t, err := parseAsTimestamp(string(slit))
		if err != nil {
			return nil, fmt.Errorf("coercing string literal %q to TIMESTAMP: %v", slit, err)
		}
		return coercedValue{
			val:  t,
			orig: slit,
		}, nil
	}

	// TODO: Any others?

	return nil, fmt.Errorf("unable to coerce string literal %q to match %v", slit, ci.Type)
}

func evalLiteralOrParam(lop spansql.LiteralOrParam, params queryParams) (int64, error) {
	switch v := lop.(type) {
	case spansql.IntegerLiteral:
		return int64(v), nil
	case spansql.Param:
		return paramAsInteger(v, params)
	default:
		return 0, fmt.Errorf("LiteralOrParam with %T not supported", v)
	}
}

func paramAsInteger(p spansql.Param, params queryParams) (int64, error) {
	qp, ok := params[string(p)]
	if !ok {
		return 0, fmt.Errorf("unbound param %s", p.SQL())
	}
	switch v := qp.Value.(type) {
	default:
		return 0, fmt.Errorf("can't interpret parameter %s (%s) value of type %T as integer", p.SQL(), qp.Type.SQL(), v)
	case int64:
		return v, nil
	case string:
		x, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("bad int64 string %q: %v", v, err)
		}
		return x, nil
	}
}

// compareValLists compares pair-wise elements of a and b.
// If desc is not nil, it indicates which comparisons should be reversed.
func compareValLists(a, b []interface{}, desc []bool) int {
	for i := range a {
		cmp := compareVals(a[i], b[i])
		if cmp == 0 {
			continue
		}
		if desc != nil && desc[i] {
			cmp = -cmp
		}
		return cmp
	}
	return 0
}

func compareVals(x, y interface{}) int {
	// NULL is always the minimum possible value.
	if x == nil && y == nil {
		return 0
	} else if x == nil {
		return -1
	} else if y == nil {
		return 1
	}

	// TODO: coerce between more comparable types? factor this out for expressions other than comparisons.

	switch x := x.(type) {
	default:
		panic(fmt.Sprintf("unhandled comparison on %T", x))
	case bool:
		// false < true
		y := y.(bool)
		if !x && y {
			return -1
		} else if x && !y {
			return 1
		}
		return 0
	case int64:
		if s, ok := y.(string); ok {
			var err error
			y, err = strconv.ParseInt(s, 10, 64)
			if err != nil {
				panic(fmt.Sprintf("bad int64 string %q: %v", s, err))
			}
		}
		if f, ok := y.(float64); ok {
			// Coersion from INT64 to FLOAT64 is allowed.
			return compareVals(x, f)
		}
		y := y.(int64)
		if x < y {
			return -1
		} else if x > y {
			return 1
		}
		return 0
	case float64:
		// Coersion from INT64 to FLOAT64 is allowed.
		if i, ok := y.(int64); ok {
			y = float64(i)
		}
		y := y.(float64)
		if x < y {
			return -1
		} else if x > y {
			return 1
		}
		return 0
	case string:
		return strings.Compare(x, y.(string))
	case civil.Date:
		y := y.(civil.Date)
		if x.Before(y) {
			return -1
		} else if x.After(y) {
			return 1
		}
		return 0
	case time.Time:
		y := y.(time.Time)
		if x.Before(y) {
			return -1
		} else if x.After(y) {
			return 1
		}
		return 0
	case []byte:
		return bytes.Compare(x, y.([]byte))
	}
}

var (
	boolType    = spansql.Type{Base: spansql.Bool}
	int64Type   = spansql.Type{Base: spansql.Int64}
	float64Type = spansql.Type{Base: spansql.Float64}
	stringType  = spansql.Type{Base: spansql.String}
)

func (ec evalContext) colInfo(e spansql.Expr) (colInfo, error) {
	// TODO: more types
	switch e := e.(type) {
	case spansql.BoolLiteral:
		return colInfo{Type: boolType}, nil
	case spansql.IntegerLiteral:
		return colInfo{Type: int64Type}, nil
	case spansql.StringLiteral:
		return colInfo{Type: stringType}, nil
	case spansql.BytesLiteral:
		return colInfo{Type: spansql.Type{Base: spansql.Bytes}}, nil
	case spansql.ArithOp:
		t, err := ec.arithColType(e)
		if err != nil {
			return colInfo{}, err
		}
		return colInfo{Type: t}, nil
	case spansql.LogicalOp, spansql.ComparisonOp, spansql.IsOp:
		return colInfo{Type: spansql.Type{Base: spansql.Bool}}, nil
	case spansql.PathExp, spansql.ID:
		// TODO: support more than only naming a table column.
		i, err := ec.resolveColumnIndex(e)
		if err == nil {
			return ec.cols[i], nil
		}
		// Let errors fall through.
	case spansql.Param:
		qp, ok := ec.params[string(e)]
		if !ok {
			return colInfo{}, fmt.Errorf("unbound param %s", e.SQL())
		}
		return colInfo{Type: qp.Type}, nil
	case spansql.Paren:
		return ec.colInfo(e.Expr)
	case spansql.Array:
		// Assume all element of an array literal have the same type.
		if len(e) == 0 {
			// TODO: What does the real Spanner do here?
			return colInfo{Type: spansql.Type{Base: spansql.Int64, Array: true}}, nil
		}
		ci, err := ec.colInfo(e[0])
		if err != nil {
			return colInfo{}, err
		}
		if ci.Type.Array {
			return colInfo{}, fmt.Errorf("can't nest array literals")
		}
		ci.Type.Array = true
		return ci, nil
	case spansql.NullLiteral:
		// There isn't necessarily something sensible here.
		// Empirically, though, the real Spanner returns Int64.
		return colInfo{Type: int64Type}, nil
	case aggSentinel:
		return colInfo{Type: e.Type, AggIndex: e.AggIndex}, nil
	}
	return colInfo{}, fmt.Errorf("can't deduce column type from expression [%s] (type %T)", e.SQL(), e)
}

func (ec evalContext) arithColType(ao spansql.ArithOp) (spansql.Type, error) {
	// The type depends on the particular operator and the argument types.
	// https://cloud.google.com/spanner/docs/functions-and-operators#arithmetic_operators

	var lhs, rhs spansql.Type
	var err error
	if ao.LHS != nil {
		ci, err := ec.colInfo(ao.LHS)
		if err != nil {
			return spansql.Type{}, err
		}
		lhs = ci.Type
	}
	ci, err := ec.colInfo(ao.RHS)
	if err != nil {
		return spansql.Type{}, err
	}
	rhs = ci.Type

	switch ao.Op {
	default:
		return spansql.Type{}, fmt.Errorf("can't deduce column type from ArithOp [%s]", ao.SQL())
	case spansql.Neg, spansql.BitNot:
		return rhs, nil
	case spansql.Add, spansql.Sub, spansql.Mul:
		if lhs == int64Type && rhs == int64Type {
			return int64Type, nil
		}
		return float64Type, nil
	case spansql.Div:
		return float64Type, nil
	case spansql.Concat:
		if !lhs.Array {
			return stringType, nil
		}
		return lhs, nil
	case spansql.BitShl, spansql.BitShr, spansql.BitAnd, spansql.BitXor, spansql.BitOr:
		// "All bitwise operators return the same type and the same length as the first operand."
		return lhs, nil
	}
}

func pathExpEqual(a, b spansql.PathExp) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func evalLike(str, pat string) bool {
	/*
		% matches any number of chars.
		_ matches a single char.
		TODO: handle escaping
	*/

	// Lean on regexp for simplicity.
	pat = regexp.QuoteMeta(pat)
	pat = strings.Replace(pat, "%", ".*", -1)
	pat = strings.Replace(pat, "_", ".", -1)
	match, err := regexp.MatchString(pat, str)
	if err != nil {
		panic(fmt.Sprintf("internal error: constructed bad regexp /%s/: %v", pat, err))
	}
	return match
}
