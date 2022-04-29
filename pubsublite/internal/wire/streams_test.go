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
	"errors"
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/pubsublite/internal/test"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	vkit "cloud.google.com/go/pubsublite/apiv1"
	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

const streamTestTimeout = 30 * time.Second

var errInvalidInitialResponse = errors.New("invalid initial response")

// testStreamHandler is a simplified publisher service that owns a
// retryableStream.
type testStreamHandler struct {
	CancelCtx  context.CancelFunc
	Topic      topicPartition
	InitialReq *pb.PublishRequest
	Stream     *retryableStream

	t         *testing.T
	statuses  chan streamStatus
	responses chan interface{}
	pubClient *vkit.PublisherClient
}

func newTestStreamHandler(t *testing.T, connectTimeout, idleTimeout time.Duration) *testStreamHandler {
	ctx, cancel := context.WithCancel(context.Background())
	pubClient, err := newPublisherClient(ctx, "ignored", testServer.ClientConn())
	if err != nil {
		t.Fatal(err)
	}

	topic := topicPartition{Path: "path/to/topic", Partition: 1}
	sh := &testStreamHandler{
		CancelCtx:  cancel,
		Topic:      topic,
		InitialReq: initPubReq(topic),
		t:          t,
		statuses:   make(chan streamStatus, 3),
		responses:  make(chan interface{}, 1),
		pubClient:  pubClient,
	}
	sh.Stream = newRetryableStream(ctx, sh, connectTimeout, idleTimeout, reflect.TypeOf(pb.PublishResponse{}))
	return sh
}

func (sh *testStreamHandler) NextStatus() streamStatus {
	select {
	case status := <-sh.statuses:
		return status
	case <-time.After(streamTestTimeout):
		sh.t.Errorf("Stream did not change state within %v", streamTestTimeout)
		return streamUninitialized
	}
}

func (sh *testStreamHandler) NextResponse() interface{} {
	select {
	case response := <-sh.responses:
		return response
	case <-time.After(streamTestTimeout):
		sh.t.Errorf("Stream did not receive response within %v", streamTestTimeout)
		return nil
	}
}

func (sh *testStreamHandler) newStream(ctx context.Context) (grpc.ClientStream, error) {
	return sh.pubClient.Publish(ctx)
}

func (sh *testStreamHandler) validateInitialResponse(response interface{}) error {
	pubResponse, _ := response.(*pb.PublishResponse)
	if pubResponse.GetInitialResponse() == nil {
		return errInvalidInitialResponse
	}
	return nil
}

func (sh *testStreamHandler) initialRequest() (interface{}, initialResponseRequired) {
	return sh.InitialReq, initialResponseRequired(true)
}

func (sh *testStreamHandler) onStreamStatusChange(status streamStatus) {
	sh.statuses <- status

	// Close connections.
	if status == streamTerminated {
		sh.pubClient.Close()
	}
}

func (sh *testStreamHandler) onResponse(response interface{}) {
	sh.responses <- response
}

func TestRetryableStreamStartOnce(t *testing.T) {
	pub := newTestStreamHandler(t, streamTestTimeout, streamTestTimeout)

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(pub.InitialReq, initPubResp(), nil)
	verifiers.AddPublishStream(pub.Topic.Path, pub.Topic.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	// Ensure that new streams are not opened if the publisher is started twice
	// (note: only 1 stream verifier was added to the mock server above).
	pub.Stream.Start()
	pub.Stream.Start()
	pub.Stream.Start()
	if got, want := pub.NextStatus(), streamReconnecting; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if got, want := pub.NextStatus(), streamConnected; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}

	pub.Stream.Stop()
	if got, want := pub.NextStatus(), streamTerminated; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if gotErr := pub.Stream.Error(); gotErr != nil {
		t.Errorf("Stream final err: got (%v), want <nil>", gotErr)
	}
}

