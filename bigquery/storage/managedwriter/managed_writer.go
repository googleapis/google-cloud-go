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
	"log"
	"strings"

	"cloud.google.com/go/bigquery"
	storage "cloud.google.com/go/bigquery/storage/apiv1beta2"
	gax "github.com/googleapis/gax-go/v2"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta2"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// ManagedWriter exposes the contract with no impl.
type ManagedWriter struct {
	settings   *WriteSettings
	streamType StreamType
	as         *appendStream
	client     *storage.BigQueryWriteClient
}

// Settings that the user controls.
//
// My suspicion is that we end up turning this into an option pattern
// and/or arguments to NewWriter.
type WriteSettings struct {
	StreamType StreamType

	// MaxInflightRequests governs how many unacknowledged
	// append writes can be outstanding into the system.
	MaxInflightRequests int

	// MaxInflightBytes governs how many unacknowledged
	// request bytes can be outstanding into the system.
	MaxInflightBytes int

	Serializer RowSerializer

	// TracePrefix sets a suitable prefix for the trace ID set on
	// append requests.  Useful for diagnostic purposes.
	TracePrefix string
}

func defaultSettings() *WriteSettings {
	return &WriteSettings{
		StreamType:          DefaultStream,
		MaxInflightRequests: 1000,
		MaxInflightBytes:    0,
		Serializer:          nil,
		TracePrefix:         "defaultManagedWriter",
	}
}

// NewManagedWriter instantiates a new managed writer.
func NewManagedWriter(ctx context.Context, client *storage.BigQueryWriteClient, table *bigquery.Table, opts ...WriterOption) (*ManagedWriter, error) {
	mw := &ManagedWriter{
		settings: defaultSettings(),
		client:   client,
	}

	// apply writer options
	for _, opt := range opts {
		opt(mw)
	}

	streamName := fmt.Sprintf("projects/%s/datasets/%s/tables/%s/_default", table.ProjectID, table.DatasetID, table.TableID)
	if mw.settings.StreamType != DefaultStream {
		// for all other types, we need to first create a stream.
		req := &storagepb.CreateWriteStreamRequest{
			Parent: fmt.Sprintf("projects/%s/datasets/%s/tables/%s", table.ProjectID, table.DatasetID, table.TableID),
			WriteStream: &storagepb.WriteStream{
				Type: streamTypeToEnum(mw.settings.StreamType),
			}}
		resp, err := mw.client.CreateWriteStream(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("couldn't create write stream: %v", err)
		}
		streamName = resp.GetName()
	}
	// ready an appendStream
	fc := newFlowController(mw.settings.MaxInflightRequests, mw.settings.MaxInflightBytes)
	mw.as = newAppendStream(ctx, mw.Append, fc, streamName, constructProtoSchema(mw.settings.Serializer), mw.settings.TracePrefix)

	return mw, nil
}

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

func (mw *ManagedWriter) Append(ctx context.Context, opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
	var resp storagepb.BigQueryWrite_AppendRowsClient
	// TODO: add retries for calls
	resp, err := mw.client.AppendRows(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func constructProtoSchema(rs RowSerializer) *storagepb.ProtoSchema {
	if rs == nil {
		return nil
	}
	return &storagepb.ProtoSchema{
		ProtoDescriptor: rs.Describe(),
	}
}

// When we start a connection and start sending appends, we need to communicate the proto
// schema.
func (mw *ManagedWriter) protoSchema() *storagepb.ProtoSchema {
	if mw.settings == nil {
		return nil
	}
	if mw.settings.Serializer == nil {
		return nil
	}
	return &storagepb.ProtoSchema{
		ProtoDescriptor: mw.settings.Serializer.Describe(),
	}
}

// StreamID returns the corresponding write stream ID being managed by this writer.
func (mw *ManagedWriter) StreamName() (string, error) {
	if mw.as == nil {
		return "", fmt.Errorf("writer has no corresponding stream")
	}
	return mw.as.streamName, nil
}

// Wait blocks until there are no outstanding writes, the context has expired,
// or a non-transient error has occurred.
//
// This is an alternative for tracking all the pending appends when you
// only care about completion, not granular errors (e.g. default streams).
//
// Consider:  should we return stats?
func (mw *ManagedWriter) Wait(ctx context.Context) error {
	return fmt.Errorf("unimplemented")
}

func (mw *ManagedWriter) CloseSend(ctx context.Context) error {
	return mw.as.CloseSend()
}

// AppendRows handles conversion of the input data using the registered serializer.
// It returns an AppendResult for each row generated, or an error.
func (mw *ManagedWriter) AppendRows(data interface{}, offset int64) ([]*AppendResult, error) {
	bs, err := mw.settings.Serializer.Convert(data)
	if err != nil {
		return nil, fmt.Errorf("failed to convert data to rows: %v", err)
	}
	pw := newPendingWrite(bs, offset)
	log.Println("created pending")
	if err := mw.as.append(pw); err != nil {
		log.Printf("wtf no append: %v", err)
	}
	return pw.results, nil
}

// FlushRows signals that rows for a buffered stream are ready to a given offset,
// making them available for reading in BigQuery.
//
// TODO: testing: confirm if flushing non-buffered streams is an error.
func (mw *ManagedWriter) FlushRows(ctx context.Context, offset int64) (int64, error) {
	req := &storagepb.FlushRowsRequest{
		WriteStream: mw.as.streamName,
		Offset: &wrapperspb.Int64Value{
			Value: offset,
		},
	}
	resp, err := mw.client.FlushRows(ctx, req)
	if err != nil {
		return 0, err
	}
	return resp.GetOffset(), nil
}

// Finalize marks a write stream so that no new data can be appended.
// Finalizing the default stream yields an error, as it cannot be finalized.
//
// Finalizing does not advance the flushing of a buffered stream, nor does it commit
// a pending stream. (confirm)
//
// Should this implicitly Stop() the stream?
//
// Should this be exposed to users, or is this for the writer to own as part its shutdown?
// e.g. finalize everything but default
//
func (mw *ManagedWriter) Finalize(ctx context.Context) (int64, error) {
	// do we block appends? do we allow finalization with writes in flight?
	count := mw.as.fc.count()
	if count > 0 {
		return 0, fmt.Errorf("cannot finalize with writes in flight. %d in flight", count)
	}
	req := &storagepb.FinalizeWriteStreamRequest{
		Name: mw.as.streamName,
	}
	resp, err := mw.client.FinalizeWriteStream(ctx, req)
	if err != nil {
		return -1, err
	}
	return resp.GetRowCount(), nil
}

// Commit signals that one or more Pending streams should be committed.  Streams must first be
// finalized before they may be committed.  If you supply other stream IDs to the commit,  they
// must all be valid streams of the same table this writer is appending data into.
//
// we should probably wrap the response, but what the hey
func (mw *ManagedWriter) Commit(ctx context.Context, otherStreams ...string) (*storagepb.BatchCommitWriteStreamsResponse, error) {

	req := &storagepb.BatchCommitWriteStreamsRequest{
		Parent:       tableParentFromStreamName(mw.as.streamName),
		WriteStreams: []string{mw.as.streamName},
	}
	req.WriteStreams = append(req.WriteStreams, otherStreams...)
	return mw.client.BatchCommitWriteStreams(ctx, req)
}

func tableParentFromStreamName(streamName string) string {
	// Example streamName
	// projects/{project}/datasets/{dataset}/tables/{table}/blah

	parts := strings.SplitN(streamName, "/", 7)
	if len(parts) < 7 {
		// invalid; just pass back the input
		return streamName
	}
	return strings.Join(parts[:6], "/")
}
