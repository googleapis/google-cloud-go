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
	"fmt"
	"net/url"
	"strings"

	"cloud.google.com/go/bigquery/v2/query"
)

// Connector is a database/sql/driver.Connector for BigQuery.
type Connector struct {
	projectID string
	// other options can go here
}

// NewConnector creates a new Connector.
// The name is a connection string in the following format:
// "bigquery://<project_id>"
func NewConnector(name string) (*Connector, error) {
	u, err := url.Parse(name)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "bigquery" {
		return nil, fmt.Errorf("invalid connection string: scheme must be bigquery")
	}
	projectID := u.Host
	if projectID == "" {
		// fallback to path for cases like bigquery:/project-id
		projectID = strings.Trim(u.Path, "/")
	}
	if projectID == "" {
		return nil, fmt.Errorf("invalid connection string: missing project_id")
	}
	return &Connector{
		projectID: projectID,
	}, nil
}

// Connect returns a new connection to the database.
func (c *Connector) Connect(ctx context.Context) (driver.Conn, error) {
	client, err := query.NewClient(ctx, c.projectID)
	if err != nil {
		return nil, err
	}
	return &conn{
		client: client,
	}, nil
}

// Driver returns the underlying driver.
func (c *Connector) Driver() driver.Driver {
	return &Driver{}
}

// conn is a database/sql/driver.Conn for BigQuery.
type conn struct {
	client *query.Client
}

// Prepare returns a prepared statement, bound to this connection.
func (c *conn) Prepare(q string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), q)
}

// PrepareContext returns a prepared statement, bound to this connection.
func (c *conn) PrepareContext(ctx context.Context, q string) (driver.Stmt, error) {
	return &stmt{
		conn: c,
		sql:  q,
	}, nil
}

// Close closes the connection.
func (c *conn) Close() error {
	return c.client.Close()
}

// Begin starts and returns a new transaction.
// BigQuery does not support transactions, so this is a no-op.
func (c *conn) Begin() (driver.Tx, error) {
	return &tx{}, nil
}

// tx is a database/sql/driver.Tx for BigQuery.
// BigQuery does not support transactions, so this is a no-op.
type tx struct{}

// Commit is a no-op.
func (t *tx) Commit() error {
	return nil
}

// Rollback is a no-op.
func (t *tx) Rollback() error {
	return nil
}
