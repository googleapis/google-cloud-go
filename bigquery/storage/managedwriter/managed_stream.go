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
	"fmt"
	"io"
	"sync"
	"time"

	"cloud.google.com/go/bigquery/internal"
	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"github.com/googleapis/gax-go/v2"
	"go.opencensus.io/tag"
	"google.golang.org/grpc"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// StreamType indicates the type of stream this write client is managing.
type StreamType string

var (
	// DefaultStream most closely mimics the legacy bigquery
	// tabledata.insertAll semantics.  Successful inserts are
	// committed immediately, and there's no tracking offsets as
	// all writes go into a "default" stream that always exists
	// for a table.
	DefaultStream StreamType = "DEFAULT"

	// CommittedStream appends data immediately, but creates a
	// discrete stream for the work so that offset tracking can
	// be used to track writes.
	CommittedStream StreamType = "COMMITTED"

	// BufferedStream is a form of checkpointed stream, that allows
	// you to advance the offset of visible rows via Flush operations.
	BufferedStream StreamType = "BUFFERED"

	// PendingStream is a stream in which no data is made visible to
	// readers until the stream is finalized and committed explicitly.
	PendingStream StreamType = "PENDING"
)

func streamTypeToEnum(t StreamType) storagepb.WriteStream_Type {
	switch t {
	case CommittedStream:
		return storagepb.WriteStream_COMMITTED
	case PendingStream:
		return storagepb.WriteStream_PENDING
	case BufferedStream:
		return storagepb.WriteStream_BUFFERED
	default:
		return storagepb.WriteStream_TYPE_UNSPECIFIED
	}
}

// ManagedStream is the abstraction over a single write stream.
type ManagedStream struct {
	// Unique id for the managedstream instance.
	id string

	// pool retains a reference to the writer's pool.  A writer is only associated to a single pool.
	pool *connectionPool

	streamSettings *streamSettings
	// retains the current descriptor for the stream.
	curTemplate *versionedTemplate
	c           *Client
	retry       *statelessRetryer

	// writer state
	mu     sync.Mutex
	ctx    context.Context // used for stats/instrumentation, and to check the writer is live.
	cancel context.CancelFunc
	err    error // retains any terminal error (writer was closed)
}

// streamSettings is for capturing configuration and option information.
type streamSettings struct {

	// streamID contains the reference to the destination stream.
	streamID string

	// streamType governs behavior of the client, such as how
	// offset handling is managed.
	streamType StreamType

	// MaxInflightRequests governs how many unacknowledged
	// append writes can be outstanding into the system.
	MaxInflightRequests int

	// MaxInflightBytes governs how many unacknowledged
	// request bytes can be outstanding into the system.
	MaxInflightBytes int

	// TraceID can be set when appending data on a stream. It's
	// purpose is to aid in debug and diagnostic scenarios.
	TraceID string

	// dataOrigin can be set for classifying metrics generated
	// by a stream.
	dataOrigin string

	// retains reference to the target table when resolving settings
	destinationTable string

	appendCallOptions []gax.CallOption

	// enable multiplex?
	multiplex bool

	// retain a copy of the stream client func.
	streamFunc streamClientFunc
}

func defaultStreamSettings() *streamSettings {
	return &streamSettings{
		streamType:          DefaultStream,
		MaxInflightRequests: 1000,
		MaxInflightBytes:    0,
		appendCallOptions: []gax.CallOption{
			gax.WithGRPCOptions(grpc.MaxCallRecvMsgSize(10 * 1024 * 1024)),
		},
	}
}

// buildTraceID handles prefixing of a user-supplied trace ID with a client identifier.
func buildTraceID(s *streamSettings) string {
	base := fmt.Sprintf("go-managedwriter:%s", internal.Version)
	if s != nil && s.TraceID != "" {
		return fmt.Sprintf("%s %s", base, s.TraceID)
	}
	return base
}

// StreamName returns the corresponding write stream ID being managed by this writer.
func (ms *ManagedStream) StreamName() string {
	return ms.streamSettings.streamID
}

// StreamType returns the configured type for this stream.
func (ms *ManagedStream) StreamType() StreamType {
	return ms.streamSettings.streamType
}

