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
	"container/heap"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	. "cloud.google.com/go/spanner/internal/testutil"
	"github.com/googleapis/gax-go/v2/apierror"
	"google.golang.org/api/iterator"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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
	t.Parallel()
	_, client, teardown := setupMockedTestServer(t)
	defer teardown()

	for _, test := range []struct {
		spc SessionPoolConfig
		err error
	}{
		{
			SessionPoolConfig{
				MinOpened: 10,
				MaxOpened: 5,
			},
			errMinOpenedGTMaxOpened(5, 10),
		},
		{
			SessionPoolConfig{
				WriteSessions: -0.1,
			},
			nil,
		},
		{
			SessionPoolConfig{
				WriteSessions: 2.0,
			},
			nil,
		},
		{
			SessionPoolConfig{
				HealthCheckWorkers: -1,
			},
			errHealthCheckWorkersNegative(-1),
		},
		{
			SessionPoolConfig{
				HealthCheckInterval: -time.Second,
			},
			errHealthCheckIntervalNegative(-time.Second),
		},
	} {
		if _, err := newSessionPool(client.sc, test.spc); !testEqual(err, test.err) {
			t.Fatalf("want %v, got %v", test.err, err)
		}
	}
}

// TestSessionCreation tests session creation during sessionPool.Take().
func TestSessionCreation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, client, teardown := setupMockedTestServer(t)
	defer teardown()
	sp := client.idleSessions

	// Take three sessions from session pool, this should trigger session pool
	// to create SessionPoolConfig.incStep new sessions.
	shs := make([]*sessionHandle, 3)
	for i := 0; i < len(shs); i++ {
		var err error
		shs[i], err = sp.take(ctx)
		if err != nil {
			t.Fatalf("failed to get session(%v): %v", i, err)
		}
	}
	// Wait until session creation has seized.
	timeout := time.After(4 * time.Second)
	var numBeingCreated uint64
loop:
	for {
		sp.mu.Lock()
		numBeingCreated = sp.createReqs
		sp.mu.Unlock()
		select {
		case <-timeout:
			t.Fatalf("timed out, still %d session(s) being created, want %d", numBeingCreated, 0)
		default:
			if numBeingCreated == 0 {
				break loop
			}
		}
	}
	md := metadata.Pairs(resourcePrefixHeader, "projects/p/instances/i/databases/d")
	ctx = metadata.NewOutgoingContext(ctx, md)
	for _, sh := range shs {
		if _, err := sh.getClient().GetSession(ctx, &sppb.GetSessionRequest{
			Name: sh.getID(),
		}); err != nil {
			t.Fatalf("error getting expected session from server: %v", err)
		}
	}
	// Verify that created sessions are recorded correctly in session pool.
	sp.mu.Lock()
	if sp.numOpened != sp.incStep {
		t.Fatalf("session pool reports %v open sessions, want %v", sp.numOpened, sp.incStep)
	}
	if sp.createReqs != 0 {
		t.Fatalf("session pool reports %v session create requests, want 0", int(sp.createReqs))
	}
	sp.mu.Unlock()
	// Verify that created sessions are tracked correctly by healthcheck queue.
	hc := sp.hc
	hc.mu.Lock()
	if uint64(hc.queue.Len()) != sp.incStep {
		t.Fatalf("healthcheck queue length = %v, want %v", hc.queue.Len(), sp.incStep)
	}
	hc.mu.Unlock()
}

// TestLIFOSessionOrder tests if session pool hand out sessions in LIFO order.
func TestLIFOSessionOrder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, client, teardown := setupMockedTestServerWithConfig(t,
		ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig: SessionPoolConfig{
				MaxOpened: 3,
				MinOpened: 3,
			},
		})
	defer teardown()
	sp := client.idleSessions
	// Create/take three sessions and recycle them.
	shs, shsIDs := make([]*sessionHandle, 3), make([]string, 3)
	for i := 0; i < len(shs); i++ {
		var err error
		if shs[i], err = sp.take(ctx); err != nil {
			t.Fatalf("failed to take session(%v): %v", i, err)
		}
		shsIDs[i] = shs[i].getID()
	}
	for i := 0; i < len(shs); i++ {
		shs[i].recycle()
	}
	for i := 2; i >= 0; i-- {
		sh, err := sp.take(ctx)
		if err != nil {
			t.Fatalf("cannot take session from session pool: %v", err)
		}
		// check, if sessions returned in LIFO order.
		if wantID, gotID := shsIDs[i], sh.getID(); wantID != gotID {
			t.Fatalf("got session with id: %v, want: %v", gotID, wantID)
		}
	}
}

// TestTakeFromIdleList tests taking sessions from session pool's idle list.
func TestTakeFromIdleList(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Make sure maintainer keeps the idle sessions.
	server, client, teardown := setupMockedTestServerWithConfig(t,
		ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig: SessionPoolConfig{
				MaxIdle:                   10,
				MaxOpened:                 10,
				healthCheckSampleInterval: 10 * time.Millisecond,
			},
		})
	defer teardown()
	sp := client.idleSessions

	// Take ten sessions from session pool and recycle them.
	shs := make([]*sessionHandle, 10)
	for i := 0; i < len(shs); i++ {
		var err error
		shs[i], err = sp.take(ctx)
		if err != nil {
			t.Fatalf("failed to get session(%v): %v", i, err)
		}
	}
	// Make sure it's sampled once before recycling, otherwise it will be
	// cleaned up.
	<-time.After(sp.SessionPoolConfig.healthCheckSampleInterval)
	for i := 0; i < len(shs); i++ {
		shs[i].recycle()
	}
	// Further session requests from session pool won't cause mockclient to
	// create more sessions.
	wantSessions := server.TestSpanner.DumpSessions()
	// Take ten sessions from session pool again, this time all sessions should
	// come from idle list.
	gotSessions := map[string]bool{}
	for i := 0; i < len(shs); i++ {
		sh, err := sp.take(ctx)
		if err != nil {
			t.Fatalf("cannot take session from session pool: %v", err)
		}
		gotSessions[sh.getID()] = true
	}
	if len(gotSessions) != 10 {
		t.Fatalf("got %v unique sessions, want 10", len(gotSessions))
	}
	if sp.multiplexedSession != nil {
		gotSessions[sp.multiplexedSession.getID()] = true
	}
	if !testEqual(gotSessions, wantSessions) {
		t.Fatalf("got sessions: %v, want %v", gotSessions, wantSessions)
	}
}

