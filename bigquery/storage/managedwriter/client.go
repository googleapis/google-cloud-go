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
	"sync"

	"cloud.google.com/go/bigquery/internal"
	storage "cloud.google.com/go/bigquery/storage/apiv1"
	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"cloud.google.com/go/internal/detect"
	"github.com/google/uuid"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	"google.golang.org/grpc/metadata"
)

// DetectProjectID is a sentinel value that instructs NewClient to detect the
// project ID. It is given in place of the projectID argument. NewClient will
// use the project ID from the given credentials or the default credentials
// (https://developers.google.com/accounts/docs/application-default-credentials)
// if no credentials were provided. When providing credentials, not all
// options will allow NewClient to extract the project ID. Specifically a JWT
// does not have the project ID encoded.
const DetectProjectID = "*detect-project-id*"

// Client is a managed BigQuery Storage write client scoped to a single project.
type Client struct {
	rawClient *storage.BigQueryWriteClient
	projectID string

	// retained context.  primarily used for connection management and the underlying
	// client.
	ctx    context.Context
	cancel context.CancelFunc

	// cfg retains general settings (custom ClientOptions).
	cfg *writerClientConfig

	// mu guards access to shared connectionPool instances.
	mu sync.Mutex
	// When multiplexing is enabled, this map retains connectionPools keyed by region ID.
	pools map[string]*connectionPool
}

// NewClient instantiates a new client.
//
// The context provided here is retained and used for background connection management
// between the client and the BigQuery Storage service.
func NewClient(ctx context.Context, projectID string, opts ...option.ClientOption) (c *Client, err error) {
	// Set a reasonable default for the gRPC connection pool size.
	numConns := runtime.GOMAXPROCS(0)
	if numConns > 4 {
		numConns = 4
	}
	o := []option.ClientOption{
		option.WithGRPCConnectionPool(numConns),
	}
	o = append(o, opts...)

	cCtx, cancel := context.WithCancel(ctx)

	rawClient, err := storage.NewBigQueryWriteClient(cCtx, o...)
	if err != nil {
		cancel()
		return nil, err
	}
	rawClient.SetGoogleClientInfo("gccl", internal.Version)

	// Handle project autodetection.
	projectID, err = detect.ProjectID(ctx, projectID, "", opts...)
	if err != nil {
		cancel()
		return nil, err
	}

	return &Client{
		rawClient: rawClient,
		projectID: projectID,
		ctx:       cCtx,
		cancel:    cancel,
		cfg:       newWriterClientConfig(opts...),
		pools:     make(map[string]*connectionPool),
	}, nil
}

// Close releases resources held by the client.
func (c *Client) Close() error {

	// Shutdown the per-region pools.
	c.mu.Lock()
	defer c.mu.Unlock()
	var firstErr error
	for _, pool := range c.pools {
		if err := pool.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Close the underlying client stub.
	if err := c.rawClient.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	// Cancel the retained client context.
	if c.cancel != nil {
		c.cancel()
	}
	return firstErr
}

// NewManagedStream establishes a new managed stream for appending data into a table.
//
// Context here is retained for use by the underlying streaming connections the managed stream may create.
func (c *Client) NewManagedStream(ctx context.Context, opts ...WriterOption) (*ManagedStream, error) {
	return c.buildManagedStream(ctx, c.rawClient.AppendRows, false, opts...)
}

// createOpenF builds the opener function we need to access the AppendRows bidi stream.
func createOpenF(streamFunc streamClientFunc, routingHeader string) func(ctx context.Context, opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
	return func(ctx context.Context, opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
		if routingHeader != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, "x-goog-request-params", routingHeader)
		}
		arc, err := streamFunc(ctx, opts...)
		if err != nil {
			return nil, err
		}
		return arc, nil
	}
}

