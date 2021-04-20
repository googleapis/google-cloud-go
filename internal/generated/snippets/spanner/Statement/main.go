// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// [START spanner_generated_spanner_Statement_regexpContains]

package main

import (
	"cloud.google.com/go/spanner"
)

func main() {
	// Search for accounts with valid emails using regexp as per:
	//   https://cloud.google.com/spanner/docs/functions-and-operators#regexp_contains
	stmt := spanner.Statement{
		SQL: `SELECT * FROM users WHERE REGEXP_CONTAINS(email, @valid_email)`,
		Params: map[string]interface{}{
			"valid_email": `\Q@\E`,
		},
	}
	_ = stmt // TODO: Use stmt in a query.
}

// [END spanner_generated_spanner_Statement_regexpContains]
