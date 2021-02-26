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

	storage "cloud.google.com/go/bigquery/storage/apiv1beta2"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// appendStream is an abstraction over the append stream that supports reconnection/retry.
type appendStream struct {
	// for debugging.
	traceID       string
	offset        int64
	ctx           context.Context
	client        *storage.BigQueryWriteClient
	fc            *flowController
	recvProcessor func(ctx context.Context)
	cancelR       func()

	schema *storagepb.ProtoSchema
	arc    storagepb.BigQueryWrite_AppendRowsClient

	// things we need for reopen
	streamName string
	sentSchema bool
	pending    chan *pendingWrite

	curOffset int64

	statsMu sync.Mutex

	// terminalErr contains the terminal error that triggered stream close.
	// In the case of a normal user close, it will hold io.EOF
	terminalErr       error
	terminalErrorRows int64
}

func newAppendStream(ctx context.Context, client *storage.BigQueryWriteClient, fc *flowController, streamName string, schema *storagepb.ProtoSchema, tracePrefix string) (*appendStream, error) {
	as := &appendStream{
		ctx:        ctx,
		fc:         fc,
		client:     client,
		streamName: streamName,
		schema:     schema,
		pending:    make(chan *pendingWrite, fc.maxInsertCount+1),
		traceID:    fmt.Sprintf("%s-%d", tracePrefix, time.Now().UnixNano()),
	}
	arc, err := client.AppendRows(ctx)
	if err != nil {
		return nil, err
	}
	as.arc = arc
	procCtx, _ := context.WithCancel(ctx)
	go defaultRecvProcessor(procCtx, as, as.pending)
	return as, nil
}

// Close signals user-level close of the stream.  If the stream already has a terminal error state, it is returned.
func (as *appendStream) userClose() error {
	as.statsMu.Lock()
	defer as.statsMu.Unlock()
	if as.terminalErr != nil {
		return as.terminalErr
	}
	err := as.arc.CloseSend()
	if err != nil {
		log.Printf("CloseSend returned err: %v", err)
	}
	// mark stream done.
	as.terminalErr = io.EOF
	return nil
}

func (as *appendStream) append(pw *pendingWrite) error {

	// TODO need a lock here on whether it's safe to append.

	pw.request.TraceId = as.traceID
	if !as.sentSchema {
		pw.request.WriteStream = as.streamName
		pw.request.GetProtoRows().WriterSchema = as.schema
	}
	reqSize := proto.Size(pw.request)
	if err := as.fc.acquire(as.ctx, reqSize); err != nil {
		return fmt.Errorf("flow controller issue: %v", err)
	}
	if err := as.arc.Send(pw.request); err != nil {
		// give back to the user?
		// or should we finalize the pending write?
		log.Printf("failed Send(): %#v\nerr: %v", pw.request, err)
		as.fc.release(reqSize)
		return err
	}
	as.pending <- pw
	return nil
}

func (as *appendStream) isClosed() bool {
	as.statsMu.Lock()
	defer as.statsMu.Unlock()
	return as.terminalErr == nil
}

func (as *appendStream) flush(offset int64) (int64, error) {
	req := &storagepb.FlushRowsRequest{
		WriteStream: as.streamName,
		Offset: &wrapperspb.Int64Value{
			Value: offset,
		},
	}
	resp, err := as.client.FlushRows(as.ctx, req)
	if err != nil {
		return 0, err
	}
	return resp.GetOffset(), nil
}

func (as *appendStream) finalize() (int64, error) {
	// do we block appends? do we allow finalization with writes in flight?
	count := as.fc.count()
	if count > 0 {
		return 0, fmt.Errorf("cannot finalize with writes in flight. %d in flight", count)
	}
	req := &storagepb.FinalizeWriteStreamRequest{
		Name: as.streamName,
	}
	resp, err := as.client.FinalizeWriteStream(as.ctx, req)
	if err != nil {
		return -1, err
	}
	return resp.GetRowCount(), nil
}

// defaultRecvProcessor is responsible for processing the response stream from the service.
func defaultRecvProcessor(ctx context.Context, as *appendStream, pending chan *pendingWrite) {
	for {
		// kill processing if context is done.
		select {
		case <-ctx.Done():
			return
		case nextWrite, ok := <-pending:
			if !ok {
				// Channel is closed.  We do all reconnection logic elsewhere, so here we simply return.
				return
			}
			resp, err := as.arc.Recv()
			if err == io.EOF {
				// do we need to signal reconnect elsewhere?
				// how do we start a new receiver and stream?
			}
			if err != nil {
				// handle stream-level error.
				// for now, just propagate the error.
				nextWrite.markDone(-1, err)
				continue
			}
			if status := resp.GetError(); status != nil {
				// this is a paired error.
				nextWrite.markDone(-1, statusAsError(status))
				continue
			}
			success := resp.GetAppendResult()
			off := success.GetOffset()
			if off != nil {
				expected := nextWrite.request.GetOffset().GetValue() + int64(len(nextWrite.request.GetProtoRows().GetRows().GetSerializedRows()))
				if off.GetValue() != expected {
					log.Printf("mismatched offsets.  got %d, expected %d", off.GetValue(), expected)
				}
				nextWrite.markDone(nextWrite.request.GetOffset().GetValue(), nil)
				continue
			}
			nextWrite.markDone(-1, nil)

		}
	}
}
