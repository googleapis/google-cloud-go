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
	"crypto/x509"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"cloud.google.com/go/bigtable"
	gauth "golang.org/x/oauth2/google"

	"github.com/golang/protobuf/ptypes/duration"
	pb "github.com/googleapis/cloud-bigtable-clients-test/testproxypb"
	"google.golang.org/api/option"
	btpb "google.golang.org/genproto/googleapis/bigtable/v2"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
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

// rowSetFromProto translates a Bigtable v2.RowSet object to a Bigtable.RowSet
// object.
func rowSetFromProto(rs *btpb.RowSet) bigtable.RowSet {
	rowRangeList := make(bigtable.RowRangeList, 0)

	// Convert all rowKeys into single-row RowRanges
	if rowKeys := rs.GetRowKeys(); len(rowKeys) > 0 {
		for _, b := range rowKeys {

			// Find the next highest key using byte operations
			// This allows us to create a RowRange of 1 rowKey
			e := binary.BigEndian.Uint64(b)
			e++

			s := binary.Size(e)
			bOut := make([]byte, s)
			binary.BigEndian.PutUint64(bOut, e)

			rowRangeList = append(rowRangeList, bigtable.NewRange(string(b), string(bOut)))
		}
	}

	if rowRanges := rs.GetRowRanges(); len(rowRanges) > 0 {
		for _, rrs := range rowRanges {
			var start, end string
			var rr bigtable.RowRange

			switch rrs.StartKey.(type) {
			case *btpb.RowRange_StartKeyClosed:
				start = string(rrs.GetStartKeyClosed())
			case *btpb.RowRange_StartKeyOpen:
				start = string(rrs.GetStartKeyOpen())
			default:
				start = ""
			}

			switch rrs.EndKey.(type) {
			case *btpb.RowRange_EndKeyClosed:
				end = string(rrs.GetEndKeyClosed())
				rr = bigtable.NewRange(start, end)
			case *btpb.RowRange_EndKeyOpen:
				end = string(rrs.GetEndKeyOpen())
				rr = bigtable.NewRange(start, end)
			default:
				// If not set, get the infinite row range
				rr = bigtable.InfiniteRange(start)
			}

			rowRangeList = append(rowRangeList, rr)
		}
	}
	return rowRangeList
}

// testClient contains a bigtable.Client object, cancel functions for the calls
// made using the client, an appProfileID (optionally), and a
// perOperationTimeout (optionally).
type testClient struct {
	c                   *bigtable.Client     // c stores the Bigtable client under test
	cancels             []context.CancelFunc // cancels stores a cancel() for each call to this client
	appProfileID        string               // appProfileID is currently unused
	perOperationTimeout *duration.Duration   // perOperationTimeout sets a custom timeout for methods calls on this client
}

// cancelAll calls all of the context.CancelFuncs stored in this testClient.
func (tc *testClient) cancelAll() {
	for _, c := range tc.cancels {
		c()
	}
}

// credentialsBundle implements credentials.Bundle interface
// [See documentation for usage](https://pkg.go.dev/google.golang.org/grpc/credentials#Bundle).
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
// Note that the Go client libraries don't explicitly have the concept of
// channel credentials, call credentials, or composite call credentials per
// [gRPC documentation](https://grpc.io/docs/guides/auth/).
func getCredentialsOptions(req *pb.CreateClientRequest) ([]grpc.DialOption, error) {
	opts := make([]grpc.DialOption, 0)

	if req.CallCredential == nil &&
		req.ChannelCredential == nil &&
		req.OverrideSslTargetName == "" {
		return opts, nil
	}

	// If you have call credentials, then you must have channel credentials too
	if req.CallCredential != nil && req.ChannelCredential == nil {
		return nil, fmt.Errorf("%s: must supply channel credentials with call credentials", logLabel)
	}

	// This may not be needed--OverrideSslTargetName is provided to when
	// creating the channel credentials.
	if req.OverrideSslTargetName != "" {
		d := grpc.WithAuthority(req.OverrideSslTargetName)
		opts = append(opts, d)
	}

	// Case 1: No additional credentials provided
	chc := req.GetChannelCredential()
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
		opts = append(opts, d)
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
	opts = append(opts, d)

	return opts, nil
}

// getChannelCredentials extracts the channel credentials (credentials for use)
// with all calls on this client.
func getChannelCredentials(credsProto *pb.ChannelCredential, sslTargetName string) (credentials.TransportCredentials, error) {
	var creds credentials.TransportCredentials
	v := credsProto.GetValue()
	switch t := v.(type) {
	case *pb.ChannelCredential_Ssl:
		pem := t.Ssl.GetPemRootCerts()

		cert, err := x509.ParseCertificate([]byte(pem))
		if err != nil {
			return nil, err
		}

		pool := x509.NewCertPool()
		pool.AddCert(cert)

		creds = credentials.NewClientTLSFromCert(pool, sslTargetName)
		if err != nil {
			return nil, err
		}
	case *pb.ChannelCredential_None:
		creds = insecure.NewCredentials()
	default:
		ctx := context.Background()
		c, err := gauth.FindDefaultCredentials(ctx, []string{"https://www.googleapis.com/auth/cloud-platform"}...)
		if err != nil {
			return nil, err
		}

		// TODO(developer): Determine how to pass this call option back to caller
		option.WithTokenSource(c.TokenSource)

		return nil, nil
	}
	return creds, nil
}

