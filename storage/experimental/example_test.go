// Copyright 2026 Google LLC
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

package experimental_test

import (
	"context"
	"time"

	"cloud.google.com/go/storage"
	"cloud.google.com/go/storage/experimental"
)

func ExampleWithGRPCKeepalive() {
	ctx := context.Background()
	// Enable HTTP/2 keepalive pings so that a connection whose peer has
	// silently stopped responding is detected and closed within roughly
	// Time+Timeout, rather than waiting for the OS TCP retransmission limit.
	client, err := storage.NewGRPCClient(ctx, experimental.WithGRPCKeepalive(experimental.GRPCKeepaliveConfig{
		Time:    30 * time.Second,
		Timeout: 20 * time.Second,
	}))
	if err != nil {
		// TODO: handle error.
	}
	_ = client // TODO: Use the client.
}
