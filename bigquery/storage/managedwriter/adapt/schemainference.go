// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package adapt

import (
	"fmt"

	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var scalarTypeKindMap = map[protoreflect.Kind]storagepb.TableFieldSchema_Type{
	protoreflect.BoolKind:     storagepb.TableFieldSchema_BOOL,
	protoreflect.Int32Kind:    storagepb.TableFieldSchema_INT64,
	protoreflect.Sint32Kind:   storagepb.TableFieldSchema_INT64,
	protoreflect.Uint32Kind:   storagepb.TableFieldSchema_INT64,
	protoreflect.Sfixed32Kind: storagepb.TableFieldSchema_INT64,
	protoreflect.Fixed32Kind:  storagepb.TableFieldSchema_INT64,
	protoreflect.Fixed64Kind:  storagepb.TableFieldSchema_INT64,
	protoreflect.Sfixed64Kind: storagepb.TableFieldSchema_INT64,
	protoreflect.FloatKind:    storagepb.TableFieldSchema_DOUBLE,
	protoreflect.DoubleKind:   storagepb.TableFieldSchema_DOUBLE,
	protoreflect.StringKind:   storagepb.TableFieldSchema_STRING,
	protoreflect.BytesKind:    storagepb.TableFieldSchema_BYTES,

	// This should be an option
	protoreflect.EnumKind: storagepb.TableFieldSchema_INT64,
}

type schemaInferer struct {
	config struct {
		enumAsString        bool
		relaxRequiredFields bool
	}
}

func (s *schemaInferer) isScalarProtoKind(k protoreflect.Kind) bool {
	if _, ok := scalarTypeKindMap[k]; ok {
		return true
	}
	return false
}

func (s *schemaInferer) protoKindToSchemaType(k protoreflect.Kind) storagepb.TableFieldSchema_Type {
	t, ok := scalarTypeKindMap[k]
	if !ok {
		return storagepb.TableFieldSchema_TYPE_UNSPECIFIED
	}
	if k == protoreflect.EnumKind {
		if s.config.enumAsString {
			t = storagepb.TableFieldSchema_STRING
		}
	}
	return t
}

func (s *schemaInferer) cardinalityToMode(c protoreflect.Cardinality) storagepb.TableFieldSchema_Mode {
	mode := storagepb.TableFieldSchema_NULLABLE
	if c == protoreflect.Repeated {
		mode = storagepb.TableFieldSchema_REPEATED
	}
	if c == protoreflect.Required && !s.config.relaxRequiredFields {
		mode = storagepb.TableFieldSchema_REQUIRED
	}
	return mode
}

func (s *schemaInferer) isWellKnownWrapperType(fn protoreflect.FullName) bool {
	for _, v := range bqTypeToWrapperMap {
		if string(fn) == v {
			return true
		}
	}
	return false
}

func (s *schemaInferer) wrapperNameToSchemaType(fn protoreflect.FullName) storagepb.TableFieldSchema_Type {
	for t, name := range bqTypeToWrapperMap {
		if string(fn) == name {
			return t
		}
	}
	return storagepb.TableFieldSchema_TYPE_UNSPECIFIED
}

// InferSchemaFromProtoMessage infers a BigQuery table schema given an input message.
func InferSchemaFromProtoMessage(in proto.Message, opts ...InferOption) (*storagepb.TableSchema, error) {
	si := &schemaInferer{}

	for _, o := range opts {
		o(si)
	}

	return si.inferSchemaFromProtoMessage(in)
}

func (s *schemaInferer) inferSchemaFromProtoMessage(in proto.Message) (*storagepb.TableSchema, error) {
	if in == nil {
		return nil, fmt.Errorf("no input message")
	}
	schema := storagepb.TableSchema{}
	md := in.ProtoReflect().Descriptor()
	for i := 0; i < md.Fields().Len(); i++ {
		tfs, err := s.convertToTFS(md.Fields().Get(i))
		if err != nil {
			return nil, fmt.Errorf("Error converting field %d in message %s: %v", i, md.FullName().Name(), err)
		}
		schema.Fields = append(schema.Fields, tfs)
	}

	return nil, fmt.Errorf("unimplemented")

}

// convertToTFS converts a protoreflect.FieldDescriptor to a schema.
func (s *schemaInferer) convertToTFS(field protoreflect.FieldDescriptor) (*storagepb.TableFieldSchema, error) {
	if field == nil {
		return nil, fmt.Errorf("nil FieldDescriptor encountered")
	}
	// TODO: normalize of field name needed?
	tfs := &storagepb.TableFieldSchema{
		Name: string(field.Name()),
		Mode: s.cardinalityToMode(field.Cardinality()),
	}

	resolved := false
	if s.isScalarProtoKind(field.Kind()) {
		tfs.Type = s.protoKindToSchemaType(field.Kind())
		resolved = true
	}

	// We're now dealing with group/message types
	if !resolved && s.isWellKnownWrapperType(field.Message().FullName()) {
		tfs.Type = s.wrapperNameToSchemaType(field.Message().FullName())
	}

	return nil, fmt.Errorf("unimplemented")
}

type inferOptions struct {
	options map[string]interface{}
}

// InferOption can be used to customize the behavior of InferSchema.
type InferOption func(*schemaInferer)

func InferEnumAsString(b bool) InferOption {
	return func(si *schemaInferer) {
		si.config.enumAsString = true
	}
}

func RelaxRequiredFields(b bool) InferOption {
	return func(si *schemaInferer) {
		si.config.relaxRequiredFields = true
	}
}
