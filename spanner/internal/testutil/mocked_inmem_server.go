// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package testutil

import (
	"fmt"
	"net"
	"strconv"
	"testing"

	structpb "github.com/golang/protobuf/ptypes/struct"
	"google.golang.org/api/option"
	instancepb "google.golang.org/genproto/googleapis/spanner/admin/instance/v1"
	spannerpb "google.golang.org/genproto/googleapis/spanner/v1"
	"google.golang.org/grpc"
)

// SelectFooFromBar is a SELECT statement that is added to the mocked test
// server and will return a one-col-two-rows result set containing the INT64
// values 1 and 2.
const SelectFooFromBar = "SELECT FOO FROM BAR"
const selectFooFromBarRowCount int64 = 2
const selectFooFromBarColCount int = 1

var selectFooFromBarResults = [...]int64{1, 2}

// SelectSingerIDAlbumIDAlbumTitleFromAlbums i a SELECT statement that is added
// to the mocked test server and will return a 3-cols-3-rows result set.
const SelectSingerIDAlbumIDAlbumTitleFromAlbums = "SELECT SingerId, AlbumId, AlbumTitle FROM Albums"

// SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount is the number of rows
// returned by the SelectSingerIDAlbumIDAlbumTitleFromAlbums statement.
const SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount int64 = 3

// SelectSingerIDAlbumIDAlbumTitleFromAlbumsColCount is the number of cols
// returned by the SelectSingerIDAlbumIDAlbumTitleFromAlbums statement.
const SelectSingerIDAlbumIDAlbumTitleFromAlbumsColCount int = 3

// UpdateBarSetFoo is an UPDATE	statement that is added to the mocked test
// server that will return an update count of 5.
const UpdateBarSetFoo = "UPDATE FOO SET BAR=1 WHERE BAZ=2"

// UpdateBarSetFooRowCount is the constant update count value returned by the
// statement defined in UpdateBarSetFoo.
const UpdateBarSetFooRowCount = 5

// MockedSpannerInMemTestServer is an InMemSpannerServer with results for a
// number of SQL statements readily mocked.
type MockedSpannerInMemTestServer struct {
	TestSpanner       InMemSpannerServer
	TestInstanceAdmin InMemInstanceAdminServer
	server            *grpc.Server
}

// NewMockedSpannerInMemTestServer creates a MockedSpannerInMemTestServer at
// localhost with a random port and returns client options that can be used
// to connect to it.
func NewMockedSpannerInMemTestServer(t *testing.T) (mockedServer *MockedSpannerInMemTestServer, opts []option.ClientOption, teardown func()) {
	return NewMockedSpannerInMemTestServerWithAddr(t, "localhost:0")
}

// NewMockedSpannerInMemTestServerWithAddr creates a MockedSpannerInMemTestServer
// at a given listening address and returns client options that can be used
// to connect to it.
func NewMockedSpannerInMemTestServerWithAddr(t *testing.T, addr string) (mockedServer *MockedSpannerInMemTestServer, opts []option.ClientOption, teardown func()) {
	mockedServer = &MockedSpannerInMemTestServer{}
	opts = mockedServer.setupMockedServerWithAddr(t, addr)
	return mockedServer, opts, func() {
		mockedServer.TestSpanner.Stop()
		mockedServer.TestInstanceAdmin.Stop()
		mockedServer.server.Stop()
	}
}

func (s *MockedSpannerInMemTestServer) setupMockedServerWithAddr(t *testing.T, addr string) []option.ClientOption {
	s.TestSpanner = NewInMemSpannerServer()
	s.TestInstanceAdmin = NewInMemInstanceAdminServer()
	s.setupFooResults()
	s.setupSingersResults()
	s.server = grpc.NewServer()
	spannerpb.RegisterSpannerServer(s.server, s.TestSpanner)
	instancepb.RegisterInstanceAdminServer(s.server, s.TestInstanceAdmin)

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	go s.server.Serve(lis)

	serverAddress := lis.Addr().String()
	opts := []option.ClientOption{
		option.WithEndpoint(serverAddress),
		option.WithGRPCDialOption(grpc.WithInsecure()),
		option.WithoutAuthentication(),
	}
	return opts
}

