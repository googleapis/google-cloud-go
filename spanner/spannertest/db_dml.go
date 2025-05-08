/*
Copyright 2025 Google LLC

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

	"cloud.google.com/go/spanner/spansql"
)

// executeDML executes a DML statement and returns a rowIter that contains the result.
func (s *server) executeDML(stmt spansql.DMLStmt, params queryParams) (rowIter, error) {
	// For DML statements, we need to return a rowIter that contains the number of rows affected
	affected, err := s.db.Execute(stmt, params)
	if err != nil {
		return nil, err
	}

	// Create a rowIter that returns a single row with the number of rows affected
	return &dmlResultIter{
		affected: affected,
	}, nil
}

// dmlResultIter is a rowIter that returns a single row containing the number of rows affected by a DML statement
type dmlResultIter struct {
	affected int
	done     bool
}

func (di *dmlResultIter) Cols() []colInfo {
	return []colInfo{
		{
			Name: "affected_rows",
			Type: spansql.Type{Base: spansql.Int64},
		},
	}
}

func (di *dmlResultIter) Next() (row, error) {
	if di.done {
		return nil, io.EOF
	}
	di.done = true
	return row{int64(di.affected)}, nil
}

// tryParseDML attempts to parse a SQL statement as a DML statement.
// If successful, it returns the DML statement and true.
// If not a DML statement, it returns nil and false.
func tryParseDML(sql string) (spansql.DMLStmt, bool) {
	stmt, err := spansql.ParseDMLStmt(sql)
	if err == nil {
		return stmt, true
	}
	return nil, false
}
