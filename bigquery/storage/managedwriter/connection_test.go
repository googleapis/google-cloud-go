// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package managedwriter

import (
	"context"
	"testing"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestConnection_OpenWithRetry(t *testing.T) {

	testCases := []struct {
		desc     string
		errors   []error
		wantFail bool
	}{
		{
			desc:     "no error",
			errors:   []error{nil},
			wantFail: false,
		},
		{
			desc: "transient failures",
			errors: []error{
				status.Errorf(codes.Unavailable, "try 1"),
				status.Errorf(codes.Unavailable, "try 2"),
				nil},
			wantFail: false,
		},
		{
			desc:     "terminal error",
			errors:   []error{status.Errorf(codes.InvalidArgument, "bad args")},
			wantFail: true,
		},
	}

	for _, tc := range testCases {
		pool := &connectionPool{
			ctx: context.Background(),
			open: func(opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
				if len(tc.errors) == 0 {
					panic("out of errors")
				}
				err := tc.errors[0]
				tc.errors = tc.errors[1:]
				if err == nil {
					return &testAppendRowsClient{}, nil
				}
				return nil, err
			},
		}
		conn, err := pool.addConnection()
		if err != nil {
			t.Errorf("case %s, failed to add connection: %v", tc.desc, err)
		}
		arc, ch, err := pool.openWithRetry(conn)
		if tc.wantFail && err == nil {
			t.Errorf("case %s: wanted failure, got success", tc.desc)
		}
		if !tc.wantFail && err != nil {
			t.Errorf("case %s: wanted success, got %v", tc.desc, err)
		}
		if err == nil {
			if arc == nil {
				t.Errorf("case %s: expected append client, got nil", tc.desc)
			}
			if ch == nil {
				t.Errorf("case %s: expected channel, got nil", tc.desc)
			}
		}
	}
}

func TestConnectionPool_ConnectionManagement(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	pool := newConnectionPool(ctx, nil, nil)

	if err := pool.evictConnection(""); err == nil {
		t.Errorf("expected error when evicting empty connection id")
	}

	unowned, err := pool.addConnection()
	if unowned.pool != pool {
		t.Errorf("addConnection didn't associate connection reference to pool")
	}
	if _, ok := pool.connectionMap[unowned.id]; ok {
		t.Errorf("did not expect unowned connection to be registered")
	}
	// cancel the connection context and verify the pool is healthy.
	unowned.cancel()
	if pool.ctx.Err() != nil {
		t.Errorf("expected healthy pool context, got %v", pool.ctx.Err())
	}

	fakeWriter := &ManagedStream{
		id: newUUID("fakewriter"),
	}

	conn, err := pool.connectionForWriter(fakeWriter.id)
	if err == nil {
		t.Errorf("expected error, got connection %s", conn.id)
	}

	// inject the writer into the pool so it can be resolved
	pool.writerMap[fakeWriter.id] = fakeWriter

	conn, err = pool.connectionForWriter(fakeWriter.id)
	if err != nil {
		t.Errorf("expected successful resolution, got %v", err)
	}
	if conn.pool != pool {
		t.Errorf("expected connection to have same pool association")
	}
	if len(pool.connectionMap) != 1 {
		t.Errorf("expected single connection in map, had %d connections", len(pool.connectionMap))
	}
	if got := pool.writerToConnMap[fakeWriter.id]; got != conn.id {
		t.Errorf("mismatched conn in writerToConnMap, got %s want %s", got, conn.id)
	}
	var foundWriter bool
	for _, v := range pool.connToWriterMap[conn.id] {
		if v == fakeWriter.id {
			foundWriter = true
		}
	}
	if !foundWriter {
		t.Errorf("writer not present in connToWriterMap")
	}

	// close and evict the connection
	conn.close(true)
	if len(pool.connectionMap) != 0 {
		t.Errorf("expected connectionMap to be empty, had %d entries", len(pool.connectionMap))
	}

	// cancel parent context, ensure pool and connection are moribund.
	cancel()
	if pool.ctx.Err() != context.Canceled {
		t.Errorf("expected cancelled pool context, got %v", pool.ctx.Err())
	}
	if conn.ctx.Err() != context.Canceled {
		t.Errorf("expected cancelled connection context, got %v", pool.ctx.Err())
	}

}
