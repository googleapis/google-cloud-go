package main

import (
	"context"
	"flag"
	"fmt"

	"cloud.google.com/go/bigtable"

	pb "github.com/googleapis/cloud-bigtable-clients-test/testproxypb"
)

var (
	port = flag.Int("port", 9999, "The server port")
)

type goTestProxyServer struct {
	btClient *bigtable.Client
}

func (s *goTestProxyServer) CreateClient(ctx context.Context, req *pb.CreateClientRequest) (*pb.CreateClientResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *goTestProxyServer) RemoveClient(ctx context.Context, req *pb.RemoveClientRequest) (*pb.RemoveClientResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *goTestProxyServer) ReadRows(ctx context.Context, req *pb.ReadRowsRequest) (*pb.RowsResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *goTestProxyServer) MutateRow(ctx context.Context, req *pb.MutateRowsRequest) (*pb.MutateRowResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *goTestProxyServer) BulkMutateRows(ctx context.Context, req *pb.MutateRowsRequest) (*pb.MutateRowsResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *goTestProxyServer) CheckAndMutateRow(ctx context.Context, req *pb.CheckAndMutateRowRequest) (*pb.CheckAndMutateRowResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *goTestProxyServer) SampleRowKeys(ctx context.Context, req *pb.SampleRowKeysRequest) (*pb.SampleRowKeysResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *goTestProxyServer) ReadModifyWriteRow(ctx context.Context, req *pb.ReadModifyWriteRowRequest) (*pb.RowResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func main() {
	fmt.Println("Hello world!")
}
