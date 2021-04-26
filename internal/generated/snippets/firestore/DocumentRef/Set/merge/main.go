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

// [START firestore_generated_firestore_DocumentRef_Set_merge]

package main

import (
	"context"

	"cloud.google.com/go/firestore"
)

func main() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	// Overwrite only the fields in the map; preserve all others.
	_, err = client.Doc("States/Alabama").Set(ctx, map[string]interface{}{
		"pop": 5.2,
	}, firestore.MergeAll)
	if err != nil {
		// TODO: Handle error.
	}

	type State struct {
		Capital    string  `firestore:"capital"`
		Population float64 `firestore:"pop"` // in millions
	}

	// To do a merging Set with struct data, specify the exact fields to overwrite.
	// MergeAll is disallowed here, because it would probably be a mistake: the "capital"
	// field would be overwritten with the empty string.
	_, err = client.Doc("States/Alabama").Set(ctx, State{Population: 5.2}, firestore.Merge([]string{"pop"}))
	if err != nil {
		// TODO: Handle error.
	}
}

// [END firestore_generated_firestore_DocumentRef_Set_merge]
