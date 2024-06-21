// Copyright 2017 Google LLC
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
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ipubsub "cloud.google.com/go/internal/pubsub"
	"cloud.google.com/go/internal/testutil"
	pb "cloud.google.com/go/pubsub/apiv1/pubsubpb"
	"cloud.google.com/go/pubsub/pstest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

var (
	projName                = "P"
	topicName               = "some-topic"
	subName                 = "some-sub"
	fullyQualifiedTopicName = fmt.Sprintf("projects/%s/topics/%s", projName, topicName)
	fullyQualifiedSubName   = fmt.Sprintf("projects/%s/subscriptions/%s", projName, subName)
)

func TestMakeBatches(t *testing.T) {
	t.Parallel()
	ids := []string{"a", "b", "c", "d", "e"}
	for i, test := range []struct {
		ids  []string
		want [][]string
	}{
		{[]string{}, [][]string{}},                       // empty slice
		{ids, [][]string{{"a", "b"}, {"c", "d"}, {"e"}}}, // slice of size 5
		{ids[:3], [][]string{{"a", "b"}, {"c"}}},         // slice of size 3
		{ids[:1], [][]string{{"a"}}},                     // slice of size 1
	} {
		got := makeBatches(test.ids, 2)
		want := test.want
		if !testutil.Equal(len(got), len(want)) {
			t.Errorf("test %d: %v, got %v, want %v", i, test, got, want)
		}
	}
}

func TestCalcFieldSize(t *testing.T) {
	t.Parallel()
	// Create a mock ack request to test.
	req := &pb.AcknowledgeRequest{
		Subscription: "sub",
		AckIds:       []string{"aaa", "bbb", "ccc", "ddd", "eee"},
	}
	size := calcFieldSizeString(req.Subscription) + calcFieldSizeString(req.AckIds...)

	// Proto encoding is calculated from 1 tag byte and 1 size byte for each string.
	want := (1 + 1) + len(req.Subscription) + // subscription field: 1 tag byte + 1 size byte
		5*(1+1+3) // ackID size: 5 * [1 (tag byte) + 1 (size byte) + 3 (length of ackID)]
	if size != want {
		t.Errorf("pubsub: calculated ack req size of %d bytes, want %d", size, want)
	}

	req.Subscription = string(bytes.Repeat([]byte{'A'}, 300))
	size = calcFieldSizeString(req.Subscription) + calcFieldSizeString(req.AckIds...)

	// With a longer subscription name, we use an extra size byte.
	want = (1 + 2) + len(req.Subscription) + // subscription field: 1 tag byte + 2 size bytes
		5*(1+1+3) // ackID size: 5 * [1 (tag byte) + 1 (size byte) + 3 (length of ackID)]
	if size != want {
		t.Errorf("pubsub: calculated ack req size of %d bytes, want %d", size, want)
	}

	// Create a mock modack request to test.
	modAckReq := &pb.ModifyAckDeadlineRequest{
		Subscription:       "sub",
		AckIds:             []string{"aaa", "bbb", "ccc", "ddd", "eee"},
		AckDeadlineSeconds: 300,
	}

	size = calcFieldSizeString(modAckReq.Subscription) +
		calcFieldSizeString(modAckReq.AckIds...) +
		calcFieldSizeInt(int(modAckReq.AckDeadlineSeconds))

	want = (1 + 1) + len(modAckReq.Subscription) + // subscription field: 1 tag byte + 1 size byte
		5*(1+1+3) + // ackID size: 5 * [1 (tag byte) + 1 (size byte) + 3 (length of ackID)]
		(1 + 2) // ackDeadline: 1 tag byte + 2 size bytes
	if size != want {
		t.Errorf("pubsub: calculated modAck req size of %d bytes, want %d", size, want)
	}
}

