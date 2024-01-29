// Copyright 2021 Google LLC
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
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var bqModeToFieldLabelMapProto2 = map[storagepb.TableFieldSchema_Mode]descriptorpb.FieldDescriptorProto_Label{
	storagepb.TableFieldSchema_NULLABLE: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL,
	storagepb.TableFieldSchema_REPEATED: descriptorpb.FieldDescriptorProto_LABEL_REPEATED,
	storagepb.TableFieldSchema_REQUIRED: descriptorpb.FieldDescriptorProto_LABEL_REQUIRED,
}

var bqModeToFieldLabelMapProto3 = map[storagepb.TableFieldSchema_Mode]descriptorpb.FieldDescriptorProto_Label{
	storagepb.TableFieldSchema_NULLABLE: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL,
	storagepb.TableFieldSchema_REPEATED: descriptorpb.FieldDescriptorProto_LABEL_REPEATED,
	storagepb.TableFieldSchema_REQUIRED: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL,
}

func convertModeToLabel(mode storagepb.TableFieldSchema_Mode, useProto3 bool) *descriptorpb.FieldDescriptorProto_Label {
	if useProto3 {
		return bqModeToFieldLabelMapProto3[mode].Enum()
	}
	return bqModeToFieldLabelMapProto2[mode].Enum()
}

// Allows conversion between BQ schema type and FieldDescriptorProto's type.
var bqTypeToFieldTypeMap = map[storagepb.TableFieldSchema_Type]descriptorpb.FieldDescriptorProto_Type{
	storagepb.TableFieldSchema_BIGNUMERIC: descriptorpb.FieldDescriptorProto_TYPE_BYTES,
	storagepb.TableFieldSchema_BOOL:       descriptorpb.FieldDescriptorProto_TYPE_BOOL,
	storagepb.TableFieldSchema_BYTES:      descriptorpb.FieldDescriptorProto_TYPE_BYTES,
	storagepb.TableFieldSchema_DATE:       descriptorpb.FieldDescriptorProto_TYPE_INT32,
	storagepb.TableFieldSchema_DATETIME:   descriptorpb.FieldDescriptorProto_TYPE_INT64,
	storagepb.TableFieldSchema_DOUBLE:     descriptorpb.FieldDescriptorProto_TYPE_DOUBLE,
	storagepb.TableFieldSchema_GEOGRAPHY:  descriptorpb.FieldDescriptorProto_TYPE_STRING,
	storagepb.TableFieldSchema_INT64:      descriptorpb.FieldDescriptorProto_TYPE_INT64,
	storagepb.TableFieldSchema_NUMERIC:    descriptorpb.FieldDescriptorProto_TYPE_BYTES,
	storagepb.TableFieldSchema_STRING:     descriptorpb.FieldDescriptorProto_TYPE_STRING,
	storagepb.TableFieldSchema_STRUCT:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE,
	storagepb.TableFieldSchema_TIME:       descriptorpb.FieldDescriptorProto_TYPE_INT64,
	storagepb.TableFieldSchema_TIMESTAMP:  descriptorpb.FieldDescriptorProto_TYPE_INT64,
}

// Primitive types which can leverage packed encoding when repeated/arrays.
//
// Note: many/most of these aren't used when doing schema to proto conversion, but
// are included for completeness.
var packedTypes = []descriptorpb.FieldDescriptorProto_Type{
	descriptorpb.FieldDescriptorProto_TYPE_INT32,
	descriptorpb.FieldDescriptorProto_TYPE_INT64,
	descriptorpb.FieldDescriptorProto_TYPE_UINT32,
	descriptorpb.FieldDescriptorProto_TYPE_UINT64,
	descriptorpb.FieldDescriptorProto_TYPE_SINT32,
	descriptorpb.FieldDescriptorProto_TYPE_SINT64,
	descriptorpb.FieldDescriptorProto_TYPE_FIXED32,
	descriptorpb.FieldDescriptorProto_TYPE_FIXED64,
	descriptorpb.FieldDescriptorProto_TYPE_SFIXED32,
	descriptorpb.FieldDescriptorProto_TYPE_SFIXED64,
	descriptorpb.FieldDescriptorProto_TYPE_FLOAT,
	descriptorpb.FieldDescriptorProto_TYPE_DOUBLE,
	descriptorpb.FieldDescriptorProto_TYPE_BOOL,
	descriptorpb.FieldDescriptorProto_TYPE_ENUM,
}