// goTestProxyServer represents an instance of the test proxy server. It keeps
// a reference to individual clients instances (stored in a testClient object).
type goTestProxyServer struct {
	pb.UnimplementedCloudBigtableV2TestProxyServer
	clientIDs map[string]testClient // clientIDs has all of the bigtable.Client objects under test
}

// CreateClient responds to the CreateClient RPC. This method adds a new client
// instance to the goTestProxyServer
func (s *goTestProxyServer) CreateClient(ctx context.Context, req *pb.CreateClientRequest) (*pb.CreateClientResponse, error) {
	if req.ClientId == "" ||
		req.DataTarget == "" ||
		req.ProjectId == "" ||
		req.InstanceId == "" {
		return nil, stat.Error(codes.InvalidArgument,
			fmt.Sprintf("%s must provide ClientId, DataTarget, ProjectId, and InstanceId", logLabel))
	}

	if _, exists := s.clientIDs[req.ClientId]; exists {
		return nil, stat.Error(codes.AlreadyExists,
			fmt.Sprintf("%s: ClientID already exists", logLabel))
	}

	opts, err := getCredentialsOptions(req)
	if err != nil {
		return nil, stat.Error(codes.Unauthenticated,
			fmt.Sprintf("%s: failed to get credentials: %v", logLabel, err))
	}

	conn, err := grpc.Dial(req.DataTarget, opts...)
	if err != nil {
		return nil, stat.Error(codes.Unknown, fmt.Sprintf("%s: failed to create connection: %v", logLabel, err))
	}

	localCtx, cancel := context.WithCancel(context.Background())
	c, err := bigtable.NewClient(localCtx, req.ProjectId, req.InstanceId, option.WithGRPCConn(conn))
	if err != nil {
		cancel()
		return nil, stat.Error(codes.Internal,
			fmt.Sprintf("%s: failed to create client: %v", logLabel, err))
	}

	if s.clientIDs == nil {
		s.clientIDs = make(map[string]testClient)
	}

	s.clientIDs[req.ClientId] = testClient{
		c: c,
		cancels: []context.CancelFunc{
			cancel,
		},
		appProfileID:        req.AppProfileId,
		perOperationTimeout: req.PerOperationTimeout,
	}

	res := &pb.CreateClientResponse{}

	return res, nil
}

// RemoveClient responds to the RemoveClient RPC. This method removes an
// existing client from the goTestProxyServer
func (s *goTestProxyServer) RemoveClient(ctx context.Context, req *pb.RemoveClientRequest) (*pb.RemoveClientResponse, error) {
	clientId := req.ClientId
	doCancelAll := req.CancelAll

	btc, exists := s.clientIDs[clientId]
	if !exists {
		return nil, stat.Error(codes.InvalidArgument,
			fmt.Sprintf("%s: ClientID does not exist", logLabel))
	}

	if doCancelAll {
		for _, c := range btc.cancels {
			c()
		}
	}

	resp := &pb.RemoveClientResponse{}
	return resp, nil
}