func TestMaxExtensionPeriod(t *testing.T) {
	srv := pstest.NewServer()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv.Publish(fullyQualifiedTopicName, []byte("creating a topic"), nil)

	_, client, err := initConn(ctx, srv.Addr)
	if err != nil {
		t.Fatal(err)
	}
	want := 15 * time.Second
	iter := newMessageIterator(client.subc, fullyQualifiedTopicName, &pullOptions{
		maxExtensionPeriod: want,
	})

	// Add a datapoint that's greater than maxExtensionPeriod.
	receiveTime := time.Now().Add(time.Duration(-20) * time.Second)
	iter.ackTimeDist.Record(int(time.Since(receiveTime) / time.Second))

	if got := iter.ackDeadline(); got != want {
		t.Fatalf("deadline got = %v, want %v", got, want)
	}
}

func TestAckDistribution(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Skip("broken")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	minDurationPerLeaseExtension = 1 * time.Second
	pstest.SetMinAckDeadline(minDurationPerLeaseExtension)
	srv := pstest.NewServer()
	defer srv.Close()
	defer pstest.ResetMinAckDeadline()

	// Create the topic via a Publish. It's convenient to do it here as opposed to client.CreateTopic because the client
	// has not been established yet, and also because we want to create the topic once whereas the client is established
	// below twice.
	srv.Publish(fullyQualifiedTopicName, []byte("creating a topic"), nil)

	queuedMsgs := make(chan int32, 1024)
	go continuouslySend(ctx, srv, queuedMsgs)

	for _, testcase := range []struct {
		initialProcessSecs int32
		finalProcessSecs   int32
	}{
		{initialProcessSecs: 3, finalProcessSecs: 5}, // Process time goes up
		{initialProcessSecs: 5, finalProcessSecs: 3}, // Process time goes down
	} {
		t.Logf("Testing %d -> %d", testcase.initialProcessSecs, testcase.finalProcessSecs)

		// processTimeSecs is used by the sender to coordinate with the receiver how long the receiver should
		// pretend to process for. e.g. if we test 3s -> 5s, processTimeSecs will start at 3, causing receiver
		// to process messages received for 3s while sender sends the first batch. Then, as sender begins to
		// send the next batch, sender will swap processTimeSeconds to 5s and begin sending, and receiver will
		// process each message for 5s. In this way we simulate a client whose time-to-ack (process time) changes.
		processTimeSecs := testcase.initialProcessSecs

		s, client, err := initConn(ctx, srv.Addr)
		if err != nil {
			t.Fatal(err)
		}

		// recvdWg increments for each message sent, and decrements for each message received.
		recvdWg := &sync.WaitGroup{}

		go startReceiving(ctx, t, s, recvdWg, &processTimeSecs)
		startSending(t, queuedMsgs, &processTimeSecs, testcase.initialProcessSecs, testcase.finalProcessSecs, recvdWg)

		recvdWg.Wait()
		time.Sleep(100 * time.Millisecond) // Wait a bit more for resources to clean up
		err = client.Close()
		if err != nil {
			t.Fatal(err)
		}

		modacks := modacksByTime(srv.Messages())
		u := modackDeadlines(modacks)
		initialDL := int32(minDurationPerLeaseExtension / time.Second)
		if !setsAreEqual(u, []int32{initialDL, testcase.initialProcessSecs, testcase.finalProcessSecs}) {
			t.Fatalf("Expected modack deadlines to contain (exactly, and only) %ds, %ds, %ds. Instead, got %v",
				initialDL, testcase.initialProcessSecs, testcase.finalProcessSecs, toSet(u))
		}
	}
}

// modacksByTime buckets modacks by time.
func modacksByTime(msgs []*pstest.Message) map[time.Time][]pstest.Modack {
	modacks := map[time.Time][]pstest.Modack{}

	for _, msg := range msgs {
		for _, m := range msg.Modacks {
			modacks[m.ReceivedAt] = append(modacks[m.ReceivedAt], m)
		}
	}
	return modacks
}

// setsAreEqual reports whether a and b contain the same values, ignoring duplicates.
func setsAreEqual(haystack, needles []int32) bool {
	hMap := map[int32]bool{}
	nMap := map[int32]bool{}

	for _, n := range needles {
		nMap[n] = true
	}

	for _, n := range haystack {
		hMap[n] = true
	}

	return reflect.DeepEqual(nMap, hMap)
}

