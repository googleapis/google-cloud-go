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

// [START spanner_generated_spanner_NewClientWithConfig]

package main

import (
	"context"

	"cloud.google.com/go/spanner"
)

func main() {
	ctx := context.Background()
	const myDB = "projects/my-project/instances/my-instance/database/my-db"
	client, err := spanner.NewClientWithConfig(ctx, myDB, spanner.ClientConfig{
		NumChannels: 10,
	})
	if err != nil {
		// TODO: Handle error.
	}
	_ = client     // TODO: Use client.
	client.Close() // Close client when done.
}

// [END spanner_generated_spanner_NewClientWithConfig]
