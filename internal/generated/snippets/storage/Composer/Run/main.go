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

// [START storage_generated_storage_Composer_Run]

package main

import (
	"context"
	"fmt"

	"cloud.google.com/go/storage"
)

func main() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	bkt := client.Bucket("bucketname")
	src1 := bkt.Object("o1")
	src2 := bkt.Object("o2")
	dst := bkt.Object("o3")

	// Compose and modify metadata.
	c := dst.ComposerFrom(src1, src2)
	c.ContentType = "text/plain"

	// Set the expected checksum for the destination object to be validated by
	// the backend (if desired).
	c.CRC32C = 42
	c.SendCRC32C = true

	attrs, err := c.Run(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(attrs)
	// Just compose.
	attrs, err = dst.ComposerFrom(src1, src2).Run(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(attrs)
}

// [END storage_generated_storage_Composer_Run]
