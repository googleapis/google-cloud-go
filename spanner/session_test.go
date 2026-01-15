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
	sp := client.idleSessions
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