// TestTakeFromIdleListChecked tests taking sessions from session pool's idle
// list, but with a extra ping check.
func TestTakeFromIdleListChecked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Make sure maintainer keeps the idle sessions.
	server, client, teardown := setupMockedTestServerWithConfig(t,
		ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig: SessionPoolConfig{
				WriteSessions:             0.0,
				MaxIdle:                   1,
				HealthCheckInterval:       50 * time.Millisecond,
				healthCheckSampleInterval: 10 * time.Millisecond,
			},
		})
	defer teardown()
	sp := client.idleSessions

	// Stop healthcheck workers to simulate slow pings.
	sp.hc.close()

	// Create a session and recycle it.
	sh, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	// Wait until all session creation has finished.
	waitFor(t, func() error {
		sp.mu.Lock()
		// WriteSessions = 0, so we only have to check for read sessions.
		numOpened := uint64(sp.idleList.Len())
		sp.mu.Unlock()
		if numOpened < sp.SessionPoolConfig.incStep-1 {
			return errors.New("creation not yet finished")
		}
		return nil
	})

	// Force ping during the first take() by setting check time to the past.
	sp.hc.mu.Lock()
	sh.session.nextCheck = time.Now().Add(-time.Minute)
	sp.hc.mu.Unlock()
	wantSid := sh.getID()
	sh.recycle()

	// Two back-to-back session requests, both of them should return the same
	// session created before, but only the first of them should trigger a session ping.
	for i := 0; i < 2; i++ {
		// Take the session from the idle list and recycle it.
		sh, err = sp.take(ctx)
		if err != nil {
			t.Fatalf("%v - failed to get session: %v", i, err)
		}
		if gotSid := sh.getID(); gotSid != wantSid {
			t.Fatalf("%v - got session id: %v, want %v", i, gotSid, wantSid)
		}

		// The two back-to-back session requests shouldn't trigger any session
		// pings because sessionPool.Take reschedules the next healthcheck.
		if got, want := server.TestSpanner.DumpPings(), ([]string{wantSid}); !testEqual(got, want) {
			t.Fatalf("%v - got ping session requests: %v, want %v", i, got, want)
		}
		sh.recycle()
	}

	// Inject session error to server stub, and take the session from the
	// session pool, the old session should be destroyed and the session pool
	// will create a new session.
	server.TestSpanner.PutExecutionTime(MethodExecuteSql,
		SimulatedExecutionTime{
			Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")},
		})

	// Force ping by setting check time in the past.
	s := sp.idleList.Front().Value.(*session)
	s.nextCheck = time.Now().Add(-time.Minute)

	// take will take the idle session. Then it will send a GetSession request
	// to check if it's healthy. It'll discover that it's not healthy
	// (NotFound) and drop it. No new session will be created as MinOpened=0.
	sh, err = sp.take(ctx)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if sh.getID() == wantSid {
		t.Fatalf("sessionPool.Take still returns the same session %v, want it to create a new one", wantSid)
	}
}

// TestSessionLeak tests leaking a session and getting the stack of the
// goroutine that leaked it.
func TestSessionLeak(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			TrackSessionHandles: true,
			MinOpened:           0,
			MaxOpened:           1,
		},
	})
	defer teardown()

	// Execute a query without calling rowIterator.Stop. This will cause the
	// session not to be returned to the pool.
	single := client.Single()
	iter := single.Query(ctx, NewStatement(SelectFooFromBar))
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatalf("Got unexpected error while iterating results: %v\n", err)
		}
	}
	// The session should not have been returned to the pool.
	if g, w := client.idleSessions.idleList.Len(), 0; g != w {
		t.Fatalf("Idle sessions count mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	// The checked out session should contain a stack trace.
	if single.sh.stack == nil && !isMultiplexEnabled {
		t.Fatalf("Missing stacktrace from session handle")
	}
	stack := string(single.sh.stack)
	testMethod := "TestSessionLeak"
	if !strings.Contains(stack, testMethod) && !isMultiplexEnabled {
		t.Fatalf("Stacktrace does not contain '%s'\nGot: %s", testMethod, stack)
	}
	// Return the session to the pool.
	iter.Stop()
	// The stack should now have been removed from the session handle.
	if single.sh.stack != nil {
		t.Fatalf("Got unexpected stacktrace in session handle: %s", single.sh.stack)
	}

	// Do another query and hold on to the session.
	single = client.Single()
	iter = single.Query(ctx, NewStatement(SelectFooFromBar))
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatalf("Got unexpected error while iterating results: %v\n", err)
		}
	}
	// Try to do another query. This will fail as MaxOpened=1.
	ctxWithTimeout, cancel := context.WithTimeout(ctx, time.Millisecond*10)
	defer cancel()
	single2 := client.Single()
	iter2 := single2.Query(ctxWithTimeout, NewStatement(SelectFooFromBar))
	_, gotErr := iter2.Next()
	wantErr := client.idleSessions.errGetSessionTimeoutWithTrackedSessionHandles(codes.DeadlineExceeded)
	if isMultiplexEnabled {
		wantErr = nil
	}
	// The error should contain the stacktraces of all the checked out
	// sessions.
	if !testEqual(gotErr, wantErr) {
		t.Fatalf("Error mismatch on iterating result set.\nGot: %v\nWant: %v\n", gotErr, wantErr)
	}
	if wantErr != nil {
		if !strings.Contains(gotErr.Error(), testMethod) {
			t.Fatalf("Error does not contain '%s'\nGot: %s", testMethod, gotErr.Error())
		}
	}
	// Close iterators to check sessions back into the pool before closing.
	iter2.Stop()
	iter.Stop()
}

