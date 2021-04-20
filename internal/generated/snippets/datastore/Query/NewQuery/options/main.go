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

// [START datastore_generated_datastore_NewQuery_options]

package main

import (
	"cloud.google.com/go/datastore"
)

func main() {
	// Query to order the posts by the number of comments they have received.
	q := datastore.NewQuery("Post").Order("-Comments")
	// Start listing from an offset and limit the results.
	q = q.Offset(20).Limit(10)
	_ = q // TODO: Use the query.
}

// [END datastore_generated_datastore_NewQuery_options]
