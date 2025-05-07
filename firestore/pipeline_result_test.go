// Copyright 2025 Google LLC
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

package firestore

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestStreamPipelineResultIterator_Next(t *testing.T) {
	ctx := context.Background()
	client := newTestClient() // For PipelineResult construction
	p := &Pipeline{c: client} // Dummy pipeline for iterator context

	now := time.Now()
	tsNow := timestamppb.New(now)
	ts2MinLater := timestamppb.New(now.Add(-2 * time.Minute))

	mockResponses := []*pb.ExecutePipelineResponse{
		{ // First response with two results
			Results: []*pb.Document{
				{Name: "projects/test-project/databases/test-db/documents/col/doc1", Fields: map[string]*pb.Value{"foo": {ValueType: &pb.Value_StringValue{StringValue: "bar1"}}}, CreateTime: tsNow, UpdateTime: tsNow},
				{Name: "projects/test-project/databases/test-db/documents/col/doc2", Fields: map[string]*pb.Value{"foo": {ValueType: &pb.Value_StringValue{StringValue: "bar2"}}}, CreateTime: tsNow, UpdateTime: tsNow},
			},
			ExecutionTime: tsNow,
		},
		{ // Second response with one result
			Results: []*pb.Document{
				{Name: "projects/test-project/databases/test-db/documents/col/doc3", Fields: map[string]*pb.Value{"foo": {ValueType: &pb.Value_StringValue{StringValue: "bar3"}}}, CreateTime: tsNow, UpdateTime: tsNow},
			},
			ExecutionTime: ts2MinLater,
		},
	}

	tests := []struct {
		name      string
		responses []*pb.ExecutePipelineResponse
		errors    []error
		gotCount  int
		wantErr   error
		wantData  []map[string]interface{}
	}{
		{
			name:      "successful iteration",
			responses: mockResponses,
			errors:    []error{nil, nil, io.EOF}, // EOF after 2 responses (containing 3 docs)
			gotCount:  3,
			wantErr:   iterator.Done,
			wantData: []map[string]interface{}{
				{"foo": "bar1"},
				{"foo": "bar2"},
				{"foo": "bar3"},
			},
		},
		{
			name:      "iteration with gRPC error",
			responses: []*pb.ExecutePipelineResponse{mockResponses[0]}, // Only first response
			errors:    []error{nil, status.Error(codes.Unavailable, "service unavailable")},
			gotCount:  2, // Expect results from the first response before error
			wantErr:   status.Error(codes.Unavailable, "service unavailable"),
			wantData: []map[string]interface{}{
				{"foo": "bar1"},
				{"foo": "bar2"},
			},
		},
		{
			name:      "no results",
			responses: []*pb.ExecutePipelineResponse{{Results: []*pb.Document{}}},
			errors:    []error{io.EOF},
			gotCount:  0,
			wantErr:   iterator.Done,
		},
		{
			name:      "partial progress then results",
			responses: []*pb.ExecutePipelineResponse{{ExecutionTime: tsNow /* No results */}, mockResponses[0]},
			errors:    []error{nil, nil, io.EOF},
			gotCount:  2,
			wantErr:   iterator.Done,
			wantData: []map[string]interface{}{
				{"foo": "bar1"},
				{"foo": "bar2"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockStreamClient := &mockExecutePipelineClient{
				RecvResponses: tc.responses,
				RecvErrors:    tc.errors,
				ContextVal:    ctx,
			}
			iter := &streamPipelineResultIterator{
				ctx:          ctx,
				cancel:       func() {},
				p:            p,
				streamClient: mockStreamClient,
			}
			defer iter.stop()
			var results []*PipelineResult
			var gotErr error
			var pr *PipelineResult
			for {
				pr, gotErr = iter.next()
				if gotErr != nil {
					break
				}
				results = append(results, pr)
			}

			if len(results) != tc.gotCount {
				t.Errorf("results got %d, want %d", len(results), tc.gotCount)
			}

			if tc.wantErr != nil {
				if gotErr == nil {
					t.Fatalf("error %v, got nil", tc.wantErr)
				}
				if !errors.Is(gotErr, tc.wantErr) && gotErr.Error() != tc.wantErr.Error() {
					t.Errorf("error got %v, want %v", gotErr, tc.wantErr)
				}
			} else if gotErr != nil {
				t.Errorf("error got %v, want %v", gotErr, nil)
			}

			if tc.wantData != nil {
				if len(results) != len(tc.wantData) {
					t.Fatalf("Result count mismatch for data check: expected %d, got %d", len(tc.wantData), len(results))
				}
				for i, pr := range results {
					if diff := cmp.Diff(tc.wantData[i], pr.Data()); diff != "" {
						t.Errorf("Data mismatch for result %d (-want +got):\n%s", i, diff)
					}
				}
			}
		})
	}
}

func TestPipelineResultIterator_Stop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := newTestClient()
	p := &Pipeline{c: client}

	mockStreamClient := &mockExecutePipelineClient{
		ContextVal: ctx, // Iterator will use this context
	}

	// Create the public iterator which wraps the stream iterator
	publicIter := &PipelineResultIterator{
		iter: &streamPipelineResultIterator{
			ctx:          ctx,    // This context is passed to the stream client
			cancel:       cancel, // This cancel func should be called by Stop
			p:            p,
			streamClient: mockStreamClient,
		},
	}

	publicIter.Stop()

	// Check if the context was cancelled
	select {
	case <-ctx.Done():
		// Expected: context is cancelled
	default:
		t.Errorf("Expected context to be cancelled after Stop(), but it was not")
	}

	// Calling Stop again should be a no-op
	publicIter.Stop() // Should not panic or error

	// Check that Next after Stop returns iterator.Done
	_, err := publicIter.Next()
	if !errors.Is(err, iterator.Done) {
		t.Errorf("Next after Stop(): got %v, want %v", err, iterator.Done)
	}
}

