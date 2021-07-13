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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// NoStreamOffset is a sentinel value for signalling we're not tracking
// stream offset (e.g. a default stream which allows simultaneous append streams).
const NoStreamOffset int64 = -1

// AppendResult tracks the status of a single row of data.
type AppendResult struct {
	// rowData contains the serialized row data.
	rowData []byte

	ready chan struct{}

	// if the encapsulating append failed, this will retain a reference to the error.
	err error

	// the stream offset
	offset int64
}

func newAppendResult(data []byte) *AppendResult {
	return &AppendResult{
		ready:   make(chan struct{}),
		rowData: data,
	}
}

// Ready blocks until the append request is completed.
func (ar *AppendResult) Ready() <-chan struct{} { return ar.ready }

// GetResult returns the optional offset of this row, or the associated
// error.
func (ar *AppendResult) GetResult(ctx context.Context) (int64, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-ar.Ready():
		return ar.offset, ar.err
	}
}

// pendingWrite tracks state for a set of rows that are part of a single
// append request.
type pendingWrite struct {
	request *storagepb.AppendRowsRequest
	results []*AppendResult

	// this is used by the flow controller.
	reqSize int
}

// newPendingWrite constructs the proto request and attaches references
// to the pending results for later consumption.  The reason for this is
// that in the future, we may want to allow row batching to be managed by
// the server (e.g. for default/COMMITTED streams).  For BUFFERED/PENDING
// streams, this should be managed by the user.
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
	// We compute the size now for flow controller purposes, though
	// the actual request size may be slightly larger (e.g. the first
	// request in a new stream bears schema and stream id).
	pw.reqSize = proto.Size(pw.request)
	return pw
}

// markDone propagates finalization of an append request to associated
// AppendResult references.
func (pw *pendingWrite) markDone(startOffset int64, err error) {
	curOffset := startOffset
	for _, ar := range pw.results {
		if err != nil {
			ar.err = err
			close(ar.ready)
			continue
		}

		ar.offset = curOffset
		// only advance curOffset if we were given a valid starting offset.
		if startOffset >= 0 {
			curOffset = curOffset + 1
		}
		close(ar.ready)
	}
	// Clear the reference to the request.
	pw.request = nil
}
