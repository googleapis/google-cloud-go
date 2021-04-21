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

// [START datastore_generated_datastore_DecodeCursor]

package main

import (
	"cloud.google.com/go/datastore"
)

func main() {
	// See Query.Start for a fuller example of DecodeCursor.
	// getCursor represents a function that returns a cursor from a previous
	// iteration in string form.
	cursorString := getCursor()
	cursor, err := datastore.DecodeCursor(cursorString)
	if err != nil {
		// TODO: Handle error.
	}
	_ = cursor // TODO: Use the cursor with Query.Start or Query.End.
}

func getCursor() string { return "" }

// [END datastore_generated_datastore_DecodeCursor]