func TestPipelineResultIterator_GetAll(t *testing.T) {
	ctx := context.Background()
	client := newTestClient()
	p := &Pipeline{c: client}

	mockStreamClient := &mockExecutePipelineClient{
		RecvResponses: []*pb.ExecutePipelineResponse{
			{Results: []*pb.Document{
				{Name: "projects/p/databases/d/documents/c/doc1", Fields: map[string]*pb.Value{"id": {ValueType: &pb.Value_IntegerValue{IntegerValue: 1}}}},
			}},
			{Results: []*pb.Document{
				{Name: "projects/p/databases/d/documents/c/doc2", Fields: map[string]*pb.Value{"id": {ValueType: &pb.Value_IntegerValue{IntegerValue: 2}}}},
			}},
		},
		RecvErrors: []error{nil, nil, io.EOF}, // EOF after two responses
		ContextVal: ctx,
	}

	publicIter := &PipelineResultIterator{
		iter: &streamPipelineResultIterator{
			ctx:          ctx,
			cancel:       func() {},
			p:            p,
			streamClient: mockStreamClient,
		},
	}

	allResults, err := publicIter.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(allResults) != 2 {
		t.Errorf("results from GetAll(): got %d, want: 2", len(allResults))
	}
	if allResults[0].Data()["id"].(int64) != 1 {
		t.Errorf("first result id: got %v, want: 1", allResults[0].Data()["id"])
	}
	if allResults[1].Data()["id"].(int64) != 2 {
		t.Errorf("second result id: got %v, want: 2", allResults[1].Data()["id"])
	}

	// After GetAll, Next should return iterator.Done
	_, nextErr := publicIter.Next()
	if !errors.Is(nextErr, iterator.Done) {
		t.Errorf("Next after GetAll(): got %v, want: %v", nextErr, iterator.Done)
	}
}

