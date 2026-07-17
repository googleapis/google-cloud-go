// Copyright 2026 Google LLC
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

package managedwriter

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	storagepb "cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// slowGetWriteStreamServer is a minimal fake BigQueryWrite server whose
// GetWriteStream blocks for a fixed delay. It lets us observe whether concurrent
// resolvePool calls run their GetWriteStream RPCs in parallel or serially.
type slowGetWriteStreamServer struct {
	storagepb.UnimplementedBigQueryWriteServer
	delay    time.Duration
	location string

	mu    sync.Mutex
	calls int
}

func (s *slowGetWriteStreamServer) GetWriteStream(ctx context.Context, req *storagepb.GetWriteStreamRequest) (*storagepb.WriteStream, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return &storagepb.WriteStream{Name: req.GetName(), Location: s.location}, nil
}

// startFakeWriteServer stands up the fake server on an in-process listener and
// returns a dialed gRPC connection plus a cleanup func.
func startFakeWriteServer(t *testing.T, srv storagepb.BigQueryWriteServer) (*grpc.ClientConn, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	gsrv := grpc.NewServer()
	storagepb.RegisterBigQueryWriteServer(gsrv, srv)
	go func() { _ = gsrv.Serve(lis) }()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		gsrv.Stop()
		t.Fatalf("dial: %v", err)
	}
	return conn, func() { conn.Close(); gsrv.Stop() }
}

// TestResolvePool_ConcurrentOpensAreNotSerialized is a regression test for the
// client-wide mutex previously held across the GetWriteStream RPC in
// resolvePool. With the lock held across the RPC, N concurrent opens serialize
// and take ~N*delay; with the RPC moved out of the lock they run in parallel and
// take ~delay. It also verifies the create-once-per-location invariant holds.
func TestResolvePool_ConcurrentOpensAreNotSerialized(t *testing.T) {
	const (
		delay   = 100 * time.Millisecond
		workers = 20
	)
	fake := &slowGetWriteStreamServer{delay: delay, location: "us"}
	conn, cleanup := startFakeWriteServer(t, fake)
	defer cleanup()

	ctx := context.Background()
	c, err := NewClient(ctx, "test-project",
		option.WithGRPCConn(conn),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	settings := &streamSettings{streamID: "projects/test-project/datasets/d/tables/t/streams/_default"}

	var wg sync.WaitGroup
	pools := make([]*connectionPool, workers)
	errs := make([]error, workers)
	start := time.Now()
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pools[i], errs[i] = c.resolvePool(ctx, settings, c.rawClient.AppendRows)
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	for i, err := range errs {
		if err != nil {
			t.Fatalf("resolvePool[%d]: %v", i, err)
		}
	}

	// Serialized behavior would be ~workers*delay (~2s here). Parallel behavior is
	// ~delay plus scheduling overhead. Assert well under the serialized bound.
	if elapsed >= time.Duration(workers)*delay/2 {
		t.Errorf("resolvePool serialized concurrent opens: %d workers took %v (delay=%v); "+
			"expected them to run in parallel (~%v), not serially (~%v)",
			workers, elapsed, delay, delay, time.Duration(workers)*delay)
	}

	// Create-once invariant: every caller must get the same single pool.
	for i := 1; i < workers; i++ {
		if pools[i] != pools[0] {
			t.Errorf("resolvePool created multiple pools for one location: pools[%d]=%p != pools[0]=%p",
				i, pools[i], pools[0])
		}
	}
	if got := len(c.pools); got != 1 {
		t.Errorf("expected exactly 1 cached pool, got %d", got)
	}
}
