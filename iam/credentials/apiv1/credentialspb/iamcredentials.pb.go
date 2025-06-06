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
// source: google/iam/credentials/v1/iamcredentials.proto

package credentialspb

import (
	context "context"
	_ "google.golang.org/genproto/googleapis/api/annotations"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

var File_google_iam_credentials_v1_iamcredentials_proto protoreflect.FileDescriptor

var file_google_iam_credentials_v1_iamcredentials_proto_rawDesc = []byte{
	0x0a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x69, 0x61, 0x6d, 0x2f, 0x63, 0x72, 0x65,
	0x64, 0x65, 0x6e, 0x74, 0x69, 0x61, 0x6c, 0x73, 0x2f, 0x76, 0x31, 0x2f, 0x69, 0x61, 0x6d, 0x63,
	0x72, 0x65, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x61, 0x6c, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x12, 0x19, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x69, 0x61, 0x6d, 0x2e, 0x63, 0x72, 0x65,
	0x64, 0x65, 0x6e, 0x74, 0x69, 0x61, 0x6c, 0x73, 0x2e, 0x76, 0x31, 0x1a, 0x1c, 0x67, 0x6f, 0x6f,
	0x67, 0x6c, 0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x61, 0x6e, 0x6e, 0x6f, 0x74, 0x61, 0x74, 0x69,
	0x6f, 0x6e, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x17, 0x67, 0x6f, 0x6f, 0x67, 0x6c,
	0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x63, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x1a, 0x26, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x69, 0x61, 0x6d, 0x2f, 0x63,
	0x72, 0x65, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x61, 0x6c, 0x73, 0x2f, 0x76, 0x31, 0x2f, 0x63, 0x6f,
	0x6d, 0x6d, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x32, 0xad, 0x07, 0x0a, 0x0e, 0x49,
	0x41, 0x4d, 0x43, 0x72, 0x65, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x61, 0x6c, 0x73, 0x12, 0xec, 0x01,
	0x0a, 0x13, 0x47, 0x65, 0x6e, 0x65, 0x72, 0x61, 0x74, 0x65, 0x41, 0x63, 0x63, 0x65, 0x73, 0x73,
	0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x12, 0x35, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x69,
	0x61, 0x6d, 0x2e, 0x63, 0x72, 0x65, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x61, 0x6c, 0x73, 0x2e, 0x76,
	0x31, 0x2e, 0x47, 0x65, 0x6e, 0x65, 0x72, 0x61, 0x74, 0x65, 0x41, 0x63, 0x63, 0x65, 0x73, 0x73,
	0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x36, 0x2e, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x69, 0x61, 0x6d, 0x2e, 0x63, 0x72, 0x65, 0x64, 0x65, 0x6e,
	0x74, 0x69, 0x61, 0x6c, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x47, 0x65, 0x6e, 0x65, 0x72, 0x61, 0x74,
	0x65, 0x41, 0x63, 0x63, 0x65, 0x73, 0x73, 0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x22, 0x66, 0xda, 0x41, 0x1d, 0x6e, 0x61, 0x6d, 0x65, 0x2c, 0x64, 0x65,
	0x6c, 0x65, 0x67, 0x61, 0x74, 0x65, 0x73, 0x2c, 0x73, 0x63, 0x6f, 0x70, 0x65, 0x2c, 0x6c, 0x69,
	0x66, 0x65, 0x74, 0x69, 0x6d, 0x65, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x40, 0x3a, 0x01, 0x2a, 0x22,
	0x3b, 0x2f, 0x76, 0x31, 0x2f, 0x7b, 0x6e, 0x61, 0x6d, 0x65, 0x3d, 0x70, 0x72, 0x6f, 0x6a, 0x65,
	0x63, 0x74, 0x73, 0x2f, 0x2a, 0x2f, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x41, 0x63, 0x63,
	0x6f, 0x75, 0x6e, 0x74, 0x73, 0x2f, 0x2a, 0x7d, 0x3a, 0x67, 0x65, 0x6e, 0x65, 0x72, 0x61, 0x74,
	0x65, 0x41, 0x63, 0x63, 0x65, 0x73, 0x73, 0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x12, 0xe4, 0x01, 0x0a,
	0x0f, 0x47, 0x65, 0x6e, 0x65, 0x72, 0x61, 0x74, 0x65, 0x49, 0x64, 0x54, 0x6f, 0x6b, 0x65, 0x6e,
	0x12, 0x31, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x69, 0x61, 0x6d, 0x2e, 0x63, 0x72,
	0x65, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x61, 0x6c, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x47, 0x65, 0x6e,
	0x65, 0x72, 0x61, 0x74, 0x65, 0x49, 0x64, 0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x1a, 0x32, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x69, 0x61, 0x6d,
	0x2e, 0x63, 0x72, 0x65, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x61, 0x6c, 0x73, 0x2e, 0x76, 0x31, 0x2e,
	0x47, 0x65, 0x6e, 0x65, 0x72, 0x61, 0x74, 0x65, 0x49, 0x64, 0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x6a, 0xda, 0x41, 0x25, 0x6e, 0x61, 0x6d, 0x65,
	0x2c, 0x64, 0x65, 0x6c, 0x65, 0x67, 0x61, 0x74, 0x65, 0x73, 0x2c, 0x61, 0x75, 0x64, 0x69, 0x65,
	0x6e, 0x63, 0x65, 0x2c, 0x69, 0x6e, 0x63, 0x6c, 0x75, 0x64, 0x65, 0x5f, 0x65, 0x6d, 0x61, 0x69,
	0x6c, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x3c, 0x3a, 0x01, 0x2a, 0x22, 0x37, 0x2f, 0x76, 0x31, 0x2f,
	0x7b, 0x6e, 0x61, 0x6d, 0x65, 0x3d, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x73, 0x2f, 0x2a,
	0x2f, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x41, 0x63, 0x63, 0x6f, 0x75, 0x6e, 0x74, 0x73,
	0x2f, 0x2a, 0x7d, 0x3a, 0x67, 0x65, 0x6e, 0x65, 0x72, 0x61, 0x74, 0x65, 0x49, 0x64, 0x54, 0x6f,
	0x6b, 0x65, 0x6e, 0x12, 0xb9, 0x01, 0x0a, 0x08, 0x53, 0x69, 0x67, 0x6e, 0x42, 0x6c, 0x6f, 0x62,
	0x12, 0x2a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x69, 0x61, 0x6d, 0x2e, 0x63, 0x72,
	0x65, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x61, 0x6c, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x53, 0x69, 0x67,
	0x6e, 0x42, 0x6c, 0x6f, 0x62, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x2b, 0x2e, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x69, 0x61, 0x6d, 0x2e, 0x63, 0x72, 0x65, 0x64, 0x65, 0x6e,
	0x74, 0x69, 0x61, 0x6c, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x53, 0x69, 0x67, 0x6e, 0x42, 0x6c, 0x6f,
	0x62, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x54, 0xda, 0x41, 0x16, 0x6e, 0x61,
	0x6d, 0x65, 0x2c, 0x64, 0x65, 0x6c, 0x65, 0x67, 0x61, 0x74, 0x65, 0x73, 0x2c, 0x70, 0x61, 0x79,
	0x6c, 0x6f, 0x61, 0x64, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x35, 0x3a, 0x01, 0x2a, 0x22, 0x30, 0x2f,
	0x76, 0x31, 0x2f, 0x7b, 0x6e, 0x61, 0x6d, 0x65, 0x3d, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74,
	0x73, 0x2f, 0x2a, 0x2f, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x41, 0x63, 0x63, 0x6f, 0x75,
	0x6e, 0x74, 0x73, 0x2f, 0x2a, 0x7d, 0x3a, 0x73, 0x69, 0x67, 0x6e, 0x42, 0x6c, 0x6f, 0x62, 0x12,
	0xb5, 0x01, 0x0a, 0x07, 0x53, 0x69, 0x67, 0x6e, 0x4a, 0x77, 0x74, 0x12, 0x29, 0x2e, 0x67, 0x6f,
	0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x69, 0x61, 0x6d, 0x2e, 0x63, 0x72, 0x65, 0x64, 0x65, 0x6e, 0x74,
	0x69, 0x61, 0x6c, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x53, 0x69, 0x67, 0x6e, 0x4a, 0x77, 0x74, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x2a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e,
	0x69, 0x61, 0x6d, 0x2e, 0x63, 0x72, 0x65, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x61, 0x6c, 0x73, 0x2e,
	0x76, 0x31, 0x2e, 0x53, 0x69, 0x67, 0x6e, 0x4a, 0x77, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x22, 0x53, 0xda, 0x41, 0x16, 0x6e, 0x61, 0x6d, 0x65, 0x2c, 0x64, 0x65, 0x6c, 0x65,
	0x67, 0x61, 0x74, 0x65, 0x73, 0x2c, 0x70, 0x61, 0x79, 0x6c, 0x6f, 0x61, 0x64, 0x82, 0xd3, 0xe4,
	0x93, 0x02, 0x34, 0x3a, 0x01, 0x2a, 0x22, 0x2f, 0x2f, 0x76, 0x31, 0x2f, 0x7b, 0x6e, 0x61, 0x6d,
	0x65, 0x3d, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x73, 0x2f, 0x2a, 0x2f, 0x73, 0x65, 0x72,
	0x76, 0x69, 0x63, 0x65, 0x41, 0x63, 0x63, 0x6f, 0x75, 0x6e, 0x74, 0x73, 0x2f, 0x2a, 0x7d, 0x3a,
	0x73, 0x69, 0x67, 0x6e, 0x4a, 0x77, 0x74, 0x1a, 0x51, 0xca, 0x41, 0x1d, 0x69, 0x61, 0x6d, 0x63,
	0x72, 0x65, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x61, 0x6c, 0x73, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c,
	0x65, 0x61, 0x70, 0x69, 0x73, 0x2e, 0x63, 0x6f, 0x6d, 0xd2, 0x41, 0x2e, 0x68, 0x74, 0x74, 0x70,
	0x73, 0x3a, 0x2f, 0x2f, 0x77, 0x77, 0x77, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x61, 0x70,
	0x69, 0x73, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x61, 0x75, 0x74, 0x68, 0x2f, 0x63, 0x6c, 0x6f, 0x75,
	0x64, 0x2d, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x42, 0xca, 0x01, 0x0a, 0x23, 0x63,
	0x6f, 0x6d, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e,
	0x69, 0x61, 0x6d, 0x2e, 0x63, 0x72, 0x65, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x61, 0x6c, 0x73, 0x2e,
	0x76, 0x31, 0x42, 0x13, 0x49, 0x41, 0x4d, 0x43, 0x72, 0x65, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x61,
	0x6c, 0x73, 0x50, 0x72, 0x6f, 0x74, 0x6f, 0x50, 0x01, 0x5a, 0x45, 0x63, 0x6c, 0x6f, 0x75, 0x64,
	0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x67, 0x6f, 0x2f, 0x69,
	0x61, 0x6d, 0x2f, 0x63, 0x72, 0x65, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x61, 0x6c, 0x73, 0x2f, 0x61,
	0x70, 0x69, 0x76, 0x31, 0x2f, 0x63, 0x72, 0x65, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x61, 0x6c, 0x73,
	0x70, 0x62, 0x3b, 0x63, 0x72, 0x65, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x61, 0x6c, 0x73, 0x70, 0x62,
	0xf8, 0x01, 0x01, 0xaa, 0x02, 0x1f, 0x47, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x43, 0x6c, 0x6f,
	0x75, 0x64, 0x2e, 0x49, 0x61, 0x6d, 0x2e, 0x43, 0x72, 0x65, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x61,
	0x6c, 0x73, 0x2e, 0x56, 0x31, 0xca, 0x02, 0x1f, 0x47, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x5c, 0x43,
	0x6c, 0x6f, 0x75, 0x64, 0x5c, 0x49, 0x61, 0x6d, 0x5c, 0x43, 0x72, 0x65, 0x64, 0x65, 0x6e, 0x74,
	0x69, 0x61, 0x6c, 0x73, 0x5c, 0x56, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var file_google_iam_credentials_v1_iamcredentials_proto_goTypes = []any{
	(*GenerateAccessTokenRequest)(nil),  // 0: google.iam.credentials.v1.GenerateAccessTokenRequest
	(*GenerateIdTokenRequest)(nil),      // 1: google.iam.credentials.v1.GenerateIdTokenRequest
	(*SignBlobRequest)(nil),             // 2: google.iam.credentials.v1.SignBlobRequest
	(*SignJwtRequest)(nil),              // 3: google.iam.credentials.v1.SignJwtRequest
	(*GenerateAccessTokenResponse)(nil), // 4: google.iam.credentials.v1.GenerateAccessTokenResponse
	(*GenerateIdTokenResponse)(nil),     // 5: google.iam.credentials.v1.GenerateIdTokenResponse
	(*SignBlobResponse)(nil),            // 6: google.iam.credentials.v1.SignBlobResponse
	(*SignJwtResponse)(nil),             // 7: google.iam.credentials.v1.SignJwtResponse
}
var file_google_iam_credentials_v1_iamcredentials_proto_depIdxs = []int32{
	0, // 0: google.iam.credentials.v1.IAMCredentials.GenerateAccessToken:input_type -> google.iam.credentials.v1.GenerateAccessTokenRequest
	1, // 1: google.iam.credentials.v1.IAMCredentials.GenerateIdToken:input_type -> google.iam.credentials.v1.GenerateIdTokenRequest
	2, // 2: google.iam.credentials.v1.IAMCredentials.SignBlob:input_type -> google.iam.credentials.v1.SignBlobRequest
	3, // 3: google.iam.credentials.v1.IAMCredentials.SignJwt:input_type -> google.iam.credentials.v1.SignJwtRequest
	4, // 4: google.iam.credentials.v1.IAMCredentials.GenerateAccessToken:output_type -> google.iam.credentials.v1.GenerateAccessTokenResponse
	5, // 5: google.iam.credentials.v1.IAMCredentials.GenerateIdToken:output_type -> google.iam.credentials.v1.GenerateIdTokenResponse
	6, // 6: google.iam.credentials.v1.IAMCredentials.SignBlob:output_type -> google.iam.credentials.v1.SignBlobResponse
	7, // 7: google.iam.credentials.v1.IAMCredentials.SignJwt:output_type -> google.iam.credentials.v1.SignJwtResponse
	4, // [4:8] is the sub-list for method output_type
	0, // [0:4] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_google_iam_credentials_v1_iamcredentials_proto_init() }
func file_google_iam_credentials_v1_iamcredentials_proto_init() {
	if File_google_iam_credentials_v1_iamcredentials_proto != nil {
		return
	}
	file_google_iam_credentials_v1_common_proto_init()
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_google_iam_credentials_v1_iamcredentials_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   0,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_google_iam_credentials_v1_iamcredentials_proto_goTypes,
		DependencyIndexes: file_google_iam_credentials_v1_iamcredentials_proto_depIdxs,
	}.Build()
	File_google_iam_credentials_v1_iamcredentials_proto = out.File
	file_google_iam_credentials_v1_iamcredentials_proto_rawDesc = nil
	file_google_iam_credentials_v1_iamcredentials_proto_goTypes = nil
	file_google_iam_credentials_v1_iamcredentials_proto_depIdxs = nil
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConnInterface

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion6

// IAMCredentialsClient is the client API for IAMCredentials service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type IAMCredentialsClient interface {
	// Generates an OAuth 2.0 access token for a service account.
	GenerateAccessToken(ctx context.Context, in *GenerateAccessTokenRequest, opts ...grpc.CallOption) (*GenerateAccessTokenResponse, error)
	// Generates an OpenID Connect ID token for a service account.
	GenerateIdToken(ctx context.Context, in *GenerateIdTokenRequest, opts ...grpc.CallOption) (*GenerateIdTokenResponse, error)
	// Signs a blob using a service account's system-managed private key.
	SignBlob(ctx context.Context, in *SignBlobRequest, opts ...grpc.CallOption) (*SignBlobResponse, error)
	// Signs a JWT using a service account's system-managed private key.
	SignJwt(ctx context.Context, in *SignJwtRequest, opts ...grpc.CallOption) (*SignJwtResponse, error)
}

type iAMCredentialsClient struct {
	cc grpc.ClientConnInterface
}

func NewIAMCredentialsClient(cc grpc.ClientConnInterface) IAMCredentialsClient {
	return &iAMCredentialsClient{cc}
}

func (c *iAMCredentialsClient) GenerateAccessToken(ctx context.Context, in *GenerateAccessTokenRequest, opts ...grpc.CallOption) (*GenerateAccessTokenResponse, error) {
	out := new(GenerateAccessTokenResponse)
	err := c.cc.Invoke(ctx, "/google.iam.credentials.v1.IAMCredentials/GenerateAccessToken", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *iAMCredentialsClient) GenerateIdToken(ctx context.Context, in *GenerateIdTokenRequest, opts ...grpc.CallOption) (*GenerateIdTokenResponse, error) {
	out := new(GenerateIdTokenResponse)
	err := c.cc.Invoke(ctx, "/google.iam.credentials.v1.IAMCredentials/GenerateIdToken", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *iAMCredentialsClient) SignBlob(ctx context.Context, in *SignBlobRequest, opts ...grpc.CallOption) (*SignBlobResponse, error) {
	out := new(SignBlobResponse)
	err := c.cc.Invoke(ctx, "/google.iam.credentials.v1.IAMCredentials/SignBlob", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *iAMCredentialsClient) SignJwt(ctx context.Context, in *SignJwtRequest, opts ...grpc.CallOption) (*SignJwtResponse, error) {
	out := new(SignJwtResponse)
	err := c.cc.Invoke(ctx, "/google.iam.credentials.v1.IAMCredentials/SignJwt", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// IAMCredentialsServer is the server API for IAMCredentials service.
type IAMCredentialsServer interface {
	// Generates an OAuth 2.0 access token for a service account.
	GenerateAccessToken(context.Context, *GenerateAccessTokenRequest) (*GenerateAccessTokenResponse, error)
	// Generates an OpenID Connect ID token for a service account.
	GenerateIdToken(context.Context, *GenerateIdTokenRequest) (*GenerateIdTokenResponse, error)
	// Signs a blob using a service account's system-managed private key.
	SignBlob(context.Context, *SignBlobRequest) (*SignBlobResponse, error)
	// Signs a JWT using a service account's system-managed private key.
	SignJwt(context.Context, *SignJwtRequest) (*SignJwtResponse, error)
}

// UnimplementedIAMCredentialsServer can be embedded to have forward compatible implementations.
type UnimplementedIAMCredentialsServer struct {
}

func (*UnimplementedIAMCredentialsServer) GenerateAccessToken(context.Context, *GenerateAccessTokenRequest) (*GenerateAccessTokenResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GenerateAccessToken not implemented")
}
func (*UnimplementedIAMCredentialsServer) GenerateIdToken(context.Context, *GenerateIdTokenRequest) (*GenerateIdTokenResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GenerateIdToken not implemented")
}
func (*UnimplementedIAMCredentialsServer) SignBlob(context.Context, *SignBlobRequest) (*SignBlobResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SignBlob not implemented")
}
func (*UnimplementedIAMCredentialsServer) SignJwt(context.Context, *SignJwtRequest) (*SignJwtResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SignJwt not implemented")
}

func RegisterIAMCredentialsServer(s *grpc.Server, srv IAMCredentialsServer) {
	s.RegisterService(&_IAMCredentials_serviceDesc, srv)
}

func _IAMCredentials_GenerateAccessToken_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GenerateAccessTokenRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(IAMCredentialsServer).GenerateAccessToken(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.iam.credentials.v1.IAMCredentials/GenerateAccessToken",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(IAMCredentialsServer).GenerateAccessToken(ctx, req.(*GenerateAccessTokenRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _IAMCredentials_GenerateIdToken_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GenerateIdTokenRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(IAMCredentialsServer).GenerateIdToken(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.iam.credentials.v1.IAMCredentials/GenerateIdToken",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(IAMCredentialsServer).GenerateIdToken(ctx, req.(*GenerateIdTokenRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _IAMCredentials_SignBlob_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SignBlobRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(IAMCredentialsServer).SignBlob(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.iam.credentials.v1.IAMCredentials/SignBlob",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(IAMCredentialsServer).SignBlob(ctx, req.(*SignBlobRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _IAMCredentials_SignJwt_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SignJwtRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(IAMCredentialsServer).SignJwt(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.iam.credentials.v1.IAMCredentials/SignJwt",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(IAMCredentialsServer).SignJwt(ctx, req.(*SignJwtRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _IAMCredentials_serviceDesc = grpc.ServiceDesc{
	ServiceName: "google.iam.credentials.v1.IAMCredentials",
	HandlerType: (*IAMCredentialsServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GenerateAccessToken",
			Handler:    _IAMCredentials_GenerateAccessToken_Handler,
		},
		{
			MethodName: "GenerateIdToken",
			Handler:    _IAMCredentials_GenerateIdToken_Handler,
		},
		{
			MethodName: "SignBlob",
			Handler:    _IAMCredentials_SignBlob_Handler,
		},
		{
			MethodName: "SignJwt",
			Handler:    _IAMCredentials_SignJwt_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "google/iam/credentials/v1/iamcredentials.proto",
}