// FlushRows advances the offset at which rows in a BufferedStream are visible.  Calling
// this method for other stream types yields an error.
func (ms *ManagedStream) FlushRows(ctx context.Context, offset int64, opts ...gax.CallOption) (int64, error) {
	req := &storagepb.FlushRowsRequest{
		WriteStream: ms.streamSettings.streamID,
		Offset: &wrapperspb.Int64Value{
			Value: offset,
		},
	}
	resp, err := ms.c.rawClient.FlushRows(ctx, req, opts...)
	recordWriterStat(ms, FlushRequests, 1)
	if err != nil {
		return 0, err
	}
	return resp.GetOffset(), nil
}

// Finalize is used to mark a stream as complete, and thus ensure no further data can
// be appended to the stream.  You cannot finalize a DefaultStream, as it always exists.
//
// Finalizing does not advance the current offset of a BufferedStream, nor does it commit
// data in a PendingStream.
func (ms *ManagedStream) Finalize(ctx context.Context, opts ...gax.CallOption) (int64, error) {
	// TODO: consider blocking for in-flight appends once we have an appendStream plumbed in.
	req := &storagepb.FinalizeWriteStreamRequest{
		Name: ms.streamSettings.streamID,
	}
	resp, err := ms.c.rawClient.FinalizeWriteStream(ctx, req, opts...)
	if err != nil {
		return 0, err
	}
	return resp.GetRowCount(), nil
}

// appendWithRetry handles the details of adding sending an append request on a stream.  Appends are sent on a long
// lived bidirectional network stream, with it's own managed context (ms.ctx), and there's a per-request context
// attached to the pendingWrite.
func (ms *ManagedStream) appendWithRetry(pw *pendingWrite, opts ...gax.CallOption) error {
	for {
		ms.mu.Lock()
		err := ms.err
		ms.mu.Unlock()
		if err != nil {
			return err
		}
		conn, err := ms.pool.selectConn(pw)
		if err != nil {
			pw.markDone(nil, err)
			return err
		}
		appendErr := conn.lockingAppend(pw)
		if appendErr != nil {
			// Append yielded an error.  Retry by continuing or return.
			status := grpcstatus.Convert(appendErr)
			if status != nil {
				recordCtx := ms.ctx
				if ctx, err := tag.New(ms.ctx, tag.Insert(keyError, status.Code().String())); err == nil {
					recordCtx = ctx
				}
				recordStat(recordCtx, AppendRequestErrors, 1)
			}
			bo, shouldRetry := ms.statelessRetryer().Retry(appendErr, pw.attemptCount)
			if shouldRetry {
				if err := gax.Sleep(ms.ctx, bo); err != nil {
					return err
				}
				continue
			}
			// This append cannot be retried locally.  It is not the responsibility of this function to finalize the pending
			// write however, as that's handled by callers.
			// Related: https://github.com/googleapis/google-cloud-go/issues/7380
			return appendErr
		}
		return nil
	}
}

// Close closes a managed stream.
func (ms *ManagedStream) Close() error {

	ms.mu.Lock()
	defer ms.mu.Unlock()

	var returned error

	if ms.pool != nil {
		if err := ms.pool.removeWriter(ms); err != nil {
			returned = err
		}
	}

	// Cancel the underlying context for the stream, we don't allow re-open.
	if ms.cancel != nil {
		ms.cancel()
		ms.cancel = nil
	}

	// For normal operation, mark the stream error as io.EOF.
	if ms.err == nil {
		ms.err = io.EOF
	}
	if returned == nil {
		returned = ms.err
	}
	return returned
}

// buildRequest constructs an optimized AppendRowsRequest.
// Offset (if specified) is applied later.
func (ms *ManagedStream) buildRequest(data [][]byte) *storagepb.AppendRowsRequest {
	return &storagepb.AppendRowsRequest{
		Rows: &storagepb.AppendRowsRequest_ProtoRows{
			ProtoRows: &storagepb.AppendRowsRequest_ProtoData{
				Rows: &storagepb.ProtoRows{
					SerializedRows: data,
				},
			},
		},
	}
}

