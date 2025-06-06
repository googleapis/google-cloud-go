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
// source: google/cloud/apihub/v1/host_project_registration_service.proto

package apihubpb

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

// The
// [CreateHostProjectRegistration][google.cloud.apihub.v1.HostProjectRegistrationService.CreateHostProjectRegistration]
// method's request.
type CreateHostProjectRegistrationRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Required. The parent resource for the host project.
	// Format: `projects/{project}/locations/{location}`
	Parent string `protobuf:"bytes,1,opt,name=parent,proto3" json:"parent,omitempty"`
	// Required. The ID to use for the Host Project Registration, which will
	// become the final component of the host project registration's resource
	// name. The ID must be the same as the Google cloud project specified in the
	// host_project_registration.gcp_project field.
	HostProjectRegistrationId string `protobuf:"bytes,2,opt,name=host_project_registration_id,json=hostProjectRegistrationId,proto3" json:"host_project_registration_id,omitempty"`
	// Required. The host project registration to register.
	HostProjectRegistration *HostProjectRegistration `protobuf:"bytes,3,opt,name=host_project_registration,json=hostProjectRegistration,proto3" json:"host_project_registration,omitempty"`
}

func (x *CreateHostProjectRegistrationRequest) Reset() {
	*x = CreateHostProjectRegistrationRequest{}
	mi := &file_google_cloud_apihub_v1_host_project_registration_service_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *CreateHostProjectRegistrationRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CreateHostProjectRegistrationRequest) ProtoMessage() {}

func (x *CreateHostProjectRegistrationRequest) ProtoReflect() protoreflect.Message {
	mi := &file_google_cloud_apihub_v1_host_project_registration_service_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CreateHostProjectRegistrationRequest.ProtoReflect.Descriptor instead.
func (*CreateHostProjectRegistrationRequest) Descriptor() ([]byte, []int) {
	return file_google_cloud_apihub_v1_host_project_registration_service_proto_rawDescGZIP(), []int{0}
}

func (x *CreateHostProjectRegistrationRequest) GetParent() string {
	if x != nil {
		return x.Parent
	}
	return ""
}

func (x *CreateHostProjectRegistrationRequest) GetHostProjectRegistrationId() string {
	if x != nil {
		return x.HostProjectRegistrationId
	}
	return ""
}

func (x *CreateHostProjectRegistrationRequest) GetHostProjectRegistration() *HostProjectRegistration {
	if x != nil {
		return x.HostProjectRegistration
	}
	return nil
}

// The
// [GetHostProjectRegistration][google.cloud.apihub.v1.HostProjectRegistrationService.GetHostProjectRegistration]
// method's request.
type GetHostProjectRegistrationRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Required. Host project registration resource name.
	// projects/{project}/locations/{location}/hostProjectRegistrations/{host_project_registration_id}
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
}

func (x *GetHostProjectRegistrationRequest) Reset() {
	*x = GetHostProjectRegistrationRequest{}
	mi := &file_google_cloud_apihub_v1_host_project_registration_service_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetHostProjectRegistrationRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetHostProjectRegistrationRequest) ProtoMessage() {}

