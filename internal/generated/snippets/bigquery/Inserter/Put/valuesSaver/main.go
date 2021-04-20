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

// [START bigquery_generated_bigquery_Inserter_Put_valuesSaver]

package main

import (
	"context"

	"cloud.google.com/go/bigquery"
)

var schema bigquery.Schema

func main() {
	ctx := context.Background()
	client, err := bigquery.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	ins := client.Dataset("my_dataset").Table("my_table").Inserter()

	var vss []*bigquery.ValuesSaver
	for i, name := range []string{"n1", "n2", "n3"} {
		// Assume schema holds the table's schema.
		vss = append(vss, &bigquery.ValuesSaver{
			Schema:   schema,
			InsertID: name,
			Row:      []bigquery.Value{name, int64(i)},
		})
	}

	if err := ins.Put(ctx, vss); err != nil {
		// TODO: Handle error.
	}
}

// [END bigquery_generated_bigquery_Inserter_Put_valuesSaver]
