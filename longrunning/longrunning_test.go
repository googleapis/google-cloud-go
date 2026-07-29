// Copyright 2016 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package lro supports Long Running Operations for the Google Cloud Libraries.
//
// This package is still experimental and subject to change.
package longrunning

import (
	"context"
	"errors"
	"testing"
	"time"

	pb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	gax "github.com/googleapis/gax-go/v2"
	"github.com/googleapis/gax-go/v2/apierror"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	rpcstatus "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
)

type getterService struct {
	operationsClient

	// clock represents the fake current time of the service.
	// It is the running sum of the of the duration we have slept.
	clock time.Duration

	// getTimes records the times at which GetOperation is called.
	getTimes []time.Duration

	// results are the fake results that GetOperation should return.
	results []*pb.Operation
}

func (s *getterService) GetOperation(context.Context, *pb.GetOperationRequest, ...gax.CallOption) (*pb.Operation, error) {
	i := len(s.getTimes)
	s.getTimes = append(s.getTimes, s.clock)
	if i >= len(s.results) {
		return nil, errors.New("unexpected call")
	}
	return s.results[i], nil
}

func (s *getterService) sleeper() sleeper {
	return func(_ context.Context, d time.Duration) error {
		s.clock += d
		return nil
	}
}

func TestWait(t *testing.T) {
	responseDur := durationpb.New(42 * time.Second)
	responseAny, err := anypb.New(responseDur)
	if err != nil {
		t.Fatal(err)
	}

	s := &getterService{
		results: []*pb.Operation{
			{Name: "foo"},
			{Name: "foo"},
			{Name: "foo"},
			{Name: "foo"},
			{Name: "foo"},
			{
				Name: "foo",
				Done: true,
				Result: &pb.Operation_Response{
					Response: responseAny,
				},
			},
		},
	}
	op := &Operation{
		c:     s,
		proto: &pb.Operation{Name: "foo"},
	}
	if op.Done() {
		t.Fatal("operation should not have completed yet")
	}

	var resp durationpb.Duration
	bo := gax.Backoff{
		Initial: 1 * time.Second,
		Max:     3 * time.Second,
	}
	if err := op.wait(context.Background(), &resp, &bo, s.sleeper()); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(&resp, responseDur) {
		t.Errorf("response, got %v, want %v", &resp, responseDur)
	}
	if !op.Done() {
		t.Errorf("operation should have completed")
	}

	maxWait := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		3 * time.Second,
		3 * time.Second,
		3 * time.Second,
	}
	for i := 0; i < len(s.getTimes)-1; i++ {
		w := s.getTimes[i+1] - s.getTimes[i]
		if mw := maxWait[i]; w > mw {
			t.Errorf("backoff, waited %s, max %s", w, mw)
		}
	}
}

func TestPollRequestError(t *testing.T) {
	const opName = "foo"

	// All calls error.
	s := &getterService{}
	op := &Operation{
		c:     s,
		proto: &pb.Operation{Name: opName},
	}
	if err := op.Poll(context.Background(), nil); err == nil {
		t.Fatalf("Poll should error")
	}
	if n := op.Name(); n != opName {
		t.Errorf("operation name, got %q, want %q", n, opName)
	}
	if op.Done() {
		t.Errorf("operation should not have completed; we failed to fetch state")
	}
}

func TestPollErrorResult(t *testing.T) {
	const (
		errCode = codes.NotFound
		errMsg  = "my error"
	)
	details := &errdetails.ErrorInfo{Reason: "things happen"}
	a, err := anypb.New(details)
	if err != nil {
		t.Fatalf("anypb.New() = %v", err)
	}
	op := &Operation{
		proto: &pb.Operation{
			Name: "foo",
			Done: true,
			Result: &pb.Operation_Error{
				Error: &rpcstatus.Status{
					Code:    int32(errCode),
					Message: errMsg,
					Details: []*anypb.Any{a},
				},
			},
		},
	}
	err = op.Poll(context.Background(), nil)
	if got := status.Code(err); got != errCode {
		t.Errorf("error code, want %s, got %s", errCode, got)
	}
	if got := grpc.ErrorDesc(err); got != errMsg {
		t.Errorf("error code, want %s, got %s", errMsg, got)
	}
	if !op.Done() {
		t.Errorf("operation should have completed")
	}
	var ae *apierror.APIError
	errors.As(err, &ae)
	if got := ae.Details().ErrorInfo.Reason; got != details.Reason {
		t.Errorf("got %q, want %q", got, details.Reason)
	}
}

