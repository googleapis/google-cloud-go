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

// [START bigquery_generated_bigquery_Inserter_Put]

package main

import (
	"context"

	"cloud.google.com/go/bigquery"
)

type Item struct {
	Name  string
	Size  float64
	Count int
}

// Save implements the ValueSaver interface.
func (i *Item) Save() (map[string]bigquery.Value, string, error) {
	return map[string]bigquery.Value{
		"Name":  i.Name,
		"Size":  i.Size,
		"Count": i.Count,
	}, "", nil
}

func main() {
	ctx := context.Background()
	client, err := bigquery.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	ins := client.Dataset("my_dataset").Table("my_table").Inserter()
	// Item implements the ValueSaver interface.
	items := []*Item{
		{Name: "n1", Size: 32.6, Count: 7},
		{Name: "n2", Size: 4, Count: 2},
		{Name: "n3", Size: 101.5, Count: 1},
	}
	if err := ins.Put(ctx, items); err != nil {
		// TODO: Handle error.
	}
}

// [END bigquery_generated_bigquery_Inserter_Put]
