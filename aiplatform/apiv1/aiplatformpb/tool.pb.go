// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.31.0
// 	protoc        v4.23.2
// source: google/cloud/aiplatform/v1/tool.proto

package aiplatformpb

import (
	_ "google.golang.org/genproto/googleapis/api/annotations"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	structpb "google.golang.org/protobuf/types/known/structpb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// Tool details that the model may use to generate response.
//
// A `Tool` is a piece of code that enables the system to interact with
// external systems to perform an action, or set of actions, outside of
// knowledge and scope of the model.
type Tool struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Optional. One or more function declarations to be passed to the model along
	// with the current user query. Model may decide to call a subset of these
	// functions by populating [FunctionCall][content.part.function_call] in the
	// response. User should provide a
	// [FunctionResponse][content.part.function_response] for each function call
	// in the next turn. Based on the function responses, Model will generate the
	// final response back to the user. Maximum 64 function declarations can be
	// provided.
	FunctionDeclarations []*FunctionDeclaration `protobuf:"bytes,1,rep,name=function_declarations,json=functionDeclarations,proto3" json:"function_declarations,omitempty"`
}

func (x *Tool) Reset() {
	*x = Tool{}
	if protoimpl.UnsafeEnabled {
		mi := &file_google_cloud_aiplatform_v1_tool_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Tool) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Tool) ProtoMessage() {}

func (x *Tool) ProtoReflect() protoreflect.Message {
	mi := &file_google_cloud_aiplatform_v1_tool_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Tool.ProtoReflect.Descriptor instead.
func (*Tool) Descriptor() ([]byte, []int) {
	return file_google_cloud_aiplatform_v1_tool_proto_rawDescGZIP(), []int{0}
}

func (x *Tool) GetFunctionDeclarations() []*FunctionDeclaration {
	if x != nil {
		return x.FunctionDeclarations
	}
	return nil
}

// Structured representation of a function declaration as defined by the
// [OpenAPI 3.0 specification](https://spec.openapis.org/oas/v3.0.3). Included
// in this declaration are the function name and parameters. This
// FunctionDeclaration is a representation of a block of code that can be used
// as a `Tool` by the model and executed by the client.
type FunctionDeclaration struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Required. The name of the function to call.
	// Must start with a letter or an underscore.
	// Must be a-z, A-Z, 0-9, or contain underscores and dashes, with a maximum
	// length of 64.
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Optional. Description and purpose of the function.
	// Model uses it to decide how and whether to call the function.
	Description string `protobuf:"bytes,2,opt,name=description,proto3" json:"description,omitempty"`
	// Optional. Describes the parameters to this function in JSON Schema Object
	// format. Reflects the Open API 3.03 Parameter Object. string Key: the name
	// of the parameter. Parameter names are case sensitive. Schema Value: the
	// Schema defining the type used for the parameter. For function with no
	// parameters, this can be left unset. Example with 1 required and 1 optional
	// parameter: type: OBJECT properties:
	//  param1:
	//    type: STRING
	//  param2:
	//    type: INTEGER
	// required:
	//  - param1
	Parameters *Schema `protobuf:"bytes,3,opt,name=parameters,proto3" json:"parameters,omitempty"`
}

func (x *FunctionDeclaration) Reset() {
	*x = FunctionDeclaration{}
	if protoimpl.UnsafeEnabled {
		mi := &file_google_cloud_aiplatform_v1_tool_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FunctionDeclaration) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FunctionDeclaration) ProtoMessage() {}

func (x *FunctionDeclaration) ProtoReflect() protoreflect.Message {
	mi := &file_google_cloud_aiplatform_v1_tool_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FunctionDeclaration.ProtoReflect.Descriptor instead.
func (*FunctionDeclaration) Descriptor() ([]byte, []int) {
	return file_google_cloud_aiplatform_v1_tool_proto_rawDescGZIP(), []int{1}
}

func (x *FunctionDeclaration) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *FunctionDeclaration) GetDescription() string {
	if x != nil {
		return x.Description
	}
	return ""
}

