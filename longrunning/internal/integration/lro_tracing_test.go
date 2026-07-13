// Copyright 2026 Google LLC
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

package integration

import (
	"context"
	"net"
	"testing"

	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	texttospeech "cloud.google.com/go/texttospeech/apiv1"
	texttospeechpb "cloud.google.com/go/texttospeech/apiv1/texttospeechpb"
	gax "github.com/googleapis/gax-go/v2"
	otel "go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	tracetest "go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type mockLongAudioServer struct {
	texttospeechpb.UnimplementedTextToSpeechLongAudioSynthesizeServer
	longrunningpb.UnimplementedOperationsServer

	reqs  []proto.Message
	resps []proto.Message
	err   error
}

func (s *mockLongAudioServer) SynthesizeLongAudio(ctx context.Context, req *texttospeechpb.SynthesizeLongAudioRequest) (*longrunningpb.Operation, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*longrunningpb.Operation), nil
}

func (s *mockLongAudioServer) GetOperation(ctx context.Context, req *longrunningpb.GetOperationRequest) (*longrunningpb.Operation, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[1].(*longrunningpb.Operation), nil
}

func TestLongAudioSynthesizeTracing(t *testing.T) {
	t.Setenv("GOOGLE_SDK_GO_EXPERIMENTAL_TRACING", "true")
	if !gax.IsFeatureEnabled("TRACING") {
		t.Skip("TRACING feature flag is not enabled")
	}

	// Setup mock OTel TracerProvider
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	oldTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(oldTP)

	// Setup local gRPC mock server
	mock := &mockLongAudioServer{}
	serv := grpc.NewServer()
	texttospeechpb.RegisterTextToSpeechLongAudioSynthesizeServer(serv, mock)
	longrunningpb.RegisterOperationsServer(serv, mock)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	go serv.Serve(lis)
	defer serv.Stop()

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Setup mock responses
	expectedResponse := &texttospeechpb.SynthesizeLongAudioResponse{}
	anyResp, err := anypb.New(expectedResponse)
	if err != nil {
		t.Fatal(err)
	}

	mock.resps = []proto.Message{
		&longrunningpb.Operation{
			Name: "projects/test-project/locations/global/operations/op-123",
		},
		&longrunningpb.Operation{
			Name:   "projects/test-project/locations/global/operations/op-123",
			Done:   true,
			Result: &longrunningpb.Operation_Response{Response: anyResp},
		},
	}

	// Create client pointing to mock server
	client, err := texttospeech.NewTextToSpeechLongAudioSynthesizeClient(context.Background(), option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Call LRO method
	ctx := context.Background()
	tracer := otel.GetTracerProvider().Tracer("test-tracer")
	parentCtx, parentSpan := tracer.Start(ctx, "immediate-polling")

	op, err := client.SynthesizeLongAudio(parentCtx, &texttospeechpb.SynthesizeLongAudioRequest{
		Parent: "projects/test-project/locations/global",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for LRO
	_, err = op.Wait(parentCtx)
	parentSpan.End()

	if err != nil {
		t.Fatal(err)
	}

	// Verify spans
	spans := sr.Ended()
	if len(spans) < 3 {
		t.Fatalf("expected at least 3 spans, got %d", len(spans))
	}

	var waitSpan sdktrace.ReadOnlySpan
	var pollSpans []sdktrace.ReadOnlySpan

	for _, span := range spans {
		switch span.Name() {
		case "*texttospeech.SynthesizeLongAudioOperation.Wait":
			waitSpan = span
		case "*longrunning.OperationsClient.GetOperation":
			pollSpans = append(pollSpans, span)
		}
	}

	if waitSpan == nil {
		t.Fatal("expected T2 wait span, got nil")
	}

	// Verify T2 Wait Span has a Link back to the parent span
	links := waitSpan.Links()
	if len(links) != 1 {
		t.Fatalf("expected 1 span link, got %d", len(links))
	}
	if links[0].SpanContext.SpanID() != parentSpan.SpanContext().SpanID() {
		t.Errorf("expected T2 Wait span link to point to parent span, got different span ID")
	}

	// Verify Resource ID on T2 Wait Span
	waitAttrs := waitSpan.Attributes()
	hasResID := false
	for _, attr := range waitAttrs {
		if attr.Key == "gcp.resource.destination.id" && attr.Value.AsString() == "projects/test-project/locations/global/operations/op-123" {
			hasResID = true
		}
	}
	if !hasResID {
		t.Error("expected wait span to have gcp.resource.destination.id set to Operation ID")
	}

	// Verify T3 Poll Spans
	if len(pollSpans) != 1 {
		t.Fatalf("expected 1 poll attempt span, got %d", len(pollSpans))
	}

	pollSpan := pollSpans[0]
	pollAttrs := pollSpan.Attributes()
	var resID string
	var attempt int
	var done bool
	var statusCode int
	var hasDone, hasAttempt, hasStatus bool

	for _, attr := range pollAttrs {
		switch attr.Key {
		case "gcp.resource.destination.id":
			resID = attr.Value.AsString()
		case "gcp.longrunning.poll_attempt_count":
			attempt = int(attr.Value.AsInt64())
			hasAttempt = true
		case "gcp.longrunning.done":
			done = attr.Value.AsBool()
			hasDone = true
		case "gcp.longrunning.status_code":
			statusCode = int(attr.Value.AsInt64())
			hasStatus = true
		}
	}

	if resID != "projects/test-project/locations/global/operations/op-123" {
		t.Errorf("expected poll span resource ID to be Operation ID, got '%s'", resID)
	}
	if !hasAttempt || attempt != 1 {
		t.Errorf("expected poll attempt count to be 1, got %d", attempt)
	}
	if !hasDone || !done {
		t.Errorf("expected done to be true on terminal poll, got %t", done)
	}
	if !hasStatus || statusCode != 0 {
		t.Errorf("expected status code to be 0 on terminal poll, got %d", statusCode)
	}
}

func TestLongAudioSynthesizeTracingResumed(t *testing.T) {
	t.Setenv("GOOGLE_SDK_GO_EXPERIMENTAL_TRACING", "true")
	if !gax.IsFeatureEnabled("TRACING") {
		t.Skip("TRACING feature flag is not enabled")
	}

	// 1. Setup mock server and exporter
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	oldTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(oldTP)

	mockServer := &mockLongAudioServer{}
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	lis, err := net.ListenTCP("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer()
	texttospeechpb.RegisterTextToSpeechLongAudioSynthesizeServer(s, mockServer)
	longrunningpb.RegisterOperationsServer(s, mockServer)
	go s.Serve(lis)
	defer s.Stop()

	// 2. Setup client
	ctx := context.Background()
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client, err := texttospeech.NewTextToSpeechLongAudioSynthesizeClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// 3. Resumed LRO setup
	opName := "projects/test-project/locations/global/operations/op-123"
	responseProto := &texttospeechpb.SynthesizeLongAudioResponse{}
	responseAny, err := anypb.New(responseProto)
	if err != nil {
		t.Fatal(err)
	}
	opProto := &longrunningpb.Operation{
		Name: opName,
		Done: true,
		Result: &longrunningpb.Operation_Response{
			Response: responseAny,
		},
	}
	mockServer.resps = []proto.Message{opProto, opProto}

	op := client.SynthesizeLongAudioOperation(opName)

	// 4. Wait for LRO to complete
	tracer := otel.GetTracerProvider().Tracer("test-tracer")
	parentCtx, parentSpan := tracer.Start(ctx, "resumed-polling")

	_, err = op.Wait(parentCtx)
	parentSpan.End()

	if err != nil {
		t.Fatal(err)
	}

	// 5. Verify spans
	spans := sr.Ended()
	var waitSpan sdktrace.ReadOnlySpan
	var pollSpans []sdktrace.ReadOnlySpan
	for _, span := range spans {
		switch span.Name() {
		case "*texttospeech.SynthesizeLongAudioOperation.Wait":
			waitSpan = span
		case "*longrunning.OperationsClient.GetOperation":
			pollSpans = append(pollSpans, span)
		}
	}

	if waitSpan == nil {
		t.Fatal("expected T2 wait span, got nil")
	}

	// Verify NO Span Links are created in resumed polling
	links := waitSpan.Links()
	if len(links) != 0 {
		t.Fatalf("expected 0 span links for resumed LRO tracing, got %d", len(links))
	}

	// Verify Resource ID on T2 Wait Span
	waitAttrs := waitSpan.Attributes()
	hasResID := false
	for _, attr := range waitAttrs {
		if attr.Key == "gcp.resource.destination.id" && attr.Value.AsString() == opName {
			hasResID = true
		}
	}
	if !hasResID {
		t.Error("expected wait span to have gcp.resource.destination.id set to Operation ID")
	}

	// Verify T3 Poll Spans
	if len(pollSpans) != 1 {
		t.Fatalf("expected 1 poll attempt span, got %d", len(pollSpans))
	}

	pollSpan := pollSpans[0]
	pollAttrs := pollSpan.Attributes()
	var resID string
	var attempt int
	var done bool
	var statusCode int
	var hasDone, hasAttempt, hasStatus bool

	for _, attr := range pollAttrs {
		switch attr.Key {
		case "gcp.resource.destination.id":
			resID = attr.Value.AsString()
		case "gcp.longrunning.poll_attempt_count":
			attempt = int(attr.Value.AsInt64())
			hasAttempt = true
		case "gcp.longrunning.done":
			done = attr.Value.AsBool()
			hasDone = true
		case "gcp.longrunning.status_code":
			statusCode = int(attr.Value.AsInt64())
			hasStatus = true
		}
	}

	if resID != opName {
		t.Errorf("expected poll span resource ID to be Operation ID, got '%s'", resID)
	}
	if !hasAttempt || attempt != 1 {
		t.Errorf("expected poll attempt count to be 1, got %d", attempt)
	}
	if !hasDone || !done {
		t.Errorf("expected done to be true on terminal poll, got %t", done)
	}
	if !hasStatus || statusCode != 0 {
		t.Errorf("expected status code to be 0 on terminal poll, got %d", statusCode)
	}
}
