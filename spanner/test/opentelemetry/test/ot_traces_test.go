//go:build go1.20
// +build go1.20

/*
Copyright 2024 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package test

import (
	"context"
	"errors"
	"testing"

	"cloud.google.com/go/internal/trace"
	"cloud.google.com/go/spanner"
	stestutil "cloud.google.com/go/spanner/internal/testutil"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestSpannerTracesWithOpenTelemetry(t *testing.T) {
	ctx := context.Background()
	te := newOpenTelemetryTestExporter(false, true)
	old := trace.IsOpenTelemetryTracingEnabled()
	trace.SetOpenTelemetryTracingEnabledField(true)

	t.Cleanup(func() {
		trace.SetOpenTelemetryTracingEnabledField(old)
		te.Unregister(ctx)
	})

	if trace.IsOpenCensusTracingEnabled() {
		t.Errorf("got true, want false")
	}
	if !trace.IsOpenTelemetryTracingEnabled() {
		t.Errorf("got false, want true")
	}

	minOpened := uint64(1)
	server, client, teardown := setupMockedTestServerWithConfig(t, spanner.ClientConfig{
		SessionPoolConfig: spanner.SessionPoolConfig{
			MinOpened: minOpened,
		},
	})
	defer teardown()

	waitFor(t, func() error {
		if uint64(server.TestSpanner.TotalSessionsCreated()) == minOpened {
			return nil
		}
		return errors.New("not yet initialized")
	})

	iter := client.Single().Query(context.Background(), spanner.NewStatement(stestutil.SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	iter.Do(func(r *spanner.Row) error {
		return nil
	})
	spans := te.spans()
	if len(spans) == 0 {
		t.Fatal("No spans were exported")
	}
	spanName := "cloud.google.com/go/spanner.Query"
	if !findSpan(spans, spanName) {
		t.Errorf("Expected span %s not found", spanName)
	}
}

func findSpan(spans tracetest.SpanStubs, spanName string) bool {
	for _, span := range spans {
		if span.Name == spanName {
			return true
		}
	}
	return false
}
