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
	"testing"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"cloud.google.com/go/internal/testutil"
)

func TestPipelineSource_Collection(t *testing.T) {
	client := newTestClient()
	ps := &PipelineSource{client: client}
	p := ps.Collection("users")

	if p.err != nil {
		t.Fatalf("Collection: %v", p.err)
	}
	if len(p.stages) != 1 {
		t.Fatalf("initial stages: got %d, want 1", len(p.stages))
	}

	req, err := p.toExecutePipelineRequest()
	if err != nil {
		t.Fatalf("toExecutePipelineRequest: %v", err)
	}

	wantStage := &pb.Pipeline_Stage{
		Name: "collection",
		Args: []*pb.Value{{ValueType: &pb.Value_ReferenceValue{ReferenceValue: "/users"}}},
	}

	if len(req.GetStructuredPipeline().GetPipeline().GetStages()) != 1 {
		t.Fatalf("stage in proto: got %d, want 1", len(req.GetStructuredPipeline().GetPipeline().GetStages()))
	}
	if diff := testutil.Diff(wantStage, req.GetStructuredPipeline().GetPipeline().GetStages()[0]); diff != "" {
		t.Errorf("toExecutePipelineRequest mismatch for collection stage (-want +got):\n%s", diff)
	}
}
