// Copyright 2022 Google LLC
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

package storage

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestCreateBucketEmulated_HTTP(t *testing.T) {
	if os.Getenv("STORAGE_EMULATOR_HOST") == "" {
		t.Skip("Emulator tests skipped without emulator running")
	}

	c, err := newHTTPStorageClient(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	want := &BucketAttrs{
		Name: fmt.Sprintf("http-bucket-%d", time.Now().Second()),
	}
	got, err := c.CreateBucket(context.Background(), "http-project", want)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(got.Name, want.Name); diff != "" {
		t.Errorf("got(-),want(+):\n%s", diff)
	}
}
