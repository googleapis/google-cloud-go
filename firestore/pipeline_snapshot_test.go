// Copyright 2026 Google LLC
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
	"testing"
	"time"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestPipeline_Execute_ExecutionTime(t *testing.T) {
	ctx := context.Background()
	srv, cleanup, err := newMockServer()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	client, err := NewClient(ctx, "project", option.WithEndpoint(srv.Addr), option.WithoutAuthentication(), option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Prepare pipeline
	p := client.Pipeline().Collection("C").Limit(1)

	// Prepare mock response
	execTime := time.Date(2023, 10, 26, 12, 0, 0, 0, time.UTC)
	ts := timestamppb.New(execTime)

	// We expect one request
	req, err := p.toExecutePipelineRequest()
	if err != nil {
		t.Fatal(err)
	}
	req.Database = "projects/project/databases/(default)"

	srv.addRPC(req, []interface{}{
		&pb.ExecutePipelineResponse{
			Results: []*pb.Document{
				{
					Name: "projects/project/databases/(default)/documents/C/doc1",
					Fields: map[string]*pb.Value{
						"a": {ValueType: &pb.Value_IntegerValue{IntegerValue: 1}},
					},
				},
			},
			ExecutionTime: ts,
		},
	})

	snap := p.Execute(ctx)
	iter := snap.Results()

	// Verify ExecutionTime fails before iteration done
	if _, err := snap.ExecutionTime(); err != errExecutionTimeBeforeEnd {
		t.Errorf("ExecutionTime() error before end: got %v, want %v", err, errExecutionTimeBeforeEnd)
	}

	// Iterate
	doc, err := iter.Next()
	if err != nil {
		t.Fatalf("iter.Next() failed: %v", err)
	}
	if doc == nil {
		t.Fatal("doc is nil")
	}

	_, err = iter.Next()
	if err != iterator.Done {
		t.Errorf("Expected iterator.Done, got %v", err)
	}

	// Verify ExecutionTime
	gotTime, err := snap.ExecutionTime()
	if err != nil {
		t.Errorf("ExecutionTime() failed: %v", err)
	}
	if gotTime == nil {
		t.Fatal("ExecutionTime() returned nil")
	}
	if !gotTime.Equal(execTime) {
		t.Errorf("ExecutionTime(): got %v, want %v", gotTime, execTime)
	}
}

func TestPipeline_Execute_ExecutionTime_Updated(t *testing.T) {
	// Test that the last received ExecutionTime is used
	ctx := context.Background()
	srv, cleanup, err := newMockServer()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	client, err := NewClient(ctx, "project", option.WithEndpoint(srv.Addr), option.WithoutAuthentication(), option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	p := client.Pipeline().Collection("C").Limit(2)

	ts1 := timestamppb.New(time.Date(2023, 10, 26, 12, 0, 0, 0, time.UTC))
	execTime2 := time.Date(2023, 10, 26, 12, 0, 1, 0, time.UTC)
	ts2 := timestamppb.New(execTime2)

	req, err := p.toExecutePipelineRequest()
	if err != nil {
		t.Fatal(err)
	}
	req.Database = "projects/project/databases/(default)"

	srv.addRPC(req, []interface{}{
		&pb.ExecutePipelineResponse{
			Results: []*pb.Document{
				{Name: "projects/project/databases/(default)/documents/C/doc1"},
			},
			ExecutionTime: ts1,
		},
		&pb.ExecutePipelineResponse{
			Results: []*pb.Document{
				{Name: "projects/project/databases/(default)/documents/C/doc2"},
			},
			ExecutionTime: ts2,
		},
	})

	snap := p.Execute(ctx)
	iter := snap.Results()

	iter.GetAll() // Consume all

	gotTime, err := snap.ExecutionTime()
	if err != nil {
		t.Errorf("ExecutionTime() failed: %v", err)
	}
	if gotTime == nil {
		t.Fatal("ExecutionTime() returned nil")
	}
	if !gotTime.Equal(execTime2) {
		t.Errorf("ExecutionTime(): got %v, want %v", gotTime, execTime2)
	}
}
