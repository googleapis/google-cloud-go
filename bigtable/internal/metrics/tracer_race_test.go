/*
Copyright 2026 Google LLC

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

package internal

import (
	"context"
	"errors"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/metric/noop"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/protobuf/proto"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
)

// TestHandleRPC_ConcurrentInTrailerAndEnd_NoRace pins down the fix for
// https://github.com/googleapis/google-cloud-go/issues/20152 and the
// sibling FlakyBot P1 issues (20147, 20148, 20150, 20151) that all
// shipped from the same nightly run.
//
// Before the fix, HandleRPC stored ev.Header / ev.Trailer on the
// current attempt and re-read them from the stats.End branch. On the
// happy path all three events dispatch from the transport reader
// goroutine and there is no race — but under cancel / deadline /
// GOAWAY, csAttempt.finish (stream.go:1251) dispatches End on the
// caller goroutine while the reader is still processing the trailer
// frame. That interleave was observed on a TestIntegration_Presidents
// run and produced:
//
//	WARNING: DATA RACE
//	  Read  at ... bigtable/internal/metrics/tracer.go:867
//	  Write at ... bigtable/internal/metrics/tracer.go:859
//
// plus two follow-on races on the underlying metadata.MD map (MD.Copy
// vs MD.Get from extractLocation).
//
// This test reproduces the interleave deterministically: it fires
// InHeader on the "reader" goroutine, then races InTrailer against End
// across two goroutines. Under `go test -race`, the pre-fix code
// panics; the post-fix code passes because End reads only primitive
// AttemptTracer fields that ingestMetadata wrote on the reader side.
func TestHandleRPC_ConcurrentInTrailerAndEnd_NoRace(t *testing.T) {
	tf, err := NewFactoryForTest("proj", "inst", "app", noop.NewMeterProvider())
	if err != nil {
		t.Fatalf("NewFactoryForTest: %v", err)
	}
	sh := &StatsHandler{}

	// Build a small ResponseParams payload so extractLocation has real
	// data to iterate (the map-content race in the original dump was
	// on the MD map that carried this key).
	cluster, zone := "test-cluster", "test-zone"
	rp, err := proto.Marshal(&btpb.ResponseParams{ClusterId: &cluster, ZoneId: &zone})
	if err != nil {
		t.Fatalf("marshal ResponseParams: %v", err)
	}
	trailerMD := metadata.MD{
		LocationMDKey:     []string{string(rp)},
		ServerTimingMDKey: []string{"gfet4t7; dur=42"},
	}

	// 200 iterations comfortably fits the race detector's shadow-state
	// budget for a targeted regression, without stretching the short
	// CI wall time.
	const iterations = 200
	for i := 0; i < iterations; i++ {
		tracer := tf.CreateTracer(context.Background(), "test-table", false)
		tracer.SetMethod("ReadRows")
		tracer.currOp.currAttempt = AttemptTracer{}
		ctx := NewContext(context.Background(), tracer)

		// Initial header arrives on the transport reader in the real
		// dispatch. Fire it inline (no goroutine) since InHeader always
		// precedes both InTrailer and End on the wire — the race is
		// specifically the InTrailer-vs-End overlap.
		sh.HandleRPC(ctx, &stats.InHeader{Client: true, Header: metadata.MD{}})

		var wg sync.WaitGroup
		wg.Add(2)
		// "Reader" goroutine: still processing the trailer frame.
		go func() {
			defer wg.Done()
			sh.HandleRPC(ctx, &stats.InTrailer{Client: true, Trailer: trailerMD})
		}()
		// "Caller" goroutine: csAttempt.finish path (simulating cancel).
		go func() {
			defer wg.Done()
			sh.HandleRPC(ctx, &stats.End{Client: true, Error: errors.New("canceled by caller")})
		}()
		wg.Wait()
	}
}
