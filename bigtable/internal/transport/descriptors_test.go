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
	if MUTATE_ROWS.Method() != "MutateRows" {
		t.Errorf("MUTATE_ROWS.Method() = %q, want %q", MUTATE_ROWS.Method(), "MutateRows")
	}
	if READ_ROW_AUTH_VIEW.Method() != "ReadRow" {
		t.Errorf("READ_ROW_AUTH_VIEW.Method() = %q, want %q", READ_ROW_AUTH_VIEW.Method(), "ReadRow")
	}
	if MUTATE_ROW_AUTH_VIEW.Method() != "MutateRow" {
		t.Errorf("MUTATE_ROW_AUTH_VIEW.Method() = %q, want %q", MUTATE_ROW_AUTH_VIEW.Method(), "MutateRow")
	}
	if CHECK_AND_MUTATE_ROW_AUTH_VIEW.Method() != "CheckAndMutateRow" {
		t.Errorf("CHECK_AND_MUTATE_ROW_AUTH_VIEW.Method() = %q, want %q", CHECK_AND_MUTATE_ROW_AUTH_VIEW.Method(), "CheckAndMutateRow")
	}
	if READ_MODIFY_WRITE_ROW_AUTH_VIEW.Method() != "ReadModifyWriteRow" {
		t.Errorf("READ_MODIFY_WRITE_ROW_AUTH_VIEW.Method() = %q, want %q", READ_MODIFY_WRITE_ROW_AUTH_VIEW.Method(), "ReadModifyWriteRow")
	}
	if MUTATE_ROWS_AUTH_VIEW.Method() != "MutateRows" {
		t.Errorf("MUTATE_ROWS_AUTH_VIEW.Method() = %q, want %q", MUTATE_ROWS_AUTH_VIEW.Method(), "MutateRows")
	}
	if READ_ROW_MAT_VIEW.Method() != "ReadRow" {
		t.Errorf("READ_ROW_MAT_VIEW.Method() = %q, want %q", READ_ROW_MAT_VIEW.Method(), "ReadRow")
	}
}

func TestTableDescriptors_ReadRow(t *testing.T) {
	// 1. Test Encoding
	args := ReadRowArgs{
		RowKey: "row-key-1",
		Filter: &btpb.RowFilter{
			Filter: &btpb.RowFilter_CellsPerRowLimitFilter{CellsPerRowLimitFilter: 1},
		},
	}

	encodedBytes, err := READ_ROW.Encode(args)
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

	// 2. Test Decoding
	row := &btpb.Row{
		Key: []byte("row-key-1"),
		Families: []*btpb.Family{
			{Name: "cf1"},
		},
	}
	innerResp := &btpb.SessionReadRowResponse{
		Row: row,
	}
	respEnvelope := &btpb.TableResponse{
		Payload: &btpb.TableResponse_ReadRow{
			ReadRow: innerResp,
		},
	}

	envelopeBytes, err := proto.Marshal(respEnvelope)
	if err != nil {
		t.Fatalf("Failed to marshal TableResponse envelope: %v", err)
	}

	decoded, err := READ_ROW.Decode(envelopeBytes)
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
}

func TestTableDescriptors_MutateRow(t *testing.T) {
	// 1. Test Encoding
	args := MutateRowArgs{
		RowKey: "row-key-2",
		Mutations: []*btpb.Mutation{
			{
				Mutation: &btpb.Mutation_DeleteFromFamily_{
					DeleteFromFamily: &btpb.Mutation_DeleteFromFamily{
						FamilyName: "cf2",
					},
				},
			},
		},
	}

	encodedBytes, err := MUTATE_ROW.Encode(args)
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

	// 2. Test Decoding
	innerResp := &btpb.SessionMutateRowResponse{}
	respEnvelope := &btpb.TableResponse{
		Payload: &btpb.TableResponse_MutateRow{
			MutateRow: innerResp,
		},
	}

	envelopeBytes, err := proto.Marshal(respEnvelope)
	if err != nil {
		t.Fatalf("Failed to marshal TableResponse envelope: %v", err)
	}

	decoded, err := MUTATE_ROW.Decode(envelopeBytes)
	if err != nil {
		t.Fatalf("Failed to decode MUTATE_ROW: %v", err)
	}

	_, ok = decoded.(*btpb.SessionMutateRowResponse)
	if !ok {
		t.Fatalf("Expected decoded result to be *btpb.SessionMutateRowResponse, got %T", decoded)
	}
}

