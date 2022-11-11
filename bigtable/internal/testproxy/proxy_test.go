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
	"fmt"
	"log"
	"net"
	"os"

	"testing"

	"cloud.google.com/go/bigtable"
	"cloud.google.com/go/bigtable/bttest"
	pb "github.com/googleapis/cloud-bigtable-clients-test/testproxypb"
	"google.golang.org/api/option"
	btpb "google.golang.org/genproto/googleapis/bigtable/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const (
	buffer           = 1024 * 1024
	tableName        = "projects/my-project/instances/my-instance/tables/table"
	tableID          = "table"
	columnFamily     = "cf"
	testProxyClient  = "testProxyClient"
	testProxyAddress = "localhost:9990"
	bigtableAddress  = "localhost:9999"
	rowKey           = "row"
)

var (
	lis    *bufconn.Listener
	client pb.CloudBigtableV2TestProxyClient
)

func bufDialer(context.Context, string) (net.Conn, error) {
	return lis.Dial()
}

// helper function to populate the in-memory BT table.
func populateTable(bts *bttest.Server) error {
	ctx := context.Background()

	conn, err := grpc.Dial(bts.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("testproxy setup: can't dial inmem Bigtable server: %v", err)
	}
	defer conn.Close()

	adminClient, err := bigtable.NewAdminClient(ctx, "client", "instance",
		option.WithGRPCConn(conn), option.WithGRPCDialOption(grpc.WithBlock()))
	if err != nil {
		return fmt.Errorf("testproxy setup: can't create AdminClient: %v", err)
	}
	defer adminClient.Close()

	if err := adminClient.CreateTable(ctx, tableID); err != nil {
		return fmt.Errorf("testproxy setup: can't create table: %v", err)
	}

	// Create column families (3 is an arbitrarily sufficient number)
	count := 3
	for i := 0; i < count; i++ {
		cfName := fmt.Sprintf("%s%d", columnFamily, i)
		if err := adminClient.CreateColumnFamily(ctx, tableID, cfName); err != nil {
			return fmt.Errorf("testproxy setup: can't create column family: %s", cfName)
		}
	}

	dataClient, err := bigtable.NewClient(ctx, "client", "instance",
		option.WithGRPCConn(conn), option.WithGRPCDialOption(grpc.WithBlock()))
	if err != nil {
		return fmt.Errorf("testproxy setup: can't create Bigtable client: %v", err)
	}
	defer dataClient.Close()

	t := dataClient.Open(tableID)

	for fc := 0; fc < count; fc++ {
		for cc := count; cc > 0; cc-- {
			for tc := 0; tc < count; tc++ {
				rmw := bigtable.NewReadModifyWrite()
				rmw.AppendValue(fmt.Sprintf("%s%d", columnFamily, fc), fmt.Sprintf("coll%d", cc), []byte("test data"))

				_, err = t.ApplyReadModifyWrite(ctx, rowKey, rmw)
				if err != nil {
					return fmt.Errorf("testproxy setup: failure populating row: %v", err)
				}
			}
		}
	}

	return nil
}

/*
TestMain has three threads that it needs to start:
1) The mocked Bigtable service (server)
2) The NewCloudBigtableV2TestProxyClient client that sends requests to the testproxy server.
3) The testproxy server under test

	The communication sequence looks kind of like:

	TestProxyClient <=> test proxy server (what we're testing) <=> mocked BT server
*/
func TestMain(m *testing.M) {
	ctx := context.Background()

	// 1) Start the mocked Bigtable service
	// This requires creating a "table" in memory
	bts, err := bttest.NewServer(bigtableAddress)
	if err != nil {
		log.Fatalf("testproxy setup: can't create inmem Bigtable server")
	}
	err = populateTable(bts)
	if err != nil {
		log.Fatalf("testproxy setup: can't populate mock table")
	}

	// 3) Start the test proxy server
	lis = bufconn.Listen(buffer)
	s := newProxyServer(lis)
	defer s.Stop()
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	// 2) Create the test proxy client
	conn2, err := grpc.DialContext(ctx, lis.Addr().String(),
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("testproxy setup: failed to dial testproxy: %v", err)
	}
	defer conn2.Close()
	client = pb.NewCloudBigtableV2TestProxyClient(conn2)

	// This could create a little bit of a race condition with the previous
	// go routine ...
	// However, we need to have the test proxy server running in order to create
	// a reusable client stored in its memory
	req := &pb.CreateClientRequest{
		ClientId:   testProxyClient,
		ProjectId:  "client",
		DataTarget: bigtableAddress,
		InstanceId: "instance",
	}

	_, err = client.CreateClient(ctx, req)
	if err != nil {
		log.Fatalf("testproxy setup:  CreateClient() failed: %v", err)
	}

	os.Exit(m.Run())
}

