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
	"log"

	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var scalarTypeKindMap = map[protoreflect.Kind]storagepb.TableFieldSchema_Type{
	protoreflect.BoolKind:     storagepb.TableFieldSchema_BOOL,
	protoreflect.Int32Kind:    storagepb.TableFieldSchema_INT64,
	protoreflect.Int64Kind:    storagepb.TableFieldSchema_INT64,
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

var bqWrapperToTypeMap = map[string]storagepb.TableFieldSchema_Type{
	".google.protobuf.BoolValue":   storagepb.TableFieldSchema_BOOL,
	".google.protobuf.BytesValue":  storagepb.TableFieldSchema_BYTES,
	".google.protobuf.Int32Value":  storagepb.TableFieldSchema_INT64,
	".google.protobuf.Int64Value":  storagepb.TableFieldSchema_INT64,
	".google.protobuf.DoubleValue": storagepb.TableFieldSchema_DOUBLE,
	".google.protobuf.StringValue": storagepb.TableFieldSchema_STRING,
}

type NameStyle string

var (
	DefaultNameStyle NameStyle = "DEFAULT"
	TextNameStyle    NameStyle = "TEXT"
	JSONNameStyle    NameStyle = "JSON"
)

type schemaInferer struct {
	config struct {
		nameStyle           NameStyle
		enumAsString        bool
		relaxRequiredFields bool
		allowWrapperTypes   bool
	}
}

func (s *schemaInferer) isScalarProtoKind(k protoreflect.Kind) bool {
	if _, ok := scalarTypeKindMap[k]; ok {
		return true
	}
	return false
}

func (s *schemaInferer) getFieldName(field protoreflect.FieldDescriptor) string {
	if s.config.nameStyle == TextNameStyle {
		return field.TextName()
	}
	if s.config.nameStyle == JSONNameStyle {
		return field.JSONName()
	}
	return string(field.Name())
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
	name := string(fn)
	// TODO: determine if we should fully qualify refs in this way.
	if name[0:1] != "." {
		name = fmt.Sprintf(".%s", name)
	}
	log.Printf("type: %s", fn)
	if s.config.allowWrapperTypes {
		for _, v := range bqTypeToWrapperMap {
			if name == v {
				return true
			}
		}
	}
	return false
}

func (s *schemaInferer) wrapperNameToSchemaType(fn protoreflect.FullName) storagepb.TableFieldSchema_Type {
	name := string(fn)
	// TODO: determine if we should fully qualify refs in this way.
	if name[0:1] != "." {
		name = fmt.Sprintf(".%s", name)
	}
	for wrap, typ := range bqWrapperToTypeMap {

		if name == wrap {
			return typ
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
	schema := &storagepb.TableSchema{}
	md := in.ProtoReflect().Descriptor()
	for i := 0; i < md.Fields().Len(); i++ {
		tfs, err := s.convertToTFS(md.Fields().Get(i))
		if err != nil {
			return nil, fmt.Errorf("Error converting field %d/%s in message %s: %v", i, md.Fields().Get(i).Name(), md.FullName().Name(), err)
		}
		schema.Fields = append(schema.Fields, tfs)
	}

	return schema, nil
}

// convertToTFS converts a protoreflect.FieldDescriptor to a schema.
func (s *schemaInferer) convertToTFS(field protoreflect.FieldDescriptor) (*storagepb.TableFieldSchema, error) {
	if field == nil {
		return nil, fmt.Errorf("nil FieldDescriptor encountered")
	}
	// TODO: normalize of field name needed?
	tfs := &storagepb.TableFieldSchema{
		Name: s.getFieldName(field),
		Mode: s.cardinalityToMode(field.Cardinality()),
	}

	resolved := false
	if s.isScalarProtoKind(field.Kind()) {
		tfs.Type = s.protoKindToSchemaType(field.Kind())
		resolved = true
	}

	// We're now dealing with group/message types
	if !resolved && field.Message() != nil {
		if s.isWellKnownWrapperType(field.Message().FullName()) {
			tfs.Type = s.wrapperNameToSchemaType(field.Message().FullName())
		} else {
			return nil, fmt.Errorf("TODO implement nested message parsing")
		}
	}

	return tfs, nil
}

type inferOptions struct {
	options map[string]interface{}
}

// InferOption can be used to customize the behavior of InferSchema.
type InferOption func(*schemaInferer)

// InferEnumAsString governs the behavior of how Enum fields are inferred.  By default, enums
// are treated as INT64 values, but this option infers enums as a string (e.g. the enum nameß).
func InferEnumAsString(val bool) InferOption {
	return func(si *schemaInferer) {
		si.config.enumAsString = val
	}
}

// RelaxRequiredFields relaxes all required fields to a nullable equivalent when inferring
// schema.
func RelaxRequiredFields(val bool) InferOption {
	return func(si *schemaInferer) {
		si.config.relaxRequiredFields = val
	}
}

// UseNameStyle determines the naming style to use for inferred column/field names.
func UseNameStyle(style NameStyle) InferOption {
	return func(si *schemaInferer) {
		si.config.nameStyle = style
	}
}

// AllowWrapperTypes governs whether well-known wrapper types are simplifed to their
// underlying wrapped types.
func AllowWrapperTypes(val bool) InferOption {
	return func(si *schemaInferer) {
		si.config.allowWrapperTypes = val
	}
}