func TestRetryableStreamStopWhileConnecting(t *testing.T) {
	pub := newTestStreamHandler(t, streamTestTimeout, streamTestTimeout)

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	barrier := stream.PushWithBarrier(pub.InitialReq, initPubResp(), nil)
	verifiers.AddPublishStream(pub.Topic.Path, pub.Topic.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	pub.Stream.Start()
	if got, want := pub.NextStatus(), streamReconnecting; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}

	barrier.Release()
	pub.Stream.Stop()

	// The stream should transition to terminated and the client stream should be
	// discarded.
	if got, want := pub.NextStatus(), streamTerminated; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if pub.Stream.currentStream() != nil {
		t.Error("Client stream should be nil")
	}
	if gotErr := pub.Stream.Error(); gotErr != nil {
		t.Errorf("Stream final err: got (%v), want <nil>", gotErr)
	}
}

func TestRetryableStreamStopAbortsRetries(t *testing.T) {
	pub := newTestStreamHandler(t, streamTestTimeout, streamTestTimeout)

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	// Aborted is a retryable error, but the stream should not be retried because
	// the publisher is stopped.
	barrier := stream.PushWithBarrier(pub.InitialReq, nil, status.Error(codes.Aborted, "abort retry"))
	verifiers.AddPublishStream(pub.Topic.Path, pub.Topic.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	pub.Stream.Start()
	if got, want := pub.NextStatus(), streamReconnecting; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}

	barrier.Release()
	pub.Stream.Stop()

	// The stream should transition to terminated and the client stream should be
	// discarded.
	if got, want := pub.NextStatus(), streamTerminated; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if pub.Stream.currentStream() != nil {
		t.Error("Client stream should be nil")
	}
	if gotErr := pub.Stream.Error(); gotErr != nil {
		t.Errorf("Stream final err: got (%v), want <nil>", gotErr)
	}
}

func TestRetryableStreamConnectRetries(t *testing.T) {
	pub := newTestStreamHandler(t, streamTestTimeout, streamTestTimeout)

	verifiers := test.NewVerifiers(t)

	// First 2 errors are retryable.
	stream1 := test.NewRPCVerifier(t)
	stream1.Push(pub.InitialReq, nil, status.Error(codes.Unavailable, "server unavailable"))
	verifiers.AddPublishStream(pub.Topic.Path, pub.Topic.Partition, stream1)

	stream2 := test.NewRPCVerifier(t)
	stream2.Push(pub.InitialReq, nil, status.Error(codes.Canceled, "canceled"))
	verifiers.AddPublishStream(pub.Topic.Path, pub.Topic.Partition, stream2)

	// Third stream should succeed.
	stream3 := test.NewRPCVerifier(t)
	stream3.Push(pub.InitialReq, initPubResp(), nil)
	verifiers.AddPublishStream(pub.Topic.Path, pub.Topic.Partition, stream3)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	pub.Stream.Start()
	if got, want := pub.NextStatus(), streamReconnecting; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if got, want := pub.NextStatus(), streamConnected; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}

	pub.Stream.Stop()
	if got, want := pub.NextStatus(), streamTerminated; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
}

func TestRetryableStreamContextCanceledNotRetried(t *testing.T) {
	pub := newTestStreamHandler(t, streamTestTimeout, streamTestTimeout)

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(pub.InitialReq, initPubResp(), nil)
	verifiers.AddPublishStream(pub.Topic.Path, pub.Topic.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	pub.Stream.Start()
	if got, want := pub.NextStatus(), streamReconnecting; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if got, want := pub.NextStatus(), streamConnected; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}

	// Cancelling the parent context will cause the current gRPC stream to fail
	// with a retryable Canceled error.
	pub.CancelCtx()
	if got, want := pub.NextStatus(), streamReconnecting; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	// Reconnection then fails.
	if got, want := pub.NextStatus(), streamTerminated; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if gotErr, wantErr := pub.Stream.Error(), context.Canceled; !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("Stream final err: got (%v), want (%v)", gotErr, wantErr)
	}
}