type errService struct {
	operationsClient
	errCancel, errDelete error
}

func (s *errService) CancelOperation(context.Context, *pb.CancelOperationRequest, ...gax.CallOption) error {
	return s.errCancel
}

func (s *errService) DeleteOperation(context.Context, *pb.DeleteOperationRequest, ...gax.CallOption) error {
	return s.errDelete
}

func TestCancelReturnsError(t *testing.T) {
	s := &errService{
		errCancel: errors.New("cancel error"),
	}
	op := &Operation{
		c:     s,
		proto: &pb.Operation{Name: "foo"},
	}
	if got, want := op.Cancel(context.Background()), s.errCancel; got != want {
		t.Errorf("cancel, got error %s, want %s", got, want)
	}
}

func TestDeleteReturnsError(t *testing.T) {
	s := &errService{
		errDelete: errors.New("delete error"),
	}
	op := &Operation{
		c:     s,
		proto: &pb.Operation{Name: "foo"},
	}
	if got, want := op.Delete(context.Background()), s.errDelete; got != want {
		t.Errorf("cancel, got error %s, want %s", got, want)
	}
}

func TestInternalNewOperationWithMetadata(t *testing.T) {
	opName := "test-operation"
	op := InternalNewOperationWithMetadata(nil, &pb.Operation{Name: "foo"}, opName)
	if op.opName != opName {
		t.Errorf("expected opName to be %q, got %q", opName, op.opName)
	}

	sc := trace.SpanContext{}
	op.SetParentSpanContext(sc)
	if !op.initSpanContext.Equal(sc) {
		t.Error("expected initSpanContext to match the set SpanContext")
	}
}

func TestWaitTraced(t *testing.T) {
	t.Setenv("GOOGLE_SDK_GO_EXPERIMENTAL_TRACING", "true")
	if !gax.IsFeatureEnabled("TRACING") {
		t.Skip("TRACING feature flag is not enabled")
	}

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	oldTP := otel.GetTracerProvider()
	defer otel.SetTracerProvider(oldTP)
	otel.SetTracerProvider(tp)

	responseDur := durationpb.New(42 * time.Second)
	responseAny, err := anypb.New(responseDur)
	if err != nil {
		t.Fatal(err)
	}

	s := &getterService{
		results: []*pb.Operation{
			{Name: "foo"},
			{Name: "foo"},
			{
				Name: "foo",
				Done: true,
				Result: &pb.Operation_Response{
					Response: responseAny,
				},
			},
		},
	}

	op := &Operation{
		c:      s,
		proto:  &pb.Operation{Name: "foo"},
		opName: "*speech.Client.BatchRecognizeOperation",
	}

	ctx := context.Background()
	tracer := otel.GetTracerProvider().Tracer("test-tracer")
	parentCtx, parentSpan := tracer.Start(ctx, "creation-span")
	op.SetParentSpanContext(parentSpan.SpanContext())

	var resp durationpb.Duration
	err = op.waitWithInterval(parentCtx, &resp, 3*time.Millisecond, s.sleeper())
	parentSpan.End()

	if err != nil {
		t.Fatal(err)
	}

	spans := sr.Ended()
	var waitSpan sdktrace.ReadOnlySpan
	var pollSpans []sdktrace.ReadOnlySpan
	var sleepSpans []sdktrace.ReadOnlySpan

	for _, span := range spans {
		switch span.Name() {
		case "*speech.Client.BatchRecognizeOperation.Wait":
			waitSpan = span
		case "*longrunning.OperationsClient.GetOperation":
			pollSpans = append(pollSpans, span)
		case "LRO Sleep":
			sleepSpans = append(sleepSpans, span)
		}
	}

	if waitSpan == nil {
		t.Fatal("expected a T2 LRO Wait span, got nil")
	}

	links := waitSpan.Links()
	if len(links) != 1 {
		t.Fatalf("expected 1 span link, got %d", len(links))
	}
	if links[0].SpanContext.SpanID() != parentSpan.SpanContext().SpanID() {
		t.Errorf("expected span link pointing to parent span, got different span ID")
	}

	waitAttrs := waitSpan.Attributes()
	hasResID := false
	for _, attr := range waitAttrs {
		if attr.Key == "gcp.resource.destination.id" && attr.Value.AsString() == "foo" {
			hasResID = true
		}
	}
	if !hasResID {
		t.Error("expected wait span to have gcp.resource.destination.id attribute set to 'foo'")
	}

	// Verify T3 Poll Spans
	if len(pollSpans) != 3 {
		t.Fatalf("expected 3 poll spans, got %d", len(pollSpans))
	}

	verifyPollAndSleepSpans(t, pollSpans, sleepSpans, "foo")
}

