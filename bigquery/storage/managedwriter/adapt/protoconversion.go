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
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta2"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Allows conversion between BQ schema mode and FieldDescriptorProto's Label type.
var bqModeToFieldLabelMap = map[storagepb.TableFieldSchema_Mode]descriptorpb.FieldDescriptorProto_Label{
	storagepb.TableFieldSchema_NULLABLE: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL,
	storagepb.TableFieldSchema_REPEATED: descriptor.FieldDescriptorProto_LABEL_REPEATED,
	storagepb.TableFieldSchema_REQUIRED: descriptor.FieldDescriptorProto_LABEL_REQUIRED,
}

// Allows conversion between BQ schema type and FieldDescriptorProto's type.
// TODO: Should we instead map to wrapperspb type references to allow for nulls?
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

type protoDependencies map[string]*descriptorpb.FileDescriptorProto

func (pd *protoDependencies) getIt() protoreflect.FileDescriptor {
	return nil
}

// StorageSchemaToFileDescriptorSet builds a Descriptor for a given table schema.
func StorageSchemaToFileDescriptorSet(inSchema *storagepb.TableSchema, scope string, dependencies protoDependencies) (protoreflect.Descriptor, error) {
	if inSchema == nil {
		return nil, fmt.Errorf("no input schema provided")
	}

	var fields []*descriptorpb.FieldDescriptorProto
	var fNumber int32

	for _, f := range inSchema.GetFields() {
		fNumber = fNumber + 1
		currentScope := fmt.Sprintf("%s__%s", scope, f.GetName())
		if f.Type == storagepb.TableFieldSchema_STRUCT {
			// check dependency map to see if we've already built a suitable FileDescriptor

			if dep := dependencies.getIt(); dep != nil {
				// we've already built an appropriate FileDescriptor, use it.
			} else {
				ts := &storagepb.TableSchema{
					Fields: f.GetFields(),
				}
				_, err := StorageSchemaToFileDescriptorSet(ts, currentScope, dependencies)
				if err != nil {
					return nil, fmt.Errorf("couldn't compile (%s): %v", currentScope, err)
				}
				// TODO: add desc to dependencies
				fdp, err := tableFieldSchemaToFieldDescriptorProto(f, fNumber, currentScope)
				if err != nil {
					return nil, fmt.Errorf("couldn't compute field schema (%s): %v", currentScope, err)
				}
				fields = append(fields, fdp)
			}
		} else {
			fd, err := tableFieldSchemaToFieldDescriptorProto(f, fNumber, currentScope)
			if err != nil {
				return nil, err
			}
			fields = append(fields, fd)
		}
	}
	dp := &descriptorpb.DescriptorProto{
		Name:  &scope,
		Field: fields,
	}
	fdp := &descriptorpb.FileDescriptorProto{
		MessageType: []*descriptor.DescriptorProto{dp},
		Syntax:      proto.String("proto2"),
		Name:        proto.String(fmt.Sprintf("%s.proto", scope)),
	}

	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptor.FileDescriptorProto{fdp},
	}
	/*
		for _, d := range dependencies {
			// add dependencies into the filedescriptorset
		}
	*/

	files, err := protodesc.NewFiles(fds)
	if err != nil {
		return nil, fmt.Errorf("protodesc.NewFiles: %v", err)
	}
	return files.FindDescriptorByName(protoreflect.FullName(scope))
}

func tableFieldSchemaToFieldDescriptorProto(field *storagepb.TableFieldSchema, idx int32, scope string) (*descriptorpb.FieldDescriptorProto, error) {
	name := strings.ToLower(field.GetName())
	if field.GetType() == storagepb.TableFieldSchema_STRUCT {
		return &descriptorpb.FieldDescriptorProto{
			Name:     proto.String(name),
			Number:   proto.Int32(idx),
			TypeName: proto.String(scope),
			Label:    bqModeToFieldLabelMap[field.GetMode()].Enum(),
		}, nil
	}
	return &descriptorpb.FieldDescriptorProto{
		Name:   &name,
		Number: &idx,
		Type:   bqTypeToFieldTypeMap[field.GetType()].Enum(),
		Label:  bqModeToFieldLabelMap[field.GetMode()].Enum(),
	}, nil
}