func TestAuthorizedViewDescriptors_ReadRow(t *testing.T) {
	// 1. Test Encoding
	args := ReadRowArgs{
		RowKey: "row-key-3",
	}

	encodedBytes, err := READ_ROW_AUTH_VIEW.Encode(args)
	if err != nil {
		t.Fatalf("Failed to encode READ_ROW_AUTH_VIEW: %v", err)
	}

	var env btpb.AuthorizedViewRequest
	if err := proto.Unmarshal(encodedBytes, &env); err != nil {
		t.Fatalf("Failed to unmarshal AuthorizedViewRequest envelope: %v", err)
	}

	payload, ok := env.Payload.(*btpb.AuthorizedViewRequest_ReadRow)
	if !ok {
		t.Fatalf("Expected AuthorizedViewRequest_ReadRow payload, got %T", env.Payload)
	}

	readRowReq := payload.ReadRow
	if !bytes.Equal(readRowReq.Key, []byte("row-key-3")) {
		t.Errorf("Expected key %q, got %q", "row-key-3", string(readRowReq.Key))
	}

	// 2. Test Decoding
	row := &btpb.Row{
		Key: []byte("row-key-3"),
	}
	innerResp := &btpb.SessionReadRowResponse{
		Row: row,
	}
	respEnvelope := &btpb.AuthorizedViewResponse{
		Payload: &btpb.AuthorizedViewResponse_ReadRow{
			ReadRow: innerResp,
		},
	}

	envelopeBytes, err := proto.Marshal(respEnvelope)
	if err != nil {
		t.Fatalf("Failed to marshal AuthorizedViewResponse envelope: %v", err)
	}

	decoded, err := READ_ROW_AUTH_VIEW.Decode(envelopeBytes)
	if err != nil {
		t.Fatalf("Failed to decode READ_ROW_AUTH_VIEW: %v", err)
	}

	result, ok := decoded.(*btpb.SessionReadRowResponse)
	if !ok {
		t.Fatalf("Expected decoded result to be *btpb.SessionReadRowResponse, got %T", decoded)
	}

	if !bytes.Equal(result.Row.Key, []byte("row-key-3")) {
		t.Errorf("Decoded row key mismatch: expected %q, got %q", "row-key-3", string(result.Row.Key))
	}
}

func TestAuthorizedViewDescriptors_MutateRow(t *testing.T) {
	// 1. Test Encoding
	args := MutateRowArgs{
		RowKey: "row-key-4",
	}

	encodedBytes, err := MUTATE_ROW_AUTH_VIEW.Encode(args)
	if err != nil {
		t.Fatalf("Failed to encode MUTATE_ROW_AUTH_VIEW: %v", err)
	}

	var env btpb.AuthorizedViewRequest
	if err := proto.Unmarshal(encodedBytes, &env); err != nil {
		t.Fatalf("Failed to unmarshal AuthorizedViewRequest envelope: %v", err)
	}

	payload, ok := env.Payload.(*btpb.AuthorizedViewRequest_MutateRow)
	if !ok {
		t.Fatalf("Expected AuthorizedViewRequest_MutateRow payload, got %T", env.Payload)
	}

	mutateRowReq := payload.MutateRow
	if !bytes.Equal(mutateRowReq.Key, []byte("row-key-4")) {
		t.Errorf("Expected key %q, got %q", "row-key-4", string(mutateRowReq.Key))
	}

	// 2. Test Decoding
	innerResp := &btpb.SessionMutateRowResponse{}
	respEnvelope := &btpb.AuthorizedViewResponse{
		Payload: &btpb.AuthorizedViewResponse_MutateRow{
			MutateRow: innerResp,
		},
	}

	envelopeBytes, err := proto.Marshal(respEnvelope)
	if err != nil {
		t.Fatalf("Failed to marshal AuthorizedViewResponse envelope: %v", err)
	}

	decoded, err := MUTATE_ROW_AUTH_VIEW.Decode(envelopeBytes)
	if err != nil {
		t.Fatalf("Failed to decode MUTATE_ROW_AUTH_VIEW: %v", err)
	}

	_, ok = decoded.(*btpb.SessionMutateRowResponse)
	if !ok {
		t.Fatalf("Expected decoded result to be *btpb.SessionMutateRowResponse, got %T", decoded)
	}
}

