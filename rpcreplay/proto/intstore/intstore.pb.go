// Copyright 2017 Google LLC
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

// IntStore is a service for testing the rpcreplay package.
// It is a simple key-value store for integers.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.25.0-devel
// 	protoc        v3.12.2
// source: intstore.proto

package intstore

import (
	context "context"
	reflect "reflect"
	sync "sync"

	proto "github.com/golang/protobuf/proto"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// This is a compile-time assertion that a sufficiently up-to-date version
// of the legacy proto package is being used.
const _ = proto.ProtoPackageIsVersion4

type Item struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name  string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Value int32  `protobuf:"varint,2,opt,name=value,proto3" json:"value,omitempty"`
}

func (x *Item) Reset() {
	*x = Item{}
	if protoimpl.UnsafeEnabled {
		mi := &file_intstore_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Item) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Item) ProtoMessage() {}

func (x *Item) ProtoReflect() protoreflect.Message {
	mi := &file_intstore_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Item.ProtoReflect.Descriptor instead.
func (*Item) Descriptor() ([]byte, []int) {
	return file_intstore_proto_rawDescGZIP(), []int{0}
}

func (x *Item) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Item) GetValue() int32 {
	if x != nil {
		return x.Value
	}
	return 0
}

type SetResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	PrevValue int32 `protobuf:"varint,1,opt,name=prev_value,json=prevValue,proto3" json:"prev_value,omitempty"`
}

func (x *SetResponse) Reset() {
	*x = SetResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_intstore_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SetResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SetResponse) ProtoMessage() {}

func (x *SetResponse) ProtoReflect() protoreflect.Message {
	mi := &file_intstore_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SetResponse.ProtoReflect.Descriptor instead.
func (*SetResponse) Descriptor() ([]byte, []int) {
	return file_intstore_proto_rawDescGZIP(), []int{1}
}

func (x *SetResponse) GetPrevValue() int32 {
	if x != nil {
		return x.PrevValue
	}
	return 0
}

type GetRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
}

func (x *GetRequest) Reset() {
	*x = GetRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_intstore_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetRequest) ProtoMessage() {}

func (x *GetRequest) ProtoReflect() protoreflect.Message {
	mi := &file_intstore_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetRequest.ProtoReflect.Descriptor instead.
func (*GetRequest) Descriptor() ([]byte, []int) {
	return file_intstore_proto_rawDescGZIP(), []int{2}
}

func (x *GetRequest) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

type Summary struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Count int32 `protobuf:"varint,1,opt,name=count,proto3" json:"count,omitempty"`
}

func (x *Summary) Reset() {
	*x = Summary{}
	if protoimpl.UnsafeEnabled {
		mi := &file_intstore_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Summary) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Summary) ProtoMessage() {}

func (x *Summary) ProtoReflect() protoreflect.Message {
	mi := &file_intstore_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Summary.ProtoReflect.Descriptor instead.
func (*Summary) Descriptor() ([]byte, []int) {
	return file_intstore_proto_rawDescGZIP(), []int{3}
}

func (x *Summary) GetCount() int32 {
	if x != nil {
		return x.Count
	}
	return 0
}

type ListItemsRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Only list items whose value is greater than this.
	GreaterThan int32 `protobuf:"varint,1,opt,name=greaterThan,proto3" json:"greaterThan,omitempty"`
}

func (x *ListItemsRequest) Reset() {
	*x = ListItemsRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_intstore_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ListItemsRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ListItemsRequest) ProtoMessage() {}