func TestSessionLeak_WhenInactiveTransactions_RemoveSessionsFromPool(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := log.Default()
	logger.SetOutput(io.Discard)
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened: 0,
			MaxOpened: 1,
			InactiveTransactionRemovalOptions: InactiveTransactionRemovalOptions{
				ActionOnInactiveTransaction: WarnAndClose,
			},
			TrackSessionHandles: true,
		},
		Logger: logger,
	})
	defer teardown()

	// Execute a query without calling rowIterator.Stop. This will cause the
	// session not to be returned to the pool.
	single := client.Single()
	iter := single.Query(ctx, NewStatement(SelectFooFromBar))
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatalf("Got unexpected error while iterating results: %v\n", err)
		}
	}
	// The session should not have been returned to the pool.
	p := client.idleSessions
	p.mu.Lock()
	if g, w := p.idleList.Len(), 0; g != w { // No of sessions in the pool must be 0
		p.mu.Unlock()
		t.Fatalf("Idle sessions count mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	p.mu.Unlock()
	// The checked out session should contain a stack trace as Logging is true.
	single.sh.mu.Lock()
	if single.sh.stack == nil {
		if !isMultiplexEnabled {
			single.sh.mu.Unlock()
			t.Fatalf("Missing stacktrace from session handle")
		}
	}
	if g, w := single.sh.eligibleForLongRunning, false; g != w {
		single.sh.mu.Unlock()
		t.Fatalf("isLongRunningTransaction mismatch\nGot: %v\nWant: %v\n", g, w)
	}

	// Mock the session lastUseTime to be greater than 60 mins
	single.sh.lastUseTime = time.Now().Add(-time.Hour)
	single.sh.mu.Unlock()

	// force run task to clean up unexpected long-running sessions
	p.removeLongRunningSessions()

	// The session should have been removed from pool.
	p.mu.Lock()
	if g, w := p.idleList.Len(), 0; g != w {
		t.Fatalf("Idle Sessions in pool, count mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	if g, w := p.numInUse, uint64(0); g != w {
		t.Fatalf("Number of sessions currently in use mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	if g, w := p.numOpened, uint64(0); g != w {
		t.Fatalf("Session pool size mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	expectedLeakedSession := uint64(1)
	if isMultiplexEnabled {
		expectedLeakedSession = 0
	}
	if g, w := p.numOfLeakedSessionsRemoved, expectedLeakedSession; g != w {
		t.Fatalf("Number of leaked sessions removed mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	p.mu.Unlock()
	iter.Stop()
}

func TestMaintainer_LongRunningTransactionsCleanup_IfClose_VerifyInactiveSessionsClosed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := log.Default()
	logger.SetOutput(io.Discard)
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:                 1,
			MaxOpened:                 3,
			healthCheckSampleInterval: 10 * time.Millisecond, // maintainer runs every 10ms
			InactiveTransactionRemovalOptions: InactiveTransactionRemovalOptions{
				ActionOnInactiveTransaction: WarnAndClose,
				executionFrequency:          15 * time.Millisecond, // check long-running sessions every 20ms
			},
		},
		Logger: logger,
	})
	defer teardown()
	sp := client.idleSessions

	// get session-1 from pool
	s1, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get the session: %v", err)
	}
	// get session-2 from pool
	s2, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get the session: %v", err)
	}
	// get session-3 from pool
	s3, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get the session: %v", err)
	}
	sp.mu.Lock()
	if g, w := sp.numOpened, uint64(3); g != w {
		sp.mu.Unlock()
		t.Fatalf("No of sessions opened mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	if g, w := sp.numInUse, uint64(3); g != w {
		sp.mu.Unlock()
		t.Fatalf("No of sessions in use mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	sp.mu.Unlock()
	s1.mu.Lock()
	s1.eligibleForLongRunning = false
	s1.lastUseTime = time.Now().Add(-time.Hour)
	s1.mu.Unlock()

	s2.mu.Lock()
	s2.eligibleForLongRunning = false
	s2.lastUseTime = time.Now().Add(-time.Hour)
	s2.mu.Unlock()

	s3.mu.Lock()
	s3.eligibleForLongRunning = true
	s3.lastUseTime = time.Now().Add(-time.Hour)
	s3.mu.Unlock()

	// Sleep for maintainer to run long-running cleanup task
	time.Sleep(30 * time.Millisecond)
	// force run task to clean up unexpected long-running sessions
	sp.removeLongRunningSessions()

	sp.mu.Lock()
	defer sp.mu.Unlock()
	if g, w := sp.numOfLeakedSessionsRemoved, uint64(2); g != w {
		t.Fatalf("No of leaked sessions removed mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	if g, w := sp.numOpened, uint64(1); g != w {
		t.Fatalf("Session pool size mismatch\nGot: %d\nWant: %d\n", g, w)
	}
}

func TestLongRunningTransactionsCleanup_IfClose_VerifyInactiveSessionsClosed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := log.Default()
	logger.SetOutput(io.Discard)
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened: 1,
			MaxOpened: 3,
			InactiveTransactionRemovalOptions: InactiveTransactionRemovalOptions{
				ActionOnInactiveTransaction: WarnAndClose,
			},
		},
		Logger: logger,
	})
	defer teardown()
	sp := client.idleSessions

	// get session-1 from pool
	s1, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get the session: %v", err)
	}
	// get session-2 from pool
	s2, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get the session: %v", err)
	}
	// get session-3 from pool
	s3, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get the session: %v", err)
	}
	sp.mu.Lock()
	if g, w := sp.numOpened, uint64(3); g != w {
		sp.mu.Unlock()
		t.Fatalf("No of sessions opened mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	if g, w := sp.numInUse, uint64(3); g != w {
		sp.mu.Unlock()
		t.Fatalf("No of sessions in use mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	sp.mu.Unlock()
	s1.mu.Lock()
	s1.eligibleForLongRunning = false
	s1.lastUseTime = time.Now().Add(-time.Hour)
	s1.mu.Unlock()

	s2.mu.Lock()
	s2.eligibleForLongRunning = false
	s2.lastUseTime = time.Now().Add(-time.Hour)
	s2.mu.Unlock()

	s3.mu.Lock()
	s3.eligibleForLongRunning = true
	s3.lastUseTime = time.Now().Add(-time.Hour)
	s3.mu.Unlock()

	// force run task to clean up unexpected long-running sessions
	sp.removeLongRunningSessions()

	sp.mu.Lock()
	defer sp.mu.Unlock()
	if g, w := sp.numOfLeakedSessionsRemoved, uint64(2); g != w {
		t.Fatalf("No of leaked sessions removed mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	if g, w := sp.numOpened, uint64(1); g != w {
		t.Fatalf("Session pool size mismatch\nGot: %d\nWant: %d\n", g, w)
	}
}

func TestLongRunningTransactionsCleanup_IfLog_VerifyInactiveSessionsOpen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := log.Default()
	logger.SetOutput(io.Discard)
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened: 1,
			MaxOpened: 3,
			InactiveTransactionRemovalOptions: InactiveTransactionRemovalOptions{
				ActionOnInactiveTransaction: Warn,
			},
		},
		Logger: logger,
	})
	defer teardown()
	sp := client.idleSessions

	// get session-1 from pool
	s1, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get the session: %v", err)
	}
	// get session-2 from pool
	s2, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get the session: %v", err)
	}
	// get session-3 from pool
	s3, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get the session: %v", err)
	}
	sp.mu.Lock()
	if g, w := sp.numInUse, uint64(3); g != w {
		sp.mu.Unlock()
		t.Fatalf("Number of sessions currently in use mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	if g, w := sp.numOpened, uint64(3); g != w {
		sp.mu.Unlock()
		t.Fatalf("Session pool size mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	sp.mu.Unlock()
	s1.mu.Lock()
	s1.eligibleForLongRunning = false
	s1.lastUseTime = time.Now().Add(-time.Hour)
	s1.mu.Unlock()

	s2.mu.Lock()
	s2.eligibleForLongRunning = false
	s2.lastUseTime = time.Now().Add(-time.Hour)
	s2.mu.Unlock()

	s3.mu.Lock()
	s3.eligibleForLongRunning = true
	s3.lastUseTime = time.Now().Add(-time.Hour)
	s3.mu.Unlock()

	// force run task to clean up unexpected long-running sessions
	sp.removeLongRunningSessions()

	s1.mu.Lock()
	if !s1.isSessionLeakLogged {
		t.Fatalf("Expect session leak logged for session %v", s1.session.id)
	}
	s1.mu.Unlock()

	s2.mu.Lock()
	if !s2.isSessionLeakLogged {
		t.Fatalf("Expect session leak logged for session %v", s2.session.id)
	}
	s2.mu.Unlock()

	s3.mu.Lock()
	if s3.isSessionLeakLogged {
		t.Fatalf("Incorrect session leak log as transaction is long running for session: %v", s3.session.id)
	}
	s3.mu.Unlock()

	sp.mu.Lock()
	defer sp.mu.Unlock()
	if g, w := sp.numOfLeakedSessionsRemoved, uint64(0); g != w {
		t.Fatalf("No of leaked sessions removed mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	if g, w := sp.numOpened, uint64(3); g != w {
		t.Fatalf("Session pool size mismatch\nGot: %d\nWant: %d\n", g, w)
	}
}

func TestLongRunningTransactionsCleanup_UtilisationBelowThreshold_VerifyInactiveSessionsOpen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened: 1,
			MaxOpened: 3,
			incStep:   1,
			InactiveTransactionRemovalOptions: InactiveTransactionRemovalOptions{
				ActionOnInactiveTransaction: WarnAndClose,
			},
		},
	})
	defer teardown()
	sp := client.idleSessions

	// get session-1 from pool
	s1, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get the session: %v", err)
	}
	// get session-2 from pool
	s2, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get the session: %v", err)
	}
	sp.mu.Lock()
	if g, w := sp.numInUse, uint64(2); g != w {
		sp.mu.Unlock()
		t.Fatalf("Number of sessions currently in use mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	if g, w := sp.numOpened, uint64(2); g != w {
		sp.mu.Unlock()
		t.Fatalf("Session pool size mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	sp.mu.Unlock()
	s1.mu.Lock()
	s1.eligibleForLongRunning = false
	s1.lastUseTime = time.Now().Add(-time.Hour)
	s1.mu.Unlock()

	s2.mu.Lock()
	s2.eligibleForLongRunning = false
	s2.lastUseTime = time.Now().Add(-time.Hour)
	s2.mu.Unlock()

	// force run task to clean up unexpected long-running sessions
	sp.removeLongRunningSessions()

	sp.mu.Lock()
	defer sp.mu.Unlock()
	// 2/3 sessions are used. Hence utilisation < 95%.
	if g, w := sp.numOfLeakedSessionsRemoved, uint64(0); g != w {
		t.Fatalf("No of leaked sessions removed mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	if g, w := sp.numOpened, uint64(2); g != w {
		t.Fatalf("Session pool size mismatch\nGot: %d\nWant: %d\n", g, w)
	}
}

func TestLongRunningTransactions_WhenAllExpectedlyLongRunning_VerifyInactiveSessionsOpen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened: 1,
			MaxOpened: 3,
			InactiveTransactionRemovalOptions: InactiveTransactionRemovalOptions{
				ActionOnInactiveTransaction: Warn,
			},
		},
	})
	defer teardown()
	sp := client.idleSessions

	// get session-1 from pool
	s1, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get the session: %v", err)
	}
	// get session-2 from pool
	s2, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get the session: %v", err)
	}
	// get session-3 from pool
	s3, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get the session: %v", err)
	}
	sp.mu.Lock()
	if g, w := sp.numInUse, uint64(3); g != w {
		sp.mu.Unlock()
		t.Fatalf("Number of sessions currently in use mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	if g, w := sp.numOpened, uint64(3); g != w {
		sp.mu.Unlock()
		t.Fatalf("Session pool size mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	sp.mu.Unlock()
	s1.mu.Lock()
	s1.eligibleForLongRunning = true
	s1.lastUseTime = time.Now().Add(-time.Hour)
	s1.mu.Unlock()

	s2.mu.Lock()
	s2.eligibleForLongRunning = true
	s2.lastUseTime = time.Now().Add(-time.Hour)
	s2.mu.Unlock()

	s3.mu.Lock()
	s3.eligibleForLongRunning = true
	s3.lastUseTime = time.Now().Add(-time.Hour)
	s3.mu.Unlock()

	// force run task to clean up unexpected long-running sessions
	sp.removeLongRunningSessions()

	sp.mu.Lock()
	defer sp.mu.Unlock()
	if g, w := sp.numOfLeakedSessionsRemoved, uint64(0); g != w {
		t.Fatalf("No of leaked sessions removed mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	if g, w := sp.numOpened, uint64(3); g != w {
		t.Fatalf("Session pool size mismatch\nGot: %d\nWant: %d\n", g, w)
	}
}

func TestLongRunningTransactions_WhenDurationBelowThreshold_VerifyInactiveSessionsOpen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened: 1,
			MaxOpened: 3,
			InactiveTransactionRemovalOptions: InactiveTransactionRemovalOptions{
				ActionOnInactiveTransaction: Warn,
			},
		},
	})
	defer teardown()
	sp := client.idleSessions

	// get session-1 from pool
	s1, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get the session: %v", err)
	}
	// get session-2 from pool
	s2, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get the session: %v", err)
	}
	// get session-3 from pool
	s3, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get the session: %v", err)
	}
	if g, w := sp.numInUse, uint64(3); g != w {
		sp.mu.Unlock()
		t.Fatalf("Number of sessions currently in use mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	sp.mu.Lock()
	if g, w := sp.numOpened, uint64(3); g != w {
		sp.mu.Unlock()
		t.Fatalf("Session pool size mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	sp.mu.Unlock()
	s1.mu.Lock()
	s1.eligibleForLongRunning = false
	s1.lastUseTime = time.Now().Add(-50 * time.Minute)
	s1.mu.Unlock()

	s2.mu.Lock()
	s2.eligibleForLongRunning = false
	s2.lastUseTime = time.Now().Add(-50 * time.Minute)
	s2.mu.Unlock()

	s3.mu.Lock()
	s3.eligibleForLongRunning = true
	s3.lastUseTime = time.Now().Add(-50 * time.Minute)
	s3.mu.Unlock()

	// force run task to clean up unexpected long-running sessions
	sp.removeLongRunningSessions()

	s1.mu.Lock()
	if s1.isSessionLeakLogged {
		t.Fatalf("Session leak should not be logged for session %v as checkout duration is <60 mins", s1.session.id)
	}
	s1.mu.Unlock()

	s2.mu.Lock()
	if s2.isSessionLeakLogged {
		t.Fatalf("Session leak should not be logged for session %v as checkout duration is <60 mins", s2.session.id)
	}
	s2.mu.Unlock()

	sp.mu.Lock()
	defer sp.mu.Unlock()
	if g, w := sp.numOfLeakedSessionsRemoved, uint64(0); g != w {
		t.Fatalf("No of leaked sessions removed mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	if g, w := sp.numOpened, uint64(3); g != w {
		t.Fatalf("Session pool size mismatch\nGot: %d\nWant: %d\n", g, w)
	}
}

// TestMaxOpenedSessions tests max open sessions constraint.
func TestMaxOpenedSessions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, client, teardown := setupMockedTestServerWithConfig(t,
		ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig: SessionPoolConfig{
				MaxOpened: 1,
			},
		})
	defer teardown()
	sp := client.idleSessions

	sh1, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot take session from session pool: %v", err)
	}

	// Session request will timeout due to the max open sessions constraint.
	ctx2, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	_, gotErr := sp.take(ctx2)
	if wantErr := sp.errGetBasicSessionTimeout(codes.DeadlineExceeded); !testEqual(gotErr, wantErr) {
		t.Fatalf("the second session retrival returns error %v, want %v", gotErr, wantErr)
	}
	doneWaiting := make(chan struct{})
	go func() {
		// Destroy the first session to allow the next session request to
		// proceed.
		<-doneWaiting
		sh1.destroy()
	}()

	go func() {
		// Wait a short random time before destroying the session handle.
		<-time.After(10 * time.Millisecond)
		close(doneWaiting)
	}()
	// Now session request can be processed because the first session will be
	// destroyed.
	ctx3, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	sh2, err := sp.take(ctx3)
	if err != nil {
		t.Fatalf("after the first session is destroyed, session retrival still returns error %v, want nil", err)
	}
	if !sh2.session.isValid() || sh2.getID() == "" {
		t.Fatalf("got invalid session: %v", sh2.session)
	}
}

// TestMinOpenedSessions tests min open session constraint.
func TestMinOpenedSessions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, client, teardown := setupMockedTestServerWithConfig(t,
		ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig: SessionPoolConfig{
				MinOpened:                 1,
				healthCheckSampleInterval: time.Millisecond,
			},
		})
	defer teardown()
	sp := client.idleSessions

	// Take ten sessions from session pool and recycle them.
	var ss []*session
	var shs []*sessionHandle
	for i := 0; i < 10; i++ {
		sh, err := sp.take(ctx)
		if err != nil {
			t.Fatalf("failed to get session(%v): %v", i, err)
		}
		ss = append(ss, sh.session)
		shs = append(shs, sh)
		sh.recycle()
	}
	for _, sh := range shs {
		sh.recycle()
	}

	// Simulate session expiration.
	for _, s := range ss {
		s.destroy(true, false)
	}

	// Wait until the maintainer has had a chance to replenish the pool.
	for i := 0; i < 10; i++ {
		sp.mu.Lock()
		if sp.numOpened > 0 {
			sp.mu.Unlock()
			break
		}
		sp.mu.Unlock()
		<-time.After(sp.healthCheckSampleInterval)
	}
	sp.mu.Lock()
	defer sp.mu.Unlock()
	// There should be still one session left in either the idle list or in one
	// of the other opened states due to the min open sessions constraint.
	if (sp.idleList.Len() +
		int(sp.createReqs)) != 1 {
		t.Fatalf(
			"got %v sessions in idle lists, want 1. Opened: %d, Creation: %d",
			sp.idleList.Len(), sp.numOpened, sp.createReqs)
	}
}

