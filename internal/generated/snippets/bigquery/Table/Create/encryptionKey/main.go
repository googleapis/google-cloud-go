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

// [START bigquery_generated_bigquery_Table_Create_encryptionKey]

package main

import (
	"context"

	"cloud.google.com/go/bigquery"
)

func main() {
	ctx := context.Background()
	// Infer table schema from a Go type.
	schema, err := bigquery.InferSchema(Item{})
	if err != nil {
		// TODO: Handle error.
	}
	client, err := bigquery.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	t := client.Dataset("my_dataset").Table("new-table")

	// TODO: Replace this key with a key you have created in Cloud KMS.
	keyName := "projects/P/locations/L/keyRings/R/cryptoKeys/K"
	if err := t.Create(ctx,
		&bigquery.TableMetadata{
			Name:             "My New Table",
			Schema:           schema,
			EncryptionConfig: &bigquery.EncryptionConfig{KMSKeyName: keyName},
		}); err != nil {
		// TODO: Handle error.
	}
}

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

// [END bigquery_generated_bigquery_Table_Create_encryptionKey]
