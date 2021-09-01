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

	// keyError tags metrics using the status code of returned errors.
	keyError = tag.MustNewKey("error")
)

// DefaultOpenCensusViews retains the set of all opencensus views that this
// library has instrumented, to add view registration for exporters.
var DefaultOpenCensusViews []*view.View

const statsPrefix = "cloud.google.com/go/bigquery/storage/managedwriter/"

var (
	// AppendClientOpenCount is a measure of the number of times the AppendRowsClient was opened.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendClientOpenCount = stats.Int64(statsPrefix+"stream_open_count", "Number of times AppendRowsClient was opened", stats.UnitDimensionless)

	// AppendClientOpenRetryCount is a measure of the number of times the AppendRowsClient open was retried.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendClientOpenRetryCount = stats.Int64(statsPrefix+"stream_open_retry_count", "Number of times AppendRowsClient open was retried", stats.UnitDimensionless)

	// AppendRequests is a measure of the number of append requests sent.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendRequests = stats.Int64(statsPrefix+"append_requests", "Number of append requests sent", stats.UnitDimensionless)

	// AppendRequestBytes is a measure of the bytes sent as append requests.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendRequestBytes = stats.Int64(statsPrefix+"append_request_bytes", "Number of bytes sent as append requests", stats.UnitBytes)

	// AppendRequestErrors is a measure of the number of append requests that errored on send.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendRequestErrors = stats.Int64(statsPrefix+"append_request_errors", "Number of append requests that yielded immediate error", stats.UnitDimensionless)

	// AppendRequestRows is a measure of the number of append rows sent.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendRequestRows = stats.Int64(statsPrefix+"append_rows", "Number of append rows sent", stats.UnitDimensionless)

	// AppendResponses is a measure of the number of append responses received.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendResponses = stats.Int64(statsPrefix+"append_responses", "Number of append responses sent", stats.UnitDimensionless)

	// AppendResponseErrors is a measure of the number of append responses received with an error attached.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendResponseErrors = stats.Int64(statsPrefix+"append_response_errors", "Number of append responses with errors attached", stats.UnitDimensionless)

	// FlushRequests is a measure of the number of FlushRows requests sent.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	FlushRequests = stats.Int64(statsPrefix+"flush_requests", "Number of FlushRows requests sent", stats.UnitDimensionless)
)

var (

	// AppendClientOpenView is a cumulative sum of AppendClientOpenCount.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendClientOpenView *view.View

	// AppendClientOpenRetryView is a cumulative sum of AppendClientOpenRetryCount.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendClientOpenRetryView *view.View

	// AppendRequestsView is a cumulative sum of AppendRequests.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendRequestsView *view.View

	// AppendRequestBytesView is a cumulative sum of AppendRequestBytes.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendRequestBytesView *view.View

	// AppendRequestErrorsView is a cumulative sum of AppendRequestErrors.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendRequestErrorsView *view.View

	// AppendRequestRowsView is a cumulative sum of AppendRows.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendRequestRowsView *view.View

	// AppendResponsesView is a cumulative sum of AppendResponses.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendResponsesView *view.View

	// AppendResponseErrorsView is a cumulative sum of AppendResponseErrors.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendResponseErrorsView *view.View

	// FlushRequestsView is a cumulative sum of FlushRequests.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	FlushRequestsView *view.View
)

func init() {
	AppendClientOpenView = createSumView(stats.Measure(AppendClientOpenCount), keyStream, keyDataOrigin)
	AppendClientOpenRetryView = createSumView(stats.Measure(AppendClientOpenRetryCount), keyStream, keyDataOrigin)

	AppendRequestsView = createSumView(stats.Measure(AppendRequests), keyStream, keyDataOrigin)
	AppendRequestBytesView = createSumView(stats.Measure(AppendRequestBytes), keyStream, keyDataOrigin)
	AppendRequestErrorsView = createSumView(stats.Measure(AppendRequestErrors), keyStream, keyDataOrigin, keyError)
	AppendRequestRowsView = createSumView(stats.Measure(AppendRequestRows), keyStream, keyDataOrigin)

	AppendResponsesView = createSumView(stats.Measure(AppendResponses), keyStream, keyDataOrigin)
	AppendResponseErrorsView = createSumView(stats.Measure(AppendResponseErrors), keyStream, keyDataOrigin, keyError)

	FlushRequestsView = createSumView(stats.Measure(FlushRequests), keyStream, keyDataOrigin)

	DefaultOpenCensusViews = []*view.View{
		AppendClientOpenView,
		AppendClientOpenRetryView,

		AppendRequestsView,
		AppendRequestBytesView,
		AppendRequestErrorsView,
		AppendRequestRowsView,

		AppendResponsesView,
		AppendResponseErrorsView,

		FlushRequestsView,
	}
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