// TestPositiveNumInUseSessions tests that num_in_use session should always be greater than 0.
func TestPositiveNumInUseSessions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, client, teardown := setupMockedTestServerWithConfig(t,
		ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig: SessionPoolConfig{
				MinOpened:                 1,
				healthCheckSampleInterval: time.Millisecond,
			},
		})
	defer teardown()
	sp := client.idleSessions
	defer sp.close(ctx)
	// Take ten sessions from session pool and recycle them.
	var shs []*sessionHandle
	for i := 0; i < 10; i++ {
		sh, err := sp.take(ctx)
		if err != nil {
			t.Fatalf("failed to get session(%v): %v", i, err)
		}
		shs = append(shs, sh)
	}
	for _, sh := range shs {
		sh.recycle()
	}
	waitFor(t, func() error {
		sp.mu.Lock()
		if sp.idleList.Len() != 1 {
			sp.mu.Unlock()
			return errInvalidSessionPool
		}
		sp.mu.Unlock()
		return nil
	})
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if int64(sp.numInUse) < 0 {
		t.Fatal("numInUse must be >= 0")
	}
	// There should be still one session left in the idle list.
	if sp.idleList.Len() != 1 {
		t.Fatalf("got %v sessions in idle lists, want 1. Opened: %d, Creation: %d", sp.idleList.Len(), sp.numOpened, sp.createReqs)
	}
}

