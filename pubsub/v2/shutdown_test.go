// Copyright 2025 Google LLC
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
	"sync"
	"testing"
	"time"

	pb "cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
)

func TestShutdown_NackImmediately(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, srv := newFake(t)
	defer client.Close()
	defer srv.Close()

	topic := mustCreateTopic(t, client, "projects/p/topics/t")
	sub := mustCreateSubConfig(t, client, &pb.Subscription{
		Name:  "projects/p/subscriptions/s",
		Topic: topic.String(),
	})

	// Part of this test: pretend to extend the min duration quite a bit so we can test
	// if the message has been properly nacked.
	sub.ReceiveSettings.MinDurationPerAckExtension = 10 * time.Minute
	sub.ReceiveSettings.ShutdownOptions = &ShutdownOptions{
		Behavior: ShutdownBehaviorNackImmediately,
		Timeout:  1 * time.Minute,
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := topic.Publish(ctx, &Message{Data: []byte("m1")}).Get(ctx)
		if err != nil {
			t.Errorf("Publish().Get() got err: %v", err)
		}
	}()
	wg.Wait()

	cctx, ccancel := context.WithCancel(ctx)
	go sub.Receive(cctx, func(ctx context.Context, m *Message) {
		// First time receiving, cancel the context to trigger shutdown.
		// Don't cancel away to avoid race condition with fake.
		time.AfterFunc(2*time.Second, ccancel)
	})

	// Wait for the message to be redelivered.
	time.Sleep(5 * time.Second)

	var received int
	var receiveLock sync.Mutex
	ctx2, cancel := context.WithTimeout(ctx, 30*time.Second)
	err := sub.Receive(ctx2, func(ctx context.Context, m *Message) {
		receiveLock.Lock()
		defer receiveLock.Unlock()
		received++
		m.Ack()
		cancel()
	})
	if err != nil {
		t.Errorf("got err from recv: %v", err)
	}
	if received != 1 {
		t.Errorf("expected 1 delivery, got %d", received)
	}
}

func TestShutdown_WaitForProcessing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		shutdownTimeout time.Duration
		expectedTimeout time.Duration
		minTime         time.Duration
	}{
		{
			name:            "BailImmediately",
			shutdownTimeout: 0 * time.Second,
			expectedTimeout: 5 * time.Second,
		},
		{
			name:            "WithTimeout",
			shutdownTimeout: 5 * time.Second,
			expectedTimeout: 6 * time.Second,
			minTime:         4 * time.Second,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client, srv := newFake(t)
			defer client.Close()
			defer srv.Close()

			topic := mustCreateTopic(t, client, "projects/p/topics/t")
			sub := mustCreateSubConfig(t, client, &pb.Subscription{
				Name:  "projects/p/subscriptions/s",
				Topic: topic.String(),
			})
			sub.ReceiveSettings.ShutdownOptions = &ShutdownOptions{
				Behavior: ShutdownBehaviorWaitForProcessing,
				Timeout:  tc.shutdownTimeout,
			}
			processingTime := 1 * time.Hour

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := topic.Publish(ctx, &Message{Data: []byte("m1")}).Get(ctx)
				if err != nil {
					t.Errorf("Publish().Get() got err: %v", err)
				}
			}()
			wg.Wait()

			cctx, cancel2 := context.WithCancel(ctx)
			defer cancel2()
			start := time.Now()
			sub.Receive(cctx, func(ctx context.Context, m *Message) {
				cancel()
				// Simulate a long processing message that we want to cancel right away.
				// The message should never be acked since we expect the client to bail early.
				time.Sleep(processingTime)
				m.Ack()
			})

			elapsed := time.Since(start)
			if elapsed > tc.expectedTimeout {
				t.Errorf("expected quick cancellation, elapsed: %v, want less than: %v", elapsed, tc.expectedTimeout)
			}
			if tc.minTime > 0 && elapsed < tc.minTime {
				t.Errorf("expected to wait for shutdown, elapsed: %v, want greater than: %v", elapsed, tc.minTime)
			}
		})
	}
}
