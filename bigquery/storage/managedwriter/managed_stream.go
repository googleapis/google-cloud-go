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
	"io"
	"log"
	"sync"

	"github.com/googleapis/gax-go/v2"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta2"
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

	// aspects of the stream client
	ctx     context.Context // retained context for the stream
	cancel  context.CancelFunc
	open    func() (storagepb.BigQueryWrite_AppendRowsClient, error) // how we get a new connection
	arc     *storagepb.BigQueryWrite_AppendRowsClient                // current stream connection
	mu      sync.Mutex
	err     error // terminal error
	pending chan *pendingWrite
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

	// TracePrefix sets a suitable prefix for the trace ID set on
	// append requests.  Useful for diagnostic purposes.
	TracePrefix string
}

func defaultStreamSettings() *streamSettings {
	return &streamSettings{
		streamType:          DefaultStream,
		MaxInflightRequests: 1000,
		MaxInflightBytes:    0,
		TracePrefix:         "defaultManagedWriter",
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
func (ms *ManagedStream) FlushRows(ctx context.Context, offset int64) (int64, error) {
	req := &storagepb.FlushRowsRequest{
		WriteStream: ms.streamSettings.streamID,
		Offset: &wrapperspb.Int64Value{
			Value: offset,
		},
	}
	resp, err := ms.c.rawClient.FlushRows(ctx, req)
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
func (ms *ManagedStream) Finalize(ctx context.Context) (int64, error) {
	// TODO: consider blocking for in-flight appends once we have an appendStream plumbed in.
	req := &storagepb.FinalizeWriteStreamRequest{
		Name: ms.streamSettings.streamID,
	}
	resp, err := ms.c.rawClient.FinalizeWriteStream(ctx, req)
	if err != nil {
		return 0, err
	}
	return resp.GetRowCount(), nil
}

// getStream returns either a valid client or permanent error.
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
func (ms *ManagedStream) openWithRetry() (storagepb.BigQueryWrite_AppendRowsClient, chan *pendingWrite, error) {
	r := defaultRetryer{}
	for {
		arc, err := ms.open()
		bo, shouldRetry := r.Retry(err)
		if err != nil && shouldRetry {
			if err := gax.Sleep(ms.ctx, bo); err != nil {
				return nil, nil, err
			}
			continue
		}
		if err == nil {
			// The channel relationship with its ARC is 1:1.  If we get a new ARC, create a new chan
			// and fire up the associated receive processor.
			ch := make(chan *pendingWrite)
			go recvProcessor(ms.ctx, arc, ch)
			return arc, ch, nil
		}
		return arc, nil, err
	}
}

// call serves as a closure that forwards the call to a (possibly reopened) AppendRowsClient), and the associated
// pendingWrite channel for the ARC connection.
func (ms *ManagedStream) call(f func(storagepb.BigQueryWrite_AppendRowsClient, chan *pendingWrite) error, opts ...gax.CallOption) error {
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
		err = f(*arc, ch)
		if err != nil {
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
		return err
	}
}

func (ms *ManagedStream) append(pw *pendingWrite) error {
	return ms.call(func(arc storagepb.BigQueryWrite_AppendRowsClient, ch chan *pendingWrite) error {
		// TODO: we should only send stream ID and schema for the first message in a new stream, but
		// we need an elegant way to handle this.
		pw.request.WriteStream = ms.streamSettings.streamID
		pw.request.GetProtoRows().WriterSchema = &storagepb.ProtoSchema{
			ProtoDescriptor: ms.schemaDescriptor,
		}
		err := arc.Send(pw.request)
		if err == nil {
			ch <- pw
		}
		return err
	})
}

func (ms *ManagedStream) CloseSend() error {
	err := ms.call(func(arc storagepb.BigQueryWrite_AppendRowsClient, ch chan *pendingWrite) error {
		err := arc.CloseSend()
		if err == nil {
			close(ch)
		}
		return err
	})
	ms.mu.Lock()
	ms.err = io.EOF
	ms.mu.Unlock()
	return err
}

// AppendRows sends the append requests to the service, and returns one AppendResult per row.
func (ms *ManagedStream) AppendRows(data [][]byte, offset int64) ([]*AppendResult, error) {
	pw := newPendingWrite(data, offset)
	if err := ms.append(pw); err != nil {
		// pending write is DOA, mark it done.
		pw.markDone(NoStreamOffset, err)
		return nil, err
	}
	return pw.results, nil
}

// recvProcessor is used to pair responses back up with the origin writes.
func recvProcessor(ctx context.Context, arc storagepb.BigQueryWrite_AppendRowsClient, ch <-chan *pendingWrite) {
	for {
		select {
		case <-ctx.Done():
			// Context is done, so we're not going to get further updates.  However, we need to finalize all remaining
			// writes on the channel so users don't block indefinitely.
			for {
				pw, ok := <-ch
				if !ok {
					return
				}
				pw.markDone(NoStreamOffset, ctx.Err())
			}
		case nextWrite, ok := <-ch:
			if !ok {
				// Channel closed, all elements processed.
				return
			}

			resp, err := arc.Recv()
			if err != nil {
				log.Printf("recv got err: %#v", err)
				nextWrite.markDone(NoStreamOffset, err)
			}

			if status := resp.GetError(); status != nil {
				log.Printf("recv got err status: %#v", status)
				nextWrite.markDone(NoStreamOffset, grpcstatus.ErrorProto(status))
				continue
			}
			success := resp.GetAppendResult()
			off := success.GetOffset()
			if off != nil {
				nextWrite.markDone(off.GetValue(), nil)
			}
			nextWrite.markDone(NoStreamOffset, nil)
		}
	}
}
