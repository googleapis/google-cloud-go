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
	"log"
	"sync"
	"time"

	gax "github.com/googleapis/gax-go/v2"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta2"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// appendStream is an abstraction over the append stream that supports reconnection/retry.
type appendStream struct {
	ctx context.Context
	// Aids debugging.
	traceID string

	open func() (storagepb.BigQueryWrite_AppendRowsClient, error)

	cancel context.CancelFunc

	mu  sync.Mutex
	arc *storagepb.BigQueryWrite_AppendRowsClient
	err error // terminal error.
	fc  *flowController

	sentFirstAppend bool
	schema          *storagepb.ProtoSchema
	streamName      string
	pending         chan *pendingWrite

	// statistics
	// TODO: determine the fate of opencensus vs opentelemtry before release.
	maxOffsetSent     int64
	maxOffsetReceived int64
}

type appendStreamFunc func(context.Context, ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error)

func newAppendStream(ctx context.Context, append appendStreamFunc, fc *flowController, streamName string, schema *storagepb.ProtoSchema, tracePrefix string) *appendStream {
	ctx, cancel := context.WithCancel(ctx)
	as := &appendStream{
		ctx:     ctx,
		traceID: fmt.Sprintf("%s-%d", tracePrefix, time.Now().UnixNano()),
		cancel:  cancel,
		open: func() (storagepb.BigQueryWrite_AppendRowsClient, error) {
			arc, err := append(ctx)
			if err == nil {
				// collect stats
				// here's where I'd send an AppendRequest to init the stream with the stream ID and schema, if the service allowed it.
			}
			if err != nil {
				return nil, err
			}
			return arc, nil
		},
		streamName: streamName,
		schema:     schema,
		pending:    make(chan *pendingWrite),
		fc:         fc,
	}
	return as
}

func (as *appendStream) get(arc *storagepb.BigQueryWrite_AppendRowsClient) (*storagepb.BigQueryWrite_AppendRowsClient, error) {
	as.mu.Lock()
	defer as.mu.Unlock()
	if as.err != nil {
		return nil, as.err
	}
	// if context is done, so are we.
	as.err = as.ctx.Err()
	if as.err != nil {
		return nil, as.err
	}

	// if current and arg AppendRowsClient differ, return the current one.
	// 1. We have an SPC and the caller is getting the stream for the first time.
	// 2. The caller wants to retry, but they have an older SPC; we've already retried.
	if arc != as.arc {
		// confirm: better we do this here or in openWithRetry()?
		as.sentFirstAppend = false
		// TODO: opportunity to drain the non-current client
		return as.arc, nil
	}

	as.arc = new(storagepb.BigQueryWrite_AppendRowsClient)
	*as.arc, as.err = as.openWithRetry()
	return as.arc, as.err
}

func (as *appendStream) openWithRetry() (storagepb.BigQueryWrite_AppendRowsClient, error) {
	r := defaultRetryer{}
	for {
		arc, err := as.open()
		bo, shouldRetry := r.Retry(err)
		if err != nil && shouldRetry {
			if err := gax.Sleep(as.ctx, bo); err != nil {
				return nil, err
			}
			continue
		}
		// we call this with the mutex lock in as.get(), so safe to toggle state here.
		as.sentFirstAppend = false
		// we're effectively starting a new stream here, so we establish a new channel and
		// start a processor on it.

		// This is called when we already have the lock, so it's okay to update the reference in appendstream.
		pending := make(chan *pendingWrite)
		go defaultRecvProcessor(as.ctx, as, arc, pending)
		as.pending = pending
		return arc, err
	}

}

func (as *appendStream) call(f func(storagepb.BigQueryWrite_AppendRowsClient) error, opts ...gax.CallOption) error {
	var settings gax.CallSettings
	for _, opt := range opts {
		opt.Resolve(&settings)
	}
	var r gax.Retryer = &defaultRetryer{}
	if settings.Retry != nil {
		r = settings.Retry()
	}

	var (
		arc *storagepb.BigQueryWrite_AppendRowsClient
		err error
	)
	for {
		arc, err = as.get(arc)
		if err != nil {
			return err
		}
		start := time.Now()
		err = f(*arc)
		if err != nil {
			bo, shouldRetry := r.Retry(err)
			if shouldRetry {
				if time.Since(start) < 30*time.Second {
					if err := gax.Sleep(as.ctx, bo); err != nil {
						return err
					}
				}
			}
			as.mu.Lock()
			as.err = err
			as.mu.Unlock()
		}
		return err
	}
}

