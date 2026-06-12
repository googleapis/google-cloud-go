package adapters

import (
	v2pb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"testing"
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
	if res != "projects/p1/instances/i1/tables/t1" {
		t.Errorf("expected resource projects/p1/instances/i1/tables/t1, got %s", res)
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
