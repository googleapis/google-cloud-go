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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	oauth "google.golang.org/grpc/credentials/oauth"
	stat "google.golang.org/grpc/status"
)

var (
	port     = flag.Int("port", 9999, "The server port")
	logLabel = "cbt-go-proxy"
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
			// Format of column name is `family:columnQualifier`
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

// credentialsBundle implements credentials.Bundle interface
type credentialsBundle struct {
	channel credentials.TransportCredentials
	call    credentials.PerRPCCredentials
}

// TransportCredentials gets the channel credentials as TransportCredentials
func (c credentialsBundle) TransportCredentials() credentials.TransportCredentials {
	return c.channel
}

// PerRPCCredentials gets the call credentials ars PerRPCCredentials
func (c credentialsBundle) PerRPCCredentials() credentials.PerRPCCredentials {
	return c.call
}

// NewWithMode is not used. Always returns nil
func (c credentialsBundle) NewWithMode(mode string) (credentials.Bundle, error) {
	return nil, nil
}

// getCredentialsOptions extracts the authentication details--SSL name override,
// call credentials, channel credentials--from a CreateClientRequest object.
//
// There are three base cases to address:
//  1. CreateClientRequest specifies no unique credentials; so ADC will be used.
//     This method returns an empty slice.
//  2. CreateClientRequest specifies only a channel credential.
//  3. CreateClientRequest specifies both call and channel credentials. In
//     this case, we need to create a combined credential (Bundle).
//
// Discussed [here](https://github.com/grpc/grpc-go/tree/master/examples/features/authentication).
func getCredentialsOptions(req *pb.CreateClientRequest) ([]option.ClientOption, error) {
	opts := make([]option.ClientOption, 0)

	if req.CallCredential == nil &&
		req.ChannelCredential == nil &&
		req.OverrideSslTargetName == "" {
		return opts, nil
	}

	// If you have call credentials, then you must have channel credentials too
	if req.CallCredential != nil && req.ChannelCredential == nil {
		return nil, fmt.Errorf("%s: must supply channel credentials with call credentials", logLabel)
	}

	if req.OverrideSslTargetName != "" {
		d := grpc.WithAuthority(req.OverrideSslTargetName)
		opts = append(opts, option.WithGRPCDialOption(d))
	}

	// Case 1: No additional credentials provided
	chc := req.ChannelCredential
	if chc == nil {
		return opts, nil
	}
	channelCreds, err := getChannelCredentials(chc, req.OverrideSslTargetName)
	if err != nil {
		return nil, err
	}

	// Case 2: Only channel credentials provided
	cc := req.CallCredential
	if cc == nil {
		d := grpc.WithTransportCredentials(channelCreds)
		opts = append(opts, option.WithGRPCDialOption(d))
		return opts, nil
	}

	// Case 3: Both channel & call credentials provided
	sa := cc.GetJsonServiceAccount()
	clc, err := oauth.NewJWTAccessFromKey([]byte(sa))
	if err != nil {
		return nil, err
	}

	b := credentialsBundle{
		channel: channelCreds,
		call:    clc,
	}

	d := grpc.WithCredentialsBundle(b)
	opts = append(opts, option.WithGRPCDialOption(d))

	return opts, nil
}

func getChannelCredentials(credsProto *pb.ChannelCredential, sslTargetName string) (credentials.TransportCredentials, error) {
	pem := credsProto.GetSsl().GetPemRootCerts()
	creds, err := credentials.NewClientTLSFromFile(pem, sslTargetName)
	if err != nil {
		return nil, err
	}
	return creds, nil
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
		return nil, stat.Error(codes.InvalidArgument,
			fmt.Sprintf("%s must provide ClientId, DataTarget, ProjectId, and InstanceId", logLabel))
	}

	opts := make([]option.ClientOption, 0)
	opts = append(opts, option.WithEndpoint(req.DataTarget))

	credOpts, err := getCredentialsOptions(req)
	if err != nil {
		return nil, stat.Error(codes.Unauthenticated,
			fmt.Sprintf("%s: failed to set credentials: %v", logLabel, err))
	}
	opts = append(opts, credOpts...)

	c, err := bigtable.NewClient(ctx, req.ProjectId, req.InstanceId, opts...)
	if err != nil {
		return nil, stat.Error(codes.Internal,
			fmt.Sprintf("%s: failed to create client: %v", logLabel, err))
	}

	s.btClient = c
	s.clientID = req.ClientId // Might need to be stored in a map
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
		return nil, fmt.Errorf("%s: no error or row returned from ReadRow()", logLabel)
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

func newProxyServer(lis net.Listener) *grpc.Server {
	s := grpc.NewServer()
	pb.RegisterCloudBigtableV2TestProxyServer(s, &goTestProxyServer{})
	log.Printf("server listening at %v", lis.Addr())
	return s
}

func main() {
	flag.Parse()

	log.Println(*port)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := newProxyServer(lis)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
