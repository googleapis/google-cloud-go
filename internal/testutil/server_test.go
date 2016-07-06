package testutil

import (
	"testing"

	grpc "google.golang.org/grpc"
)

func TestNewServer(t *testing.T) {
	srv, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	srv.Start()
	conn, err := grpc.Dial(srv.Addr, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	srv.Close()
}
