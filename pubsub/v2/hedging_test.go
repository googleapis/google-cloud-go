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

package pubsub

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	pb "cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"cloud.google.com/go/pubsub/v2/pstest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestCancellationSharer(t *testing.T) {
	ctx := context.Background()
	mainCtx, mainCancel := context.WithCancel(ctx)
	hedgedCtx, hedgedCancel := context.WithCancel(ctx)

	cs := newCancellationSharer(nil)
	mainID := cs.add(mainCancel)
	_ = cs.add(hedgedCancel)

	// Test that winning main cancels hedged
	cs.win(mainID)
	if hedgedCtx.Err() == nil {
		t.Errorf("expected hedgedCtx to be cancelled when win(mainID) is called")
	}
	if mainCtx.Err() != nil {
		t.Errorf("expected mainCtx to not be cancelled yet")
	}

	cs.cancelAll()
	if mainCtx.Err() == nil {
		t.Errorf("expected mainCtx to be cancelled after cancelAll()")
	}

	// Test adding after done returns -1 and cancels immediately
	lateCtx, lateCancel := context.WithCancel(ctx)
	lateID := cs.add(lateCancel)
	if lateID != -1 {
		t.Errorf("expected lateID to be -1, got %d", lateID)
	}
	if lateCtx.Err() == nil {
		t.Errorf("expected lateCtx to be cancelled immediately upon add after done")
	}
}

func TestCancellationSharer_ReleasesBatchAndWaitsForHedges(t *testing.T) {
	batch := &hedgeBatch{ctx: context.Background(), pbMsgs: []*pb.PubsubMessage{{Data: []byte("x")}}}
	cs := newCancellationSharer(batch)

	id, hctx, b, ok := cs.acquireHedge()
	if !ok {
		t.Fatal("acquireHedge: got ok=false before the publish resolved")
	}
	if b != batch {
		t.Fatalf("acquireHedge: got batch %p, want %p", b, batch)
	}

	cs.cancelAll()
	if hctx.Err() == nil {
		t.Errorf("hedge %d context not cancelled by cancelAll", id)
	}
	cs.mu.Lock()
	gotBatch := cs.batch
	cs.mu.Unlock()
	if gotBatch != nil {
		t.Error("cancelAll did not release the shared batch payload")
	}
	if _, _, _, ok := cs.acquireHedge(); ok {
		t.Error("acquireHedge after cancelAll: got ok=true, want false")
	}

	// wait must block until the acquired hedge releases.
	waited := make(chan struct{})
	go func() {
		cs.wait()
		close(waited)
	}()
	select {
	case <-waited:
		t.Fatal("wait returned while a hedge was still in flight")
	case <-time.After(50 * time.Millisecond):
	}
	cs.releaseHedge()
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("wait did not return after releaseHedge")
	}
}

func TestCancellationSharer_WinReleasesBatch(t *testing.T) {
	cs := newCancellationSharer(&hedgeBatch{ctx: context.Background()})
	id, _, _, ok := cs.acquireHedge()
	if !ok {
		t.Fatal("acquireHedge: got ok=false")
	}
	cs.win(id)
	cs.mu.Lock()
	gotBatch := cs.batch
	cs.mu.Unlock()
	if gotBatch != nil {
		t.Error("win did not release the shared batch payload")
	}
	cs.releaseHedge()
	cs.cancelAll()
	cs.wait()
}

// newFakeWithInterceptor is like newFake but installs a unary client interceptor.
func newFakeWithInterceptor(t *testing.T, ic grpc.UnaryClientInterceptor) (*Client, *pstest.Server) {
	t.Helper()
	srv := pstest.NewServer()
	client, err := NewClient(context.Background(), projName,
		option.WithEndpoint(srv.Addr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(ic)),
		option.WithTelemetryDisabled(),
	)
	if err != nil {
		srv.Close()
		t.Fatal(err)
	}
	return client, srv
}

// Queued hedgedRequests stay in the hedging queue until their sendAfter time,
// even when the publish resolves immediately. They must not keep the batch
// payload alive after flow control has released it.
func TestPublishHedging_QueueDoesNotRetainPayload(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	topic := fmt.Sprintf("projects/%s/topics/test-topic-hedging-retain", testutil.ProjID())
	p := mustCreateTopic(t, c, topic)
	defer p.Stop()
	p.PublishSettings.HedgingSettings = &HedgingSettings{Delay: maxHedgingDelay}

	if _, err := publishSingleMessage(ctx, p, "payload").Get(ctx); err != nil {
		t.Fatalf("Get: %v", err)
	}

	p.hedgingMu.Lock()
	queue := append([]*hedgedRequest(nil), p.hedgingQueue...)
	p.hedgingMu.Unlock()
	if len(queue) != 1 {
		t.Fatalf("got %d queued hedged requests, want 1 (the not-yet-due initial hedge)", len(queue))
	}
	cs := queue[0].cs
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if !cs.done {
		t.Error("queued request's cancellationSharer is not done after the publish resolved")
	}
	if cs.batch != nil {
		t.Error("queued hedged request still retains the batch payload after the publish resolved")
	}
}

