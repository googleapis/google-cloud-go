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

	"cloud.google.com/go/bigquery/internal"
	"github.com/googleapis/gax-go/v2"
	"go.opencensus.io/tag"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
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
	streamSettings   *streamSettings
	schemaDescriptor *descriptorpb.DescriptorProto
	destinationTable string
	c                *Client
	fc               *flowController

	// aspects of the stream client
	ctx         context.Context // retained context for the stream
	cancel      context.CancelFunc
	callOptions []gax.CallOption                                                                                // options passed when opening an append client
	open        func(streamID string, opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) // how we get a new connection

	mu          sync.Mutex
	arc         *storagepb.BigQueryWrite_AppendRowsClient // current stream connection
	reconnect   bool                                      // Request a reconnect before issuing another send.
	err         error                                     // terminal error
	pending     chan *pendingWrite                        // writes awaiting status
	streamSetup *sync.Once                                // handles amending the first request in a new stream
}

// enables testing
type streamClientFunc func(context.Context, ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error)

// streamSettings govern behavior of the append stream RPCs.
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
}

func defaultStreamSettings() *streamSettings {
	return &streamSettings{
		streamType:          DefaultStream,
		MaxInflightRequests: 1000,
		MaxInflightBytes:    0,
		TraceID:             buildTraceID(""),
	}
}

