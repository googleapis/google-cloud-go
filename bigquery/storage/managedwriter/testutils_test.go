// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package managedwriter

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"cloud.google.com/go/bigquery"
)

// validateTableConstraints is used to validate properties of a table by computing stats using the query engine.
func validateTableConstraints(ctx context.Context, t *testing.T, client *bigquery.Client, table *bigquery.Table, description string, opts ...constraintOption) {
	vi := &validationInfo{
		constraints: make(map[string]*constraint),
	}

	for _, o := range opts {
		o(vi)
	}

	if len(vi.constraints) == 0 {
		t.Errorf("%q: no constraints were specified", description)
		return
	}

	sql := new(bytes.Buffer)
	sql.WriteString("SELECT\n")
	var i int
	for _, c := range vi.constraints {
		if i > 0 {
			sql.WriteString(",")
		}
		sql.WriteString(c.projection)
		i++
	}
	sql.WriteString(fmt.Sprintf("\nFROM `%s`.%s.%s", table.ProjectID, table.DatasetID, table.TableID))
	q := client.Query(sql.String())
	it, err := q.Read(ctx)
	if err != nil {
		t.Errorf("%q: failed to issue validation query: %v", description, err)
		return
	}
	var resultrow []bigquery.Value
	err = it.Next(&resultrow)
	if err != nil {
		t.Errorf("%q: failed to get result row: %v", description, err)
		return
	}

	for colname, con := range vi.constraints {
		off := -1
		for k, v := range it.Schema {
			if v.Name == colname {
				off = k
				break
			}
		}
		if off == -1 {
			t.Errorf("%q: missing constraint %q from results", description, colname)
			continue
		}
		val, ok := resultrow[off].(int64)
		if !ok {
			t.Errorf("%q: constraint %q type mismatch", description, colname)
		}
		if con.allowedError == 0 {
			if val != con.expectedValue {
				t.Errorf("%q: constraint %q mismatch, got %d want %d", description, colname, val, con.expectedValue)
			}
			continue
		}
		res := val - con.expectedValue
		if res < 0 {
			res = -res
		}
		if res > con.allowedError {
			t.Errorf("%q: constraint %q outside error bound %d, got %d want %d", description, colname, con.allowedError, val, con.expectedValue)
		}
	}
}

// constraint is a specific table constraint.
type constraint struct {
	// sql fragment that projects a result value
	projection string

	// all validation constraints must eval as int64.
	expectedValue int64

	// if nonzero, the constraint value must be within allowedError distance of expectedValue.
	allowedError int64
}

// validationInfo is keyed by the result column name.
type validationInfo struct {
	constraints map[string]*constraint
}

// constraintOption is for building validation rules.
type constraintOption func(*validationInfo)

// withExactRowCount asserts the exact total row count of the table.
func withExactRowCount(totalRows int64) constraintOption {
	return func(vi *validationInfo) {
		resultCol := "total_rows"
		vi.constraints[resultCol] = &constraint{
			projection:    fmt.Sprintf("COUNT(1) AS %s", resultCol),
			expectedValue: totalRows,
		}
	}
}

// withNullCount asserts the number of null values in a column.
func withNullCount(colname string, nullCount int64) constraintOption {
	return func(vi *validationInfo) {
		resultCol := fmt.Sprintf("nullcol_count_%s", colname)
		vi.constraints[resultCol] = &constraint{
			projection:    fmt.Sprintf("SUM(IF(%s IS NULL,1,0)) AS %s", colname, resultCol),
			expectedValue: nullCount,
		}
	}
}

// withNonNullCount asserts the number of non null values in a column.
func withNonNullCount(colname string, nonNullCount int64) constraintOption {
	return func(vi *validationInfo) {
		resultCol := fmt.Sprintf("nonnullcol_count_%s", colname)
		vi.constraints[resultCol] = &constraint{
			projection:    fmt.Sprintf("SUM(IF(%s IS NOT NULL,1,0)) AS %s", colname, resultCol),
			expectedValue: nonNullCount,
		}
	}
}

// withDistinctValues validates the exact cardinality of a column.
func withDistinctValues(colname string, distinctVals int64) constraintOption {
	return func(vi *validationInfo) {
		resultCol := fmt.Sprintf("distinct_count_%s", colname)
		vi.constraints[resultCol] = &constraint{
			projection:    fmt.Sprintf("COUNT(DISTINCT %s) AS %s", colname, resultCol),
			expectedValue: distinctVals,
		}
	}
}

// withApproxDistinctValues validates the approximate cardinality of a column with an error bound.
func withApproxDistinctValues(colname string, approxValues int64, errorBound int64) constraintOption {
	return func(vi *validationInfo) {
		resultCol := fmt.Sprintf("distinct_count_%s", colname)
		vi.constraints[resultCol] = &constraint{
			projection:    fmt.Sprintf("APPROX_COUNT_DISTINCT(%s) AS %s", colname, resultCol),
			expectedValue: approxValues,
			allowedError:  errorBound,
		}
	}
}
