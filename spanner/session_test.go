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
	"bytes"
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

// TestSessionPoolConfigValidation tests session pool config validation.
func TestSessionPoolConfigValidation(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool configuration")
}

// TestSessionCreation tests session creation during sessionPool.Take().
func TestSessionCreation(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

// TestLIFOSessionOrder tests if session pool hand out sessions in LIFO order.
func TestLIFOSessionOrder(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

// TestTakeFromIdleList tests taking sessions from session pool's idle list.
func TestTakeFromIdleList(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

// TestTakeFromIdleListChecked tests taking sessions from session pool's idle
// list, but with a extra ping check.
func TestTakeFromIdleListChecked(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

// TestSessionLeak tests leaking a session and getting the stack of the
// goroutine that leaked it.
func TestSessionLeak(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

func TestSessionLeak_WhenInactiveTransactions_RemoveSessionsFromPool(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

func TestMaintainer_LongRunningTransactionsCleanup_IfClose_VerifyInactiveSessionsClosed(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

func TestLongRunningTransactionsCleanup_IfClose_VerifyInactiveSessionsClosed(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

func TestLongRunningTransactionsCleanup_IfLog_VerifyInactiveSessionsOpen(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

func TestLongRunningTransactionsCleanup_UtilisationBelowThreshold_VerifyInactiveSessionsOpen(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

func TestLongRunningTransactions_WhenAllExpectedlyLongRunning_VerifyInactiveSessionsOpen(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

func TestLongRunningTransactions_WhenDurationBelowThreshold_VerifyInactiveSessionsOpen(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

// TestMaxOpenedSessions tests max open sessions constraint.
func TestMaxOpenedSessions(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

// TestMinOpenedSessions tests min open session constraint.
func TestMinOpenedSessions(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

// TestPositiveNumInUseSessions tests that num_in_use session should always be greater than 0.
func TestPositiveNumInUseSessions(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

// TestMaxBurst tests max burst constraint.
func TestMaxBurst(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

// TestSessionRecycle tests recycling sessions.
func TestSessionRecycle(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

// TestSessionDestroy tests destroying sessions.
func TestSessionDestroy(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

// TestHcHeap tests heap operation on top of hcHeap.
func TestHcHeap(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

// TestHealthCheckScheduler tests if healthcheck workers can schedule and
// perform healthchecks properly.
func TestHealthCheckScheduler(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

// TestHealthCheck_FirstHealthCheck tests if the first healthcheck scheduling
// works properly.
func TestHealthCheck_FirstHealthCheck(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

// TestHealthCheck_NonFirstHealthCheck tests if the scheduling after the first
// health check works properly.
func TestHealthCheck_NonFirstHealthCheck(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

// TestSessionHealthCheck tests healthchecking cases.
func TestSessionHealthCheck(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

// TestStressSessionPool does stress test on session pool by the following concurrent operations:
//  1. Test worker gets a session from the pool.
//  2. Test worker turns a session back into the pool.
//  3. Test worker destroys a session got from the pool.
//  4. Healthcheck destroys a broken session (because a worker has already destroyed it).
//  5. Test worker closes the session pool.
//
// During the test, the session pool maintainer maintains the number of sessions,
// and it is expected that all sessions that are taken from session pool remains valid.
// When all test workers and healthcheck workers exit, mockclient, session pool
// and healthchecker should be in consistent state.
func TestStressSessionPool(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

// TestMaintainer checks the session pool maintainer maintains the number of
// sessions in the following cases:
//
//  1. On initialization of session pool, replenish session pool to meet
//     MinOpened or MaxIdle.
//  2. On increased session usage, provision extra MaxIdle sessions.
//  3. After the surge passes, scale down the session pool accordingly.
func TestMaintainer(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
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
			return errInvalidSessionPool
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
			return errInvalidSessionPool
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

// Tests that the session pool creates up to MinOpened connections.
//
// Historical context: This test also checks that a low
// healthCheckSampleInterval does not prevent it from opening connections.
// The low healthCheckSampleInterval will however sometimes cause session
// creations to time out. That should not be considered a problem, but it
// could cause the test case to fail if it happens too often.
// See: https://github.com/googleapis/google-cloud-go/issues/1259
func TestInit_CreatesSessions(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

func (s1 *session) Equal(s2 *session) bool {
	return s1.client == s2.client &&
		s1.id == s2.id &&
		s1.pool == s2.pool &&
		s1.createTime == s2.createTime &&
		s1.valid == s2.valid &&
		testEqual(s1.md, s2.md) &&
		bytes.Equal(s1.tx, s2.tx)
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

// Tests that maintainer only deletes sessions after a full maintenance window
// of 10 cycles has finished.
func TestMaintainer_DeletesSessions(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

func TestMaintenanceWindow_CycleAndUpdateMaxCheckedOut(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

func TestSessionCreationIsDistributedOverChannels(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

func TestSessionRecycleAfterPoolClose_NoDoubleDecrement(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}

func TestSessionRecycle_AlreadyInvalidSession(t *testing.T) {
	t.Skip("session pool has been removed - this test validates session pool behavior")
}
