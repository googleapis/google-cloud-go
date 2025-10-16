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

// Package query provides a simplified API for running queries in BigQuery.
//
// This package is EXPERIMENTAL and subject to change without notice.
//
// The query client provides a simplified interface for running queries and
// retrieving results. It handles the complexities of job management and result
// pagination.
//
// Example usage:
//
//	ctx := context.Background()
//	client, err := apiv2_client.NewClient(ctx)
//	if err != nil {
//		// TODO: Handle error.
//	}
//	defer client.Close()
//
//	helper, err := query.NewHelper(client, "my-project")
//	if err != nil {
//		// TODO: Handle error.
//	}
//
//	req := helper.FromSQL("SELECT 1 as foo")
//	q, err := helper.StartQuery(ctx, req)
//	if err != nil {
//		// TODO: Handle error.
//	}
//
//	if err := q.Wait(ctx); err != nil {
//		// TODO: Handle error.
//	}
//
//	it, err := q.Read(ctx)
//	if err != nil {
//		// TODO: Handle error.
//	}
//	// TODO: iterate results.
package query