func (x *FunctionDeclaration) GetParameters() *Schema {
	if x != nil {
		return x.Parameters
	}
	return nil
}

// A predicted [FunctionCall] returned from the model that contains a string
// representing the [FunctionDeclaration.name] and a structured JSON object
// containing the parameters and their values.
type FunctionCall struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Required. The name of the function to call.
	// Matches [FunctionDeclaration.name].
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Optional. Required. The function parameters and values in JSON object
	// format. See [FunctionDeclaration.parameters] for parameter details.
	Args *structpb.Struct `protobuf:"bytes,2,opt,name=args,proto3" json:"args,omitempty"`
}

func (x *FunctionCall) Reset() {
	*x = FunctionCall{}
	if protoimpl.UnsafeEnabled {
		mi := &file_google_cloud_aiplatform_v1_tool_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FunctionCall) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FunctionCall) ProtoMessage() {}

func (x *FunctionCall) ProtoReflect() protoreflect.Message {
	mi := &file_google_cloud_aiplatform_v1_tool_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FunctionCall.ProtoReflect.Descriptor instead.
func (*FunctionCall) Descriptor() ([]byte, []int) {
	return file_google_cloud_aiplatform_v1_tool_proto_rawDescGZIP(), []int{2}
}

func (x *FunctionCall) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *FunctionCall) GetArgs() *structpb.Struct {
	if x != nil {
		return x.Args
	}
	return nil
}

// The result output from a [FunctionCall] that contains a string representing
// the [FunctionDeclaration.name] and a structured JSON object containing any
// output from the function is used as context to the model. This should contain
// the result of a [FunctionCall] made based on model prediction.
type FunctionResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Required. The name of the function to call.
	// Matches [FunctionDeclaration.name] and [FunctionCall.name].
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Required. The function response in JSON object format.
	Response *structpb.Struct `protobuf:"bytes,2,opt,name=response,proto3" json:"response,omitempty"`
}

func (x *FunctionResponse) Reset() {
	*x = FunctionResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_google_cloud_aiplatform_v1_tool_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FunctionResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FunctionResponse) ProtoMessage() {}

func (x *FunctionResponse) ProtoReflect() protoreflect.Message {
	mi := &file_google_cloud_aiplatform_v1_tool_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FunctionResponse.ProtoReflect.Descriptor instead.
func (*FunctionResponse) Descriptor() ([]byte, []int) {
	return file_google_cloud_aiplatform_v1_tool_proto_rawDescGZIP(), []int{3}
}

func (x *FunctionResponse) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *FunctionResponse) GetResponse() *structpb.Struct {
	if x != nil {
		return x.Response
	}
	return nil
}

var File_google_cloud_aiplatform_v1_tool_proto protoreflect.FileDescriptor

