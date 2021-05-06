// Copyright 2021 Google LLC
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

	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta2"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// AppendResult is used to retrieve the state of an individual row append.
type AppendResult struct {
	// Only retain the serialized row data.
	rowData []byte
	ready   chan struct{}
	err     error
	offset  int64
}

func newAppendResult(data []byte) *AppendResult {
	return &AppendResult{
		ready:   make(chan struct{}),
		rowData: data,
	}
}

func (ar *AppendResult) Ready() <-chan struct{} { return ar.ready }

func (ar *AppendResult) GetResult(ctx context.Context) (int64, error) {
	select {
	case <-ar.Ready():
		return ar.offset, ar.err
	default:
	}

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-ar.Ready():
		return ar.offset, ar.err
	}
}

type pendingWrite struct {
	request *storagepb.AppendRowsRequest
	results []*AppendResult
	reqSize int
}

// newPendingWrite constructs the proto request and attaches references
// to the pending results for later consumption.
func newPendingWrite(appends [][]byte, offset int64) *pendingWrite {

	results := make([]*AppendResult, len(appends))
	for k, r := range appends {
		results[k] = newAppendResult(r)
	}
	pw := &pendingWrite{
		request: &storagepb.AppendRowsRequest{
			Rows: &storagepb.AppendRowsRequest_ProtoRows{
				ProtoRows: &storagepb.AppendRowsRequest_ProtoData{
					Rows: &storagepb.ProtoRows{
						SerializedRows: appends,
					},
				},
			},
		},
		results: results,
	}
	if offset > 0 {
		pw.request.Offset = &wrapperspb.Int64Value{Value: offset}
	}
	return pw
}

func (pw *pendingWrite) markDone(startOffset int64, err error) {
	curOffset := startOffset
	for _, ar := range pw.results {
		if err != nil {
			ar.err = err
			close(ar.ready)
			continue
		}

		ar.offset = curOffset
		// only advance curOffset if we were given a valid start.
		if startOffset >= 0 {
			curOffset = curOffset + 1
		}
		close(ar.ready)
	}
	// clear the reference to the request
	pw.request = nil
}