func TestMaterializedViewDescriptors_ReadRow(t *testing.T) {
	// 1. Test Encoding
	args := ReadRowArgs{
		RowKey: "row-key-5",
	}

	encodedBytes, err := READ_ROW_MAT_VIEW.Encode(args)
	if err != nil {
		t.Fatalf("Failed to encode READ_ROW_MAT_VIEW: %v", err)
	}

	var env btpb.MaterializedViewRequest
	if err := proto.Unmarshal(encodedBytes, &env); err != nil {
		t.Fatalf("Failed to unmarshal MaterializedViewRequest envelope: %v", err)
	}

	payload, ok := env.Payload.(*btpb.MaterializedViewRequest_ReadRow)
	if !ok {
		t.Fatalf("Expected MaterializedViewRequest_ReadRow payload, got %T", env.Payload)
	}

	readRowReq := payload.ReadRow
	if !bytes.Equal(readRowReq.Key, []byte("row-key-5")) {
		t.Errorf("Expected key %q, got %q", "row-key-5", string(readRowReq.Key))
	}

	// 2. Test Decoding
	row := &btpb.Row{
		Key: []byte("row-key-5"),
	}
	innerResp := &btpb.SessionReadRowResponse{
		Row: row,
	}
	respEnvelope := &btpb.MaterializedViewResponse{
		Payload: &btpb.MaterializedViewResponse_ReadRow{
			ReadRow: innerResp,
		},
	}

	envelopeBytes, err := proto.Marshal(respEnvelope)
	if err != nil {
		t.Fatalf("Failed to marshal MaterializedViewResponse envelope: %v", err)
	}

	decoded, err := READ_ROW_MAT_VIEW.Decode(envelopeBytes)
	if err != nil {
		t.Fatalf("Failed to decode READ_ROW_MAT_VIEW: %v", err)
	}

	result, ok := decoded.(*btpb.SessionReadRowResponse)
	if !ok {
		t.Fatalf("Expected decoded result to be *btpb.SessionReadRowResponse, got %T", decoded)
	}

	if !bytes.Equal(result.Row.Key, []byte("row-key-5")) {
		t.Errorf("Decoded row key mismatch: expected %q, got %q", "row-key-5", string(result.Row.Key))
	}
}

