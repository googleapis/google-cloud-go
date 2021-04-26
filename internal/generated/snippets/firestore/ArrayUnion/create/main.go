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

// [START firestore_generated_firestore_ArrayUnion_create]

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

	wr, err := client.Doc("States/Colorado").Create(ctx, map[string]interface{}{
		"cities": firestore.ArrayUnion("Denver", "Golden", "Boulder"),
		"pop":    5.5,
	})
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(wr.UpdateTime)
}

// [END firestore_generated_firestore_ArrayUnion_create]
