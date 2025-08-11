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

package query

import (
	storage "cloud.google.com/go/bigquery/storage/apiv1"
	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"cloud.google.com/go/bigquery/v2/apiv2_client"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
)

// WithClient allows to override the internal bigquery apiv2_client.Client
func WithClient(client *apiv2_client.Client) option.ClientOption {
	return &customClientOption{client: client}
}

// WithBillingProjectID sets the billing project ID for the client.
func WithBillingProjectID(projectID string) option.ClientOption {
	return &customClientOption{billingProjectID: projectID}
}

// WithDefaultJobCreationMode sets default job mode creation.
func WithDefaultJobCreationMode(mode bigquerypb.QueryRequest_JobCreationMode) option.ClientOption {
	return &customClientOption{defaultJobCreationMode: mode}
}

// WithReadClient sets the read client for the query reader.
func WithReadClient(rc *storage.BigQueryReadClient) option.ClientOption {
	return &customClientOption{readClient: rc}
}

type customClientOption struct {
	internaloption.EmbeddableAdapter
	client                 *apiv2_client.Client
	readClient             *storage.BigQueryReadClient
	defaultJobCreationMode bigquerypb.QueryRequest_JobCreationMode
	billingProjectID       string
}

func (s *customClientOption) ApplyCustomClientOpt(c *Client) {
	if s.client != nil {
		c.c = s.client
	}
	if s.billingProjectID != "" {
		c.billingProjectID = s.billingProjectID
	}
	c.defaultJobCreationMode = s.defaultJobCreationMode
	if s.readClient != nil {
		c.rc = s.readClient
	}
}

// ReadOption is an option for reading query results.
type ReadOption func(*readState)

type readState struct {
	pageToken  string
	readClient *storage.BigQueryReadClient
}

// WithPageToken sets the page token for reading query results.
func WithPageToken(t string) ReadOption {
	return func(s *readState) {
		s.pageToken = t
	}
}

// WithStorageReadClient reads the given query using the Storage Read API
func WithStorageReadClient(rc *storage.BigQueryReadClient) ReadOption {
	return func(s *readState) {
		s.readClient = rc
	}
}

func hasRetry(opts []gax.CallOption) bool {
	cs := &gax.CallSettings{}
	for _, opt := range opts {
		opt.Resolve(cs)
		if cs.Retry != nil {
			return true
		}
	}
	return false
}
