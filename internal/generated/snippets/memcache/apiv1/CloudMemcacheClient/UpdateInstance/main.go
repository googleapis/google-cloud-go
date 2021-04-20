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

// [START memcache_generated_memcache_apiv1_CloudMemcacheClient_UpdateInstance]

package main

import (
	"context"

	memcache "cloud.google.com/go/memcache/apiv1"
	memcachepb "google.golang.org/genproto/googleapis/cloud/memcache/v1"
)

func main() {
	// import memcachepb "google.golang.org/genproto/googleapis/cloud/memcache/v1"

	ctx := context.Background()
	c, err := memcache.NewCloudMemcacheClient(ctx)
	if err != nil {
		// TODO: Handle error.
	}

	req := &memcachepb.UpdateInstanceRequest{
		// TODO: Fill request struct fields.
	}
	op, err := c.UpdateInstance(ctx, req)
	if err != nil {
		// TODO: Handle error.
	}

	resp, err := op.Wait(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	// TODO: Use resp.
	_ = resp
}

// [END memcache_generated_memcache_apiv1_CloudMemcacheClient_UpdateInstance]
