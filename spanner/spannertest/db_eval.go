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
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"cloud.google.com/go/spanner/spansql"
)

// evalContext represents the context for evaluating an expression.
type evalContext struct {
	table  *table // may be nil
	row    row    // set if table is set, only during expr evaluation
	params queryParams
}

func (d *database) evalSelect(sel spansql.Select, params queryParams) (ri rowIter, evalErr error) {
	ri = &nullIter{}
	ec := evalContext{
		params: params,
	}

	// First stage is to identify the data source.
	// If there's a FROM then that names a table to use.
	if len(sel.From) > 1 {
		return nil, fmt.Errorf("selecting from more than one table not yet supported")
	}
	if len(sel.From) == 1 {
		tableName := sel.From[0].Table
		t, err := d.table(tableName)
		if err != nil {
			return nil, err
		}
		t.mu.Lock()
		defer t.mu.Unlock()
		ri = &tableIter{t: t}
		ec.table = t
	}
	defer func() {
		// If we're about to return a tableIter, convert it to a rawIter
		// so that the table may be safely unlocked.
		if evalErr == nil {
			if ti, ok := ri.(*tableIter); ok {
				ri, evalErr = toRawIter(ti)
			}
		}
	}()

	// Apply WHERE.
	if sel.Where != nil {
		ri = whereIter{
			ri:    ri,
			ec:    ec,
			where: sel.Where,
		}
	}

	// Handle COUNT(*) specially.
	// TODO: Handle aggregation more generally.
	if len(sel.List) == 1 && isCountStar(sel.List[0]) {
		// Replace the `COUNT(*)` with `1`, then aggregate on the way out.
		sel.List[0] = spansql.IntegerLiteral(1)
		defer func() {
			if evalErr != nil {
				return
			}
			raw, err := toRawIter(ri)
			if err != nil {
				ri, evalErr = nil, err
			}
			count := int64(len(raw.rows))
			raw.rows = []row{{count}}
			ri, evalErr = raw, nil
		}()
	}

	// TODO: Support table sampling.

	// Apply SELECT list.
	var colInfos []colInfo
	// Is this a `SELECT *` query?
	selectStar := len(sel.List) == 1 && sel.List[0] == spansql.Star
	if selectStar {
		// Every column will appear in the output.
		colInfos = append([]colInfo(nil), ec.table.cols...)
	} else {
		for _, e := range sel.List {
			ci, err := ec.colInfo(e)
			if err != nil {
				return nil, err
			}
			// TODO: deal with ci.Name == ""?
			colInfos = append(colInfos, ci)
		}
	}
	ri = selIter{
		ri:   ri,
		ec:   ec,
		cis:  colInfos,
		list: sel.List,
	}

	// Apply DISTINCT.
	if sel.Distinct {
		ri = &distinctIter{ri: ri}
	}

	return ri, nil
}

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

