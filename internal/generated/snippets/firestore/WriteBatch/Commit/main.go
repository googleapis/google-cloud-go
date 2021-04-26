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

// [START firestore_generated_firestore_WriteBatch_Commit]

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

	type State struct {
		Capital    string  `firestore:"capital"`
		Population float64 `firestore:"pop"` // in millions
	}

	ny := client.Doc("States/NewYork")
	ca := client.Doc("States/California")

	writeResults, err := client.Batch().
		Create(ny, State{Capital: "Albany", Population: 19.8}).
		Set(ca, State{Capital: "Sacramento", Population: 39.14}).
		Delete(client.Doc("States/WestDakota")).
		Commit(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(writeResults)
}

// [END firestore_generated_firestore_WriteBatch_Commit]
