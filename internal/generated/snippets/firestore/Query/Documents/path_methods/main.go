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

// [START firestore_generated_firestore_Query_Documents_path_methods]

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

	q := client.Collection("Unusual").SelectPaths([]string{"*"}, []string{"[~]"}).
		WherePath([]string{"/"}, ">", 10).
		OrderByPath([]string{"/"}, firestore.Desc).
		Limit(10)
	iter1 := q.Documents(ctx)
	_ = iter1 // TODO: Use iter1.

	// You can call Documents directly on a CollectionRef as well.
	iter2 := client.Collection("States").Documents(ctx)
	_ = iter2 // TODO: Use iter2.
}

// [END firestore_generated_firestore_Query_Documents_path_methods]