func buildTraceID(id string) string {
	base := fmt.Sprintf("go-managedwriter:%s", internal.Version)
	if id != "" {
		return fmt.Sprintf("%s %s", base, id)
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
	recordStat(ms.ctx, FlushRequests, 1)
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

// getStream returns either a valid ARC client stream or permanent error.
//
// Any calls to getStream should do so in possesion of the critical section lock.
func (ms *ManagedStream) getStream(arc *storagepb.BigQueryWrite_AppendRowsClient, forceReconnect bool) (*storagepb.BigQueryWrite_AppendRowsClient, chan *pendingWrite, error) {
	if ms.err != nil {
		return nil, nil, ms.err
	}
	ms.err = ms.ctx.Err()
	if ms.err != nil {
		return nil, nil, ms.err
	}

	// Previous activity on the stream indicated it is not healthy, so propagate that as a reconnect.
	if ms.reconnect {
		forceReconnect = true
		ms.reconnect = false
	}
	// Always return the retained ARC if the arg differs.
	if arc != ms.arc && !forceReconnect {
		return ms.arc, ms.pending, nil
	}
	if arc != ms.arc && forceReconnect && ms.arc != nil {
		// In this case, we're forcing a close on the existing stream.
		// This is due to either needing to reconnect to satisfy the needs of
		// the current request (e.g. to signal a schema change), or because
		// a previous request on the stream yielded a transient error and we
		// want to reconnect before issuing a subsequent request.
		//
		// TODO: clean this up once internal issue 205756033 is resolved.
		(*ms.arc).CloseSend()
	}

	ms.arc = new(storagepb.BigQueryWrite_AppendRowsClient)
	*ms.arc, ms.pending, ms.err = ms.openWithRetry()
	return ms.arc, ms.pending, ms.err
}

// openWithRetry is responsible for navigating the (re)opening of the underlying stream connection.
//
// Only getStream() should call this.
func (ms *ManagedStream) openWithRetry() (storagepb.BigQueryWrite_AppendRowsClient, chan *pendingWrite, error) {
	r := defaultRetryer{}
	for {
		recordStat(ms.ctx, AppendClientOpenCount, 1)
		streamID := ""
		if ms.streamSettings != nil {
			streamID = ms.streamSettings.streamID
		}
		arc, err := ms.open(streamID, ms.callOptions...)
		bo, shouldRetry := r.Retry(err)
		if err != nil && shouldRetry {
			recordStat(ms.ctx, AppendClientOpenRetryCount, 1)
			if err := gax.Sleep(ms.ctx, bo); err != nil {
				return nil, nil, err
			}
			continue
		}
		if err == nil {
			// The channel relationship with its ARC is 1:1.  If we get a new ARC, create a new pending
			// write channel and fire up the associated receive processor.  The channel ensures that
			// responses for a connection are processed in the same order that appends were sent.
			depth := 1000 // default backend queue limit
			if ms.streamSettings != nil {
				if ms.streamSettings.MaxInflightRequests > 0 {
					depth = ms.streamSettings.MaxInflightRequests
				}
			}
			ch := make(chan *pendingWrite, depth)
			go recvProcessor(ms.ctx, arc, ms.fc, ch)
			// Also, replace the sync.Once for setting up a new stream, as we need to do "special" work
			// for every new connection.
			ms.streamSetup = new(sync.Once)
			return arc, ch, nil
		}
		return arc, nil, err
	}
}

// lockingAppend handles a single append attempt.  When successful, it returns the number of rows
// in the request for metrics tracking.
func (ms *ManagedStream) lockingAppend(pw *pendingWrite) error {

	// Don't both calling/retrying if this append's context is already expired.
	if err := pw.reqCtx.Err(); err != nil {
		return err
	}

	// we use this to record stats if needed after we unlock on defer.
	var statsOnExit func()

	// critical section:  Things that need to happen inside the critical section:
	//
	// * Getting the stream connection (in case of reconnects)
	// * Issuing the append request
	// * Adding the pending write to the channel to keep ordering correct on response
	ms.mu.Lock()
	defer func() {
		ms.mu.Unlock()
		if statsOnExit != nil {
			statsOnExit()
		}
	}()

	var arc *storagepb.BigQueryWrite_AppendRowsClient
	var ch chan *pendingWrite
	var err error

	// If an updated schema is present, we need to reconnect the stream and update the reference
	// schema for the stream.
	reconnect := false
	if pw.newSchema != nil && !proto.Equal(pw.newSchema, ms.schemaDescriptor) {
		reconnect = true
		ms.schemaDescriptor = proto.Clone(pw.newSchema).(*descriptorpb.DescriptorProto)
	}
	arc, ch, err = ms.getStream(arc, reconnect)
	if err != nil {
		return err
	}

	// Resolve the special work for the first append on a stream.
	var req *storagepb.AppendRowsRequest
	ms.streamSetup.Do(func() {
		reqCopy := proto.Clone(pw.request).(*storagepb.AppendRowsRequest)
		reqCopy.WriteStream = ms.streamSettings.streamID
		reqCopy.GetProtoRows().WriterSchema = &storagepb.ProtoSchema{
			ProtoDescriptor: ms.schemaDescriptor,
		}
		if ms.streamSettings.TraceID != "" {
			reqCopy.TraceId = ms.streamSettings.TraceID
		}
		req = reqCopy
	})

	if req != nil {
		// First append in a new connection needs properties like schema and stream name set.
		err = (*arc).Send(req)
	} else {
		// Subsequent requests need no modification.
		err = (*arc).Send(pw.request)
	}
	if err != nil {
		if shouldReconnect(err) {
			ms.reconnect = true
		}
		return err
	}
	// Compute numRows, once we pass ownership to the channel the request may be
	// cleared.
	numRows := int64(len(pw.request.GetProtoRows().Rows.GetSerializedRows()))
	statsOnExit = func() {
		// these will get recorded once we exit the critical section.
		recordStat(ms.ctx, AppendRequestRows, numRows)
		recordStat(ms.ctx, AppendRequests, 1)
		recordStat(ms.ctx, AppendRequestBytes, int64(pw.reqSize))
	}
	ch <- pw
	return nil
}

// appendWithRetry handles the details of adding sending an append request on a stream.  Appends are sent on a long
// lived bidirectional network stream, with it's own managed context (ms.ctx).  requestCtx is checked
// for expiry to enable faster failures, it is not propagated more deeply.
func (ms *ManagedStream) appendWithRetry(pw *pendingWrite, opts ...gax.CallOption) error {

	// Resolve retry settings.
	var settings gax.CallSettings
	for _, opt := range opts {
		opt.Resolve(&settings)
	}
	var r gax.Retryer = &defaultRetryer{}
	if settings.Retry != nil {
		r = settings.Retry()
	}

	for {
		appendErr := ms.lockingAppend(pw)
		if appendErr != nil {
			// Append yielded an error.  Retry by continuing or return.
			status := grpcstatus.Convert(appendErr)
			if status != nil {
				ctx, _ := tag.New(ms.ctx, tag.Insert(keyError, status.Code().String()))
				recordStat(ctx, AppendRequestErrors, 1)
			}
			bo, shouldRetry := r.Retry(appendErr)
			if shouldRetry {
				if err := gax.Sleep(ms.ctx, bo); err != nil {
					return err
				}
				continue
			}
			// Mark the pending write done.  This will not be returned to the user, they'll receive the returned error.
			pw.markDone(nil, appendErr, ms.fc)
			return appendErr
		}
		return nil
	}
}

// Close closes a managed stream.
func (ms *ManagedStream) Close() error {
	// Critical section: get connection, close, mark closed.
	ms.mu.Lock()
	defer ms.mu.Unlock()

	var arc *storagepb.BigQueryWrite_AppendRowsClient
	arc, ch, err := ms.getStream(arc, false)
	if err != nil {
		return err
	}
	if ms.arc == nil {
		return fmt.Errorf("no stream exists")
	}
	err = (*arc).CloseSend()
	// Regardless of the outcome of CloseSend(), we're done with this channel.
	close(ch)
	// Additionally, cancel the underlying context for the stream, we don't allow re-open.
	if ms.cancel != nil {
		ms.cancel()
		ms.cancel = nil
	}

	if err != nil {
		// For error on CloseSend, save that as the stream error and return.
		ms.err = err
		return err
	}
	// For normal operation, mark the stream error as io.EOF and return.
	ms.err = io.EOF
	return nil
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
	pw := newPendingWrite(ctx, data)
	// apply AppendOption opts
	for _, opt := range opts {
		opt(pw)
	}
	// check flow control
	if err := ms.fc.acquire(ctx, pw.reqSize); err != nil {
		// in this case, we didn't acquire, so don't pass the flow controller reference to avoid a release.
		pw.markDone(nil, err, nil)
		return nil, err
	}
	// Call the underlying append.  The stream has it's own retained context and will surface expiry on
	// it's own, but we also need to respect any deadline for the provided context.
	errCh := make(chan error)
	var appendErr error
	go func() {
		select {
		case errCh <- ms.appendWithRetry(pw):
		case <-ctx.Done():
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
	case appendErr = <-errCh:
		if appendErr != nil {
			return nil, appendErr
		}
		return pw.result, nil
	}
}

// recvProcessor is used to propagate append responses back up with the originating write requests in a goroutine.
//
// The receive processor only deals with a single instance of a connection/channel, and thus should never interact
// with the mutex lock.
func recvProcessor(ctx context.Context, arc storagepb.BigQueryWrite_AppendRowsClient, fc *flowController, ch <-chan *pendingWrite) {
	// TODO:  We'd like to re-send requests that are in an ambiguous state due to channel errors.  For now, we simply
	// ensure that pending writes get acknowledged with a terminal state.
	for {
		select {
		case <-ctx.Done():
			// Context is done, so we're not going to get further updates.  Mark all work failed with the context error.
			for {
				pw, ok := <-ch
				if !ok {
					return
				}
				pw.markDone(nil, ctx.Err(), fc)
			}
		case nextWrite, ok := <-ch:
			if !ok {
				// Channel closed, all elements processed.
				return
			}

			// block until we get a corresponding response or err from stream.
			resp, err := arc.Recv()
			if err != nil {
				nextWrite.markDone(nil, err, fc)
				continue
			}
			recordStat(ctx, AppendResponses, 1)

			if status := resp.GetError(); status != nil {
				tagCtx, _ := tag.New(ctx, tag.Insert(keyError, codes.Code(status.GetCode()).String()))
				if err != nil {
					tagCtx = ctx
				}
				recordStat(tagCtx, AppendResponseErrors, 1)
			}
			nextWrite.markDone(resp, nil, fc)
		}
	}
}