// startReceiving pretends to be a client. It calls s.Receive and acks messages after some random delay. It also
// looks out for dupes - any message that arrives twice will cause a failure.
func startReceiving(ctx context.Context, t *testing.T, s *Subscription, recvdWg *sync.WaitGroup, processTimeSecs *int32) {
	t.Log("Receiving..")

	var recvdMu sync.Mutex
	recvd := map[string]bool{}

	err := s.Receive(ctx, func(ctx context.Context, msg *Message) {
		msgData := string(msg.Data)
		recvdMu.Lock()
		_, ok := recvd[msgData]
		if ok {
			recvdMu.Unlock()
			t.Logf("already saw \"%s\"\n", msgData)
			return
		}
		recvd[msgData] = true
		recvdMu.Unlock()

		select {
		case <-ctx.Done():
			msg.Nack()
			recvdWg.Done()
		case <-time.After(time.Duration(atomic.LoadInt32(processTimeSecs)) * time.Second):
			msg.Ack()
			recvdWg.Done()
		}
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Error(err)
	}
}

// startSending sends four batches of messages broken up by minDeadline, initialProcessSecs, and finalProcessSecs.
func startSending(t *testing.T, queuedMsgs chan int32, processTimeSecs *int32, initialProcessSecs int32, finalProcessSecs int32, recvdWg *sync.WaitGroup) {
	var msg int32

	// We must send this block to force the receiver to send its initially-configured modack time. The time that
	// gets sent should be ignorant of the distribution, since there haven't been enough (any, actually) messages
	// to create a distribution yet.
	t.Log("minAckDeadlineSecsSending an initial message")
	recvdWg.Add(1)
	msg++
	queuedMsgs <- msg
	<-time.After(minDurationPerLeaseExtension)

	t.Logf("Sending some messages to update distribution to %d. This new distribution will be used "+
		"when the next batch of messages go out.", initialProcessSecs)
	for i := 0; i < 10; i++ {
		recvdWg.Add(1)
		msg++
		queuedMsgs <- msg
	}
	atomic.SwapInt32(processTimeSecs, finalProcessSecs)
	<-time.After(time.Duration(initialProcessSecs) * time.Second)

	t.Logf("Sending many messages to update distribution to %d. This new distribution will be used "+
		"when the next batch of messages go out.", finalProcessSecs)
	for i := 0; i < 100; i++ {
		recvdWg.Add(1)
		msg++
		queuedMsgs <- msg // Send many messages to drastically change distribution
	}
	<-time.After(time.Duration(finalProcessSecs) * time.Second)

	t.Logf("Last message going out, whose deadline should be %d.", finalProcessSecs)
	recvdWg.Add(1)
	msg++
	queuedMsgs <- msg
}

// continuouslySend continuously sends messages that exist on the queuedMsgs chan.
func continuouslySend(ctx context.Context, srv *pstest.Server, queuedMsgs chan int32) {
	for {
		select {
		case <-ctx.Done():
			return
		case m := <-queuedMsgs:
			srv.Publish(fullyQualifiedTopicName, []byte(fmt.Sprintf("message %d", m)), nil)
		}
	}
}

func toSet(arr []int32) []int32 {
	var s []int32
	m := map[int32]bool{}

	for _, v := range arr {
		_, ok := m[v]
		if !ok {
			s = append(s, v)
			m[v] = true
		}
	}

	return s

}

func initConn(ctx context.Context, addr string) (*Subscription, *Client, error) {
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		return nil, nil, err
	}
	e := testutil.DefaultHeadersEnforcer()
	opts := append(e.CallOptions(), option.WithGRPCConn(conn))
	client, err := NewClient(ctx, projName, opts...)
	if err != nil {
		return nil, nil, err
	}

	topic := client.Topic(topicName)
	s, err := client.CreateSubscription(ctx, fmt.Sprintf("sub-%d", time.Now().UnixNano()), SubscriptionConfig{Topic: topic})
	if err != nil {
		return nil, nil, err
	}

	exists, err := s.Exists(ctx)
	if !exists {
		return nil, nil, errors.New("Subscription does not exist")
	}
	if err != nil {
		return nil, nil, err
	}

	return s, client, nil
}

