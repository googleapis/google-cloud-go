// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and

package wire

import (
	"context"
	"testing"

	"cloud.google.com/go/pubsublite/internal/test"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// testCommitter wraps a committer for ease of testing.
type testCommitter struct {
	cmt *committer
	serviceTestProxy
}

func newTestCommitter(t *testing.T, subscription subscriptionPartition, acks *ackTracker) *testCommitter {
	ctx := context.Background()
	cursorClient, err := newCursorClient(ctx, "ignored", testClientOpts...)
	if err != nil {
		t.Fatal(err)
	}

	tc := &testCommitter{
		cmt: newCommitter(ctx, cursorClient, testReceiveSettings(), subscription, acks, true),
	}
	tc.initAndStart(t, tc.cmt, "Committer")
	return tc
}

// SendBatchCommit invokes the periodic background batch commit. Note that the
// periodic task is disabled in tests.
func (tc *testCommitter) SendBatchCommit() {
	tc.cmt.commitOffsetToStream()
}

func (tc *testCommitter) Terminate() {
	tc.cmt.Terminate()
}

func TestCommitterStreamReconnect(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-subs", 0}
	ack1 := newAckConsumer(33, 0, nil)
	ack2 := newAckConsumer(55, 0, nil)
	acks := newAckTracker()
	acks.Push(ack1)
	acks.Push(ack2)

	verifiers := test.NewVerifiers(t)

	// Simulate a transient error that results in a reconnect.
	stream1 := test.NewRPCVerifier(t)
	stream1.Push(initCommitReq(subscription), initCommitResp(), nil)
	barrier := stream1.PushWithBarrier(commitReq(34), nil, status.Error(codes.Unavailable, "server unavailable"))
	verifiers.AddCommitStream(subscription.Path, subscription.Partition, stream1)

	// When the stream reconnects, the latest commit offset should be sent to the
	// server.
	stream2 := test.NewRPCVerifier(t)
	stream2.Push(initCommitReq(subscription), initCommitResp(), nil)
	stream2.Push(commitReq(56), commitResp(1), nil)
	verifiers.AddCommitStream(subscription.Path, subscription.Partition, stream2)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	cmt := newTestCommitter(t, subscription, acks)
	if gotErr := cmt.StartError(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}

	// Send 2 commits.
	ack1.Ack()
	cmt.SendBatchCommit()
	ack2.Ack()
	cmt.SendBatchCommit()

	// Then send the retryable error, which results in reconnect.
	barrier.Release()
	cmt.StopVerifyNoError()
}

func TestCommitterStopFlushesCommits(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-subs", 0}
	ack1 := newAckConsumer(33, 0, nil)
	ack2 := newAckConsumer(55, 0, nil)
	acks := newAckTracker()
	acks.Push(ack1)
	acks.Push(ack2)

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initCommitReq(subscription), initCommitResp(), nil)
	stream.Push(commitReq(34), commitResp(1), nil)
	stream.Push(commitReq(56), commitResp(1), nil)
	verifiers.AddCommitStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	cmt := newTestCommitter(t, subscription, acks)
	if gotErr := cmt.StartError(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}

	ack1.Ack()
	cmt.Stop() // Stop should flush the first offset
	ack2.Ack() // Acks after Stop() are processed
	cmt.SendBatchCommit()
	// Committer terminates when all acks are processed.
	if gotErr := cmt.FinalError(); gotErr != nil {
		t.Errorf("Final err: (%v), want: <nil>", gotErr)
	}
}

