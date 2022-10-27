// Copyright 2022 Google LLC
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

package reader_test

import (
	"context"
	"fmt"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/reader"
	"google.golang.org/api/iterator"
)

func ExampleReadFromSources() {
	ctx := context.Background()
	projectID := "project-id"
	client, err := bigquery.NewClient(ctx, projectID)
	if err != nil {
		// TODO: Handle error.
	}
	storageReadClient, err := reader.NewClient(ctx, projectID)
	if err != nil {
		// TODO: Handle error.
	}

	r, err := storageReadClient.NewReader()
	if err != nil {
		// TODO: Handle error.
	}

	sql := fmt.Sprintf(`SELECT name, number, state FROM %s WHERE state = "CA"`, `bigquery-public-data.usa_names.usa_1910_current`)
	q := client.Query(sql)
	job, err := q.Run(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	status, err := job.Wait(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	if err := status.Err(); err != nil {
		// TODO: Handle error.
	}

	it, err := r.ReadQuery(ctx, q)
	if err != nil {
		// TODO: Handle error.
	}
	type S struct {
		Name   string
		Number int
		State  string
	}
	for {
		var s S
		err := it.Next(&s)
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}
	}
}
