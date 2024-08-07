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

	// AppendRequestReconnects is a measure of the number of times that sending an append request triggered reconnect.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendRequestReconnects = stats.Int64(statsPrefix+"append_reconnections", "Number of append rows reconnections", stats.UnitDimensionless)

	// AppendRequestRows is a measure of the number of append rows sent.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendRequestRows = stats.Int64(statsPrefix+"append_rows", "Number of append rows sent", stats.UnitDimensionless)

	// AppendResponses is a measure of the number of append responses received.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendResponses = stats.Int64(statsPrefix+"append_responses", "Number of append responses sent", stats.UnitDimensionless)

	// AppendResponseErrors is a measure of the number of append responses received with an error attached.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendResponseErrors = stats.Int64(statsPrefix+"append_response_errors", "Number of append responses with errors attached", stats.UnitDimensionless)

	// AppendRetryCount is a measure of the number of appends that were automatically retried by the library
	// after receiving a non-successful response.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendRetryCount = stats.Int64(statsPrefix+"append_retry_count", "Number of appends that were retried", stats.UnitDimensionless)

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

	// AppendRequestReconnectsView is a cumulative sum of AppendRequestReconnects.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendRequestReconnectsView *view.View

	// AppendRequestRowsView is a cumulative sum of AppendRows.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendRequestRowsView *view.View

	// AppendResponsesView is a cumulative sum of AppendResponses.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendResponsesView *view.View

	// AppendResponseErrorsView is a cumulative sum of AppendResponseErrors.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendResponseErrorsView *view.View

	// AppendRetryView is a cumulative sum of AppendRetryCount.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	AppendRetryView *view.View

	// FlushRequestsView is a cumulative sum of FlushRequests.
	// It is EXPERIMENTAL and subject to change or removal without notice.
	FlushRequestsView *view.View
)

func init() {
	AppendClientOpenView = createSumView(stats.Measure(AppendClientOpenCount), keyError)
	AppendClientOpenRetryView = createSumView(stats.Measure(AppendClientOpenRetryCount))

	AppendRequestsView = createSumView(stats.Measure(AppendRequests), keyStream, keyDataOrigin)
	AppendRequestBytesView = createSumView(stats.Measure(AppendRequestBytes), keyStream, keyDataOrigin)
	AppendRequestErrorsView = createSumView(stats.Measure(AppendRequestErrors), keyStream, keyDataOrigin, keyError)
	AppendRequestReconnectsView = createSumView(stats.Measure(AppendRequestReconnects), keyStream, keyDataOrigin, keyError)
	AppendRequestRowsView = createSumView(stats.Measure(AppendRequestRows), keyStream, keyDataOrigin)

	AppendResponsesView = createSumView(stats.Measure(AppendResponses), keyStream, keyDataOrigin)
	AppendResponseErrorsView = createSumView(stats.Measure(AppendResponseErrors), keyStream, keyDataOrigin, keyError)
	AppendRetryView = createSumView(stats.Measure(AppendRetryCount), keyStream, keyDataOrigin)
	FlushRequestsView = createSumView(stats.Measure(FlushRequests), keyStream, keyDataOrigin)

	DefaultOpenCensusViews = []*view.View{
		AppendClientOpenView,
		AppendClientOpenRetryView,

		AppendRequestsView,
		AppendRequestBytesView,
		AppendRequestErrorsView,
		AppendRequestReconnectsView,
		AppendRequestRowsView,

		AppendResponsesView,
		AppendResponseErrorsView,
		AppendRetryView,

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

// setupWriterStatContext returns a new context modified with the instrumentation tags.
// This will panic if no managedstream is provided
func setupWriterStatContext(ms *ManagedStream) context.Context {
	if ms == nil {
		panic("no ManagedStream provided")
	}
	kCtx := ms.ctx
	if ms.streamSettings == nil {
		return kCtx
	}
	if ms.streamSettings.streamID != "" {
		ctx, err := tag.New(kCtx, tag.Upsert(keyStream, ms.streamSettings.streamID))
		if err != nil {
			return kCtx // failed to add a tag, return the original context.
		}
		kCtx = ctx
	}
	if ms.streamSettings.dataOrigin != "" {
		ctx, err := tag.New(kCtx, tag.Upsert(keyDataOrigin, ms.streamSettings.dataOrigin))
		if err != nil {
			return kCtx
		}
		kCtx = ctx
	}
	return kCtx
}

// recordWriterStat records a measure which may optionally contain writer-related tags like stream ID
// or data origin.
func recordWriterStat(ms *ManagedStream, m *stats.Int64Measure, n int64) {
	stats.Record(ms.ctx, m.M(n))
}

func recordStat(ctx context.Context, m *stats.Int64Measure, n int64) {
	stats.Record(ctx, m.M(n))
}
