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
	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// WithDefaultLocation if set, will be used as the default location for all subsequent
// job operations. A location specified directly in one of those operations will override this value.
func WithDefaultLocation(location string) option.ClientOption {
	return &customClientOption{location: location}
}

// WithDefaultJobCreationMode sets default job mode creation for all subsequent
// job operations. A mode specified directly in one of those operations will override this value.
func WithDefaultJobCreationMode(mode bigquerypb.QueryRequest_JobCreationMode) option.ClientOption {
	return &customClientOption{jobCreationMode: mode}
}

type customClientOption struct {
	internaloption.EmbeddableAdapter
	jobCreationMode bigquerypb.QueryRequest_JobCreationMode
	location        string
}

func (s *customClientOption) ApplyCustomClientOpt(h *Helper) {
	if s.location != "" {
		h.location = wrapperspb.String(s.location)
	}
	h.jobCreationMode = s.jobCreationMode
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
