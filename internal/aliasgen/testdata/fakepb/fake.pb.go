// Copyright 2022 Google LLC
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

package fakepb

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ grpc.ClientConnInterface

// FooServiceClient is an interface.
type FooServiceClient interface {
	CreateFoo(ctx context.Context, in *CreateFooRequest, opts ...grpc.CallOption) (*CreateFooResponse, error)
	ListFoos(ctx context.Context, in *ListFoosRequest, opts ...grpc.CallOption) (*ListFoosResponse, error)
}

func NewFooServiceClient(cc grpc.ClientConnInterface) FooServiceClient {
	return &fooServiceClient{cc}
}

type fooServiceClient struct {
	cc grpc.ClientConnInterface
}

func (c *fooServiceClient) CreateFoo(ctx context.Context, in *CreateFooRequest, opts ...grpc.CallOption) (*CreateFooResponse, error) {
	return nil, nil
}
func (c *fooServiceClient) ListFoos(ctx context.Context, in *ListFoosRequest, opts ...grpc.CallOption) (*ListFoosResponse, error) {
	return nil, nil
}

// FooServiceServer is an interface.
type FooServiceServer interface {
	ListFoos(context.Context, *ListFoosRequest) (*ListFoosResponse, error)
	CreateFoo(context.Context, *CreateFooRequest) (*Foo, error)
}

// UnimplementedFooServiceServer is a struct.
type UnimplementedFooServiceServer struct{}

func (*UnimplementedFooServiceServer) ListFoos(context.Context, *ListFoosRequest) (*ListFoosResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListFoos not implemented")
}
func (*UnimplementedFooServiceServer) CreateFoo(context.Context, *CreateFooRequest) (*Foo, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateFoo not implemented")
}

func RegisterFooServiceServer(s *grpc.Server, srv FooServiceServer) {
	s.RegisterService(nil, srv)
}
