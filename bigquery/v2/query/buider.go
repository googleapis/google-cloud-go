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

package query

import (
	"fmt"

	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// FromSQL creates a query configuration from a SQL string.
func FromSQL(projectID, sql string) *bigquerypb.PostQueryRequest {
	return &bigquerypb.PostQueryRequest{
		QueryRequest: &bigquerypb.QueryRequest{
			Query:        sql,
			UseLegacySql: wrapperspb.Bool(false),
			FormatOptions: &bigquerypb.DataFormatOptions{
				UseInt64Timestamp: true,
			},
		},
		ProjectId: projectID,
	}
}

// QueryFromSQL creates a query configuration from a SQL string.
func (c *Client) FromSQL(sql string) *bigquerypb.PostQueryRequest {
	req := FromSQL(c.billingProjectID, sql)
	req.QueryRequest.JobCreationMode = c.defaultJobCreationMode
	return req
}

// InferQueryParam converts a basic Go type to a bigquerypb.QueryParameter
func InferQueryParam(name string, value any) *bigquerypb.QueryParameter {
	// TODO: infer types like we do in bigquery/v1/params.go
	return &bigquerypb.QueryParameter{
		Name: name,
		ParameterType: &bigquerypb.QueryParameterType{
			Type: "STRING",
		},
		ParameterValue: &bigquerypb.QueryParameterValue{
			Value: wrapperspb.String(fmt.Sprintf("%v", value)),
		},
	}
}
