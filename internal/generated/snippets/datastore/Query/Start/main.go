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

// [START datastore_generated_datastore_Query_Start]

package main

import (
	"context"

	"cloud.google.com/go/datastore"
)

func getCursor() string { return "" }

func main() {
	// This example demonstrates how to use cursors and Query.Start
	// to resume an iteration.
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	// getCursor represents a function that returns a cursor from a previous
	// iteration in string form.
	cursorString := getCursor()
	cursor, err := datastore.DecodeCursor(cursorString)
	if err != nil {
		// TODO: Handle error.
	}
	it := client.Run(ctx, datastore.NewQuery("Post").Start(cursor))
	_ = it // TODO: Use iterator.
}

// [END datastore_generated_datastore_Query_Start]
