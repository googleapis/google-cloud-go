// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package firestore

import (
	"context"
	"io"
	"testing"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/testing/protocmp"
)

// mockExecutePipelineClient is a mock implementation of pb.Firestore_ExecutePipelineClient.
type mockExecutePipelineClient struct {
	pb.Firestore_ExecutePipelineClient // Embed for forward compatibility
	RecvResponses                      []*pb.ExecutePipelineResponse
	RecvErrors                         []error
	RecvIdx                            int
	CloseSendErr                       error
	HeaderVal                          metadata.MD
	TrailerVal                         metadata.MD
	ContextVal                         context.Context
	SendHeaderVal                      metadata.MD
}

func (m *mockExecutePipelineClient) Recv() (*pb.ExecutePipelineResponse, error) {
	if m.ContextVal != nil && m.ContextVal.Err() != nil {
		return nil, m.ContextVal.Err()
	}
	if m.RecvIdx < len(m.RecvResponses) || m.RecvIdx < len(m.RecvErrors) {
		var resp *pb.ExecutePipelineResponse
		var err error
		if m.RecvIdx < len(m.RecvResponses) {
			resp = m.RecvResponses[m.RecvIdx]
		}
		if m.RecvIdx < len(m.RecvErrors) {
			err = m.RecvErrors[m.RecvIdx]
		}
		m.RecvIdx++
		return resp, err
	}
	return nil, io.EOF
}
func (m *mockExecutePipelineClient) CloseSend() error             { return m.CloseSendErr }
func (m *mockExecutePipelineClient) Header() (metadata.MD, error) { return m.HeaderVal, nil }
func (m *mockExecutePipelineClient) Trailer() metadata.MD         { return m.TrailerVal }
func (m *mockExecutePipelineClient) Context() context.Context     { return m.ContextVal }
func (m *mockExecutePipelineClient) SendHeader(md metadata.MD) error {
	m.SendHeaderVal = md
	return nil
}
func (m *mockExecutePipelineClient) SetHeader(md metadata.MD) error { return nil }
func (m *mockExecutePipelineClient) SetTrailer(md metadata.MD)      {}
func (m *mockExecutePipelineClient) SendMsg(i any) error            { return nil }
func (m *mockExecutePipelineClient) RecvMsg(i any) error            { return nil }

// Test helper to create a minimal Client for non-RPC tests
func newTestClient() *Client {
	return &Client{
		projectID:  "test-project",
		databaseID: "test-db",
	}
}

func TestPipeline_Limit(t *testing.T) {
	client := newTestClient()
	ps := &PipelineSource{client: client}
	p := ps.Collection("users").Limit(10)

	if p.err != nil {
		t.Fatalf("Pipeline.Limit() returned error: %v", p.err)
	}
	if len(p.stages) != 2 {
		t.Fatalf("Expected 2 stages, got %d", len(p.stages))
	}

	req, err := p.toExecutePipelineRequest()
	if err != nil {
		t.Fatalf("p.toExecutePipelineRequest() failed: %v", err)
	}

	stages := req.GetStructuredPipeline().GetPipeline().GetStages()
	if len(stages) != 2 {
		t.Fatalf("Expected 2 stages in proto, got %d", len(stages))
	}

	wantLimitStage := &pb.Pipeline_Stage{
		Name: "limit",
		Args: []*pb.Value{{ValueType: &pb.Value_IntegerValue{IntegerValue: 10}}},
	}
	if diff := cmp.Diff(wantLimitStage, stages[1], protocmp.Transform()); diff != "" {
		t.Errorf("toExecutePipelineRequest() mismatch for limit stage (-want +got):\n%s", diff)
	}
}

func TestPipeline_ToExecutePipelineRequest(t *testing.T) {
	client := newTestClient()
	ps := &PipelineSource{client: client}
	p := ps.Collection("items").Limit(5)

	req, err := p.toExecutePipelineRequest()
	if err != nil {
		t.Fatalf("toExecutePipelineRequest: %v", err)
	}

	if req.GetDatabase() != "projects/test-project/databases/test-db" {
		t.Errorf("req.GetDatabase: got %s, want %s", req.GetDatabase(), "projects/test-project/databases/test-db")
	}

	pipelineProto := req.GetStructuredPipeline().GetPipeline()
	if pipelineProto == nil {
		t.Fatal("StructuredPipeline.Pipeline is nil")
	}

	stagesProto := pipelineProto.GetStages()
	if len(stagesProto) != 2 {
		t.Fatalf("stages: got %d want 2", len(stagesProto))
	}

	// Check collection stage
	wantCollStage := &pb.Pipeline_Stage{
		Name: "collection",
		Args: []*pb.Value{{ValueType: &pb.Value_ReferenceValue{ReferenceValue: "/items"}}},
	}
	if diff := cmp.Diff(wantCollStage, stagesProto[0], protocmp.Transform()); diff != "" {
		t.Errorf("Collection stage mismatch (-want +got):\n%s", diff)
	}

	// Check limit stage
	wantLimitStage := &pb.Pipeline_Stage{
		Name: "limit",
		Args: []*pb.Value{{ValueType: &pb.Value_IntegerValue{IntegerValue: 5}}},
	}
	if diff := cmp.Diff(wantLimitStage, stagesProto[1], protocmp.Transform()); diff != "" {
		t.Errorf("Limit stage mismatch (-want +got):\n%s", diff)
	}
}

