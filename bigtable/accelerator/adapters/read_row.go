package adapters

import (
	v2pb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ReadRowRequestAdapter adapts V2 ReadRowsRequest to SessionReadRowRequest.
type ReadRowRequestAdapter struct{}

func (a *ReadRowRequestAdapter) Adapt(from *v2pb.ReadRowsRequest) (*v2pb.SessionReadRowRequest, error) {
	if from == nil {
		return nil, nil
	}
	req := &v2pb.SessionReadRowRequest{}
	if from.Rows != nil && len(from.Rows.RowKeys) > 0 {
		req.Key = from.Rows.RowKeys[0]
	}
	req.Filter = from.Filter
	return req, nil
}

func (a *ReadRowRequestAdapter) ExtractResource(from *v2pb.ReadRowsRequest) (string, error) {
	if from == nil {
		return "", status.Errorf(codes.InvalidArgument, "request is nil")
	}
	return from.TableName, nil
}

// ReadRowResponseAdapter adapts SessionReadRowResponse to ReadRowsResponse.
type ReadRowResponseAdapter struct{}

func (a *ReadRowResponseAdapter) Adapt(from *v2pb.SessionReadRowResponse) (*v2pb.ReadRowsResponse, error) {
	if from == nil {
		return nil, nil
	}
	// Bare minimum scaffold.
	return &v2pb.ReadRowsResponse{}, nil
}
