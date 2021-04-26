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

// [START spanner_generated_spanner_InsertStruct]

package main

import (
	"cloud.google.com/go/spanner"
)

func main() {
	type User struct {
		Name, Email string
	}
	u := User{Name: "alice", Email: "a@example.com"}
	m, err := spanner.InsertStruct("Users", u)
	if err != nil {
		// TODO: Handle error.
	}
	_ = m // TODO: use with Client.Apply or in a ReadWriteTransaction.
}

// [END spanner_generated_spanner_InsertStruct]
