// Copyright 2022 Google LLC
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

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"strings"

	"cloud.google.com/go/bigtable"

	"github.com/golang/protobuf/ptypes/duration"
	pb "github.com/googleapis/cloud-bigtable-clients-test/testproxypb"
	"google.golang.org/api/option"
	btpb "google.golang.org/genproto/googleapis/bigtable/v2"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	port = flag.Int("port", 9999, "The server port")
)

// rowToRowProto converts a Bigtable Go client Row struct into a
// Bigtable protobuf Row struct. It iterates over all of the column families
// (keys) and ReadItem slices (values) in the client Row struct
func rowToRowProto(btRow bigtable.Row) (*btpb.Row, error) {
	pbRow := &btpb.Row{
		Key:      []byte(btRow.Key()),
		Families: make([]*btpb.Family, 0),
	}

	for fam, ris := range btRow {
		pbFam := &btpb.Family{
			Name:    fam,
			Columns: make([]*btpb.Column, 0),
		}

		for _, col := range ris {
			colQualifier := strings.Split(col.Column, ":")[1]
			pbCol := &btpb.Column{
				Qualifier: []byte(colQualifier),
				Cells: []*btpb.Cell{
					{
						Value:           col.Value,
						TimestampMicros: col.Timestamp.Time().UnixMicro(),
						Labels:          col.Labels,
					},
				},
			}
			pbFam.Columns = append(pbFam.Columns, pbCol)
		}

		pbRow.Families = append(pbRow.Families, pbFam)
	}

	return pbRow, nil
}

func getCredentialsOptions(req *pb.CreateClientRequest) ([]option.ClientOption, error) {
	opts := make([]option.ClientOption, 0)

	if req.CallCredential == nil &&
		req.ChannelCredential == nil &&
		req.OverrideSslTargetName == "" {
		return opts, nil
	}

	if req.OverrideSslTargetName != "" {
		d := grpc.WithAuthority(req.OverrideSslTargetName)
		opts = append(opts, option.WithGRPCDialOption(d))
	}

	if req.CallCredential != nil {
		cc := req.CallCredential
		sa := cc.GetJsonServiceAccount()
		creds, err := credentials.NewClientTLSFromFile(sa, req.OverrideSslTargetName)
		if err != nil {
			return nil, err
		}

		d := grpc.WithTransportCredentials(creds)
		opts = append(opts, option.WithGRPCDialOption(d))
	}

	if req.ChannelCredential != nil {
		chc := req.ChannelCredential
		ssl := chc.GetSsl()
		pem := ssl.GetPemRootCerts()
		creds := credentials.NewClientTLSFromCert(pem, req.OverrideSslTargetName)
		d := grpc.WithTransportCredentials(creds)
		opts = append(opts, option.WithGRPCDialOption(d))
	}

	return opts, nil
}

type goTestProxyServer struct {
	pb.UnimplementedCloudBigtableV2TestProxyServer
	btClient            *bigtable.Client
	clientID            string
	appProfileID        string
	perOperationTimeout *duration.Duration
}

func (s *goTestProxyServer) CreateClient(ctx context.Context, req *pb.CreateClientRequest) (*pb.CreateClientResponse, error) {
	if req.ClientId == "" ||
		req.DataTarget == "" ||
		req.ProjectId == "" ||
		req.InstanceId == "" {
		return nil, fmt.Errorf("cbt-go-proxy: must provide ClientId, DataTarget, ProjectId, and InstanceId")
	}

	opts := make([]option.ClientOption, 0)
	opts = append(opts, option.WithEndpoint(req.DataTarget))

	credOpts, err := getCredentialsOptions(req)
	if err != nil {
		return nil, err
	}
	opts = append(opts, credOpts...)

	c, err := bigtable.NewClient(ctx, req.ProjectId, req.InstanceId, opts...)
	if err != nil {
		return nil, err
	}

	s.btClient = c
	s.clientID = req.ClientId
	s.appProfileID = req.AppProfileId
	s.perOperationTimeout = req.PerOperationTimeout

	res := &pb.CreateClientResponse{}

	return res, nil
}

func (s *goTestProxyServer) RemoveClient(ctx context.Context, req *pb.RemoveClientRequest) (*pb.RemoveClientResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *goTestProxyServer) ReadRow(ctx context.Context, req *pb.ReadRowRequest) (*pb.RowResult, error) {

	tName := req.TableName
	t := s.btClient.Open(tName)

	r, err := t.ReadRow(ctx, req.RowKey)

	if err != nil {
		return nil, err
	}

	if r == nil {
		return nil, fmt.Errorf("no error or row returned from ReadRow()")
	}

	pbRow, err := rowToRowProto(r)
	if err != nil {
		return nil, err
	}

	res := &pb.RowResult{
		Status: &status.Status{},
		Row:    pbRow,
	}

	return res, nil
}

func (s *goTestProxyServer) ReadRows(ctx context.Context, req *pb.ReadRowsRequest) (*pb.RowsResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *goTestProxyServer) MutateRow(ctx context.Context, req *pb.MutateRowRequest) (*pb.MutateRowResult, error) {
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

func (s *goTestProxyServer) mustEmbedUnimplementedCloudBigtableV2TestProxyServer() {}

func main() {
	flag.Parse()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterCloudBigtableV2TestProxyServer(s, &goTestProxyServer{})
	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
