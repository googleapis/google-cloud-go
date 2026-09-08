// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package storage

import (
	"context"
	"io"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"cloud.google.com/go/storage/internal/apiv2/storagepb"
)

type mockBidiStream struct {
	storagepb.Storage_BidiReadObjectClient
	resps []*storagepb.BidiReadObjectResponse
	err   error
	i     int
}

func (m *mockBidiStream) Recv() (*storagepb.BidiReadObjectResponse, error) {
	if m.i < len(m.resps) {
		r := m.resps[m.i]
		m.i++
		return r, nil
	}
	if m.err != nil {
		return nil, m.err
	}
	return nil, io.EOF
}

func TestBidiTracing_receiveLoop(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(sr))
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test_stream_open")

	ch := make(chan mrdSessionResult, 10)

	s := &bidiReadStreamSession{
		managerCtx: ctx,
		ctx:        ctx,
		client:     &grpcStorageClient{},
		firstResp:  true,
		t0:         time.Now().Add(-10 * time.Millisecond),
		t1:         time.Now().Add(-8 * time.Millisecond),
		t2:         time.Now().Add(-5 * time.Millisecond),
		respC:      ch,
		stream: &mockBidiStream{
			resps: []*storagepb.BidiReadObjectResponse{
				{ObjectDataRanges: []*storagepb.ObjectRangeData{}},
			},
			err: io.EOF,
		},
	}

	s.receiveLoop()
	span.End()

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	gotSpan := spans[0]

	found := false
	for _, ev := range gotSpan.Events() {
		if ev.Name == "gcp.storage.client.bidi.stream_open" {
			found = true
			if len(ev.Attributes) != 3 {
				t.Errorf("expected 3 attributes, got %d", len(ev.Attributes))
			}
			var numFound int
			for _, attr := range ev.Attributes {
				if attr.Key == "gcp.storage.client.bidi.latency.network_handshake_us" ||
					attr.Key == "gcp.storage.client.bidi.latency.server_metadata_us" ||
					attr.Key == "gcp.storage.client.bidi.latency.stream_open_us" {
					numFound++
				}
			}
			if numFound != 3 {
				t.Errorf("expected the 3 specific attributes, only found %d", numFound)
			}
		}
	}
	if !found {
		t.Error("expected event gcp.storage.client.bidi.stream_open not found in span")
	}
}

func TestBidiTracing_processDataRanges(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(sr))
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test_read_range")

	m := &multiRangeDownloaderManager{
		spanCtx: ctx,
		client:  &grpcStorageClient{},
	}

	req := &rangeRequest{}
	mrdStream := &mrdStream{
		pendingRanges: map[int64]*rangeRequest{
			1: req,
		},
	}

	session := &bidiReadStreamSession{}
	session.t4Map.Store(int64(1), time.Now().Add(-10*time.Millisecond))

	result := mrdSessionResult{
		decoder: &readResponseDecoder{},
		session: session,
		t5:      time.Now().Add(-5 * time.Millisecond),
		t6:      time.Now().Add(-2 * time.Millisecond),
	}
	resp := &storagepb.BidiReadObjectResponse{
		ObjectDataRanges: []*storagepb.ObjectRangeData{
			{
				ReadRange: &storagepb.ReadRange{ReadId: 1},
			},
		},
	}

	m.processDataRanges(result, mrdStream, resp)

	span.End()

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	gotSpan := spans[0]

	found := false
	for _, ev := range gotSpan.Events() {
		if ev.Name == "gcp.storage.client.bidi.read_range" {
			found = true
			if len(ev.Attributes) != 4 {
				t.Errorf("expected 4 attributes, got %d", len(ev.Attributes))
			}
			var numFound int
			for _, attr := range ev.Attributes {
				if attr.Key == "gcp.storage.client.bidi.latency.network_transit_us" ||
					attr.Key == "gcp.storage.client.bidi.latency.sdk_processing_overhead_us" ||
					attr.Key == "gcp.storage.client.bidi.latency.client_handoff_delay_us" ||
					attr.Key == "gcp.storage.client.bidi.latency.end_to_end_us" {
					numFound++
				}
			}
			if numFound != 4 {
				t.Errorf("expected 4 specific attributes, got %d", numFound)
			}
		}
	}
	if !found {
		t.Error("expected event gcp.storage.client.bidi.read_range not found in span")
	}
}