func (x *GetHostProjectRegistrationRequest) ProtoReflect() protoreflect.Message {
	mi := &file_google_cloud_apihub_v1_host_project_registration_service_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetHostProjectRegistrationRequest.ProtoReflect.Descriptor instead.
func (*GetHostProjectRegistrationRequest) Descriptor() ([]byte, []int) {
	return file_google_cloud_apihub_v1_host_project_registration_service_proto_rawDescGZIP(), []int{1}
}

func (x *GetHostProjectRegistrationRequest) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

// The
// [ListHostProjectRegistrations][google.cloud.apihub.v1.HostProjectRegistrationService.ListHostProjectRegistrations]
// method's request.
type ListHostProjectRegistrationsRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Required. The parent, which owns this collection of host projects.
	// Format: `projects/*/locations/*`
	Parent string `protobuf:"bytes,1,opt,name=parent,proto3" json:"parent,omitempty"`
	// Optional. The maximum number of host project registrations to return. The
	// service may return fewer than this value. If unspecified, at most 50 host
	// project registrations will be returned. The maximum value is 1000; values
	// above 1000 will be coerced to 1000.
	PageSize int32 `protobuf:"varint,2,opt,name=page_size,json=pageSize,proto3" json:"page_size,omitempty"`
	// Optional. A page token, received from a previous
	// `ListHostProjectRegistrations` call. Provide this to retrieve the
	// subsequent page.
	//
	// When paginating, all other parameters (except page_size) provided to
	// `ListHostProjectRegistrations` must match the call that provided the page
	// token.
	PageToken string `protobuf:"bytes,3,opt,name=page_token,json=pageToken,proto3" json:"page_token,omitempty"`
	// Optional. An expression that filters the list of HostProjectRegistrations.
	//
	// A filter expression consists of a field name, a comparison
	// operator, and a value for filtering. The value must be a string. All
	// standard operators as documented at https://google.aip.dev/160 are
	// supported.
	//
	// The following fields in the `HostProjectRegistration` are eligible for
	// filtering:
	//
	//   - `name` - The name of the HostProjectRegistration.
	//   - `create_time` - The time at which the HostProjectRegistration was
	//     created. The value should be in the
	//     (RFC3339)[https://tools.ietf.org/html/rfc3339] format.
	//   - `gcp_project` - The Google cloud project associated with the
	//     HostProjectRegistration.
	Filter string `protobuf:"bytes,4,opt,name=filter,proto3" json:"filter,omitempty"`
	// Optional. Hint for how to order the results.
	OrderBy string `protobuf:"bytes,5,opt,name=order_by,json=orderBy,proto3" json:"order_by,omitempty"`
}

func (x *ListHostProjectRegistrationsRequest) Reset() {
	*x = ListHostProjectRegistrationsRequest{}
	mi := &file_google_cloud_apihub_v1_host_project_registration_service_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ListHostProjectRegistrationsRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ListHostProjectRegistrationsRequest) ProtoMessage() {}

func (x *ListHostProjectRegistrationsRequest) ProtoReflect() protoreflect.Message {
	mi := &file_google_cloud_apihub_v1_host_project_registration_service_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ListHostProjectRegistrationsRequest.ProtoReflect.Descriptor instead.
func (*ListHostProjectRegistrationsRequest) Descriptor() ([]byte, []int) {
	return file_google_cloud_apihub_v1_host_project_registration_service_proto_rawDescGZIP(), []int{2}
}

func (x *ListHostProjectRegistrationsRequest) GetParent() string {
	if x != nil {
		return x.Parent
	}
	return ""
}

func (x *ListHostProjectRegistrationsRequest) GetPageSize() int32 {
	if x != nil {
		return x.PageSize
	}
	return 0
}

func (x *ListHostProjectRegistrationsRequest) GetPageToken() string {
	if x != nil {
		return x.PageToken
	}
	return ""
}

func (x *ListHostProjectRegistrationsRequest) GetFilter() string {
	if x != nil {
		return x.Filter
	}
	return ""
}

func (x *ListHostProjectRegistrationsRequest) GetOrderBy() string {
	if x != nil {
		return x.OrderBy
	}
	return ""
}

// The
// [ListHostProjectRegistrations][google.cloud.apihub.v1.HostProjectRegistrationService.ListHostProjectRegistrations]
// method's response.
type ListHostProjectRegistrationsResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// The list of host project registrations.
	HostProjectRegistrations []*HostProjectRegistration `protobuf:"bytes,1,rep,name=host_project_registrations,json=hostProjectRegistrations,proto3" json:"host_project_registrations,omitempty"`
	// A token, which can be sent as `page_token` to retrieve the next page.
	// If this field is omitted, there are no subsequent pages.
	NextPageToken string `protobuf:"bytes,2,opt,name=next_page_token,json=nextPageToken,proto3" json:"next_page_token,omitempty"`
}

