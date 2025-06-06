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
// source: google/firestore/admin/v1/backup.proto

package adminpb

import (
	_ "google.golang.org/genproto/googleapis/api/annotations"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// Indicate the current state of the backup.
type Backup_State int32

const (
	// The state is unspecified.
	Backup_STATE_UNSPECIFIED Backup_State = 0
	// The pending backup is still being created. Operations on the
	// backup will be rejected in this state.
	Backup_CREATING Backup_State = 1
	// The backup is complete and ready to use.
	Backup_READY Backup_State = 2
	// The backup is not available at this moment.
	Backup_NOT_AVAILABLE Backup_State = 3
)

// Enum value maps for Backup_State.
var (
	Backup_State_name = map[int32]string{
		0: "STATE_UNSPECIFIED",
		1: "CREATING",
		2: "READY",
		3: "NOT_AVAILABLE",
	}
	Backup_State_value = map[string]int32{
		"STATE_UNSPECIFIED": 0,
		"CREATING":          1,
		"READY":             2,
		"NOT_AVAILABLE":     3,
	}
)

func (x Backup_State) Enum() *Backup_State {
	p := new(Backup_State)
	*p = x
	return p
}

func (x Backup_State) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Backup_State) Descriptor() protoreflect.EnumDescriptor {
	return file_google_firestore_admin_v1_backup_proto_enumTypes[0].Descriptor()
}

func (Backup_State) Type() protoreflect.EnumType {
	return &file_google_firestore_admin_v1_backup_proto_enumTypes[0]
}

func (x Backup_State) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Backup_State.Descriptor instead.
func (Backup_State) EnumDescriptor() ([]byte, []int) {
	return file_google_firestore_admin_v1_backup_proto_rawDescGZIP(), []int{0, 0}
}

// A Backup of a Cloud Firestore Database.
//
// The backup contains all documents and index configurations for the given
// database at a specific point in time.
type Backup struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Output only. The unique resource name of the Backup.
	//
	// Format is `projects/{project}/locations/{location}/backups/{backup}`.
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Output only. Name of the Firestore database that the backup is from.
	//
	// Format is `projects/{project}/databases/{database}`.
	Database string `protobuf:"bytes,2,opt,name=database,proto3" json:"database,omitempty"`
	// Output only. The system-generated UUID4 for the Firestore database that the
	// backup is from.
	DatabaseUid string `protobuf:"bytes,7,opt,name=database_uid,json=databaseUid,proto3" json:"database_uid,omitempty"`
	// Output only. The backup contains an externally consistent copy of the
	// database at this time.
	SnapshotTime *timestamppb.Timestamp `protobuf:"bytes,3,opt,name=snapshot_time,json=snapshotTime,proto3" json:"snapshot_time,omitempty"`
	// Output only. The timestamp at which this backup expires.
	ExpireTime *timestamppb.Timestamp `protobuf:"bytes,4,opt,name=expire_time,json=expireTime,proto3" json:"expire_time,omitempty"`
	// Output only. Statistics about the backup.
	//
	// This data only becomes available after the backup is fully materialized to
	// secondary storage. This field will be empty till then.
	Stats *Backup_Stats `protobuf:"bytes,6,opt,name=stats,proto3" json:"stats,omitempty"`
	// Output only. The current state of the backup.
	State Backup_State `protobuf:"varint,8,opt,name=state,proto3,enum=google.firestore.admin.v1.Backup_State" json:"state,omitempty"`
}

func (x *Backup) Reset() {
	*x = Backup{}
	mi := &file_google_firestore_admin_v1_backup_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Backup) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Backup) ProtoMessage() {}

func (x *Backup) ProtoReflect() protoreflect.Message {
	mi := &file_google_firestore_admin_v1_backup_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Backup.ProtoReflect.Descriptor instead.
func (*Backup) Descriptor() ([]byte, []int) {
	return file_google_firestore_admin_v1_backup_proto_rawDescGZIP(), []int{0}
}

func (x *Backup) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Backup) GetDatabase() string {
	if x != nil {
		return x.Database
	}
	return ""
}

func (x *Backup) GetDatabaseUid() string {
	if x != nil {
		return x.DatabaseUid
	}
	return ""
}

func (x *Backup) GetSnapshotTime() *timestamppb.Timestamp {
	if x != nil {
		return x.SnapshotTime
	}
	return nil
}

func (x *Backup) GetExpireTime() *timestamppb.Timestamp {
	if x != nil {
		return x.ExpireTime
	}
	return nil
}

func (x *Backup) GetStats() *Backup_Stats {
	if x != nil {
		return x.Stats
	}
	return nil
}

func (x *Backup) GetState() Backup_State {
	if x != nil {
		return x.State
	}
	return Backup_STATE_UNSPECIFIED
}