func (ec evalContext) evalBoolExpr(be spansql.BoolExpr) (bool, error) {
	switch be := be.(type) {
	default:
		return false, fmt.Errorf("unhandled BoolExpr %T", be)
	case spansql.BoolLiteral:
		return bool(be), nil
	case spansql.ID, spansql.Paren:
		e, err := ec.evalExpr(be)
		if err != nil {
			return false, err
		}
		if e == nil { // NULL is a false boolean.
			return false, nil
		}
		b, ok := e.(bool)
		if !ok {
			return false, fmt.Errorf("got %T, want bool", e)
		}
		return b, nil
	case spansql.LogicalOp:
		var lhs, rhs bool
		var err error
		if be.LHS != nil {
			lhs, err = ec.evalBoolExpr(be.LHS)
			if err != nil {
				return false, err
			}
		}
		rhs, err = ec.evalBoolExpr(be.RHS)
		if err != nil {
			return false, err
		}
		switch be.Op {
		case spansql.And:
			return lhs && rhs, nil
		case spansql.Or:
			return lhs || rhs, nil
		case spansql.Not:
			return !rhs, nil
		default:
			return false, fmt.Errorf("unhandled LogicalOp %d", be.Op)
		}
	case spansql.ComparisonOp:
		var lhs, rhs interface{}
		var err error
		lhs, err = ec.evalExpr(be.LHS)
		if err != nil {
			return false, err
		}
		rhs, err = ec.evalExpr(be.RHS)
		if err != nil {
			return false, err
		}
		switch be.Op {
		default:
			return false, fmt.Errorf("TODO: ComparisonOp %d", be.Op)
		case spansql.Lt:
			return compareVals(lhs, rhs) < 0, nil
		case spansql.Le:
			return compareVals(lhs, rhs) <= 0, nil
		case spansql.Gt:
			return compareVals(lhs, rhs) > 0, nil
		case spansql.Ge:
			return compareVals(lhs, rhs) >= 0, nil
		case spansql.Eq:
			return compareVals(lhs, rhs) == 0, nil
		case spansql.Ne:
			return compareVals(lhs, rhs) != 0, nil
		case spansql.Like, spansql.NotLike:
			left, ok := lhs.(string)
			if !ok {
				// TODO: byte works here too?
				return false, fmt.Errorf("LHS of LIKE is %T, not string", lhs)
			}
			right, ok := rhs.(string)
			if !ok {
				// TODO: byte works here too?
				return false, fmt.Errorf("RHS of LIKE is %T, not string", rhs)
			}

			match := evalLike(left, right)
			if be.Op == spansql.NotLike {
				match = !match
			}
			return match, nil
		case spansql.Between, spansql.NotBetween:
			rhs2, err := ec.evalExpr(be.RHS2)
			if err != nil {
				return false, err
			}
			b := compareVals(rhs, lhs) <= 0 && compareVals(lhs, rhs2) <= 0
			if be.Op == spansql.NotBetween {
				b = !b
			}
			return b, nil
		}
	case spansql.IsOp:
		lhs, err := ec.evalExpr(be.LHS)
		if err != nil {
			return false, err
		}
		var b bool
		switch rhs := be.RHS.(type) {
		default:
			return false, fmt.Errorf("unhandled IsOp %T", rhs)
		case spansql.BoolLiteral:
			if lhs == nil {
				// For `X IS TRUE`, X being NULL is okay, and this evaluates
				// to false. Same goes for `X IS FALSE`.
				lhs = !bool(rhs)
			}
			lhsBool, ok := lhs.(bool)
			if !ok {
				return false, fmt.Errorf("non-bool value %T on LHS for %s", lhs, be.SQL())
			}
			b = (lhsBool == bool(rhs))
		case spansql.NullLiteral:
			b = (lhs == nil)
		}
		if be.Neg {
			b = !b
		}
		return b, nil
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
	switch e := e.(type) {
	default:
		return nil, fmt.Errorf("TODO: evalExpr(%s %T)", e.SQL(), e)
	case spansql.ID:
		return ec.evalID(e)
	case spansql.Param:
		v, ok := ec.params[string(e)]
		if !ok {
			return 0, fmt.Errorf("unbound param %s", e.SQL())
		}
		return v, nil
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
	case spansql.ArithOp:
		return ec.evalArithOp(e)
	case spansql.LogicalOp:
		return ec.evalBoolExpr(e)
	case spansql.ComparisonOp:
		return ec.evalBoolExpr(e)
	case spansql.IsOp:
		return ec.evalBoolExpr(e)
	}
}

func (ec evalContext) evalID(id spansql.ID) (interface{}, error) {
	// TODO: look beyond column names.
	if ec.table == nil {
		return nil, fmt.Errorf("identifier %s when not SELECTing on a table is not supported", string(id))
	}
	i, ok := ec.table.colIndex[string(id)]
	if !ok {
		return nil, fmt.Errorf("couldn't resolve identifier %s", string(id))
	}
	return ec.row.copyDataElem(i), nil
}

func evalLimit(lim spansql.Limit, params queryParams) (int64, error) {
	switch lim := lim.(type) {
	case spansql.IntegerLiteral:
		return int64(lim), nil
	case spansql.Param:
		return paramAsInteger(lim, params)
	default:
		return 0, fmt.Errorf("LIMIT with %T not supported", lim)
	}
}

func paramAsInteger(p spansql.Param, params queryParams) (int64, error) {
	v, ok := params[string(p)]
	if !ok {
		return 0, fmt.Errorf("unbound param %s", p.SQL())
	}
	switch v := v.(type) {
	default:
		return 0, fmt.Errorf("can't interpret parameter %s value of type %T as integer", p.SQL(), v)
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
		// This handles DATE and TIMESTAMP too.
		return strings.Compare(x, y.(string))
	}
}

var (
	int64Type   = spansql.Type{Base: spansql.Int64}
	float64Type = spansql.Type{Base: spansql.Float64}
	stringType  = spansql.Type{Base: spansql.String}
)

func (ec evalContext) colInfo(e spansql.Expr) (colInfo, error) {
	// TODO: more types
	switch e := e.(type) {
	case spansql.IntegerLiteral:
		return colInfo{Type: int64Type}, nil
	case spansql.StringLiteral:
		return colInfo{Type: spansql.Type{Base: spansql.String}}, nil
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
	case spansql.ID:
		// TODO: support more than only naming a table column.
		name := string(e)
		if ec.table != nil {
			if i, ok := ec.table.colIndex[name]; ok {
				return ec.table.cols[i], nil
			}
		}
	case spansql.Paren:
		return ec.colInfo(e.Expr)
	case spansql.NullLiteral:
		// There isn't necessarily something sensible here.
		// Empirically, though, the real Spanner returns Int64.
		return colInfo{Type: int64Type}, nil
	}
	return colInfo{}, fmt.Errorf("can't deduce column type from expression [%s]", e.SQL())
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

func isCountStar(e spansql.Expr) bool {
	f, ok := e.(spansql.Func)
	if !ok {
		return false
	}
	if f.Name != "COUNT" || len(f.Args) != 1 {
		return false
	}
	return f.Args[0] == spansql.Star
}
