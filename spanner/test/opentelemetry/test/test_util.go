/*
Copyright 2024 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	stestutil "cloud.google.com/go/spanner/internal/testutil"
	"google.golang.org/api/option"
)

func setupMockedTestServerWithConfig(t *testing.T, config spanner.ClientConfig) (server *stestutil.MockedSpannerInMemTestServer, client *spanner.Client, teardown func()) {
	return setupMockedTestServerWithConfigAndClientOptions(t, config, []option.ClientOption{})
}

func setupMockedTestServerWithConfigAndClientOptions(t *testing.T, config spanner.ClientConfig, clientOptions []option.ClientOption) (server *stestutil.MockedSpannerInMemTestServer, client *spanner.Client, teardown func()) {
	/*grpcHeaderChecker := &itestutil.HeadersEnforcer{
		OnFailure: t.Fatalf,
		Checkers: []*itestutil.HeaderChecker{
			{
				Key: "x-goog-api-client",
				ValuesValidator: func(token ...string) error {
					if len(token) != 1 {
						return status.Errorf(codes.Internal, "unexpected number of api client token headers: %v", len(token))
					}
					if !strings.HasPrefix(token[0], "gl-go/") {
						return status.Errorf(codes.Internal, "unexpected api client token: %v", token[0])
					}
					if !strings.Contains(token[0], "gccl/") {
						return status.Errorf(codes.Internal, "unexpected api client token: %v", token[0])
					}
					return nil
				},
			},
		},
	}
	clientOptions = append(clientOptions, grpcHeaderChecker.CallOptions()...)*/
	server, opts, serverTeardown := stestutil.NewMockedSpannerInMemTestServer(t)
	opts = append(opts, clientOptions...)
	ctx := context.Background()
	formattedDatabase := fmt.Sprintf("projects/%s/instances/%s/databases/%s", "[PROJECT]", "[INSTANCE]", "[DATABASE]")
	client, err := spanner.NewClientWithConfig(ctx, formattedDatabase, config, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return server, client, func() {
		client.Close()
		serverTeardown()
	}
}

func waitFor(t *testing.T, assert func() error) {
	t.Helper()
	timeout := 120 * time.Second
	ta := time.After(timeout)

	for {
		select {
		case <-ta:
			if err := assert(); err != nil {
				t.Fatalf("after %v waiting, got %v", timeout, err)
			}
			return
		default:
		}

		if err := assert(); err != nil {
			// Fail. Let's pause and retry.
			time.Sleep(10 * time.Millisecond)
			continue
		}

		return
	}
}
