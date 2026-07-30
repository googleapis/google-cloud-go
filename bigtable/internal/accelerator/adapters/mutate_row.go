// Copyright 2026 Google LLC
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

package adapters

import (
	v2pb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MutateRowRequestAdapter adapts V2 MutateRowRequest to SessionMutateRowRequest.
type MutateRowRequestAdapter struct{}

func (a *MutateRowRequestAdapter) Adapt(from *v2pb.MutateRowRequest) (*v2pb.SessionMutateRowRequest, error) {
	if from == nil {
		return nil, nil
	}
	return &v2pb.SessionMutateRowRequest{
		Key:       from.RowKey,
		Mutations: from.Mutations,
	}, nil
}

// ExtractResource returns the resource the request targets, tagged with its
// kind. A MutateRowRequest names either a table or an authorized view
// (materialized views are read-only and have no field here); the authorized
// view is checked first so the correct resource is surfaced regardless of which
// the caller populated.
func (a *MutateRowRequestAdapter) ExtractResource(from *v2pb.MutateRowRequest) (Resource, error) {
	if from == nil {
		return Resource{}, status.Errorf(codes.InvalidArgument, "request is nil")
	}
	switch {
	case from.AuthorizedViewName != "":
		return Resource{Kind: ResourceAuthorizedView, Name: from.AuthorizedViewName}, nil
	case from.TableName != "":
		return Resource{Kind: ResourceTable, Name: from.TableName}, nil
	default:
		return Resource{}, status.Errorf(codes.InvalidArgument, "MutateRowRequest names no table or authorized view")
	}
}

// MutateRowResponseAdapter adapts SessionMutateRowResponse to MutateRowResponse.
type MutateRowResponseAdapter struct{}

func (a *MutateRowResponseAdapter) Adapt(from *v2pb.SessionMutateRowResponse) (*v2pb.MutateRowResponse, error) {
	if from == nil {
		return nil, nil
	}
	// Bare minimum scaffold.
	return &v2pb.MutateRowResponse{}, nil
}