func TestCommitterTerminateDiscardsOutstandingAcks(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-subs", 0}
	ack1 := newAckConsumer(33, 0, nil)
	ack2 := newAckConsumer(55, 0, nil)
	acks := newAckTracker()
	acks.Push(ack1)
	acks.Push(ack2)

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initCommitReq(subscription), initCommitResp(), nil)
	stream.Push(commitReq(34), commitResp(1), nil)
	verifiers.AddCommitStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	cmt := newTestCommitter(t, subscription, acks)
	if gotErr := cmt.StartError(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}

	ack1.Ack()
	cmt.Terminate()       // Terminate should flush the first offset
	ack2.Ack()            // Acks after Terminate() are discarded
	cmt.SendBatchCommit() // Should do nothing (server does not expect second commit)
	if gotErr := cmt.FinalError(); gotErr != nil {
		t.Errorf("Final err: (%v), want: <nil>", gotErr)
	}
}

func TestCommitterPermanentStreamError(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-subs", 0}
	acks := newAckTracker()
	wantErr := status.Error(codes.FailedPrecondition, "failed")

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initCommitReq(subscription), nil, wantErr)
	verifiers.AddCommitStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	cmt := newTestCommitter(t, subscription, acks)
	if gotErr := cmt.StartError(); !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("Start() got err: (%v), want: (%v)", gotErr, wantErr)
	}
}

func TestCommitterInvalidInitialResponse(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-subs", 0}
	acks := newAckTracker()

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initCommitReq(subscription), commitResp(1234), nil) // Invalid initial response
	verifiers.AddCommitStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	cmt := newTestCommitter(t, subscription, acks)

	wantErr := errInvalidInitialCommitResponse
	if gotErr := cmt.StartError(); !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("Start() got err: (%v), want: (%v)", gotErr, wantErr)
	}
	if gotErr := cmt.FinalError(); !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("Final err: (%v), want: (%v)", gotErr, wantErr)
	}
}

func TestCommitterInvalidCommitResponse(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-subs", 0}
	ack := newAckConsumer(33, 0, nil)
	acks := newAckTracker()
	acks.Push(ack)

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initCommitReq(subscription), initCommitResp(), nil)
	stream.Push(commitReq(34), initCommitResp(), nil) // Invalid commit response
	verifiers.AddCommitStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	cmt := newTestCommitter(t, subscription, acks)
	if gotErr := cmt.StartError(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}

	ack.Ack()
	cmt.SendBatchCommit()

	if gotErr, wantErr := cmt.FinalError(), errInvalidCommitResponse; !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("Final err: (%v), want: (%v)", gotErr, wantErr)
	}
}

func TestCommitterExcessConfirmedOffsets(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-subs", 0}
	ack := newAckConsumer(33, 0, nil)
	acks := newAckTracker()
	acks.Push(ack)

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initCommitReq(subscription), initCommitResp(), nil)
	stream.Push(commitReq(34), commitResp(2), nil) // More confirmed offsets than committed
	verifiers.AddCommitStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	cmt := newTestCommitter(t, subscription, acks)
	if gotErr := cmt.StartError(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}

	ack.Ack()
	cmt.SendBatchCommit()

	wantMsg := "server acknowledged 2 cursor commits"
	if gotErr := cmt.FinalError(); !test.ErrorHasMsg(gotErr, wantMsg) {
		t.Errorf("Final err: (%v), want msg: (%v)", gotErr, wantMsg)
	}
}

func TestCommitterZeroConfirmedOffsets(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-subs", 0}
	ack := newAckConsumer(33, 0, nil)
	acks := newAckTracker()
	acks.Push(ack)

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initCommitReq(subscription), initCommitResp(), nil)
	stream.Push(commitReq(34), commitResp(0), nil) // Zero confirmed offsets (invalid)
	verifiers.AddCommitStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	cmt := newTestCommitter(t, subscription, acks)
	if gotErr := cmt.StartError(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}

	ack.Ack()
	cmt.SendBatchCommit()

	wantMsg := "server acknowledged an invalid commit count"
	if gotErr := cmt.FinalError(); !test.ErrorHasMsg(gotErr, wantMsg) {
		t.Errorf("Final err: (%v), want msg: (%v)", gotErr, wantMsg)
	}
}