var file_google_cloud_aiplatform_v1_tool_proto_rawDesc = []byte{
	0x0a, 0x25, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2f, 0x61,
	0x69, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x2f, 0x76, 0x31, 0x2f, 0x74, 0x6f, 0x6f,
	0x6c, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x1a, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e,
	0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x61, 0x69, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d,
	0x2e, 0x76, 0x31, 0x1a, 0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f,
	0x66, 0x69, 0x65, 0x6c, 0x64, 0x5f, 0x62, 0x65, 0x68, 0x61, 0x76, 0x69, 0x6f, 0x72, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x28, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x63, 0x6c, 0x6f,
	0x75, 0x64, 0x2f, 0x61, 0x69, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x2f, 0x76, 0x31,
	0x2f, 0x6f, 0x70, 0x65, 0x6e, 0x61, 0x70, 0x69, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1c,
	0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f,
	0x73, 0x74, 0x72, 0x75, 0x63, 0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x71, 0x0a, 0x04,
	0x54, 0x6f, 0x6f, 0x6c, 0x12, 0x69, 0x0a, 0x15, 0x66, 0x75, 0x6e, 0x63, 0x74, 0x69, 0x6f, 0x6e,
	0x5f, 0x64, 0x65, 0x63, 0x6c, 0x61, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0x01, 0x20,
	0x03, 0x28, 0x0b, 0x32, 0x2f, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x63, 0x6c, 0x6f,
	0x75, 0x64, 0x2e, 0x61, 0x69, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x2e, 0x76, 0x31,
	0x2e, 0x46, 0x75, 0x6e, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x44, 0x65, 0x63, 0x6c, 0x61, 0x72, 0x61,
	0x74, 0x69, 0x6f, 0x6e, 0x42, 0x03, 0xe0, 0x41, 0x01, 0x52, 0x14, 0x66, 0x75, 0x6e, 0x63, 0x74,
	0x69, 0x6f, 0x6e, 0x44, 0x65, 0x63, 0x6c, 0x61, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x22,
	0x9e, 0x01, 0x0a, 0x13, 0x46, 0x75, 0x6e, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x44, 0x65, 0x63, 0x6c,
	0x61, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x17, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x42, 0x03, 0xe0, 0x41, 0x02, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65,
	0x12, 0x25, 0x0a, 0x0b, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x09, 0x42, 0x03, 0xe0, 0x41, 0x01, 0x52, 0x0b, 0x64, 0x65, 0x73, 0x63,
	0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x47, 0x0a, 0x0a, 0x70, 0x61, 0x72, 0x61, 0x6d,
	0x65, 0x74, 0x65, 0x72, 0x73, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x22, 0x2e, 0x67, 0x6f,
	0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x61, 0x69, 0x70, 0x6c, 0x61,
	0x74, 0x66, 0x6f, 0x72, 0x6d, 0x2e, 0x76, 0x31, 0x2e, 0x53, 0x63, 0x68, 0x65, 0x6d, 0x61, 0x42,
	0x03, 0xe0, 0x41, 0x01, 0x52, 0x0a, 0x70, 0x61, 0x72, 0x61, 0x6d, 0x65, 0x74, 0x65, 0x72, 0x73,
	0x22, 0x59, 0x0a, 0x0c, 0x46, 0x75, 0x6e, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x43, 0x61, 0x6c, 0x6c,
	0x12, 0x17, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x42, 0x03,
	0xe0, 0x41, 0x02, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x30, 0x0a, 0x04, 0x61, 0x72, 0x67,
	0x73, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x17, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x53, 0x74, 0x72, 0x75, 0x63, 0x74,
	0x42, 0x03, 0xe0, 0x41, 0x01, 0x52, 0x04, 0x61, 0x72, 0x67, 0x73, 0x22, 0x65, 0x0a, 0x10, 0x46,
	0x75, 0x6e, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12,
	0x17, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x42, 0x03, 0xe0,
	0x41, 0x02, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x38, 0x0a, 0x08, 0x72, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x17, 0x2e, 0x67, 0x6f, 0x6f,
	0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x53, 0x74, 0x72,
	0x75, 0x63, 0x74, 0x42, 0x03, 0xe0, 0x41, 0x02, 0x52, 0x08, 0x72, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x42, 0xc7, 0x01, 0x0a, 0x1e, 0x63, 0x6f, 0x6d, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c,
	0x65, 0x2e, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x61, 0x69, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f,
	0x72, 0x6d, 0x2e, 0x76, 0x31, 0x42, 0x09, 0x54, 0x6f, 0x6f, 0x6c, 0x50, 0x72, 0x6f, 0x74, 0x6f,
	0x50, 0x01, 0x5a, 0x3e, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x67, 0x6f, 0x2f, 0x61, 0x69, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f,
	0x72, 0x6d, 0x2f, 0x61, 0x70, 0x69, 0x76, 0x31, 0x2f, 0x61, 0x69, 0x70, 0x6c, 0x61, 0x74, 0x66,
	0x6f, 0x72, 0x6d, 0x70, 0x62, 0x3b, 0x61, 0x69, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d,
	0x70, 0x62, 0xaa, 0x02, 0x1a, 0x47, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x43, 0x6c, 0x6f, 0x75,
	0x64, 0x2e, 0x41, 0x49, 0x50, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x2e, 0x56, 0x31, 0xca,
	0x02, 0x1a, 0x47, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x5c, 0x43, 0x6c, 0x6f, 0x75, 0x64, 0x5c, 0x41,
	0x49, 0x50, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x5c, 0x56, 0x31, 0xea, 0x02, 0x1d, 0x47,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x3a, 0x3a, 0x43, 0x6c, 0x6f, 0x75, 0x64, 0x3a, 0x3a, 0x41, 0x49,
	0x50, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x3a, 0x3a, 0x56, 0x31, 0x62, 0x06, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_google_cloud_aiplatform_v1_tool_proto_rawDescOnce sync.Once
	file_google_cloud_aiplatform_v1_tool_proto_rawDescData = file_google_cloud_aiplatform_v1_tool_proto_rawDesc
)