func TestRetryableStreamConnectPermanentFailure(t *testing.T) {
	pub := newTestStreamHandler(t, streamTestTimeout, streamTestTimeout)
	permanentErr := status.Error(codes.PermissionDenied, "denied")

	verifiers := test.NewVerifiers(t)
	// The stream connection results in a non-retryable error, so the publisher
	// cannot start.
	stream := test.NewRPCVerifier(t)
	stream.Push(pub.InitialReq, nil, permanentErr)
	verifiers.AddPublishStream(pub.Topic.Path, pub.Topic.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	pub.Stream.Start()
	if got, want := pub.NextStatus(), streamReconnecting; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if got, want := pub.NextStatus(), streamTerminated; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if pub.Stream.currentStream() != nil {
		t.Error("Client stream should be nil")
	}
	if gotErr := pub.Stream.Error(); !test.ErrorEqual(gotErr, permanentErr) {
		t.Errorf("Stream final err: got (%v), want (%v)", gotErr, permanentErr)
	}
}

func TestRetryableStreamConnectTimeout(t *testing.T) {
	// Set a very low timeout to ensure no retries.
	timeout := time.Millisecond
	pub := newTestStreamHandler(t, timeout, streamTestTimeout)
	pub.Stream.initTimeout = defaultStreamInitTimeout
	wantErr := status.Error(codes.DeadlineExceeded, "timeout")

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	barrier := stream.PushWithBarrier(pub.InitialReq, nil, wantErr)
	verifiers.AddPublishStream(pub.Topic.Path, pub.Topic.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	pub.Stream.Start()
	if got, want := pub.NextStatus(), streamReconnecting; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}

	// Send the initial server response well after the timeout setting.
	time.Sleep(10 * timeout)
	barrier.Release()

	if got, want := pub.NextStatus(), streamTerminated; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if pub.Stream.currentStream() != nil {
		t.Error("Client stream should be nil")
	}
	if gotErr := pub.Stream.Error(); !test.ErrorEqual(gotErr, ErrBackendUnavailable) {
		t.Errorf("Stream final err: got (%v), want (%v)", gotErr, ErrBackendUnavailable)
	}
}

func TestRetryableStreamInitTimeout(t *testing.T) {
	const streamInitTimeout = 50 * time.Millisecond
	const streamResponseDelay = 75 * time.Millisecond

	pub := newTestStreamHandler(t, streamTestTimeout, streamTestTimeout)
	pub.Stream.initTimeout = streamInitTimeout

	verifiers := test.NewVerifiers(t)

	// First stream will have a delayed response.
	stream1 := test.NewRPCVerifier(t)
	barrier := stream1.PushWithBarrier(pub.InitialReq, initPubResp(), nil)
	verifiers.AddPublishStream(pub.Topic.Path, pub.Topic.Partition, stream1)

	// Second stream should succeed.
	stream2 := test.NewRPCVerifier(t)
	stream2.Push(pub.InitialReq, initPubResp(), nil)
	verifiers.AddPublishStream(pub.Topic.Path, pub.Topic.Partition, stream2)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	pub.Stream.Start()
	if got, want := pub.NextStatus(), streamReconnecting; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}

	barrier.ReleaseAfter(func() {
		time.Sleep(streamResponseDelay)
	})
	if got, want := pub.NextStatus(), streamConnected; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}

	pub.Stream.Stop()
	if got, want := pub.NextStatus(), streamTerminated; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
}

func TestRetryableStreamSendReceive(t *testing.T) {
	pub := newTestStreamHandler(t, streamTestTimeout, streamTestTimeout)
	req := msgPubReq(&pb.PubSubMessage{Data: []byte("msg")})
	wantResp := msgPubResp(5)

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	barrier := stream.PushWithBarrier(pub.InitialReq, initPubResp(), nil)
	stream.Push(req, wantResp, nil)
	verifiers.AddPublishStream(pub.Topic.Path, pub.Topic.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	pub.Stream.Start()
	if got, want := pub.NextStatus(), streamReconnecting; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}

	// While the stream is reconnecting, requests are discarded.
	if got, want := pub.Stream.Send(req), false; got != want {
		t.Errorf("Stream send: got %v, want %v", got, want)
	}

	barrier.Release()
	if got, want := pub.NextStatus(), streamConnected; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}

	if got, want := pub.Stream.Send(req), true; got != want {
		t.Errorf("Stream send: got %v, want %v", got, want)
	}
	if gotResp := pub.NextResponse(); !testutil.Equal(gotResp, wantResp) {
		t.Errorf("Stream response: got %v, want %v", gotResp, wantResp)
	}

	pub.Stream.Stop()
	if got, want := pub.NextStatus(), streamTerminated; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if gotErr := pub.Stream.Error(); gotErr != nil {
		t.Errorf("Stream final err: got (%v), want <nil>", gotErr)
	}
}

