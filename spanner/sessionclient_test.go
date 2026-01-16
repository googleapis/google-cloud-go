/*
Copyright 2019 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	vkit "cloud.google.com/go/spanner/apiv1"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

type testSessionCreateError struct {
	err error
	num int32
}

type testConsumer struct {
	numExpected int32

	mu       sync.Mutex
	sessions []*session
	errors   []*testSessionCreateError
	numErr   int32

	receivedAll chan struct{}
}

func (tc *testConsumer) sessionReady(_ context.Context, s *session) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.sessions = append(tc.sessions, s)
	tc.checkReceivedAll()
}

func (tc *testConsumer) sessionCreationFailed(_ context.Context, err error) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.errors = append(tc.errors, &testSessionCreateError{
		err: err,
		num: 1,
	})
	tc.numErr++
	tc.checkReceivedAll()
}

func (tc *testConsumer) checkReceivedAll() {
	if int32(len(tc.sessions))+tc.numErr == tc.numExpected {
		close(tc.receivedAll)
	}
}

func newTestConsumer(numExpected int32) *testConsumer {
	return &testConsumer{
		numExpected: numExpected,
		receivedAll: make(chan struct{}),
	}
}

func TestNextClient(t *testing.T) {
	t.Parallel()
	if useGRPCgcp {
		// For GCPMultiEndpoint the nextClient is indirectly tested via TestBatchCreateAndCloseSession.
		t.Skip("GCPMultiEndpoint does not provide a connection via Connection().")
	}

	n := 4
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		NumChannels:          n,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened: 0,
			MaxOpened: 100,
		},
	})
	defer teardown()
	sc := client.sm.sc
	connections := make(map[*grpc.ClientConn]int)
	for i := 0; i < n; i++ {
		client, err := sc.nextClient()
		if err != nil {
			t.Fatalf("Error getting a gapic client from the session client\nGot: %v", err)
		}
		conn1 := client.Connection()
		conn2 := client.Connection()
		if conn1 != conn2 {
			t.Fatalf("Client connection mismatch. Expected to get two equal connections.\nGot: %v and %v", conn1, conn2)
		}
		if index, ok := connections[conn1]; ok {
			t.Fatalf("Same connection used multiple times for different clients.\nClient 1: %v\nClient 2: %v", index, i)
		}
		connections[conn1] = i
	}
	// Pass through all the clients once more. This time the exact same
	// connections should be found.
	for i := 0; i < n; i++ {
		client, err := sc.nextClient()
		if err != nil {
			t.Fatalf("Error getting a gapic client from the session client\nGot: %v", err)
		}
		conn := client.Connection()
		if _, ok := connections[conn]; !ok {
			t.Fatalf("Connection not found for index %v", i)
		}
	}
}

func TestCreateAndCloseSession(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
	})
	defer teardown()

	s, err := client.sc.createSession(context.Background())
	if err != nil {
		t.Fatalf("batch.next() return error mismatch\ngot: %v\nwant: nil", err)
	}
	if s == nil {
		t.Fatalf("batch.next() return value mismatch\ngot: %v\nwant: any session", s)
	}
	if server.TestSpanner.TotalSessionsCreated() != uint(2) {
		t.Fatalf("number of sessions created mismatch\ngot: %v\nwant: %v", server.TestSpanner.TotalSessionsCreated(), 2)
	}
}

func TestCreateSessionWithDatabaseRole(t *testing.T) {
	// Make sure that there is always only one session in the sm.
	sc := SessionPoolConfig{
		MinOpened: 0,
		MaxOpened: 1,
	}
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, SessionPoolConfig: sc, DatabaseRole: "test"})
	defer teardown()
	ctx := context.Background()

	s, err := client.sc.createSession(ctx)
	if err != nil {
		t.Fatalf("batch.next() return error mismatch\ngot: %v\nwant: nil", err)
	}
	if s == nil {
		t.Fatalf("batch.next() return value mismatch\ngot: %v\nwant: any session", s)
	}

	if g, w := server.TestSpanner.TotalSessionsCreated(), uint(2); g != w {
		t.Fatalf("number of sessions created mismatch\ngot: %v\nwant: %v", g, w)
	}

	resp, err := server.TestSpanner.GetSession(ctx, &sppb.GetSessionRequest{Name: s.id})
	if err != nil {
		t.Fatalf("Failed to get session unexpectedly: %v", err)
	}
	if g, w := resp.CreatorRole, "test"; g != w {
		t.Fatalf("database role mismatch.\nGot: %v\nWant: %v", g, w)
	}
}

func TestClientIDGenerator(t *testing.T) {
	cidGen = newClientIDGenerator()
	for _, tt := range []struct {
		database string
		clientID string
	}{
		{"db", "client-1"},
		{"db-new", "client-1"},
		{"db", "client-2"},
	} {
		if got, want := cidGen.nextID(tt.database), tt.clientID; got != want {
			t.Fatalf("Generate wrong client ID: got %v, want %v", got, want)
		}
	}
}

func TestMergeCallOptions(t *testing.T) {
	a := &vkit.CallOptions{
		CreateSession: []gax.CallOption{
			gax.WithRetry(func() gax.Retryer {
				return gax.OnCodes([]codes.Code{
					codes.Unavailable, codes.DeadlineExceeded,
				}, gax.Backoff{
					Initial:    100 * time.Millisecond,
					Max:        16000 * time.Millisecond,
					Multiplier: 1.0,
				})
			}),
		},
		GetSession: []gax.CallOption{
			gax.WithRetry(func() gax.Retryer {
				return gax.OnCodes([]codes.Code{
					codes.Unavailable, codes.DeadlineExceeded,
				}, gax.Backoff{
					Initial:    250 * time.Millisecond,
					Max:        32000 * time.Millisecond,
					Multiplier: 1.30,
				})
			}),
		},
	}
	b := &vkit.CallOptions{
		CreateSession: []gax.CallOption{
			gax.WithRetry(func() gax.Retryer {
				return gax.OnCodes([]codes.Code{
					codes.Unavailable,
				}, gax.Backoff{
					Initial:    250 * time.Millisecond,
					Max:        32000 * time.Millisecond,
					Multiplier: 1.30,
				})
			}),
		},
		BatchCreateSessions: []gax.CallOption{
			gax.WithRetry(func() gax.Retryer {
				return gax.OnCodes([]codes.Code{
					codes.Unavailable,
				}, gax.Backoff{
					Initial:    250 * time.Millisecond,
					Max:        32000 * time.Millisecond,
					Multiplier: 1.30,
				})
			}),
		}}

	merged := mergeCallOptions(b, a)
	cs := &gax.CallSettings{}
	// We can't access the fields of Retryer so we have test the result by
	// comparing strings.
	merged.CreateSession[0].Resolve(cs)
	if got, want := fmt.Sprintf("%v", cs.Retry()), "&{{250000000 32000000000 1.3 0} [14]}"; got != want {
		t.Fatalf("merged CallOptions is incorrect: got %v, want %v", got, want)
	}

	merged.CreateSession[1].Resolve(cs)
	if got, want := fmt.Sprintf("%v", cs.Retry()), "&{{100000000 16000000000 1 0} [14 4]}"; got != want {
		t.Fatalf("merged CallOptions is incorrect: got %v, want %v", got, want)
	}

	merged.GetSession[0].Resolve(cs)
	if got, want := fmt.Sprintf("%v", cs.Retry()), "&{{250000000 32000000000 1.3 0} [14 4]}"; got != want {
		t.Fatalf("merged CallOptions is incorrect: got %v, want %v", got, want)
	}

	merged.BatchCreateSessions[0].Resolve(cs)
	if got, want := fmt.Sprintf("%v", cs.Retry()), "&{{250000000 32000000000 1.3 0} [14]}"; got != want {
		t.Fatalf("merged CallOptions is incorrect: got %v, want %v", got, want)
	}
}
