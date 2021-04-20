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

// [START storage_generated_storage_Writer_Write_timeout]

package main

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/storage"
)

func main() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel() // Cancel when done, whether we time out or not.
	wc := client.Bucket("bucketname").Object("filename1").NewWriter(tctx)
	wc.ContentType = "text/plain"
	wc.ACL = []storage.ACLRule{{Entity: storage.AllUsers, Role: storage.RoleReader}}
	if _, err := wc.Write([]byte("hello world")); err != nil {
		// TODO: handle error.
		// Note that Write may return nil in some error situations,
		// so always check the error from Close.
	}
	if err := wc.Close(); err != nil {
		// TODO: handle error.
	}
	fmt.Println("updated object:", wc.Attrs())
}

// [END storage_generated_storage_Writer_Write_timeout]
