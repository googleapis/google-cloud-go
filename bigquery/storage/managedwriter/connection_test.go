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
		pool.router = newSimpleRouter(pool)
		writer := &ManagedStream{id: "foo"}
		pool.addWriter(writer)
		conn, err := pool.router.pickConnection(nil)
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

func TestSimpleRouter(t *testing.T) {

	ctx := context.Background()

	pool := &connectionPool{
		ctx: ctx,
		open: func(opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
			return &testAppendRowsClient{}, nil
		},
	}
	// TODO: switch to attach semantics in future PR.
	pool.router = newSimpleRouter(pool)
	pw := newPendingWrite(ctx, [][]byte{[]byte("foo")})
	// picking before attaching should yield error
	if _, err := pool.router.pickConnection(pw); err == nil {
		t.Errorf("pickConnection: expected error, got success")
	}
	writer := &ManagedStream{
		id:   "writer",
		pool: pool,
	}
	if err := pool.addWriter(writer); err != nil {
		t.Errorf("addWriter: %v", err)
	}
	if _, err := pool.router.pickConnection(pw); err != nil {
		t.Errorf("pickConnection error: %v", err)
	}
	if err := pool.removeWriter(writer); err != nil {
		t.Errorf("disconnectWriter: %v", err)
	}
	if _, err := pool.router.pickConnection(pw); err == nil {
		t.Errorf("pickConnection: expected error, got success")
	}
}
