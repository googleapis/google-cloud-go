// Copyright 2022 Google LLC
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

package reader

import (
	"context"
	"fmt"
	"runtime"

	"cloud.google.com/go/bigquery/internal"
	storage "cloud.google.com/go/bigquery/storage/apiv1"
	"cloud.google.com/go/internal/detect"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
)

// Client is a managed BigQuery Storage read client scoped to a single project.
type Client struct {
	rawClient *storage.BigQueryReadClient
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

	rawClient, err := storage.NewBigQueryReadClient(ctx, o...)
	if err != nil {
		return nil, err
	}
	rawClient.SetGoogleClientInfo("gccl", internal.Version)

	// Handle project autodetection.
	projectID, err = detect.ProjectID(ctx, projectID, "", opts...)
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

// NewReader establishes a new reader to fetch data.
func (c *Client) NewReader(opts ...ReadOption) (*Reader, error) {
	return c.buildReader(opts...)
}

func (c *Client) buildReader(opts ...ReadOption) (*Reader, error) {
	r := &Reader{
		settings: defaultSettings(),
		c:        c,
	}

	// apply read options
	for _, opt := range opts {
		opt(r)
	}

	return r, nil
}

func (c *Client) createReadSession(ctx context.Context, req *storagepb.CreateReadSessionRequest, opts ...gax.CallOption) (*storagepb.ReadSession, error) {
	return c.rawClient.CreateReadSession(ctx, req, opts...)
}

func (c *Client) readRows(ctx context.Context, req *storagepb.ReadRowsRequest, opts ...gax.CallOption) (storagepb.BigQueryRead_ReadRowsClient, error) {
	return c.rawClient.ReadRows(ctx, req, opts...)
}