func TestCreateAndRemoveClient(t *testing.T) {
	// Test
	cid := "testCreateClient"
	ctx := context.Background()

	req := &pb.CreateClientRequest{
		ClientId:   cid,
		ProjectId:  "client",
		DataTarget: bigtableAddress,
		InstanceId: "instance",
	}

	_, err := client.CreateClient(ctx, req)

	if err != nil {
		t.Fatalf("testproxy test: CreateClient() failed: %v", err)
	}

	t.Log("testproxy test: client created successfully in test proxy")

	_, err = client.RemoveClient(ctx, &pb.RemoveClientRequest{
		ClientId: cid,
	})

	if err != nil {
		t.Errorf("testproxy test: RemoveClient() failed: %v", err)
	}
}

func TestReadRow(t *testing.T) {
	ctx := context.Background()
	req := &pb.ReadRowRequest{
		TableName: tableName,
		ClientId:  testProxyClient,
		RowKey:    rowKey,
	}

	resp, err := client.ReadRow(ctx, req)
	if err != nil {
		t.Fatalf("testproxy test: ReadRow() failed: %v", err)
	}

	stat := resp.Status.Code
	if stat != int32(codes.OK) {
		t.Errorf("testproxy test: ReadRow() didn't return OK")
	}

	row := resp.Row
	if string(row.Key) != "row" {
		t.Errorf("testproxy test: ReadRow() returned wrong row")
	}
}

func TestBulkMutateRows(t *testing.T) {
	ctx := context.Background()
	req := &pb.MutateRowsRequest{
		ClientId: testProxyClient,
		Request: &btpb.MutateRowsRequest{
			TableName: tableName,
			Entries: []*btpb.MutateRowsRequest_Entry{
				{
					RowKey: []byte(rowKey),
					Mutations: []*btpb.Mutation{
						{
							Mutation: &btpb.Mutation_SetCell_{
								SetCell: &btpb.Mutation_SetCell{
									ColumnQualifier: []byte("coll2"),
									FamilyName:      "cf0",
									Value:           []byte("bulked up mutant!"),
								},
							},
						},
					},
				},
			},
		},
	}

	resp, err := client.BulkMutateRows(ctx, req)
	if err != nil {
		t.Fatalf("testproxy test: BulkMutateRows returned error: %v", err)
	}

	if resp.Status.Code != int32(codes.OK) {
		t.Errorf("testproxy test: BulkMutateRows() didn't return OK; got %v", resp.Status.Code)
	}

	if len(resp.Entry) != 0 {
		t.Errorf("testproxy test: BulkMutateRows() returned individual errors; got %v", resp.Entry)
	}
}

func TestMutateRow(t *testing.T) {
	ctx := context.Background()
	req := &pb.MutateRowRequest{
		ClientId: testProxyClient,
		Request: &btpb.MutateRowRequest{
			TableName: tableName,
			RowKey:    []byte(rowKey),
			Mutations: []*btpb.Mutation{
				{
					Mutation: &btpb.Mutation_SetCell_{
						SetCell: &btpb.Mutation_SetCell{
							ColumnQualifier: []byte("coll1"),
							FamilyName:      "cf0",
							Value:           []byte("mutant!"),
						},
					},
				},
			},
		},
	}

	resp, err := client.MutateRow(ctx, req)
	if err != nil {
		t.Fatalf("testproxy test: MutateRow() returned error: %v", err)
	}

	if resp.Status.Code != int32(codes.OK) {
		t.Errorf("testproxy test: MutateRow() didn't return OK; got %v", resp.Status.Code)

	}
}