// ReadRow responds to the ReadRow RPC. This method gets all of the column
// data for a single row in the Table.
func (s *goTestProxyServer) ReadRow(ctx context.Context, req *pb.ReadRowRequest) (*pb.RowResult, error) {
	btc, exists := s.clientIDs[req.ClientId]
	if !exists {
		return nil, stat.Error(codes.InvalidArgument,
			fmt.Sprintf("%s: ClientID does not exist", logLabel))
	}

	tName := req.TableName
	t := btc.c.Open(tName)

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

// ReadRows responds to the ReadRows RPC. This method gets all of the column
// data for a set of rows, a range of rows, or the entire table.
func (s *goTestProxyServer) ReadRows(ctx context.Context, req *pb.ReadRowsRequest) (*pb.RowsResult, error) {
	btc, exists := s.clientIDs[req.ClientId]
	if !exists {
		return nil, stat.Error(codes.InvalidArgument,
			fmt.Sprintf("%s: ClientID does not exist", logLabel))
	}

	rrq := req.GetRequest()
	lim := req.GetCancelAfterRows()

	if rrq == nil {
		return nil, stat.Error(codes.InvalidArgument, "request to ReadRows() is missing inner request")
	}

	t := btc.c.Open(rrq.TableName)

	ctx, cancel := context.WithCancel(ctx)
	btc.cancels = append(btc.cancels, cancel)

	rowPbs := rrq.Rows

	var rs bigtable.RowSet

	// Go client doesn't have a Table.GetAll() function--RowSet must be provided
	// for ReadRows. We need to use
	if len(rowPbs.GetRowKeys()) == 0 && len(rowPbs.GetRowRanges()) == 0 {
		// Should be lowest possible key value
		rs = bigtable.InfiniteRange("0")
	} else {
		rs = rowSetFromProto(rowPbs)
	}

	var c int32
	rowsPb := make([]*btpb.Row, 0)

	t.ReadRows(ctx, rs, func(r bigtable.Row) bool {
		c++
		if c == lim {
			return false
		}
		rpb, err := rowToRowProto(r)
		if err != nil {
			return false
		}
		rowsPb = append(rowsPb, rpb)
		return true
	})

	res := &pb.RowsResult{
		Status: &status.Status{
			Code: int32(codes.OK),
		},
		Row: rowsPb,
	}

	return res, nil
}

// MutateRow responds to the MutateRow RPC. This methods applies a series of
// changes (or deletions) to a single row in a table.
func (s *goTestProxyServer) MutateRow(ctx context.Context, req *pb.MutateRowRequest) (*pb.MutateRowResult, error) {
	btc, exists := s.clientIDs[req.ClientId]
	if !exists {
		return nil, stat.Error(codes.InvalidArgument,
			fmt.Sprintf("%s: ClientID does not exist", logLabel))
	}

	rrq := req.GetRequest()
	if rrq == nil {
		return nil, stat.Error(codes.InvalidArgument, "request to ReadRows() is missing inner request")
	}

	m := bigtable.NewMutation()
	mPbs := rrq.Mutations
	for _, mpb := range mPbs {

		switch mut := mpb.Mutation; mut.(type) {
		case *btpb.Mutation_DeleteFromColumn_:
			del := mut.(*btpb.Mutation_DeleteFromColumn_)
			fam := del.DeleteFromColumn.FamilyName
			col := del.DeleteFromColumn.ColumnQualifier

			if del.DeleteFromColumn.TimeRange != nil {
				start := bigtable.Time(time.UnixMicro(del.DeleteFromColumn.TimeRange.StartTimestampMicros))
				end := bigtable.Time(time.UnixMicro(del.DeleteFromColumn.TimeRange.EndTimestampMicros))
				m.DeleteTimestampRange(fam, string(col), start, end)
			} else {
				m.DeleteCellsInColumn(fam, string(col))
			}

		case *btpb.Mutation_DeleteFromFamily_:
			del := mut.(*btpb.Mutation_DeleteFromFamily_)
			fam := del.DeleteFromFamily.FamilyName
			m.DeleteCellsInFamily(fam)

		case *btpb.Mutation_DeleteFromRow_:
			m.DeleteRow()

		case *btpb.Mutation_SetCell_:
			setCell := mut.(*btpb.Mutation_SetCell_)
			fam := setCell.SetCell.FamilyName
			col := setCell.SetCell.ColumnQualifier
			val := setCell.SetCell.Value
			ts := setCell.SetCell.TimestampMicros
			bts := bigtable.Time(time.UnixMicro(ts))
			m.Set(fam, string(col), bts, val)

		}
	}

	t := btc.c.Open(rrq.TableName)
	row := rrq.RowKey

	ctx, cancel := context.WithCancel(ctx)
	btc.cancels = append(btc.cancels, cancel)

	err := t.Apply(ctx, string(row), m)
	if err != nil {
		return nil, err
	}

	res := &pb.MutateRowResult{
		Status: &status.Status{
			Code: int32(codes.OK),
		},
	}
	return res, nil
}

func (s *goTestProxyServer) BulkMutateRows(ctx context.Context, req *pb.MutateRowsRequest) (*pb.MutateRowsResult, error) {
	return nil, stat.Error(codes.Unimplemented, "method BulkMutateRows() not implemented")
}

func (s *goTestProxyServer) CheckAndMutateRow(ctx context.Context, req *pb.CheckAndMutateRowRequest) (*pb.CheckAndMutateRowResult, error) {
	return nil, stat.Error(codes.Unimplemented, "method CheckAndMutateRow() not implemented")
}

func (s *goTestProxyServer) SampleRowKeys(ctx context.Context, req *pb.SampleRowKeysRequest) (*pb.SampleRowKeysResult, error) {
	return nil, stat.Error(codes.Unimplemented, "method SampleRowKeys() not implemented")
}

func (s *goTestProxyServer) ReadModifyWriteRow(ctx context.Context, req *pb.ReadModifyWriteRowRequest) (*pb.RowResult, error) {
	return nil, stat.Error(codes.Unimplemented, "method ReadModifyWriteRow() not implemented")
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
