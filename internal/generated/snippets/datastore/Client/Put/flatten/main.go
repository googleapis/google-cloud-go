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

// [START datastore_generated_datastore_Client_Put_flatten]

package main

import (
	"context"
	"log"

	"cloud.google.com/go/datastore"
)

func main() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		log.Fatal(err)
	}

	type Animal struct {
		Name  string
		Type  string
		Breed string
	}

	type Human struct {
		Name   string
		Height int
		Pet    Animal `datastore:",flatten"`
	}

	newKey := datastore.IncompleteKey("Human", nil)
	_, err = client.Put(ctx, newKey, &Human{
		Name:   "Susan",
		Height: 67,
		Pet: Animal{
			Name:  "Fluffy",
			Type:  "Cat",
			Breed: "Sphynx",
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}

// [END datastore_generated_datastore_Client_Put_flatten]
