package main

import (
	"context"
	"fmt"

	pb "github.com/googleapis/cloud-bigtable-clients-test/testproxypb"
	// btpb "google.golang.org/genproto/googleapis/bigtable/v2"
)

func (s *goTestProxyServer) ReadRow(ctx context.Context, req *pb.ReadRowRequest) (*pb.RowResult, error) {

	tName := req.TableName
	t := s.btClient.Open(tName)

	r, err := t.ReadRow(ctx, req.RowKey)

	if err != nil {
		return nil, err
	}

	if r != nil {
		return nil, fmt.Errorf("no error or row returned from ReadRow()")
	}

	// TODO(telpirion): translate Go client types to BT proto types
	res := &pb.RowResult{
		Row:    nil,
		Status: nil,
	}

	return res, nil
}
