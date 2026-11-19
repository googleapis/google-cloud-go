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
	"fmt"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/protobuf/proto"
)

// ResourceEnvelope wraps method-level session request protos into the outer
// wire envelope for a specific Bigtable resource kind (TableRequest,
// AuthorizedViewRequest, or MaterializedViewRequest) and unwraps the
// corresponding wire response back into the method-level session response
// proto.
type ResourceEnvelope interface {
	WrapRequest(inner proto.Message) ([]byte, error)
	UnwrapResponse(buf []byte) (proto.Message, error)
}

// Envelope returns the ResourceEnvelope corresponding to this SessionType.
func (t SessionType) Envelope() (ResourceEnvelope, error) {
	switch t {
	case SessionTypeTable:
		return TableEnvelope, nil
	case SessionTypeAuthorizedView:
		return AuthorizedViewEnvelope, nil
	case SessionTypeMaterializedView:
		return MaterializedViewEnvelope, nil
	default:
		return nil, fmt.Errorf("unknown session type: %v", t)
	}
}

type tableEnvelope struct{}

func (tableEnvelope) WrapRequest(inner proto.Message) ([]byte, error) {
	if inner == nil || !inner.ProtoReflect().IsValid() {
		return nil, fmt.Errorf("nil table vRPC request payload (%T)", inner)
	}
	env := &btpb.TableRequest{}
	switch r := inner.(type) {
	case *btpb.SessionReadRowRequest:
		env.Payload = &btpb.TableRequest_ReadRow{ReadRow: r}
	case *btpb.SessionMutateRowRequest:
		env.Payload = &btpb.TableRequest_MutateRow{MutateRow: r}
	case *btpb.SessionCheckAndMutateRowRequest:
		env.Payload = &btpb.TableRequest_CheckAndMutateRow{CheckAndMutateRow: r}
	case *btpb.SessionReadModifyWriteRowRequest:
		env.Payload = &btpb.TableRequest_ReadModifyWriteRow{ReadModifyWriteRow: r}
	default:
		return nil, fmt.Errorf("unsupported table vRPC request payload %T", inner)
	}
	return proto.Marshal(env)
}

func (tableEnvelope) UnwrapResponse(buf []byte) (proto.Message, error) {
	env := &btpb.TableResponse{}
	if err := proto.Unmarshal(buf, env); err != nil {
		return nil, err
	}
	var inner proto.Message
	switch p := env.Payload.(type) {
	case *btpb.TableResponse_ReadRow:
		inner = p.ReadRow
	case *btpb.TableResponse_MutateRow:
		inner = p.MutateRow
	case *btpb.TableResponse_CheckAndMutateRow:
		inner = p.CheckAndMutateRow
	case *btpb.TableResponse_ReadModifyWriteRow:
		inner = p.ReadModifyWriteRow
	case nil:
		return nil, fmt.Errorf("missing table vRPC response payload")
	default:
		return nil, fmt.Errorf("unsupported table vRPC response payload %T", env.Payload)
	}
	if inner == nil || !inner.ProtoReflect().IsValid() {
		return nil, fmt.Errorf("missing table vRPC response payload (%T)", env.Payload)
	}
	return inner, nil
}

type authorizedViewEnvelope struct{}

func (authorizedViewEnvelope) WrapRequest(inner proto.Message) ([]byte, error) {
	if inner == nil || !inner.ProtoReflect().IsValid() {
		return nil, fmt.Errorf("nil authorized view vRPC request payload (%T)", inner)
	}
	env := &btpb.AuthorizedViewRequest{}
	switch r := inner.(type) {
	case *btpb.SessionReadRowRequest:
		env.Payload = &btpb.AuthorizedViewRequest_ReadRow{ReadRow: r}
	case *btpb.SessionMutateRowRequest:
		env.Payload = &btpb.AuthorizedViewRequest_MutateRow{MutateRow: r}
	case *btpb.SessionCheckAndMutateRowRequest:
		env.Payload = &btpb.AuthorizedViewRequest_CheckAndMutateRow{CheckAndMutateRow: r}
	case *btpb.SessionReadModifyWriteRowRequest:
		env.Payload = &btpb.AuthorizedViewRequest_ReadModifyWriteRow{ReadModifyWriteRow: r}
	default:
		return nil, fmt.Errorf("unsupported authorized view vRPC request payload %T", inner)
	}
	return proto.Marshal(env)
}

func (authorizedViewEnvelope) UnwrapResponse(buf []byte) (proto.Message, error) {
	env := &btpb.AuthorizedViewResponse{}
	if err := proto.Unmarshal(buf, env); err != nil {
		return nil, err
	}
	var inner proto.Message
	switch p := env.Payload.(type) {
	case *btpb.AuthorizedViewResponse_ReadRow:
		inner = p.ReadRow
	case *btpb.AuthorizedViewResponse_MutateRow:
		inner = p.MutateRow
	case *btpb.AuthorizedViewResponse_CheckAndMutateRow:
		inner = p.CheckAndMutateRow
	case *btpb.AuthorizedViewResponse_ReadModifyWriteRow:
		inner = p.ReadModifyWriteRow
	case nil:
		return nil, fmt.Errorf("missing authorized view vRPC response payload")
	default:
		return nil, fmt.Errorf("unsupported authorized view vRPC response payload %T", env.Payload)
	}
	if inner == nil || !inner.ProtoReflect().IsValid() {
		return nil, fmt.Errorf("missing authorized view vRPC response payload (%T)", env.Payload)
	}
	return inner, nil
}