func TestReadRows(t *testing.T) {
	ctx := context.Background()
	req := &pb.ReadRowsRequest{
		ClientId: testProxyClient,
		Request: &btpb.ReadRowsRequest{
			TableName: tableName,
		},
	}

	resp, err := client.ReadRows(ctx, req)
	if err != nil {
		t.Fatalf("testproxy test: ReadRows returned error: %v", err)
	}

	if resp.Status.Code != int32(codes.OK) {
		t.Errorf("testproxy test: ReadRows() didn't return OK; got %v", resp.Status.Code)
	}

	if len(resp.Row) != 1 {
		t.Errorf("testproxy test: SampleRowKeys() returned wrong number of results; got: %d", len(resp.Row))

	}
}

func TestCheckAndMutateRow(t *testing.T) {
	ctx := context.Background()
	req := &pb.CheckAndMutateRowRequest{
		ClientId: testProxyClient,
		Request: &btpb.CheckAndMutateRowRequest{
			TableName: tableName,
			RowKey:    []byte(rowKey),
			PredicateFilter: &btpb.RowFilter{
				Filter: &btpb.RowFilter_PassAllFilter{},
			},
			TrueMutations: []*btpb.Mutation{
				{
					Mutation: &btpb.Mutation_SetCell_{
						SetCell: &btpb.Mutation_SetCell{
							ColumnQualifier: []byte("coll1"),
							FamilyName:      "cf0",
							Value:           []byte("check success"),
						},
					},
				},
			},
			FalseMutations: []*btpb.Mutation{
				{
					Mutation: &btpb.Mutation_SetCell_{
						SetCell: &btpb.Mutation_SetCell{
							ColumnQualifier: []byte("coll1"),
							FamilyName:      "cf0",
							Value:           []byte("check failed!"),
						},
					},
				},
			},
		},
	}

	resp, err := client.CheckAndMutateRow(ctx, req)
	if err != nil {
		t.Fatalf("testproxy test: CheckAndMutateRow() returned error: %v", err)
	}

	if resp.Status.Code != int32(codes.OK) {
		t.Errorf("testproxy test: CheckAndMutateRow() didn't return OK; got %v", resp.Status.Code)
	}

	if resp.Result.PredicateMatched != true {
		t.Errorf("testproxy test: CheckAndMutateRow() returned wrong results; got: %v", resp.Result.PredicateMatched)
	}
}

func TestSampleRowKeys(t *testing.T) {
	ctx := context.Background()
	req := &pb.SampleRowKeysRequest{
		ClientId: testProxyClient,
		Request: &btpb.SampleRowKeysRequest{
			TableName: tableName,
		},
	}

	resp, err := client.SampleRowKeys(ctx, req)
	if err != nil {
		t.Fatalf("testproxy test: SampleRowKeys() returned error: %v", err)
	}

	if resp.Status.Code != int32(codes.OK) {
		t.Errorf("testproxy test: SampleRowKeys() didn't return OK; got %v", resp.Status.Code)
	}

	if len(resp.Sample) != 1 {
		t.Errorf("testproxy test: SampleRowKeys() returned wrong number of results; got: %d", len(resp.Sample))
	}
}

func TestReadWriteRow(t *testing.T) {
	ctx := context.Background()
	req := &pb.ReadModifyWriteRowRequest{
		ClientId: testProxyClient,
		Request: &btpb.ReadModifyWriteRowRequest{
			TableName: tableName,
			RowKey:    []byte(rowKey),
			Rules: []*btpb.ReadModifyWriteRule{
				{
					Rule: &btpb.ReadModifyWriteRule_AppendValue{
						AppendValue: []byte("appended!"),
					},
					FamilyName:      "cf0",
					ColumnQualifier: []byte("coll1"),
				},
			},
		},
	}

	resp, err := client.ReadModifyWriteRow(ctx, req)
	if err != nil {
		t.Fatalf("testproxy test: ReadModifyWriteRow() returned error: %v", err)
	}

	if resp.Status.Code != int32(codes.OK) {
		t.Errorf("testproxy test: ReadModifyWriteRow() didn't return OK; got %v", resp.Status.Code)
	}

	if string(resp.Row.Key) != rowKey {
		t.Errorf("testproxy test: ReadModifyWriteRow() returned wrong results; got: %v", resp.Row.Key)
	}
}
