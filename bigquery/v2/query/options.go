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
	"cloud.google.com/go/bigquery/v2/apiv2_client"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
)

// WithClient allows to override the internal bigquery apiv2_client.Client
func WithClient(client *apiv2_client.Client) option.ClientOption {
	return &customClientOption{client: client}
}

type customClientOption struct {
	internaloption.EmbeddableAdapter
	client *apiv2_client.Client
}

func (s *customClientOption) ApplyCustomClientOpt(c *Client) {
	if s.client != nil {
		c.c = s.client
	}
}

// ReadOption is an option for reading query results.
type ReadOption func(*readState)

type readState struct {
	pageToken string
}

// WithPageToken sets the page token for reading query results.
func WithPageToken(t string) ReadOption {
	return func(s *readState) {
		s.pageToken = t
	}
}
