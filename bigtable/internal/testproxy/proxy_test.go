package main

import (
	"context"
	"log"
	"net"

	"testing"

	pb "github.com/googleapis/cloud-bigtable-clients-test/testproxypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

var lis *bufconn.Listener

func init() {
	lis = bufconn.Listen(bufSize)
	s := newProxyServer(lis)

	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
}

func bufDialer(context.Context, string) (net.Conn, error) {
	return lis.Dial()
}

func TestCreateClient(t *testing.T) {
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
