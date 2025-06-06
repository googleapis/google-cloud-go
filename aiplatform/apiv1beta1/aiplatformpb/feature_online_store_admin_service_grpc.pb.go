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

// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.3.0
// - protoc             v4.25.7
// source: google/cloud/aiplatform/v1beta1/feature_online_store_admin_service.proto

package aiplatformpb

import (
	longrunningpb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

const (
	FeatureOnlineStoreAdminService_CreateFeatureOnlineStore_FullMethodName = "/google.cloud.aiplatform.v1beta1.FeatureOnlineStoreAdminService/CreateFeatureOnlineStore"
	FeatureOnlineStoreAdminService_GetFeatureOnlineStore_FullMethodName    = "/google.cloud.aiplatform.v1beta1.FeatureOnlineStoreAdminService/GetFeatureOnlineStore"
	FeatureOnlineStoreAdminService_ListFeatureOnlineStores_FullMethodName  = "/google.cloud.aiplatform.v1beta1.FeatureOnlineStoreAdminService/ListFeatureOnlineStores"
	FeatureOnlineStoreAdminService_UpdateFeatureOnlineStore_FullMethodName = "/google.cloud.aiplatform.v1beta1.FeatureOnlineStoreAdminService/UpdateFeatureOnlineStore"
	FeatureOnlineStoreAdminService_DeleteFeatureOnlineStore_FullMethodName = "/google.cloud.aiplatform.v1beta1.FeatureOnlineStoreAdminService/DeleteFeatureOnlineStore"
	FeatureOnlineStoreAdminService_CreateFeatureView_FullMethodName        = "/google.cloud.aiplatform.v1beta1.FeatureOnlineStoreAdminService/CreateFeatureView"
	FeatureOnlineStoreAdminService_GetFeatureView_FullMethodName           = "/google.cloud.aiplatform.v1beta1.FeatureOnlineStoreAdminService/GetFeatureView"
	FeatureOnlineStoreAdminService_ListFeatureViews_FullMethodName         = "/google.cloud.aiplatform.v1beta1.FeatureOnlineStoreAdminService/ListFeatureViews"
	FeatureOnlineStoreAdminService_UpdateFeatureView_FullMethodName        = "/google.cloud.aiplatform.v1beta1.FeatureOnlineStoreAdminService/UpdateFeatureView"
	FeatureOnlineStoreAdminService_DeleteFeatureView_FullMethodName        = "/google.cloud.aiplatform.v1beta1.FeatureOnlineStoreAdminService/DeleteFeatureView"
	FeatureOnlineStoreAdminService_SyncFeatureView_FullMethodName          = "/google.cloud.aiplatform.v1beta1.FeatureOnlineStoreAdminService/SyncFeatureView"
	FeatureOnlineStoreAdminService_GetFeatureViewSync_FullMethodName       = "/google.cloud.aiplatform.v1beta1.FeatureOnlineStoreAdminService/GetFeatureViewSync"
	FeatureOnlineStoreAdminService_ListFeatureViewSyncs_FullMethodName     = "/google.cloud.aiplatform.v1beta1.FeatureOnlineStoreAdminService/ListFeatureViewSyncs"
)

// FeatureOnlineStoreAdminServiceClient is the client API for FeatureOnlineStoreAdminService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type FeatureOnlineStoreAdminServiceClient interface {
	// Creates a new FeatureOnlineStore in a given project and location.
	CreateFeatureOnlineStore(ctx context.Context, in *CreateFeatureOnlineStoreRequest, opts ...grpc.CallOption) (*longrunningpb.Operation, error)
	// Gets details of a single FeatureOnlineStore.
	GetFeatureOnlineStore(ctx context.Context, in *GetFeatureOnlineStoreRequest, opts ...grpc.CallOption) (*FeatureOnlineStore, error)
	// Lists FeatureOnlineStores in a given project and location.
	ListFeatureOnlineStores(ctx context.Context, in *ListFeatureOnlineStoresRequest, opts ...grpc.CallOption) (*ListFeatureOnlineStoresResponse, error)
	// Updates the parameters of a single FeatureOnlineStore.
	UpdateFeatureOnlineStore(ctx context.Context, in *UpdateFeatureOnlineStoreRequest, opts ...grpc.CallOption) (*longrunningpb.Operation, error)
	// Deletes a single FeatureOnlineStore. The FeatureOnlineStore must not
	// contain any FeatureViews.
	DeleteFeatureOnlineStore(ctx context.Context, in *DeleteFeatureOnlineStoreRequest, opts ...grpc.CallOption) (*longrunningpb.Operation, error)
	// Creates a new FeatureView in a given FeatureOnlineStore.
	CreateFeatureView(ctx context.Context, in *CreateFeatureViewRequest, opts ...grpc.CallOption) (*longrunningpb.Operation, error)
	// Gets details of a single FeatureView.
	GetFeatureView(ctx context.Context, in *GetFeatureViewRequest, opts ...grpc.CallOption) (*FeatureView, error)
	// Lists FeatureViews in a given FeatureOnlineStore.
	ListFeatureViews(ctx context.Context, in *ListFeatureViewsRequest, opts ...grpc.CallOption) (*ListFeatureViewsResponse, error)
	// Updates the parameters of a single FeatureView.
	UpdateFeatureView(ctx context.Context, in *UpdateFeatureViewRequest, opts ...grpc.CallOption) (*longrunningpb.Operation, error)
	// Deletes a single FeatureView.
	DeleteFeatureView(ctx context.Context, in *DeleteFeatureViewRequest, opts ...grpc.CallOption) (*longrunningpb.Operation, error)
	// Triggers on-demand sync for the FeatureView.
	SyncFeatureView(ctx context.Context, in *SyncFeatureViewRequest, opts ...grpc.CallOption) (*SyncFeatureViewResponse, error)
	// Gets details of a single FeatureViewSync.
	GetFeatureViewSync(ctx context.Context, in *GetFeatureViewSyncRequest, opts ...grpc.CallOption) (*FeatureViewSync, error)
	// Lists FeatureViewSyncs in a given FeatureView.
	ListFeatureViewSyncs(ctx context.Context, in *ListFeatureViewSyncsRequest, opts ...grpc.CallOption) (*ListFeatureViewSyncsResponse, error)
}

type featureOnlineStoreAdminServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewFeatureOnlineStoreAdminServiceClient(cc grpc.ClientConnInterface) FeatureOnlineStoreAdminServiceClient {
	return &featureOnlineStoreAdminServiceClient{cc}
}

func (c *featureOnlineStoreAdminServiceClient) CreateFeatureOnlineStore(ctx context.Context, in *CreateFeatureOnlineStoreRequest, opts ...grpc.CallOption) (*longrunningpb.Operation, error) {
	out := new(longrunningpb.Operation)
	err := c.cc.Invoke(ctx, FeatureOnlineStoreAdminService_CreateFeatureOnlineStore_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *featureOnlineStoreAdminServiceClient) GetFeatureOnlineStore(ctx context.Context, in *GetFeatureOnlineStoreRequest, opts ...grpc.CallOption) (*FeatureOnlineStore, error) {
	out := new(FeatureOnlineStore)
	err := c.cc.Invoke(ctx, FeatureOnlineStoreAdminService_GetFeatureOnlineStore_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *featureOnlineStoreAdminServiceClient) ListFeatureOnlineStores(ctx context.Context, in *ListFeatureOnlineStoresRequest, opts ...grpc.CallOption) (*ListFeatureOnlineStoresResponse, error) {
	out := new(ListFeatureOnlineStoresResponse)
	err := c.cc.Invoke(ctx, FeatureOnlineStoreAdminService_ListFeatureOnlineStores_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *featureOnlineStoreAdminServiceClient) UpdateFeatureOnlineStore(ctx context.Context, in *UpdateFeatureOnlineStoreRequest, opts ...grpc.CallOption) (*longrunningpb.Operation, error) {
	out := new(longrunningpb.Operation)
	err := c.cc.Invoke(ctx, FeatureOnlineStoreAdminService_UpdateFeatureOnlineStore_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *featureOnlineStoreAdminServiceClient) DeleteFeatureOnlineStore(ctx context.Context, in *DeleteFeatureOnlineStoreRequest, opts ...grpc.CallOption) (*longrunningpb.Operation, error) {
	out := new(longrunningpb.Operation)
	err := c.cc.Invoke(ctx, FeatureOnlineStoreAdminService_DeleteFeatureOnlineStore_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *featureOnlineStoreAdminServiceClient) CreateFeatureView(ctx context.Context, in *CreateFeatureViewRequest, opts ...grpc.CallOption) (*longrunningpb.Operation, error) {
	out := new(longrunningpb.Operation)
	err := c.cc.Invoke(ctx, FeatureOnlineStoreAdminService_CreateFeatureView_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *featureOnlineStoreAdminServiceClient) GetFeatureView(ctx context.Context, in *GetFeatureViewRequest, opts ...grpc.CallOption) (*FeatureView, error) {
	out := new(FeatureView)
	err := c.cc.Invoke(ctx, FeatureOnlineStoreAdminService_GetFeatureView_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *featureOnlineStoreAdminServiceClient) ListFeatureViews(ctx context.Context, in *ListFeatureViewsRequest, opts ...grpc.CallOption) (*ListFeatureViewsResponse, error) {
	out := new(ListFeatureViewsResponse)
	err := c.cc.Invoke(ctx, FeatureOnlineStoreAdminService_ListFeatureViews_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *featureOnlineStoreAdminServiceClient) UpdateFeatureView(ctx context.Context, in *UpdateFeatureViewRequest, opts ...grpc.CallOption) (*longrunningpb.Operation, error) {
	out := new(longrunningpb.Operation)
	err := c.cc.Invoke(ctx, FeatureOnlineStoreAdminService_UpdateFeatureView_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *featureOnlineStoreAdminServiceClient) DeleteFeatureView(ctx context.Context, in *DeleteFeatureViewRequest, opts ...grpc.CallOption) (*longrunningpb.Operation, error) {
	out := new(longrunningpb.Operation)
	err := c.cc.Invoke(ctx, FeatureOnlineStoreAdminService_DeleteFeatureView_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *featureOnlineStoreAdminServiceClient) SyncFeatureView(ctx context.Context, in *SyncFeatureViewRequest, opts ...grpc.CallOption) (*SyncFeatureViewResponse, error) {
	out := new(SyncFeatureViewResponse)
	err := c.cc.Invoke(ctx, FeatureOnlineStoreAdminService_SyncFeatureView_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *featureOnlineStoreAdminServiceClient) GetFeatureViewSync(ctx context.Context, in *GetFeatureViewSyncRequest, opts ...grpc.CallOption) (*FeatureViewSync, error) {
	out := new(FeatureViewSync)
	err := c.cc.Invoke(ctx, FeatureOnlineStoreAdminService_GetFeatureViewSync_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *featureOnlineStoreAdminServiceClient) ListFeatureViewSyncs(ctx context.Context, in *ListFeatureViewSyncsRequest, opts ...grpc.CallOption) (*ListFeatureViewSyncsResponse, error) {
	out := new(ListFeatureViewSyncsResponse)
	err := c.cc.Invoke(ctx, FeatureOnlineStoreAdminService_ListFeatureViewSyncs_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// FeatureOnlineStoreAdminServiceServer is the server API for FeatureOnlineStoreAdminService service.
// All implementations should embed UnimplementedFeatureOnlineStoreAdminServiceServer
// for forward compatibility
type FeatureOnlineStoreAdminServiceServer interface {
	// Creates a new FeatureOnlineStore in a given project and location.
	CreateFeatureOnlineStore(context.Context, *CreateFeatureOnlineStoreRequest) (*longrunningpb.Operation, error)
	// Gets details of a single FeatureOnlineStore.
	GetFeatureOnlineStore(context.Context, *GetFeatureOnlineStoreRequest) (*FeatureOnlineStore, error)
	// Lists FeatureOnlineStores in a given project and location.
	ListFeatureOnlineStores(context.Context, *ListFeatureOnlineStoresRequest) (*ListFeatureOnlineStoresResponse, error)
	// Updates the parameters of a single FeatureOnlineStore.
	UpdateFeatureOnlineStore(context.Context, *UpdateFeatureOnlineStoreRequest) (*longrunningpb.Operation, error)
	// Deletes a single FeatureOnlineStore. The FeatureOnlineStore must not
	// contain any FeatureViews.
	DeleteFeatureOnlineStore(context.Context, *DeleteFeatureOnlineStoreRequest) (*longrunningpb.Operation, error)
	// Creates a new FeatureView in a given FeatureOnlineStore.
	CreateFeatureView(context.Context, *CreateFeatureViewRequest) (*longrunningpb.Operation, error)
	// Gets details of a single FeatureView.
	GetFeatureView(context.Context, *GetFeatureViewRequest) (*FeatureView, error)
	// Lists FeatureViews in a given FeatureOnlineStore.
	ListFeatureViews(context.Context, *ListFeatureViewsRequest) (*ListFeatureViewsResponse, error)
	// Updates the parameters of a single FeatureView.
	UpdateFeatureView(context.Context, *UpdateFeatureViewRequest) (*longrunningpb.Operation, error)
	// Deletes a single FeatureView.
	DeleteFeatureView(context.Context, *DeleteFeatureViewRequest) (*longrunningpb.Operation, error)
	// Triggers on-demand sync for the FeatureView.
	SyncFeatureView(context.Context, *SyncFeatureViewRequest) (*SyncFeatureViewResponse, error)
	// Gets details of a single FeatureViewSync.
	GetFeatureViewSync(context.Context, *GetFeatureViewSyncRequest) (*FeatureViewSync, error)
	// Lists FeatureViewSyncs in a given FeatureView.
	ListFeatureViewSyncs(context.Context, *ListFeatureViewSyncsRequest) (*ListFeatureViewSyncsResponse, error)
}

// UnimplementedFeatureOnlineStoreAdminServiceServer should be embedded to have forward compatible implementations.
type UnimplementedFeatureOnlineStoreAdminServiceServer struct {
}

func (UnimplementedFeatureOnlineStoreAdminServiceServer) CreateFeatureOnlineStore(context.Context, *CreateFeatureOnlineStoreRequest) (*longrunningpb.Operation, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateFeatureOnlineStore not implemented")
}
func (UnimplementedFeatureOnlineStoreAdminServiceServer) GetFeatureOnlineStore(context.Context, *GetFeatureOnlineStoreRequest) (*FeatureOnlineStore, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetFeatureOnlineStore not implemented")
}
func (UnimplementedFeatureOnlineStoreAdminServiceServer) ListFeatureOnlineStores(context.Context, *ListFeatureOnlineStoresRequest) (*ListFeatureOnlineStoresResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListFeatureOnlineStores not implemented")
}
func (UnimplementedFeatureOnlineStoreAdminServiceServer) UpdateFeatureOnlineStore(context.Context, *UpdateFeatureOnlineStoreRequest) (*longrunningpb.Operation, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdateFeatureOnlineStore not implemented")
}
func (UnimplementedFeatureOnlineStoreAdminServiceServer) DeleteFeatureOnlineStore(context.Context, *DeleteFeatureOnlineStoreRequest) (*longrunningpb.Operation, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteFeatureOnlineStore not implemented")
}
func (UnimplementedFeatureOnlineStoreAdminServiceServer) CreateFeatureView(context.Context, *CreateFeatureViewRequest) (*longrunningpb.Operation, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateFeatureView not implemented")
}
func (UnimplementedFeatureOnlineStoreAdminServiceServer) GetFeatureView(context.Context, *GetFeatureViewRequest) (*FeatureView, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetFeatureView not implemented")
}
func (UnimplementedFeatureOnlineStoreAdminServiceServer) ListFeatureViews(context.Context, *ListFeatureViewsRequest) (*ListFeatureViewsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListFeatureViews not implemented")
}
func (UnimplementedFeatureOnlineStoreAdminServiceServer) UpdateFeatureView(context.Context, *UpdateFeatureViewRequest) (*longrunningpb.Operation, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdateFeatureView not implemented")
}
func (UnimplementedFeatureOnlineStoreAdminServiceServer) DeleteFeatureView(context.Context, *DeleteFeatureViewRequest) (*longrunningpb.Operation, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteFeatureView not implemented")
}
func (UnimplementedFeatureOnlineStoreAdminServiceServer) SyncFeatureView(context.Context, *SyncFeatureViewRequest) (*SyncFeatureViewResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SyncFeatureView not implemented")
}
func (UnimplementedFeatureOnlineStoreAdminServiceServer) GetFeatureViewSync(context.Context, *GetFeatureViewSyncRequest) (*FeatureViewSync, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetFeatureViewSync not implemented")
}
func (UnimplementedFeatureOnlineStoreAdminServiceServer) ListFeatureViewSyncs(context.Context, *ListFeatureViewSyncsRequest) (*ListFeatureViewSyncsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListFeatureViewSyncs not implemented")
}

// UnsafeFeatureOnlineStoreAdminServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to FeatureOnlineStoreAdminServiceServer will
// result in compilation errors.
type UnsafeFeatureOnlineStoreAdminServiceServer interface {
	mustEmbedUnimplementedFeatureOnlineStoreAdminServiceServer()
}

func RegisterFeatureOnlineStoreAdminServiceServer(s grpc.ServiceRegistrar, srv FeatureOnlineStoreAdminServiceServer) {
	s.RegisterService(&FeatureOnlineStoreAdminService_ServiceDesc, srv)
}

func _FeatureOnlineStoreAdminService_CreateFeatureOnlineStore_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateFeatureOnlineStoreRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FeatureOnlineStoreAdminServiceServer).CreateFeatureOnlineStore(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FeatureOnlineStoreAdminService_CreateFeatureOnlineStore_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FeatureOnlineStoreAdminServiceServer).CreateFeatureOnlineStore(ctx, req.(*CreateFeatureOnlineStoreRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FeatureOnlineStoreAdminService_GetFeatureOnlineStore_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetFeatureOnlineStoreRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FeatureOnlineStoreAdminServiceServer).GetFeatureOnlineStore(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FeatureOnlineStoreAdminService_GetFeatureOnlineStore_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FeatureOnlineStoreAdminServiceServer).GetFeatureOnlineStore(ctx, req.(*GetFeatureOnlineStoreRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FeatureOnlineStoreAdminService_ListFeatureOnlineStores_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListFeatureOnlineStoresRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FeatureOnlineStoreAdminServiceServer).ListFeatureOnlineStores(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FeatureOnlineStoreAdminService_ListFeatureOnlineStores_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FeatureOnlineStoreAdminServiceServer).ListFeatureOnlineStores(ctx, req.(*ListFeatureOnlineStoresRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FeatureOnlineStoreAdminService_UpdateFeatureOnlineStore_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateFeatureOnlineStoreRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FeatureOnlineStoreAdminServiceServer).UpdateFeatureOnlineStore(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FeatureOnlineStoreAdminService_UpdateFeatureOnlineStore_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FeatureOnlineStoreAdminServiceServer).UpdateFeatureOnlineStore(ctx, req.(*UpdateFeatureOnlineStoreRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FeatureOnlineStoreAdminService_DeleteFeatureOnlineStore_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeleteFeatureOnlineStoreRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FeatureOnlineStoreAdminServiceServer).DeleteFeatureOnlineStore(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FeatureOnlineStoreAdminService_DeleteFeatureOnlineStore_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FeatureOnlineStoreAdminServiceServer).DeleteFeatureOnlineStore(ctx, req.(*DeleteFeatureOnlineStoreRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FeatureOnlineStoreAdminService_CreateFeatureView_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateFeatureViewRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FeatureOnlineStoreAdminServiceServer).CreateFeatureView(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FeatureOnlineStoreAdminService_CreateFeatureView_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FeatureOnlineStoreAdminServiceServer).CreateFeatureView(ctx, req.(*CreateFeatureViewRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FeatureOnlineStoreAdminService_GetFeatureView_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetFeatureViewRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FeatureOnlineStoreAdminServiceServer).GetFeatureView(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FeatureOnlineStoreAdminService_GetFeatureView_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FeatureOnlineStoreAdminServiceServer).GetFeatureView(ctx, req.(*GetFeatureViewRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FeatureOnlineStoreAdminService_ListFeatureViews_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListFeatureViewsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FeatureOnlineStoreAdminServiceServer).ListFeatureViews(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FeatureOnlineStoreAdminService_ListFeatureViews_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FeatureOnlineStoreAdminServiceServer).ListFeatureViews(ctx, req.(*ListFeatureViewsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FeatureOnlineStoreAdminService_UpdateFeatureView_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateFeatureViewRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FeatureOnlineStoreAdminServiceServer).UpdateFeatureView(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FeatureOnlineStoreAdminService_UpdateFeatureView_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FeatureOnlineStoreAdminServiceServer).UpdateFeatureView(ctx, req.(*UpdateFeatureViewRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FeatureOnlineStoreAdminService_DeleteFeatureView_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeleteFeatureViewRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FeatureOnlineStoreAdminServiceServer).DeleteFeatureView(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FeatureOnlineStoreAdminService_DeleteFeatureView_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FeatureOnlineStoreAdminServiceServer).DeleteFeatureView(ctx, req.(*DeleteFeatureViewRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FeatureOnlineStoreAdminService_SyncFeatureView_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SyncFeatureViewRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FeatureOnlineStoreAdminServiceServer).SyncFeatureView(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FeatureOnlineStoreAdminService_SyncFeatureView_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FeatureOnlineStoreAdminServiceServer).SyncFeatureView(ctx, req.(*SyncFeatureViewRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FeatureOnlineStoreAdminService_GetFeatureViewSync_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetFeatureViewSyncRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FeatureOnlineStoreAdminServiceServer).GetFeatureViewSync(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FeatureOnlineStoreAdminService_GetFeatureViewSync_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FeatureOnlineStoreAdminServiceServer).GetFeatureViewSync(ctx, req.(*GetFeatureViewSyncRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FeatureOnlineStoreAdminService_ListFeatureViewSyncs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListFeatureViewSyncsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FeatureOnlineStoreAdminServiceServer).ListFeatureViewSyncs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FeatureOnlineStoreAdminService_ListFeatureViewSyncs_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FeatureOnlineStoreAdminServiceServer).ListFeatureViewSyncs(ctx, req.(*ListFeatureViewSyncsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// FeatureOnlineStoreAdminService_ServiceDesc is the grpc.ServiceDesc for FeatureOnlineStoreAdminService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var FeatureOnlineStoreAdminService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "google.cloud.aiplatform.v1beta1.FeatureOnlineStoreAdminService",
	HandlerType: (*FeatureOnlineStoreAdminServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "CreateFeatureOnlineStore",
			Handler:    _FeatureOnlineStoreAdminService_CreateFeatureOnlineStore_Handler,
		},
		{
			MethodName: "GetFeatureOnlineStore",
			Handler:    _FeatureOnlineStoreAdminService_GetFeatureOnlineStore_Handler,
		},
		{
			MethodName: "ListFeatureOnlineStores",
			Handler:    _FeatureOnlineStoreAdminService_ListFeatureOnlineStores_Handler,
		},
		{
			MethodName: "UpdateFeatureOnlineStore",
			Handler:    _FeatureOnlineStoreAdminService_UpdateFeatureOnlineStore_Handler,
		},
		{
			MethodName: "DeleteFeatureOnlineStore",
			Handler:    _FeatureOnlineStoreAdminService_DeleteFeatureOnlineStore_Handler,
		},
		{
			MethodName: "CreateFeatureView",
			Handler:    _FeatureOnlineStoreAdminService_CreateFeatureView_Handler,
		},
		{
			MethodName: "GetFeatureView",
			Handler:    _FeatureOnlineStoreAdminService_GetFeatureView_Handler,
		},
		{
			MethodName: "ListFeatureViews",
			Handler:    _FeatureOnlineStoreAdminService_ListFeatureViews_Handler,
		},
		{
			MethodName: "UpdateFeatureView",
			Handler:    _FeatureOnlineStoreAdminService_UpdateFeatureView_Handler,
		},
		{
			MethodName: "DeleteFeatureView",
			Handler:    _FeatureOnlineStoreAdminService_DeleteFeatureView_Handler,
		},
		{
			MethodName: "SyncFeatureView",
			Handler:    _FeatureOnlineStoreAdminService_SyncFeatureView_Handler,
		},
		{
			MethodName: "GetFeatureViewSync",
			Handler:    _FeatureOnlineStoreAdminService_GetFeatureViewSync_Handler,
		},
		{
			MethodName: "ListFeatureViewSyncs",
			Handler:    _FeatureOnlineStoreAdminService_ListFeatureViewSyncs_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "google/cloud/aiplatform/v1beta1/feature_online_store_admin_service.proto",
}