func (s *MockedSpannerInMemTestServer) setupFooResults() {
	fields := make([]*spannerpb.StructType_Field, selectFooFromBarColCount)
	fields[0] = &spannerpb.StructType_Field{
		Name: "FOO",
		Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64},
	}
	rowType := &spannerpb.StructType{
		Fields: fields,
	}
	metadata := &spannerpb.ResultSetMetadata{
		RowType: rowType,
	}
	rows := make([]*structpb.ListValue, selectFooFromBarRowCount)
	for idx, value := range selectFooFromBarResults {
		rowValue := make([]*structpb.Value, selectFooFromBarColCount)
		rowValue[0] = &structpb.Value{
			Kind: &structpb.Value_StringValue{StringValue: strconv.FormatInt(value, 10)},
		}
		rows[idx] = &structpb.ListValue{
			Values: rowValue,
		}
	}
	resultSet := &spannerpb.ResultSet{
		Metadata: metadata,
		Rows:     rows,
	}
	result := &StatementResult{Type: StatementResultResultSet, ResultSet: resultSet}
	s.TestSpanner.PutStatementResult(SelectFooFromBar, result)
	s.TestSpanner.PutStatementResult(UpdateBarSetFoo, &StatementResult{
		Type:        StatementResultUpdateCount,
		UpdateCount: UpdateBarSetFooRowCount,
	})
}

func (s *MockedSpannerInMemTestServer) setupSingersResults() {
	metadata := createSingersMetadata()
	rows := make([]*structpb.ListValue, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
	var idx int64
	for idx = 0; idx < SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount; idx++ {
		rows[idx] = createSingersRow(idx)
	}
	resultSet := &spannerpb.ResultSet{
		Metadata: metadata,
		Rows:     rows,
	}
	result := &StatementResult{Type: StatementResultResultSet, ResultSet: resultSet}
	s.TestSpanner.PutStatementResult(SelectSingerIDAlbumIDAlbumTitleFromAlbums, result)
}

// CreateSingleRowSingersResult creates a result set containing a single row of
// the SelectSingerIDAlbumIDAlbumTitleFromAlbums result set, or zero rows if
// the given rowNum is greater than the number of rows in the result set. This
// method can be used to mock results for different partitions of a
// BatchReadOnlyTransaction.
func (s *MockedSpannerInMemTestServer) CreateSingleRowSingersResult(rowNum int64) *StatementResult {
	metadata := createSingersMetadata()
	var returnedRows int
	if rowNum < SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount {
		returnedRows = 1
	} else {
		returnedRows = 0
	}
	rows := make([]*structpb.ListValue, returnedRows)
	if returnedRows > 0 {
		rows[0] = createSingersRow(rowNum)
	}
	resultSet := &spannerpb.ResultSet{
		Metadata: metadata,
		Rows:     rows,
	}
	return &StatementResult{Type: StatementResultResultSet, ResultSet: resultSet}
}

func createSingersMetadata() *spannerpb.ResultSetMetadata {
	fields := make([]*spannerpb.StructType_Field, SelectSingerIDAlbumIDAlbumTitleFromAlbumsColCount)
	fields[0] = &spannerpb.StructType_Field{
		Name: "SingerId",
		Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64},
	}
	fields[1] = &spannerpb.StructType_Field{
		Name: "AlbumId",
		Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64},
	}
	fields[2] = &spannerpb.StructType_Field{
		Name: "AlbumTitle",
		Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING},
	}
	rowType := &spannerpb.StructType{
		Fields: fields,
	}
	return &spannerpb.ResultSetMetadata{
		RowType: rowType,
	}
}

func createSingersRow(idx int64) *structpb.ListValue {
	rowValue := make([]*structpb.Value, SelectSingerIDAlbumIDAlbumTitleFromAlbumsColCount)
	rowValue[0] = &structpb.Value{
		Kind: &structpb.Value_StringValue{StringValue: strconv.FormatInt(idx+1, 10)},
	}
	rowValue[1] = &structpb.Value{
		Kind: &structpb.Value_StringValue{StringValue: strconv.FormatInt(idx*10+idx, 10)},
	}
	rowValue[2] = &structpb.Value{
		Kind: &structpb.Value_StringValue{StringValue: fmt.Sprintf("Album title %d", idx)},
	}
	return &structpb.ListValue{
		Values: rowValue,
	}
}
