// Copyright 2014 Google Inc. All Rights Reserved.
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
	"fmt"
	"math/rand"
	"reflect"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/context"

	"cloud.google.com/go/internal/testutil"
	"google.golang.org/api/option"
)

const timeout = time.Minute * 10
const ackDeadline = time.Second * 10

const nMessages = 1e4

// TestEndToEnd pumps many messages into a topic and tests that they are all
// delivered to each subscription for the topic. It also tests that messages
// are not unexpectedly redelivered.
func TestEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	ctx := context.Background()
	ts := testutil.TokenSource(ctx, ScopePubSub, ScopeCloudPlatform)
	if ts == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}

	now := time.Now()
	topicName := fmt.Sprintf("endtoend-%d", now.Unix())
	subPrefix := fmt.Sprintf("endtoend-%d", now.Unix())

	client, err := NewClient(ctx, testutil.ProjID(), option.WithTokenSource(ts))
	if err != nil {
		t.Fatalf("Creating client error: %v", err)
	}

	var topic *Topic
	if topic, err = client.CreateTopic(ctx, topicName); err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)

	// Two subscriptions to the same topic.
	var subs [2]*Subscription
	for i := 0; i < len(subs); i++ {
		subs[i], err = client.CreateSubscription(ctx, fmt.Sprintf("%s-%d", subPrefix, i), topic, ackDeadline, nil)
		if err != nil {
			t.Fatalf("CreateSub error: %v", err)
		}
		defer subs[i].Delete(ctx)
	}

	ids, err := publish(ctx, topic, nMessages)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	wantCounts := make(map[string]int)
	for _, id := range ids {
		wantCounts[id] = 1
	}

	// recv provides an indication that messages are still arriving.
	recv := make(chan struct{})
	// We have two subscriptions to our topic.
	// Each subscription will get a copy of each published message.
	var wg sync.WaitGroup
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	consumers := []*consumer{
		{counts: make(map[string]int), recv: recv, durations: []time.Duration{time.Hour}},
		{counts: make(map[string]int), recv: recv,
			durations: []time.Duration{ackDeadline, ackDeadline, ackDeadline / 2, ackDeadline / 2}},
	}
	for i, con := range consumers {
		con := con
		sub := subs[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			con.consume(t, cctx, sub)
		}()
	}
	timeoutC := time.After(timeout)
	// Every time this ticker ticks, we will check if we have received any
	// messages since the last time it ticked.  We check less frequently
	// than the ack deadline, so that we can detect if messages are
	// redelivered after having their ack deadline extended.
	checkQuiescence := time.NewTicker(ackDeadline * 3)
	defer checkQuiescence.Stop()

	var received bool
loop:
	for {
		select {
		case <-recv:
			received = true
		case <-checkQuiescence.C:
			if received {
				received = false
			} else {
				cancel()
				break loop
			}
		case <-timeoutC:
			t.Errorf("timed out")
			cancel()
			return
		}
	}

	wg.Wait()
	for i, con := range consumers {
		if got, want := con.counts, wantCounts; !reflect.DeepEqual(got, want) {
			t.Errorf("%d: message counts: %v\n", i, diff(got, want))
		}
	}
}

// publish publishes n messages to topic, and returns the published message IDs.
func publish(ctx context.Context, topic *Topic, n int) ([]string, error) {
	var rs []*PublishResult
	for i := 0; i < n; i++ {
		m := &Message{Data: []byte(fmt.Sprintf("msg %d", i))}
		rs = append(rs, topic.Publish(ctx, m))
	}
	var ids []string
	for _, r := range rs {
		id, err := r.Get(ctx)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// consumer consumes messages according to its configuration.
type consumer struct {
	durations []time.Duration

	// A value is sent to recv each time Inc is called.
	recv chan struct{}

	mu     sync.Mutex
	counts map[string]int
}

// consume reads messages from a subscription, and keeps track of what it receives in mc.
// After consume returns, the caller should wait on wg to ensure that no more updates to mc will be made.
func (c *consumer) consume(t *testing.T, ctx context.Context, sub *Subscription) {
	for _, dur := range c.durations {
		ctx2, cancel := context.WithCancel(ctx)
		defer cancel()
		go func() {
			select {
			case <-time.After(dur):
				cancel()
			case <-ctx2.Done():
			}
		}()
		err := sub.Receive(ctx2, c.process)
		if err != nil {
			t.Errorf("error from Receive: %v", err)
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

// process handles a message and records it in mc.
func (c *consumer) process(_ context.Context, m *Message) {
	c.mu.Lock()
	c.counts[m.ID] += 1
	c.mu.Unlock()
	c.recv <- struct{}{}
	// Simulate time taken to process m, while continuing to process more messages.
	go func() {
		// Some messages will need to have their ack deadline extended due to this delay.
		delay := rand.Intn(int(ackDeadline * 3))
		time.After(time.Duration(delay))
		m.Ack()
	}()
}

// diff returns counts of the differences between got and want.
func diff(got, want map[string]int) map[string]int {
	ids := make(map[string]struct{})
	for k := range got {
		ids[k] = struct{}{}
	}
	for k := range want {
		ids[k] = struct{}{}
	}

	gotWantCount := make(map[string]int)
	for k := range ids {
		if got[k] == want[k] {
			continue
		}
		desc := fmt.Sprintf("<got: %v ; want: %v>", got[k], want[k])
		gotWantCount[desc] += 1
	}
	return gotWantCount
}