func TestTableDescriptors_CheckAndMutateRow(t *testing.T) {
	args := CheckAndMutateRowArgs{
		RowKey: "row-key-cam",
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

	encodedBytes, err := CHECK_AND_MUTATE_ROW.Encode(args)
	if err != nil {
		t.Fatalf("Failed to encode CHECK_AND_MUTATE_ROW: %v", err)
	}

	var env btpb.TableRequest
	if err := proto.Unmarshal(encodedBytes, &env); err != nil {
		t.Fatalf("Failed to unmarshal TableRequest: %v", err)
	}

	req := env.GetCheckAndMutateRow()
	if req == nil {
		t.Fatalf("Expected CheckAndMutateRow payload, got %T", env.Payload)
	}
	if !bytes.Equal(req.Key, []byte("row-key-cam")) {
		t.Errorf("Key = %q, want %q", string(req.Key), "row-key-cam")
	}
	if !req.GetPredicateFilter().GetPassAllFilter() {
		t.Errorf("PredicateFilter mismatch: %v", req.GetPredicateFilter())
	}
	if len(req.GetTrueMutations()) != 1 || len(req.GetFalseMutations()) != 1 {
		t.Errorf("Mutations mismatch: true=%d false=%d", len(req.GetTrueMutations()), len(req.GetFalseMutations()))
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
	decoded, err := CHECK_AND_MUTATE_ROW.Decode(envelopeBytes)
	if err != nil {
		t.Fatalf("Failed to decode CHECK_AND_MUTATE_ROW: %v", err)
	}
	res, ok := decoded.(*btpb.SessionCheckAndMutateRowResponse)
	if !ok || !res.GetPredicateMatched() {
		t.Fatalf("Decoded result = %#v, want PredicateMatched=true", decoded)
	}
}

func TestTableDescriptors_ReadModifyWriteRow(t *testing.T) {
	args := ReadModifyWriteRowArgs{
		RowKey: "row-key-rmw",
		Rules: []*btpb.ReadModifyWriteRule{
			{
				FamilyName:      "cf1",
				ColumnQualifier: []byte("col"),
				Rule:            &btpb.ReadModifyWriteRule_IncrementAmount{IncrementAmount: 5},
			},
		},
	}

	encodedBytes, err := READ_MODIFY_WRITE_ROW.Encode(args)
	if err != nil {
		t.Fatalf("Failed to encode READ_MODIFY_WRITE_ROW: %v", err)
	}

	var env btpb.TableRequest
	if err := proto.Unmarshal(encodedBytes, &env); err != nil {
		t.Fatalf("Failed to unmarshal TableRequest: %v", err)
	}

	req := env.GetReadModifyWriteRow()
	if req == nil {
		t.Fatalf("Expected ReadModifyWriteRow payload, got %T", env.Payload)
	}
	if !bytes.Equal(req.Key, []byte("row-key-rmw")) {
		t.Errorf("Key = %q, want %q", string(req.Key), "row-key-rmw")
	}
	if len(req.GetRules()) != 1 || req.GetRules()[0].GetIncrementAmount() != 5 {
		t.Errorf("Rules mismatch: %v", req.GetRules())
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
	decoded, err := READ_MODIFY_WRITE_ROW.Decode(envelopeBytes)
	if err != nil {
		t.Fatalf("Failed to decode READ_MODIFY_WRITE_ROW: %v", err)
	}
	res, ok := decoded.(*btpb.SessionReadModifyWriteRowResponse)
	if !ok || string(res.GetRow().GetKey()) != "row-key-rmw" {
		t.Fatalf("Decoded result = %#v", decoded)
	}
}

func TestTableDescriptors_MutateRows(t *testing.T) {
	args := MutateRowsArgs{
		Entries: []*btpb.SessionMutateRowsRequest_Entry{
			{
				Key: []byte("r1"),
				Mutations: []*btpb.Mutation{
					{Mutation: &btpb.Mutation_DeleteFromRow_{DeleteFromRow: &btpb.Mutation_DeleteFromRow{}}},
				},
			},
		},
	}

	encodedBytes, err := MUTATE_ROWS.Encode(args)
	if err != nil {
		t.Fatalf("Failed to encode MUTATE_ROWS: %v", err)
	}

	var env btpb.TableRequest
	if err := proto.Unmarshal(encodedBytes, &env); err != nil {
		t.Fatalf("Failed to unmarshal TableRequest: %v", err)
	}

	req := env.GetMutateRows()
	if req == nil || len(req.GetEntries()) != 1 || string(req.GetEntries()[0].GetKey()) != "r1" {
		t.Fatalf("Unexpected MutateRows payload: %v", req)
	}

	respEnvelope := &btpb.TableResponse{
		Payload: &btpb.TableResponse_MutateRows{
			MutateRows: &btpb.SessionMutateRowsResponse{
				Entries: []*btpb.SessionMutateRowsResponse_Entry{
					{Index: 0},
				},
			},
		},
	}
	envelopeBytes, err := proto.Marshal(respEnvelope)
	if err != nil {
		t.Fatalf("Failed to marshal TableResponse: %v", err)
	}
	decoded, err := MUTATE_ROWS.Decode(envelopeBytes)
	if err != nil {
		t.Fatalf("Failed to decode MUTATE_ROWS: %v", err)
	}
	res, ok := decoded.(*btpb.SessionMutateRowsResponse)
	if !ok || len(res.GetEntries()) != 1 {
		t.Fatalf("Decoded result = %#v", decoded)
	}
}

func TestAuthorizedViewDescriptors_WriteVRpcs(t *testing.T) {
	// 1. CheckAndMutateRow
	camBytes, err := CHECK_AND_MUTATE_ROW_AUTH_VIEW.Encode(CheckAndMutateRowArgs{RowKey: "av-cam"})
	if err != nil {
		t.Fatalf("CHECK_AND_MUTATE_ROW_AUTH_VIEW.Encode: %v", err)
	}
	var avReq btpb.AuthorizedViewRequest
	if err := proto.Unmarshal(camBytes, &avReq); err != nil {
		t.Fatalf("unmarshal AuthorizedViewRequest: %v", err)
	}
	if string(avReq.GetCheckAndMutateRow().GetKey()) != "av-cam" {
		t.Errorf("got key %q, want av-cam", avReq.GetCheckAndMutateRow().GetKey())
	}
	camRespBytes, _ := proto.Marshal(&btpb.AuthorizedViewResponse{
		Payload: &btpb.AuthorizedViewResponse_CheckAndMutateRow{
			CheckAndMutateRow: &btpb.SessionCheckAndMutateRowResponse{PredicateMatched: true},
		},
	})
	camDecoded, err := CHECK_AND_MUTATE_ROW_AUTH_VIEW.Decode(camRespBytes)
	if err != nil || !camDecoded.(*btpb.SessionCheckAndMutateRowResponse).GetPredicateMatched() {
		t.Errorf("CHECK_AND_MUTATE_ROW_AUTH_VIEW.Decode = %v, %v", camDecoded, err)
	}

	// 2. ReadModifyWriteRow
	rmwBytes, err := READ_MODIFY_WRITE_ROW_AUTH_VIEW.Encode(ReadModifyWriteRowArgs{RowKey: "av-rmw"})
	if err != nil {
		t.Fatalf("READ_MODIFY_WRITE_ROW_AUTH_VIEW.Encode: %v", err)
	}
	avReq.Reset()
	if err := proto.Unmarshal(rmwBytes, &avReq); err != nil {
		t.Fatalf("unmarshal AuthorizedViewRequest: %v", err)
	}
	if string(avReq.GetReadModifyWriteRow().GetKey()) != "av-rmw" {
		t.Errorf("got key %q, want av-rmw", avReq.GetReadModifyWriteRow().GetKey())
	}
	rmwRespBytes, _ := proto.Marshal(&btpb.AuthorizedViewResponse{
		Payload: &btpb.AuthorizedViewResponse_ReadModifyWriteRow{
			ReadModifyWriteRow: &btpb.SessionReadModifyWriteRowResponse{
				Row: &btpb.Row{Key: []byte("av-rmw")},
			},
		},
	})
	rmwDecoded, err := READ_MODIFY_WRITE_ROW_AUTH_VIEW.Decode(rmwRespBytes)
	if err != nil || string(rmwDecoded.(*btpb.SessionReadModifyWriteRowResponse).GetRow().GetKey()) != "av-rmw" {
		t.Errorf("READ_MODIFY_WRITE_ROW_AUTH_VIEW.Decode = %v, %v", rmwDecoded, err)
	}

	// 3. MutateRows
	mrBytes, err := MUTATE_ROWS_AUTH_VIEW.Encode(MutateRowsArgs{
		Entries: []*btpb.SessionMutateRowsRequest_Entry{{Key: []byte("av-mr")}},
	})
	if err != nil {
		t.Fatalf("MUTATE_ROWS_AUTH_VIEW.Encode: %v", err)
	}
	avReq.Reset()
	if err := proto.Unmarshal(mrBytes, &avReq); err != nil {
		t.Fatalf("unmarshal AuthorizedViewRequest: %v", err)
	}
	if len(avReq.GetMutateRows().GetEntries()) != 1 || string(avReq.GetMutateRows().GetEntries()[0].GetKey()) != "av-mr" {
		t.Errorf("got entries %v, want av-mr", avReq.GetMutateRows().GetEntries())
	}
	mrRespBytes, _ := proto.Marshal(&btpb.AuthorizedViewResponse{
		Payload: &btpb.AuthorizedViewResponse_MutateRows{
			MutateRows: &btpb.SessionMutateRowsResponse{
				Entries: []*btpb.SessionMutateRowsResponse_Entry{{Index: 0}},
			},
		},
	})
	mrDecoded, err := MUTATE_ROWS_AUTH_VIEW.Decode(mrRespBytes)
	if err != nil || len(mrDecoded.(*btpb.SessionMutateRowsResponse).GetEntries()) != 1 {
		t.Errorf("MUTATE_ROWS_AUTH_VIEW.Decode = %v, %v", mrDecoded, err)
	}
}
