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
	"log"
	"sync"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

var (
	// Metrics on a stream are tagged with the stream ID.
	keyStream = tag.MustNewKey("streamID")

	// We allow users to annotate streams with a data origin for monitoring purposes.
	// See the WithDataOrigin writer option for providing this.
	keyDataOrigin = tag.MustNewKey("dataOrigin")
)

const statsPrefix = "cloud.google.com/go/bigquery/storage/managedwriter/"

var (
	// AppendRequests is a measure of the number of append requests sent.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendRequests = stats.Int64(statsPrefix+"append_requests", "Number of append requests sent", stats.UnitDimensionless)

	// AppendBytes is a measure of the bytes sent as append requests.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendBytes = stats.Int64(statsPrefix+"append_bytes", "Number of bytes sent as append requests", stats.UnitBytes)

	// AppendResponses is a measure of the number of append responses received.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendResponses = stats.Int64(statsPrefix+"append_responses", "Number of append responses sent", stats.UnitDimensionless)

	// FlushRequests is a measure of the number of FlushRows requests sent.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	FlushRequests = stats.Int64(statsPrefix+"flush_requests", "Number of FlushRows requests sent", stats.UnitDimensionless)

	// AppendClientOpenCount is a measure of the number of times the AppendRowsClient was opened.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendClientOpenCount = stats.Int64(statsPrefix+"stream_open_count", "Number of times AppendRowsClient was opened", stats.UnitDimensionless)

	// AppendClientOpenRetryCount is a measure of the number of times the AppendRowsClient open was retried.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendClientOpenRetryCount = stats.Int64(statsPrefix+"stream_open_retry_count", "Number of times AppendRowsClient open was retried", stats.UnitDimensionless)
)

var (
	// AppendRequestsView is a cumulative sum of AppendRequests.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendRequestsView *view.View

	// AppendBytesView is a cumulative sum of AppendBytes.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendBytesView *view.View

	// AppendResponsesView is a cumulative sum of AppendResponses.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendResponsesView *view.View

	// FlushRequestsView is a cumulative sum of FlushRequests.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	FlushRequestsView *view.View

	// AppendClientOpenView is a cumulative sum of AppendClientOpenCount.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendClientOpenView *view.View

	// AppendClientOpenRetryView is a cumulative sum of AppendClientOpenRetryCount.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendClientOpenRetryView *view.View
)

func init() {
	AppendRequestsView = createSumView(stats.Measure(AppendRequests), keyStream, keyDataOrigin)
	AppendBytesView = createSumView(stats.Measure(AppendBytes), keyStream, keyDataOrigin)
	AppendResponsesView = createSumView(stats.Measure(AppendResponses), keyStream, keyDataOrigin)
	FlushRequestsView = createSumView(stats.Measure(FlushRequests), keyStream, keyDataOrigin)
	AppendClientOpenView = createSumView(stats.Measure(AppendClientOpenCount), keyStream, keyDataOrigin)
	AppendClientOpenRetryView = createSumView(stats.Measure(AppendClientOpenRetryCount), keyStream, keyDataOrigin)
}

func createView(m stats.Measure, agg *view.Aggregation, keys ...tag.Key) *view.View {
	return &view.View{
		Name:        m.Name(),
		Description: m.Description(),
		TagKeys:     keys,
		Measure:     m,
		Aggregation: agg,
	}
}

func createSumView(m stats.Measure, keys ...tag.Key) *view.View {
	return createView(m, view.Sum(), keys...)
}

var logTagStreamOnce sync.Once
var logTagOriginOnce sync.Once

// keyContextWithStreamID returns a new context modified with the instrumentation tags.
func keyContextWithTags(ctx context.Context, streamID, dataOrigin string) context.Context {
	ctx, err := tag.New(ctx, tag.Upsert(keyStream, streamID))
	if err != nil {
		logTagStreamOnce.Do(func() {
			log.Printf("managedwriter: error creating tag map for 'streamID' key: %v", err)
		})
	}
	ctx, err = tag.New(ctx, tag.Upsert(keyDataOrigin, dataOrigin))
	if err != nil {
		logTagOriginOnce.Do(func() {
			log.Printf("managedwriter: error creating tag map for 'dataOrigin' key: %v", err)
		})
	}
	return ctx
}

func recordStat(ctx context.Context, m *stats.Int64Measure, n int64) {
	stats.Record(ctx, m.M(n))
}
