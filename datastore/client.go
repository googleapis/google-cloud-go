// Copyright 2017 Google Inc. All Rights Reserved.
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

package datastore

import (
	"fmt"

	"cloud.google.com/go/internal/version"
	"golang.org/x/net/context"
	pb "google.golang.org/genproto/googleapis/datastore/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// datastoreClient is a wrapper for the pb.DatastoreClient that includes gRPC
// metadata to be sent in each request for server-side traffic management.
type datastoreClient struct {
	// Embed so we still implement the DatastoreClient interface,
	// if the interface adds more methods.
	pb.DatastoreClient

	c  pb.DatastoreClient
	md metadata.MD
}

func newDatastoreClient(conn *grpc.ClientConn, projectID string) pb.DatastoreClient {
	return &datastoreClient{
		c: pb.NewDatastoreClient(conn),
		md: metadata.Pairs(
			resourcePrefixHeader, "projects/"+projectID,
			"x-goog-api-client", fmt.Sprintf("gl-go/%s gccl/%s grpc/", version.Go(), version.Repo)),
	}
}

func (dc *datastoreClient) Lookup(ctx context.Context, in *pb.LookupRequest, opts ...grpc.CallOption) (*pb.LookupResponse, error) {
	return dc.c.Lookup(metadata.NewOutgoingContext(ctx, dc.md), in, opts...)
}

func (dc *datastoreClient) RunQuery(ctx context.Context, in *pb.RunQueryRequest, opts ...grpc.CallOption) (*pb.RunQueryResponse, error) {
	return dc.c.RunQuery(metadata.NewOutgoingContext(ctx, dc.md), in, opts...)
}

func (dc *datastoreClient) BeginTransaction(ctx context.Context, in *pb.BeginTransactionRequest, opts ...grpc.CallOption) (*pb.BeginTransactionResponse, error) {
	return dc.c.BeginTransaction(metadata.NewOutgoingContext(ctx, dc.md), in, opts...)
}

func (dc *datastoreClient) Commit(ctx context.Context, in *pb.CommitRequest, opts ...grpc.CallOption) (*pb.CommitResponse, error) {
	return dc.c.Commit(metadata.NewOutgoingContext(ctx, dc.md), in, opts...)
}

func (dc *datastoreClient) Rollback(ctx context.Context, in *pb.RollbackRequest, opts ...grpc.CallOption) (*pb.RollbackResponse, error) {
	return dc.c.Rollback(metadata.NewOutgoingContext(ctx, dc.md), in, opts...)
}

func (dc *datastoreClient) AllocateIds(ctx context.Context, in *pb.AllocateIdsRequest, opts ...grpc.CallOption) (*pb.AllocateIdsResponse, error) {
	return dc.c.AllocateIds(metadata.NewOutgoingContext(ctx, dc.md), in, opts...)
}