// TestMaxBurst tests max burst constraint.
func TestMaxBurst(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServerWithConfig(t,
		ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig: SessionPoolConfig{
				MaxBurst: 1,
			},
		})
	defer teardown()

	sp := client.idleSessions

	// Will cause session creation RPC to be retried forever.
	server.TestSpanner.PutExecutionTime(MethodBatchCreateSession,
		SimulatedExecutionTime{
			Errors:    []error{status.Errorf(codes.Unavailable, "try later")},
			KeepError: true,
		})

	// This session request will never finish until the injected error is
	// cleared.
	go sp.take(ctx)

	// Poll for the execution of the first session request.
	for {
		sp.mu.Lock()
		cr := sp.createReqs
		sp.mu.Unlock()
		if cr == 0 {
			<-time.After(time.Millisecond * 10)
			continue
		}
		// The first session request is being executed.
		break
	}

	ctx2, cancel := context.WithTimeout(ctx, time.Millisecond*10)
	defer cancel()
	_, gotErr := sp.take(ctx2)

	// Since MaxBurst == 1, the second session request should block.
	if wantErr := sp.errGetBasicSessionTimeout(codes.DeadlineExceeded); !testEqual(gotErr, wantErr) {
		t.Fatalf("session retrival returns error %v, want %v", gotErr, wantErr)
	}

	// Let the first session request succeed.
	server.TestSpanner.Freeze()
	server.TestSpanner.PutExecutionTime(MethodBatchCreateSession, SimulatedExecutionTime{})
	server.TestSpanner.Unfreeze()

	// Now new session request can proceed because the first session request will eventually succeed.
	sh, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("session retrival returns error %v, want nil", err)
	}
	if !sh.session.isValid() || sh.getID() == "" {
		t.Fatalf("got invalid session: %v", sh.session)
	}
}

// TestSessionRecycle tests recycling sessions.
func TestSessionRecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Set MaxBurst=MinOpened to prevent additional sessions to be created
	// while session pool initialization is still running.
	_, client, teardown := setupMockedTestServerWithConfig(t,
		ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig: SessionPoolConfig{
				MinOpened: 1,
				MaxIdle:   5,
				MaxBurst:  1,
			},
		})
	defer teardown()
	sp := client.idleSessions

	// Test session is correctly recycled and reused.
	for i := 0; i < 20; i++ {
		s, err := sp.take(ctx)
		if err != nil {
			t.Fatalf("cannot get the session %v: %v", i, err)
		}
		s.recycle()
	}

	sp.mu.Lock()
	defer sp.mu.Unlock()
	// The session pool should only contain 1 session, as there is no minimum
	// configured. In addition, there has never been more than one session in
	// use at any time, so there's no need for the session pool to create a
	// second session. The session has also been in use all the time, so there
	// also no reason for the session pool to delete the session.
	if sp.numOpened != 1 {
		t.Fatalf("Expect session pool size 1, got %d", sp.numOpened)
	}
}

// TestSessionDestroy tests destroying sessions.
func TestSessionDestroy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := log.Default()
	logger.SetOutput(io.Discard)
	_, client, teardown := setupMockedTestServerWithConfig(t,
		ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig: SessionPoolConfig{
				MinOpened: 1,
				MaxBurst:  1,
			},
			Logger: logger,
		})
	defer teardown()
	sp := client.idleSessions

	// Creating a session pool with MinSessions=1 will automatically start the
	// creation of 1 session when the session pool is created. As MaxBurst=1,
	// the session pool will never create more than 1 session at a time, so the
	// take() method will wait if the initial session has not yet been created.
	sh, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get session from session pool: %v", err)
	}
	s := sh.session
	sh.recycle()
	if d := s.destroy(true, false); d || !s.isValid() {
		// Session should be remaining because of min open session's constraint.
		t.Fatalf("session %s invalid, want it to stay alive. (destroy in expiration mode, success: %v)", s.id, d)
	}
	if d := s.destroy(false, true); !d || s.isValid() {
		// Session should be destroyed.
		t.Fatalf("failed to destroy session %s. (destroy in default mode, success: %v)", s.id, d)
	}
}

// TestHcHeap tests heap operation on top of hcHeap.
func TestHcHeap(t *testing.T) {
	in := []*session{
		{nextCheck: time.Unix(10, 0)},
		{nextCheck: time.Unix(0, 5)},
		{nextCheck: time.Unix(1, 8)},
		{nextCheck: time.Unix(11, 7)},
		{nextCheck: time.Unix(6, 3)},
	}
	want := []*session{
		{nextCheck: time.Unix(1, 8), hcIndex: 0},
		{nextCheck: time.Unix(6, 3), hcIndex: 1},
		{nextCheck: time.Unix(8, 2), hcIndex: 2},
		{nextCheck: time.Unix(10, 0), hcIndex: 3},
		{nextCheck: time.Unix(11, 7), hcIndex: 4},
	}
	hh := hcHeap{}
	for _, s := range in {
		heap.Push(&hh, s)
	}
	// Change top of the heap and do a adjustment.
	hh.sessions[0].nextCheck = time.Unix(8, 2)
	heap.Fix(&hh, 0)
	for idx := 0; hh.Len() > 0; idx++ {
		got := heap.Pop(&hh).(*session)
		want[idx].hcIndex = -1
		if !testEqual(got, want[idx]) {
			t.Fatalf("%v: heap.Pop returns %v, want %v", idx, got, want[idx])
		}
	}
}

// TestHealthCheckScheduler tests if healthcheck workers can schedule and
// perform healthchecks properly.
func TestHealthCheckScheduler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServerWithConfig(t,
		ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig: SessionPoolConfig{
				HealthCheckInterval:       50 * time.Millisecond,
				healthCheckSampleInterval: 10 * time.Millisecond,
			},
		})
	defer teardown()
	sp := client.idleSessions

	// Create 50 sessions.
	for i := 0; i < 50; i++ {
		_, err := sp.take(ctx)
		if err != nil {
			t.Fatalf("cannot get session from session pool: %v", err)
		}
	}

	// Make sure we start with a ping history to avoid that the first
	// sessions that were created have not already exceeded the maximum
	// number of pings.
	server.TestSpanner.ClearPings()
	// Wait for 10-30 pings per session.
	waitFor(t, func() error {
		// Only check actually live sessions and ignore any sessions the
		// session pool may have deleted in the meantime.
		liveSessions := server.TestSpanner.DumpSessions()
		dp := server.TestSpanner.DumpPings()
		gotPings := map[string]int64{}
		for _, p := range dp {
			gotPings[p]++
		}
		for s := range liveSessions {
			if strings.Contains(s, "multiplexed") {
				// no pings for multiplexed sessions
				if gotPings[s] > 0 {
					return fmt.Errorf("got %v healthchecks on multiplexed session %v, want 0", gotPings[s], s)
				}
				continue
			}
			want := int64(20)
			if got := gotPings[s]; got < want/2 || got > want+want/2 {
				// This is an unnacceptable amount of pings.
				return fmt.Errorf("got %v healthchecks on session %v, want it between (%v, %v)", got, s, want/2, want+want/2)
			}
		}
		return nil
	})
}

