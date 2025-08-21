// Copyright 2025 Google LLC
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

// Package apiv2_client provides an experimental combined client for interacting with the
// various RPC services that comprise the BigQuery v2 API surface.  It simplifies
// interactions with the bigquery service by allowing the user to only manage a single
// unified client rather than instantiating multiple clients.
package apiv2_client

import (
	"context"
	"errors"
	"fmt"

	bigquery "cloud.google.com/go/bigquery/v2/apiv2"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

// Client represents the aggregate client which manages the various per-RPC service
// clients.
type Client struct {
	dsClient    *bigquery.DatasetClient
	jobClient   *bigquery.JobClient
	modelClient *bigquery.ModelClient
	projClient  *bigquery.ProjectClient
	routClient  *bigquery.RoutineClient
	rapClient   *bigquery.RowAccessPolicyClient
	tblClient   *bigquery.TableClient
}

// NewClient creates a new Client based on gRPC.
// The returned client must be Closed when it is done being used to clean up its underlying connections.
func NewClient(ctx context.Context, opts ...option.ClientOption) (*Client, error) {
	var errs []error
	var err error
	mc := &Client{}
	mc.dsClient, err = bigquery.NewDatasetClient(ctx, opts...)
	if err != nil {
		errs = append(errs, fmt.Errorf("NewDatasetClient: %w", err))
	}
	mc.jobClient, err = bigquery.NewJobClient(ctx, opts...)
	if err != nil {
		errs = append(errs, fmt.Errorf("NewJobClient: %w", err))
	}
	mc.modelClient, err = bigquery.NewModelClient(ctx, opts...)
	if err != nil {
		errs = append(errs, fmt.Errorf("NewModelClient: %w", err))
	}
	mc.projClient, err = bigquery.NewProjectClient(ctx, opts...)
	if err != nil {
		errs = append(errs, fmt.Errorf("NewProjectClient: %w", err))
	}
	mc.routClient, err = bigquery.NewRoutineClient(ctx, opts...)
	if err != nil {
		errs = append(errs, fmt.Errorf("NewRoutineClient: %w", err))
	}
	mc.rapClient, err = bigquery.NewRowAccessPolicyClient(ctx, opts...)
	if err != nil {
		errs = append(errs, fmt.Errorf("NewRowAccessPolicyClient: %w", err))
	}
	mc.tblClient, err = bigquery.NewTableClient(ctx, opts...)
	if err != nil {
		errs = append(errs, fmt.Errorf("NewTableClient: %w", err))
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return mc, nil
}

// NewRESTClient creates a new Client based on REST.
// The returned client must be Closed when it is done being used to clean up its underlying connections.
func NewRESTClient(ctx context.Context, opts ...option.ClientOption) (*Client, error) {
	var errs []error
	var err error
	mc := &Client{}
	mc.dsClient, err = bigquery.NewDatasetRESTClient(ctx, opts...)
	if err != nil {
		errs = append(errs, fmt.Errorf("NewDatasetRESTClient: %w", err))
	}
	mc.jobClient, err = bigquery.NewJobRESTClient(ctx, opts...)
	if err != nil {
		errs = append(errs, fmt.Errorf("NewJobRESTClient: %w", err))
	}
	mc.modelClient, err = bigquery.NewModelRESTClient(ctx, opts...)
	if err != nil {
		errs = append(errs, fmt.Errorf("NewModelRESTClient: %w", err))
	}
	mc.projClient, err = bigquery.NewProjectRESTClient(ctx, opts...)
	if err != nil {
		errs = append(errs, fmt.Errorf("NewProjectRESTClient: %w", err))
	}
	mc.routClient, err = bigquery.NewRoutineRESTClient(ctx, opts...)
	if err != nil {
		errs = append(errs, fmt.Errorf("NewRoutineRESTClient: %w", err))
	}
	mc.rapClient, err = bigquery.NewRowAccessPolicyRESTClient(ctx, opts...)
	if err != nil {
		errs = append(errs, fmt.Errorf("NewRowAccessPolicyRESTClient: %w", err))
	}
	mc.tblClient, err = bigquery.NewTableRESTClient(ctx, opts...)
	if err != nil {
		errs = append(errs, fmt.Errorf("NewTableRESTClient: %w", err))
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return mc, nil
}

// Close closes the connection to the API service. The user should invoke this when
// the client is no longer required.
func (mc *Client) Close() error {
	var errs []error
	err := mc.dsClient.Close()
	if err != nil {
		errs = append(errs, fmt.Errorf("DatasetClient: %w", err))
	}
	err = mc.jobClient.Close()
	if err != nil {
		errs = append(errs, fmt.Errorf("JobClient: %w", err))
	}
	err = mc.modelClient.Close()
	if err != nil {
		errs = append(errs, fmt.Errorf("ModelClient: %w", err))
	}
	err = mc.projClient.Close()
	if err != nil {
		errs = append(errs, fmt.Errorf("ProjectClient: %w", err))
	}
	err = mc.routClient.Close()
	if err != nil {
		errs = append(errs, fmt.Errorf("RoutineClient: %w", err))
	}
	err = mc.rapClient.Close()
	if err != nil {
		errs = append(errs, fmt.Errorf("RowAccessPolicyClient: %w", err))
	}
	err = mc.tblClient.Close()
	if err != nil {
		errs = append(errs, fmt.Errorf("TableClient: %w", err))
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Deprecated: Connection() always returns nil.  This method
// exists solely to satisfy the underlying per-RPC service interface(s)
// and should not be used.
func (c *Client) Connection() *grpc.ClientConn {
	return nil
}
