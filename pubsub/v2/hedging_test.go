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
	"errors"
	"fmt"
	"strings"
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
	"google.golang.org/grpc/status"
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
	p.hedgingTokenBucket = 5 * tokenScaleFactor

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

func TestPublishHedging(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	topic := fmt.Sprintf("projects/%s/topics/test-topic-hedging", testutil.ProjID())
	publisher := mustCreateTopic(t, c, topic)
	defer publisher.Stop()

	publisher.PublishSettings.HedgingSettings = &HedgingSettings{
		Delay: 100 * time.Millisecond,
	}
	publisher.hedgingTokenBucket = tokenScaleFactor

	srv.SetAutoPublishResponse(false)
	for i := 0; i < 10; i++ {
		addSingleResponse(srv, "msg-123")
	}

	res := publishSingleMessage(ctx, publisher, "test data")
	id, err := res.Get(ctx)
	if err != nil {
		t.Fatalf("res.Get got err: %v", err)
	}
	if id != "msg-123" {
		t.Errorf("got msg ID %q, want msg-123", id)
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

func TestPublishDynamicMultiHedging(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	topic := fmt.Sprintf("projects/%s/topics/test-topic-multi-hedging", testutil.ProjID())
	publisher := mustCreateTopic(t, c, topic)
	defer publisher.Stop()

	publisher.PublishSettings.HedgingSettings = &HedgingSettings{
		Delay: 100 * time.Millisecond,
	}
	publisher.hedgingTokenBucket = 5 * tokenScaleFactor

	srv.SetAutoPublishResponse(false)
	for i := 0; i < 10; i++ {
		addSingleResponse(srv, "msg-multi-123")
	}

	res := publishSingleMessage(ctx, publisher, "test data")
	id, err := res.Get(ctx)
	if err != nil {
		t.Fatalf("res.Get got err: %v", err)
	}
	if id != "msg-multi-123" {
		t.Errorf("got msg ID %q, want msg-multi-123", id)
	}
}

func TestValidateHedgingSettings(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	topic := "projects/proj-id/topics/test-topic-hedging-validation"
	basePub := mustCreateTopic(t, c, topic)
	basePub.Stop()

	tests := []struct {
		name     string
		settings *HedgingSettings
		wantErr  bool
	}{
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
			pub := c.Publisher(topic)
			defer pub.Stop()
			pub.PublishSettings.HedgingSettings = tc.settings

			res := pub.Publish(ctx, &Message{Data: []byte("test")})
			_, err := res.Get(ctx)
			if tc.wantErr && err == nil {
				t.Errorf("expected validation error for %+v, got nil", tc.settings)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for %+v: %v", tc.settings, err)
			}
		})
	}
}

func TestPublishHedging_TokenBucketStartsEmptyAndFixedPointScaling(t *testing.T) {
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	topic := "projects/proj-id/topics/test-topic-hedging-bucket"
	pub := mustCreateTopic(t, c, topic)
	defer pub.Stop()

	if pub.hedgingTokenBucket != 0 {
		t.Fatalf("expected initial hedgingTokenBucket to be 0 (empty), got %d", pub.hedgingTokenBucket)
	}

	pub.PublishSettings.HedgingSettings = &HedgingSettings{
		Delay:       100 * time.Millisecond,
		MaxTokens:   50,
		RefillRatio: 0.1,
	}

	// Replenishing 10 times with ratio 0.1 must reach exactly 1 full token (1000 milli-tokens)
	// without IEEE-754 float64 accumulation drift (where 0.1 * 10 == 0.9999999999999999 < 1.0).
	for i := 0; i < 10; i++ {
		pub.replenishHedgingTokens()
	}
	if pub.hedgingTokenBucket != tokenScaleFactor {
		t.Errorf("expected hedgingTokenBucket after 10 replenishes at 0.1 ratio to be %d, got %d", tokenScaleFactor, pub.hedgingTokenBucket)
	}
}

func TestPublishHedging_DiscardsAllHedgedErrors(t *testing.T) {
	ctx := context.Background()
	srv := pstest.NewServer()
	defer srv.Close()

	var attemptCount int32
	c, err := NewClient(ctx, "proj-id",
		option.WithEndpoint(srv.Addr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			if method == "/google.pubsub.v1.Publisher/Publish" {
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
		})),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	topic := "projects/proj-id/topics/test-topic-discard-hedged-err"
	pub := mustCreateTopic(t, c, topic)
	defer pub.Stop()

	pub.PublishSettings.HedgingSettings = &HedgingSettings{
		Delay:       100 * time.Millisecond,
		MaxTokens:   50,
		RefillRatio: 0.1,
	}
	pub.hedgingTokenBucket = tokenScaleFactor

	res := pub.Publish(ctx, &Message{Data: []byte("hello")})
	id, err := res.Get(ctx)
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