// AppendRows sends the append requests to the service, and returns a single AppendResult for tracking
// the set of data.
//
// The format of the row data is binary serialized protocol buffer bytes.  The message must be compatible
// with the schema currently set for the stream.
//
// Use the WithOffset() AppendOption to set an explicit offset for this append.  Setting an offset for
// a default stream is unsupported.
//
// The size of a single request must be less than 10 MB in size.
// Requests larger than this return an error, typically `INVALID_ARGUMENT`.
func (ms *ManagedStream) AppendRows(ctx context.Context, data [][]byte, opts ...AppendOption) (*AppendResult, error) {
	// before we do anything, ensure the writer isn't closed.
	ms.mu.Lock()
	err := ms.err
	ms.mu.Unlock()
	if err != nil {
		return nil, err
	}
	// Ensure we build the request and pending write with a consistent schema version.
	curTemplate := ms.curTemplate
	req := ms.buildRequest(data)
	pw := newPendingWrite(ctx, ms, req, curTemplate, ms.streamSettings.streamID, ms.streamSettings.TraceID)
	// apply AppendOption opts
	for _, opt := range opts {
		opt(pw)
	}
	// Post-request fixup after options are applied.
	if pw.reqTmpl != nil {
		if pw.reqTmpl.tmpl != nil {
			// MVIs must be set on each request, but _default_ MVIs persist across the stream lifetime.  Sigh.
			pw.req.MissingValueInterpretations = pw.reqTmpl.tmpl.GetMissingValueInterpretations()
		}
	}

	// Call the underlying append.  The stream has it's own retained context and will surface expiry on
	// it's own, but we also need to respect any deadline for the provided context.
	errCh := make(chan error)
	var appendErr error
	go func() {
		select {
		case errCh <- ms.appendWithRetry(pw):
		case <-ctx.Done():
		case <-ms.ctx.Done():
		}
		close(errCh)
	}()
	select {
	case <-ctx.Done():
		// It is incorrect to simply mark the request done, as it's potentially in flight in the bidi stream
		// where we can't propagate a cancellation.  Our options are to return the pending write even though
		// it's in an ambiguous state, or to return the error and simply drop the pending write on the floor.
		//
		// This API expresses request idempotency through offset management, so users who care to use offsets
		// can deal with the dropped request.
		return nil, ctx.Err()
	case <-ms.ctx.Done():
		// Same as the request context being done, this indicates the writer context expired.  For this case,
		// we also attempt to close the writer.
		ms.mu.Lock()
		if ms.err == nil {
			ms.err = ms.ctx.Err()
		}
		ms.mu.Unlock()
		ms.Close()
		// Don't relock to fetch the writer terminal error, as we've already ensured that the writer is closed.
		return nil, ms.err
	case appendErr = <-errCh:
		if appendErr != nil {
			return nil, appendErr
		}
		return pw.result, nil
	}
}

// processRetry is responsible for evaluating and re-enqueing an append.
// If the append is not retried, it is marked complete.
func (ms *ManagedStream) processRetry(pw *pendingWrite, srcConn *connection, appendResp *storagepb.AppendRowsResponse, initialErr error) {
	err := initialErr
	for {
		pause, shouldRetry := ms.statelessRetryer().Retry(err, pw.attemptCount)
		if !shouldRetry {
			// Should not attempt to re-append.
			pw.markDone(appendResp, err)
			return
		}
		time.Sleep(pause)
		err = ms.appendWithRetry(pw)
		if err != nil {
			// Re-enqueue failed, send it through the loop again.
			continue
		}
		// Break out of the loop, we were successful and the write has been
		// re-inserted.
		recordWriterStat(ms, AppendRetryCount, 1)
		break
	}
}

// returns the stateless retryer.  If one's not set (re-enqueue retries disabled),
// it returns a retryer that only permits single attempts.
func (ms *ManagedStream) statelessRetryer() *statelessRetryer {
	if ms.retry != nil {
		return ms.retry
	}
	if ms.pool != nil {
		return ms.pool.defaultRetryer()
	}
	return &statelessRetryer{
		maxAttempts: 1,
	}
}
