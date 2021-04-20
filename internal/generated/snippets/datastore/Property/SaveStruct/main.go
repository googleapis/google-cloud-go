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

// [START datastore_generated_datastore_SaveStruct]

package main

import (
	"fmt"

	"cloud.google.com/go/datastore"
)

func main() {
	type Player struct {
		User  string
		Score int
	}

	p := &Player{
		User:  "Alice",
		Score: 97,
	}
	props, err := datastore.SaveStruct(p)
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(props)
	// TODO(jba): make this output stable: Output: [{User Alice false} {Score 97 false}]
}

// [END datastore_generated_datastore_SaveStruct]
