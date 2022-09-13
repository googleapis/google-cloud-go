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
	"log"
	"net"

	"testing"

	"cloud.google.com/go/bigtable"
	"cloud.google.com/go/bigtable/bttest"
	pb "github.com/googleapis/cloud-bigtable-clients-test/testproxypb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

var lis *bufconn.Listener
var c *pb.CloudBigtableV2TestProxyClient

func bufDialer(context.Context, string) (net.Conn, error) {
	return lis.Dial()
}

/*
TestMain has three threads that it needs to start:
1) The mocked Bigtable service (server)
2) The testproxy server under test
3) The NewCloudBigtableV2TestProxyClient client that sends requests to the testproxy server.

	The communication sequence looks kind of like:

	TestProxyClient <=> test proxy server (what we're testing) <=> Mocked BT server
*/
func TestMain(m *testing.M) {

	lis = bufconn.Listen(bufSize)
	s := newProxyServer(lis)

	srv, err := bttest.NewServer("localhost:9999")
	if err != nil {
		log.Fatalf("can't create inmem Bigtable server")
	}
	conn, err := grpc.Dial(srv.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		log.Fatalf("can't dial inmem Bigtable server")
	}

	adminClient, err := bigtable.NewAdminClient(context.Background(), "client", "instance", option.WithGRPCConn(conn), option.WithGRPCDialOption(grpc.WithBlock()))
	if err != nil {
		log.Fatalf("can't create AdminClient")
	}
	defer adminClient.Close()

	if err := adminClient.CreateTable(context.Background(), "table"); err != nil {
		log.Fatalf("can't create table")
	}
	if err := adminClient.CreateColumnFamily(context.Background(), "table", "cf"); err != nil {
		log.Fatalf("can't create column family")
	}

	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	go func() {

	}()
	m.Run()
}

func TestCreateClient(t *testing.T) {
	// Test
	cid := "testCreateClient"
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "testproxy", grpc.WithContextDialer(bufDialer), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("testproxy test: failed to dial testproxy: %v", err)
	}
	defer conn.Close()
	client := pb.NewCloudBigtableV2TestProxyClient(conn)

	req := &pb.CreateClientRequest{
		ClientId:   cid,
		ProjectId:  "fakeProject",
		DataTarget: "fakeTarget",
		InstanceId: "fakeInstance",
	}

	resp, err := client.CreateClient(ctx, req)
	if err != nil {
		t.Fatalf("testproxy test: CreateClient() failed: %v", err)
	}
	t.Logf("testproxy test: CreateClient() response: %+v", resp)

	// Teardown
	_, err = client.RemoveClient(ctx, &pb.RemoveClientRequest{
		ClientId:  cid,
		CancelAll: true,
	})

	if err != nil {
		t.Errorf("testproxy test: CreateClient() teardown failed: %v", err)
	}
}

func TestRemoveClient(t *testing.T) {
	// Setup
	cid := "testRemoveClient"
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "testproxy", grpc.WithContextDialer(bufDialer), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("testproxy test: failed to dial testproxy: %v", err)
	}
	defer conn.Close()
	client := pb.NewCloudBigtableV2TestProxyClient(conn)

	req := &pb.CreateClientRequest{
		ClientId:   cid,
		ProjectId:  "fakeProject",
		DataTarget: "fakeTarget",
		InstanceId: "fakeInstance",
	}

	_, err = client.CreateClient(ctx, req)
	if err != nil {
		t.Fatalf("testproxy test: failed to create client: %v", err)
	}

	// Test
	resp, err := client.RemoveClient(ctx, &pb.RemoveClientRequest{
		ClientId:  cid,
		CancelAll: true,
	})

	if err != nil {
		t.Errorf("testproxy test: RemoveClient() failed: %v", err)
	}
	t.Logf("testproxy test: RemoveClient() response: %+v", resp)
}
