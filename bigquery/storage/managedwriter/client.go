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
	"runtime"
	"strings"

	storage "cloud.google.com/go/bigquery/storage/apiv1beta2"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta2"
	"google.golang.org/grpc"
)

// Client is a managed BigQuery Storage write client scoped to a single project.
type Client struct {
	rawClient *storage.BigQueryWriteClient
	projectID string
}

// NewClient instantiates a new client.
func NewClient(ctx context.Context, projectID string, opts ...option.ClientOption) (c *Client, err error) {
	numConns := runtime.GOMAXPROCS(0)
	if numConns > 4 {
		numConns = 4
	}
	o := []option.ClientOption{
		option.WithGRPCConnectionPool(numConns),
	}
	o = append(o, opts...)

	rawClient, err := storage.NewBigQueryWriteClient(ctx, o...)
	if err != nil {
		return nil, err
	}

	return &Client{
		rawClient: rawClient,
		projectID: projectID,
	}, nil
}

// Close releases resources held by the client.
func (c *Client) Close() error {
	// TODO: consider if we should propagate a cancellation from client to all associated managed streams.
	if c.rawClient == nil {
		return fmt.Errorf("already closed")
	}
	c.rawClient.Close()
	c.rawClient = nil
	return nil
}

// NewManagedStream establishes a new managed stream for appending data into a table.
func (c *Client) NewManagedStream(ctx context.Context, opts ...WriterOption) (*ManagedStream, error) {
	return c.buildManagedStream(ctx, c.rawClient.AppendRows, false, opts...)
}

func (c *Client) buildManagedStream(ctx context.Context, streamFunc streamClientFunc, skipSetup bool, opts ...WriterOption) (*ManagedStream, error) {

	ctx, cancel := context.WithCancel(ctx)

	ms := &ManagedStream{
		streamSettings: defaultStreamSettings(),
		c:              c,
		ctx:            ctx,
		cancel:         cancel,
		open: func() (storagepb.BigQueryWrite_AppendRowsClient, error) {
			arc, err := streamFunc(ctx, gax.WithGRPCOptions(grpc.MaxCallRecvMsgSize(10*1024*1024)))
			if err != nil {
				return nil, err
			}
			return arc, nil
		},
	}

	// apply writer options
	for _, opt := range opts {
		opt(ms)
	}

	// skipSetup exists for testing scenarios.
	if !skipSetup {
		if err := c.validateOptions(ctx, ms); err != nil {
			return nil, err
		}

		if ms.streamSettings.streamID == "" {
			// not instantiated with a stream, construct one.
			streamName := fmt.Sprintf("%s/_default", ms.destinationTable)
			if ms.streamSettings.streamType != DefaultStream {
				// For everything but a default stream, we create a new stream on behalf of the user.
				req := &storagepb.CreateWriteStreamRequest{
					Parent: ms.destinationTable,
					WriteStream: &storagepb.WriteStream{
						Type: streamTypeToEnum(ms.streamSettings.streamType),
					}}
				resp, err := ms.c.rawClient.CreateWriteStream(ctx, req)
				if err != nil {
					return nil, fmt.Errorf("couldn't create write stream: %v", err)
				}
				streamName = resp.GetName()
			}
			ms.streamSettings.streamID = streamName
			// TODO(followup CLs): instantiate an appendstream client, flow controller, etc.
		}
	}

	return ms, nil
}

// validateOptions is used to validate that we received a sane/compatible set of WriterOptions
// for constructing a new managed stream.
func (c *Client) validateOptions(ctx context.Context, ms *ManagedStream) error {
	if ms == nil {
		return fmt.Errorf("no managed stream definition")
	}
	if ms.streamSettings.streamID != "" {
		// User supplied a stream, we need to verify it exists.
		info, err := c.getWriteStream(ctx, ms.streamSettings.streamID)
		if err != nil {
			return fmt.Errorf("a streamname was specified, but lookup of stream failed: %v", err)
		}
		// update type and destination based on stream metadata
		ms.streamSettings.streamType = StreamType(info.Type.String())
		ms.destinationTable = tableParentFromStreamName(ms.streamSettings.streamID)
	}
	if ms.destinationTable == "" {
		return fmt.Errorf("no destination table specified")
	}
	// we could auto-select DEFAULT here, but let's force users to be specific for now.
	if ms.StreamType() == "" {
		return fmt.Errorf("stream type wasn't specified")
	}
	return nil
}

// BatchCommit is used to commit one or more PendingStream streams belonging to the same table
// as a single transaction.  Streams must be finalized before committing.
//
// TODO: this currently exposes the raw proto response, but a future CL will wrap this with a nicer type.
func (c *Client) BatchCommit(ctx context.Context, parentTable string, streamNames []string) (*storagepb.BatchCommitWriteStreamsResponse, error) {

	// determine table from first streamName, as all must share the same table.
	if len(streamNames) <= 0 {
		return nil, fmt.Errorf("no streamnames provided")
	}

	req := &storagepb.BatchCommitWriteStreamsRequest{
		Parent:       tableParentFromStreamName(streamNames[0]),
		WriteStreams: streamNames,
	}
	return c.rawClient.BatchCommitWriteStreams(ctx, req)
}

// getWriteStream returns information about a given write stream.
//
// It's primarily used for setup validation, and not exposed directly to end users.
func (c *Client) getWriteStream(ctx context.Context, streamName string) (*storagepb.WriteStream, error) {
	req := &storagepb.GetWriteStreamRequest{
		Name: streamName,
	}
	return c.rawClient.GetWriteStream(ctx, req)
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
