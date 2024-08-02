//go:build go1.20
// +build go1.20

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
	"os"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	stestutil "cloud.google.com/go/spanner/internal/testutil"
)

var (
	isMultiplexEnabled = getMultiplexEnableFlag()
)

func getMultiplexEnableFlag() bool {
	return os.Getenv("GOOGLE_CLOUD_SPANNER_MULTIPLEXED_SESSIONS") == "true"
}

func setupMockedTestServerWithConfig(t *testing.T, config spanner.ClientConfig) (server *stestutil.MockedSpannerInMemTestServer, client *spanner.Client, teardown func()) {
	server, opts, serverTeardown := stestutil.NewMockedSpannerInMemTestServer(t)
	ctx := context.Background()
	formattedDatabase := fmt.Sprintf("projects/%s/instances/%s/databases/%s", "[PROJECT]", "[INSTANCE]", "[DATABASE]")
	client, err := spanner.NewClientWithConfig(ctx, formattedDatabase, config, opts...)
	if err != nil {
		t.Fatal(err)
	}
	if isMultiplexEnabled {
		// trigger R/O txn for multiplexed session creation to avoid flakiness
		waitFor(t, func() error {
			iter := client.Single().Query(ctx, spanner.NewStatement(stestutil.SelectSingerIDAlbumIDAlbumTitleFromAlbums))
			return iter.Do(func(_ *spanner.Row) error { return nil })
		})
	}
	return server, client, func() {
		client.Close()
		serverTeardown()
	}
}

func waitFor(t *testing.T, assert func() error) {
	t.Helper()
	timeout := 15 * time.Second
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
