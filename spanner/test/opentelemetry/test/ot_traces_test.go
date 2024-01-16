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
	old := trace.OpenTelemetryTracingEnabled
	trace.OpenTelemetryTracingEnabled = true

	t.Cleanup(func() {
		trace.OpenTelemetryTracingEnabled = old
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
	// Preferably we would want to lock the TestExporter here, but the mutex TestExporter.mu is not exported, so we
	// cannot do that.
	if len(spans) == 0 {
		t.Fatal("No spans were exported")
	}
	spanName := "cloud.google.com/go/spanner.Query"
	if !findSpan(spans, spanName) {
		t.Errorf("Expected span %s not found", spanName)
	}
	/*s := spans[len(spans)-1].Status
	if want := otcodes.Ok; s.Code != want {
		t.Errorf("got %v, want %v", s.Code, want)
	}*/
}

func findSpan(spans tracetest.SpanStubs, spanName string) bool {
	for _, span := range spans {
		if span.Name == spanName {
			return true
		}
	}
	return false
}