func (x *ListHostProjectRegistrationsResponse) Reset() {
	*x = ListHostProjectRegistrationsResponse{}
	mi := &file_google_cloud_apihub_v1_host_project_registration_service_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ListHostProjectRegistrationsResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ListHostProjectRegistrationsResponse) ProtoMessage() {}

func (x *ListHostProjectRegistrationsResponse) ProtoReflect() protoreflect.Message {
	mi := &file_google_cloud_apihub_v1_host_project_registration_service_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ListHostProjectRegistrationsResponse.ProtoReflect.Descriptor instead.
func (*ListHostProjectRegistrationsResponse) Descriptor() ([]byte, []int) {
	return file_google_cloud_apihub_v1_host_project_registration_service_proto_rawDescGZIP(), []int{3}
}

func (x *ListHostProjectRegistrationsResponse) GetHostProjectRegistrations() []*HostProjectRegistration {
	if x != nil {
		return x.HostProjectRegistrations
	}
	return nil
}

func (x *ListHostProjectRegistrationsResponse) GetNextPageToken() string {
	if x != nil {
		return x.NextPageToken
	}
	return ""
}

// Host project registration refers to the registration of a Google cloud
// project with Api Hub as a host project. This is the project where Api Hub is
// provisioned. It acts as the consumer project for the Api Hub instance
// provisioned. Multiple runtime projects can be attached to the host project
// and these attachments define the scope of Api Hub.
type HostProjectRegistration struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Identifier. The name of the host project registration.
	// Format:
	// "projects/{project}/locations/{location}/hostProjectRegistrations/{host_project_registration}".
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Required. Immutable. Google cloud project name in the format:
	// "projects/abc" or "projects/123". As input, project name with either
	// project id or number are accepted. As output, this field will contain
	// project number.
	GcpProject string `protobuf:"bytes,2,opt,name=gcp_project,json=gcpProject,proto3" json:"gcp_project,omitempty"`
	// Output only. The time at which the host project registration was created.
	CreateTime *timestamppb.Timestamp `protobuf:"bytes,3,opt,name=create_time,json=createTime,proto3" json:"create_time,omitempty"`
}

func (x *HostProjectRegistration) Reset() {
	*x = HostProjectRegistration{}
	mi := &file_google_cloud_apihub_v1_host_project_registration_service_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *HostProjectRegistration) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*HostProjectRegistration) ProtoMessage() {}

func (x *HostProjectRegistration) ProtoReflect() protoreflect.Message {
	mi := &file_google_cloud_apihub_v1_host_project_registration_service_proto_msgTypes[4]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use HostProjectRegistration.ProtoReflect.Descriptor instead.
func (*HostProjectRegistration) Descriptor() ([]byte, []int) {
	return file_google_cloud_apihub_v1_host_project_registration_service_proto_rawDescGZIP(), []int{4}
}

func (x *HostProjectRegistration) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *HostProjectRegistration) GetGcpProject() string {
	if x != nil {
		return x.GcpProject
	}
	return ""
}

func (x *HostProjectRegistration) GetCreateTime() *timestamppb.Timestamp {
	if x != nil {
		return x.CreateTime
	}
	return nil
}

var File_google_cloud_apihub_v1_host_project_registration_service_proto protoreflect.FileDescriptor