// For TableFieldSchema OPTIONAL mode, we use the wrapper types to allow for the
// proper representation of NULL values, as proto3 semantics would just use default value.
var bqTypeToWrapperMap = map[storagepb.TableFieldSchema_Type]string{
	storagepb.TableFieldSchema_BIGNUMERIC: ".google.protobuf.BytesValue",
	storagepb.TableFieldSchema_BOOL:       ".google.protobuf.BoolValue",
	storagepb.TableFieldSchema_BYTES:      ".google.protobuf.BytesValue",
	storagepb.TableFieldSchema_DATE:       ".google.protobuf.Int32Value",
	storagepb.TableFieldSchema_DATETIME:   ".google.protobuf.Int64Value",
	storagepb.TableFieldSchema_DOUBLE:     ".google.protobuf.DoubleValue",
	storagepb.TableFieldSchema_GEOGRAPHY:  ".google.protobuf.StringValue",
	storagepb.TableFieldSchema_INT64:      ".google.protobuf.Int64Value",
	storagepb.TableFieldSchema_NUMERIC:    ".google.protobuf.BytesValue",
	storagepb.TableFieldSchema_STRING:     ".google.protobuf.StringValue",
	storagepb.TableFieldSchema_TIME:       ".google.protobuf.Int64Value",
	storagepb.TableFieldSchema_TIMESTAMP:  ".google.protobuf.Int64Value",
}

// filename used by well known types proto
var wellKnownTypesWrapperName = "google/protobuf/wrappers.proto"

// dependencyCache is used to reduce the number of unique messages we generate by caching based on the tableschema.
//
// Keys are based on the base64-encoded serialized tableschema value.
type dependencyCache map[string]protoreflect.MessageDescriptor

func (dm dependencyCache) get(schema *storagepb.TableSchema) protoreflect.MessageDescriptor {
	if dm == nil {
		return nil
	}
	b, err := proto.Marshal(schema)
	if err != nil {
		return nil
	}
	encoded := base64.StdEncoding.EncodeToString(b)
	if desc, ok := (dm)[encoded]; ok {
		return desc
	}
	return nil
}

func (dm dependencyCache) getFileDescriptorProtos() []*descriptorpb.FileDescriptorProto {
	var fdpList []*descriptorpb.FileDescriptorProto
	for _, d := range dm {
		if fd := d.ParentFile(); fd != nil {
			fdp := protodesc.ToFileDescriptorProto(fd)
			fdpList = append(fdpList, fdp)
		}
	}
	return fdpList
}

