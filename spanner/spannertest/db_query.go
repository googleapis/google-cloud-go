/*
Copyright 2020 Google LLC

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

import (
	"io"
	"sort"

	"cloud.google.com/go/spanner/spansql"
)

/*
There's several ways to conceptualise SQL queries. The simplest, and what
we implement here, is a series of pipelines that transform the data, whether
pulling from a table (FROM tbl), filtering (WHERE expr), re-ordering (ORDER BY expr)
or other transformations.

The order of operations among those supported by Cloud Spanner is
	FROM + JOIN + set ops [TODO: JOIN and set ops]
	WHERE
	GROUP BY [TODO]
	aggregation [TODO]
	HAVING [TODO]
	SELECT
	DISTINCT
	ORDER BY
	OFFSET [TODO]
	LIMIT
*/

// rowIter represents some iteration over rows of data.
// It is returned by reads and queries.
type rowIter interface {
	// Cols returns the metadata about the returned data.
	Cols() []colInfo

	// Next returns the next row.
	// If done, it returns (nil, io.EOF).
	Next() (row, error)
}

// nullIter is a rowIter that returns one empty row only.
// This is used for queries without a table.
type nullIter struct {
	done bool
}

func (ni *nullIter) Cols() []colInfo { return nil }
func (ni *nullIter) Next() (row, error) {
	if ni.done {
		return nil, io.EOF
	}
	ni.done = true
	return nil, nil
}

// tableIter is a rowIter that walks a table.
// It assumes the table is locked for the duration.
type tableIter struct {
	t        *table
	rowIndex int // index of next row to return
}

func (ti *tableIter) Cols() []colInfo { return ti.t.cols }
func (ti *tableIter) Next() (row, error) {
	if ti.rowIndex >= len(ti.t.rows) {
		return nil, io.EOF
	}
	res := ti.t.rows[ti.rowIndex]
	ti.rowIndex++
	return res, nil
}

// rawIter is a rowIter with fixed data.
type rawIter struct {
	// cols is the metadata about the returned data.
	cols []colInfo

	// rows holds the result data itself.
	rows []row
}

func (raw *rawIter) Cols() []colInfo { return raw.cols }
func (raw *rawIter) Next() (row, error) {
	if len(raw.rows) == 0 {
		return nil, io.EOF
	}
	res := raw.rows[0]
	raw.rows = raw.rows[1:]
	return res, nil
}

func (raw *rawIter) add(src row, colIndexes []int) {
	raw.rows = append(raw.rows, src.copyData(colIndexes))
}

func toRawIter(ri rowIter) (*rawIter, error) {
	if raw, ok := ri.(*rawIter); ok {
		return raw, nil
	}
	raw := &rawIter{cols: ri.Cols()}
	for {
		row, err := ri.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		raw.rows = append(raw.rows, row)
	}
	return raw, nil
}

// whereIter applies a WHERE clause.
type whereIter struct {
	ri    rowIter
	ec    evalContext
	where spansql.BoolExpr
}

func (wi whereIter) Cols() []colInfo { return wi.ri.Cols() }
func (wi whereIter) Next() (row, error) {
	for {
		row, err := wi.ri.Next()
		if err != nil {
			return nil, err
		}
		wi.ec.row = row

		b, err := wi.ec.evalBoolExpr(wi.where)
		if err != nil {
			return nil, err
		}
		if !b {
			continue
		}
		return row, nil
	}
}

// selIter applies a SELECT list.
type selIter struct {
	ri   rowIter
	ec   evalContext
	cis  []colInfo
	list []spansql.Expr
}

func (si selIter) Cols() []colInfo { return si.cis }
func (si selIter) Next() (row, error) {
	row, err := si.ri.Next()
	if err != nil {
		return nil, err
	}
	si.ec.row = row

	selectStar := len(si.list) == 1 && si.list[0] == spansql.Star
	if selectStar {
		return row, nil
	}

	return si.ec.evalExprList(si.list)
}

// distinctIter applies a DISTINCT filter.
type distinctIter struct {
	ri   rowIter
	seen []row
}

func (di *distinctIter) Cols() []colInfo { return di.ri.Cols() }
func (di *distinctIter) Next() (row, error) {
	// This is hilariously inefficient; O(N^2) in the number of returned rows.
	// Some sort of hashing could be done to deduplicate instead.
	// This also breaks on array/struct types.
	for {
		row, err := di.ri.Next()
		if err != nil {
			return nil, err
		}
		dupe := false
		for _, prev := range di.seen {
			if rowEqual(prev, row) {
				dupe = true
				break
			}
		}
		if dupe {
			continue
		}
		di.seen = append(di.seen, row)
		return row, nil
	}
}

type queryParams map[string]interface{}

func (d *database) Query(q spansql.Query, params queryParams) (rowIter, error) {
	// If there's an ORDER BY clause, extend the query to include the expressions we need
	// so they get evaluated during evalSelect. TODO: Is this actually okay?
	var aux []spansql.Expr
	var desc []bool
	for _, o := range q.Order {
		aux = append(aux, o.Expr)
		desc = append(desc, o.Desc)
	}
	q.Select.List = append(q.Select.List, aux...)

	ri, err := d.evalSelect(q.Select, params)
	if err != nil {
		return nil, err
	}

	// Apply ORDER BY.
	if len(q.Order) > 0 {
		raw, err := toRawIter(ri)
		if err != nil {
			return nil, err
		}
		sort.Slice(raw.rows, func(one, two int) bool {
			r1, r2 := raw.rows[one], raw.rows[two]
			aux1, aux2 := r1[len(r1)-len(aux):], r2[len(r2)-len(aux):] // sort keys
			for i := range aux1 {
				cmp := compareVals(aux1[i], aux2[i])
				if desc[i] {
					cmp = -cmp
				}
				if cmp == 0 {
					continue
				}
				return cmp < 0
			}
			return false
		})
		// Remove ORDER BY values.
		raw.cols = raw.cols[:len(raw.cols)-len(aux)]
		for i, row := range raw.rows {
			raw.rows[i] = row[:len(row)-len(aux)]
		}
		ri = raw
	}

	// TODO: OFFSET

	// Apply LIMIT.
	// TODO: this can be an iter too.
	if q.Limit != nil {
		lim, err := evalLimit(q.Limit, params)
		if err != nil {
			return nil, err
		}
		raw, err := toRawIter(ri)
		if err != nil {
			return nil, err
		}
		if n := int(lim); n < len(raw.rows) {
			raw.rows = raw.rows[:n]
		}
		ri = raw
	}

	return ri, nil
}

// TODO: move evalSelect here.
