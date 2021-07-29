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

// constraint is a specific table constraint
type constraint struct {
	projection    string
	expectedValue int64
	allowedError  int64
}

type validationInfo struct {
	constraints map[string]*constraint
}

type constraintOption func(*validationInfo)

// WithType sets the stream type for the managed stream.
func withExactRowCount(totalRows int64) constraintOption {
	return func(vi *validationInfo) {
		result_col := "total_rows"
		vi.constraints[result_col] = &constraint{
			projection:    fmt.Sprintf("COUNT(1) AS %s", result_col),
			expectedValue: totalRows,
		}
	}
}

func withNullCount(colname string, nullcount int64) constraintOption {
	return func(vi *validationInfo) {
		result_col := fmt.Sprintf("nullcol_count_%s", colname)
		vi.constraints[result_col] = &constraint{
			projection:    fmt.Sprintf("COUNTIF(ISNULL(%s)) AS %s", colname, result_col),
			expectedValue: nullcount,
		}
	}
}

func withNonNullCount(colname string, nullcount int64) constraintOption {
	return func(vi *validationInfo) {
		result_col := fmt.Sprintf("nonnullcol_count_%s", colname)
		vi.constraints[result_col] = &constraint{
			projection:    fmt.Sprintf("COUNTIF(NOT ISNULL(%s)) AS %s", colname, result_col),
			expectedValue: nullcount,
		}
	}
}

func withDistinctValues(colname string, distinctVals int64) constraintOption {
	return func(vi *validationInfo) {
		result_col := fmt.Sprintf("distinct_count_%s", colname)
		vi.constraints[result_col] = &constraint{
			projection:    fmt.Sprintf("COUNT(DISTINCT %s) AS %s", colname, result_col),
			expectedValue: distinctVals,
		}
	}
}

func withApproxDistinctValues(colname string, approxValues int64, errorBound int64) constraintOption {
	return func(vi *validationInfo) {
		result_col := fmt.Sprintf("distinct_count_%s", colname)
		vi.constraints[result_col] = &constraint{
			projection:    fmt.Sprintf("APPROX_COUNT_DISTINCT(%s) AS %s", colname, result_col),
			expectedValue: approxValues,
			allowedError:  errorBound,
		}
	}
}
