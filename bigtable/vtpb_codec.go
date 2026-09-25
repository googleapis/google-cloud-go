/*
Copyright 2025 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bigtable

import (
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// vtprotoUnmarshaler is implemented by vtprotobuf-generated types.
type vtprotoUnmarshaler interface {
	UnmarshalVT([]byte) error
}

// vtprotoCodec is a gRPC codec that routes UnmarshalVT for types that support
// it (i.e. vtprotobuf-generated messages), falling back to proto.Unmarshal.
// It is used as a per-call grpc.ForceCodec option on ReadRows so that
// ReadRowsResponse is decoded via the faster vtproto path without affecting
// any other RPCs or global gRPC state.
type vtprotoCodec struct{}

func (vtprotoCodec) Marshal(v any) ([]byte, error) {
	return proto.Marshal(v.(proto.Message))
}

func (vtprotoCodec) Unmarshal(data []byte, v any) error {
	if m, ok := v.(vtprotoUnmarshaler); ok {
		return m.UnmarshalVT(data)
	}
	return proto.Unmarshal(data, v.(proto.Message))
}

func (vtprotoCodec) Name() string { return "proto" }

// readRowsCodecOpt is the grpc.CallOption that activates the vtproto codec for
// a single ReadRows stream, routing response deserialization through UnmarshalVT.
var readRowsCodecOpt = grpc.ForceCodec(vtprotoCodec{})
