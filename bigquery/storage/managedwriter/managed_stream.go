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

	"github.com/googleapis/gax-go/v2"
	"go.opencensus.io/tag"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
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
	//
	// NOTE: Buffered Streams are currently in limited preview, and as such
	// methods like FlushRows() may yield errors for non-enrolled projects.
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
		TraceID:             "",
	}
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
// Calling getStream locks the mutex.
func (ms *ManagedStream) getStream(arc *storagepb.BigQueryWrite_AppendRowsClient) (*storagepb.BigQueryWrite_AppendRowsClient, chan *pendingWrite, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if ms.err != nil {
		return nil, nil, ms.err
	}
	ms.err = ms.ctx.Err()
	if ms.err != nil {
		return nil, nil, ms.err
	}

	// Always return the retained ARC if the arg differs.
	if arc != ms.arc {
		return ms.arc, ms.pending, nil
	}

	ms.arc = new(storagepb.BigQueryWrite_AppendRowsClient)
	*ms.arc, ms.pending, ms.err = ms.openWithRetry()
	return ms.arc, ms.pending, ms.err
}

// openWithRetry is responsible for navigating the (re)opening of the underlying stream connection.
//
// Only getStream() should call this, and thus the calling code has the mutex lock.
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
			// The channel relationship with its ARC is 1:1.  If we get a new ARC, create a new chan
			// and fire up the associated receive processor.
			ch := make(chan *pendingWrite)
			go recvProcessor(ms.ctx, arc, ms.fc, ch)
			// Also, replace the sync.Once for setting up a new stream, as we need to do "special" work
			// for every new connection.
			ms.streamSetup = new(sync.Once)
			return arc, ch, nil
		}
		return arc, nil, err
	}
}

func (ms *ManagedStream) append(pw *pendingWrite, opts ...gax.CallOption) error {
	var settings gax.CallSettings
	for _, opt := range opts {
		opt.Resolve(&settings)
	}
	var r gax.Retryer = &defaultRetryer{}
	if settings.Retry != nil {
		r = settings.Retry()
	}

	var arc *storagepb.BigQueryWrite_AppendRowsClient
	var ch chan *pendingWrite
	var err error

	for {
		arc, ch, err = ms.getStream(arc)
		if err != nil {
			return err
		}
		var req *storagepb.AppendRowsRequest
		ms.streamSetup.Do(func() {
			reqCopy := *pw.request
			reqCopy.WriteStream = ms.streamSettings.streamID
			reqCopy.GetProtoRows().WriterSchema = &storagepb.ProtoSchema{
				ProtoDescriptor: ms.schemaDescriptor,
			}
			if ms.streamSettings.TraceID != "" {
				reqCopy.TraceId = ms.streamSettings.TraceID
			}
			req = &reqCopy
		})

		var err error
		if req == nil {
			err = (*arc).Send(pw.request)
		} else {
			// we had to amend the initial request
			err = (*arc).Send(req)
		}
		recordStat(ms.ctx, AppendRequests, 1)
		recordStat(ms.ctx, AppendRequestBytes, int64(pw.reqSize))
		recordStat(ms.ctx, AppendRequestRows, int64(len(pw.request.GetProtoRows().Rows.GetSerializedRows())))
		if err != nil {
			status := grpcstatus.Convert(err)
			if status != nil {
				ctx, _ := tag.New(ms.ctx, tag.Insert(keyError, status.Code().String()))
				recordStat(ctx, AppendRequestErrors, 1)
			}
			bo, shouldRetry := r.Retry(err)
			if shouldRetry {
				if err := gax.Sleep(ms.ctx, bo); err != nil {
					return err
				}
				continue
			}
			ms.mu.Lock()
			ms.err = err
			ms.mu.Unlock()
		}
		if err == nil {
			ch <- pw
		}
		return err
	}
}

// Close closes a managed stream.
func (ms *ManagedStream) Close() error {

	var arc *storagepb.BigQueryWrite_AppendRowsClient

	arc, ch, err := ms.getStream(arc)
	if err != nil {
		return err
	}
	if ms.arc == nil {
		return fmt.Errorf("no stream exists")
	}
	err = (*arc).CloseSend()
	if err == nil {
		close(ch)
	}
	ms.mu.Lock()
	ms.err = io.EOF
	ms.mu.Unlock()
	// Propagate cancellation.
	if ms.cancel != nil {
		ms.cancel()
	}
	return err
}

// AppendRows sends the append requests to the service, and returns a single AppendResult for tracking
// the set of data.
//
// The format of the row data is binary serialized protocol buffer bytes, and and the message
// must adhere to the format of the schema Descriptor passed in when creating the managed stream.
func (ms *ManagedStream) AppendRows(ctx context.Context, data [][]byte, offset int64) (*AppendResult, error) {
	pw := newPendingWrite(data, offset)
	// check flow control
	if err := ms.fc.acquire(ctx, pw.reqSize); err != nil {
		// in this case, we didn't acquire, so don't pass the flow controller reference to avoid a release.
		pw.markDone(NoStreamOffset, err, nil)
	}
	// proceed to call
	if err := ms.append(pw); err != nil {
		// pending write is DOA.
		pw.markDone(NoStreamOffset, err, ms.fc)
		return nil, err
	}
	return pw.result, nil
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
				pw.markDone(NoStreamOffset, ctx.Err(), fc)
			}
		case nextWrite, ok := <-ch:
			if !ok {
				// Channel closed, all elements processed.
				return
			}

			// block until we get a corresponding response or err from stream.
			resp, err := arc.Recv()
			if err != nil {
				nextWrite.markDone(NoStreamOffset, err, fc)
				continue
			}
			recordStat(ctx, AppendResponses, 1)

			if status := resp.GetError(); status != nil {
				tagCtx, _ := tag.New(ctx, tag.Insert(keyError, codes.Code(status.GetCode()).String()))
				if err != nil {
					tagCtx = ctx
				}
				recordStat(tagCtx, AppendResponseErrors, 1)
				nextWrite.markDone(NoStreamOffset, grpcstatus.ErrorProto(status), fc)
				continue
			}
			success := resp.GetAppendResult()
			off := success.GetOffset()
			if off != nil {
				nextWrite.markDone(off.GetValue(), nil, fc)
			} else {
				nextWrite.markDone(NoStreamOffset, nil, fc)
			}
		}
	}
}
