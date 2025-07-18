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
	"errors"
)

// result is a database/sql/driver.Result for BigQuery.
type result struct {
	numDMLAffectedRows int64
}

// LastInsertId is not supported by BigQuery.
func (r *result) LastInsertId() (int64, error) {
	return 0, errors.New("bigquery: LastInsertId is not supported")
}

// RowsAffected returns the number of rows affected by the query.
func (r *result) RowsAffected() (int64, error) {
	return r.numDMLAffectedRows, nil
}
