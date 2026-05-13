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

package datastore

import (
	"context"
	"testing"

	pb "cloud.google.com/go/datastore/apiv1/datastorepb"
	"cloud.google.com/go/internal/testutil"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTransactionTracingContextPropagation(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})

	client, srv, cleanup := newMock(t)
	defer cleanup()

	mockTxnID := []byte("tid")
	mockKey := NameKey("Kind", "name", nil)
	mockEntity := &pb.Entity{
		Key: keyToProto(mockKey),
	}

	srv.addRPC(&pb.BeginTransactionRequest{ProjectId: mockProjectID}, &pb.BeginTransactionResponse{Transaction: mockTxnID})
	srv.addRPC(&pb.LookupRequest{
		ProjectId: mockProjectID,
		Keys:      []*pb.Key{keyToProto(mockKey)},
		ReadOptions: &pb.ReadOptions{
			ConsistencyType: &pb.ReadOptions_Transaction{Transaction: mockTxnID},
		},
	}, &pb.LookupResponse{Found: []*pb.EntityResult{{Entity: mockEntity}}})
	srv.addRPC(&pb.LookupRequest{
		ProjectId: mockProjectID,
		Keys:      []*pb.Key{keyToProto(mockKey)},
		ReadOptions: &pb.ReadOptions{
			ConsistencyType: &pb.ReadOptions_Transaction{Transaction: mockTxnID},
		},
	}, &pb.LookupResponse{Found: []*pb.EntityResult{{Entity: mockEntity}}})
	srv.addRPC(&pb.CommitRequest{
		ProjectId:           mockProjectID,
		TransactionSelector: &pb.CommitRequest_Transaction{Transaction: mockTxnID},
		Mode:                pb.CommitRequest_TRANSACTIONAL,
	}, &pb.CommitResponse{})

	tx, err := client.NewTransaction(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var dst struct{}
	if err := tx.Get(mockKey, &dst); err != nil {
		t.Fatal(err)
	}
	if err := tx.Get(mockKey, &dst); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	spans := te.Spans()

	var newTxnSpan, beginTxnSpan tracetest.SpanStub
	var getSpans []tracetest.SpanStub
	var commitSpan tracetest.SpanStub
	var hasNewTxn, hasBeginTxn, hasCommit bool

	for _, s := range spans {
		switch s.Name {
		case "cloud.google.com/go/datastore.NewTransaction":
			newTxnSpan = s
			hasNewTxn = true
		case "cloud.google.com/go/datastore.Transaction.BeginTransaction":
			beginTxnSpan = s
			hasBeginTxn = true
		case "cloud.google.com/go/datastore.Transaction.Get":
			getSpans = append(getSpans, s)
		case "cloud.google.com/go/datastore.Transaction.Commit":
			commitSpan = s
			hasCommit = true
		}
	}

	if !hasNewTxn {
		t.Fatal("missing NewTransaction span")
	}
	newTxnSpanID := newTxnSpan.SpanContext.SpanID()

	if !hasBeginTxn {
		t.Fatal("missing BeginTransaction span")
	}
	if beginTxnSpan.Parent.SpanID() != newTxnSpanID {
		t.Errorf("BeginTransaction span parent = %v, want %v", beginTxnSpan.Parent.SpanID(), newTxnSpanID)
	}

	if len(getSpans) != 2 {
		t.Fatalf("got %d Get spans, want 2", len(getSpans))
	}

	for i, s := range getSpans {
		if s.Parent.SpanID() != newTxnSpanID {
			t.Errorf("Get span %d parent = %v, want %v (parent span: %v)", i, s.Parent.SpanID(), newTxnSpanID, s.Parent)
		}
	}

	if !hasCommit {
		t.Fatal("missing Commit span")
	}
	if commitSpan.Parent.SpanID() != newTxnSpanID {
		t.Errorf("Commit span parent = %v, want %v", commitSpan.Parent.SpanID(), newTxnSpanID)
	}
}

func TestIteratorTracingContextPropagation(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})

	client, srv, cleanup := newMock(t)
	defer cleanup()

	mockKind := "Kind"
	srv.addRPC(&pb.RunQueryRequest{
		ProjectId: mockProjectID,
		QueryType: &pb.RunQueryRequest_Query{Query: &pb.Query{Kind: []*pb.KindExpression{{Name: mockKind}}}},
	}, &pb.RunQueryResponse{})

	q := NewQuery(mockKind)
	it := client.Run(ctx, q)

	_, _ = it.Cursor()
	_, _ = it.Cursor()

	spans := te.Spans()

	var runSpan tracetest.SpanStub
	var cursorSpans []tracetest.SpanStub
	var hasRun bool

	for _, s := range spans {
		switch s.Name {
		case "cloud.google.com/go/datastore.Query.Run":
			runSpan = s
			hasRun = true
		case "cloud.google.com/go/datastore.Query.Cursor":
			cursorSpans = append(cursorSpans, s)
		}
	}

	if !hasRun {
		t.Fatal("missing Query.Run span")
	}
	runSpanID := runSpan.SpanContext.SpanID()

	if len(cursorSpans) != 2 {
		t.Fatalf("got %d Cursor spans, want 2", len(cursorSpans))
	}

	for i, s := range cursorSpans {
		if s.Parent.SpanID() != runSpanID {
			t.Errorf("Cursor span %d parent = %v, want %v", i, s.Parent.SpanID(), runSpanID)
		}
	}
}