func (dm dependencyCache) add(schema *storagepb.TableSchema, descriptor protoreflect.MessageDescriptor) error {
	if dm == nil {
		return fmt.Errorf("cache is nil")
	}
	b, err := proto.Marshal(schema)
	if err != nil {
		return fmt.Errorf("failed to serialize tableschema: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(b)
	(dm)[encoded] = descriptor
	return nil
}

// StorageSchemaToProto2Descriptor builds a protoreflect.Descriptor for a given table schema using proto2 syntax.
func StorageSchemaToProto2Descriptor(inSchema *storagepb.TableSchema, scope string) (protoreflect.Descriptor, error) {
	dc := make(dependencyCache)
	// TODO: b/193064992 tracks support for wrapper types.  In the interim, disable wrapper usage.
	return storageSchemaToDescriptorInternal(inSchema, scope, &dc, false)
}

// StorageSchemaToProto3Descriptor builds a protoreflect.Descriptor for a given table schema using proto3 syntax.
//
// NOTE: Currently the write API doesn't yet support proto3 behaviors (default value, wrapper types, etc), but this is provided for
// completeness.
func StorageSchemaToProto3Descriptor(inSchema *storagepb.TableSchema, scope string) (protoreflect.Descriptor, error) {
	dc := make(dependencyCache)
	return storageSchemaToDescriptorInternal(inSchema, scope, &dc, true)
}

// Internal implementation of the conversion code.
func storageSchemaToDescriptorInternal(inSchema *storagepb.TableSchema, scope string, cache *dependencyCache, useProto3 bool) (protoreflect.MessageDescriptor, error) {
	if inSchema == nil {
		return nil, newConversionError(scope, fmt.Errorf("no input schema was provided"))
	}

	var fields []*descriptorpb.FieldDescriptorProto
	var deps []protoreflect.FileDescriptor
	var fNumber int32

	for _, f := range inSchema.GetFields() {
		fNumber = fNumber + 1
		currentScope := fmt.Sprintf("%s__%s", scope, f.GetName())
		// If we're dealing with a STRUCT type, we must deal with sub messages.
		// As multiple submessages may share the same type definition, we use a dependency cache
		// and interrogate it / populate it as we're going.
		if f.Type == storagepb.TableFieldSchema_STRUCT {
			foundDesc := cache.get(&storagepb.TableSchema{Fields: f.GetFields()})
			if foundDesc != nil {
				// check to see if we already have this in current dependency list
				haveDep := false
				for _, dep := range deps {
					if messageDependsOnFile(foundDesc, dep) {
						haveDep = true
						break
					}
				}
				// If dep is missing, add to current dependencies.
				if !haveDep {
					deps = append(deps, foundDesc.ParentFile())
				}
				// Construct field descriptor for the message.
				fdp, err := tableFieldSchemaToFieldDescriptorProto(f, fNumber, string(foundDesc.FullName()), useProto3)
				if err != nil {
					return nil, newConversionError(scope, fmt.Errorf("couldn't convert field to FieldDescriptorProto: %w", err))
				}
				fields = append(fields, fdp)
			} else {
				// Wrap the current struct's fields in a TableSchema outer message, and then build the submessage.
				ts := &storagepb.TableSchema{
					Fields: f.GetFields(),
				}
				desc, err := storageSchemaToDescriptorInternal(ts, currentScope, cache, useProto3)
				if err != nil {
					return nil, newConversionError(currentScope, fmt.Errorf("couldn't convert message: %w", err))
				}
				// Now that we have the submessage definition, we append it both to the local dependencies, as well
				// as inserting it into the cache for possible reuse elsewhere.
				deps = append(deps, desc.ParentFile())
				err = cache.add(ts, desc)
				if err != nil {
					return nil, newConversionError(currentScope, fmt.Errorf("failed to add descriptor to dependency cache: %w", err))
				}
				fdp, err := tableFieldSchemaToFieldDescriptorProto(f, fNumber, currentScope, useProto3)
				if err != nil {
					return nil, newConversionError(currentScope, fmt.Errorf("couldn't compute field schema : %w", err))
				}
				fields = append(fields, fdp)
			}
		} else {
			fd, err := tableFieldSchemaToFieldDescriptorProto(f, fNumber, currentScope, useProto3)
			if err != nil {
				return nil, newConversionError(currentScope, err)
			}
			fields = append(fields, fd)
		}
	}
	// Start constructing a DescriptorProto.
	dp := &descriptorpb.DescriptorProto{
		Name:  proto.String(scope),
		Field: fields,
	}

	// Use the local dependencies to generate a list of filenames.
	depNames := []string{wellKnownTypesWrapperName}
	for _, d := range deps {
		depNames = append(depNames, d.ParentFile().Path())
	}

	// Now, construct a FileDescriptorProto.
	fdp := &descriptorpb.FileDescriptorProto{
		MessageType: []*descriptorpb.DescriptorProto{dp},
		Name:        proto.String(fmt.Sprintf("%s.proto", scope)),
		Syntax:      proto.String("proto3"),
		Dependency:  depNames,
	}
	if !useProto3 {
		fdp.Syntax = proto.String("proto2")
	}

	// We'll need a FileDescriptorSet as we have a FileDescriptorProto for the current
	// descriptor we're building, but we need to include all the referenced dependencies.

	fdpList := []*descriptorpb.FileDescriptorProto{
		fdp,
		protodesc.ToFileDescriptorProto(wrapperspb.File_google_protobuf_wrappers_proto),
	}
	fdpList = append(fdpList, cache.getFileDescriptorProtos()...)

	fds := &descriptorpb.FileDescriptorSet{
		File: fdpList,
	}

	// Load the set into a registry, then interrogate it for the descriptor corresponding to the top level message.
	files, err := protodesc.NewFiles(fds)
	if err != nil {
		return nil, err
	}
	found, err := files.FindDescriptorByName(protoreflect.FullName(scope))
	if err != nil {
		return nil, err
	}
	return found.(protoreflect.MessageDescriptor), nil
}

// messageDependsOnFile checks if the given message descriptor already belongs to the file descriptor.
// To check for that, first we check if the message descriptor parent file is the same as the file descriptor.
// If not, check if the message descriptor belongs is contained as a child of the file descriptor.
func messageDependsOnFile(msg protoreflect.MessageDescriptor, file protoreflect.FileDescriptor) bool {
	parentFile := msg.ParentFile()
	parentFileName := parentFile.FullName()
	if parentFileName != "" {
		if parentFileName == file.FullName() {
			return true
		}
	}
	fileMessages := file.Messages()
	for i := 0; i < fileMessages.Len(); i++ {
		childMsg := fileMessages.Get(i)
		if msg.FullName() == childMsg.FullName() {
			return true
		}
	}
	return false
}

// tableFieldSchemaToFieldDescriptorProto builds individual field descriptors for a proto message.
//
// For proto3, in cases where the mode is nullable we use the well known wrapper types.
// For proto2, we propagate the mode->label annotation as expected.
//
// Messages are always nullable, and repeated fields are as well.
func tableFieldSchemaToFieldDescriptorProto(field *storagepb.TableFieldSchema, idx int32, scope string, useProto3 bool) (*descriptorpb.FieldDescriptorProto, error) {
	name := field.GetName()
	var fdp *descriptorpb.FieldDescriptorProto

	if field.GetType() == storagepb.TableFieldSchema_STRUCT {
		fdp = &descriptorpb.FieldDescriptorProto{
			Name:     proto.String(name),
			Number:   proto.Int32(idx),
			TypeName: proto.String(scope),
			Label:    convertModeToLabel(field.GetMode(), useProto3),
		}
	} else {
		// For (REQUIRED||REPEATED) fields for proto3, or all cases for proto2, we can use the expected scalar types.
		if field.GetMode() != storagepb.TableFieldSchema_NULLABLE || !useProto3 {
			outType := bqTypeToFieldTypeMap[field.GetType()]
			fdp = &descriptorpb.FieldDescriptorProto{
				Name:   proto.String(name),
				Number: proto.Int32(idx),
				Type:   outType.Enum(),
				Label:  convertModeToLabel(field.GetMode(), useProto3),
			}

			// Special case: proto2 repeated fields may benefit from using packed annotation.
			if field.GetMode() == storagepb.TableFieldSchema_REPEATED && !useProto3 {
				for _, v := range packedTypes {
					if outType == v {
						fdp.Options = &descriptorpb.FieldOptions{
							Packed: proto.Bool(true),
						}
						break
					}
				}
			}
		} else {
			// For NULLABLE proto3 fields, use a wrapper type.
			fdp = &descriptorpb.FieldDescriptorProto{
				Name:     proto.String(name),
				Number:   proto.Int32(idx),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				TypeName: proto.String(bqTypeToWrapperMap[field.GetType()]),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			}
		}
	}
	if nameRequiresAnnotation(name) {
		// Use a prefix + base64 encoded name when annotations bear the actual name.
		// Base 64 standard encoding may also contain certain characters (+,/,=) which
		// we remove from the generated name.
		encoded := strings.Trim(base64.StdEncoding.EncodeToString([]byte(name)), "+/=")
		fdp.Name = proto.String(fmt.Sprintf("col_%s", encoded))
		opts := fdp.GetOptions()
		if opts == nil {
			fdp.Options = &descriptorpb.FieldOptions{}
		}
		proto.SetExtension(fdp.Options, storagepb.E_ColumnName, name)
	}
	return fdp, nil
}

// nameRequiresAnnotation determines whether a field name requires unicode-annotation.
func nameRequiresAnnotation(in string) bool {
	return !protoreflect.Name(in).IsValid()
}

// NormalizeDescriptor builds a self-contained DescriptorProto suitable for communicating schema
// information with the BigQuery Storage write API.  It's primarily used for cases where users are
// interested in sending data using a predefined protocol buffer message.
//
// The storage API accepts a single DescriptorProto for decoding message data.  In many cases, a message
// is comprised of multiple independent messages, from the same .proto file or from multiple sources.  Rather
// than being forced to communicate all these messages independently, what this method does is rewrite the
// DescriptorProto to inline all messages as nested submessages.  As the backend only cares about the types
// and not the namespaces when decoding, this is sufficient for the needs of the API's representation.
//
// In addition to nesting messages, this method also handles some encapsulation of enum types to avoid possible
// conflicts due to ambiguities, and clears oneof indices as oneof isn't a concept that maps into BigQuery
// schemas.
//
// To enable proto3 usage, this function will also rewrite proto3 descriptors into equivalent proto2 form.
// Such rewrites include setting the appropriate default values for proto3 fields.
func NormalizeDescriptor(in protoreflect.MessageDescriptor) (*descriptorpb.DescriptorProto, error) {
	return normalizeDescriptorInternal(in, newStringSet(), newStringSet(), newStringSet(), nil)
}

func normalizeDescriptorInternal(in protoreflect.MessageDescriptor, visitedTypes, enumTypes, structTypes *stringSet, root *descriptorpb.DescriptorProto) (*descriptorpb.DescriptorProto, error) {
	if in == nil {
		return nil, fmt.Errorf("no messagedescriptor provided")
	}
	resultDP := &descriptorpb.DescriptorProto{}
	if root == nil {
		root = resultDP
	}
	fullProtoName := string(in.FullName())
	resultDP.Name = proto.String(normalizeName(fullProtoName))
	visitedTypes.add(fullProtoName)
	for i := 0; i < in.Fields().Len(); i++ {
		inField := in.Fields().Get(i)
		resultFDP := protodesc.ToFieldDescriptorProto(inField)
		// For messages without explicit presence, use default values to match implicit presence behavior.
		if !inField.HasPresence() && inField.Cardinality() != protoreflect.Repeated {
			switch resultFDP.GetType() {
			case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
				resultFDP.DefaultValue = proto.String("false")
			case descriptorpb.FieldDescriptorProto_TYPE_BYTES, descriptorpb.FieldDescriptorProto_TYPE_STRING:
				resultFDP.DefaultValue = proto.String("")
			case descriptorpb.FieldDescriptorProto_TYPE_ENUM:
				// Resolve the proto3 default value.  The default value should be the value name.
				defValue := inField.Enum().Values().ByNumber(inField.Default().Enum())
				resultFDP.DefaultValue = proto.String(string(defValue.Name()))
			case descriptorpb.FieldDescriptorProto_TYPE_DOUBLE,
				descriptorpb.FieldDescriptorProto_TYPE_FLOAT,
				descriptorpb.FieldDescriptorProto_TYPE_INT64,
				descriptorpb.FieldDescriptorProto_TYPE_UINT64,
				descriptorpb.FieldDescriptorProto_TYPE_INT32,
				descriptorpb.FieldDescriptorProto_TYPE_FIXED64,
				descriptorpb.FieldDescriptorProto_TYPE_FIXED32,
				descriptorpb.FieldDescriptorProto_TYPE_UINT32,
				descriptorpb.FieldDescriptorProto_TYPE_SFIXED32,
				descriptorpb.FieldDescriptorProto_TYPE_SFIXED64,
				descriptorpb.FieldDescriptorProto_TYPE_SINT32,
				descriptorpb.FieldDescriptorProto_TYPE_SINT64:
				resultFDP.DefaultValue = proto.String("0")
			}
		}
		// Clear proto3 optional annotation, as the backend converter can
		// treat this as a proto2 optional.
		if resultFDP.Proto3Optional != nil {
			resultFDP.Proto3Optional = nil
		}
		if resultFDP.OneofIndex != nil {
			resultFDP.OneofIndex = nil
		}
		if inField.Kind() == protoreflect.MessageKind || inField.Kind() == protoreflect.GroupKind {
			// Handle fields that reference messages.
			// Groups are a proto2-ism which predated nested messages.
			msgFullName := string(inField.Message().FullName())
			if !skipNormalization(msgFullName) {
				// for everything but well known types, normalize.
				normName := normalizeName(string(msgFullName))
				if structTypes.contains(msgFullName) {
					resultFDP.TypeName = proto.String(normName)
				} else {
					if visitedTypes.contains(msgFullName) {
						return nil, fmt.Errorf("recursive type not supported: %s", inField.FullName())
					}
					visitedTypes.add(msgFullName)
					dp, err := normalizeDescriptorInternal(inField.Message(), visitedTypes, enumTypes, structTypes, root)
					if err != nil {
						return nil, fmt.Errorf("error converting message %s: %v", inField.FullName(), err)
					}
					root.NestedType = append(root.NestedType, dp)
					visitedTypes.delete(msgFullName)
					lastNested := root.GetNestedType()[len(root.GetNestedType())-1].GetName()
					resultFDP.TypeName = proto.String(lastNested)
				}
			}
		}
		if inField.Kind() == protoreflect.EnumKind {
			// For enums, in order to avoid value conflict, we will always define
			// a enclosing struct called enum_full_name_E that includes the actual
			// enum.
			enumFullName := string(inField.Enum().FullName())
			enclosingTypeName := normalizeName(enumFullName) + "_E"
			enumName := string(inField.Enum().Name())
			actualFullName := fmt.Sprintf("%s.%s", enclosingTypeName, enumName)
			if enumTypes.contains(enumFullName) {
				resultFDP.TypeName = proto.String(actualFullName)
			} else {
				enumDP := protodesc.ToEnumDescriptorProto(inField.Enum())
				enumDP.Name = proto.String(enumName)
				// Ensure values in enum are sorted.
				vals := enumDP.GetValue()
				sort.SliceStable(vals, func(i, j int) bool {
					return vals[i].GetNumber() < vals[j].GetNumber()
				})
				// Append wrapped enum to nested types.
				root.NestedType = append(root.NestedType, &descriptorpb.DescriptorProto{
					Name:     proto.String(enclosingTypeName),
					EnumType: []*descriptorpb.EnumDescriptorProto{enumDP},
				})
				resultFDP.TypeName = proto.String(actualFullName)
				enumTypes.add(enumFullName)
			}
		}
		resultDP.Field = append(resultDP.Field, resultFDP)
	}
	// To reduce comparison jitter, order the common slices fields where possible.
	//
	// First, fields are sorted by ID number.
	fields := resultDP.GetField()
	sort.SliceStable(fields, func(i, j int) bool {
		return fields[i].GetNumber() < fields[j].GetNumber()
	})
	// Then, sort nested messages in NestedType by name.
	nested := resultDP.GetNestedType()
	sort.SliceStable(nested, func(i, j int) bool {
		return nested[i].GetName() < nested[j].GetName()
	})
	structTypes.add(fullProtoName)
	return resultDP, nil
}

type stringSet struct {
	m map[string]struct{}
}

func (s *stringSet) contains(k string) bool {
	_, ok := s.m[k]
	return ok
}

func (s *stringSet) add(k string) {
	s.m[k] = struct{}{}
}

func (s *stringSet) delete(k string) {
	delete(s.m, k)
}

func newStringSet() *stringSet {
	return &stringSet{
		m: make(map[string]struct{}),
	}
}

func normalizeName(in string) string {
	return strings.Replace(in, ".", "_", -1)
}

// These types don't get normalized into the fully-contained structure.
var normalizationSkipList = []string{
	/*
		TODO: when backend supports resolving well known types, this list should be enabled.
		"google.protobuf.DoubleValue",
		"google.protobuf.FloatValue",
		"google.protobuf.Int64Value",
		"google.protobuf.UInt64Value",
		"google.protobuf.Int32Value",
		"google.protobuf.Uint32Value",
		"google.protobuf.BoolValue",
		"google.protobuf.StringValue",
		"google.protobuf.BytesValue",
	*/
}

func skipNormalization(fullName string) bool {
	for _, v := range normalizationSkipList {
		if v == fullName {
			return true
		}
	}
	return false
}
