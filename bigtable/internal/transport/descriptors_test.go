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

package internal

import (
	"bytes"
	"testing"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/protobuf/proto"
)

func TestVRpcDescriptor_Method(t *testing.T) {
	if READ_ROW.Method() != "ReadRow" {
		t.Errorf("READ_ROW.Method() = %q, want %q", READ_ROW.Method(), "ReadRow")
	}
	if MUTATE_ROW.Method() != "MutateRow" {
		t.Errorf("MUTATE_ROW.Method() = %q, want %q", MUTATE_ROW.Method(), "MutateRow")
	}
	if CHECK_AND_MUTATE_ROW.Method() != "CheckAndMutateRow" {
		t.Errorf("CHECK_AND_MUTATE_ROW.Method() = %q, want %q", CHECK_AND_MUTATE_ROW.Method(), "CheckAndMutateRow")
	}
	if READ_MODIFY_WRITE_ROW.Method() != "ReadModifyWriteRow" {
		t.Errorf("READ_MODIFY_WRITE_ROW.Method() = %q, want %q", READ_MODIFY_WRITE_ROW.Method(), "ReadModifyWriteRow")
	}
}

func TestSessionType_Envelope(t *testing.T) {
	cases := []struct {
		st      SessionType
		want    ResourceEnvelope
		wantErr bool
	}{
		{SessionTypeTable, TableEnvelope, false},
		{SessionTypeAuthorizedView, AuthorizedViewEnvelope, false},
		{SessionTypeMaterializedView, MaterializedViewEnvelope, false},
		{SessionType(999), nil, true},
	}
	for _, tc := range cases {
		got, err := tc.st.Envelope()
		if (err != nil) != tc.wantErr {
			t.Errorf("SessionType(%v).Envelope() err = %v, wantErr %v", tc.st, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("SessionType(%v).Envelope() = %T, want %T", tc.st, got, tc.want)
		}
	}
}

func TestTableEnvelope_ReadRow(t *testing.T) {
	req := &btpb.SessionReadRowRequest{
		Key: []byte("row-key-1"),
		Filter: &btpb.RowFilter{
			Filter: &btpb.RowFilter_CellsPerRowLimitFilter{CellsPerRowLimitFilter: 1},
		},
	}

	encodedBytes, err := READ_ROW.Encode(TableEnvelope, req)
	if err != nil {
		t.Fatalf("Failed to encode READ_ROW: %v", err)
	}

	var env btpb.TableRequest
	if err := proto.Unmarshal(encodedBytes, &env); err != nil {
		t.Fatalf("Failed to unmarshal TableRequest envelope: %v", err)
	}

	payload, ok := env.Payload.(*btpb.TableRequest_ReadRow)
	if !ok {
		t.Fatalf("Expected TableRequest_ReadRow payload, got %T", env.Payload)
	}

	readRowReq := payload.ReadRow
	if !bytes.Equal(readRowReq.Key, []byte("row-key-1")) {
		t.Errorf("Expected key %q, got %q", "row-key-1", string(readRowReq.Key))
	}
	if readRowReq.Filter.GetCellsPerRowLimitFilter() != 1 {
		t.Errorf("Expected CellsPerRowLimitFilter = 1, got %v", readRowReq.Filter)
	}

	respEnvelope := &btpb.TableResponse{
		Payload: &btpb.TableResponse_ReadRow{
			ReadRow: &btpb.SessionReadRowResponse{
				Row: &btpb.Row{
					Key:      []byte("row-key-1"),
					Families: []*btpb.Family{{Name: "cf1"}},
				},
			},
		},
	}
	envelopeBytes, err := proto.Marshal(respEnvelope)
	if err != nil {
		t.Fatalf("Failed to marshal TableResponse envelope: %v", err)
	}

	decoded, err := READ_ROW.Decode(TableEnvelope, envelopeBytes)
	if err != nil {
		t.Fatalf("Failed to decode READ_ROW: %v", err)
	}

	result, ok := decoded.(*btpb.SessionReadRowResponse)
	if !ok {
		t.Fatalf("Expected decoded result to be *btpb.SessionReadRowResponse, got %T", decoded)
	}
	if !bytes.Equal(result.Row.Key, []byte("row-key-1")) {
		t.Errorf("Decoded row key mismatch: expected %q, got %q", "row-key-1", string(result.Row.Key))
	}
	if len(result.Row.Families) != 1 || result.Row.Families[0].Name != "cf1" {
		t.Errorf("Decoded row families mismatch: got %v", result.Row.Families)
	}

	// Row-not-found sets the ReadRow oneof arm to an empty SessionReadRowResponse{}.
	notFoundBytes, err := proto.Marshal(&btpb.TableResponse{
		Payload: &btpb.TableResponse_ReadRow{ReadRow: &btpb.SessionReadRowResponse{}},
	})
	if err != nil {
		t.Fatalf("Failed to marshal row-not-found TableResponse: %v", err)
	}
	notFoundDecoded, err := READ_ROW.Decode(TableEnvelope, notFoundBytes)
	if err != nil {
		t.Fatalf("Decode(row-not-found) unexpected err: %v", err)
	}
	if nf, ok := notFoundDecoded.(*btpb.SessionReadRowResponse); !ok || nf == nil || nf.Row != nil {
		t.Errorf("Decode(row-not-found) = %#v, want non-nil *SessionReadRowResponse with nil Row", notFoundDecoded)
	}
}

func TestTableEnvelope_MutateRow(t *testing.T) {
	req := &btpb.SessionMutateRowRequest{
		Key: []byte("row-key-2"),
		Mutations: []*btpb.Mutation{
			{
				Mutation: &btpb.Mutation_DeleteFromFamily_{
					DeleteFromFamily: &btpb.Mutation_DeleteFromFamily{FamilyName: "cf2"},
				},
			},
		},
	}

	encodedBytes, err := MUTATE_ROW.Encode(TableEnvelope, req)
	if err != nil {
		t.Fatalf("Failed to encode MUTATE_ROW: %v", err)
	}

	var env btpb.TableRequest
	if err := proto.Unmarshal(encodedBytes, &env); err != nil {
		t.Fatalf("Failed to unmarshal TableRequest envelope: %v", err)
	}

	payload, ok := env.Payload.(*btpb.TableRequest_MutateRow)
	if !ok {
		t.Fatalf("Expected TableRequest_MutateRow payload, got %T", env.Payload)
	}

	mutateRowReq := payload.MutateRow
	if !bytes.Equal(mutateRowReq.Key, []byte("row-key-2")) {
		t.Errorf("Expected key %q, got %q", "row-key-2", string(mutateRowReq.Key))
	}
	if len(mutateRowReq.Mutations) != 1 || mutateRowReq.Mutations[0].GetDeleteFromFamily().FamilyName != "cf2" {
		t.Errorf("Mutations list mismatch: got %v", mutateRowReq.Mutations)
	}

	respEnvelope := &btpb.TableResponse{
		Payload: &btpb.TableResponse_MutateRow{
			MutateRow: &btpb.SessionMutateRowResponse{},
		},
	}
	envelopeBytes, err := proto.Marshal(respEnvelope)
	if err != nil {
		t.Fatalf("Failed to marshal TableResponse envelope: %v", err)
	}

	decoded, err := MUTATE_ROW.Decode(TableEnvelope, envelopeBytes)
	if err != nil {
		t.Fatalf("Failed to decode MUTATE_ROW: %v", err)
	}
	if _, ok := decoded.(*btpb.SessionMutateRowResponse); !ok {
		t.Fatalf("Expected decoded result to be *btpb.SessionMutateRowResponse, got %T", decoded)
	}
}

func TestTableEnvelope_CheckAndMutateRow(t *testing.T) {
	req := &btpb.SessionCheckAndMutateRowRequest{
		Key: []byte("row-key-cam"),
		PredicateFilter: &btpb.RowFilter{
			Filter: &btpb.RowFilter_PassAllFilter{PassAllFilter: true},
		},
		TrueMutations: []*btpb.Mutation{
			{Mutation: &btpb.Mutation_DeleteFromRow_{DeleteFromRow: &btpb.Mutation_DeleteFromRow{}}},
		},
		FalseMutations: []*btpb.Mutation{
			{Mutation: &btpb.Mutation_DeleteFromFamily_{DeleteFromFamily: &btpb.Mutation_DeleteFromFamily{FamilyName: "cf1"}}},
		},
	}

	encodedBytes, err := CHECK_AND_MUTATE_ROW.Encode(TableEnvelope, req)
	if err != nil {
		t.Fatalf("Failed to encode CHECK_AND_MUTATE_ROW: %v", err)
	}

	var env btpb.TableRequest
	if err := proto.Unmarshal(encodedBytes, &env); err != nil {
		t.Fatalf("Failed to unmarshal TableRequest: %v", err)
	}

	gotReq := env.GetCheckAndMutateRow()
	if gotReq == nil {
		t.Fatalf("Expected CheckAndMutateRow payload, got %T", env.Payload)
	}
	if !bytes.Equal(gotReq.Key, []byte("row-key-cam")) {
		t.Errorf("Key = %q, want %q", string(gotReq.Key), "row-key-cam")
	}
	if !gotReq.GetPredicateFilter().GetPassAllFilter() {
		t.Errorf("PredicateFilter mismatch: %v", gotReq.GetPredicateFilter())
	}
	if len(gotReq.GetTrueMutations()) != 1 || len(gotReq.GetFalseMutations()) != 1 {
		t.Errorf("Mutations mismatch: true=%d false=%d", len(gotReq.GetTrueMutations()), len(gotReq.GetFalseMutations()))
	}

	respEnvelope := &btpb.TableResponse{
		Payload: &btpb.TableResponse_CheckAndMutateRow{
			CheckAndMutateRow: &btpb.SessionCheckAndMutateRowResponse{PredicateMatched: true},
		},
	}
	envelopeBytes, err := proto.Marshal(respEnvelope)
	if err != nil {
		t.Fatalf("Failed to marshal TableResponse: %v", err)
	}
	decoded, err := CHECK_AND_MUTATE_ROW.Decode(TableEnvelope, envelopeBytes)
	if err != nil {
		t.Fatalf("Failed to decode CHECK_AND_MUTATE_ROW: %v", err)
	}
	res, ok := decoded.(*btpb.SessionCheckAndMutateRowResponse)
	if !ok || !res.GetPredicateMatched() {
		t.Fatalf("Decoded result = %#v, want PredicateMatched=true", decoded)
	}
}

func TestTableEnvelope_ReadModifyWriteRow(t *testing.T) {
	req := &btpb.SessionReadModifyWriteRowRequest{
		Key: []byte("row-key-rmw"),
		Rules: []*btpb.ReadModifyWriteRule{
			{
				FamilyName:      "cf1",
				ColumnQualifier: []byte("col"),
				Rule:            &btpb.ReadModifyWriteRule_IncrementAmount{IncrementAmount: 5},
			},
		},
	}

	encodedBytes, err := READ_MODIFY_WRITE_ROW.Encode(TableEnvelope, req)
	if err != nil {
		t.Fatalf("Failed to encode READ_MODIFY_WRITE_ROW: %v", err)
	}

	var env btpb.TableRequest
	if err := proto.Unmarshal(encodedBytes, &env); err != nil {
		t.Fatalf("Failed to unmarshal TableRequest: %v", err)
	}

	gotReq := env.GetReadModifyWriteRow()
	if gotReq == nil {
		t.Fatalf("Expected ReadModifyWriteRow payload, got %T", env.Payload)
	}
	if !bytes.Equal(gotReq.Key, []byte("row-key-rmw")) {
		t.Errorf("Key = %q, want %q", string(gotReq.Key), "row-key-rmw")
	}
	if len(gotReq.GetRules()) != 1 || gotReq.GetRules()[0].GetIncrementAmount() != 5 {
		t.Errorf("Rules mismatch: %v", gotReq.GetRules())
	}

	respEnvelope := &btpb.TableResponse{
		Payload: &btpb.TableResponse_ReadModifyWriteRow{
			ReadModifyWriteRow: &btpb.SessionReadModifyWriteRowResponse{
				Row: &btpb.Row{Key: []byte("row-key-rmw")},
			},
		},
	}
	envelopeBytes, err := proto.Marshal(respEnvelope)
	if err != nil {
		t.Fatalf("Failed to marshal TableResponse: %v", err)
	}
	decoded, err := READ_MODIFY_WRITE_ROW.Decode(TableEnvelope, envelopeBytes)
	if err != nil {
		t.Fatalf("Failed to decode READ_MODIFY_WRITE_ROW: %v", err)
	}
	res, ok := decoded.(*btpb.SessionReadModifyWriteRowResponse)
	if !ok || string(res.GetRow().GetKey()) != "row-key-rmw" {
		t.Fatalf("Decoded result = %#v", decoded)
	}
}

func TestAuthorizedViewEnvelope_ReadRow(t *testing.T) {
	req := &btpb.SessionReadRowRequest{Key: []byte("row-key-3")}

	encodedBytes, err := READ_ROW.Encode(AuthorizedViewEnvelope, req)
	if err != nil {
		t.Fatalf("Failed to encode READ_ROW with AuthorizedViewEnvelope: %v", err)
	}

	var env btpb.AuthorizedViewRequest
	if err := proto.Unmarshal(encodedBytes, &env); err != nil {
		t.Fatalf("Failed to unmarshal AuthorizedViewRequest envelope: %v", err)
	}

	payload, ok := env.Payload.(*btpb.AuthorizedViewRequest_ReadRow)
	if !ok {
		t.Fatalf("Expected AuthorizedViewRequest_ReadRow payload, got %T", env.Payload)
	}
	if !bytes.Equal(payload.ReadRow.Key, []byte("row-key-3")) {
		t.Errorf("Expected key %q, got %q", "row-key-3", string(payload.ReadRow.Key))
	}

	respEnvelope := &btpb.AuthorizedViewResponse{
		Payload: &btpb.AuthorizedViewResponse_ReadRow{
			ReadRow: &btpb.SessionReadRowResponse{
				Row: &btpb.Row{Key: []byte("row-key-3")},
			},
		},
	}
	envelopeBytes, err := proto.Marshal(respEnvelope)
	if err != nil {
		t.Fatalf("Failed to marshal AuthorizedViewResponse envelope: %v", err)
	}

	decoded, err := READ_ROW.Decode(AuthorizedViewEnvelope, envelopeBytes)
	if err != nil {
		t.Fatalf("Failed to decode READ_ROW with AuthorizedViewEnvelope: %v", err)
	}
	result, ok := decoded.(*btpb.SessionReadRowResponse)
	if !ok {
		t.Fatalf("Expected decoded result to be *btpb.SessionReadRowResponse, got %T", decoded)
	}
	if !bytes.Equal(result.Row.Key, []byte("row-key-3")) {
		t.Errorf("Decoded row key mismatch: expected %q, got %q", "row-key-3", string(result.Row.Key))
	}
}

func TestAuthorizedViewEnvelope_MutateRow(t *testing.T) {
	req := &btpb.SessionMutateRowRequest{Key: []byte("row-key-4")}

	encodedBytes, err := MUTATE_ROW.Encode(AuthorizedViewEnvelope, req)
	if err != nil {
		t.Fatalf("Failed to encode MUTATE_ROW with AuthorizedViewEnvelope: %v", err)
	}

	var env btpb.AuthorizedViewRequest
	if err := proto.Unmarshal(encodedBytes, &env); err != nil {
		t.Fatalf("Failed to unmarshal AuthorizedViewRequest envelope: %v", err)
	}

	payload, ok := env.Payload.(*btpb.AuthorizedViewRequest_MutateRow)
	if !ok {
		t.Fatalf("Expected AuthorizedViewRequest_MutateRow payload, got %T", env.Payload)
	}
	if !bytes.Equal(payload.MutateRow.Key, []byte("row-key-4")) {
		t.Errorf("Expected key %q, got %q", "row-key-4", string(payload.MutateRow.Key))
	}

	respEnvelope := &btpb.AuthorizedViewResponse{
		Payload: &btpb.AuthorizedViewResponse_MutateRow{
			MutateRow: &btpb.SessionMutateRowResponse{},
		},
	}
	envelopeBytes, err := proto.Marshal(respEnvelope)
	if err != nil {
		t.Fatalf("Failed to marshal AuthorizedViewResponse envelope: %v", err)
	}

	decoded, err := MUTATE_ROW.Decode(AuthorizedViewEnvelope, envelopeBytes)
	if err != nil {
		t.Fatalf("Failed to decode MUTATE_ROW with AuthorizedViewEnvelope: %v", err)
	}
	if _, ok := decoded.(*btpb.SessionMutateRowResponse); !ok {
		t.Fatalf("Expected decoded result to be *btpb.SessionMutateRowResponse, got %T", decoded)
	}
}

func TestAuthorizedViewEnvelope_CheckAndMutateRow(t *testing.T) {
	camBytes, err := CHECK_AND_MUTATE_ROW.Encode(AuthorizedViewEnvelope, &btpb.SessionCheckAndMutateRowRequest{Key: []byte("av-cam")})
	if err != nil {
		t.Fatalf("CHECK_AND_MUTATE_ROW.Encode(AuthorizedViewEnvelope): %v", err)
	}
	var avReq btpb.AuthorizedViewRequest
	if err := proto.Unmarshal(camBytes, &avReq); err != nil {
		t.Fatalf("unmarshal AuthorizedViewRequest: %v", err)
	}
	if string(avReq.GetCheckAndMutateRow().GetKey()) != "av-cam" {
		t.Errorf("got key %q, want av-cam", avReq.GetCheckAndMutateRow().GetKey())
	}
	camRespBytes, err := proto.Marshal(&btpb.AuthorizedViewResponse{
		Payload: &btpb.AuthorizedViewResponse_CheckAndMutateRow{
			CheckAndMutateRow: &btpb.SessionCheckAndMutateRowResponse{PredicateMatched: true},
		},
	})
	if err != nil {
		t.Fatalf("marshal AuthorizedViewResponse: %v", err)
	}
	camDecoded, err := CHECK_AND_MUTATE_ROW.Decode(AuthorizedViewEnvelope, camRespBytes)
	if err != nil || !camDecoded.(*btpb.SessionCheckAndMutateRowResponse).GetPredicateMatched() {
		t.Errorf("CHECK_AND_MUTATE_ROW.Decode(AuthorizedViewEnvelope) = %v, %v", camDecoded, err)
	}
}

func TestAuthorizedViewEnvelope_ReadModifyWriteRow(t *testing.T) {
	rmwBytes, err := READ_MODIFY_WRITE_ROW.Encode(AuthorizedViewEnvelope, &btpb.SessionReadModifyWriteRowRequest{Key: []byte("av-rmw")})
	if err != nil {
		t.Fatalf("READ_MODIFY_WRITE_ROW.Encode(AuthorizedViewEnvelope): %v", err)
	}
	var avReq btpb.AuthorizedViewRequest
	if err := proto.Unmarshal(rmwBytes, &avReq); err != nil {
		t.Fatalf("unmarshal AuthorizedViewRequest: %v", err)
	}
	if string(avReq.GetReadModifyWriteRow().GetKey()) != "av-rmw" {
		t.Errorf("got key %q, want av-rmw", avReq.GetReadModifyWriteRow().GetKey())
	}
	rmwRespBytes, err := proto.Marshal(&btpb.AuthorizedViewResponse{
		Payload: &btpb.AuthorizedViewResponse_ReadModifyWriteRow{
			ReadModifyWriteRow: &btpb.SessionReadModifyWriteRowResponse{
				Row: &btpb.Row{Key: []byte("av-rmw")},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal AuthorizedViewResponse: %v", err)
	}
	rmwDecoded, err := READ_MODIFY_WRITE_ROW.Decode(AuthorizedViewEnvelope, rmwRespBytes)
	if err != nil || string(rmwDecoded.(*btpb.SessionReadModifyWriteRowResponse).GetRow().GetKey()) != "av-rmw" {
		t.Errorf("READ_MODIFY_WRITE_ROW.Decode(AuthorizedViewEnvelope) = %v, %v", rmwDecoded, err)
	}
}

func TestMaterializedViewEnvelope_ReadRowAndRejectsWrites(t *testing.T) {
	req := &btpb.SessionReadRowRequest{Key: []byte("row-key-5")}

	encodedBytes, err := READ_ROW.Encode(MaterializedViewEnvelope, req)
	if err != nil {
		t.Fatalf("Failed to encode READ_ROW with MaterializedViewEnvelope: %v", err)
	}

	var env btpb.MaterializedViewRequest
	if err := proto.Unmarshal(encodedBytes, &env); err != nil {
		t.Fatalf("Failed to unmarshal MaterializedViewRequest envelope: %v", err)
	}

	payload, ok := env.Payload.(*btpb.MaterializedViewRequest_ReadRow)
	if !ok {
		t.Fatalf("Expected MaterializedViewRequest_ReadRow payload, got %T", env.Payload)
	}
	if !bytes.Equal(payload.ReadRow.Key, []byte("row-key-5")) {
		t.Errorf("Expected key %q, got %q", "row-key-5", string(payload.ReadRow.Key))
	}

	respEnvelope := &btpb.MaterializedViewResponse{
		Payload: &btpb.MaterializedViewResponse_ReadRow{
			ReadRow: &btpb.SessionReadRowResponse{
				Row: &btpb.Row{Key: []byte("row-key-5")},
			},
		},
	}
	envelopeBytes, err := proto.Marshal(respEnvelope)
	if err != nil {
		t.Fatalf("Failed to marshal MaterializedViewResponse envelope: %v", err)
	}

	decoded, err := READ_ROW.Decode(MaterializedViewEnvelope, envelopeBytes)
	if err != nil {
		t.Fatalf("Failed to decode READ_ROW with MaterializedViewEnvelope: %v", err)
	}
	result, ok := decoded.(*btpb.SessionReadRowResponse)
	if !ok {
		t.Fatalf("Expected decoded result to be *btpb.SessionReadRowResponse, got %T", decoded)
	}
	if !bytes.Equal(result.Row.Key, []byte("row-key-5")) {
		t.Errorf("Decoded row key mismatch: expected %q, got %q", "row-key-5", string(result.Row.Key))
	}

	if _, err := MUTATE_ROW.Encode(MaterializedViewEnvelope, &btpb.SessionMutateRowRequest{Key: []byte("k")}); err == nil {
		t.Errorf("MUTATE_ROW.Encode(MaterializedViewEnvelope) succeeded, want unsupported-payload error")
	}
	if _, err := CHECK_AND_MUTATE_ROW.Encode(MaterializedViewEnvelope, &btpb.SessionCheckAndMutateRowRequest{Key: []byte("k")}); err == nil {
		t.Errorf("CHECK_AND_MUTATE_ROW.Encode(MaterializedViewEnvelope) succeeded, want unsupported-payload error")
	}
	if _, err := READ_MODIFY_WRITE_ROW.Encode(MaterializedViewEnvelope, &btpb.SessionReadModifyWriteRowRequest{Key: []byte("k")}); err == nil {
		t.Errorf("READ_MODIFY_WRITE_ROW.Encode(MaterializedViewEnvelope) succeeded, want unsupported-payload error")
	}
}

type stubEnvelope struct {
	resp proto.Message
}

func (s stubEnvelope) WrapRequest(proto.Message) ([]byte, error) { return nil, nil }
func (s stubEnvelope) UnwrapResponse([]byte) (proto.Message, error) {
	return s.resp, nil
}

func TestVRpcDescriptor_ErrorCases(t *testing.T) {
	if _, err := READ_ROW.Encode(nil, &btpb.SessionReadRowRequest{Key: []byte("k")}); err == nil {
		t.Errorf("Encode(nil envelope) = nil, want error")
	}
	if _, err := READ_ROW.Decode(nil, nil); err == nil {
		t.Errorf("Decode(nil envelope) = nil, want error")
	}
	if _, err := READ_ROW.Encode(TableEnvelope, (*btpb.SessionReadRowRequest)(nil)); err == nil {
		t.Errorf("Encode(typed nil request) = nil, want error")
	}
	if _, err := READ_ROW.Encode(TableEnvelope, &btpb.SessionMutateRowRequest{Key: []byte("k")}); err == nil {
		t.Errorf("Encode(wrong request type) = nil, want error")
	}

	// Direct ResourceEnvelope calls must reject both untyped nil and typed-nil protos
	// on WrapRequest, and must reject unset response oneofs on UnwrapResponse.
	for _, tc := range []struct {
		name string
		env  ResourceEnvelope
	}{
		{"table", TableEnvelope},
		{"authorized_view", AuthorizedViewEnvelope},
		{"materialized_view", MaterializedViewEnvelope},
	} {
		if _, err := tc.env.WrapRequest(nil); err == nil {
			t.Errorf("%s.WrapRequest(nil) = nil, want error", tc.name)
		}
		if _, err := tc.env.WrapRequest((*btpb.SessionReadRowRequest)(nil)); err == nil {
			t.Errorf("%s.WrapRequest(typed nil) = nil, want error", tc.name)
		}
		if _, err := tc.env.UnwrapResponse(nil); err == nil {
			t.Errorf("%s.UnwrapResponse(empty) = nil, want error", tc.name)
		}
		if _, err := READ_ROW.Decode(tc.env, nil); err == nil {
			t.Errorf("READ_ROW.Decode(%s, empty) = nil, want error", tc.name)
		}
	}

	// Descriptor Decode rejects untyped nil and typed-nil responses even from custom envelopes.
	if _, err := READ_ROW.Decode(stubEnvelope{resp: nil}, nil); err == nil {
		t.Errorf("READ_ROW.Decode(untyped nil inner) = nil, want error")
	}
	if _, err := READ_ROW.Decode(stubEnvelope{resp: (*btpb.SessionReadRowResponse)(nil)}, nil); err == nil {
		t.Errorf("READ_ROW.Decode(typed nil inner) = nil, want error")
	}

	// Mismatched response oneof (MutateRow response for ReadRow descriptor) errors.
	mismatchBytes, err := proto.Marshal(&btpb.TableResponse{
		Payload: &btpb.TableResponse_MutateRow{MutateRow: &btpb.SessionMutateRowResponse{}},
	})
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	if _, err := READ_ROW.Decode(TableEnvelope, mismatchBytes); err == nil {
		t.Errorf("READ_ROW.Decode(MutateRow payload) = nil, want error")
	}
}
