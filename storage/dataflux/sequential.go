// Copyright 2024 Google LLC
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

package dataflux

import (
	"context"

	"cloud.google.com/go/storage"
)

// Listing performs a sequential listing on the given bucket.
// It returns a list of objects and the next token to use to continue listing.
// If the next token is empty, then listing is complete.
func sequentialListing(ctx context.Context, opts Lister) ([]*storage.ObjectAttrs, string) {
	return nil, ""
}
