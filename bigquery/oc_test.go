// Copyright 2018 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build go1.8

package bigquery

import (
	"testing"

	"cloud.google.com/go/internal/testutil"
	"golang.org/x/net/context"
)

func TestOCTracing(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}

	te := testutil.NewTestExporter()
	defer te.Unregister()

	ctx := context.Background()
	client, err := NewClient(ctx, "client-project-id")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	q := client.Query("select *")
	q.Run(ctx) // Doesn't matter if we get an error; span should be created either way

	if len(te.Spans) != 1 {
		t.Fatalf("Expected 1 span to be created, but got %d", len(te.Spans))
	}
}
