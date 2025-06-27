// Copyright 2024 Google LLC
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

package query

import (
	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// QueryFromSQL creates a query configuration from a SQL string.
func QueryFromSQL(projectID, sql string) *bigquerypb.PostQueryRequest {
	return &bigquerypb.PostQueryRequest{
		QueryRequest: &bigquerypb.QueryRequest{
			Query:        sql,
			UseLegacySql: wrapperspb.Bool(false),
		},
		ProjectId: projectID,
	}
}

// QueryFromSQL creates a query configuration from a SQL string.
func (c *QueryClient) QueryFromSQL(sql string) *bigquerypb.PostQueryRequest {
	return QueryFromSQL(c.billingProjectID, sql)
}
