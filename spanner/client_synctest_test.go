//go:build go1.25

/*
Copyright 2025 Google LLC

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

package spanner

import (
	"context"
	"testing"
	"testing/synctest"

	"golang.org/x/sync/errgroup"

	. "cloud.google.com/go/spanner/internal/testutil"
)

func TestClient_GoroutineLeak(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name string
		test func(ctx context.Context, client *Client) error
	}{
		{
			name: "Connect",
			test: func(ctx context.Context, client *Client) error {
				return nil
			},
		},
		{
			name: "Single.ReadRow",
			test: func(ctx context.Context, client *Client) error {
				_, err := client.Single().ReadRow(ctx, "Albums", Key{"foo"}, []string{"SingerId", "AlbumId", "AlbumTitle"})
				return err
			},
		},
		{
			name: "ReadOnlyTransaction",
			test: func(ctx context.Context, client *Client) error {
				roTxn := client.ReadOnlyTransaction()
				defer roTxn.Close()

				iter := roTxn.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
				if err := iter.Do(func(row *Row) error {
					return nil
				}); err != nil {
					return err
				}

				iter = roTxn.Read(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"})
				return iter.Do(func(row *Row) error {
					return nil
				})
			},
		},
		{
			name: "ReadWriteTransaction",
			test: func(ctx context.Context, client *Client) error {
				_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *ReadWriteTransaction) error {
					iter := txn.Read(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"})
					return iter.Do(func(r *Row) error {
						return nil
					})
				})
				return err
			},
		},
		{
			name: "parallel Single.ReadRow",
			test: func(ctx context.Context, client *Client) error {
				g := new(errgroup.Group)
				for range 25 {
					g.Go(func() error {
						_, err := client.Single().ReadRow(ctx, "Albums", Key{"foo"}, []string{"SingerId", "AlbumId", "AlbumTitle"})
						return err
					})
				}
				return g.Wait()
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, opts, teardown := NewMockedSpannerInMemTestServer(t)
			defer teardown()

			synctest.Test(t, func(t *testing.T) {
				ctx := t.Context()

				// wait for any started goroutine to stop
				defer synctest.Wait()

				config := ClientConfig{}
				client, err := makeClientWithConfig(ctx, "projects/p/instances/i/databases/d", config, server.ServerAddress, opts...)
				if err != nil {
					t.Fatalf("failed to get a client: %v", err)
				}
				defer client.Close()

				test.test(ctx, client)
			})
		})
	}
}
