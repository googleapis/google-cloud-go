// Copyright 2025 Google LLC
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

package driver

import (
	"context"
	"database/sql/driver"

	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"cloud.google.com/go/bigquery/v2/query"
)

// stmt is a database/sql/driver.Stmt for BigQuery.
type stmt struct {
	conn *conn
	sql  string
}

// Close closes the statement.
func (s *stmt) Close() error {
	return nil
}

// NumInput returns the number of placeholder parameters.
func (s *stmt) NumInput() int {
	// BigQuery uses named parameters, so we can't know the number of inputs.
	// -1 indicates that the number of inputs is unknown.
	return -1
}

// Exec executes a query that doesn't return rows, such as an INSERT or UPDATE.
func (s *stmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.ExecContext(context.Background(), convertToNamedValues(args))
}

// ExecContext executes a query that doesn't return rows, such as an INSERT or UPDATE.
func (s *stmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	q, err := s.startQuery(ctx, args)
	if err != nil {
		return nil, err
	}
	if err := q.Wait(ctx); err != nil {
		return nil, err
	}
	return &result{
		numDMLAffectedRows: q.NumAffectedRows(),
	}, nil
}

// Query executes a query that returns rows, such as a SELECT.
func (s *stmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.QueryContext(context.Background(), convertToNamedValues(args))
}

// QueryContext executes a query that returns rows, such as a SELECT.
func (s *stmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	q, err := s.startQuery(ctx, args)
	if err != nil {
		return nil, err
	}
	err = q.Wait(ctx)
	if err != nil {
		return nil, err
	}
	it, err := q.Read(ctx)
	if err != nil {
		return nil, err
	}
	return &rows{
		it:     it,
		schema: q.Schema(),
	}, nil
}

func (s *stmt) startQuery(ctx context.Context, args []driver.NamedValue) (*query.Query, error) {
	req := s.conn.client.FromSQL(s.sql)
	params := make([]*bigquerypb.QueryParameter, len(args))
	for i, arg := range args {
		params[i] = query.InferQueryParam(arg.Name, arg.Value)
	}
	req.QueryRequest.QueryParameters = params
	return s.conn.client.StartQuery(ctx, req)
}

func convertToNamedValues(args []driver.Value) []driver.NamedValue {
	namedValues := make([]driver.NamedValue, len(args))
	for i, v := range args {
		namedValues[i] = driver.NamedValue{
			Ordinal: i + 1,
			Value:   v,
		}
	}
	return namedValues
}
