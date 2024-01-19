// Copyright 2023 Google LLC
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

package bigquery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStorageIteratorRetry(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	testCases := []struct {
		ctx      context.Context
		desc     string
		errors   []error
		wantFail bool
	}{
		{
			desc:     "no error",
			errors:   []error{},
			wantFail: false,
		},
		{
			desc: "transient failures",
			errors: []error{
				status.Errorf(codes.DeadlineExceeded, "try 1"),
				status.Errorf(codes.Unavailable, "try 2"),
				status.Errorf(codes.Canceled, "try 3"),
				status.Errorf(codes.Internal, "try 4"),
			},
			wantFail: false,
		},
		{
			desc: "not enough permission",
			errors: []error{
				status.Errorf(codes.PermissionDenied, "the user does not have 'bigquery.readsessions.getData' permission"),
			},
			wantFail: true,
		},
		{
			desc: "permanent error",
			errors: []error{
				status.Errorf(codes.InvalidArgument, "invalid args"),
			},
			wantFail: true,
		},
		{
			ctx:  cancelledCtx,
			desc: "context cancelled/deadline exceeded",
			errors: []error{
				fmt.Errorf("random error"),
				fmt.Errorf("another random error"),
				fmt.Errorf("yet another random error"),
			},
			wantFail: true,
		},
	}
	for _, tc := range testCases {
		rrc := &testReadRowsClient{
			errors: tc.errors,
		}
		readRowsFuncs := map[string]func(context.Context, *storagepb.ReadRowsRequest, ...gax.CallOption) (storagepb.BigQueryRead_ReadRowsClient, error){
			"readRows fail on first call": func(ctx context.Context, req *storagepb.ReadRowsRequest, opts ...gax.CallOption) (storagepb.BigQueryRead_ReadRowsClient, error) {
				if len(tc.errors) == 0 {
					return &testReadRowsClient{}, nil
				}
				err := tc.errors[0]
				tc.errors = tc.errors[1:]
				if err != nil {
					return nil, err
				}
				return &testReadRowsClient{}, nil
			},
			"readRows fails on Recv": func(ctx context.Context, req *storagepb.ReadRowsRequest, opts ...gax.CallOption) (storagepb.BigQueryRead_ReadRowsClient, error) {
				return rrc, nil
			},
		}
		for readRowsFuncType, readRowsFunc := range readRowsFuncs {
			baseCtx := tc.ctx
			if baseCtx == nil {
				baseCtx = context.Background()
			}
			ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
			defer cancel()

			it, err := newRawStorageRowIterator(&readSession{
				ctx:          ctx,
				settings:     defaultReadClientSettings(),
				readRowsFunc: readRowsFunc,
				bqSession:    &storagepb.ReadSession{},
			}, Schema{})
			if err != nil {
				t.Fatalf("case %s: newRawStorageRowIterator: %v", tc.desc, err)
			}

			it.processStream("test-stream")

			if errors.Is(it.ctx.Err(), context.Canceled) || errors.Is(it.ctx.Err(), context.DeadlineExceeded) {
				if tc.wantFail {
					continue
				}
				t.Fatalf("case %s(%s): deadline exceeded", tc.desc, readRowsFuncType)
			}
			if tc.wantFail && len(it.errs) == 0 {
				t.Fatalf("case %s(%s):want test to fail, but found no errors", tc.desc, readRowsFuncType)
			}
			if !tc.wantFail && len(it.errs) > 0 {
				t.Fatalf("case %s(%s):test should not fail, but found %d errors", tc.desc, readRowsFuncType, len(it.errs))
			}
		}
	}
}

type testReadRowsClient struct {
	storagepb.BigQueryRead_ReadRowsClient
	responses []*storagepb.ReadRowsResponse
	errors    []error
}

func (trrc *testReadRowsClient) Recv() (*storagepb.ReadRowsResponse, error) {
	if len(trrc.errors) > 0 {
		err := trrc.errors[0]
		trrc.errors = trrc.errors[1:]
		return nil, err
	}
	if len(trrc.responses) > 0 {
		r := trrc.responses[0]
		trrc.responses = trrc.responses[:1]
		return r, nil
	}
	return nil, io.EOF
}
