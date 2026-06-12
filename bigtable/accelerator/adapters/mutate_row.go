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

func (a *MutateRowRequestAdapter) ExtractResource(from *v2pb.MutateRowRequest) (string, error) {
	if from == nil {
		return "", status.Errorf(codes.InvalidArgument, "request is nil")
	}
	return from.TableName, nil
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