// Users may reuse a message's Data and Attributes once PublishResult.Get
// returns. Hedged attempts that lost the race can still be inside the RPC when
// the winner resolves; the publish must wait for them to exit before setting
// results.
func TestPublishHedging_ResultWaitsForLosingHedges(t *testing.T) {
	ctx := context.Background()
	var (
		publishCalls   int64
		inflightHedges int64
		hedgesStarted  int64
	)
	ic := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if !strings.HasSuffix(method, "/Publish") {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		if atomic.AddInt64(&publishCalls, 1) == 1 {
			// Original attempt: slow enough for hedges to fire, then succeed.
			time.Sleep(300 * time.Millisecond)
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		// Hedged attempt: block until cancelled by the winner, then keep
		// reading the request for a bit to model an RPC that is still
		// serializing when cancellation arrives.
		atomic.AddInt64(&hedgesStarted, 1)
		atomic.AddInt64(&inflightHedges, 1)
		defer atomic.AddInt64(&inflightHedges, -1)
		<-ctx.Done()
		deadline := time.Now().Add(50 * time.Millisecond)
		for time.Now().Before(deadline) {
			for _, m := range req.(*pb.PublishRequest).Messages {
				_ = len(m.Data)
				for k, v := range m.Attributes {
					_, _ = k, v
				}
			}
		}
		return ctx.Err()
	}
	c, srv := newFakeWithInterceptor(t, ic)
	defer c.Close()
	defer srv.Close()

	topic := fmt.Sprintf("projects/%s/topics/test-topic-hedging-wait", testutil.ProjID())
	p := mustCreateTopic(t, c, topic)
	defer p.Stop()
	p.PublishSettings.HedgingSettings = &HedgingSettings{Delay: 100 * time.Millisecond}
	p.hedgingTokenBucket.Store(5 * tokenScaleFactor)

	msg := &Message{Data: []byte("payload"), Attributes: map[string]string{"k": "v"}}
	if _, err := p.Publish(ctx, msg).Get(ctx); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if n := atomic.LoadInt64(&inflightHedges); n != 0 {
		t.Errorf("PublishResult resolved while %d hedged attempt(s) were still in flight", n)
	}
	if atomic.LoadInt64(&hedgesStarted) == 0 {
		t.Fatal("no hedged attempts were sent; test did not exercise the race")
	}

	// Reusing the message after Get must not race with hedged attempts
	// (detected under -race; concurrent map access would also be fatal).
	msg.Attributes["k"] = "reused"
	msg.Data[0] = 'X'
}

// publishOperation decodes the x-goog-pubsub-client-telemetry header attached
// to an outgoing Publish call. It is safe to call from interceptor goroutines.
func publishOperation(t *testing.T, ctx context.Context) *pb.PubsubClientTelemetry_PublishOperation {
	md, _ := metadata.FromOutgoingContext(ctx)
	vals := md.Get(pubsubClientTelemetryHeader)
	if len(vals) != 1 {
		t.Errorf("got %d %s header values, want 1", len(vals), pubsubClientTelemetryHeader)
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(vals[0])
	if err != nil {
		t.Errorf("decoding %s header: %v", pubsubClientTelemetryHeader, err)
		return nil
	}
	var tel pb.PubsubClientTelemetry
	if err := proto.Unmarshal(raw, &tel); err != nil {
		t.Errorf("unmarshalling %s header: %v", pubsubClientTelemetryHeader, err)
		return nil
	}
	if tel.GetPublishOperation() == nil {
		t.Errorf("%s header has no publish_operation", pubsubClientTelemetryHeader)
	}
	return tel.GetPublishOperation()
}

// newHedgingPublisher returns a publisher whose Publish RPCs go through ic,
// with hedging enabled at the minimum delay and the given number of tokens.
func newHedgingPublisher(t *testing.T, ic grpc.UnaryClientInterceptor, tokens int64) *Publisher {
	t.Helper()
	c, srv := newFakeWithInterceptor(t, ic)
	t.Cleanup(func() {
		c.Close()
		srv.Close()
	})
	topic := fmt.Sprintf("projects/%s/topics/%s", testutil.ProjID(), strings.ReplaceAll(t.Name(), "/", "-"))
	p := mustCreateTopic(t, c, topic)
	t.Cleanup(p.Stop)
	p.PublishSettings.HedgingSettings = &HedgingSettings{Delay: minHedgingDelay}
	p.hedgingTokenBucket.Store(tokens * tokenScaleFactor)
	return p
}

func isPublish(method string) bool { return strings.HasSuffix(method, "/Publish") }

// When the original attempt stalls, the hedged attempt's response resolves
// the publish and the original attempt is cancelled. Both attempts carry the
// client telemetry header with their attempt number and a shared start time.
func TestPublishHedging_HedgeWins(t *testing.T) {
	ctx := context.Background()
	var (
		mu         sync.Mutex
		ops        []*pb.PubsubClientTelemetry_PublishOperation
		hedgeIDs   []string
		origCancel = make(chan struct{}, 1)
	)
	ic := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if !isPublish(method) {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		op := publishOperation(t, ctx)
		mu.Lock()
		ops = append(ops, op)
		mu.Unlock()
		if op.GetHedgedAttemptCount() == 0 {
			// Original attempt: stall until the winning hedge cancels it.
			select {
			case <-ctx.Done():
				select {
				case origCancel <- struct{}{}:
				default:
				}
				return ctx.Err()
			case <-time.After(10 * time.Second):
				return status.Error(codes.PermissionDenied, "original attempt was never cancelled")
			}
		}
		if err := invoker(ctx, method, req, reply, cc, opts...); err != nil {
			return err
		}
		mu.Lock()
		hedgeIDs = append(hedgeIDs, reply.(*pb.PublishResponse).MessageIds...)
		mu.Unlock()
		return nil
	}
	p := newHedgingPublisher(t, ic, 1)

	id, err := publishSingleMessage(ctx, p, "payload").Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(hedgeIDs) != 1 || id != hedgeIDs[0] {
		t.Errorf("got message ID %q, want the hedged attempt's ID (hedge returned %v)", id, hedgeIDs)
	}
	select {
	case <-origCancel:
	default:
		t.Error("original attempt was not cancelled after the hedge won")
	}
	if len(ops) != 2 {
		t.Fatalf("got %d Publish attempts, want 2 (original + 1 hedge)", len(ops))
	}
	for i, op := range ops {
		if got := op.GetHedgedAttemptCount(); got != int32(i) {
			t.Errorf("attempt %d: got hedged_attempt_count %d, want %d", i, got, i)
		}
		if op.GetPublishStartTime() == nil {
			t.Errorf("attempt %d: publish_start_time not set", i)
		}
	}
	if !proto.Equal(ops[0].GetPublishStartTime(), ops[1].GetPublishStartTime()) {
		t.Errorf("publish_start_time differs between attempts: %v vs %v", ops[0].GetPublishStartTime(), ops[1].GetPublishStartTime())
	}
}

// While the original attempt is slow, a new hedge is sent every Delay until
// the token bucket runs out; once a due hedge finds no token, the chain stops.
func TestPublishHedging_HedgesUntilTokensRunOut(t *testing.T) {
	ctx := context.Background()
	var (
		mu     sync.Mutex
		counts []int
	)
	ic := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if !isPublish(method) {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		n := int(publishOperation(t, ctx).GetHedgedAttemptCount())
		mu.Lock()
		counts = append(counts, n)
		mu.Unlock()
		if n == 0 {
			// Original attempt outlives several hedge intervals, then succeeds.
			time.Sleep(5 * minHedgingDelay)
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		<-ctx.Done() // hedges stall until the original wins
		return ctx.Err()
	}
	p := newHedgingPublisher(t, ic, 2)

	if _, err := publishSingleMessage(ctx, p, "payload").Get(ctx); err != nil {
		t.Fatalf("Get: %v", err)
	}

	mu.Lock()
	got := append([]int(nil), counts...)
	mu.Unlock()
	sort.Ints(got)
	if want := []int{0, 1, 2}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got hedged_attempt_counts %v, want %v (original + one hedge per token)", got, want)
	}
	p.hedgingMu.Lock()
	defer p.hedgingMu.Unlock()
	if want := int64(tokenScaleFactor / 10); p.hedgingTokenBucket.Load() != want {
		t.Errorf("got %d milli-tokens after publish, want %d (2 spent, then one 0.1 refill)", p.hedgingTokenBucket.Load(), want)
	}
}

// The token bucket starts empty, so the first slow publish is never hedged,
// and a successful publish refills it.
func TestPublishHedging_NoHedgeWithoutTokens(t *testing.T) {
	ctx := context.Background()
	var calls int64
	ic := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if isPublish(method) {
			atomic.AddInt64(&calls, 1)
			time.Sleep(3 * minHedgingDelay)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
	p := newHedgingPublisher(t, ic, 0)

	if _, err := publishSingleMessage(ctx, p, "payload").Get(ctx); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Errorf("got %d Publish attempts with an empty token bucket, want 1", got)
	}
	p.hedgingMu.Lock()
	defer p.hedgingMu.Unlock()
	if want := int64(tokenScaleFactor / 10); p.hedgingTokenBucket.Load() != want {
		t.Errorf("got %d milli-tokens after a successful publish, want %d", p.hedgingTokenBucket.Load(), want)
	}
}

func TestPublishHedgingWithOrdering(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	topic := fmt.Sprintf("projects/%s/topics/test-topic-hedging-ordering", testutil.ProjID())
	publisher := mustCreateTopic(t, c, topic)
	defer publisher.Stop()

	publisher.EnableMessageOrdering = true
	publisher.PublishSettings.HedgingSettings = &HedgingSettings{
		Delay: 100 * time.Millisecond,
	}

	res := publishSingleMessageWithKey(ctx, publisher, "test", "key")
	if _, err := res.Get(ctx); !errors.Is(err, errPublisherHedgingAndOrderingEnabled) {
		t.Errorf("got %v, want errPublisherHedgingAndOrderingEnabled", err)
	}
}

func TestValidateHedgingSettings(t *testing.T) {
	tests := []struct {
		name     string
		settings *HedgingSettings
		wantErr  bool
	}{
		{
			name:     "nil (hedging disabled)",
			settings: nil,
			wantErr:  false,
		},
		{
			name:     "defaults (all zero)",
			settings: &HedgingSettings{},
			wantErr:  false,
		},
		{
			name: "valid boundary min",
			settings: &HedgingSettings{
				Delay:       100 * time.Millisecond,
				MaxTokens:   1,
				RefillRatio: 0.001,
			},
			wantErr: false,
		},
		{
			name: "valid boundary max",
			settings: &HedgingSettings{
				Delay:       10 * time.Second,
				MaxTokens:   250,
				RefillRatio: 0.2,
			},
			wantErr: false,
		},
		{
			name:     "delay too low (< 100ms)",
			settings: &HedgingSettings{Delay: 50 * time.Millisecond},
			wantErr:  true,
		},
		{
			name:     "delay too high (> 10s)",
			settings: &HedgingSettings{Delay: 11 * time.Second},
			wantErr:  true,
		},
		{
			name:     "maxTokens negative",
			settings: &HedgingSettings{MaxTokens: -1},
			wantErr:  true,
		},
		{
			name:     "maxTokens too high (> 250)",
			settings: &HedgingSettings{MaxTokens: 251},
			wantErr:  true,
		},
		{
			name:     "refillRatio too low (< 0.001)",
			settings: &HedgingSettings{RefillRatio: 0.0005},
			wantErr:  true,
		},
		{
			name:     "refillRatio too high (> 0.2)",
			settings: &HedgingSettings{RefillRatio: 0.25},
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateHedgingSettings(tc.settings)
			if gotErr := err != nil; gotErr != tc.wantErr {
				t.Errorf("validateHedgingSettings(%+v) = %v, wantErr %t", tc.settings, err, tc.wantErr)
			}
		})
	}
}

// Invalid HedgingSettings fail the PublishResult rather than being ignored.
func TestPublishHedging_InvalidSettingsFailPublish(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	p := mustCreateTopic(t, c, "projects/proj-id/topics/test-topic-hedging-invalid")
	defer p.Stop()
	p.PublishSettings.HedgingSettings = &HedgingSettings{Delay: minHedgingDelay - time.Millisecond}

	if _, err := publishSingleMessage(ctx, p, "payload").Get(ctx); err == nil || !strings.Contains(err.Error(), "HedgingSettings.Delay") {
		t.Errorf("got err %v, want a HedgingSettings.Delay validation error", err)
	}
}

func TestPublishHedging_TokenBucket(t *testing.T) {
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	topic := "projects/proj-id/topics/test-topic-hedging-bucket"
	pub := mustCreateTopic(t, c, topic)
	defer pub.Stop()

	if pub.hedgingTokenBucket.Load() != 0 {
		t.Fatalf("expected initial hedgingTokenBucket to be 0 (empty), got %d", pub.hedgingTokenBucket.Load())
	}

	const maxTokens = 50
	pub.PublishSettings.HedgingSettings = &HedgingSettings{
		Delay:       100 * time.Millisecond,
		MaxTokens:   maxTokens,
		RefillRatio: 0.1,
	}
	pub.initHedging()
	// Settings are snapshotted by initHedging; later changes have no effect.
	pub.PublishSettings.HedgingSettings.MaxTokens = 1
	pub.PublishSettings.HedgingSettings.RefillRatio = 0.2

	// Replenishing 10 times with ratio 0.1 must reach exactly 1 full token (1000 milli-tokens)
	// without IEEE-754 float64 accumulation drift (where 0.1 * 10 == 0.9999999999999999 < 1.0).
	for i := 0; i < 10; i++ {
		pub.replenishHedgingTokens()
	}
	if pub.hedgingTokenBucket.Load() != tokenScaleFactor {
		t.Errorf("expected hedgingTokenBucket after 10 replenishes at 0.1 ratio to be %d, got %d", tokenScaleFactor, pub.hedgingTokenBucket.Load())
	}

	// Refills are capped at MaxTokens, including a partial refill that would overshoot.
	maxMilli := int64(maxTokens) * tokenScaleFactor
	pub.hedgingTokenBucket.Store(maxMilli - tokenScaleFactor/20)
	for i := 0; i < 2; i++ {
		pub.replenishHedgingTokens()
		if pub.hedgingTokenBucket.Load() != maxMilli {
			t.Errorf("replenish %d near the cap: got %d milli-tokens, want %d (MaxTokens)", i, pub.hedgingTokenBucket.Load(), maxMilli)
		}
	}

	// Acquiring takes exactly one whole token and fails below one token.
	pub.hedgingTokenBucket.Store(tokenScaleFactor + tokenScaleFactor/2)
	if !pub.tryAcquireHedgingToken() {
		t.Error("tryAcquireHedgingToken with 1.5 tokens: got false, want true")
	}
	if pub.tryAcquireHedgingToken() {
		t.Error("tryAcquireHedgingToken with 0.5 tokens: got true, want false")
	}
	if got, want := pub.hedgingTokenBucket.Load(), tokenScaleFactor/2; got != want {
		t.Errorf("got %d milli-tokens after acquiring, want %d", got, want)
	}
}

// Concurrent refills and acquisitions must not lose updates or exceed the cap.
func TestPublishHedging_TokenBucketConcurrent(t *testing.T) {
	p := &Publisher{hedgingMaxMilliTokens: 1000 * tokenScaleFactor, hedgingRefillMilliTokens: tokenScaleFactor}
	const n = 500
	p.hedgingTokenBucket.Store(n * tokenScaleFactor)

	var wg sync.WaitGroup
	var acquired atomic.Int64
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			p.replenishHedgingTokens()
		}()
		go func() {
			defer wg.Done()
			if p.tryAcquireHedgingToken() {
				acquired.Add(1)
			}
		}()
	}
	wg.Wait()

	// Starting at n tokens, n acquisitions always succeed, and n refills of
	// one token each land below the cap, so the bucket ends where it started.
	if got := acquired.Load(); got != n {
		t.Errorf("got %d successful acquisitions, want %d", got, n)
	}
	if got, want := p.hedgingTokenBucket.Load(), int64(n*tokenScaleFactor); got != want {
		t.Errorf("got %d milli-tokens, want %d", got, want)
	}
}

func TestPublishHedging_DiscardsAllHedgedErrors(t *testing.T) {
	ctx := context.Background()
	var attemptCount int32
	ic := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if isPublish(method) {
			n := atomic.AddInt32(&attemptCount, 1)
			if n == 1 {
				// Delay the original attempt so the hedged attempt fires and finishes first.
				time.Sleep(180 * time.Millisecond)
				return invoker(ctx, method, req, reply, cc, opts...)
			}
			// Hedged attempt fails with a permanent error (PermissionDenied); it must be discarded.
			return status.Error(codes.PermissionDenied, "hedged permanent error should be discarded")
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
	pub := newHedgingPublisher(t, ic, 1)

	id, err := pub.Publish(ctx, &Message{Data: []byte("hello")}).Get(ctx)
	if err != nil {
		t.Fatalf("expected original publish attempt to succeed after discarding hedged PermissionDenied error, got err: %v", err)
	}
	if id == "" {
		t.Errorf("expected non-empty message ID")
	}
	if got := atomic.LoadInt32(&attemptCount); got < 2 {
		t.Errorf("expected at least 2 RPC attempts (1 original + 1 hedged), got %d", got)
	}
}