func verifyPollAndSleepSpans(t *testing.T, pollSpans, sleepSpans []sdktrace.ReadOnlySpan, expectedResID string) {
	t.Helper()
	for i, pollSpan := range pollSpans {
		attrs := pollSpan.Attributes()
		var resID string
		var hasCount bool
		var count int
		var done bool
		for _, attr := range attrs {
			if attr.Key == "gcp.resource.destination.id" {
				resID = attr.Value.AsString()
			}
			if attr.Key == "gcp.longrunning.poll_attempt_count" {
				hasCount = true
				count = int(attr.Value.AsInt64())
			}
			if attr.Key == "gcp.longrunning.done" {
				done = attr.Value.AsBool()
			}
		}

		if resID != expectedResID {
			t.Errorf("poll span %d expected gcp.resource.destination.id to be '%s', got '%s'", i, expectedResID, resID)
		}
		if !hasCount || count != i+1 {
			t.Errorf("poll span %d expected count to be %d, got %d", i, i+1, count)
		}

		if i != len(pollSpans)-1 && done {
			t.Errorf("poll span %d expected gcp.longrunning.done to be set", i)
		}

		if i == len(pollSpans)-1 {
			if !done {
				t.Error("expected done to be true on terminal poll span")
			}
			hasStatus := false
			var statusCode int
			for _, attr := range attrs {
				if attr.Key == "gcp.longrunning.status_code" {
					statusCode = int(attr.Value.AsInt64())
					hasStatus = true
				}
			}
			if !hasStatus || statusCode != 0 {
				t.Errorf("expected status code 0 on terminal poll, got %d (hasStatus: %t)", statusCode, hasStatus)
			}
		}
	}

	// Verify T5 Sleep Spans
	expectedSleeps := len(pollSpans) - 1
	if len(sleepSpans) != expectedSleeps {
		t.Errorf("expected %d sleep spans, got %d", expectedSleeps, len(sleepSpans))
	}
}

