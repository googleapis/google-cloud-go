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

// [START spanner_generated_spanner_GenericColumnValue_Decode]

package main

import (
	"fmt"

	"cloud.google.com/go/spanner"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
)

func main() {
	// In real applications, rows can be retrieved by methods like client.Single().ReadRow().
	row, err := spanner.NewRow([]string{"intCol", "strCol"}, []interface{}{42, "my-text"})
	if err != nil {
		// TODO: Handle error.
	}
	for i := 0; i < row.Size(); i++ {
		var col spanner.GenericColumnValue
		if err := row.Column(i, &col); err != nil {
			// TODO: Handle error.
		}
		switch col.Type.Code {
		case sppb.TypeCode_INT64:
			var v int64
			if err := col.Decode(&v); err != nil {
				// TODO: Handle error.
			}
			fmt.Println("int", v)
		case sppb.TypeCode_STRING:
			var v string
			if err := col.Decode(&v); err != nil {
				// TODO: Handle error.
			}
			fmt.Println("string", v)
		}
	}
}

// [END spanner_generated_spanner_GenericColumnValue_Decode]
