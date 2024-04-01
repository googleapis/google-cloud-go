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
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/bigtable"
	pb "github.com/googleapis/cloud-bigtable-clients-test/testproxypb"
	"google.golang.org/api/option"
	btpb "google.golang.org/genproto/googleapis/bigtable/v2"
	statpb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	stat "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
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
	rowKeys := rs.GetRowKeys()
	rowRanges := rs.GetRowRanges()

	if len(rowKeys) == 0 && len(rowRanges) == 0 {
		return nil
	}

	// Convert all rowKeys into single-row RowRanges
	if len(rowKeys) > 0 {
		var rowList bigtable.RowList
		for _, b := range rowKeys {
			rowList = append(rowList, string(b))
		}
		return rowList
	}

	if len(rowRanges) > 0 {
		rowRangeList := make(bigtable.RowRangeList, 0)
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
		return rowRangeList
	}
	return nil
}

// mutationFromProto translates a slice of Bigtable v2.Mutation objects into
// a single Bigtable.Mutation object.
func mutationFromProto(mPbs []*btpb.Mutation) *bigtable.Mutation {
	m := bigtable.NewMutation()
	for _, mpb := range mPbs {

		switch mut := mpb.Mutation.(type) {
		case *btpb.Mutation_DeleteFromColumn_:
			del := mut
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
			del := mut
			fam := del.DeleteFromFamily.FamilyName
			m.DeleteCellsInFamily(fam)

		case *btpb.Mutation_DeleteFromRow_:
			m.DeleteRow()

		case *btpb.Mutation_SetCell_:
			setCell := mut
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
	switch fpb := rfPb.Filter.(type) {
	case *btpb.RowFilter_Chain_:
		c := fpb
		var fs []bigtable.Filter
		for _, cfpb := range c.Chain.Filters {
			cf := filterFromProto(cfpb)
			fs = append(fs, *cf)
		}
		cf := bigtable.ChainFilters(fs...)
		f = &cf

	case *btpb.RowFilter_Interleave_:
		i := fpb
		fs := make([]bigtable.Filter, 0)
		for _, ipb := range i.Interleave.Filters {
			ipbf := filterFromProto(ipb)
			fs = append(fs, *ipbf)
		}
		inf := bigtable.InterleaveFilters(fs...)
		f = &inf

	case *btpb.RowFilter_Condition_:
		cond := fpb

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
		rf := fpb
		re := rf.RowKeyRegexFilter
		rrf := bigtable.RowKeyFilter(string(re))
		f = &rrf

	case *btpb.RowFilter_RowSampleFilter:
		rsf := fpb
		rs := rsf.RowSampleFilter
		rf := bigtable.RowSampleFilter(rs)
		f = &rf

	case *btpb.RowFilter_FamilyNameRegexFilter:
		fnf := fpb
		re := fnf.FamilyNameRegexFilter
		fn := bigtable.FamilyFilter(re)
		f = &fn

	case *btpb.RowFilter_ColumnQualifierRegexFilter:
		cqf := fpb
		re := cqf.ColumnQualifierRegexFilter
		cq := bigtable.ColumnFilter(string(re))
		f = &cq

	case *btpb.RowFilter_ColumnRangeFilter:
		crf := fpb
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
		trf := fpb
		tsr := trf.TimestampRangeFilter

		tsf := bigtable.TimestampRangeFilter(time.UnixMicro(tsr.StartTimestampMicros), time.UnixMicro(tsr.EndTimestampMicros))
		f = &tsf

	case *btpb.RowFilter_ValueRegexFilter:
		vrf := fpb
		re := vrf.ValueRegexFilter
		vr := bigtable.ValueFilter(string(re))
		f = &vr

	case *btpb.RowFilter_ValueRangeFilter:
		vrf := fpb

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
		cof := fpb
		off := cof.CellsPerRowOffsetFilter
		co := bigtable.CellsPerRowOffsetFilter(int(off))
		f = &co

	case *btpb.RowFilter_CellsPerRowLimitFilter:
		clf := fpb
		lim := clf.CellsPerRowLimitFilter
		cl := bigtable.CellsPerRowLimitFilter(int(lim))
		f = &cl

	case *btpb.RowFilter_CellsPerColumnLimitFilter:
		ccf := fpb
		lim := ccf.CellsPerColumnLimitFilter
		cc := bigtable.LatestNFilter(int(lim))
		f = &cc

	case *btpb.RowFilter_StripValueTransformer:
		sv := bigtable.StripValueFilter()
		f = &sv

	case *btpb.RowFilter_ApplyLabelTransformer:
		alf := fpb
		l := alf.ApplyLabelTransformer
		al := bigtable.LabelFilter(l)
		f = &al
	}
	return f
}

// statusFromError converts an error into a Status code.
func statusFromError(err error) *statpb.Status {
	log.Printf("error: %v\n", err)
	st := &statpb.Status{
		Code:    int32(codes.Unknown),
		Message: fmt.Sprintf("%v", err),
	}
	if s, ok := stat.FromError(err); ok {
		st = &statpb.Status{
			Code:    int32(s.Code()),
			Message: s.Message(),
		}
	}
	return st
}

// parseTableID extracts a table ID from a table name.
// For example, a table ID is in the format projects/<project>/instances/<instance>/tables/<tableID>
//
// Note that this function does not check all variants and edge cases. It assumes
// that the test suite used with the test proxy sends *generally* correct requests.
func parseTableID(tableName string) (tableID string, _ error) {
	paths := strings.Split(tableName, "/")

	if len(paths) < 6 {
		return "", errors.New("table resource name does not have the correct format")
	}

	tableID = paths[len(paths)-1]
	var err error
	if tableID == "" {
		err = errors.New("cannot read tableID from table name")
	}

	return tableID, err
}

// testClient contains a bigtable.Client object, cancel functions for the calls
// made using the client, an appProfileID (optionally), and a
// perOperationTimeout (optionally).
type testClient struct {
	c                   *bigtable.Client     // c stores the Bigtable client under test
	appProfileID        string               // appProfileID is currently unused
	perOperationTimeout *durationpb.Duration // perOperationTimeout sets a custom timeout for methods calls on this client
}

// timeout adds a timeout setting to a context if perOperationTimeout is set on
// the testClient object.
func (tc *testClient) timeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if tc.perOperationTimeout != nil {
		return context.WithTimeout(ctx, tc.perOperationTimeout.AsDuration())
	}
	return context.WithCancel(ctx)
}

// getCredentialsOptions provides credentials for a Bigtable client.
//
// Note: this proxy uses insecure credentials. This function may need to be
// expanded to support different credential types.
func getCredentialsOptions(req *pb.CreateClientRequest) (opts []grpc.DialOption, _ error) {
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	return opts, nil
}

// goTestProxyServer represents an instance of the test proxy server. It keeps
// a reference to individual clients instances (stored in a testClient object).
type goTestProxyServer struct {
	pb.UnimplementedCloudBigtableV2TestProxyServer
	clientsLock sync.RWMutex           // clientsLock prevents simultaneous mutation of the clientIDs map
	clientIDs   map[string]*testClient // clientIDs has all of the bigtable.Client objects under test
}

// client retrieves a testClient from the clientIDs map. You must lock clientsLock before calling
// this method.
func (s *goTestProxyServer) client(clientID string) (*testClient, error) {
	client, ok := s.clientIDs[clientID]
	if !ok {
		return nil, fmt.Errorf("client ID %s does not exist", clientID)
	}
	return client, nil
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

	config := bigtable.ClientConfig{
		AppProfile: req.AppProfileId,
	}
	c, err := bigtable.NewClientWithConfig(ctx, req.ProjectId, req.InstanceId, config, option.WithGRPCConn(conn))
	if err != nil {
		return nil, stat.Error(codes.Internal,
			fmt.Sprintf("%s: failed to create client: %v", logLabel, err))
	}

	s.clientIDs[req.ClientId] = &testClient{
		c:                   c,
		appProfileID:        req.AppProfileId,
		perOperationTimeout: req.PerOperationTimeout,
	}

	return &pb.CreateClientResponse{}, nil
}

// CloseClient responds to the CloseClient RPC. This method closes an existing
// client, making it inaccessible to new requests.
func (s *goTestProxyServer) CloseClient(ctx context.Context, req *pb.CloseClientRequest) (*pb.CloseClientResponse, error) {
	clientID := req.ClientId
	s.clientsLock.Lock()
	defer s.clientsLock.Unlock()

	btc, err := s.client(clientID)
	if err != nil {
		return nil, err
	}
	btc.c.Close()

	return &pb.CloseClientResponse{}, nil
}

// RemoveClient responds to the RemoveClient RPC. This method removes an
// existing client from the goTestProxyServer
func (s *goTestProxyServer) RemoveClient(ctx context.Context, req *pb.RemoveClientRequest) (*pb.RemoveClientResponse, error) {
	clientID := req.ClientId

	s.clientsLock.Lock()
	defer s.clientsLock.Unlock()

	// RemoveClient can ignore whether the client accepts new requests
	_, err := s.client(clientID)
	if err != nil {
		return nil, stat.Error(codes.InvalidArgument,
			fmt.Sprintf("%s: ClientID does not exist", logLabel))
	}
	delete(s.clientIDs, clientID)

	return &pb.RemoveClientResponse{}, nil
}

// ReadRow responds to the ReadRow RPC. This method gets all of the column
// data for a single row in the Table.
func (s *goTestProxyServer) ReadRow(ctx context.Context, req *pb.ReadRowRequest) (*pb.RowResult, error) {
	s.clientsLock.RLock()
	btc, err := s.client(req.ClientId)
	if err != nil {
		return nil, err
	}
	s.clientsLock.RUnlock()

	tid, err := parseTableID(req.TableName)
	if err != nil {
		return nil, err
	}
	t := btc.c.Open(tid)

	res := &pb.RowResult{
		Status: &statpb.Status{
			Code: int32(codes.OK),
		},
		Row: &btpb.Row{},
	}

	ctx, cancel := btc.timeout(ctx)
	defer cancel()

	r, err := t.ReadRow(ctx, req.RowKey)
	if err != nil {
		res.Status = statusFromError(err)
		return res, nil
	}

	if r == nil {
		return res, nil
	}

	pbRow, err := rowToProto(r)
	if err != nil {
		return nil, err
	}

	res.Row = pbRow
	return res, nil
}

// ReadRows responds to the ReadRows RPC. This method gets all of the column
// data for a set of rows, a range of rows, or the entire table.
func (s *goTestProxyServer) ReadRows(ctx context.Context, req *pb.ReadRowsRequest) (*pb.RowsResult, error) {
	s.clientsLock.RLock()
	btc, err := s.client(req.ClientId)
	s.clientsLock.RUnlock()

	if err != nil {
		return nil, err
	}

	rrq := req.GetRequest()

	if rrq == nil {
		log.Printf("missing inner request: %v\n", rrq)
		return nil, stat.Error(codes.InvalidArgument, "request to ReadRows() is missing inner request")

	}

	tid, err := parseTableID(rrq.TableName)
	if err != nil {
		return nil, err
	}
	t := btc.c.Open(tid)

	rowPbs := rrq.Rows
	rs := rowSetFromProto(rowPbs)

	ctx, cancel := btc.timeout(ctx)
	defer cancel()

	var c int32
	var rowsPb []*btpb.Row
	lim := req.GetCancelAfterRows()
	err = t.ReadRows(ctx, rs, func(r bigtable.Row) bool {

		c++
		if c == lim {
			return false
		}
		rpb, err := rowToProto(r)
		if err != nil {
			return false
		}
		rowsPb = append(rowsPb, rpb)
		return true
	})

	res := &pb.RowsResult{
		Status: &statpb.Status{
			Code: int32(codes.OK),
		},
		Rows: []*btpb.Row{},
	}

	if err != nil {
		res.Status = statusFromError(err)
		return res, nil
	}

	res.Rows = rowsPb

	return res, nil
}

// MutateRow responds to the MutateRow RPC. This methods applies a series of
// changes (or deletions) to a single row in a table.
func (s *goTestProxyServer) MutateRow(ctx context.Context, req *pb.MutateRowRequest) (*pb.MutateRowResult, error) {
	s.clientsLock.RLock()
	btc, err := s.client(req.ClientId)
	s.clientsLock.RUnlock()

	if err != nil {
		return nil, err
	}

	rrq := req.GetRequest()
	if rrq == nil {
		return nil, stat.Error(codes.InvalidArgument, "request to MutateRow() is missing inner request")
	}

	mPbs := rrq.Mutations
	m := mutationFromProto(mPbs)

	tid, err := parseTableID(rrq.TableName)
	if err != nil {
		return nil, err
	}
	t := btc.c.Open(tid)
	row := rrq.RowKey

	res := &pb.MutateRowResult{
		Status: &statpb.Status{
			Code: int32(codes.OK),
		},
	}

	ctx, cancel := btc.timeout(ctx)
	defer cancel()

	err = t.Apply(ctx, string(row), m)
	if err != nil {
		res.Status = statusFromError(err)
		return res, nil
	}

	return res, nil
}

// BulkMutateRows responds to the BulkMutateRows RPC. This method applies a
// series of changes or deletions to multiple rows in a single call.
func (s *goTestProxyServer) BulkMutateRows(ctx context.Context, req *pb.MutateRowsRequest) (*pb.MutateRowsResult, error) {
	s.clientsLock.RLock()
	btc, err := s.client(req.ClientId)
	s.clientsLock.RUnlock()

	if err != nil {
		return nil, err
	}

	rrq := req.GetRequest()
	if rrq == nil {
		log.Printf("missing inner request to BulkMutateRows: %v\n", req)
		return nil, stat.Error(codes.InvalidArgument, "request to BulkMutateRows() is missing inner request")
	}

	mrs := rrq.Entries
	tid, err := parseTableID(rrq.TableName)
	if err != nil {
		return nil, err
	}
	t := btc.c.Open(tid)

	keys := make([]string, len(mrs))
	muts := make([]*bigtable.Mutation, len(mrs))

	for i, mr := range mrs {

		key := string(mr.RowKey)
		m := mutationFromProto(mr.Mutations)

		// A little tricky here ... each key corresponds to a single Mutation
		// object, where the indices of each slice must be sync'ed.
		keys[i] = key
		muts[i] = m
	}

	res := &pb.MutateRowsResult{
		Status: &statpb.Status{
			Code: int32(codes.OK),
		},
	}

	ctx, cancel := btc.timeout(ctx)
	defer cancel()

	errs, err := t.ApplyBulk(ctx, keys, muts)
	if err != nil {
		log.Printf("received error from Table.ApplyBulk(): %v", err)
		res.Status = statusFromError(err)
	}

	var entries []*btpb.MutateRowsResponse_Entry

	// Iterate over any errors returned, matching indices with errors. If
	// errs is nil, this block is skipped.
	for i, e := range errs {
		var me *btpb.MutateRowsResponse_Entry
		if e != nil {
			st := statusFromError(err)
			me = &btpb.MutateRowsResponse_Entry{
				Index:  int64(i),
				Status: st,
			}
			entries = append(entries, me)
		}
	}

	res.Entries = entries
	return res, nil
}

// CheckAndMutateRow responds to the CheckAndMutateRow RPC. This method applies
// one mutation if a condition is true and another mutation if it is false.
func (s *goTestProxyServer) CheckAndMutateRow(ctx context.Context, req *pb.CheckAndMutateRowRequest) (*pb.CheckAndMutateRowResult, error) {
	s.clientsLock.RLock()
	btc, err := s.client(req.ClientId)
	s.clientsLock.RUnlock()

	if err != nil {
		return nil, err
	}

	rrq := req.GetRequest()
	if rrq == nil {
		log.Printf("request to CheckAndMutateRow is missing inner request: received: %v", req)
		return nil, stat.Error(codes.InvalidArgument, "request to CheckAndMutateRow() is missing inner request")
	}

	trueMuts := mutationFromProto(rrq.TrueMutations)
	falseMuts := mutationFromProto(rrq.FalseMutations)

	rfPb := rrq.PredicateFilter
	f := bigtable.PassAllFilter()

	if rfPb != nil {
		f = *filterFromProto(rfPb)
	}

	c := bigtable.NewCondMutation(f, trueMuts, falseMuts)

	res := &pb.CheckAndMutateRowResult{
		Status: &statpb.Status{
			Code: int32(codes.OK),
		},
	}

	tid, err := parseTableID(rrq.TableName)
	if err != nil {
		return nil, err
	}
	t := btc.c.Open(tid)
	rowKey := string(rrq.RowKey)

	var matched bool
	ao := bigtable.GetCondMutationResult(&matched)

	ctx, cancel := btc.timeout(ctx)
	defer cancel()

	err = t.Apply(ctx, rowKey, c, ao)
	if err != nil {
		log.Printf("received error from Table.Apply: %v", err)
		res.Status = statusFromError(err)
		return res, nil
	}

	res.Result = &btpb.CheckAndMutateRowResponse{
		PredicateMatched: matched,
	}

	return res, nil
}

// SampleRowKeys responds to the SampleRowKeys RPC. This method gets a sampling
// of the keys available in a table.
func (s *goTestProxyServer) SampleRowKeys(ctx context.Context, req *pb.SampleRowKeysRequest) (*pb.SampleRowKeysResult, error) {
	s.clientsLock.RLock()
	btc, err := s.client(req.ClientId)
	s.clientsLock.RUnlock()

	if err != nil {
		return nil, err
	}

	rrq := req.GetRequest()
	if rrq == nil {
		log.Printf("missing inner request to SampleRowKeys: %v\n", req)
		return nil, stat.Error(codes.InvalidArgument, "request to SampleRowKeys() is missing inner request")
	}

	res := &pb.SampleRowKeysResult{
		Status: &statpb.Status{
			Code: int32(codes.OK),
		},
	}

	ctx, cancel := btc.timeout(ctx)
	defer cancel()

	tid, err := parseTableID(rrq.TableName)
	if err != nil {
		return nil, err
	}
	t := btc.c.Open(tid)
	keys, err := t.SampleRowKeys(ctx)
	if err != nil {
		log.Printf("received error from Table.SampleRowKeys(): %v\n", err)
		res.Status = statusFromError(err)
		return res, nil
	}

	sk := make([]*btpb.SampleRowKeysResponse, 0)
	for _, k := range keys {
		s := &btpb.SampleRowKeysResponse{
			RowKey: []byte(k),
		}
		sk = append(sk, s)
	}

	res.Samples = sk

	return res, nil
}

// ReadModifyWriteRow responds to the ReadModifyWriteRow RPC. This method
// applies a non-idempotent change to a row.
func (s *goTestProxyServer) ReadModifyWriteRow(ctx context.Context, req *pb.ReadModifyWriteRowRequest) (*pb.RowResult, error) {
	s.clientsLock.RLock()
	btc, err := s.client(req.ClientId)
	s.clientsLock.RUnlock()

	if err != nil {
		return nil, err
	}

	rrq := req.GetRequest()
	if rrq == nil {
		log.Printf("missing inner request to ReadModifyWriteRow: %v\n", req)
		return nil, stat.Error(codes.InvalidArgument, "request to CheckAndMutateRow() is missing inner request")
	}

	rpb := rrq.Rules
	rmw := bigtable.NewReadModifyWrite()

	for _, rp := range rpb {
		switch r := rp.Rule.(type) {
		case *btpb.ReadModifyWriteRule_AppendValue:
			av := r
			rmw.AppendValue(rp.FamilyName, string(rp.ColumnQualifier), av.AppendValue)
		case *btpb.ReadModifyWriteRule_IncrementAmount:
			ia := r
			rmw.Increment(rp.FamilyName, string(rp.ColumnQualifier), ia.IncrementAmount)
		}
	}

	res := &pb.RowResult{
		Status: &statpb.Status{
			Code: int32(codes.OK),
		},
	}

	tid, err := parseTableID(rrq.TableName)
	if err != nil {
		return nil, err
	}
	t := btc.c.Open(tid)
	k := string(rrq.RowKey)

	ctx, cancel := btc.timeout(ctx)
	defer cancel()

	r, err := t.ApplyReadModifyWrite(ctx, k, rmw)
	if err != nil {
		res.Status = statusFromError(err)
		return res, nil
	}

	rp, err := rowToProto(r)
	if err != nil {
		res.Status = statusFromError(err)
		return res, nil
	}

	res.Row = rp
	return res, nil
}

func (s *goTestProxyServer) mustEmbedUnimplementedCloudBigtableV2TestProxyServer() {}

func newProxyServer(lis net.Listener) *grpc.Server {
	s := grpc.NewServer()

	tps := &goTestProxyServer{
		clientIDs: make(map[string]*testClient),
	}

	pb.RegisterCloudBigtableV2TestProxyServer(s, tps)
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
