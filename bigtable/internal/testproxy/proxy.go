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
	"sync"
	"time"

	"cloud.google.com/go/bigtable"
	"github.com/golang/protobuf/ptypes/duration"
	pb "github.com/googleapis/cloud-bigtable-clients-test/testproxypb"
	gauth "golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	btpb "google.golang.org/genproto/googleapis/bigtable/v2"
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

// rowToProto converts a Bigtable Go client Row struct into a
// Bigtable protobuf Row struct. It iterates over all of the column families
// (keys) and ReadItem slices (values) in the client Row struct
func rowToProto(btRow bigtable.Row) (*btpb.Row, error) {
	pbRow := &btpb.Row{
		Key: []byte(btRow.Key()),
	}

	for fam, ris := range btRow {
		pbFam := &btpb.Family{
			Name: fam,
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

// mutationFromProto translates a slice of Bigtable v2.Mutation objects into
// a single Bigtable.Mutation object.
func mutationFromProto(mPbs []*btpb.Mutation) *bigtable.Mutation {
	m := bigtable.NewMutation()
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
	return m
}

// filterFromProto translates a Bigtable v2.RowFilter object into a Bigtable
// Filter object.
func filterFromProto(rfPb *btpb.RowFilter) *bigtable.Filter {
	var f *bigtable.Filter
	switch fpb := rfPb.Filter; fpb.(type) {
	case *btpb.RowFilter_Chain_:
		c := fpb.(*btpb.RowFilter_Chain_)
		var fs []bigtable.Filter
		for _, cfpb := range c.Chain.Filters {
			cf := filterFromProto(cfpb)
			fs = append(fs, *cf)
		}
		cf := bigtable.ChainFilters(fs...)
		f = &cf

	case *btpb.RowFilter_Interleave_:
		i := fpb.(*btpb.RowFilter_Interleave_)
		fs := make([]bigtable.Filter, 0)
		for _, ipb := range i.Interleave.Filters {
			ipbf := filterFromProto(ipb)
			fs = append(fs, *ipbf)
		}
		inf := bigtable.InterleaveFilters(fs...)
		f = &inf

	case *btpb.RowFilter_Condition_:
		cond := fpb.(*btpb.RowFilter_Condition_)

		tf := filterFromProto(cond.Condition.TrueFilter)
		ff := filterFromProto(cond.Condition.TrueFilter)
		pf := filterFromProto(cond.Condition.PredicateFilter)

		cf := bigtable.ConditionFilter(*pf, *tf, *ff)
		f = &cf

	case *btpb.RowFilter_Sink:
		// Not currently supported.
		f = nil

	case *btpb.RowFilter_PassAllFilter:
		p := bigtable.PassAllFilter()
		f = &p

	case *btpb.RowFilter_BlockAllFilter:
		b := bigtable.BlockAllFilter()
		f = &b

	case *btpb.RowFilter_RowKeyRegexFilter:
		rf := fpb.(*btpb.RowFilter_RowKeyRegexFilter)
		re := rf.RowKeyRegexFilter
		rrf := bigtable.RowKeyFilter(string(re))
		f = &rrf

	case *btpb.RowFilter_RowSampleFilter:
		rsf := fpb.(*btpb.RowFilter_RowSampleFilter)
		rs := rsf.RowSampleFilter
		rf := bigtable.RowSampleFilter(rs)
		f = &rf

	case *btpb.RowFilter_FamilyNameRegexFilter:
		fnf := fpb.(*btpb.RowFilter_FamilyNameRegexFilter)
		re := fnf.FamilyNameRegexFilter
		fn := bigtable.FamilyFilter(re)
		f = &fn

	case *btpb.RowFilter_ColumnQualifierRegexFilter:
		cqf := fpb.(*btpb.RowFilter_ColumnQualifierRegexFilter)
		re := cqf.ColumnQualifierRegexFilter
		cq := bigtable.ColumnFilter(string(re))
		f = &cq

	case *btpb.RowFilter_ColumnRangeFilter:
		crf := fpb.(*btpb.RowFilter_ColumnRangeFilter)
		cr := crf.ColumnRangeFilter

		var start, end string
		switch sf := cr.StartQualifier; sf.(type) {
		case *btpb.ColumnRange_StartQualifierOpen:
			start = string(sf.(*btpb.ColumnRange_StartQualifierOpen).StartQualifierOpen)
		case *btpb.ColumnRange_StartQualifierClosed:
			start = string(sf.(*btpb.ColumnRange_StartQualifierClosed).StartQualifierClosed)
		}

		switch ef := cr.EndQualifier; ef.(type) {
		case *btpb.ColumnRange_EndQualifierClosed:
			end = string(ef.(*btpb.ColumnRange_EndQualifierClosed).EndQualifierClosed)
		case *btpb.ColumnRange_EndQualifierOpen:
			end = string(ef.(*btpb.ColumnRange_EndQualifierOpen).EndQualifierOpen)
		}

		cf := bigtable.ColumnRangeFilter(cr.FamilyName, start, end)
		f = &cf

	case *btpb.RowFilter_TimestampRangeFilter:
		trf := fpb.(*btpb.RowFilter_TimestampRangeFilter)
		tsr := trf.TimestampRangeFilter

		tsf := bigtable.TimestampRangeFilter(time.UnixMicro(tsr.StartTimestampMicros), time.UnixMicro(tsr.EndTimestampMicros))
		f = &tsf

	case *btpb.RowFilter_ValueRegexFilter:
		vrf := fpb.(*btpb.RowFilter_ValueRegexFilter)
		re := vrf.ValueRegexFilter
		vr := bigtable.ValueFilter(string(re))
		f = &vr

	case *btpb.RowFilter_ValueRangeFilter:
		vrf := fpb.(*btpb.RowFilter_ValueRangeFilter)

		var start, end []byte
		switch sv := vrf.ValueRangeFilter.StartValue; sv.(type) {
		case *btpb.ValueRange_StartValueOpen:
			start = sv.(*btpb.ValueRange_StartValueOpen).StartValueOpen
		case *btpb.ValueRange_StartValueClosed:
			start = sv.(*btpb.ValueRange_StartValueClosed).StartValueClosed
		}

		switch ev := vrf.ValueRangeFilter.EndValue; ev.(type) {
		case *btpb.ValueRange_EndValueOpen:
			end = ev.(*btpb.ValueRange_EndValueOpen).EndValueOpen
		case *btpb.ValueRange_EndValueClosed:
			end = ev.(*btpb.ValueRange_EndValueClosed).EndValueClosed
		}

		vr := bigtable.ValueRangeFilter(start, end)
		f = &vr

	case *btpb.RowFilter_CellsPerRowOffsetFilter:
		cof := fpb.(*btpb.RowFilter_CellsPerRowOffsetFilter)
		off := cof.CellsPerRowOffsetFilter
		co := bigtable.CellsPerRowOffsetFilter(int(off))
		f = &co

	case *btpb.RowFilter_CellsPerRowLimitFilter:
		clf := fpb.(*btpb.RowFilter_CellsPerRowLimitFilter)
		lim := clf.CellsPerRowLimitFilter
		cl := bigtable.CellsPerRowLimitFilter(int(lim))
		f = &cl

	case *btpb.RowFilter_CellsPerColumnLimitFilter:
		ccf := fpb.(*btpb.RowFilter_CellsPerColumnLimitFilter)
		lim := ccf.CellsPerColumnLimitFilter
		cc := bigtable.LatestNFilter(int(lim))
		f = &cc

	case *btpb.RowFilter_StripValueTransformer:
		sv := bigtable.StripValueFilter()
		f = &sv

	case *btpb.RowFilter_ApplyLabelTransformer:
		alf := fpb.(*btpb.RowFilter_ApplyLabelTransformer)
		l := alf.ApplyLabelTransformer
		al := bigtable.LabelFilter(l)
		f = &al
	default:
		return nil
	}

	return f
}

// testClient contains a bigtable.Client object, cancel functions for the calls
// made using the client, an appProfileID (optionally), and a
// perOperationTimeout (optionally).
type testClient struct {
	c                   *bigtable.Client     // c stores the Bigtable client under test
	cancels             []context.CancelFunc // cancels stores a cancel() for each call to this client
	cancelsLock         sync.Mutex           // cancelsLock ensures that adding cancels to a client is thread safe
	appProfileID        string               // appProfileID is currently unused
	perOperationTimeout *duration.Duration   // perOperationTimeout sets a custom timeout for methods calls on this client
}

// addCancelFunction appends a context.CancelFunc to testClient.cancels slice.
// It returns a new context object composed from the original.
func (tc *testClient) addCancelFunction(ctx context.Context) context.Context {
	tc.cancelsLock.Lock()
	defer tc.cancelsLock.Unlock()
	ctx2, cancel := context.WithCancel(ctx)
	if tc.perOperationTimeout.AsDuration() > 0 {
		ctx2, cancel = context.WithTimeout(ctx, tc.perOperationTimeout.AsDuration())
	}
	tc.cancels = append(tc.cancels, cancel)
	return ctx2
}

// cancelAll calls all of the context.CancelFuncs stored in this testClient.
func (tc *testClient) cancelAll() {
	tc.cancelsLock.Lock()
	defer tc.cancelsLock.Unlock()
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
	var opts []grpc.DialOption

	if req.CallCredential == nil &&
		req.ChannelCredential == nil &&
		req.OverrideSslTargetName == "" {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
		c, err := gauth.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
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
	clientIDs   map[string]*testClient // clientIDs has all of the bigtable.Client objects under test
	clientsLock sync.Mutex
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

	s.clientsLock.Lock()
	defer s.clientsLock.Unlock()

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
		s.clientIDs = make(map[string]*testClient)
	}

	s.clientIDs[req.ClientId] = &testClient{
		c: c,
		cancels: []context.CancelFunc{
			cancel,
		},
		appProfileID:        req.AppProfileId,
		perOperationTimeout: req.PerOperationTimeout,
	}

	return &pb.CreateClientResponse{}, nil
}

// RemoveClient responds to the RemoveClient RPC. This method removes an
// existing client from the goTestProxyServer
func (s *goTestProxyServer) RemoveClient(ctx context.Context, req *pb.RemoveClientRequest) (*pb.RemoveClientResponse, error) {
	clientID := req.ClientId
	doCancelAll := req.CancelAll

	s.clientsLock.Lock()
	defer s.clientsLock.Unlock()

	btc, exists := s.clientIDs[clientID]
	if !exists {
		return nil, stat.Error(codes.InvalidArgument,
			fmt.Sprintf("%s: ClientID does not exist", logLabel))
	}

	if doCancelAll {
		btc.cancelAll()
	}
	btc.c.Close()
	delete(s.clientIDs, clientID)

	resp := &pb.RemoveClientResponse{}
	return resp, nil
}

// ReadRow responds to the ReadRow RPC. This method gets all of the column
// data for a single row in the Table.
func (s *goTestProxyServer) ReadRow(ctx context.Context, req *pb.ReadRowRequest) (*pb.RowResult, error) {
	return nil, stat.Error(codes.Unimplemented, "ReadRow not implemented")
}

// ReadRows responds to the ReadRows RPC. This method gets all of the column
// data for a set of rows, a range of rows, or the entire table.
func (s *goTestProxyServer) ReadRows(ctx context.Context, req *pb.ReadRowsRequest) (*pb.RowsResult, error) {
	return nil, stat.Error(codes.Unimplemented, "ReadRows not implemented")
}

// MutateRow responds to the MutateRow RPC. This methods applies a series of
// changes (or deletions) to a single row in a table.
func (s *goTestProxyServer) MutateRow(ctx context.Context, req *pb.MutateRowRequest) (*pb.MutateRowResult, error) {
	return nil, stat.Error(codes.Unimplemented, "MutateRow not implemented")
}

// BulkMutateRows responds to the BulkMutateRows RPC. This method applies a
// series of changes or deletions to multiple rows in a single call.
func (s *goTestProxyServer) BulkMutateRows(ctx context.Context, req *pb.MutateRowsRequest) (*pb.MutateRowsResult, error) {
	return nil, stat.Error(codes.Unimplemented, "BulkMutateRows not implemented")
}

// CheckAndMutateRow responds to the CheckAndMutateRow RPC. This method applies
// one mutation if a condition is true and another mutation if it is false.
func (s *goTestProxyServer) CheckAndMutateRow(ctx context.Context, req *pb.CheckAndMutateRowRequest) (*pb.CheckAndMutateRowResult, error) {
	return nil, stat.Error(codes.Unimplemented, "CheckAndMutateRow not implemented")
}

// SampleRowKeys responds to the SampleRowKeys RPC. This method gets a sampling
// of the keys available in a table.
func (s *goTestProxyServer) SampleRowKeys(ctx context.Context, req *pb.SampleRowKeysRequest) (*pb.SampleRowKeysResult, error) {
	return nil, stat.Error(codes.Unimplemented, "SampleRowKeys not implemented")
}

// ReadModifyWriteRow responds to the ReadModifyWriteRow RPC. This method
// applies a non-idempotent change to a row.
func (s *goTestProxyServer) ReadModifyWriteRow(ctx context.Context, req *pb.ReadModifyWriteRowRequest) (*pb.RowResult, error) {
	return nil, stat.Error(codes.Unimplemented, "ReadModifyWriteRow not implemented")
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

	log.Printf("attempting to listen on port %d", *port)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	s := newProxyServer(lis)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
	defer s.Stop()
}
