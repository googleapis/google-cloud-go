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

const defaultStreamTimeout = 30 * time.Second

var errInvalidInitialResponse = errors.New("invalid initial response")

// testStreamHandler is a simplified publisher service that owns a
// retryableStream.
type testStreamHandler struct {
	Topic      topicPartition
	InitialReq *pb.PublishRequest
	Stream     *retryableStream

	t         *testing.T
	statuses  chan streamStatus
	responses chan interface{}
	pubClient *vkit.PublisherClient
}

func newTestStreamHandler(t *testing.T, timeout time.Duration) *testStreamHandler {
	ctx := context.Background()
	pubClient, err := newPublisherClient(ctx, "ignored", testClientOpts...)
	if err != nil {
		t.Fatal(err)
	}

	topic := topicPartition{Path: "path/to/topic", Partition: 1}
	sh := &testStreamHandler{
		Topic:      topic,
		InitialReq: initPubReq(topic),
		t:          t,
		statuses:   make(chan streamStatus, 3),
		responses:  make(chan interface{}, 1),
		pubClient:  pubClient,
	}
	sh.Stream = newRetryableStream(ctx, sh, timeout, reflect.TypeOf(pb.PublishResponse{}))
	return sh
}

func (sh *testStreamHandler) NextStatus() streamStatus {
	select {
	case status := <-sh.statuses:
		return status
	case <-time.After(defaultStreamTimeout):
		sh.t.Errorf("Stream did not change state within %v", defaultStreamTimeout)
		return streamUninitialized
	}
}

func (sh *testStreamHandler) NextResponse() interface{} {
	select {
	case response := <-sh.responses:
		return response
	case <-time.After(defaultStreamTimeout):
		sh.t.Errorf("Stream did not receive response within %v", defaultStreamTimeout)
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
}

func (sh *testStreamHandler) onResponse(response interface{}) {
	sh.responses <- response
}

func TestRetryableStreamStartOnce(t *testing.T) {
	pub := newTestStreamHandler(t, defaultStreamTimeout)

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
	pub := newTestStreamHandler(t, defaultStreamTimeout)

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
	pub := newTestStreamHandler(t, defaultStreamTimeout)

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
	pub := newTestStreamHandler(t, defaultStreamTimeout)

	verifiers := test.NewVerifiers(t)

	// First 2 errors are retryable.
	stream1 := test.NewRPCVerifier(t)
	stream1.Push(pub.InitialReq, nil, status.Error(codes.Unavailable, "server unavailable"))
	verifiers.AddPublishStream(pub.Topic.Path, pub.Topic.Partition, stream1)

	stream2 := test.NewRPCVerifier(t)
	stream2.Push(pub.InitialReq, nil, status.Error(codes.Internal, "internal"))
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

func TestRetryableStreamConnectPermanentFailure(t *testing.T) {
	pub := newTestStreamHandler(t, defaultStreamTimeout)
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
	pub := newTestStreamHandler(t, timeout)
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
	if gotErr := pub.Stream.Error(); !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("Stream final err: got (%v), want (%v)", gotErr, wantErr)
	}
}

func TestRetryableStreamSendReceive(t *testing.T) {
	pub := newTestStreamHandler(t, defaultStreamTimeout)
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
