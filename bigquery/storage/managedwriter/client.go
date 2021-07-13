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
	"strings"

	"cloud.google.com/go/bigquery"
	storage "cloud.google.com/go/bigquery/storage/apiv1beta2"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta2"
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

// ManagedWriteClient exposes the contract with no impl.
type ManagedWriteClient struct {
	streamSettings *streamSettings
	client         *storage.BigQueryWriteClient
}

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

// NewManagedWriteClient instantiates a new managed writer.
func NewManagedWriteClient(ctx context.Context, client *storage.BigQueryWriteClient, table *bigquery.Table, opts ...WriterOption) (*ManagedWriteClient, error) {
	mw := &ManagedWriteClient{
		streamSettings: defaultStreamSettings(),
		client:         client,
	}

	// apply writer options
	for _, opt := range opts {
		opt(mw)
	}

	if mw.streamSettings.streamID == "" && mw.streamSettings.streamType == "" {
		return nil, fmt.Errorf("TODO insufficient validation")
	}
	if mw.streamSettings.streamID == "" {
		// not instantiated with a stream, construct one.
		streamName := fmt.Sprintf("projects/%s/datasets/%s/tables/%s/_default", table.ProjectID, table.DatasetID, table.TableID)
		if mw.streamSettings.streamType != DefaultStream {
			// For everything but a default stream, we create a new stream on behalf of the user.
			req := &storagepb.CreateWriteStreamRequest{
				Parent: fmt.Sprintf("projects/%s/datasets/%s/tables/%s", table.ProjectID, table.DatasetID, table.TableID),
				WriteStream: &storagepb.WriteStream{
					Type: streamTypeToEnum(mw.streamSettings.streamType),
				}}
			resp, err := mw.client.CreateWriteStream(ctx, req)
			if err != nil {
				return nil, fmt.Errorf("couldn't create write stream: %v", err)
			}
			streamName = resp.GetName()
		}
		mw.streamSettings.streamID = streamName
		// TODO(followup CLs): instantiate an appendstream client, flow controller, etc.
	}

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

// StreamName returns the corresponding write stream ID being managed by this writer.
func (mw *ManagedWriteClient) StreamName() string {
	return mw.streamSettings.streamID
}

// StreamType returns the configured type for this stream.
func (mw *ManagedWriteClient) StreamType() StreamType {
	return mw.streamSettings.streamType
}

// FlushRows advances the offset at which rows in a BufferedStream are visible.  Calling
// this method for other stream types yields an error.
func (mw *ManagedWriteClient) FlushRows(ctx context.Context, offset int64) (int64, error) {
	req := &storagepb.FlushRowsRequest{
		WriteStream: mw.streamSettings.streamID,
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

// Finalize is used to mark a stream as complete, and thus ensure no further data can
// be appended to the stream.  You cannot finalize a DefaultStream, as it always exists.
//
// Finalizing does not advance the current offset of a BufferedStream, nor does it commit
// data in a PendingStream.
func (mw *ManagedWriteClient) Finalize(ctx context.Context) (int64, error) {
	// TODO: consider blocking for in-flight appends once we have an appendStream plumbed in.
	req := &storagepb.FinalizeWriteStreamRequest{
		Name: mw.streamSettings.streamID,
	}
	resp, err := mw.client.FinalizeWriteStream(ctx, req)
	if err != nil {
		return 0, err
	}
	return resp.GetRowCount(), nil
}

// BatchCommit is used to commit one or more PendingStream streams belonging to the same table
// as a single transaction.  Streams must be finalized before committing.
//
// TODO: this currently exposes the raw proto response, but a future CL will wrap this with a nicer type.
func (mw *ManagedWriteClient) BatchCommit(ctx context.Context, otherStreams ...string) (*storagepb.BatchCommitWriteStreamsResponse, error) {

	req := &storagepb.BatchCommitWriteStreamsRequest{
		Parent:       tableParentFromStreamName(mw.streamSettings.streamID),
		WriteStreams: []string{mw.streamSettings.streamID},
	}
	req.WriteStreams = append(req.WriteStreams, otherStreams...)
	return mw.client.BatchCommitWriteStreams(ctx, req)
}

// getWriteStream returns information about a given write stream.
// It is not currently exported because it's unclear what we should surface here to the client, but we can use it for validation.
func (mw *ManagedWriteClient) getWriteStream(ctx context.Context) (*storagepb.WriteStream, error) {
	if mw.streamSettings.streamID == "" {
		return nil, fmt.Errorf("no stream name configured")
	}
	req := &storagepb.GetWriteStreamRequest{
		Name: mw.streamSettings.streamID,
	}
	return mw.client.GetWriteStream(ctx, req)
}

// tableParentFromStreamName return the corresponding parent table
// identifier given a fully qualified streamname.
func tableParentFromStreamName(streamName string) string {
	// Stream IDs have the following prefix:
	// projects/{project}/datasets/{dataset}/tables/{table}/blah
	parts := strings.SplitN(streamName, "/", 7)
	if len(parts) < 7 {
		// invalid; just pass back the input
		return streamName
	}
	return strings.Join(parts[:6], "/")
}
