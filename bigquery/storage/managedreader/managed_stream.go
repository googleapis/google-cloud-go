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

package managedreader

import (
	"context"

	"cloud.google.com/go/bigquery"
)

// ManagedStream is the abstraction over a single write stream.
type ManagedStream struct {
	streamSettings *streamSettings
	c              *Client
}

// streamSettings govern behavior of the append stream RPCs.
type streamSettings struct {
	// MaxStreamCount governs how many unacknowledged
	// append writes can be outstanding into the system.
	MaxStreamCount int
}

func defaultStreamSettings() *streamSettings {
	return &streamSettings{
		MaxStreamCount: 0,
	}
}

// ReadQuery creates a read stream for a given query.
func (ms *ManagedStream) ReadQuery(ctx context.Context, query *bigquery.Query) (RowIterator, error) {
	return newQueryRowIterator(ctx, ms.c, query)
}
