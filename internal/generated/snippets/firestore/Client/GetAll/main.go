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

// [START firestore_generated_firestore_Client_GetAll]

package main

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
)

func main() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()
	docs, err := client.GetAll(ctx, []*firestore.DocumentRef{
		client.Doc("States/NorthCarolina"),
		client.Doc("States/SouthCarolina"),
		client.Doc("States/WestCarolina"),
		client.Doc("States/EastCarolina"),
	})
	if err != nil {
		// TODO: Handle error.
	}
	// docs is a slice with four DocumentSnapshots, but the last two are
	// nil because there is no West or East Carolina.
	fmt.Println(docs)
}

// [END firestore_generated_firestore_Client_GetAll]
