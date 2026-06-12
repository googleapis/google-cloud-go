package adapters

import (
	v2pb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"testing"
)

func TestReadRowRequestAdapter(t *testing.T) {
	reqAdapter := &ReadRowRequestAdapter{}
	v2Req := &v2pb.ReadRowsRequest{
		TableName: "projects/p1/instances/i1/tables/t1",
		Rows: &v2pb.RowSet{
			RowKeys: [][]byte{[]byte("test-key")},
		},
		Filter: &v2pb.RowFilter{
			Filter: &v2pb.RowFilter_FamilyNameRegexFilter{FamilyNameRegexFilter: "family-regex"},
		},
	}
	jsReq, err := reqAdapter.Adapt(v2Req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if jsReq == nil {
		t.Fatal("expected non-nil jsReq")
	}

	if string(jsReq.Key) != "test-key" {
		t.Errorf("expected Key test-key, got %s", string(jsReq.Key))
	}

	if jsReq.Filter.GetFamilyNameRegexFilter() != "family-regex" {
		t.Errorf("expected Filter family-regex, got %s", jsReq.Filter.GetFamilyNameRegexFilter())
	}

	res, err := reqAdapter.ExtractResource(v2Req)
	if err != nil {
		t.Fatalf("ExtractResource failed: %v", err)
	}
	if res != "projects/p1/instances/i1/tables/t1" {
		t.Errorf("expected resource projects/p1/instances/i1/tables/t1, got %s", res)
	}
}

func TestReadRowResponseAdapter(t *testing.T) {
	resAdapter := &ReadRowResponseAdapter{}
	jsRes := &v2pb.SessionReadRowResponse{}
	v2Res, err := resAdapter.Adapt(jsRes)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if v2Res == nil {
		t.Fatal("expected non-nil v2Res")
	}
}