func TestRetryableStreamConnectReceivesResetSignal(t *testing.T) {
	pub := newTestStreamHandler(t, streamTestTimeout, streamTestTimeout)

	verifiers := test.NewVerifiers(t)

	stream1 := test.NewRPCVerifier(t)
	// Reset signal received during stream initialization.
	stream1.Push(pub.InitialReq, nil, makeStreamResetSignal())
	verifiers.AddPublishStream(pub.Topic.Path, pub.Topic.Partition, stream1)

	stream2 := test.NewRPCVerifier(t)
	stream2.Push(pub.InitialReq, initPubResp(), nil)
	verifiers.AddPublishStream(pub.Topic.Path, pub.Topic.Partition, stream2)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	pub.Stream.Start()
	if got, want := pub.NextStatus(), streamReconnecting; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if got, want := pub.NextStatus(), streamResetState; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if got, want := pub.NextStatus(), streamConnected; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}

	pub.Stream.Stop()
	if got, want := pub.NextStatus(), streamTerminated; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if gotErr := pub.Stream.Error(); gotErr != nil {
		t.Errorf("Stream final err: got (%v), want <nil>", gotErr)
	}
}

func TestRetryableStreamDisconnectedWithResetSignal(t *testing.T) {
	pub := newTestStreamHandler(t, streamTestTimeout, streamTestTimeout)

	verifiers := test.NewVerifiers(t)

	stream1 := test.NewRPCVerifier(t)
	stream1.Push(pub.InitialReq, initPubResp(), nil)
	// Reset signal received after stream is connected.
	stream1.Push(nil, nil, makeStreamResetSignal())
	verifiers.AddPublishStream(pub.Topic.Path, pub.Topic.Partition, stream1)

	stream2 := test.NewRPCVerifier(t)
	stream2.Push(pub.InitialReq, initPubResp(), nil)
	verifiers.AddPublishStream(pub.Topic.Path, pub.Topic.Partition, stream2)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	pub.Stream.Start()
	if got, want := pub.NextStatus(), streamReconnecting; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if got, want := pub.NextStatus(), streamConnected; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if got, want := pub.NextStatus(), streamReconnecting; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if got, want := pub.NextStatus(), streamResetState; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if got, want := pub.NextStatus(), streamConnected; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}

	pub.Stream.Stop()
	if got, want := pub.NextStatus(), streamTerminated; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if gotErr := pub.Stream.Error(); gotErr != nil {
		t.Errorf("Stream final err: got (%v), want <nil>", gotErr)
	}
}

func TestRetryableStreamIdleStreamDetection(t *testing.T) {
	pub := newTestStreamHandler(t, streamTestTimeout, 50*time.Millisecond)
	req := msgPubReq(&pb.PubSubMessage{Data: []byte("msg")})
	wantResp := msgPubResp(5)

	verifiers := test.NewVerifiers(t)

	stream1 := test.NewRPCVerifier(t)
	stream1.Push(pub.InitialReq, initPubResp(), nil)
	verifiers.AddPublishStream(pub.Topic.Path, pub.Topic.Partition, stream1)

	stream2 := test.NewRPCVerifier(t)
	stream2.Push(pub.InitialReq, initPubResp(), nil)
	stream2.Push(req, wantResp, nil)
	verifiers.AddPublishStream(pub.Topic.Path, pub.Topic.Partition, stream2)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	// First stream connection becomes idle (no responses received).
	pub.Stream.Start()
	if got, want := pub.NextStatus(), streamReconnecting; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if got, want := pub.NextStatus(), streamConnected; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}

	// Second stream connection.
	if got, want := pub.NextStatus(), streamReconnecting; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if got, want := pub.NextStatus(), streamConnected; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if got, want := pub.Stream.Send(req), true; got != want {
		t.Errorf("Stream send: got %v, want %v", got, want)
	}
	if gotResp := pub.NextResponse(); !testutil.Equal(gotResp, wantResp) {
		t.Errorf("Stream response: got %v, want %v", gotResp, wantResp)
	}

	pub.Stream.Stop()
	if got, want := pub.NextStatus(), streamTerminated; got != want {
		t.Errorf("Stream status change: got %d, want %d", got, want)
	}
	if gotErr := pub.Stream.Error(); gotErr != nil {
		t.Errorf("Stream final err: got (%v), want <nil>", gotErr)
	}
}
