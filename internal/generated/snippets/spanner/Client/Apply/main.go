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

// [START spanner_generated_spanner_Client_Apply]

package main

import (
	"context"

	"cloud.google.com/go/spanner"
)

const myDB = "projects/my-project/instances/my-instance/database/my-db"

func main() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	m := spanner.Update("Users", []string{"name", "email"}, []interface{}{"alice", "a@example.com"})
	_, err = client.Apply(ctx, []*spanner.Mutation{m})
	if err != nil {
		// TODO: Handle error.
	}
}

// [END spanner_generated_spanner_Client_Apply]