func TestWaitTracedResumed(t *testing.T) {
	t.Setenv("GOOGLE_SDK_GO_EXPERIMENTAL_TRACING", "true")
	if !gax.IsFeatureEnabled("TRACING") {
		t.Skip("TRACING feature flag is not enabled")
	}

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	oldTP := otel.GetTracerProvider()
	defer otel.SetTracerProvider(oldTP)
	otel.SetTracerProvider(tp)

	responseDur := durationpb.New(42 * time.Second)
	responseAny, err := anypb.New(responseDur)
	if err != nil {
		t.Fatal(err)
	}

	s := &getterService{
		results: []*pb.Operation{
			{Name: "foo"},
			{
				Name: "foo",
				Done: true,
				Result: &pb.Operation_Response{
					Response: responseAny,
				},
			},
		},
	}

	op := &Operation{
		c:      s,
		proto:  &pb.Operation{Name: "foo"},
		opName: "*speech.BatchRecognizeOperation",
	}

	ctx := context.Background()
	tracer := otel.GetTracerProvider().Tracer("test-tracer")
	parentCtx, parentSpan := tracer.Start(ctx, "resumed-polling")

	var resp durationpb.Duration
	err = op.waitWithInterval(parentCtx, &resp, 3*time.Millisecond, s.sleeper())
	parentSpan.End()

	if err != nil {
		t.Fatal(err)
	}

	spans := sr.Ended()
	var waitSpan sdktrace.ReadOnlySpan
	var pollSpans []sdktrace.ReadOnlySpan
	var sleepSpans []sdktrace.ReadOnlySpan

	for _, span := range spans {
		switch span.Name() {
		case "*speech.BatchRecognizeOperation.Wait":
			waitSpan = span
		case "*longrunning.OperationsClient.GetOperation":
			pollSpans = append(pollSpans, span)
		case "LRO Sleep":
			sleepSpans = append(sleepSpans, span)
		}
	}

	if waitSpan == nil {
		t.Fatal("expected a T2 LRO Wait span, got nil")
	}

	links := waitSpan.Links()
	if len(links) != 0 {
		t.Fatalf("expected 0 span links for resumed LRO tracing, got %d", len(links))
	}

	if len(pollSpans) != 2 {
		t.Fatalf("expected 2 poll spans, got %d", len(pollSpans))
	}

	verifyPollAndSleepSpans(t, pollSpans, sleepSpans, "foo")
}

func TestWaitTracedFallback(t *testing.T) {
	t.Setenv("GOOGLE_SDK_GO_EXPERIMENTAL_TRACING", "true")
	if !gax.IsFeatureEnabled("TRACING") {
		t.Skip("TRACING feature flag is not enabled")
	}

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	oldTP := otel.GetTracerProvider()
	defer otel.SetTracerProvider(oldTP)
	otel.SetTracerProvider(tp)

	responseDur := durationpb.New(42 * time.Second)
	responseAny, err := anypb.New(responseDur)
	if err != nil {
		t.Fatal(err)
	}

	s := &getterService{
		results: []*pb.Operation{
			{Name: "foo"},
			{
				Name: "foo",
				Done: true,
				Result: &pb.Operation_Response{
					Response: responseAny,
				},
			},
		},
	}

	op := &Operation{
		c:     s,
		proto: &pb.Operation{Name: "foo"},
	}

	ctx := context.Background()
	tracer := otel.GetTracerProvider().Tracer("test-tracer")
	parentCtx, parentSpan := tracer.Start(ctx, "fallback-polling")

	var resp durationpb.Duration
	err = op.waitWithInterval(parentCtx, &resp, 3*time.Millisecond, s.sleeper())
	parentSpan.End()

	if err != nil {
		t.Fatal(err)
	}

	spans := sr.Ended()
	var waitSpan sdktrace.ReadOnlySpan
	var pollSpans []sdktrace.ReadOnlySpan
	var sleepSpans []sdktrace.ReadOnlySpan

	for _, span := range spans {
		switch span.Name() {
		case "*longrunning.Operation.Wait":
			waitSpan = span
		case "*longrunning.OperationsClient.GetOperation":
			pollSpans = append(pollSpans, span)
		case "LRO Sleep":
			sleepSpans = append(sleepSpans, span)
		}
	}

	if waitSpan == nil {
		t.Fatal("expected a T2 LRO Wait span, got nil")
	}

	links := waitSpan.Links()
	if len(links) != 0 {
		t.Fatalf("expected 0 span links for fallback LRO tracing, got %d", len(links))
	}

	if len(pollSpans) != 2 {
		t.Fatalf("expected 2 poll spans, got %d", len(pollSpans))
	}

	verifyPollAndSleepSpans(t, pollSpans, sleepSpans, "foo")
}