func TestPipelineResult_DataExtraction(t *testing.T) {
	client := newTestClient()
	now := time.Now()
	tsNowProto := timestamppb.New(now)

	docProto := &pb.Document{
		Name:       "projects/test/databases/d/documents/mycoll/mydoc",
		CreateTime: tsNowProto,
		UpdateTime: tsNowProto,
		Fields: map[string]*pb.Value{
			"stringProp": {ValueType: &pb.Value_StringValue{StringValue: "hello"}},
			"intProp":    {ValueType: &pb.Value_IntegerValue{IntegerValue: 123}},
			"boolProp":   {ValueType: &pb.Value_BooleanValue{BooleanValue: true}},
			"mapProp": {ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{
				Fields: map[string]*pb.Value{
					"nestedString": {ValueType: &pb.Value_StringValue{StringValue: "world"}},
				},
			}}},
		},
	}
	execTimeProto := timestamppb.New(now.Add(time.Second))

	docRef, _ := pathToDoc(docProto.Name, client)
	pr, err := newPipelineResult(docRef, docProto, client, execTimeProto)
	if err != nil {
		t.Fatalf("newPipelineResult: %v", err)
	}

	if !pr.Exists() {
		t.Error("pr.Exists: got false, want true")
	}

	// Test Data()
	dataMap := pr.Data()
	if dataMap["stringProp"].(string) != "hello" {
		t.Errorf("stringProp: got %v, want 'hello'", dataMap["stringProp"])
	}

	if dataMap["intProp"].(int64) != 123 {
		t.Errorf("intProp: got %v, want 123", dataMap["intProp"])
	}
	nestedMap, ok := dataMap["mapProp"].(map[string]interface{})
	if !ok {
		t.Fatalf("mapProp is not a map[string]interface{}")
	}
	if nestedMap["nestedString"].(string) != "world" {
		t.Errorf("nestedString: got %v, want 'world'", nestedMap["nestedString"])
	}

	// Test DataTo() with a struct
	type MyStruct struct {
		StringProp  string                 `firestore:"stringProp"`
		IntProp     int                    `firestore:"intProp"`
		BoolProp    bool                   `firestore:"boolProp"`
		MapProp     map[string]interface{} `firestore:"mapProp"`
		NonExistent float64                `firestore:"nonExistent"`
	}
	gotDst := MyStruct{
		StringProp:  "world",
		IntProp:     456,
		BoolProp:    false,
		MapProp:     map[string]interface{}{"nestedString": "hello"},
		NonExistent: 456.789,
	}

	wantDst := MyStruct{
		StringProp:  "hello",
		IntProp:     123,
		BoolProp:    true,
		MapProp:     map[string]interface{}{"nestedString": "world"},
		NonExistent: 456.789,
	}

	if err := pr.DataTo(&gotDst); err != nil {
		t.Fatalf("pr.DataTo(&gotDst): %v", err)
	}

	if diff := testutil.Diff(wantDst, gotDst); diff != "" {
		t.Errorf("dst mismatch (-want +got):\n%s", diff)
	}

	// Test Timestamps
	if pr.CreateTime == nil || !pr.CreateTime.Equal(now) {
		t.Errorf("CreateTime: got %v, want %v", pr.CreateTime, now)
	}
	if pr.ExecutionTime == nil || !pr.ExecutionTime.Equal(now.Add(time.Second)) {
		t.Errorf("ExecutionTime: got %v, want %v", pr.ExecutionTime, now.Add(time.Second))
	}
}

func TestPipelineResult_NoResults(t *testing.T) {
	client := newTestClient()
	execTime := time.Now()
	execTimeProto := timestamppb.New(execTime)

	pr, err := newPipelineResult(nil, nil, client, execTimeProto) // No proto document
	if err != nil {
		t.Fatalf("newPipelineResult: %v", err)
	}

	if pr.Exists() {
		t.Error("pr.Exists() for non-existent result: got true, want false")
	}
	if data := pr.Data(); data != nil {
		t.Errorf("pr.Data() for non-existent result: got %v, want nil", data)
	}

	type MyStruct struct{ Foo string }
	var s MyStruct
	err = pr.DataTo(&s) // Should behave like populating from an empty map
	if err != nil {
		// DataTo on a non-existent PipelineResult should not error out but result in a zero-valued struct.
		t.Fatalf("pr.DataTo(&s) on non-existent result failed: %v", err)
	}
	if s.Foo != "" {
		t.Errorf("Struct Foo for non-existent result: got %q, want \"\"", s.Foo)
	}

	if pr.ExecutionTime == nil || !pr.ExecutionTime.Equal(execTime) {
		t.Errorf("ExecutionTime for non-existent result: got %v, want %v", pr.ExecutionTime, execTime)
	}
}