func (c *Client) buildManagedStream(ctx context.Context, streamFunc streamClientFunc, skipSetup bool, opts ...WriterOption) (*ManagedStream, error) {
	// First, we create a minimal managed stream.
	writer := &ManagedStream{
		id:             newUUID(writerIDPrefix),
		c:              c,
		streamSettings: defaultStreamSettings(),
		curTemplate:    newVersionedTemplate(),
	}
	// apply writer options.
	for _, opt := range opts {
		opt(writer)
	}

	// skipSetup allows for customization at test time.
	// Examine out config writer and apply settings to the real one.
	if !skipSetup {
		if err := c.validateOptions(ctx, writer); err != nil {
			return nil, err
		}

		if writer.streamSettings.streamID == "" {
			// not instantiated with a stream, construct one.
			streamName := fmt.Sprintf("%s/streams/_default", writer.streamSettings.destinationTable)
			if writer.streamSettings.streamType != DefaultStream {
				// For everything but a default stream, we create a new stream on behalf of the user.
				req := &storagepb.CreateWriteStreamRequest{
					Parent: writer.streamSettings.destinationTable,
					WriteStream: &storagepb.WriteStream{
						Type: streamTypeToEnum(writer.streamSettings.streamType),
					}}
				resp, err := writer.c.rawClient.CreateWriteStream(ctx, req)
				if err != nil {
					return nil, fmt.Errorf("couldn't create write stream: %w", err)
				}
				streamName = resp.GetName()
			}
			writer.streamSettings.streamID = streamName
		}
	}
	// we maintain a pool per region, and attach all exclusive and multiplex writers to that pool.
	pool, err := c.resolvePool(ctx, writer.streamSettings, streamFunc)
	if err != nil {
		return nil, err
	}
	// Add the writer to the pool.
	if err := pool.addWriter(writer); err != nil {
		return nil, err
	}
	writer.ctx, writer.cancel = context.WithCancel(ctx)

	// Attach any tag keys to the context on the writer, so instrumentation works as expected.
	writer.ctx = setupWriterStatContext(writer)
	return writer, nil
}

// validateOptions is used to validate that we received a sane/compatible set of WriterOptions
// for constructing a new managed stream.
func (c *Client) validateOptions(ctx context.Context, ms *ManagedStream) error {
	if ms == nil {
		return fmt.Errorf("no managed stream definition")
	}
	if ms.streamSettings.streamID != "" {
		// User supplied a stream, we need to verify it exists.
		info, err := c.getWriteStream(ctx, ms.streamSettings.streamID, false)
		if err != nil {
			return fmt.Errorf("a streamname was specified, but lookup of stream failed: %v", err)
		}
		// update type and destination based on stream metadata
		ms.streamSettings.streamType = StreamType(info.Type.String())
		ms.streamSettings.destinationTable = TableParentFromStreamName(ms.streamSettings.streamID)
	}
	if ms.streamSettings.destinationTable == "" {
		return fmt.Errorf("no destination table specified")
	}
	// we could auto-select DEFAULT here, but let's force users to be specific for now.
	if ms.StreamType() == "" {
		return fmt.Errorf("stream type wasn't specified")
	}
	return nil
}

// resolvePool either returns an existing connectionPool, or returns a new pool if this is the first writer in a given region.
func (c *Client) resolvePool(ctx context.Context, settings *streamSettings, streamFunc streamClientFunc) (*connectionPool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	resp, err := c.getWriteStream(ctx, settings.streamID, false)
	if err != nil {
		return nil, err
	}
	loc := resp.GetLocation()
	if pool, ok := c.pools[loc]; ok {
		return pool, nil
	}

	// No existing pool available, create one for the location and add to shared pools.
	pool, err := c.createPool(loc, streamFunc)
	if err != nil {
		return nil, err
	}
	c.pools[loc] = pool
	return pool, nil
}