var file_google_cloud_apihub_v1_host_project_registration_service_proto_rawDesc = []byte{
	0x0a, 0x3e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2f, 0x61,
	0x70, 0x69, 0x68, 0x75, 0x62, 0x2f, 0x76, 0x31, 0x2f, 0x68, 0x6f, 0x73, 0x74, 0x5f, 0x70, 0x72,
	0x6f, 0x6a, 0x65, 0x63, 0x74, 0x5f, 0x72, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69,
	0x6f, 0x6e, 0x5f, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x12, 0x16, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x61,
	0x70, 0x69, 0x68, 0x75, 0x62, 0x2e, 0x76, 0x31, 0x1a, 0x1c, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2f, 0x61, 0x70, 0x69, 0x2f, 0x61, 0x6e, 0x6e, 0x6f, 0x74, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x17, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x61,
	0x70, 0x69, 0x2f, 0x63, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a,
	0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x66, 0x69, 0x65, 0x6c,
	0x64, 0x5f, 0x62, 0x65, 0x68, 0x61, 0x76, 0x69, 0x6f, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x1a, 0x19, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x72, 0x65, 0x73,
	0x6f, 0x75, 0x72, 0x63, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1f, 0x67, 0x6f, 0x6f,
	0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x74, 0x69, 0x6d,
	0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xad, 0x02, 0x0a,
	0x24, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x48, 0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65,
	0x63, 0x74, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x4d, 0x0a, 0x06, 0x70, 0x61, 0x72, 0x65, 0x6e, 0x74, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x42, 0x35, 0xe0, 0x41, 0x02, 0xfa, 0x41, 0x2f, 0x12, 0x2d, 0x61,
	0x70, 0x69, 0x68, 0x75, 0x62, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x61, 0x70, 0x69, 0x73,
	0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x48, 0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74,
	0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x06, 0x70, 0x61,
	0x72, 0x65, 0x6e, 0x74, 0x12, 0x44, 0x0a, 0x1c, 0x68, 0x6f, 0x73, 0x74, 0x5f, 0x70, 0x72, 0x6f,
	0x6a, 0x65, 0x63, 0x74, 0x5f, 0x72, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f,
	0x6e, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x42, 0x03, 0xe0, 0x41, 0x02, 0x52,
	0x19, 0x68, 0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52, 0x65, 0x67, 0x69,
	0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x49, 0x64, 0x12, 0x70, 0x0a, 0x19, 0x68, 0x6f,
	0x73, 0x74, 0x5f, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x5f, 0x72, 0x65, 0x67, 0x69, 0x73,
	0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x2f, 0x2e,
	0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x61, 0x70, 0x69,
	0x68, 0x75, 0x62, 0x2e, 0x76, 0x31, 0x2e, 0x48, 0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65,
	0x63, 0x74, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x42, 0x03,
	0xe0, 0x41, 0x02, 0x52, 0x17, 0x68, 0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74,
	0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x22, 0x6e, 0x0a, 0x21,
	0x47, 0x65, 0x74, 0x48, 0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52, 0x65,
	0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x12, 0x49, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x42,
	0x35, 0xe0, 0x41, 0x02, 0xfa, 0x41, 0x2f, 0x0a, 0x2d, 0x61, 0x70, 0x69, 0x68, 0x75, 0x62, 0x2e,
	0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x61, 0x70, 0x69, 0x73, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x48,
	0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74,
	0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x22, 0xf7, 0x01, 0x0a,
	0x23, 0x4c, 0x69, 0x73, 0x74, 0x48, 0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74,
	0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x52, 0x65, 0x71,
	0x75, 0x65, 0x73, 0x74, 0x12, 0x4d, 0x0a, 0x06, 0x70, 0x61, 0x72, 0x65, 0x6e, 0x74, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x09, 0x42, 0x35, 0xe0, 0x41, 0x02, 0xfa, 0x41, 0x2f, 0x12, 0x2d, 0x61, 0x70,
	0x69, 0x68, 0x75, 0x62, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x61, 0x70, 0x69, 0x73, 0x2e,
	0x63, 0x6f, 0x6d, 0x2f, 0x48, 0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52,
	0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x06, 0x70, 0x61, 0x72,
	0x65, 0x6e, 0x74, 0x12, 0x20, 0x0a, 0x09, 0x70, 0x61, 0x67, 0x65, 0x5f, 0x73, 0x69, 0x7a, 0x65,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x05, 0x42, 0x03, 0xe0, 0x41, 0x01, 0x52, 0x08, 0x70, 0x61, 0x67,
	0x65, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x22, 0x0a, 0x0a, 0x70, 0x61, 0x67, 0x65, 0x5f, 0x74, 0x6f,
	0x6b, 0x65, 0x6e, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x42, 0x03, 0xe0, 0x41, 0x01, 0x52, 0x09,
	0x70, 0x61, 0x67, 0x65, 0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x12, 0x1b, 0x0a, 0x06, 0x66, 0x69, 0x6c,
	0x74, 0x65, 0x72, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x42, 0x03, 0xe0, 0x41, 0x01, 0x52, 0x06,
	0x66, 0x69, 0x6c, 0x74, 0x65, 0x72, 0x12, 0x1e, 0x0a, 0x08, 0x6f, 0x72, 0x64, 0x65, 0x72, 0x5f,
	0x62, 0x79, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09, 0x42, 0x03, 0xe0, 0x41, 0x01, 0x52, 0x07, 0x6f,
	0x72, 0x64, 0x65, 0x72, 0x42, 0x79, 0x22, 0xbd, 0x01, 0x0a, 0x24, 0x4c, 0x69, 0x73, 0x74, 0x48,
	0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74,
	0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12,
	0x6d, 0x0a, 0x1a, 0x68, 0x6f, 0x73, 0x74, 0x5f, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x5f,
	0x72, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0x01, 0x20,
	0x03, 0x28, 0x0b, 0x32, 0x2f, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x63, 0x6c, 0x6f,
	0x75, 0x64, 0x2e, 0x61, 0x70, 0x69, 0x68, 0x75, 0x62, 0x2e, 0x76, 0x31, 0x2e, 0x48, 0x6f, 0x73,
	0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61,
	0x74, 0x69, 0x6f, 0x6e, 0x52, 0x18, 0x68, 0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63,
	0x74, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x12, 0x26,
	0x0a, 0x0f, 0x6e, 0x65, 0x78, 0x74, 0x5f, 0x70, 0x61, 0x67, 0x65, 0x5f, 0x74, 0x6f, 0x6b, 0x65,
	0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0d, 0x6e, 0x65, 0x78, 0x74, 0x50, 0x61, 0x67,
	0x65, 0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x22, 0x94, 0x03, 0x0a, 0x17, 0x48, 0x6f, 0x73, 0x74, 0x50,
	0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69,
	0x6f, 0x6e, 0x12, 0x17, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09,
	0x42, 0x03, 0xe0, 0x41, 0x08, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x57, 0x0a, 0x0b, 0x67,
	0x63, 0x70, 0x5f, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09,
	0x42, 0x36, 0xe0, 0x41, 0x02, 0xe0, 0x41, 0x05, 0xfa, 0x41, 0x2d, 0x0a, 0x2b, 0x63, 0x6c, 0x6f,
	0x75, 0x64, 0x72, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x6d, 0x61, 0x6e, 0x61, 0x67, 0x65,
	0x72, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x61, 0x70, 0x69, 0x73, 0x2e, 0x63, 0x6f, 0x6d,
	0x2f, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52, 0x0a, 0x67, 0x63, 0x70, 0x50, 0x72, 0x6f,
	0x6a, 0x65, 0x63, 0x74, 0x12, 0x40, 0x0a, 0x0b, 0x63, 0x72, 0x65, 0x61, 0x74, 0x65, 0x5f, 0x74,
	0x69, 0x6d, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65,
	0x73, 0x74, 0x61, 0x6d, 0x70, 0x42, 0x03, 0xe0, 0x41, 0x03, 0x52, 0x0a, 0x63, 0x72, 0x65, 0x61,
	0x74, 0x65, 0x54, 0x69, 0x6d, 0x65, 0x3a, 0xc4, 0x01, 0xea, 0x41, 0xc0, 0x01, 0x0a, 0x2d, 0x61,
	0x70, 0x69, 0x68, 0x75, 0x62, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x61, 0x70, 0x69, 0x73,
	0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x48, 0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74,
	0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x5c, 0x70, 0x72,
	0x6f, 0x6a, 0x65, 0x63, 0x74, 0x73, 0x2f, 0x7b, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x7d,
	0x2f, 0x6c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2f, 0x7b, 0x6c, 0x6f, 0x63, 0x61,
	0x74, 0x69, 0x6f, 0x6e, 0x7d, 0x2f, 0x68, 0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63,
	0x74, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2f, 0x7b,
	0x68, 0x6f, 0x73, 0x74, 0x5f, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x5f, 0x72, 0x65, 0x67,
	0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x7d, 0x2a, 0x18, 0x68, 0x6f, 0x73, 0x74,
	0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x73, 0x32, 0x17, 0x68, 0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63,
	0x74, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x32, 0xe1, 0x06,
	0x0a, 0x1e, 0x48, 0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52, 0x65, 0x67,
	0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65,
	0x12, 0xb0, 0x02, 0x0a, 0x1d, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x48, 0x6f, 0x73, 0x74, 0x50,
	0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69,
	0x6f, 0x6e, 0x12, 0x3c, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x63, 0x6c, 0x6f, 0x75,
	0x64, 0x2e, 0x61, 0x70, 0x69, 0x68, 0x75, 0x62, 0x2e, 0x76, 0x31, 0x2e, 0x43, 0x72, 0x65, 0x61,
	0x74, 0x65, 0x48, 0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52, 0x65, 0x67,
	0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x1a, 0x2f, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e,
	0x61, 0x70, 0x69, 0x68, 0x75, 0x62, 0x2e, 0x76, 0x31, 0x2e, 0x48, 0x6f, 0x73, 0x74, 0x50, 0x72,
	0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f,
	0x6e, 0x22, 0x9f, 0x01, 0xda, 0x41, 0x3d, 0x70, 0x61, 0x72, 0x65, 0x6e, 0x74, 0x2c, 0x68, 0x6f,
	0x73, 0x74, 0x5f, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x5f, 0x72, 0x65, 0x67, 0x69, 0x73,
	0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2c, 0x68, 0x6f, 0x73, 0x74, 0x5f, 0x70, 0x72, 0x6f,
	0x6a, 0x65, 0x63, 0x74, 0x5f, 0x72, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f,
	0x6e, 0x5f, 0x69, 0x64, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x59, 0x3a, 0x19, 0x68, 0x6f, 0x73, 0x74,
	0x5f, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x5f, 0x72, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72,
	0x61, 0x74, 0x69, 0x6f, 0x6e, 0x22, 0x3c, 0x2f, 0x76, 0x31, 0x2f, 0x7b, 0x70, 0x61, 0x72, 0x65,
	0x6e, 0x74, 0x3d, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x73, 0x2f, 0x2a, 0x2f, 0x6c, 0x6f,
	0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2f, 0x2a, 0x7d, 0x2f, 0x68, 0x6f, 0x73, 0x74, 0x50,
	0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69,
	0x6f, 0x6e, 0x73, 0x12, 0xd5, 0x01, 0x0a, 0x1a, 0x47, 0x65, 0x74, 0x48, 0x6f, 0x73, 0x74, 0x50,
	0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69,
	0x6f, 0x6e, 0x12, 0x39, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x63, 0x6c, 0x6f, 0x75,
	0x64, 0x2e, 0x61, 0x70, 0x69, 0x68, 0x75, 0x62, 0x2e, 0x76, 0x31, 0x2e, 0x47, 0x65, 0x74, 0x48,
	0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74,
	0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x2f, 0x2e,
	0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x61, 0x70, 0x69,
	0x68, 0x75, 0x62, 0x2e, 0x76, 0x31, 0x2e, 0x48, 0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65,
	0x63, 0x74, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x22, 0x4b,
	0xda, 0x41, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x3e, 0x12, 0x3c, 0x2f,
	0x76, 0x31, 0x2f, 0x7b, 0x6e, 0x61, 0x6d, 0x65, 0x3d, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74,
	0x73, 0x2f, 0x2a, 0x2f, 0x6c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2f, 0x2a, 0x2f,
	0x68, 0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52, 0x65, 0x67, 0x69, 0x73,
	0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2f, 0x2a, 0x7d, 0x12, 0xe8, 0x01, 0x0a, 0x1c,
	0x4c, 0x69, 0x73, 0x74, 0x48, 0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52,
	0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x12, 0x3b, 0x2e, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x61, 0x70, 0x69, 0x68,
	0x75, 0x62, 0x2e, 0x76, 0x31, 0x2e, 0x4c, 0x69, 0x73, 0x74, 0x48, 0x6f, 0x73, 0x74, 0x50, 0x72,
	0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f,
	0x6e, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x3c, 0x2e, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2e, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x61, 0x70, 0x69, 0x68, 0x75, 0x62, 0x2e,
	0x76, 0x31, 0x2e, 0x4c, 0x69, 0x73, 0x74, 0x48, 0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65,
	0x63, 0x74, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x4d, 0xda, 0x41, 0x06, 0x70, 0x61, 0x72, 0x65,
	0x6e, 0x74, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x3e, 0x12, 0x3c, 0x2f, 0x76, 0x31, 0x2f, 0x7b, 0x70,
	0x61, 0x72, 0x65, 0x6e, 0x74, 0x3d, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x73, 0x2f, 0x2a,
	0x2f, 0x6c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2f, 0x2a, 0x7d, 0x2f, 0x68, 0x6f,
	0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72,
	0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x1a, 0x49, 0xca, 0x41, 0x15, 0x61, 0x70, 0x69, 0x68, 0x75,
	0x62, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x61, 0x70, 0x69, 0x73, 0x2e, 0x63, 0x6f, 0x6d,
	0xd2, 0x41, 0x2e, 0x68, 0x74, 0x74, 0x70, 0x73, 0x3a, 0x2f, 0x2f, 0x77, 0x77, 0x77, 0x2e, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x61, 0x70, 0x69, 0x73, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x61, 0x75,
	0x74, 0x68, 0x2f, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2d, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72,
	0x6d, 0x42, 0xc5, 0x01, 0x0a, 0x1a, 0x63, 0x6f, 0x6d, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x61, 0x70, 0x69, 0x68, 0x75, 0x62, 0x2e, 0x76, 0x31,
	0x42, 0x23, 0x48, 0x6f, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x52, 0x65, 0x67,
	0x69, 0x73, 0x74, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65,
	0x50, 0x72, 0x6f, 0x74, 0x6f, 0x50, 0x01, 0x5a, 0x32, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x67, 0x6f, 0x2f, 0x61, 0x70, 0x69,
	0x68, 0x75, 0x62, 0x2f, 0x61, 0x70, 0x69, 0x76, 0x31, 0x2f, 0x61, 0x70, 0x69, 0x68, 0x75, 0x62,
	0x70, 0x62, 0x3b, 0x61, 0x70, 0x69, 0x68, 0x75, 0x62, 0x70, 0x62, 0xaa, 0x02, 0x16, 0x47, 0x6f,
	0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x43, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x41, 0x70, 0x69, 0x48, 0x75,
	0x62, 0x2e, 0x56, 0x31, 0xca, 0x02, 0x16, 0x47, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x5c, 0x43, 0x6c,
	0x6f, 0x75, 0x64, 0x5c, 0x41, 0x70, 0x69, 0x48, 0x75, 0x62, 0x5c, 0x56, 0x31, 0xea, 0x02, 0x19,
	0x47, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x3a, 0x3a, 0x43, 0x6c, 0x6f, 0x75, 0x64, 0x3a, 0x3a, 0x41,
	0x70, 0x69, 0x48, 0x75, 0x62, 0x3a, 0x3a, 0x56, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x33,
}

