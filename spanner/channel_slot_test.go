// Copyright 2026 Google LLC
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

package spanner

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/spanner/apiv1/spannerpb"
	. "cloud.google.com/go/spanner/internal/testutil"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// pathTestServer is an in-memory Spanner server that counts the RPCs and the
// client connections it serves, standing in for one side of a slot.
type pathTestServer struct {
	*MockedSpannerInMemTestServer
	opts  []option.ClientOption
	mu    sync.Mutex
	calls map[string]int
}

func newPathTestServer(t *testing.T) *pathTestServer {
	t.Helper()
	s := &pathTestServer{calls: map[string]int{}}
	server, opts, teardown := NewMockedSpannerInMemTestServer(t,
		grpc.ChainUnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			s.count(info.FullMethod)
			return handler(ctx, req)
		}),
		grpc.ChainStreamInterceptor(func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			s.count(info.FullMethod)
			return handler(srv, ss)
		}),
	)
	t.Cleanup(teardown)
	s.MockedSpannerInMemTestServer = server
	s.opts = opts
	return s
}

func (s *pathTestServer) count(method string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls[method]++
}

func (s *pathTestServer) callCount(method string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[method]
}

const (
	createSessionMethod       = "/google.spanner.v1.Spanner/CreateSession"
	executeStreamingSQLMethod = "/google.spanner.v1.Spanner/ExecuteStreamingSql"
)

func dialTestPath(t *testing.T, opts []option.ClientOption) gtransport.ConnPool {
	t.Helper()
	pool, err := gtransport.DialPool(context.Background(), append(opts, option.WithGRPCConnectionPool(1))...)
	if err != nil {
		t.Fatalf("DialPool() failed: %v", err)
	}
	return pool
}

// countingDial dials opts and counts how often it was called.
type countingDial struct {
	t     *testing.T
	opts  []option.ClientOption
	err   error // returned instead of dialing while set
	mu    sync.Mutex
	dials int
}

func (d *countingDial) dial(ctx context.Context) (gtransport.ConnPool, error) {
	d.mu.Lock()
	d.dials++
	err := d.err
	d.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return gtransport.DialPool(ctx, append(d.opts, option.WithGRPCConnectionPool(1))...)
}

func (d *countingDial) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dials
}

// forceDirectPathFallback moves f to CloudPath through the real switch rule:
// a window in which every DirectPath call failed.
func forceDirectPathFallback(t *testing.T, f *directPathFallback) {
	t.Helper()
	f.closeWindow(time.Now())
	f.record(f.current().generation, true)
	if res := f.closeWindow(time.Now()); !res.switched {
		t.Fatalf("pool did not fall back: %+v", res)
	}
}

type slotTest struct {
	direct, cloud *pathTestServer
	dial          *countingDial
	fallback      *directPathFallback
	slot          *channelSlot
	client        spannerpb.SpannerClient
}

func newSlotTest(t *testing.T) *slotTest {
	t.Helper()
	st := &slotTest{direct: newPathTestServer(t), cloud: newPathTestServer(t)}
	st.dial = &countingDial{t: t, opts: st.cloud.opts}
	st.fallback = newTestDirectPathFallback(t)
	st.slot = newChannelSlot(7, dialTestPath(t, st.direct.opts), st.fallback, st.dial.dial)
	t.Cleanup(func() { st.slot.Close() })
	st.client = spannerpb.NewSpannerClient(st.slot)
	return st
}

func (st *slotTest) createSession(t *testing.T) *spannerpb.Session {
	t.Helper()
	s, err := st.client.CreateSession(context.Background(), &spannerpb.CreateSessionRequest{Database: "projects/p/instances/i/databases/d"})
	if err != nil {
		t.Fatalf("CreateSession() failed: %v", err)
	}
	return s
}