func TestPipeline_Sort(t *testing.T) {
	client := newTestClient()
	ps := &PipelineSource{client: client}
	p := ps.Collection("users").Sort(Ordering{Expr: FieldOf("age"), Direction: OrderingDesc})

	req, err := p.toExecutePipelineRequest()
	if err != nil {
		t.Fatalf("p.toExecutePipelineRequest() failed: %v", err)
	}

	stages := req.GetStructuredPipeline().GetPipeline().GetStages()
	if len(stages) != 2 {
		t.Fatalf("Expected 2 stages in proto, got %d", len(stages))
	}

	wantSortStage := &pb.Pipeline_Stage{
		Name: "sort",
		Args: []*pb.Value{
			{
				ValueType: &pb.Value_MapValue{
					MapValue: &pb.MapValue{
						Fields: map[string]*pb.Value{
							"expression": {ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "age"}},
							"direction":  {ValueType: &pb.Value_StringValue{StringValue: "descending"}},
						},
					},
				},
			},
		},
	}
	if diff := cmp.Diff(wantSortStage, stages[1], protocmp.Transform()); diff != "" {
		t.Errorf("toExecutePipelineRequest() mismatch for sort stage (-want +got):\n%s", diff)
	}
}

func TestPipeline_Offset(t *testing.T) {
	client := newTestClient()
	ps := &PipelineSource{client: client}
	p := ps.Collection("users").Offset(20)

	req, err := p.toExecutePipelineRequest()
	if err != nil {
		t.Fatalf("p.toExecutePipelineRequest() failed: %v", err)
	}

	stages := req.GetStructuredPipeline().GetPipeline().GetStages()
	if len(stages) != 2 {
		t.Fatalf("Expected 2 stages in proto, got %d", len(stages))
	}

	wantOffsetStage := &pb.Pipeline_Stage{
		Name: "offset",
		Args: []*pb.Value{{ValueType: &pb.Value_IntegerValue{IntegerValue: 20}}},
	}
	if diff := cmp.Diff(wantOffsetStage, stages[1], protocmp.Transform()); diff != "" {
		t.Errorf("toExecutePipelineRequest() mismatch for offset stage (-want +got):\n%s", diff)
	}
}

func TestPipeline_Select(t *testing.T) {
	client := newTestClient()
	ps := &PipelineSource{client: client}
	p := ps.Collection("users").Select("name", FieldOf("age"), Add(FieldOf("score"), 10).As("new_score"))

	req, err := p.toExecutePipelineRequest()
	if err != nil {
		t.Fatalf("p.toExecutePipelineRequest() failed: %v", err)
	}

	stages := req.GetStructuredPipeline().GetPipeline().GetStages()
	if len(stages) != 2 {
		t.Fatalf("Expected 2 stages in proto, got %d", len(stages))
	}

	wantSelectStage := &pb.Pipeline_Stage{
		Name: "select",
		Args: []*pb.Value{
			{ValueType: &pb.Value_MapValue{
				MapValue: &pb.MapValue{
					Fields: map[string]*pb.Value{
						"name": {ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "name"}},
						"age":  {ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "age"}},
						"new_score": {ValueType: &pb.Value_FunctionValue{FunctionValue: &pb.Function{
							Name: "add",
							Args: []*pb.Value{
								{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "score"}},
								{ValueType: &pb.Value_IntegerValue{IntegerValue: 10}},
							},
						}}},
					},
				},
			}},
		},
	}
	if diff := cmp.Diff(wantSelectStage, stages[1], protocmp.Transform()); diff != "" {
		t.Errorf("toExecutePipelineRequest() mismatch for select stage (-want +got):\n%s", diff)
	}
}