// Backup specific statistics.
type Backup_Stats struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Output only. Summation of the size of all documents and index entries in
	// the backup, measured in bytes.
	SizeBytes int64 `protobuf:"varint,1,opt,name=size_bytes,json=sizeBytes,proto3" json:"size_bytes,omitempty"`
	// Output only. The total number of documents contained in the backup.
	DocumentCount int64 `protobuf:"varint,2,opt,name=document_count,json=documentCount,proto3" json:"document_count,omitempty"`
	// Output only. The total number of index entries contained in the backup.
	IndexCount int64 `protobuf:"varint,3,opt,name=index_count,json=indexCount,proto3" json:"index_count,omitempty"`
}

func (x *Backup_Stats) Reset() {
	*x = Backup_Stats{}
	mi := &file_google_firestore_admin_v1_backup_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Backup_Stats) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Backup_Stats) ProtoMessage() {}

func (x *Backup_Stats) ProtoReflect() protoreflect.Message {
	mi := &file_google_firestore_admin_v1_backup_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Backup_Stats.ProtoReflect.Descriptor instead.
func (*Backup_Stats) Descriptor() ([]byte, []int) {
	return file_google_firestore_admin_v1_backup_proto_rawDescGZIP(), []int{0, 0}
}

func (x *Backup_Stats) GetSizeBytes() int64 {
	if x != nil {
		return x.SizeBytes
	}
	return 0
}

func (x *Backup_Stats) GetDocumentCount() int64 {
	if x != nil {
		return x.DocumentCount
	}
	return 0
}

func (x *Backup_Stats) GetIndexCount() int64 {
	if x != nil {
		return x.IndexCount
	}
	return 0
}

var File_google_firestore_admin_v1_backup_proto protoreflect.FileDescriptor

var file_google_firestore_admin_v1_backup_proto_rawDesc = []byte{
	0x0a, 0x26, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x66, 0x69, 0x72, 0x65, 0x73, 0x74, 0x6f,
	0x72, 0x65, 0x2f, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2f, 0x76, 0x31, 0x2f, 0x62, 0x61, 0x63, 0x6b,
	0x75, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x19, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x66, 0x69, 0x72, 0x65, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x2e, 0x61, 0x64, 0x6d, 0x69, 0x6e,
	0x2e, 0x76, 0x31, 0x1a, 0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f,
	0x66, 0x69, 0x65, 0x6c, 0x64, 0x5f, 0x62, 0x65, 0x68, 0x61, 0x76, 0x69, 0x6f, 0x72, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x19, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x61, 0x70, 0x69,
	0x2f, 0x72, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a,
	0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66,
	0x2f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x22, 0xcb, 0x05, 0x0a, 0x06, 0x42, 0x61, 0x63, 0x6b, 0x75, 0x70, 0x12, 0x17, 0x0a, 0x04, 0x6e,
	0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x42, 0x03, 0xe0, 0x41, 0x03, 0x52, 0x04,
	0x6e, 0x61, 0x6d, 0x65, 0x12, 0x45, 0x0a, 0x08, 0x64, 0x61, 0x74, 0x61, 0x62, 0x61, 0x73, 0x65,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x42, 0x29, 0xe0, 0x41, 0x03, 0xfa, 0x41, 0x23, 0x0a, 0x21,
	0x66, 0x69, 0x72, 0x65, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x61, 0x70, 0x69, 0x73, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x44, 0x61, 0x74, 0x61, 0x62, 0x61, 0x73,
	0x65, 0x52, 0x08, 0x64, 0x61, 0x74, 0x61, 0x62, 0x61, 0x73, 0x65, 0x12, 0x26, 0x0a, 0x0c, 0x64,
	0x61, 0x74, 0x61, 0x62, 0x61, 0x73, 0x65, 0x5f, 0x75, 0x69, 0x64, 0x18, 0x07, 0x20, 0x01, 0x28,
	0x09, 0x42, 0x03, 0xe0, 0x41, 0x03, 0x52, 0x0b, 0x64, 0x61, 0x74, 0x61, 0x62, 0x61, 0x73, 0x65,
	0x55, 0x69, 0x64, 0x12, 0x44, 0x0a, 0x0d, 0x73, 0x6e, 0x61, 0x70, 0x73, 0x68, 0x6f, 0x74, 0x5f,
	0x74, 0x69, 0x6d, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f,
	0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d,
	0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x42, 0x03, 0xe0, 0x41, 0x03, 0x52, 0x0c, 0x73, 0x6e, 0x61,
	0x70, 0x73, 0x68, 0x6f, 0x74, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x40, 0x0a, 0x0b, 0x65, 0x78, 0x70,
	0x69, 0x72, 0x65, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a,
	0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66,
	0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x42, 0x03, 0xe0, 0x41, 0x03, 0x52,
	0x0a, 0x65, 0x78, 0x70, 0x69, 0x72, 0x65, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x42, 0x0a, 0x05, 0x73,
	0x74, 0x61, 0x74, 0x73, 0x18, 0x06, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x27, 0x2e, 0x67, 0x6f, 0x6f,
	0x67, 0x6c, 0x65, 0x2e, 0x66, 0x69, 0x72, 0x65, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x2e, 0x61, 0x64,
	0x6d, 0x69, 0x6e, 0x2e, 0x76, 0x31, 0x2e, 0x42, 0x61, 0x63, 0x6b, 0x75, 0x70, 0x2e, 0x53, 0x74,
	0x61, 0x74, 0x73, 0x42, 0x03, 0xe0, 0x41, 0x03, 0x52, 0x05, 0x73, 0x74, 0x61, 0x74, 0x73, 0x12,
	0x42, 0x0a, 0x05, 0x73, 0x74, 0x61, 0x74, 0x65, 0x18, 0x08, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x27,
	0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x66, 0x69, 0x72, 0x65, 0x73, 0x74, 0x6f, 0x72,
	0x65, 0x2e, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2e, 0x76, 0x31, 0x2e, 0x42, 0x61, 0x63, 0x6b, 0x75,
	0x70, 0x2e, 0x53, 0x74, 0x61, 0x74, 0x65, 0x42, 0x03, 0xe0, 0x41, 0x03, 0x52, 0x05, 0x73, 0x74,
	0x61, 0x74, 0x65, 0x1a, 0x7d, 0x0a, 0x05, 0x53, 0x74, 0x61, 0x74, 0x73, 0x12, 0x22, 0x0a, 0x0a,
	0x73, 0x69, 0x7a, 0x65, 0x5f, 0x62, 0x79, 0x74, 0x65, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x03,
	0x42, 0x03, 0xe0, 0x41, 0x03, 0x52, 0x09, 0x73, 0x69, 0x7a, 0x65, 0x42, 0x79, 0x74, 0x65, 0x73,
	0x12, 0x2a, 0x0a, 0x0e, 0x64, 0x6f, 0x63, 0x75, 0x6d, 0x65, 0x6e, 0x74, 0x5f, 0x63, 0x6f, 0x75,
	0x6e, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x03, 0x42, 0x03, 0xe0, 0x41, 0x03, 0x52, 0x0d, 0x64,
	0x6f, 0x63, 0x75, 0x6d, 0x65, 0x6e, 0x74, 0x43, 0x6f, 0x75, 0x6e, 0x74, 0x12, 0x24, 0x0a, 0x0b,
	0x69, 0x6e, 0x64, 0x65, 0x78, 0x5f, 0x63, 0x6f, 0x75, 0x6e, 0x74, 0x18, 0x03, 0x20, 0x01, 0x28,
	0x03, 0x42, 0x03, 0xe0, 0x41, 0x03, 0x52, 0x0a, 0x69, 0x6e, 0x64, 0x65, 0x78, 0x43, 0x6f, 0x75,
	0x6e, 0x74, 0x22, 0x4a, 0x0a, 0x05, 0x53, 0x74, 0x61, 0x74, 0x65, 0x12, 0x15, 0x0a, 0x11, 0x53,
	0x54, 0x41, 0x54, 0x45, 0x5f, 0x55, 0x4e, 0x53, 0x50, 0x45, 0x43, 0x49, 0x46, 0x49, 0x45, 0x44,
	0x10, 0x00, 0x12, 0x0c, 0x0a, 0x08, 0x43, 0x52, 0x45, 0x41, 0x54, 0x49, 0x4e, 0x47, 0x10, 0x01,
	0x12, 0x09, 0x0a, 0x05, 0x52, 0x45, 0x41, 0x44, 0x59, 0x10, 0x02, 0x12, 0x11, 0x0a, 0x0d, 0x4e,
	0x4f, 0x54, 0x5f, 0x41, 0x56, 0x41, 0x49, 0x4c, 0x41, 0x42, 0x4c, 0x45, 0x10, 0x03, 0x3a, 0x5e,
	0xea, 0x41, 0x5b, 0x0a, 0x1f, 0x66, 0x69, 0x72, 0x65, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x2e, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x61, 0x70, 0x69, 0x73, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x42, 0x61,
	0x63, 0x6b, 0x75, 0x70, 0x12, 0x38, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x73, 0x2f, 0x7b,
	0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x7d, 0x2f, 0x6c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f,
	0x6e, 0x73, 0x2f, 0x7b, 0x6c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x7d, 0x2f, 0x62, 0x61,
	0x63, 0x6b, 0x75, 0x70, 0x73, 0x2f, 0x7b, 0x62, 0x61, 0x63, 0x6b, 0x75, 0x70, 0x7d, 0x42, 0xda,
	0x01, 0x0a, 0x1d, 0x63, 0x6f, 0x6d, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x66, 0x69,
	0x72, 0x65, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x2e, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2e, 0x76, 0x31,
	0x42, 0x0b, 0x42, 0x61, 0x63, 0x6b, 0x75, 0x70, 0x50, 0x72, 0x6f, 0x74, 0x6f, 0x50, 0x01, 0x5a,
	0x39, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x63, 0x6f,
	0x6d, 0x2f, 0x67, 0x6f, 0x2f, 0x66, 0x69, 0x72, 0x65, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x2f, 0x61,
	0x70, 0x69, 0x76, 0x31, 0x2f, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2f, 0x61, 0x64, 0x6d, 0x69, 0x6e,
	0x70, 0x62, 0x3b, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x70, 0x62, 0xa2, 0x02, 0x04, 0x47, 0x43, 0x46,
	0x53, 0xaa, 0x02, 0x1f, 0x47, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x43, 0x6c, 0x6f, 0x75, 0x64,
	0x2e, 0x46, 0x69, 0x72, 0x65, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x2e, 0x41, 0x64, 0x6d, 0x69, 0x6e,
	0x2e, 0x56, 0x31, 0xca, 0x02, 0x1f, 0x47, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x5c, 0x43, 0x6c, 0x6f,
	0x75, 0x64, 0x5c, 0x46, 0x69, 0x72, 0x65, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x5c, 0x41, 0x64, 0x6d,
	0x69, 0x6e, 0x5c, 0x56, 0x31, 0xea, 0x02, 0x23, 0x47, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x3a, 0x3a,
	0x43, 0x6c, 0x6f, 0x75, 0x64, 0x3a, 0x3a, 0x46, 0x69, 0x72, 0x65, 0x73, 0x74, 0x6f, 0x72, 0x65,
	0x3a, 0x3a, 0x41, 0x64, 0x6d, 0x69, 0x6e, 0x3a, 0x3a, 0x56, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x33,
}

var (
	file_google_firestore_admin_v1_backup_proto_rawDescOnce sync.Once
	file_google_firestore_admin_v1_backup_proto_rawDescData = file_google_firestore_admin_v1_backup_proto_rawDesc
)

func file_google_firestore_admin_v1_backup_proto_rawDescGZIP() []byte {
	file_google_firestore_admin_v1_backup_proto_rawDescOnce.Do(func() {
		file_google_firestore_admin_v1_backup_proto_rawDescData = protoimpl.X.CompressGZIP(file_google_firestore_admin_v1_backup_proto_rawDescData)
	})
	return file_google_firestore_admin_v1_backup_proto_rawDescData
}

var file_google_firestore_admin_v1_backup_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_google_firestore_admin_v1_backup_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_google_firestore_admin_v1_backup_proto_goTypes = []any{
	(Backup_State)(0),             // 0: google.firestore.admin.v1.Backup.State
	(*Backup)(nil),                // 1: google.firestore.admin.v1.Backup
	(*Backup_Stats)(nil),          // 2: google.firestore.admin.v1.Backup.Stats
	(*timestamppb.Timestamp)(nil), // 3: google.protobuf.Timestamp
}
var file_google_firestore_admin_v1_backup_proto_depIdxs = []int32{
	3, // 0: google.firestore.admin.v1.Backup.snapshot_time:type_name -> google.protobuf.Timestamp
	3, // 1: google.firestore.admin.v1.Backup.expire_time:type_name -> google.protobuf.Timestamp
	2, // 2: google.firestore.admin.v1.Backup.stats:type_name -> google.firestore.admin.v1.Backup.Stats
	0, // 3: google.firestore.admin.v1.Backup.state:type_name -> google.firestore.admin.v1.Backup.State
	4, // [4:4] is the sub-list for method output_type
	4, // [4:4] is the sub-list for method input_type
	4, // [4:4] is the sub-list for extension type_name
	4, // [4:4] is the sub-list for extension extendee
	0, // [0:4] is the sub-list for field type_name
}

func init() { file_google_firestore_admin_v1_backup_proto_init() }
func file_google_firestore_admin_v1_backup_proto_init() {
	if File_google_firestore_admin_v1_backup_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_google_firestore_admin_v1_backup_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_google_firestore_admin_v1_backup_proto_goTypes,
		DependencyIndexes: file_google_firestore_admin_v1_backup_proto_depIdxs,
		EnumInfos:         file_google_firestore_admin_v1_backup_proto_enumTypes,
		MessageInfos:      file_google_firestore_admin_v1_backup_proto_msgTypes,
	}.Build()
	File_google_firestore_admin_v1_backup_proto = out.File
	file_google_firestore_admin_v1_backup_proto_rawDesc = nil
	file_google_firestore_admin_v1_backup_proto_goTypes = nil
	file_google_firestore_admin_v1_backup_proto_depIdxs = nil
}