func TestChannelSlotRoutesBySwitchAndDialsCloudPathLazily(t *testing.T) {
	skipDialectRerun(t)
	st := newSlotTest(t)

	st.createSession(t)
	if got, want := st.direct.callCount(createSessionMethod), 1; got != want {
		t.Fatalf("DirectPath CreateSession calls = %d, want %d", got, want)
	}
	if got := st.dial.count(); got != 0 {
		t.Fatalf("CloudPath dials before the switch = %d, want 0", got)
	}

	forceDirectPathFallback(t, st.fallback)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			st.createSession(t)
		}()
	}
	wg.Wait()
	if got, want := st.cloud.callCount(createSessionMethod), 20; got != want {
		t.Fatalf("CloudPath CreateSession calls = %d, want %d", got, want)
	}
	if got, want := st.direct.callCount(createSessionMethod), 1; got != want {
		t.Fatalf("DirectPath CreateSession calls after the switch = %d, want %d", got, want)
	}
	if got, want := st.dial.count(), 1; got != want {
		t.Fatalf("CloudPath dials = %d, want %d", got, want)
	}
	if got, want := st.slot.Conn().Target(), st.cloud.ServerAddress; got != want {
		t.Fatalf("Conn().Target() after the switch = %q, want %q", got, want)
	}
}

func TestChannelSlotInFlightStreamFinishesOnDirectPath(t *testing.T) {
	skipDialectRerun(t)
	st := newSlotTest(t)
	session := st.createSession(t)

	stream, err := st.client.ExecuteStreamingSql(context.Background(), &spannerpb.ExecuteSqlRequest{Session: session.Name, Sql: SelectFooFromBar})
	if err != nil {
		t.Fatalf("ExecuteStreamingSql() failed: %v", err)
	}
	forceDirectPathFallback(t, st.fallback)

	// The stream opened before the switch keeps reading from DirectPath.
	rows := 0
	for {
		prs, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Recv() failed: %v", err)
		}
		rows += len(prs.Values)
	}
	if rows == 0 {
		t.Fatal("stream returned no values")
	}
	if got := st.cloud.callCount(executeStreamingSQLMethod); got != 0 {
		t.Fatalf("CloudPath ExecuteStreamingSql calls = %d, want 0", got)
	}
	// The next RPC uses the slot's CloudPath side.
	st.createSession(t)
	if got, want := st.cloud.callCount(createSessionMethod), 1; got != want {
		t.Fatalf("CloudPath CreateSession calls after the stream = %d, want %d", got, want)
	}
}

func TestChannelSlotRecordsEachStreamOnce(t *testing.T) {
	skipDialectRerun(t)
	st := newSlotTest(t)
	session := st.createSession(t)
	st.direct.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql, SimulatedExecutionTime{
		Errors: []error{status.Error(codes.Unavailable, "DirectPath stream failed")},
	})
	stream, err := st.client.ExecuteStreamingSql(context.Background(), &spannerpb.ExecuteSqlRequest{Session: session.Name, Sql: SelectFooFromBar})
	if err != nil {
		t.Fatalf("ExecuteStreamingSql() failed: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := stream.Recv(); status.Code(err) != codes.Unavailable {
			t.Fatalf("Recv() error = %v, want UNAVAILABLE", err)
		}
	}
	res := st.fallback.closeWindow(time.Now())
	// One successful CreateSession and one failed stream, however often the
	// caller read the failure.
	if res.failures != 1 || res.successes != 1 {
		t.Fatalf("window = %d failures and %d successes, want 1 and 1", res.failures, res.successes)
	}
}

// expiringContext is a context whose deadline the test passes by hand: Done
// closes and Err reports context.DeadlineExceeded, without waiting for a
// clock.
type expiringContext struct {
	context.Context
	done    chan struct{}
	expired atomic.Bool
	once    sync.Once
}

func newExpiringContext() *expiringContext {
	return &expiringContext{Context: context.Background(), done: make(chan struct{})}
}

func (c *expiringContext) Done() <-chan struct{} { return c.done }

func (c *expiringContext) Err() error {
	if c.expired.Load() {
		return context.DeadlineExceeded
	}
	return nil
}

func (c *expiringContext) expire() {
	c.once.Do(func() {
		c.expired.Store(true)
		close(c.done)
	})
}

