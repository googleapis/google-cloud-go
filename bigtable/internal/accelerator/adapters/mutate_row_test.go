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

package adapters

import (
	"testing"

	v2pb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMutateRowRequestAdapter(t *testing.T) {
	reqAdapter := &MutateRowRequestAdapter{}
	v2Req := &v2pb.MutateRowRequest{
		TableName: "projects/p1/instances/i1/tables/t1",
		RowKey:    []byte("row-key"),
		Mutations: []*v2pb.Mutation{
			{
				Mutation: &v2pb.Mutation_SetCell_{
					SetCell: &v2pb.Mutation_SetCell{
						FamilyName:      "fam",
						ColumnQualifier: []byte("qual"),
						Value:           []byte("val"),
						TimestampMicros: 1000,
					},
				},
			},
		},
	}

	jsReq, err := reqAdapter.Adapt(v2Req)
	if err != nil {
		t.Fatalf("Adapt failed: %v", err)
	}

	if string(jsReq.Key) != "row-key" {
		t.Errorf("expected key 'row-key', got %s", string(jsReq.Key))
	}

	if len(jsReq.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %d", len(jsReq.Mutations))
	}

	setCell := jsReq.Mutations[0].GetSetCell()
	if setCell == nil {
		t.Fatal("expected SetCell mutation")
	}

	if setCell.FamilyName != "fam" || string(setCell.ColumnQualifier) != "qual" || string(setCell.Value) != "val" {
		t.Errorf("unexpected set cell content: %+v", setCell)
	}

	res, err := reqAdapter.ExtractResource(v2Req)
	if err != nil {
		t.Fatalf("ExtractResource failed: %v", err)
	}
	if res.Kind != ResourceTable || res.Name != "projects/p1/instances/i1/tables/t1" {
		t.Errorf("ExtractResource = %+v; want {ResourceTable, projects/p1/instances/i1/tables/t1}", res)
	}
}

func TestMutateRowRequestAdapter_ExtractResource(t *testing.T) {
	reqAdapter := &MutateRowRequestAdapter{}
	cases := []struct {
		name string
		req  *v2pb.MutateRowRequest
		want Resource
	}{
		{
			"table",
			&v2pb.MutateRowRequest{TableName: "projects/p/instances/i/tables/t"},
			Resource{Kind: ResourceTable, Name: "projects/p/instances/i/tables/t"},
		},
		{
			"authorized-view",
			&v2pb.MutateRowRequest{AuthorizedViewName: "projects/p/instances/i/tables/t/authorizedViews/v"},
			Resource{Kind: ResourceAuthorizedView, Name: "projects/p/instances/i/tables/t/authorizedViews/v"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := reqAdapter.ExtractResource(tc.req)
			if err != nil {
				t.Fatalf("ExtractResource: %v", err)
			}
			if got != tc.want {
				t.Errorf("ExtractResource = %+v; want %+v", got, tc.want)
			}
		})
	}
}

func TestMutateRowRequestAdapter_ExtractResource_Empty(t *testing.T) {
	reqAdapter := &MutateRowRequestAdapter{}
	_, err := reqAdapter.ExtractResource(&v2pb.MutateRowRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("ExtractResource(empty) code = %v; want InvalidArgument", status.Code(err))
	}
}

func TestMutateRowResponseAdapter(t *testing.T) {
	resAdapter := &MutateRowResponseAdapter{}
	jsRes := &v2pb.SessionMutateRowResponse{}
	v2Res, err := resAdapter.Adapt(jsRes)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if v2Res == nil {
		t.Fatal("expected non-nil v2Res")
	}
}
