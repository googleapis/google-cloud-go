// Copyright 2025 Google LLC
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
// 	protoc-gen-go v1.35.2
// 	protoc        v4.25.7
// source: google/cloud/bigquery/v2/data_format_options.proto

package bigquerypb

import (
	_ "google.golang.org/genproto/googleapis/api/annotations"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// Options for data format adjustments.
type DataFormatOptions struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Optional. Output timestamp as usec int64. Default is false.
	UseInt64Timestamp bool `protobuf:"varint,1,opt,name=use_int64_timestamp,json=useInt64Timestamp,proto3" json:"use_int64_timestamp,omitempty"`
}

func (x *DataFormatOptions) Reset() {
	*x = DataFormatOptions{}
	mi := &file_google_cloud_bigquery_v2_data_format_options_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DataFormatOptions) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DataFormatOptions) ProtoMessage() {}

func (x *DataFormatOptions) ProtoReflect() protoreflect.Message {
	mi := &file_google_cloud_bigquery_v2_data_format_options_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DataFormatOptions.ProtoReflect.Descriptor instead.
func (*DataFormatOptions) Descriptor() ([]byte, []int) {
	return file_google_cloud_bigquery_v2_data_format_options_proto_rawDescGZIP(), []int{0}
}

func (x *DataFormatOptions) GetUseInt64Timestamp() bool {
	if x != nil {
		return x.UseInt64Timestamp
	}
	return false
}

var File_google_cloud_bigquery_v2_data_format_options_proto protoreflect.FileDescriptor

var file_google_cloud_bigquery_v2_data_format_options_proto_rawDesc = []byte{
	0x0a, 0x32, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2f, 0x62,
	0x69, 0x67, 0x71, 0x75, 0x65, 0x72, 0x79, 0x2f, 0x76, 0x32, 0x2f, 0x64, 0x61, 0x74, 0x61, 0x5f,
	0x66, 0x6f, 0x72, 0x6d, 0x61, 0x74, 0x5f, 0x6f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x12, 0x18, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x63, 0x6c, 0x6f,
	0x75, 0x64, 0x2e, 0x62, 0x69, 0x67, 0x71, 0x75, 0x65, 0x72, 0x79, 0x2e, 0x76, 0x32, 0x1a, 0x1f,
	0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x66, 0x69, 0x65, 0x6c, 0x64,
	0x5f, 0x62, 0x65, 0x68, 0x61, 0x76, 0x69, 0x6f, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22,
	0x48, 0x0a, 0x11, 0x44, 0x61, 0x74, 0x61, 0x46, 0x6f, 0x72, 0x6d, 0x61, 0x74, 0x4f, 0x70, 0x74,
	0x69, 0x6f, 0x6e, 0x73, 0x12, 0x33, 0x0a, 0x13, 0x75, 0x73, 0x65, 0x5f, 0x69, 0x6e, 0x74, 0x36,
	0x34, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x08, 0x42, 0x03, 0xe0, 0x41, 0x01, 0x52, 0x11, 0x75, 0x73, 0x65, 0x49, 0x6e, 0x74, 0x36, 0x34,
	0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x42, 0x73, 0x0a, 0x1c, 0x63, 0x6f, 0x6d,
	0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x62, 0x69,
	0x67, 0x71, 0x75, 0x65, 0x72, 0x79, 0x2e, 0x76, 0x32, 0x42, 0x16, 0x44, 0x61, 0x74, 0x61, 0x46,
	0x6f, 0x72, 0x6d, 0x61, 0x74, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x50, 0x72, 0x6f, 0x74,
	0x6f, 0x5a, 0x3b, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e,
	0x63, 0x6f, 0x6d, 0x2f, 0x67, 0x6f, 0x2f, 0x62, 0x69, 0x67, 0x71, 0x75, 0x65, 0x72, 0x79, 0x2f,
	0x76, 0x32, 0x2f, 0x61, 0x70, 0x69, 0x76, 0x32, 0x2f, 0x62, 0x69, 0x67, 0x71, 0x75, 0x65, 0x72,
	0x79, 0x70, 0x62, 0x3b, 0x62, 0x69, 0x67, 0x71, 0x75, 0x65, 0x72, 0x79, 0x70, 0x62, 0x62, 0x06,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_google_cloud_bigquery_v2_data_format_options_proto_rawDescOnce sync.Once
	file_google_cloud_bigquery_v2_data_format_options_proto_rawDescData = file_google_cloud_bigquery_v2_data_format_options_proto_rawDesc
)

func file_google_cloud_bigquery_v2_data_format_options_proto_rawDescGZIP() []byte {
	file_google_cloud_bigquery_v2_data_format_options_proto_rawDescOnce.Do(func() {
		file_google_cloud_bigquery_v2_data_format_options_proto_rawDescData = protoimpl.X.CompressGZIP(file_google_cloud_bigquery_v2_data_format_options_proto_rawDescData)
	})
	return file_google_cloud_bigquery_v2_data_format_options_proto_rawDescData
}

var file_google_cloud_bigquery_v2_data_format_options_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_google_cloud_bigquery_v2_data_format_options_proto_goTypes = []any{
	(*DataFormatOptions)(nil), // 0: google.cloud.bigquery.v2.DataFormatOptions
}
var file_google_cloud_bigquery_v2_data_format_options_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_google_cloud_bigquery_v2_data_format_options_proto_init() }
func file_google_cloud_bigquery_v2_data_format_options_proto_init() {
	if File_google_cloud_bigquery_v2_data_format_options_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_google_cloud_bigquery_v2_data_format_options_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_google_cloud_bigquery_v2_data_format_options_proto_goTypes,
		DependencyIndexes: file_google_cloud_bigquery_v2_data_format_options_proto_depIdxs,
		MessageInfos:      file_google_cloud_bigquery_v2_data_format_options_proto_msgTypes,
	}.Build()
	File_google_cloud_bigquery_v2_data_format_options_proto = out.File
	file_google_cloud_bigquery_v2_data_format_options_proto_rawDesc = nil
	file_google_cloud_bigquery_v2_data_format_options_proto_goTypes = nil
	file_google_cloud_bigquery_v2_data_format_options_proto_depIdxs = nil
}