func (x *ListItemsRequest) ProtoReflect() protoreflect.Message {
	mi := &file_intstore_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ListItemsRequest.ProtoReflect.Descriptor instead.
func (*ListItemsRequest) Descriptor() ([]byte, []int) {
	return file_intstore_proto_rawDescGZIP(), []int{4}
}

func (x *ListItemsRequest) GetGreaterThan() int32 {
	if x != nil {
		return x.GreaterThan
	}
	return 0
}

var File_intstore_proto protoreflect.FileDescriptor

var file_intstore_proto_rawDesc = []byte{
	0x0a, 0x0e, 0x69, 0x6e, 0x74, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x12, 0x08, 0x69, 0x6e, 0x74, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x22, 0x30, 0x0a, 0x04, 0x49, 0x74,
	0x65, 0x6d, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x05, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x22, 0x2c, 0x0a, 0x0b,
	0x53, 0x65, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x1d, 0x0a, 0x0a, 0x70,
	0x72, 0x65, 0x76, 0x5f, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x05, 0x52,
	0x09, 0x70, 0x72, 0x65, 0x76, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x22, 0x20, 0x0a, 0x0a, 0x47, 0x65,
	0x74, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x22, 0x1f, 0x0a, 0x07,
	0x53, 0x75, 0x6d, 0x6d, 0x61, 0x72, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x63, 0x6f, 0x75, 0x6e, 0x74,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x05, 0x52, 0x05, 0x63, 0x6f, 0x75, 0x6e, 0x74, 0x22, 0x34, 0x0a,
	0x10, 0x4c, 0x69, 0x73, 0x74, 0x49, 0x74, 0x65, 0x6d, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x12, 0x20, 0x0a, 0x0b, 0x67, 0x72, 0x65, 0x61, 0x74, 0x65, 0x72, 0x54, 0x68, 0x61, 0x6e,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x05, 0x52, 0x0b, 0x67, 0x72, 0x65, 0x61, 0x74, 0x65, 0x72, 0x54,
	0x68, 0x61, 0x6e, 0x32, 0x8e, 0x02, 0x0a, 0x08, 0x49, 0x6e, 0x74, 0x53, 0x74, 0x6f, 0x72, 0x65,
	0x12, 0x2e, 0x0a, 0x03, 0x53, 0x65, 0x74, 0x12, 0x0e, 0x2e, 0x69, 0x6e, 0x74, 0x73, 0x74, 0x6f,
	0x72, 0x65, 0x2e, 0x49, 0x74, 0x65, 0x6d, 0x1a, 0x15, 0x2e, 0x69, 0x6e, 0x74, 0x73, 0x74, 0x6f,
	0x72, 0x65, 0x2e, 0x53, 0x65, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00,
	0x12, 0x2d, 0x0a, 0x03, 0x47, 0x65, 0x74, 0x12, 0x14, 0x2e, 0x69, 0x6e, 0x74, 0x73, 0x74, 0x6f,
	0x72, 0x65, 0x2e, 0x47, 0x65, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0e, 0x2e,
	0x69, 0x6e, 0x74, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x2e, 0x49, 0x74, 0x65, 0x6d, 0x22, 0x00, 0x12,
	0x3b, 0x0a, 0x09, 0x4c, 0x69, 0x73, 0x74, 0x49, 0x74, 0x65, 0x6d, 0x73, 0x12, 0x1a, 0x2e, 0x69,
	0x6e, 0x74, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x2e, 0x4c, 0x69, 0x73, 0x74, 0x49, 0x74, 0x65, 0x6d,
	0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0e, 0x2e, 0x69, 0x6e, 0x74, 0x73, 0x74,
	0x6f, 0x72, 0x65, 0x2e, 0x49, 0x74, 0x65, 0x6d, 0x22, 0x00, 0x30, 0x01, 0x12, 0x32, 0x0a, 0x09,
	0x53, 0x65, 0x74, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x12, 0x0e, 0x2e, 0x69, 0x6e, 0x74, 0x73,
	0x74, 0x6f, 0x72, 0x65, 0x2e, 0x49, 0x74, 0x65, 0x6d, 0x1a, 0x11, 0x2e, 0x69, 0x6e, 0x74, 0x73,
	0x74, 0x6f, 0x72, 0x65, 0x2e, 0x53, 0x75, 0x6d, 0x6d, 0x61, 0x72, 0x79, 0x22, 0x00, 0x28, 0x01,
	0x12, 0x32, 0x0a, 0x0a, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x43, 0x68, 0x61, 0x74, 0x12, 0x0e,
	0x2e, 0x69, 0x6e, 0x74, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x2e, 0x49, 0x74, 0x65, 0x6d, 0x1a, 0x0e,
	0x2e, 0x69, 0x6e, 0x74, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x2e, 0x49, 0x74, 0x65, 0x6d, 0x22, 0x00,
	0x28, 0x01, 0x30, 0x01, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_intstore_proto_rawDescOnce sync.Once
	file_intstore_proto_rawDescData = file_intstore_proto_rawDesc
)

func file_intstore_proto_rawDescGZIP() []byte {
	file_intstore_proto_rawDescOnce.Do(func() {
		file_intstore_proto_rawDescData = protoimpl.X.CompressGZIP(file_intstore_proto_rawDescData)
	})
	return file_intstore_proto_rawDescData
}

var file_intstore_proto_msgTypes = make([]protoimpl.MessageInfo, 5)
var file_intstore_proto_goTypes = []interface{}{
	(*Item)(nil),             // 0: intstore.Item
	(*SetResponse)(nil),      // 1: intstore.SetResponse
	(*GetRequest)(nil),       // 2: intstore.GetRequest
	(*Summary)(nil),          // 3: intstore.Summary
	(*ListItemsRequest)(nil), // 4: intstore.ListItemsRequest
}
var file_intstore_proto_depIdxs = []int32{
	0, // 0: intstore.IntStore.Set:input_type -> intstore.Item
	2, // 1: intstore.IntStore.Get:input_type -> intstore.GetRequest
	4, // 2: intstore.IntStore.ListItems:input_type -> intstore.ListItemsRequest
	0, // 3: intstore.IntStore.SetStream:input_type -> intstore.Item
	0, // 4: intstore.IntStore.StreamChat:input_type -> intstore.Item
	1, // 5: intstore.IntStore.Set:output_type -> intstore.SetResponse
	0, // 6: intstore.IntStore.Get:output_type -> intstore.Item
	0, // 7: intstore.IntStore.ListItems:output_type -> intstore.Item
	3, // 8: intstore.IntStore.SetStream:output_type -> intstore.Summary
	0, // 9: intstore.IntStore.StreamChat:output_type -> intstore.Item
	5, // [5:10] is the sub-list for method output_type
	0, // [0:5] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_intstore_proto_init() }
func file_intstore_proto_init() {
	if File_intstore_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_intstore_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Item); i {
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
		file_intstore_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SetResponse); i {
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
		file_intstore_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetRequest); i {
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
		file_intstore_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Summary); i {
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
		file_intstore_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ListItemsRequest); i {
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
			RawDescriptor: file_intstore_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   5,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_intstore_proto_goTypes,
		DependencyIndexes: file_intstore_proto_depIdxs,
		MessageInfos:      file_intstore_proto_msgTypes,
	}.Build()
	File_intstore_proto = out.File
	file_intstore_proto_rawDesc = nil
	file_intstore_proto_goTypes = nil
	file_intstore_proto_depIdxs = nil
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConnInterface

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion6

// IntStoreClient is the client API for IntStore service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type IntStoreClient interface {
	Set(ctx context.Context, in *Item, opts ...grpc.CallOption) (*SetResponse, error)
	Get(ctx context.Context, in *GetRequest, opts ...grpc.CallOption) (*Item, error)
	// A server-to-client streaming RPC.
	ListItems(ctx context.Context, in *ListItemsRequest, opts ...grpc.CallOption) (IntStore_ListItemsClient, error)
	// A client-to-server streaming RPC.
	SetStream(ctx context.Context, opts ...grpc.CallOption) (IntStore_SetStreamClient, error)
	// A Bidirectional streaming RPC.
	StreamChat(ctx context.Context, opts ...grpc.CallOption) (IntStore_StreamChatClient, error)
}

type intStoreClient struct {
	cc grpc.ClientConnInterface
}

func NewIntStoreClient(cc grpc.ClientConnInterface) IntStoreClient {
	return &intStoreClient{cc}
}

func (c *intStoreClient) Set(ctx context.Context, in *Item, opts ...grpc.CallOption) (*SetResponse, error) {
	out := new(SetResponse)
	err := c.cc.Invoke(ctx, "/intstore.IntStore/Set", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *intStoreClient) Get(ctx context.Context, in *GetRequest, opts ...grpc.CallOption) (*Item, error) {
	out := new(Item)
	err := c.cc.Invoke(ctx, "/intstore.IntStore/Get", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *intStoreClient) ListItems(ctx context.Context, in *ListItemsRequest, opts ...grpc.CallOption) (IntStore_ListItemsClient, error) {
	stream, err := c.cc.NewStream(ctx, &_IntStore_serviceDesc.Streams[0], "/intstore.IntStore/ListItems", opts...)
	if err != nil {
		return nil, err
	}
	x := &intStoreListItemsClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type IntStore_ListItemsClient interface {
	Recv() (*Item, error)
	grpc.ClientStream
}

type intStoreListItemsClient struct {
	grpc.ClientStream
}

func (x *intStoreListItemsClient) Recv() (*Item, error) {
	m := new(Item)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *intStoreClient) SetStream(ctx context.Context, opts ...grpc.CallOption) (IntStore_SetStreamClient, error) {
	stream, err := c.cc.NewStream(ctx, &_IntStore_serviceDesc.Streams[1], "/intstore.IntStore/SetStream", opts...)
	if err != nil {
		return nil, err
	}
	x := &intStoreSetStreamClient{stream}
	return x, nil
}

type IntStore_SetStreamClient interface {
	Send(*Item) error
	CloseAndRecv() (*Summary, error)
	grpc.ClientStream
}

type intStoreSetStreamClient struct {
	grpc.ClientStream
}

func (x *intStoreSetStreamClient) Send(m *Item) error {
	return x.ClientStream.SendMsg(m)
}

func (x *intStoreSetStreamClient) CloseAndRecv() (*Summary, error) {
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	m := new(Summary)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *intStoreClient) StreamChat(ctx context.Context, opts ...grpc.CallOption) (IntStore_StreamChatClient, error) {
	stream, err := c.cc.NewStream(ctx, &_IntStore_serviceDesc.Streams[2], "/intstore.IntStore/StreamChat", opts...)
	if err != nil {
		return nil, err
	}
	x := &intStoreStreamChatClient{stream}
	return x, nil
}

type IntStore_StreamChatClient interface {
	Send(*Item) error
	Recv() (*Item, error)
	grpc.ClientStream
}

type intStoreStreamChatClient struct {
	grpc.ClientStream
}

func (x *intStoreStreamChatClient) Send(m *Item) error {
	return x.ClientStream.SendMsg(m)
}

func (x *intStoreStreamChatClient) Recv() (*Item, error) {
	m := new(Item)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// IntStoreServer is the server API for IntStore service.
type IntStoreServer interface {
	Set(context.Context, *Item) (*SetResponse, error)
	Get(context.Context, *GetRequest) (*Item, error)
	// A server-to-client streaming RPC.
	ListItems(*ListItemsRequest, IntStore_ListItemsServer) error
	// A client-to-server streaming RPC.
	SetStream(IntStore_SetStreamServer) error
	// A Bidirectional streaming RPC.
	StreamChat(IntStore_StreamChatServer) error
}

// UnimplementedIntStoreServer can be embedded to have forward compatible implementations.
type UnimplementedIntStoreServer struct {
}

func (*UnimplementedIntStoreServer) Set(context.Context, *Item) (*SetResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Set not implemented")
}
func (*UnimplementedIntStoreServer) Get(context.Context, *GetRequest) (*Item, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Get not implemented")
}
func (*UnimplementedIntStoreServer) ListItems(*ListItemsRequest, IntStore_ListItemsServer) error {
	return status.Errorf(codes.Unimplemented, "method ListItems not implemented")
}
func (*UnimplementedIntStoreServer) SetStream(IntStore_SetStreamServer) error {
	return status.Errorf(codes.Unimplemented, "method SetStream not implemented")
}
func (*UnimplementedIntStoreServer) StreamChat(IntStore_StreamChatServer) error {
	return status.Errorf(codes.Unimplemented, "method StreamChat not implemented")
}

func RegisterIntStoreServer(s *grpc.Server, srv IntStoreServer) {
	s.RegisterService(&_IntStore_serviceDesc, srv)
}

func _IntStore_Set_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Item)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(IntStoreServer).Set(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/intstore.IntStore/Set",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(IntStoreServer).Set(ctx, req.(*Item))
	}
	return interceptor(ctx, in, info, handler)
}

func _IntStore_Get_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(IntStoreServer).Get(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/intstore.IntStore/Get",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(IntStoreServer).Get(ctx, req.(*GetRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _IntStore_ListItems_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(ListItemsRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(IntStoreServer).ListItems(m, &intStoreListItemsServer{stream})
}

type IntStore_ListItemsServer interface {
	Send(*Item) error
	grpc.ServerStream
}

type intStoreListItemsServer struct {
	grpc.ServerStream
}

func (x *intStoreListItemsServer) Send(m *Item) error {
	return x.ServerStream.SendMsg(m)
}

func _IntStore_SetStream_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(IntStoreServer).SetStream(&intStoreSetStreamServer{stream})
}

type IntStore_SetStreamServer interface {
	SendAndClose(*Summary) error
	Recv() (*Item, error)
	grpc.ServerStream
}

type intStoreSetStreamServer struct {
	grpc.ServerStream
}

func (x *intStoreSetStreamServer) SendAndClose(m *Summary) error {
	return x.ServerStream.SendMsg(m)
}

func (x *intStoreSetStreamServer) Recv() (*Item, error) {
	m := new(Item)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _IntStore_StreamChat_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(IntStoreServer).StreamChat(&intStoreStreamChatServer{stream})
}

type IntStore_StreamChatServer interface {
	Send(*Item) error
	Recv() (*Item, error)
	grpc.ServerStream
}

type intStoreStreamChatServer struct {
	grpc.ServerStream
}

func (x *intStoreStreamChatServer) Send(m *Item) error {
	return x.ServerStream.SendMsg(m)
}

func (x *intStoreStreamChatServer) Recv() (*Item, error) {
	m := new(Item)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

var _IntStore_serviceDesc = grpc.ServiceDesc{
	ServiceName: "intstore.IntStore",
	HandlerType: (*IntStoreServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Set",
			Handler:    _IntStore_Set_Handler,
		},
		{
			MethodName: "Get",
			Handler:    _IntStore_Get_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "ListItems",
			Handler:       _IntStore_ListItems_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "SetStream",
			Handler:       _IntStore_SetStream_Handler,
			ClientStreams: true,
		},
		{
			StreamName:    "StreamChat",
			Handler:       _IntStore_StreamChat_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "intstore.proto",
}