type materializedViewEnvelope struct{}

func (materializedViewEnvelope) WrapRequest(inner proto.Message) ([]byte, error) {
	if inner == nil || !inner.ProtoReflect().IsValid() {
		return nil, fmt.Errorf("nil materialized view vRPC request payload (%T)", inner)
	}
	env := &btpb.MaterializedViewRequest{}
	switch r := inner.(type) {
	case *btpb.SessionReadRowRequest:
		env.Payload = &btpb.MaterializedViewRequest_ReadRow{ReadRow: r}
	default:
		return nil, fmt.Errorf("unsupported materialized view vRPC request payload %T", inner)
	}
	return proto.Marshal(env)
}

func (materializedViewEnvelope) UnwrapResponse(buf []byte) (proto.Message, error) {
	env := &btpb.MaterializedViewResponse{}
	if err := proto.Unmarshal(buf, env); err != nil {
		return nil, err
	}
	var inner proto.Message
	switch p := env.Payload.(type) {
	case *btpb.MaterializedViewResponse_ReadRow:
		inner = p.ReadRow
	case nil:
		return nil, fmt.Errorf("missing materialized view vRPC response payload")
	default:
		return nil, fmt.Errorf("unsupported materialized view vRPC response payload %T", env.Payload)
	}
	if inner == nil || !inner.ProtoReflect().IsValid() {
		return nil, fmt.Errorf("missing materialized view vRPC response payload (%T)", env.Payload)
	}
	return inner, nil
}

var (
	// TableEnvelope frames vRPC payloads in TableRequest / TableResponse.
	TableEnvelope ResourceEnvelope = tableEnvelope{}
	// AuthorizedViewEnvelope frames vRPC payloads in AuthorizedViewRequest / AuthorizedViewResponse.
	AuthorizedViewEnvelope ResourceEnvelope = authorizedViewEnvelope{}
	// MaterializedViewEnvelope frames vRPC payloads in MaterializedViewRequest / MaterializedViewResponse.
	MaterializedViewEnvelope ResourceEnvelope = materializedViewEnvelope{}
)

// VRpcDescriptor defines a virtual RPC method and its method-level request and
// response conversion, independent of the outer resource envelope.
type VRpcDescriptor interface {
	Method() string
	Encode(env ResourceEnvelope, req interface{}) ([]byte, error)
	Decode(env ResourceEnvelope, buf []byte) (interface{}, error)
}

type vrpcDescriptor[ReqVal any, RespVal any, Req interface {
	*ReqVal
	proto.Message
}, Resp interface {
	*RespVal
	proto.Message
}] struct {
	method string
}

func (d vrpcDescriptor[ReqVal, RespVal, Req, Resp]) Method() string {
	return d.method
}

func (d vrpcDescriptor[ReqVal, RespVal, Req, Resp]) Encode(env ResourceEnvelope, req interface{}) ([]byte, error) {
	if env == nil {
		return nil, fmt.Errorf("nil resource envelope for %s", d.method)
	}
	typed, ok := req.(Req)
	if !ok || typed == nil {
		return nil, fmt.Errorf("invalid %s request payload (%T)", d.method, req)
	}
	return env.WrapRequest(typed)
}

func (d vrpcDescriptor[ReqVal, RespVal, Req, Resp]) Decode(env ResourceEnvelope, buf []byte) (interface{}, error) {
	if env == nil {
		return nil, fmt.Errorf("nil resource envelope for %s", d.method)
	}
	inner, err := env.UnwrapResponse(buf)
	if err != nil {
		return nil, err
	}
	resp, ok := inner.(Resp)
	if !ok || resp == nil {
		return nil, fmt.Errorf("unexpected %s response payload %T", d.method, inner)
	}
	return resp, nil
}

func newVRpcDescriptor[ReqVal any, RespVal any, Req interface {
	*ReqVal
	proto.Message
}, Resp interface {
	*RespVal
	proto.Message
}](method string) VRpcDescriptor {
	return vrpcDescriptor[ReqVal, RespVal, Req, Resp]{method: method}
}

var (
	// READ_ROW describes the ReadRow virtual RPC across all session resource kinds.
	READ_ROW = newVRpcDescriptor[btpb.SessionReadRowRequest, btpb.SessionReadRowResponse]("ReadRow")

	// MUTATE_ROW describes the MutateRow virtual RPC across writable session resource kinds.
	MUTATE_ROW = newVRpcDescriptor[btpb.SessionMutateRowRequest, btpb.SessionMutateRowResponse]("MutateRow")

	// CHECK_AND_MUTATE_ROW describes the CheckAndMutateRow virtual RPC across writable session resource kinds.
	CHECK_AND_MUTATE_ROW = newVRpcDescriptor[btpb.SessionCheckAndMutateRowRequest, btpb.SessionCheckAndMutateRowResponse]("CheckAndMutateRow")

	// READ_MODIFY_WRITE_ROW describes the ReadModifyWriteRow virtual RPC across writable session resource kinds.
	READ_MODIFY_WRITE_ROW = newVRpcDescriptor[btpb.SessionReadModifyWriteRowRequest, btpb.SessionReadModifyWriteRowResponse]("ReadModifyWriteRow")
)