func TestPipeline_AddFields(t *testing.T) {
	client := newTestClient()
	ps := &PipelineSource{client: client}
	p := ps.Collection("users").AddFields(Add(FieldOf("score"), 10).As("new_score"))

	req, err := p.toExecutePipelineRequest()
	if err != nil {
		t.Fatalf("p.toExecutePipelineRequest() failed: %v", err)
	}

	stages := req.GetStructuredPipeline().GetPipeline().GetStages()
	if len(stages) != 2 {
		t.Fatalf("Expected 2 stages in proto, got %d", len(stages))
	}

	wantAddFieldsStage := &pb.Pipeline_Stage{
		Name: "add_fields",
		Args: []*pb.Value{
			{ValueType: &pb.Value_MapValue{
				MapValue: &pb.MapValue{
					Fields: map[string]*pb.Value{
						"new_score": {ValueType: &pb.Value_FunctionValue{FunctionValue: &pb.Function{
							Name: "add",
							Args: []*pb.Value{
								{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "score"}},
								{ValueType: &pb.Value_IntegerValue{IntegerValue: 10}},
							},
						}}},
					},
				},
			}},
		},
	}
	if diff := cmp.Diff(wantAddFieldsStage, stages[1], protocmp.Transform()); diff != "" {
		t.Errorf("toExecutePipelineRequest() mismatch for addFields stage (-want +got):\n%s", diff)
	}
}

func TestPipeline_Where(t *testing.T) {
	client := newTestClient()
	ps := &PipelineSource{client: client}
	p := ps.Collection("users").Where(Equal(FieldOf("age"), 30))

	req, err := p.toExecutePipelineRequest()
	if err != nil {
		t.Fatalf("p.toExecutePipelineRequest() failed: %v", err)
	}

	stages := req.GetStructuredPipeline().GetPipeline().GetStages()
	if len(stages) != 2 {
		t.Fatalf("Expected 2 stages in proto, got %d", len(stages))
	}

	wantWhereStage := &pb.Pipeline_Stage{
		Name: "where",
		Args: []*pb.Value{
			{ValueType: &pb.Value_FunctionValue{FunctionValue: &pb.Function{
				Name: "equal",
				Args: []*pb.Value{
					{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "age"}},
					{ValueType: &pb.Value_IntegerValue{IntegerValue: 30}},
				},
			}}},
		},
	}
	if diff := cmp.Diff(wantWhereStage, stages[1], protocmp.Transform()); diff != "" {
		t.Errorf("toExecutePipelineRequest() mismatch for where stage (-want +got):\n%s", diff)
	}
}

func TestPipeline_Aggregate(t *testing.T) {
	client := newTestClient()
	ps := &PipelineSource{client: client}
	p := ps.Collection("users").Aggregate(Sum("age").As("total_age"))

	req, err := p.toExecutePipelineRequest()
	if err != nil {
		t.Fatalf("p.toExecutePipelineRequest() failed: %v", err)
	}

	stages := req.GetStructuredPipeline().GetPipeline().GetStages()
	if len(stages) != 2 {
		t.Fatalf("Expected 2 stages in proto, got %d", len(stages))
	}

	wantAggregateStage := &pb.Pipeline_Stage{
		Name: "aggregate",
		Args: []*pb.Value{
			{ValueType: &pb.Value_MapValue{
				MapValue: &pb.MapValue{
					Fields: map[string]*pb.Value{
						"total_age": {ValueType: &pb.Value_FunctionValue{FunctionValue: &pb.Function{
							Name: "sum",
							Args: []*pb.Value{
								{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "age"}},
							},
						}}},
					},
				},
			}},
			{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{}}},
		},
	}
	if diff := cmp.Diff(wantAggregateStage, stages[1], protocmp.Transform()); diff != "" {
		t.Errorf("toExecutePipelineRequest() mismatch for aggregate stage (-want +got):\n%s", diff)
	}
}

func TestPipeline_AggregateWithSpec(t *testing.T) {
	client := newTestClient()
	ps := &PipelineSource{client: client}
	spec := NewAggregateSpec(Average("rating").As("avg_rating")).WithGroups("genre")
	p := ps.Collection("books").AggregateWithSpec(spec)

	req, err := p.toExecutePipelineRequest()
	if err != nil {
		t.Fatalf("p.toExecutePipelineRequest() failed: %v", err)
	}

	stages := req.GetStructuredPipeline().GetPipeline().GetStages()
	if len(stages) != 2 {
		t.Fatalf("Expected 2 stages in proto, got %d", len(stages))
	}

	wantAggregateStage := &pb.Pipeline_Stage{
		Name: "aggregate",
		Args: []*pb.Value{
			{ValueType: &pb.Value_MapValue{
				MapValue: &pb.MapValue{
					Fields: map[string]*pb.Value{
						"avg_rating": {ValueType: &pb.Value_FunctionValue{FunctionValue: &pb.Function{
							Name: "average",
							Args: []*pb.Value{
								{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "rating"}},
							},
						}}},
					},
				},
			}},
			{ValueType: &pb.Value_MapValue{
				MapValue: &pb.MapValue{
					Fields: map[string]*pb.Value{
						"genre": {ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "genre"}},
					},
				},
			}},
		},
	}
	if diff := cmp.Diff(wantAggregateStage, stages[1], protocmp.Transform()); diff != "" {
		t.Errorf("toExecutePipelineRequest() mismatch for aggregate stage (-want +got):\n%s", diff)
	}
}
