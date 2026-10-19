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
	if READ_ROW_AUTH_VIEW.Method() != "ReadRow" {
		t.Errorf("READ_ROW_AUTH_VIEW.Method() = %q, want %q", READ_ROW_AUTH_VIEW.Method(), "ReadRow")
	}
	if MUTATE_ROW_AUTH_VIEW.Method() != "MutateRow" {
		t.Errorf("MUTATE_ROW_AUTH_VIEW.Method() = %q, want %q", MUTATE_ROW_AUTH_VIEW.Method(), "MutateRow")
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