var (
	file_google_cloud_apihub_v1_host_project_registration_service_proto_rawDescOnce sync.Once
	file_google_cloud_apihub_v1_host_project_registration_service_proto_rawDescData = file_google_cloud_apihub_v1_host_project_registration_service_proto_rawDesc
)

func file_google_cloud_apihub_v1_host_project_registration_service_proto_rawDescGZIP() []byte {
	file_google_cloud_apihub_v1_host_project_registration_service_proto_rawDescOnce.Do(func() {
		file_google_cloud_apihub_v1_host_project_registration_service_proto_rawDescData = protoimpl.X.CompressGZIP(file_google_cloud_apihub_v1_host_project_registration_service_proto_rawDescData)
	})
	return file_google_cloud_apihub_v1_host_project_registration_service_proto_rawDescData
}

var file_google_cloud_apihub_v1_host_project_registration_service_proto_msgTypes = make([]protoimpl.MessageInfo, 5)
var file_google_cloud_apihub_v1_host_project_registration_service_proto_goTypes = []any{
	(*CreateHostProjectRegistrationRequest)(nil), // 0: google.cloud.apihub.v1.CreateHostProjectRegistrationRequest
	(*GetHostProjectRegistrationRequest)(nil),    // 1: google.cloud.apihub.v1.GetHostProjectRegistrationRequest
	(*ListHostProjectRegistrationsRequest)(nil),  // 2: google.cloud.apihub.v1.ListHostProjectRegistrationsRequest
	(*ListHostProjectRegistrationsResponse)(nil), // 3: google.cloud.apihub.v1.ListHostProjectRegistrationsResponse
	(*HostProjectRegistration)(nil),              // 4: google.cloud.apihub.v1.HostProjectRegistration
	(*timestamppb.Timestamp)(nil),                // 5: google.protobuf.Timestamp
}
var file_google_cloud_apihub_v1_host_project_registration_service_proto_depIdxs = []int32{
	4, // 0: google.cloud.apihub.v1.CreateHostProjectRegistrationRequest.host_project_registration:type_name -> google.cloud.apihub.v1.HostProjectRegistration
	4, // 1: google.cloud.apihub.v1.ListHostProjectRegistrationsResponse.host_project_registrations:type_name -> google.cloud.apihub.v1.HostProjectRegistration
	5, // 2: google.cloud.apihub.v1.HostProjectRegistration.create_time:type_name -> google.protobuf.Timestamp
	0, // 3: google.cloud.apihub.v1.HostProjectRegistrationService.CreateHostProjectRegistration:input_type -> google.cloud.apihub.v1.CreateHostProjectRegistrationRequest
	1, // 4: google.cloud.apihub.v1.HostProjectRegistrationService.GetHostProjectRegistration:input_type -> google.cloud.apihub.v1.GetHostProjectRegistrationRequest
	2, // 5: google.cloud.apihub.v1.HostProjectRegistrationService.ListHostProjectRegistrations:input_type -> google.cloud.apihub.v1.ListHostProjectRegistrationsRequest
	4, // 6: google.cloud.apihub.v1.HostProjectRegistrationService.CreateHostProjectRegistration:output_type -> google.cloud.apihub.v1.HostProjectRegistration
	4, // 7: google.cloud.apihub.v1.HostProjectRegistrationService.GetHostProjectRegistration:output_type -> google.cloud.apihub.v1.HostProjectRegistration
	3, // 8: google.cloud.apihub.v1.HostProjectRegistrationService.ListHostProjectRegistrations:output_type -> google.cloud.apihub.v1.ListHostProjectRegistrationsResponse
	6, // [6:9] is the sub-list for method output_type
	3, // [3:6] is the sub-list for method input_type
	3, // [3:3] is the sub-list for extension type_name
	3, // [3:3] is the sub-list for extension extendee
	0, // [0:3] is the sub-list for field type_name
}

func init() { file_google_cloud_apihub_v1_host_project_registration_service_proto_init() }
func file_google_cloud_apihub_v1_host_project_registration_service_proto_init() {
	if File_google_cloud_apihub_v1_host_project_registration_service_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_google_cloud_apihub_v1_host_project_registration_service_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   5,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_google_cloud_apihub_v1_host_project_registration_service_proto_goTypes,
		DependencyIndexes: file_google_cloud_apihub_v1_host_project_registration_service_proto_depIdxs,
		MessageInfos:      file_google_cloud_apihub_v1_host_project_registration_service_proto_msgTypes,
	}.Build()
	File_google_cloud_apihub_v1_host_project_registration_service_proto = out.File
	file_google_cloud_apihub_v1_host_project_registration_service_proto_rawDesc = nil
	file_google_cloud_apihub_v1_host_project_registration_service_proto_goTypes = nil
	file_google_cloud_apihub_v1_host_project_registration_service_proto_depIdxs = nil
}
