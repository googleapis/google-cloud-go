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

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"github.com/googleapis/gax-go/v2/apierror"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// NoStreamOffset is a sentinel value for signalling we're not tracking
// stream offset (e.g. a default stream which allows simultaneous append streams).
const NoStreamOffset int64 = -1

// AppendResult tracks the status of a batch of data rows.
type AppendResult struct {
	ready chan struct{}

	// if the append failed without a response, this will retain a reference to the error.
	err error

	// retains the original response.
	response *storagepb.AppendRowsResponse

	// retains the number of times this individual write was enqueued.
	totalAttempts int
}

func newAppendResult() *AppendResult {
	return &AppendResult{
		ready: make(chan struct{}),
	}
}

// Ready blocks until the append request has reached a completed state,
// which may be a successful append or an error.
func (ar *AppendResult) Ready() <-chan struct{} { return ar.ready }

// GetResult returns the optional offset of this row, as well as any error encountered while
// processing the append.
//
// This call blocks until the result is ready, or context is no longer valid.
func (ar *AppendResult) GetResult(ctx context.Context) (int64, error) {
	select {
	case <-ctx.Done():
		return NoStreamOffset, ctx.Err()
	case <-ar.Ready():
		full, err := ar.FullResponse(ctx)
		offset := NoStreamOffset
		if full != nil {
			if result := full.GetAppendResult(); result != nil {
				if off := result.GetOffset(); off != nil {
					offset = off.GetValue()
				}
			}
		}
		return offset, err
	}
}

// FullResponse returns the full content of the AppendRowsResponse, and any error encountered while
// processing the append.
//
// The AppendRowResponse may contain an embedded error.  An embedded error in the response will be
// converted and returned as the error response, so this method may return both the
// AppendRowsResponse and an error.
//
// This call blocks until the result is ready, or context is no longer valid.
func (ar *AppendResult) FullResponse(ctx context.Context) (*storagepb.AppendRowsResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-ar.Ready():
		var err error
		if ar.err != nil {
			err = ar.err
		} else {
			if ar.response != nil {
				if status := ar.response.GetError(); status != nil {
					statusErr := grpcstatus.ErrorProto(status)
					// Provide an APIError if possible.
					if apiErr, ok := apierror.FromError(statusErr); ok {
						err = apiErr
					} else {
						err = statusErr
					}
				}
			}
		}
		if ar.response != nil {
			return proto.Clone(ar.response).(*storagepb.AppendRowsResponse), err
		}
		return nil, err
	}
}

func (ar *AppendResult) offset(ctx context.Context) int64 {
	select {
	case <-ctx.Done():
		return NoStreamOffset
	case <-ar.Ready():
		if ar.response != nil {
			if result := ar.response.GetAppendResult(); result != nil {
				if off := result.GetOffset(); off != nil {
					return off.GetValue()
				}
			}
		}
		return NoStreamOffset
	}
}

// UpdatedSchema returns the updated schema for a table if supplied by the backend as part
// of the append response.
//
// This call blocks until the result is ready, or context is no longer valid.
func (ar *AppendResult) UpdatedSchema(ctx context.Context) (*storagepb.TableSchema, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-ar.Ready():
		if ar.response != nil {
			if schema := ar.response.GetUpdatedSchema(); schema != nil {
				return proto.Clone(schema).(*storagepb.TableSchema), nil
			}
		}
		return nil, nil
	}
}

// TotalAttempts returns the number of times this write was attempted.
//
// This call blocks until the result is ready, or context is no longer valid.
func (ar *AppendResult) TotalAttempts(ctx context.Context) (int, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-ar.Ready():
		return ar.totalAttempts, nil
	}
}

// pendingWrite tracks state for a set of rows that are part of a single
// append request.
type pendingWrite struct {
	// writer retains a reference to the origin of a pending write.  Primary
	// used is to inform routing decisions.
	writer *ManagedStream

	// We store the request as it's simplex-optimized form, as statistically that's the most
	// likely outcome when processing requests and it allows us to be efficient on send.
	// We retain the additional information to build the complete request in the related fields.
	req           *storagepb.AppendRowsRequest
	reqTmpl       *versionedTemplate // request template at time of creation
	traceID       string
	writeStreamID string

	// Reference to the AppendResult which is exposed to the user.
	result *AppendResult

	// Flow control is based on the unoptimized request size.
	reqSize int

	// retains the original request context, primarily for checking against
	// cancellation signals.
	reqCtx context.Context

	// tracks the number of times we've attempted this append request.
	attemptCount int
}

// newPendingWrite constructs the proto request and attaches references
// to the pending results for later consumption.  The provided context is
// embedded in the pending write, as the write may be retried and we want
// to respect the original context for expiry/cancellation etc.
func newPendingWrite(ctx context.Context, src *ManagedStream, req *storagepb.AppendRowsRequest, reqTmpl *versionedTemplate, writeStreamID, traceID string) *pendingWrite {
	pw := &pendingWrite{
		writer: src,
		result: newAppendResult(),
		reqCtx: ctx,

		req:           req,     // minimal req, typically just row data
		reqTmpl:       reqTmpl, // remainder of templated request
		writeStreamID: writeStreamID,
		traceID:       traceID,
	}
	// Compute the approx size for flow control purposes.
	pw.reqSize = proto.Size(pw.req) + len(writeStreamID) + len(traceID)
	if pw.reqTmpl != nil {
		pw.reqSize += proto.Size(pw.reqTmpl.tmpl)
	}
	return pw
}

// markDone propagates finalization of an append request to the associated
// AppendResult.
func (pw *pendingWrite) markDone(resp *storagepb.AppendRowsResponse, err error) {
	// First, propagate necessary state from the pendingWrite to the final result.
	if resp != nil {
		pw.result.response = resp
	}
	pw.result.err = err
	pw.result.totalAttempts = pw.attemptCount

	// Close the result's ready channel.
	close(pw.result.ready)
	// Cleanup references remaining on the write explicitly.
	pw.req = nil
	pw.reqTmpl = nil
	pw.writer = nil
	pw.reqCtx = nil
}

func (pw *pendingWrite) constructFullRequest(addTrace bool) *storagepb.AppendRowsRequest {
	req := &storagepb.AppendRowsRequest{}
	if pw.reqTmpl != nil {
		req = proto.Clone(pw.reqTmpl.tmpl).(*storagepb.AppendRowsRequest)
	}
	if pw.req != nil {
		proto.Merge(req, pw.req)
	}
	if addTrace {
		req.TraceId = buildTraceID(&streamSettings{TraceID: pw.traceID})
	}
	req.WriteStream = pw.writeStreamID
	return req
}