func TestChannelSlotRecordsAbandonedStreamWhenContextEnds(t *testing.T) {
	skipDialectRerun(t)
	st := newSlotTest(t)
	session := st.createSession(t)
	st.fallback.closeWindow(time.Now())

	// The server holds the stream open and the caller never reads it, the way
	// generated code abandons a stream whose SendMsg failed. The stream ends
	// when its deadline passes.
	st.direct.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql, SimulatedExecutionTime{MinimumExecutionTime: 5 * time.Second})
	ctx := newExpiringContext()
	if _, err := st.client.ExecuteStreamingSql(ctx, &spannerpb.ExecuteSqlRequest{Session: session.Name, Sql: SelectFooFromBar}); err != nil {
		t.Fatalf("ExecuteStreamingSql() failed: %v", err)
	}
	ctx.expire()
	waitFor(t, func() error {
		if !st.fallback.current().fallback {
			if st.fallback.closeWindow(time.Now()).switched {
				return nil
			}
			return errors.New("abandoned stream not recorded yet")
		}
		return nil
	})
}

func TestChannelSlotBlackholedDirectPathFallsBack(t *testing.T) {
	skipDialectRerun(t)
	// A DirectPath endpoint that accepts TCP and never answers: established
	// connections that drop every packet look like this to the client.
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lis.Close() })
	var held []net.Conn
	var heldMu sync.Mutex
	go func() {
		for {
			c, err := lis.Accept()
			if err != nil {
				return
			}
			heldMu.Lock()
			held = append(held, c)
			heldMu.Unlock()
		}
	}()
	t.Cleanup(func() {
		heldMu.Lock()
		defer heldMu.Unlock()
		for _, c := range held {
			c.Close()
		}
	})

	cloud := newPathTestServer(t)
	dial := &countingDial{t: t, opts: cloud.opts}
	f := newTestDirectPathFallback(t)
	direct := dialTestPath(t, []option.ClientOption{
		option.WithEndpoint(lis.Addr().String()),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	})
	slot := newChannelSlot(1, direct, f, dial.dial)
	t.Cleanup(func() { slot.Close() })
	client := spannerpb.NewSpannerClient(slot)

	// Only the expiry matters: the blackhole never answers.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := client.CreateSession(ctx, &spannerpb.CreateSessionRequest{Database: "projects/p/instances/i/databases/d"}); status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("CreateSession() on blackholed DirectPath error = %v, want DEADLINE_EXCEEDED", err)
	}
	if res := f.closeWindow(time.Now()); !res.switched {
		t.Fatalf("pool did not fall back after a blackholed window: %+v", res)
	}
	if _, err := client.CreateSession(context.Background(), &spannerpb.CreateSessionRequest{Database: "projects/p/instances/i/databases/d"}); err != nil {
		t.Fatalf("CreateSession() after fallback failed: %v", err)
	}
	if got, want := cloud.callCount(createSessionMethod), 1; got != want {
		t.Fatalf("CloudPath CreateSession calls = %d, want %d", got, want)
	}
}

func TestChannelSlotRetriesFailedCloudPathDial(t *testing.T) {
	skipDialectRerun(t)
	st := newSlotTest(t)
	forceDirectPathFallback(t, st.fallback)
	st.dial.mu.Lock()
	st.dial.err = errors.New("dial failed")
	st.dial.mu.Unlock()
	_, err := st.client.CreateSession(context.Background(), &spannerpb.CreateSessionRequest{Database: "projects/p/instances/i/databases/d"})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("CreateSession() with a failing CloudPath dial error = %v, want UNAVAILABLE", err)
	}
	st.dial.mu.Lock()
	st.dial.err = nil
	st.dial.mu.Unlock()
	st.createSession(t)
	if got, want := st.dial.count(), 2; got != want {
		t.Fatalf("CloudPath dials = %d, want %d", got, want)
	}
}

// closeCountingPool is a ConnPool fake that only counts Close calls.
type closeCountingPool struct {
	gtransport.ConnPool
	closes atomic.Int32
}

func (p *closeCountingPool) Close() error {
	p.closes.Add(1)
	return nil
}

func (p *closeCountingPool) Num() int               { return 1 }
func (p *closeCountingPool) Conn() *grpc.ClientConn { return nil }

