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
package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"reflect"
	"sync"
	"time"

	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
)

type ThickWriter struct {
	// used to implicate writes from a specific instantiation
	writerID               string
	writeClient            *BigQueryWriteClient
	mu                     sync.Mutex
	sentSchema             bool
	streamName             string
	appendClient           storagepb.BigQueryWrite_AppendRowsClient
	protoMessageDescriptor *descriptorpb.DescriptorProto

	stopFunc func()

	// TODO: figure out if we want writer to maintain an error count

	// TODO: this should probably be a buffered chan
	pending []*AppendResult
}

func NewThickWriter(ctx context.Context, client *BigQueryWriteClient, streamName string) (*ThickWriter, error) {
	writer := &ThickWriter{
		writerID:    fmt.Sprintf("test thick writer %s", time.Now().Format(time.RFC1123Z)),
		writeClient: client,
		streamName:  streamName,
	}
	appendCtx, cancel := context.WithCancel(ctx)
	writer.stopFunc = cancel
	appendClient, err := client.AppendRows(appendCtx)
	if err != nil {
		return nil, fmt.Errorf("error starting stream client: %v", err)
	}
	writer.appendClient = appendClient
	go writer.processStream(ctx)
	return writer, nil
}

func (tw *ThickWriter) processStream(ctx context.Context) {

	for {
		// kill processing if context is done.
		select {
		case <-ctx.Done():
			return
		default:
		}

		// potential badness: not guarded by mutex, but it blocks
		resp, err := tw.appendClient.Recv()
		if err == io.EOF {
			// we're at end of stream.  break the loop, or do we fix appendClient and keep going?
			// TODO: how do we signal stopped?
			break
		}
		if err != nil {
			// observed errors here
			// open question: should these be associated to the next AR, the writer, or both?

			// when you don't send schema:
			// code = InvalidArgument desc = Invalid proto schema: BqMessage.proto: : Missing name. Entity: projects/shollyman-demo-test/datasets/storage_test_dataset_20210122_70895612072539_0002/tables/testtable_20210122_70895612106658_0002/_default
			// slow metadata update?
			// code = PermissionDenied desc = Permission 'TABLES_UPDATE_DATA' denied on resource 'projects/shollyman-demo-test/datasets/storage_test_dataset_20210122_70746896961953_0002/tables/testtable_20210122_70746897001459_0002' (or it may not exist).
			log.Printf("\ngot an error from recv: %v\n", err)

			// for now, kill the loop until we figure out
			break
		}

		// take the lock while we process an element from the queue
		tw.mu.Lock()

		// get pending AR from queue.
		nextAR := tw.pending[0]
		tw.pending = tw.pending[1:]

		if status := resp.GetError(); status != nil {
			// TODO: add retry logic here.
			// need a custom status error?
			nextAR.err = fmt.Errorf("%d: %s", status.GetCode(), status.GetMessage())
		}

		if success := resp.GetAppendResult(); success != nil {
			if off := success.GetOffset(); off != nil {
				nextAR.offset = off.GetValue()
			}
		}

		// clear the request (may never need this, once we resolve the retry strategy)
		nextAR.req = nil
		// signal this write complete
		close(nextAR.ready)
		// done for now, leave the lock
		tw.mu.Unlock()
	}
}

type AppendResult struct {
	req    *storagepb.AppendRowsRequest
	ready  chan struct{}
	err    error
	offset int64
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

func NewAppendResult() *AppendResult {
	return &AppendResult{ready: make(chan struct{})}
}

func (tw *ThickWriter) Stop(ctx context.Context) error {
	// need to close down processing of stream responses
	return fmt.Errorf("unimplemented")
}

func (tw *ThickWriter) RegisterProto(in proto.Message) error {
	if in == nil {
		return fmt.Errorf("no proto provided")
	}
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if tw.protoMessageDescriptor != nil {
		return fmt.Errorf("already have a type registered")
	}
	tw.protoMessageDescriptor = protodesc.ToDescriptorProto(in.ProtoReflect().Descriptor())
	return nil
}

// AppendRows expects a slice of proto.Messages, which it serialized and then appends.
// It returns an AppendResult, which can be used to check that the write completed.
func (tw *ThickWriter) AppendRows(protoSlice interface{}) (*AppendResult, error) {

	// this is all because Go doesn't allow a slice of an interface type, and that's
	// how proto.Message works.
	s := reflect.ValueOf(protoSlice)
	if s.Kind() != reflect.Slice {
		return nil, fmt.Errorf("did not pass a slice of proto.Message to AppendRows")
	}

	if s.IsNil() {
		return nil, fmt.Errorf("no rows in append")
	}

	serialized := make([][]byte, s.Len())
	for i := 0; i < s.Len(); i++ {
		m, err := marshalRow(s.Index(i).Interface())
		if err != nil {
			return nil, fmt.Errorf("element %d failed marshal: %v", i, err)
		}
		serialized[i] = m
	}
	return tw.appendRows(serialized)
}

// appendRows appends serialized proto messages to the stream.
func (tw *ThickWriter) appendRows(rowData [][]byte) (*AppendResult, error) {
	if rowData == nil {
		return nil, fmt.Errorf("no rows in append")
	}

	tw.mu.Lock()
	defer tw.mu.Unlock()
	if tw.protoMessageDescriptor == nil {
		return nil, fmt.Errorf("no proto schema registered")
	}

	var protoSchema *storagepb.ProtoSchema
	if !tw.sentSchema {
		protoSchema = &storagepb.ProtoSchema{
			ProtoDescriptor: tw.protoMessageDescriptor,
		}
	}
	req := &storagepb.AppendRowsRequest{
		WriteStream: tw.streamName,
		TraceId:     tw.writerID,
		Rows: &storagepb.AppendRowsRequest_ProtoRows{
			ProtoRows: &storagepb.AppendRowsRequest_ProtoData{
				Rows: &storagepb.ProtoRows{
					SerializedRows: rowData,
				},
				WriterSchema: protoSchema,
			},
		},
	}
	if err := tw.appendClient.Send(req); err != nil {
		// are these errors retriable, or only transport problems?
		// Do we close the writer?
		return nil, err
	}
	ar := NewAppendResult()
	ar.req = req
	tw.pending = append(tw.pending, ar)
	return ar, nil
}

// convert a row into a marshalled proto
func marshalRow(in interface{}) ([]byte, error) {
	if m, ok := in.(proto.Message); ok {
		return proto.Marshal(m)
	}
	return nil, fmt.Errorf("not a proto")
}
