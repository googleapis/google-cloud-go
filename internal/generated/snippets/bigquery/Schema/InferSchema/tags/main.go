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

// [START bigquery_generated_bigquery_InferSchema_tags]

package main

import (
	"fmt"

	"cloud.google.com/go/bigquery"
)

func main() {
	type Item struct {
		Name     string
		Size     float64
		Count    int    `bigquery:"number"`
		Secret   []byte `bigquery:"-"`
		Optional bigquery.NullBool
		OptBytes []byte `bigquery:",nullable"`
	}
	schema, err := bigquery.InferSchema(Item{})
	if err != nil {
		fmt.Println(err)
		// TODO: Handle error.
	}
	for _, fs := range schema {
		fmt.Println(fs.Name, fs.Type, fs.Required)
	}
}

// [END bigquery_generated_bigquery_InferSchema_tags]