func TestChannelSlotCloseClosesBothSidesOnce(t *testing.T) {
	skipDialectRerun(t)
	direct := &closeCountingPool{}
	cloud := &closeCountingPool{}
	f := newTestDirectPathFallback(t)
	slot := newChannelSlot(1, direct, f, func(context.Context) (gtransport.ConnPool, error) { return cloud, nil })
	forceDirectPathFallback(t, f)
	if _, err := slot.cloudConn(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := slot.Close(); err != nil {
		t.Fatal(err)
	}
	if err := slot.Close(); err != nil {
		t.Fatal(err)
	}
	if direct.closes.Load() != 1 || cloud.closes.Load() != 1 {
		t.Fatalf("closes = {direct: %d, cloud: %d}, want 1 each", direct.closes.Load(), cloud.closes.Load())
	}
}

func TestChannelSlotDoesNotDialAfterClose(t *testing.T) {
	skipDialectRerun(t)
	f := newTestDirectPathFallback(t)
	dials := 0
	slot := newChannelSlot(1, &closeCountingPool{}, f, func(context.Context) (gtransport.ConnPool, error) {
		dials++
		return &closeCountingPool{}, nil
	})
	forceDirectPathFallback(t, f)
	slot.Close()
	if _, err := slot.cloudConn(context.Background()); !errors.Is(err, errChannelSlotClosed) {
		t.Fatalf("cloudConn() after Close error = %v, want %v", err, errChannelSlotClosed)
	}
	if dials != 0 {
		t.Fatalf("dials after Close = %d, want 0", dials)
	}
}

func TestChannelSlotWithoutFallbackOnlyUsesDirect(t *testing.T) {
	skipDialectRerun(t)
	direct := newPathTestServer(t)
	slot := newChannelSlot(3, dialTestPath(t, direct.opts), nil, nil)
	t.Cleanup(func() { slot.Close() })
	client := spannerpb.NewSpannerClient(slot)
	if _, err := client.CreateSession(context.Background(), &spannerpb.CreateSessionRequest{Database: "projects/p/instances/i/databases/d"}); err != nil {
		t.Fatal(err)
	}
	if got, want := direct.callCount(createSessionMethod), 1; got != want {
		t.Fatalf("CreateSession calls = %d, want %d", got, want)
	}
}

// blockingCloudDial is a CloudPath dial that blocks until it is released or
// its context ends, like a dial with grpc.WithBlock to an unreachable endpoint.
type blockingCloudDial struct {
	release chan struct{}
	dials   atomic.Int32
	conns   []*closeCountingPool
	mu      sync.Mutex
}

func newBlockingCloudDial() *blockingCloudDial {
	return &blockingCloudDial{release: make(chan struct{})}
}

func (d *blockingCloudDial) dial(ctx context.Context) (gtransport.ConnPool, error) {
	d.dials.Add(1)
	select {
	case <-d.release:
		c := &closeCountingPool{}
		d.mu.Lock()
		d.conns = append(d.conns, c)
		d.mu.Unlock()
		return c, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func cloudConnWithin(t *testing.T, slot *channelSlot, timeout time.Duration) (time.Duration, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	_, err := slot.cloudConn(ctx)
	return time.Since(start), err
}

func TestChannelSlotCloudDialHonorsEachCallersContext(t *testing.T) {
	skipDialectRerun(t)
	f := newTestDirectPathFallback(t)
	dial := newBlockingCloudDial()
	slot := newChannelSlot(1, &closeCountingPool{}, f, dial.dial)
	t.Cleanup(func() { slot.Close() })
	forceDirectPathFallback(t, f)

	// The attempt that starts the dial and an attempt that waits for it both
	// give up at their own deadlines while the dial keeps running.
	type result struct {
		elapsed time.Duration
		err     error
	}
	initiator := make(chan result, 1)
	go func() {
		elapsed, err := cloudConnWithin(t, slot, 50*time.Millisecond)
		initiator <- result{elapsed, err}
	}()
	waitFor(t, func() error {
		if dial.dials.Load() == 0 {
			return errors.New("dial not started")
		}
		return nil
	})
	elapsed, err := cloudConnWithin(t, slot, 80*time.Millisecond)
	if status.Code(err) != codes.DeadlineExceeded || elapsed > time.Second {
		t.Fatalf("waiting attempt returned %v after %v, want DEADLINE_EXCEEDED at its deadline", err, elapsed)
	}
	r := <-initiator
	if status.Code(r.err) != codes.DeadlineExceeded || r.elapsed > time.Second {
		t.Fatalf("dialing attempt returned %v after %v, want DEADLINE_EXCEEDED at its deadline", r.err, r.elapsed)
	}

	// The dial outlived both attempts; once it succeeds, later attempts share
	// its connection.
	close(dial.release)
	waitFor(t, func() error {
		if _, err := cloudConnWithin(t, slot, time.Second); err != nil {
			return err
		}
		return nil
	})
	if _, err := slot.cloudConn(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := dial.dials.Load(); got != 1 {
		t.Fatalf("CloudPath dials = %d, want 1", got)
	}
}

func TestChannelSlotCloseCancelsCloudDial(t *testing.T) {
	skipDialectRerun(t)
	f := newTestDirectPathFallback(t)
	dial := newBlockingCloudDial()
	direct := &closeCountingPool{}
	slot := newChannelSlot(1, direct, f, dial.dial)
	forceDirectPathFallback(t, f)

	waiter := make(chan error, 1)
	go func() {
		_, err := slot.cloudConn(context.Background())
		waiter <- err
	}()
	waitFor(t, func() error {
		if dial.dials.Load() == 0 {
			return errors.New("dial not started")
		}
		return nil
	})
	if err := slot.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-waiter:
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("waiting attempt error after Close = %v, want UNAVAILABLE", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not end the CloudPath dial")
	}
	if got := direct.closes.Load(); got != 1 {
		t.Fatalf("direct closes = %d, want 1", got)
	}
}

func TestChannelSlotClosesCloudPathDialedAfterClose(t *testing.T) {
	skipDialectRerun(t)
	f := newTestDirectPathFallback(t)
	started := make(chan struct{})
	release := make(chan struct{})
	cloud := &closeCountingPool{}
	slot := newChannelSlot(1, &closeCountingPool{}, f, func(context.Context) (gtransport.ConnPool, error) {
		close(started)
		<-release // a dial that ignores cancellation
		return cloud, nil
	})
	forceDirectPathFallback(t, f)
	waiter := make(chan error, 1)
	go func() {
		_, err := slot.cloudConn(context.Background())
		waiter <- err
	}()
	<-started
	slot.Close()
	close(release)
	if err := <-waiter; !errors.Is(err, errChannelSlotClosed) {
		t.Fatalf("attempt error = %v, want %v", err, errChannelSlotClosed)
	}
	if got := cloud.closes.Load(); got != 1 {
		t.Fatalf("CloudPath connection dialed after Close closed %d times, want 1", got)
	}
}

// connWrapper is a ConnPool over one connection that counts Close calls
// without closing the connection.
type connWrapper struct {
	conn   *grpc.ClientConn
	closes atomic.Int32
}

func (w *connWrapper) Conn() *grpc.ClientConn { return w.conn }
func (w *connWrapper) Num() int               { return 1 }
func (w *connWrapper) Close() error {
	w.closes.Add(1)
	return nil
}
func (w *connWrapper) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...grpc.CallOption) error {
	return w.conn.Invoke(ctx, method, args, reply, opts...)
}
func (w *connWrapper) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return w.conn.NewStream(ctx, desc, method, opts...)
}

// TestChannelSlotWithoutCloudPathKeepsDirectSide covers a client with a
// caller-supplied pool: its CloudPath dial reports that there is no separate
// CloudPath, so after a switch the slot keeps sending on its direct
// connection and closes it once.
func TestChannelSlotWithoutCloudPathKeepsDirectSide(t *testing.T) {
	skipDialectRerun(t)
	server := newPathTestServer(t)
	conn, err := gtransport.Dial(context.Background(), server.opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	f := newTestDirectPathFallback(t)
	direct := &connWrapper{conn: conn}
	slot := newChannelSlot(1, direct, f, func(context.Context) (gtransport.ConnPool, error) { return nil, errNoCloudPath })
	forceDirectPathFallback(t, f)
	if _, err := spannerpb.NewSpannerClient(slot).CreateSession(context.Background(), &spannerpb.CreateSessionRequest{Database: "projects/p/instances/i/databases/d"}); err != nil {
		t.Fatal(err)
	}
	if got, want := server.callCount(createSessionMethod), 1; got != want {
		t.Fatalf("CreateSession calls on the direct connection = %d, want %d", got, want)
	}
	if err := slot.Close(); err != nil {
		t.Fatal(err)
	}
	if got := direct.closes.Load(); got != 1 {
		t.Fatalf("direct closes = %d, want 1", got)
	}
}

// TestChannelSlotRecordsLocalSendMsgFailure fails a stream's SendMsg in the
// client (the request exceeds the send limit). Generated code returns that
// error without calling RecvMsg; the outcome must still be counted once, and
// as a non-path failure.
func TestChannelSlotRecordsLocalSendMsgFailure(t *testing.T) {
	skipDialectRerun(t)
	st := newSlotTest(t)
	session := st.createSession(t)
	st.fallback.closeWindow(time.Now())
	_, err := st.client.ExecuteStreamingSql(context.Background(), &spannerpb.ExecuteSqlRequest{
		Session: session.Name,
		Sql:     SelectFooFromBar,
	}, grpc.MaxCallSendMsgSize(1))
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("ExecuteStreamingSql() error = %v, want RESOURCE_EXHAUSTED", err)
	}
	if res := st.fallback.closeWindow(time.Now()); res.failures != 0 || res.successes != 1 {
		t.Fatalf("window = %d failures and %d successes, want 0 and 1", res.failures, res.successes)
	}
}

// TestChannelSlotIgnoresStreamStoppedByCaller stops a query stream after its
// first message, the way RowIterator.Stop does: the cancelled stream must not
// count as a DirectPath success.
func TestChannelSlotIgnoresStreamStoppedByCaller(t *testing.T) {
	skipDialectRerun(t)
	st := newSlotTest(t)
	session := st.createSession(t)
	st.fallback.closeWindow(time.Now())
	// Keep the stream open after its first message.
	st.direct.TestSpanner.AddPartialResultSetError(SelectFooFromBar, PartialResultSetExecutionTime{
		ResumeToken:   EncodeResumeToken(2),
		ExecutionTime: 5 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := st.client.ExecuteStreamingSql(ctx, &spannerpb.ExecuteSqlRequest{Session: session.Name, Sql: SelectFooFromBar})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := stream.Recv(); status.Code(err) != codes.Canceled {
		t.Fatalf("Recv() after cancel error = %v, want CANCELED", err)
	}
	if res := st.fallback.closeWindow(time.Now()); res.failures != 0 || res.successes != 0 {
		t.Fatalf("window after a stopped stream = %d failures and %d successes, want none", res.failures, res.successes)
	}
}

// TestChannelSlotDoesNotRecordLocalErrors fails an attempt before it leaves
// the client: a CloudPath dial error is not a CloudPath call status.
func TestChannelSlotDoesNotRecordLocalErrors(t *testing.T) {
	skipDialectRerun(t)
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { provider.Shutdown(context.Background()) })
	f, err := newDirectPathFallback(provider, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.close)
	slot := newChannelSlot(1, &closeCountingPool{}, f, func(context.Context) (gtransport.ConnPool, error) {
		return nil, errors.New("dial failed")
	})
	forceDirectPathFallback(t, f)
	if err := slot.Invoke(context.Background(), createSessionMethod, &spannerpb.CreateSessionRequest{}, &spannerpb.Session{}); status.Code(err) != codes.Unavailable {
		t.Fatalf("Invoke() error = %v, want UNAVAILABLE", err)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	if _, ok := findTestMetric(rm, metricNameEEFCallStatus); ok {
		t.Fatal("local dial error reported in eef.call_status")
	}
}