// modackDeadlines takes a map of time => Modack, gathers all the Modack.AckDeadlines,
// and returns them as a slice
func modackDeadlines(m map[time.Time][]pstest.Modack) []int32 {
	var u []int32
	for _, vv := range m {
		for _, v := range vv {
			u = append(u, v.AckDeadline)
		}
	}
	return u
}

func TestIterator_ModifyAckContextDeadline(t *testing.T) {
	// Test that all context deadline exceeded errors in ModAckDeadline
	// are not propagated to the client.
	opts := []pstest.ServerReactorOption{
		pstest.WithErrorInjection("ModifyAckDeadline", codes.Unknown, "context deadline exceeded"),
	}
	srv := pstest.NewServer(opts...)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv.Publish(fullyQualifiedTopicName, []byte("creating a topic"), nil)
	s, client, err := initConn(ctx, srv.Addr)
	if err != nil {
		t.Fatal(err)
	}

	srv.Publish(fullyQualifiedTopicName, []byte("some-message"), nil)
	cctx, cancel := context.WithTimeout(ctx, time.Duration(5*time.Second))
	defer cancel()
	err = s.Receive(cctx, func(ctx context.Context, m *Message) {
		m.Ack()
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Got error in Receive: %v", err)
	}

	err = client.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestIterator_SynchronousPullCancel(t *testing.T) {
	srv := pstest.NewServer()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv.Publish(fullyQualifiedTopicName, []byte("creating a topic"), nil)

	_, client, err := initConn(ctx, srv.Addr)
	if err != nil {
		t.Fatal(err)
	}
	iter := newMessageIterator(client.subc, fullyQualifiedTopicName, &pullOptions{})

	// Cancelling the iterator and pulling should not result in any errors.
	iter.cancel()

	if _, err := iter.pullMessages(100); err != nil {
		t.Fatalf("Got error in pullMessages: %v", err)
	}
}

func TestIterator_BoundedDuration(t *testing.T) {
	// Use exported fields for time.Duration fields so they
	// print nicely. Otherwise, they will print as integers.
	//
	// AckDeadline is bounded by min/max ack deadline, which are
	// 10 seconds and 600 seconds respectively. This is
	// true for the real distribution data points as well.
	testCases := []struct {
		desc        string
		AckDeadline time.Duration
		MinDuration time.Duration
		MaxDuration time.Duration
		exactlyOnce bool
		Want        time.Duration
	}{
		{
			desc:        "AckDeadline should be updated to the min duration",
			AckDeadline: time.Duration(10 * time.Second),
			MinDuration: time.Duration(15 * time.Second),
			MaxDuration: time.Duration(10 * time.Minute),
			exactlyOnce: false,
			Want:        time.Duration(15 * time.Second),
		},
		{
			desc:        "AckDeadline should be updated to 1 minute when using exactly once",
			AckDeadline: time.Duration(10 * time.Second),
			MinDuration: 0,
			MaxDuration: time.Duration(10 * time.Minute),
			exactlyOnce: true,
			Want:        time.Duration(1 * time.Minute),
		},
		{
			desc:        "AckDeadline should not be updated here, even though exactly once is enabled",
			AckDeadline: time.Duration(10 * time.Second),
			MinDuration: time.Duration(15 * time.Second),
			MaxDuration: time.Duration(10 * time.Minute),
			exactlyOnce: true,
			Want:        time.Duration(15 * time.Second),
		},
		{
			desc:        "AckDeadline should not be updated here",
			AckDeadline: time.Duration(10 * time.Minute),
			MinDuration: time.Duration(15 * time.Second),
			MaxDuration: time.Duration(10 * time.Minute),
			exactlyOnce: true,
			Want:        time.Duration(10 * time.Minute),
		},
		{
			desc:        "AckDeadline should not be updated when neither durations are set",
			AckDeadline: time.Duration(5 * time.Minute),
			MinDuration: 0,
			MaxDuration: 0,
			exactlyOnce: false,
			Want:        time.Duration(5 * time.Minute),
		},
		{
			desc:        "AckDeadline should should not be updated here since it is within both boundaries",
			AckDeadline: time.Duration(5 * time.Minute),
			MinDuration: time.Duration(1 * time.Minute),
			MaxDuration: time.Duration(7 * time.Minute),
			exactlyOnce: false,
			Want:        time.Duration(5 * time.Minute),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			got := boundedDuration(tc.AckDeadline, tc.MinDuration, tc.MaxDuration, tc.exactlyOnce)
			if got != tc.Want {
				t.Errorf("boundedDuration mismatch:\n%+v\ngot: %v, want: %v", tc, got, tc.Want)
			}
		})
	}
}

func TestIterator_StreamingPullExactlyOnce(t *testing.T) {
	srv := pstest.NewServer()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv.Publish(fullyQualifiedTopicName, []byte("creating a topic"), nil)

	conn, err := grpc.Dial(srv.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	opts := withGRPCHeadersAssertion(t, option.WithGRPCConn(conn))
	client, err := NewClient(ctx, projName, opts...)
	if err != nil {
		t.Fatal(err)
	}

	topic := client.Topic(topicName)
	sc := SubscriptionConfig{
		Topic:                     topic,
		EnableMessageOrdering:     true,
		EnableExactlyOnceDelivery: true,
	}
	_, err = client.CreateSubscription(ctx, subName, sc)
	if err != nil {
		t.Fatal(err)
	}

	// Make sure to call publish before constructing the iterator.
	srv.Publish(fullyQualifiedTopicName, []byte("msg"), nil)

	iter := newMessageIterator(client.subc, fullyQualifiedSubName, &pullOptions{
		synchronous:            false,
		maxOutstandingMessages: 100,
		maxOutstandingBytes:    1e6,
		maxPrefetch:            30,
		maxExtension:           1 * time.Minute,
		maxExtensionPeriod:     10 * time.Second,
	})

	if _, err := iter.receive(10); err != nil {
		t.Fatalf("Got error in recvMessages: %v", err)
	}

	if !iter.enableExactlyOnceDelivery {
		t.Fatalf("expected iter.enableExactlyOnce=true")
	}
}

func TestAddToDistribution(t *testing.T) {
	c, _ := newFake(t)

	iter := newMessageIterator(c.subc, "some-sub", &pullOptions{})

	// Start with a datapoint that's too small that should be bounded to 10s.
	receiveTime := time.Now().Add(time.Duration(-1) * time.Second)
	iter.addToDistribution(receiveTime)
	deadline := iter.ackTimeDist.Percentile(.99)
	want := 10
	if deadline != want {
		t.Errorf("99th percentile ack distribution got: %v, want %v", deadline, want)
	}

	// The next datapoint should not be bounded.
	receiveTime = time.Now().Add(time.Duration(-300) * time.Second)
	iter.addToDistribution(receiveTime)
	deadline = iter.ackTimeDist.Percentile(.99)
	want = 300
	if deadline != want {
		t.Errorf("99th percentile ack distribution got: %v, want %v", deadline, want)
	}

	// Lastly, add a datapoint that should be bounded to 600s
	receiveTime = time.Now().Add(time.Duration(-1000) * time.Second)
	iter.addToDistribution(receiveTime)
	deadline = iter.ackTimeDist.Percentile(.99)
	want = 600
	if deadline != want {
		t.Errorf("99th percentile ack distribution got: %v, want %v", deadline, want)
	}
}

func TestPingStreamAckDeadline(t *testing.T) {
	c, srv := newFake(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv.Publish(fullyQualifiedTopicName, []byte("creating a topic"), nil)
	topic := c.Topic(topicName)
	s, err := c.CreateSubscription(ctx, subName, SubscriptionConfig{Topic: topic})
	if err != nil {
		t.Errorf("failed to create subscription: %v", err)
	}

	iter := newMessageIterator(c.subc, fullyQualifiedSubName, &pullOptions{})
	defer iter.stop()

	iter.eoMu.RLock()
	if iter.enableExactlyOnceDelivery {
		t.Error("iter.enableExactlyOnceDelivery should be false")
	}
	iter.eoMu.RUnlock()

	_, err = s.Update(ctx, SubscriptionConfigToUpdate{
		EnableExactlyOnceDelivery: true,
	})
	if err != nil {
		t.Error(err)
	}
	srv.Publish(fullyQualifiedTopicName, []byte("creating a topic"), nil)
	// Receive one message via the stream to trigger the update to enableExactlyOnceDelivery
	iter.receive(1)
	iter.eoMu.RLock()
	if !iter.enableExactlyOnceDelivery {
		t.Error("iter.enableExactlyOnceDelivery should be true")
	}
	iter.eoMu.RUnlock()
}

func compareCompletedRetryLengths(t *testing.T, completed, retry map[string]*AckResult, wantCompleted, wantRetry int) {
	if l := len(completed); l != wantCompleted {
		t.Errorf("completed slice length got %d, want %d", l, wantCompleted)
	}
	if l := len(retry); l != wantRetry {
		t.Errorf("retry slice length got %d, want %d", l, wantRetry)
	}
}

func TestExactlyOnceProcessRequests(t *testing.T) {
	ctx := context.Background()

	t.Run("NoResults", func(t *testing.T) {
		// If the ackResMap is nil, then the resulting slices should be empty.
		// nil maps here behave the same as if they were empty maps.
		completed, retry := processResults(nil, nil, nil)
		compareCompletedRetryLengths(t, completed, retry, 0, 0)
	})

	t.Run("NoErrorsNilAckResult", func(t *testing.T) {
		// No errors so request should be completed even without an AckResult.
		ackReqMap := map[string]*AckResult{
			"ackID": nil,
		}
		completed, retry := processResults(nil, ackReqMap, nil)
		compareCompletedRetryLengths(t, completed, retry, 1, 0)
	})

	t.Run("NoErrors", func(t *testing.T) {
		// No errors so AckResult should be completed with success.
		r := ipubsub.NewAckResult()
		ackReqMap := map[string]*AckResult{
			"ackID1": r,
		}
		completed, retry := processResults(nil, ackReqMap, nil)
		compareCompletedRetryLengths(t, completed, retry, 1, 0)

		// We can obtain the AckStatus from AckResult if results are completed.
		s, err := r.Get(ctx)
		if err != nil {
			t.Errorf("AckResult err: got %v, want nil", err)
		}
		if s != AcknowledgeStatusSuccess {
			t.Errorf("got %v, want AcknowledgeStatusSuccess", s)
		}
	})

	t.Run("PermanentErrorInvalidAckID", func(t *testing.T) {
		r := ipubsub.NewAckResult()
		ackReqMap := map[string]*AckResult{
			"ackID1": r,
		}
		errorsMap := map[string]string{
			"ackID1": permanentInvalidAckErrString,
		}
		completed, retry := processResults(nil, ackReqMap, errorsMap)
		compareCompletedRetryLengths(t, completed, retry, 1, 0)
		s, err := r.Get(ctx)
		if err == nil {
			t.Error("AckResult err: got nil, want err")
		}
		if s != AcknowledgeStatusInvalidAckID {
			t.Errorf("got %v, want AcknowledgeStatusSuccess", s)
		}
	})

	t.Run("TransientErrorRetry", func(t *testing.T) {
		r := ipubsub.NewAckResult()
		ackReqMap := map[string]*AckResult{
			"ackID1": r,
		}
		errorsMap := map[string]string{
			"ackID1": transientErrStringPrefix + "_FAILURE",
		}
		completed, retry := processResults(nil, ackReqMap, errorsMap)
		compareCompletedRetryLengths(t, completed, retry, 0, 1)
	})

	t.Run("UnknownError", func(t *testing.T) {
		r := ipubsub.NewAckResult()
		ackReqMap := map[string]*AckResult{
			"ackID1": r,
		}
		errorsMap := map[string]string{
			"ackID1": "unknown_error",
		}
		completed, retry := processResults(nil, ackReqMap, errorsMap)
		compareCompletedRetryLengths(t, completed, retry, 1, 0)

		s, err := r.Get(ctx)
		if s != AcknowledgeStatusOther {
			t.Errorf("got %v, want AcknowledgeStatusOther", s)
		}
		if err == nil || err.Error() != "unknown_error" {
			t.Errorf("AckResult err: got %s, want unknown_error", err.Error())
		}
	})

	t.Run("PermissionDenied", func(t *testing.T) {
		r := ipubsub.NewAckResult()
		ackReqMap := map[string]*AckResult{
			"ackID1": r,
		}
		st := status.New(codes.PermissionDenied, "permission denied")
		completed, retry := processResults(st, ackReqMap, nil)
		compareCompletedRetryLengths(t, completed, retry, 1, 0)
		s, err := r.Get(ctx)
		if err == nil {
			t.Error("AckResult err: got nil, want err")
		}
		if s != AcknowledgeStatusPermissionDenied {
			t.Errorf("got %v, want AcknowledgeStatusPermissionDenied", s)
		}
	})

	t.Run("FailedPrecondition", func(t *testing.T) {
		r := ipubsub.NewAckResult()
		ackReqMap := map[string]*AckResult{
			"ackID1": r,
		}
		st := status.New(codes.FailedPrecondition, "failed_precondition")
		completed, retry := processResults(st, ackReqMap, nil)
		compareCompletedRetryLengths(t, completed, retry, 1, 0)
		s, err := r.Get(ctx)
		if err == nil {
			t.Error("AckResult err: got nil, want err")
		}
		if s != AcknowledgeStatusFailedPrecondition {
			t.Errorf("got %v, want AcknowledgeStatusFailedPrecondition", s)
		}
	})

	t.Run("OtherErrorStatus", func(t *testing.T) {
		r := ipubsub.NewAckResult()
		ackReqMap := map[string]*AckResult{
			"ackID1": r,
		}
		st := status.New(codes.OutOfRange, "out of range")
		completed, retry := processResults(st, ackReqMap, nil)
		compareCompletedRetryLengths(t, completed, retry, 1, 0)
		s, err := r.Get(ctx)
		if err == nil {
			t.Error("AckResult err: got nil, want err")
		}
		if s != AcknowledgeStatusOther {
			t.Errorf("got %v, want AcknowledgeStatusOther", s)
		}
	})

	t.Run("MixedSuccessFailureAcks", func(t *testing.T) {
		r1 := ipubsub.NewAckResult()
		r2 := ipubsub.NewAckResult()
		r3 := ipubsub.NewAckResult()
		ackReqMap := map[string]*AckResult{
			"ackID1": r1,
			"ackID2": r2,
			"ackID3": r3,
		}
		errorsMap := map[string]string{
			"ackID1": permanentInvalidAckErrString,
			"ackID2": transientErrStringPrefix + "_FAILURE",
		}
		completed, retry := processResults(nil, ackReqMap, errorsMap)
		compareCompletedRetryLengths(t, completed, retry, 2, 1)
		// message with ackID "ackID1" fails
		s, err := r1.Get(ctx)
		if err == nil {
			t.Error("r1: AckResult err: got nil, want err")
		}
		if s != AcknowledgeStatusInvalidAckID {
			t.Errorf("r1: got %v, want AcknowledgeInvalidAckID", s)
		}

		// message with ackID "ackID2" is to be retried
		ctx2, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_, err = r2.Get(ctx2)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("r2: AckResult.Get should timeout, got: %v", err)
		}

		// message with ackID "ackID3" succeeds
		s, err = r3.Get(ctx)
		if err != nil {
			t.Errorf("r3: AckResult err: got %v, want nil\n", err)
		}
		if s != AcknowledgeStatusSuccess {
			t.Errorf("r3: got %v, want AcknowledgeStatusSuccess", s)
		}
	})

	t.Run("RetriableErrorStatusReturnsRequestForRetrying", func(t *testing.T) {
		for c := range exactlyOnceDeliveryTemporaryRetryErrors {
			r := ipubsub.NewAckResult()
			ackReqMap := map[string]*AckResult{
				"ackID1": r,
			}
			st := status.New(c, "")
			completed, retry := processResults(st, ackReqMap, nil)
			compareCompletedRetryLengths(t, completed, retry, 0, 1)
		}
	})
}
