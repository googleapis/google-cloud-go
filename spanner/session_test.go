/*
Copyright 2017 Google LLC

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
	"testing"
	"time"

	. "cloud.google.com/go/spanner/internal/testutil"
	"github.com/googleapis/gax-go/v2/apierror"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newSessionNotFoundError(name string) error {
	s := status.Newf(codes.NotFound, "Session not found: Session with id %s not found", name)
	s, _ = s.WithDetails(&errdetails.ResourceInfo{ResourceType: sessionResourceType, ResourceName: name})
	err, _ := apierror.FromError(s.Err())
	return err
}

func TestMultiplexSessionWorker(t *testing.T) {
	t.Parallel()
	if !isMultiplexEnabled {
		t.Skip("Multiplexing is not enabled")
	}
	ctx := context.Background()

	server, client, teardown := setupMockedTestServerWithConfig(t,
		ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig: SessionPoolConfig{
				MultiplexSessionCheckInterval: time.Millisecond,
			},
		})
	defer teardown()
	_, err := client.Single().ReadRow(ctx, "Albums", Key{"foo"}, []string{"SingerId", "AlbumId", "AlbumTitle"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sp := client.sm
	waitFor(t, func() error {
		sp.mu.Lock()
		defer sp.mu.Unlock()
		if sp.multiplexedSession == nil {
			return errInvalidSession
		}
		return nil
	})
	if !testEqual(uint(1), server.TestSpanner.TotalSessionsCreated()) {
		t.Fatalf("expected 1 session to be created, got %v", server.TestSpanner.TotalSessionsCreated())
	}
	// Will cause session creation RPC to be fail.
	server.TestSpanner.PutExecutionTime(MethodCreateSession,
		SimulatedExecutionTime{
			Errors:    []error{status.Errorf(codes.PermissionDenied, "try later")},
			KeepError: true,
		})
	// To save test time, update the multiplex session creation time to trigger refresh.
	sp.mu.Lock()
	oldMultiplexedSession := sp.multiplexedSession.id
	sp.multiplexedSession.createTime = sp.multiplexedSession.createTime.Add(-10 * 24 * time.Hour)
	sp.mu.Unlock()

	// Subsequent read should use existing session.
	_, err = client.Single().ReadRow(ctx, "Albums", Key{"foo"}, []string{"SingerId", "AlbumId", "AlbumTitle"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// To save test time, update the multiplex session creation time to trigger refresh.
	sp.mu.Lock()
	multiplexSessionID := sp.multiplexedSession.id
	sp.mu.Unlock()
	if !testEqual(oldMultiplexedSession, multiplexSessionID) {
		t.Errorf("TestMultiplexSessionWorker expected multiplexed session id to be=%v, got: %v", oldMultiplexedSession, multiplexSessionID)
	}

	// Let the first session request succeed.
	server.TestSpanner.Freeze()
	server.TestSpanner.PutExecutionTime(MethodCreateSession, SimulatedExecutionTime{})
	server.TestSpanner.Unfreeze()

	waitFor(t, func() error {
		if server.TestSpanner.TotalSessionsCreated() != 2 {
			return errInvalidSession
		}
		return nil
	})

	sp.mu.Lock()
	multiplexSessionID = sp.multiplexedSession.id
	sp.mu.Unlock()

	if testEqual(oldMultiplexedSession, multiplexSessionID) {
		t.Errorf("TestMultiplexSessionWorker expected multiplexed session id to be different, got: %v", multiplexSessionID)
	}
}

func TestMultiplexedSessionCreationGoroutineDeadlockOnContextCancel(t *testing.T) {
	t.Parallel()
	if !isMultiplexEnabled {
		t.Skip("Multiplexing is not enabled")
	}

	server, opts, serverTeardown := NewMockedSpannerInMemTestServer(t)
	defer serverTeardown()
	server.TestSpanner.PutExecutionTime(MethodCreateSession,
		SimulatedExecutionTime{MinimumExecutionTime: 500 * time.Millisecond})

	ctx := context.Background()
	db := fmt.Sprintf("projects/%s/instances/%s/databases/%s", "[PROJECT]", "[INSTANCE]", "[DATABASE]")
	client, err := NewClientWithConfig(ctx, db, ClientConfig{
		DisableNativeMetrics: true,
	}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()
	cancelledDone := make(chan error, 1)
	go func() {
		_, err := client.Single().ReadRow(cancelledCtx, "Albums", Key{"foo"}, []string{"SingerId", "AlbumId", "AlbumTitle"})
		cancelledDone <- err
	}()

	done := make(chan error, 1)
	go func() {
		readCtx, readCancel := context.WithTimeout(ctx, 15*time.Second)
		defer readCancel()
		_, err := client.Single().ReadRow(readCtx, "Albums", Key{"foo"}, []string{"SingerId", "AlbumId", "AlbumTitle"})
		done <- err
	}()

	select {
	case err := <-cancelledDone:
		if g, w := ErrCode(err), codes.Canceled; g != w {
			t.Fatalf("cancelled ReadRow returned code %v, want %v (err=%v)", g, w, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled ReadRow did not return while initial multiplexed session creation was in flight")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ReadRow returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ReadRow deadlocked waiting for multiplexed session readiness after another waiter canceled")
	}
}

func TestMultiplexedSessionCreationCloseReleasesWaiters(t *testing.T) {
	t.Parallel()

	// Create the server and add a delay to CreateSession so the initial
	// multiplexed session creation takes long enough for us to queue
	// requests behind it.
	server, opts, serverTeardown := NewMockedSpannerInMemTestServer(t)
	defer serverTeardown()
	server.TestSpanner.PutExecutionTime(MethodCreateSession,
		SimulatedExecutionTime{MinimumExecutionTime: 500 * time.Millisecond})

	ctx := context.Background()
	db := fmt.Sprintf("projects/%s/instances/%s/databases/%s", "[PROJECT]", "[INSTANCE]", "[DATABASE]")
	client, err := NewClientWithConfig(ctx, db, ClientConfig{
		DisableNativeMetrics: true,
	}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Issue a read with an already-cancelled context. The call reaches
	// takeMultiplexed, sees multiplexedSession==nil (the initial creation is
	// still in flight), and sends a non-force request on multiplexedSessionReq.
	// That send blocks until the goroutine finishes the initial force request
	// (~500ms), regardless of the cancelled context.
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()
	go client.Single().ReadRow(cancelledCtx, "Albums", Key{"foo"}, []string{"SingerId", "AlbumId", "AlbumTitle"})

	// Wait briefly so the cancelled ReadRow blocks on the channel send
	// first (FIFO). This ensures the goroutine receives it before the
	// valid ReadRow below.
	time.Sleep(50 * time.Millisecond)

	// Issue a second read with a valid context. This also blocks at the
	// channel send behind the cancelled request. If the goroutine dies
	// after processing the cancelled request, this read is stuck forever.
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// The goroutine may panic with "send on closed channel"
				// when client.Close() closes multiplexedSessionReq while
				// this goroutine is still blocked on the send.
				done <- fmt.Errorf("panic: %v", r)
			}
		}()
		readCtx, readCancel := context.WithTimeout(ctx, 15*time.Second)
		defer readCancel()
		_, err := client.Single().ReadRow(readCtx, "Albums", Key{"foo"}, []string{"SingerId", "AlbumId", "AlbumTitle"})
		done <- err
	}()

	// Remove the delay after the initial session is created so the second
	// read's session creation (if needed) is not slowed down.
	waitFor(t, func() error {
		client.sm.mu.Lock()
		defer client.sm.mu.Unlock()
		if client.sm.multiplexedSession == nil {
			return errInvalidSession
		}
		return nil
	})
	server.TestSpanner.Freeze()
	server.TestSpanner.PutExecutionTime(MethodCreateSession, SimulatedExecutionTime{})
	server.TestSpanner.Unfreeze()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ReadRow returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ReadRow deadlocked: the createMultiplexedSession goroutine " +
			"exited after processing a request with a cancelled context, " +
			"leaving a concurrent reader permanently blocked on the " +
			"multiplexedSessionReq channel send")
	}
}

func waitFor(t *testing.T, assert func() error) {
	t.Helper()
	timeout := 15 * time.Second
	ta := time.After(timeout)

	for {
		select {
		case <-ta:
			if err := assert(); err != nil {
				t.Fatalf("after %v waiting, got %v", timeout, err)
			}
			return
		default:
		}

		if err := assert(); err != nil {
			// Fail. Let's pause and retry.
			time.Sleep(10 * time.Millisecond)
			continue
		}

		return
	}
}