func file_google_cloud_aiplatform_v1_tool_proto_rawDescGZIP() []byte {
	file_google_cloud_aiplatform_v1_tool_proto_rawDescOnce.Do(func() {
		file_google_cloud_aiplatform_v1_tool_proto_rawDescData = protoimpl.X.CompressGZIP(file_google_cloud_aiplatform_v1_tool_proto_rawDescData)
	})
	return file_google_cloud_aiplatform_v1_tool_proto_rawDescData
}

var file_google_cloud_aiplatform_v1_tool_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_google_cloud_aiplatform_v1_tool_proto_goTypes = []interface{}{
	(*Tool)(nil),                // 0: google.cloud.aiplatform.v1.Tool
	(*FunctionDeclaration)(nil), // 1: google.cloud.aiplatform.v1.FunctionDeclaration
	(*FunctionCall)(nil),        // 2: google.cloud.aiplatform.v1.FunctionCall
	(*FunctionResponse)(nil),    // 3: google.cloud.aiplatform.v1.FunctionResponse
	(*Schema)(nil),              // 4: google.cloud.aiplatform.v1.Schema
	(*structpb.Struct)(nil),     // 5: google.protobuf.Struct
}
var file_google_cloud_aiplatform_v1_tool_proto_depIdxs = []int32{
	1, // 0: google.cloud.aiplatform.v1.Tool.function_declarations:type_name -> google.cloud.aiplatform.v1.FunctionDeclaration
	4, // 1: google.cloud.aiplatform.v1.FunctionDeclaration.parameters:type_name -> google.cloud.aiplatform.v1.Schema
	5, // 2: google.cloud.aiplatform.v1.FunctionCall.args:type_name -> google.protobuf.Struct
	5, // 3: google.cloud.aiplatform.v1.FunctionResponse.response:type_name -> google.protobuf.Struct
	4, // [4:4] is the sub-list for method output_type
	4, // [4:4] is the sub-list for method input_type
	4, // [4:4] is the sub-list for extension type_name
	4, // [4:4] is the sub-list for extension extendee
	0, // [0:4] is the sub-list for field type_name
}

func init() { file_google_cloud_aiplatform_v1_tool_proto_init() }
func file_google_cloud_aiplatform_v1_tool_proto_init() {
	if File_google_cloud_aiplatform_v1_tool_proto != nil {
		return
	}
	file_google_cloud_aiplatform_v1_openapi_proto_init()
	if !protoimpl.UnsafeEnabled {
		file_google_cloud_aiplatform_v1_tool_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Tool); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_google_cloud_aiplatform_v1_tool_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*FunctionDeclaration); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_google_cloud_aiplatform_v1_tool_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*FunctionCall); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_google_cloud_aiplatform_v1_tool_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*FunctionResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_google_cloud_aiplatform_v1_tool_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_google_cloud_aiplatform_v1_tool_proto_goTypes,
		DependencyIndexes: file_google_cloud_aiplatform_v1_tool_proto_depIdxs,
		MessageInfos:      file_google_cloud_aiplatform_v1_tool_proto_msgTypes,
	}.Build()
	File_google_cloud_aiplatform_v1_tool_proto = out.File
	file_google_cloud_aiplatform_v1_tool_proto_rawDesc = nil
	file_google_cloud_aiplatform_v1_tool_proto_goTypes = nil
	file_google_cloud_aiplatform_v1_tool_proto_depIdxs = nil
}