// TestHealthCheck_FirstHealthCheck tests if the first healthcheck scheduling
// works properly.
func TestHealthCheck_FirstHealthCheck(t *testing.T) {
	t.Parallel()
	_, client, teardown := setupMockedTestServerWithConfig(t,
		ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig: SessionPoolConfig{
				MaxOpened:           0,
				MinOpened:           0,
				HealthCheckInterval: 50 * time.Minute,
			},
		})
	defer teardown()
	sp := client.idleSessions

	now := time.Now()
	start := now.Add(time.Duration(float64(sp.hc.interval) * 0.2))
	// A second is added to avoid the edge case.
	end := now.Add(time.Duration(float64(sp.hc.interval)*1.1) + time.Second)

	s := &session{}
	sp.hc.scheduledHCLocked(s)

	if s.nextCheck.Before(start) || s.nextCheck.After(end) {
		t.Fatalf("The first healthcheck schedule is not in the correct range: %v", s.nextCheck)
	}
	if !s.firstHCDone {
		t.Fatal("The flag 'firstHCDone' should be set to true after the first healthcheck.")
	}
}

// TestHealthCheck_NonFirstHealthCheck tests if the scheduling after the first
// health check works properly.
func TestHealthCheck_NonFirstHealthCheck(t *testing.T) {
	t.Parallel()
	_, client, teardown := setupMockedTestServerWithConfig(t,
		ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig: SessionPoolConfig{
				MaxOpened:           0,
				MinOpened:           0,
				HealthCheckInterval: 50 * time.Minute,
			},
		})
	defer teardown()
	sp := client.idleSessions

	now := time.Now()
	start := now.Add(time.Duration(float64(sp.hc.interval) * 0.9))
	// A second is added to avoid the edge case.
	end := now.Add(time.Duration(float64(sp.hc.interval)*1.1) + time.Second)

	s := &session{firstHCDone: true}
	sp.hc.scheduledHCLocked(s)

	if s.nextCheck.Before(start) || s.nextCheck.After(end) {
		t.Fatalf("The non-first healthcheck schedule is not in the correct range: %v", s.nextCheck)
	}
}

// TestSessionHealthCheck tests healthchecking cases.
func TestSessionHealthCheck(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServerWithConfig(t,
		ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig: SessionPoolConfig{
				HealthCheckInterval:       time.Nanosecond,
				healthCheckSampleInterval: 10 * time.Millisecond,
				incStep:                   1,
			},
		})
	defer teardown()
	sp := client.idleSessions

	// Test pinging sessions.
	sh, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get session from session pool: %v", err)
	}

	// Wait for healthchecker to send pings to session.
	waitFor(t, func() error {
		pings := server.TestSpanner.DumpPings()
		if len(pings) == 0 || pings[0] != sh.getID() {
			return fmt.Errorf("healthchecker didn't send any ping to session %v", sh.getID())
		}
		return nil
	})
	// Test broken session detection.
	sh, err = sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get session from session pool: %v", err)
	}

	server.TestSpanner.Freeze()
	server.TestSpanner.PutExecutionTime(MethodExecuteSql,
		SimulatedExecutionTime{
			Errors:    []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")},
			KeepError: true,
		})
	server.TestSpanner.Unfreeze()

	s := sh.session
	waitFor(t, func() error {
		if sh.session.isValid() {
			return fmt.Errorf("session(%v) is still alive, want it to be dropped by healthcheck workers", s)
		}
		return nil
	})

	server.TestSpanner.Freeze()
	server.TestSpanner.PutExecutionTime(MethodExecuteSql, SimulatedExecutionTime{})
	server.TestSpanner.Unfreeze()

	// Test garbage collection.
	sh, err = sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get session from session pool: %v", err)
	}
	sp.close(context.Background())
	if sh.session.isValid() {
		t.Fatalf("session(%v) is still alive, want it to be garbage collected", s)
	}
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
	t.Parallel()

	// Use concurrent workers to test different session pool built from different configurations.
	for ti, cfg := range []SessionPoolConfig{
		{},
		{MinOpened: 10, MaxOpened: 100},
		{MaxBurst: 50},
		{MinOpened: 10, MaxOpened: 200, MaxBurst: 5},
		{MinOpened: 10, MaxOpened: 200, MaxBurst: 5, WriteSessions: 0.2},
	} {
		// Create a more aggressive session healthchecker to increase test concurrency.
		cfg.HealthCheckInterval = 50 * time.Millisecond
		cfg.healthCheckSampleInterval = 10 * time.Millisecond
		cfg.HealthCheckWorkers = 50

		server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig:    cfg,
		})
		sp := client.idleSessions

		// Create a test group for this configuration and schedule 100 sub
		// sub tests within the group.
		t.Run(fmt.Sprintf("TestStressSessionPoolGroup%v", ti), func(t *testing.T) {
			for i := 0; i < 100; i++ {
				idx := i
				t.Logf("TestStressSessionPoolWithCfg%dWorker%03d", ti, idx)
				testStressSessionPool(t, cfg, ti, idx, sp, client)
			}
		})
		sp.hc.close()
		// Here the states of healthchecker, session pool and mockclient are
		// stable.
		sp.mu.Lock()
		idleSessions := map[string]bool{}
		hcSessions := map[string]bool{}
		mockSessions := server.TestSpanner.DumpSessions()
		// Dump session pool's idle list.
		for sl := sp.idleList.Front(); sl != nil; sl = sl.Next() {
			s := sl.Value.(*session)
			if idleSessions[s.getID()] {
				t.Fatalf("%v: found duplicated session in idle list: %v", ti, s.getID())
			}
			idleSessions[s.getID()] = true
		}
		if int(sp.numOpened) != len(idleSessions) {
			t.Fatalf("%v: number of opened sessions (%v) != number of idle sessions (%v)", ti, sp.numOpened, len(idleSessions))
		}
		if sp.createReqs != 0 {
			t.Fatalf("%v: number of pending session creations = %v, want 0", ti, sp.createReqs)
		}
		// Dump healthcheck queue.
		sp.hc.mu.Lock()
		for _, s := range sp.hc.queue.sessions {
			if hcSessions[s.getID()] {
				t.Fatalf("%v: found duplicated session in healthcheck queue: %v", ti, s.getID())
			}
			hcSessions[s.getID()] = true
		}
		sp.mu.Unlock()
		sp.hc.mu.Unlock()

		// Verify that idleSessions == hcSessions == mockSessions.
		if !testEqual(idleSessions, hcSessions) {
			t.Fatalf("%v: sessions in idle list (%v) != sessions in healthcheck queue (%v)", ti, idleSessions, hcSessions)
		}
		// The server may contain more sessions than the health check queue.
		// This can be caused by a timeout client side during a CreateSession
		// request. The request may still be received and executed by the
		// server, but the session pool will not register the session.
		for id, b := range hcSessions {
			if b && !mockSessions[id] {
				t.Fatalf("%v: session in healthcheck queue (%v) was not found on server", ti, id)
			}
		}
		sp.close(context.Background())
		mockSessions = server.TestSpanner.DumpSessions()
		for id, b := range hcSessions {
			if b && mockSessions[id] {
				// We only log a warning for this, as it sometimes happens.
				// The exact reason for it is unknown, but in a real life
				// situation the session would be garbage collected by the
				// server after 60 minutes.
				t.Logf("Found session from pool still live on server: %v", id)
			}
		}
		teardown()
	}
}

