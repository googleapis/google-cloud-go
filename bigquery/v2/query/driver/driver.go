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
	"database/sql"
	"database/sql/driver"
)

func init() {
	sql.Register("bigquery", &Driver{})
}

// Driver is the database/sql driver for BigQuery.
type Driver struct{}

// Open returns a new connection to the database.
// The name is a connection string in the following format:
// "bigquery://<project_id>"
func (d *Driver) Open(name string) (driver.Conn, error) {
	c, err := NewConnector(name)
	if err != nil {
		return nil, err
	}
	return c.Connect(context.Background())
}