func (as *appendStream) CloseSend() error {
	err := as.call(func(arc storagepb.BigQueryWrite_AppendRowsClient) error {
		return arc.CloseSend()
	})
	as.mu.Lock()
	as.err = io.EOF
	as.mu.Unlock()
	return err
}

func (as *appendStream) append(pw *pendingWrite) error {

	// TODO: rethink locking here, we lock in call()

	// compute proto size pessimistically, assuming we may have to include schema and stream ID
	pw.reqSize = proto.Size(pw.request) + proto.Size(as.schema) + len(as.streamName) + len(as.traceID)

	// block on flow control
	if err := as.fc.acquire(as.ctx, pw.reqSize); err != nil {
		return fmt.Errorf("flow controller issue: %v", err)
	}

	pw.request.TraceId = as.traceID
	if !as.sentFirstAppend {
		// we only need to send schema and stream ID on the first append for a channel
		pw.request.WriteStream = as.streamName
		pw.request.GetProtoRows().WriterSchema = as.schema
	}

	err := as.call(func(arc storagepb.BigQueryWrite_AppendRowsClient) error {
		return arc.Send(pw.request)
	})
	as.fc.release(pw.reqSize)
	if err != nil {
		log.Printf("failed Send(): %#v\nerr: %v", pw.request, err)
		// the insert itself failed; finalize the callback with the same error
		// we got from the call and return.
		pw.markDone(-1, err)
		return err
	}
	as.pending <- pw
	off := pw.request.GetOffset()
	if off != nil {
		if off.Value > as.maxOffsetSent {
			as.maxOffsetSent = off.Value
		}
	}
	return nil
}

// defaultRecvProcessor is responsible for processing the response stream from the service.
//
// Need to also consider draining semantics when we do get a new stream; we need to process all the pending writes on the
// channel,
func defaultRecvProcessor(ctx context.Context, as *appendStream, arc storagepb.BigQueryWrite_AppendRowsClient, pending chan *pendingWrite) {
	for {

		// if set, we'll mark all writes with the error.
		var drainErr error

		// processing loop.
		select {
		case <-ctx.Done():
			return
		case nextWrite, ok := <-pending:
			if !ok {
				// Channel is closed, all elements processed.  Simply return and end.
				return
			}

			if drainErr != nil {
				// we've got a persistent error on the receiver, so mark any remaining pending writes with it.
				nextWrite.markDone(-1, drainErr)
				continue
			}

			// Normal operation, we get the next response from the stream and pair it with the write.
			resp, err := arc.Recv()
			if err == io.EOF {
				// In this case, we've reach EOF when we expected responses, so this is a bit unusual.
				// We won't get any more responses, so set drainErr in case there's more pending writes.
				drainErr = io.EOF
				nextWrite.markDone(-1, drainErr)
				continue
			}
			if err != nil {
				// We got an error from the Recv(), so mark the pending write with it.
				nextWrite.markDone(-1, err)
				continue
			}

			// the response embedded an error, so mark the pending write with it.
			if status := resp.GetError(); status != nil {
				nextWrite.markDone(-1, grpcstatus.ErrorProto(status))
				continue
			}
			success := resp.GetAppendResult()
			off := success.GetOffset()
			// stats thing.
			if off != nil {
				as.mu.Lock()
				if off.GetValue() > as.maxOffsetReceived {
					as.maxOffsetReceived = off.GetValue()
				}
				as.mu.Unlock()
				// mark using the offsets.
				nextWrite.markDone(nextWrite.request.GetOffset().GetValue(), nil)
				continue
			}
			// last case; success without offset present.
			nextWrite.markDone(-1, nil)
		}
	}
}
