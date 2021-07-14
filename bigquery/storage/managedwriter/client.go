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

	"cloud.google.com/go/bigquery"
	storage "cloud.google.com/go/bigquery/storage/apiv1beta2"
	"google.golang.org/api/option"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta2"
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

// NewManagedStream establishes a new stream for writing.
func (c *Client) NewManagedStream(ctx context.Context, table *bigquery.Table, opts ...WriterOption) (*ManagedStream, error) {

	ms := &ManagedStream{
		streamSettings: defaultStreamSettings(),
		c:              c,
	}

	// apply writer options
	for _, opt := range opts {
		opt(ms)
	}

	if ms.streamSettings.streamID == "" && ms.streamSettings.streamType == "" {
		return nil, fmt.Errorf("TODO insufficient validation")
	}
	if ms.streamSettings.streamID == "" {
		// not instantiated with a stream, construct one.
		streamName := fmt.Sprintf("projects/%s/datasets/%s/tables/%s/_default", table.ProjectID, table.DatasetID, table.TableID)
		if ms.streamSettings.streamType != DefaultStream {
			// For everything but a default stream, we create a new stream on behalf of the user.
			req := &storagepb.CreateWriteStreamRequest{
				Parent: fmt.Sprintf("projects/%s/datasets/%s/tables/%s", table.ProjectID, table.DatasetID, table.TableID),
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

	return ms, nil
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
// It is not currently exported because it's unclear what we should surface here to the client, but we can use it for validation.
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
