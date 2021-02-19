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
	"log"

	"cloud.google.com/go/bigquery"
	storage "cloud.google.com/go/bigquery/storage/apiv1beta2"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta2"
)

// ManagedWriter exposes the contract with no impl.
type ManagedWriter struct {
	settings   *WriteSettings
	streamName string
	streamType StreamType
	as         *appendStream
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

	// ManagedRowBatching governs whether the writer will
	// pack individual row writes into requests, or use
	// the request batching of the Append
	ManagedRowBatching bool
}

func defaultSettings() *WriteSettings {
	return &WriteSettings{
		StreamType:          DefaultStream,
		MaxInflightRequests: 1000,
		MaxInflightBytes:    0,
		Serializer:          nil,
		ManagedRowBatching:  true,
	}
}

// NewManagedWriter instantiates a new managed writer.
func NewManagedWriter(ctx context.Context, client *storage.BigQueryWriteClient, table *bigquery.Table, opts ...WriterOption) (*ManagedWriter, error) {
	mw := &ManagedWriter{
		settings: defaultSettings(),
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
		resp, err := client.CreateWriteStream(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("couldn't create write stream: %v", err)
		}
		streamName = resp.GetName()
	}
	// ready an appendStream
	fc := newFlowController(mw.settings.MaxInflightRequests, mw.settings.MaxInflightBytes)
	appendStream, err := newAppendStream(ctx, client, fc, streamName, constructProtoSchema(mw.settings.Serializer))
	if err != nil {
		return nil, fmt.Errorf("error constructing append stream: %v", err)
	}
	mw.as = appendStream
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

// Stop terminates processing of a stream.
//
// In the case of a buffered stream, it does not finalize.
func (mw *ManagedWriter) Stop() {}

// AppendRows handles conversion of the input data using the registered serializer.
// It returns an AppendResult for each row generated, or an error.
func (mw *ManagedWriter) AppendRows(data interface{}) ([]*AppendResult, error) {
	bs, err := mw.settings.Serializer.Convert(data)
	if err != nil {
		return nil, fmt.Errorf("failed to convert data to rows: %v", err)
	}
	pw := newPendingWrite(bs)
	if err := mw.as.append(pw); err != nil {
		log.Printf("wtf no append: %v", err)
		pw.markDone(-1, err)
	}
	return pw.results, nil
}

// FlushRows signals that rows for a buffered stream are ready to a given offset,
// making them available for reading in BigQuery.
//
// TODO: testing: confirm if flushing non-buffered streams is an error.
func (mw *ManagedWriter) FlushRows(toOffset int64) (int64, error) {
	return mw.as.flush(toOffset)
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
func (mw *ManagedWriter) Finalize() (int64, error) {
	return mw.as.finalize()
}

// Commit commits data from a writer managing a Pending stream.  A writer must be finalized before
// committing.
//
// Impl:
func (mw *ManagedWriter) Commit() error {
	return fmt.Errorf("unimplemented")
}

// TODO: this is our chance to clear resources from the running writer.
// Problems:  we can't signal we want to close a write stream with pending data, or a stream we want to discard.
func (mw *ManagedWriter) CloseWriter() {

}

// OTHER
//
// BatchCommitWriteStreams() implies committing multiple writers at once.
// Do we return a stream identifier from the writer, or provide a method that accepts a slice of writers and commits them all?
