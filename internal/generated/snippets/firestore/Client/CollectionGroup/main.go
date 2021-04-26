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

// [START firestore_generated_firestore_Client_CollectionGroup]

package main

import (
	"context"

	"cloud.google.com/go/firestore"
)

func main() {
	// Given:
	// France/Cities/Paris = {population: 100}
	// Canada/Cities/Montreal = {population: 95}

	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	// Query for ANY city with >95 pop, regardless of country.
	docs, err := client.CollectionGroup("Cities").
		Where("pop", ">", 95).
		OrderBy("pop", firestore.Desc).
		Limit(10).
		Documents(ctx).
		GetAll()
	if err != nil {
		// TODO: Handle error.
	}

	_ = docs // TODO: Use docs.
}

// [END firestore_generated_firestore_Client_CollectionGroup]