// createPool builds a connectionPool.
func (c *Client) createPool(location string, streamFunc streamClientFunc) (*connectionPool, error) {
	cCtx, cancel := context.WithCancel(c.ctx)

	if c.cfg == nil {
		cancel()
		return nil, fmt.Errorf("missing client config")
	}

	var routingHeader string
	/*
	 * TODO: set once backend respects the new routing header
	 * if location != "" && c.projectID != "" {
	 *  	routingHeader = fmt.Sprintf("write_location=projects/%s/locations/%s", c.projectID, location)
	 * }
	 */

	pool := &connectionPool{
		id:                 newUUID(poolIDPrefix),
		location:           location,
		ctx:                cCtx,
		cancel:             cancel,
		open:               createOpenF(streamFunc, routingHeader),
		callOptions:        c.cfg.defaultAppendRowsCallOptions,
		baseFlowController: newFlowController(c.cfg.defaultInflightRequests, c.cfg.defaultInflightBytes),
	}
	router := newSharedRouter(c.cfg.useMultiplex, c.cfg.maxMultiplexPoolSize)
	if err := pool.activateRouter(router); err != nil {
		return nil, err
	}
	return pool, nil
}

// BatchCommitWriteStreams atomically commits a group of PENDING streams that belong to the same
// parent table.
//
// Streams must be finalized before commit and cannot be committed multiple
// times. Once a stream is committed, data in the stream becomes available
// for read operations.
func (c *Client) BatchCommitWriteStreams(ctx context.Context, req *storagepb.BatchCommitWriteStreamsRequest, opts ...gax.CallOption) (*storagepb.BatchCommitWriteStreamsResponse, error) {
	return c.rawClient.BatchCommitWriteStreams(ctx, req, opts...)
}

// CreateWriteStream creates a write stream to the given table.
// Additionally, every table has a special stream named ‘_default’
// to which data can be written. This stream doesn’t need to be created using
// CreateWriteStream. It is a stream that can be used simultaneously by any
// number of clients. Data written to this stream is considered committed as
// soon as an acknowledgement is received.
func (c *Client) CreateWriteStream(ctx context.Context, req *storagepb.CreateWriteStreamRequest, opts ...gax.CallOption) (*storagepb.WriteStream, error) {
	return c.rawClient.CreateWriteStream(ctx, req, opts...)
}

// GetWriteStream returns information about a given WriteStream.
func (c *Client) GetWriteStream(ctx context.Context, req *storagepb.GetWriteStreamRequest, opts ...gax.CallOption) (*storagepb.WriteStream, error) {
	return c.rawClient.GetWriteStream(ctx, req, opts...)
}

// getWriteStream is an internal version of GetWriteStream used for writer setup and validation.
func (c *Client) getWriteStream(ctx context.Context, streamName string, fullView bool) (*storagepb.WriteStream, error) {
	req := &storagepb.GetWriteStreamRequest{
		Name: streamName,
	}
	if fullView {
		req.View = storagepb.WriteStreamView_FULL
	}
	return c.rawClient.GetWriteStream(ctx, req)
}

// TableParentFromStreamName is a utility function for extracting the parent table
// prefix from a stream name.  When an invalid stream ID is passed, this simply returns
// the original stream name.
func TableParentFromStreamName(streamName string) string {
	// Stream IDs have the following prefix:
	// projects/{project}/datasets/{dataset}/tables/{table}/blah
	parts := strings.SplitN(streamName, "/", 7)
	if len(parts) < 7 {
		// invalid; just pass back the input
		return streamName
	}
	return strings.Join(parts[:6], "/")
}

// TableParentFromParts constructs a table identifier using individual identifiers and
// returns a string in the form "projects/{project}/datasets/{dataset}/tables/{table}".
func TableParentFromParts(projectID, datasetID, tableID string) string {
	return fmt.Sprintf("projects/%s/datasets/%s/tables/%s", projectID, datasetID, tableID)
}

// newUUID simplifies generating UUIDs for internal resources.
func newUUID(prefix string) string {
	id := uuid.New()
	return fmt.Sprintf("%s_%s", prefix, id.String())
}

// canMultiplex returns true if the input identifier supports multiplexing.  Currently the only stream
// type that supports multiplexing are default streams.
func canMultiplex(in string) bool {
	// TODO: strengthen validation
	return strings.HasSuffix(in, "default")
}