func testStressSessionPool(t *testing.T, cfg SessionPoolConfig, ti int, idx int, pool *sessionPool, client *Client) {
	ctx := context.Background()
	// Test worker iterates 1K times and tries different
	// session / session pool operations.
	for j := 0; j < 1000; j++ {
		if idx%10 == 0 && j >= 900 {
			// Close the pool in selected set of workers during the
			// middle of the test.
			pool.close(context.Background())
		}
		var (
			sh     *sessionHandle
			gotErr error
		)
		wasValid := pool.isValid()
		sh, gotErr = pool.take(ctx)
		if gotErr != nil {
			if pool.isValid() {
				t.Fatalf("%v.%v: pool.take returns error when pool is still valid: %v", ti, idx, gotErr)
			}
			// If the session pool was closed when we tried to take a session
			// from the pool, then we should have gotten a specific error.
			// If the session pool was closed between the take() and now (or
			// even during a take()) then an error is ok.
			if !wasValid {
				if wantErr := errInvalidSessionPool; gotErr != wantErr {
					t.Fatalf("%v.%v: got error when pool is closed: %v, want %v", ti, idx, gotErr, wantErr)
				}
			}
			continue
		}
		// Verify if session is valid when session pool is valid.
		// Note that if session pool is invalid after sh is taken,
		// then sh might be invalidated by healthcheck workers.
		if (sh.getID() == "" || sh.session == nil || !sh.session.isValid()) && pool.isValid() {
			t.Fatalf("%v.%v: pool.take returns invalid session %v", ti, idx, sh.session)
		}
		if sh.getTransactionID() != nil {
			t.Fatalf("%v.%v: pool.take returns session %v with transaction", ti, idx, sh.session)
		}
		if rand.Intn(100) < idx {
			// Random sleep before destroying/recycling the session,
			// to give healthcheck worker a chance to step in.
			<-time.After(time.Duration(rand.Int63n(int64(cfg.HealthCheckInterval))))
		}
		if rand.Intn(100) < idx {
			// destroy the session.
			sh.destroy()
			continue
		}
		// recycle the session.
		sh.recycle()
	}
}

// TestMaintainer checks the session pool maintainer maintains the number of
// sessions in the following cases:
//
//  1. On initialization of session pool, replenish session pool to meet
//     MinOpened or MaxIdle.
//  2. On increased session usage, provision extra MaxIdle sessions.
//  3. After the surge passes, scale down the session pool accordingly.
func TestMaintainer(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	minOpened := uint64(5)
	maxIdle := uint64(4)
	_, client, teardown := setupMockedTestServerWithConfig(t,
		ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig: SessionPoolConfig{
				MinOpened:                 minOpened,
				MaxIdle:                   maxIdle,
				healthCheckSampleInterval: time.Millisecond,
			},
		})
	defer teardown()
	sp := client.idleSessions

	waitFor(t, func() error {
		sp.mu.Lock()
		defer sp.mu.Unlock()
		if sp.numOpened != minOpened {
			return fmt.Errorf("Replenish. Expect %d open, got %d", sp.MinOpened, sp.numOpened)
		}
		return nil
	})

	// To save test time, we are not creating many sessions, because the time
	// to create sessions will have impact on the decision on sessionsToKeep.
	// We also parallelize the take and recycle process.
	shs := make([]*sessionHandle, 20)
	for i := 0; i < len(shs); i++ {
		var err error
		shs[i], err = sp.take(ctx)
		if err != nil {
			t.Fatalf("cannot get session from session pool: %v", err)
		}
	}
	// Wait for all sessions to be added to the pool.
	// The pool already contained 5 sessions (MinOpened=5).
	// The test took 20 sessions from the pool. That initiated the creation of
	// additional sessions, and that is done in batches of 25 sessions, so the
	// pool should contain 30 sessions (with 20 currently checked out). It
	// could take a couple of milliseconds before all sessions have been
	// created and added to the pool.
	waitFor(t, func() error {
		sp.mu.Lock()
		defer sp.mu.Unlock()
		g, w := sp.numOpened, sp.MinOpened+sp.incStep
		if g != w {
			return fmt.Errorf("numOpened sessions mismatch\nGot: %d\nWant: %d", g, w)
		}
		return nil
	})

	// Return 14 sessions to the pool. There are still 6 sessions checked out.
	for _, sh := range shs[:14] {
		sh.recycle()
	}

	// The pool should scale down to sessionsInUse + MaxIdle = 6 + 4 = 10.
	waitFor(t, func() error {
		sp.mu.Lock()
		defer sp.mu.Unlock()
		if sp.numOpened != 10 {
			return fmt.Errorf("Keep extra MaxIdle sessions. Expect %d open, got %d", 10, sp.numOpened)
		}
		return nil
	})

	// Return the remaining 6 sessions.
	// The pool should now scale down to minOpened + maxIdle.
	for _, sh := range shs[14:] {
		sh.recycle()
	}
	waitFor(t, func() error {
		sp.mu.Lock()
		defer sp.mu.Unlock()
		if sp.numOpened != minOpened {
			return fmt.Errorf("Scale down. Expect %d open, got %d", minOpened+maxIdle, sp.numOpened)
		}
		return nil
	})
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
	t.Parallel()
	spc := SessionPoolConfig{
		MinOpened:                 10,
		MaxIdle:                   10,
		WriteSessions:             0.0,
		healthCheckSampleInterval: 20 * time.Millisecond,
	}
	server, client, teardown := setupMockedTestServerWithConfig(t,
		ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig:    spc,
			NumChannels:          4,
		})
	defer teardown()
	sp := client.idleSessions

	timeout := time.After(4 * time.Second)
	var numOpened int
loop:
	for {
		select {
		case <-timeout:
			t.Fatalf("timed out, got %d session(s), want %d", numOpened, spc.MinOpened)
		default:
			sp.mu.Lock()
			numOpened = sp.idleList.Len()
			sp.mu.Unlock()
			if numOpened == 10 {
				if isMultiplexEnabled {
					if sp.multiplexedSession == nil {
						continue
					}
				}
				break loop
			}
		}
	}
	_, err := shouldHaveReceived(server.TestSpanner, []interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BatchCreateSessionsRequest{},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (s1 *session) Equal(s2 *session) bool {
	return s1.client == s2.client &&
		s1.id == s2.id &&
		s1.pool == s2.pool &&
		s1.createTime == s2.createTime &&
		s1.valid == s2.valid &&
		s1.hcIndex == s2.hcIndex &&
		s1.idleList == s2.idleList &&
		s1.nextCheck.Equal(s2.nextCheck) &&
		s1.checkingHealth == s2.checkingHealth &&
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
	t.Parallel()

	ctx := context.Background()
	const sampleInterval = time.Millisecond * 10
	_, client, teardown := setupMockedTestServerWithConfig(t,
		ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig:    SessionPoolConfig{healthCheckSampleInterval: sampleInterval},
		})
	defer teardown()
	sp := client.idleSessions

	// Take two sessions from the pool.
	// This will cause max sessions in use to be 2 during this window.
	sh1 := takeSession(ctx, t, sp)
	sh2 := takeSession(ctx, t, sp)
	wantSessions := map[string]bool{}
	wantSessions[sh1.getID()] = true
	wantSessions[sh2.getID()] = true
	// Return the sessions to the pool and then assure that they
	// are not deleted while still within the maintenance window.
	sh1.recycle()
	sh2.recycle()
	// Wait for 20 milliseconds, i.e. approx 2 iterations of the
	// maintainer. The sessions should still be in the pool.
	<-time.After(sampleInterval * 2)
	sh3 := takeSession(ctx, t, sp)
	sh4 := takeSession(ctx, t, sp)
	// Check that the returned sessions are equal to the sessions that we got
	// the first time from the session pool.
	gotSessions := map[string]bool{}
	gotSessions[sh3.getID()] = true
	gotSessions[sh4.getID()] = true
	testEqual(wantSessions, gotSessions)
	// Return the sessions to the pool.
	sh3.recycle()
	sh4.recycle()

	// Now wait for the maintenance window to finish. This will cause the
	// maintainer to enter a new window and reset the max number of sessions in
	// use to the currently number of checked out sessions. That is 0, as all
	// sessions have been returned to the pool. That again will cause the
	// maintainer to delete these sessions at the next iteration, unless we
	// checkout new sessions during the first iteration.
	waitFor(t, func() error {
		sp.mu.Lock()
		defer sp.mu.Unlock()
		if sp.numOpened > 0 {
			return errors.New("session pool still contains more than 0 sessions")
		}
		return nil
	})
	sh5 := takeSession(ctx, t, sp)
	sh6 := takeSession(ctx, t, sp)
	// Assure that these sessions are new sessions.
	if gotSessions[sh5.getID()] || gotSessions[sh6.getID()] {
		t.Fatal("got unexpected existing session from pool")
	}
}

func takeSession(ctx context.Context, t *testing.T, sp *sessionPool) *sessionHandle {
	sh, err := sp.take(ctx)
	if err != nil {
		t.Fatalf("cannot get session from session pool: %v", err)
	}
	return sh
}

func TestMaintenanceWindow_CycleAndUpdateMaxCheckedOut(t *testing.T) {
	t.Parallel()

	maxOpened := uint64(1000)
	mw := newMaintenanceWindow(maxOpened)
	for _, m := range mw.maxSessionsCheckedOut {
		if m < maxOpened {
			t.Fatalf("Max sessions checked out mismatch.\nGot: %v\nWant: %v", m, maxOpened)
		}
	}
	// Do one cycle and simulate that there are currently no sessions checked
	// out of the pool.
	mw.startNewCycle(0)
	if g, w := mw.maxSessionsCheckedOut[0], uint64(0); g != w {
		t.Fatalf("Max sessions checked out mismatch.\nGot: %d\nWant: %d", g, w)
	}
	for _, m := range mw.maxSessionsCheckedOut[1:] {
		if m < maxOpened {
			t.Fatalf("Max sessions checked out mismatch.\nGot: %v\nWant: %v", m, maxOpened)
		}
	}
	// Check that the max checked out during the entire window is still
	// maxOpened.
	if g, w := mw.maxSessionsCheckedOutDuringWindow(), maxOpened; g != w {
		t.Fatalf("Max sessions checked out during window mismatch.\nGot: %d\nWant: %d", g, w)
	}
	// Update the max number checked out for the current cycle.
	mw.updateMaxSessionsCheckedOutDuringWindow(uint64(10))
	if g, w := mw.maxSessionsCheckedOut[0], uint64(10); g != w {
		t.Fatalf("Max sessions checked out mismatch.\nGot: %d\nWant: %d", g, w)
	}
	// The max of the entire window should still not change.
	if g, w := mw.maxSessionsCheckedOutDuringWindow(), maxOpened; g != w {
		t.Fatalf("Max sessions checked out during window mismatch.\nGot: %d\nWant: %d", g, w)
	}
	// Now pass enough cycles to complete a maintenance window. Each cycle has
	// no sessions checked out. We start at 1, as we have already passed one
	// cycle. This should then be the last cycle still in the maintenance
	// window, and the only one with a maxSessionsCheckedOut greater than 0.
	for i := 1; i < maintenanceWindowSize; i++ {
		mw.startNewCycle(0)
	}
	for _, m := range mw.maxSessionsCheckedOut[:9] {
		if m != 0 {
			t.Fatalf("Max sessions checked out mismatch.\nGot: %v\nWant: %v", m, 0)
		}
	}
	// The oldest cycle in the window should have max=10.
	if g, w := mw.maxSessionsCheckedOut[maintenanceWindowSize-1], uint64(10); g != w {
		t.Fatalf("Max sessions checked out mismatch.\nGot: %d\nWant: %d", g, w)
	}
	// The max of the entire window should now be 10.
	if g, w := mw.maxSessionsCheckedOutDuringWindow(), uint64(10); g != w {
		t.Fatalf("Max sessions checked out during window mismatch.\nGot: %d\nWant: %d", g, w)
	}
	// Do another cycle with max=0.
	mw.startNewCycle(0)
	// The max of the entire window should now be 0.
	if g, w := mw.maxSessionsCheckedOutDuringWindow(), uint64(0); g != w {
		t.Fatalf("Max sessions checked out during window mismatch.\nGot: %d\nWant: %d", g, w)
	}
	// Do another cycle with 5 sessions as max. This should now be the new
	// window max.
	mw.startNewCycle(5)
	if g, w := mw.maxSessionsCheckedOutDuringWindow(), uint64(5); g != w {
		t.Fatalf("Max sessions checked out during window mismatch.\nGot: %d\nWant: %d", g, w)
	}
	// Do a couple of cycles so that the only non-zero value is in the middle.
	// The max for the entire window should still be 5.
	for i := 0; i < maintenanceWindowSize/2; i++ {
		mw.startNewCycle(0)
	}
	if g, w := mw.maxSessionsCheckedOutDuringWindow(), uint64(5); g != w {
		t.Fatalf("Max sessions checked out during window mismatch.\nGot: %d\nWant: %d", g, w)
	}
}

func TestSessionCreationIsDistributedOverChannels(t *testing.T) {
	if useGRPCgcp {
		// Session distribution with GCPMultiEndpoint is tested in sessionclient_test.go/TestBatchCreateAndCloseSession.
		t.Skip("GCPMultiEndpoint hides behind a single grpc.ClientConn")
	}
	t.Parallel()
	numChannels := 4
	spc := SessionPoolConfig{
		MinOpened:     12,
		WriteSessions: 0.0,
		incStep:       2,
	}
	_, client, teardown := setupMockedTestServerWithConfig(t,
		ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig:    spc,
			NumChannels:          numChannels,
		})
	defer teardown()
	sp := client.idleSessions

	waitFor(t, func() error {
		sp.mu.Lock()
		// WriteSessions = 0, so we only have to check for read sessions.
		numOpened := uint64(sp.idleList.Len())
		sp.mu.Unlock()
		if numOpened < spc.MinOpened {
			return errors.New("not yet initialized")
		}
		return nil
	})

	sessionsPerChannel := getSessionsPerChannel(sp)
	if g, w := len(sessionsPerChannel), numChannels; g != w {
		t.Errorf("number of channels mismatch\nGot: %d\nWant: %d", g, w)
	}
	for k, v := range sessionsPerChannel {
		if g, w := v, int(sp.MinOpened)/numChannels; g != w {
			t.Errorf("number of sessions mismatch for %s:\nGot: %d\nWant: %d", k, g, w)
		}
	}
	// Check out all sessions + incStep * numChannels from the pool. This
	// should cause incStep * numChannels additional sessions to be created.
	checkedOut := make([]*sessionHandle, sp.MinOpened+sp.incStep*uint64(numChannels))
	var err error
	for i := 0; i < cap(checkedOut); i++ {
		checkedOut[i], err = sp.take(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < cap(checkedOut); i++ {
		checkedOut[i].recycle()
	}
	// The additional sessions should also be distributed over all available
	// channels.
	sessionsPerChannel = getSessionsPerChannel(sp)
	// There should not be any new clients (channels).
	if g, w := len(sessionsPerChannel), numChannels; g != w {
		t.Errorf("number of channels mismatch\nGot: %d\nWant: %d", g, w)
	}
	for k, v := range sessionsPerChannel {
		if g, w := v, int(sp.MinOpened)/numChannels+int(sp.incStep); g != w {
			t.Errorf("number of sessions mismatch for %s:\nGot: %d\nWant: %d", k, g, w)
		}
	}
}

func getSessionsPerChannel(sp *sessionPool) map[string]int {
	sessionsPerChannel := make(map[string]int)
	sp.mu.Lock()
	defer sp.mu.Unlock()
	el := sp.idleList.Front()
	for el != nil {
		s, _ := el.Value.(*session)
		// Get the pointer to the actual underlying gRPC ClientConn and use
		// that as the key in the map.
		val := reflect.ValueOf(s.client).Elem()
		rawClient := val.FieldByName("raw").Elem()
		internalClient := rawClient.FieldByName("internalClient").Elem().Elem()
		connPool := internalClient.FieldByName("connPool").Elem().Elem()
		conn := connPool.Field(0).Pointer()
		key := fmt.Sprintf("%v", conn)
		sessionsPerChannel[key] = sessionsPerChannel[key] + 1
		el = el.Next()
	}
	return sessionsPerChannel
}
