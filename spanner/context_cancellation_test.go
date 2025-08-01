/*
Copyright 2025 Google LLC

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
	"context"
	"io"
	"sync"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

type mockStreamingReceiver struct {
	responses        []*sppb.PartialResultSet
	index            int
	mu               sync.Mutex
	trailersReceived bool
	customRecv       func() (*sppb.PartialResultSet, error)
}

func (m *mockStreamingReceiver) Recv() (*sppb.PartialResultSet, error) {
	if m.customRecv != nil {
		return m.customRecv()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.index >= len(m.responses) {
		m.trailersReceived = true
		return nil, io.EOF
	}

	response := m.responses[m.index]
	m.index++
	return response, nil
}

func (m *mockStreamingReceiver) wasTrailersReceived() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.trailersReceived
}

func TestContextCancellationWithLastFlag_CorrectBehavior(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cancelled := make(chan bool, 1)
	go func() {
		<-ctx.Done()
		cancelled <- true
	}()

	mockStream := &mockStreamingReceiver{
		responses: []*sppb.PartialResultSet{
			{
				Values: []*structpb.Value{
					{Kind: &structpb.Value_StringValue{StringValue: "test"}},
				},
				Last: true,
			},
		},
	}

	rpc := func(ctx context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
		return mockStream, nil
	}

	decoder := newResumableStreamDecoder(ctx, cancel, nil, rpc, nil, nil)
	mt := &builtinMetricsTracer{}
	decoder.stream, _ = rpc(ctx, nil)

	decoder.tryRecv(mt, onCodes(DefaultRetryBackoff, codes.Unavailable))

	timeout := time.After(200 * time.Millisecond)
	contextWasCancelled := false

	select {
	case <-cancelled:
		contextWasCancelled = true
	case <-timeout:
	}

	if !contextWasCancelled {
		t.Error("Expected context to be cancelled with fixed implementation, but it wasn't")
	}

	time.Sleep(50 * time.Millisecond)
	if mockStream.wasTrailersReceived() {
		t.Error("Trailers were received in fixed implementation - should be skipped for performance")
	}
}

func TestEdgeCases(t *testing.T) {
	t.Run("No Last Flag", func(t *testing.T) {

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		cancelled := make(chan bool, 1)
		go func() {
			<-ctx.Done()
			cancelled <- true
		}()

		mockStream := &mockStreamingReceiver{
			responses: []*sppb.PartialResultSet{
				{
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "test"}},
					},
					Last: false,
				},
			},
		}

		rpc := func(ctx context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
			return mockStream, nil
		}

		decoder := newResumableStreamDecoder(ctx, cancel, nil, rpc, nil, nil)
		mt := &builtinMetricsTracer{}
		decoder.stream, _ = rpc(ctx, nil)

		decoder.tryRecv(mt, onCodes(DefaultRetryBackoff, codes.Unavailable))

		timeout := time.After(100 * time.Millisecond)
		contextWasCancelled := false

		select {
		case <-cancelled:
			contextWasCancelled = true
		case <-timeout:
		}

		if contextWasCancelled {
			t.Error("Context was cancelled when Last=false - this should not happen")
		}
	})

	t.Run("Nil Cancel Function", func(t *testing.T) {

		ctx := context.Background()

		mockStream := &mockStreamingReceiver{
			responses: []*sppb.PartialResultSet{
				{Last: true},
			},
		}

		rpc := func(ctx context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
			return mockStream, nil
		}

		decoder := newResumableStreamDecoder(ctx, nil, nil, rpc, nil, nil)
		mt := &builtinMetricsTracer{}
		decoder.stream, _ = rpc(ctx, nil)

		decoder.tryRecv(mt, onCodes(DefaultRetryBackoff, codes.Unavailable))

	})

	t.Run("Multiple Last Flag Messages", func(t *testing.T) {

		for i := 0; i < 3; i++ {
			ctx, cancel := context.WithCancel(context.Background())

			cancelled := make(chan bool, 1)
			go func() {
				<-ctx.Done()
				cancelled <- true
			}()

			mockStream := &mockStreamingReceiver{
				responses: []*sppb.PartialResultSet{
					{Last: true},
				},
			}

			rpc := func(ctx context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
				return mockStream, nil
			}

			decoder := newResumableStreamDecoder(ctx, cancel, nil, rpc, nil, nil)
			mt := &builtinMetricsTracer{}
			decoder.stream, _ = rpc(ctx, nil)

			decoder.tryRecv(mt, onCodes(DefaultRetryBackoff, codes.Unavailable))

			timeout := time.After(50 * time.Millisecond)
			contextWasCancelled := false

			select {
			case <-cancelled:
				contextWasCancelled = true
			case <-timeout:
			}

			if !contextWasCancelled {
				t.Errorf("Iteration %d: Context was not cancelled", i)
			}
		}

	})
}

func TestMockStreamCorrectness(t *testing.T) {
	t.Run("Mock Response Order", func(t *testing.T) {
		mockStream := &mockStreamingReceiver{
			responses: []*sppb.PartialResultSet{
				{
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "first"}},
					},
					Last: false,
				},
				{
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "second"}},
					},
					Last: true,
				},
			},
		}

		resp1, err1 := mockStream.Recv()
		if err1 != nil {
			t.Fatalf("First Recv failed: %v", err1)
		}
		if resp1.Values[0].GetStringValue() != "first" {
			t.Errorf("First response wrong: got %s, want first", resp1.Values[0].GetStringValue())
		}
		if resp1.Last {
			t.Error("First response should not have Last=true")
		}

		resp2, err2 := mockStream.Recv()
		if err2 != nil {
			t.Fatalf("Second Recv failed: %v", err2)
		}
		if resp2.Values[0].GetStringValue() != "second" {
			t.Errorf("Second response wrong: got %s, want second", resp2.Values[0].GetStringValue())
		}
		if !resp2.Last {
			t.Error("Second response should have Last=true")
		}

		_, err3 := mockStream.Recv()
		if err3 != io.EOF {
			t.Errorf("Third Recv should return EOF, got: %v", err3)
		}

		if !mockStream.wasTrailersReceived() {
			t.Error("Trailers should be marked as received after EOF")
		}

	})

	t.Run("Mock Thread Safety", func(t *testing.T) {

		mockStream := &mockStreamingReceiver{
			responses: []*sppb.PartialResultSet{
				{Last: true},
			},
		}

		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				defer func() { done <- true }()
				_ = mockStream.wasTrailersReceived()
			}()
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		mockStream.Recv()
		mockStream.Recv()

		for i := 0; i < 10; i++ {
			go func() {
				defer func() { done <- true }()
				if !mockStream.wasTrailersReceived() {
					t.Error("Race condition detected in mock")
				}
			}()
		}

		for i := 0; i < 10; i++ {
			<-done
		}

	})

	t.Run("Mock State Isolation", func(t *testing.T) {

		mock1 := &mockStreamingReceiver{
			responses: []*sppb.PartialResultSet{{Last: true}},
		}
		mock2 := &mockStreamingReceiver{
			responses: []*sppb.PartialResultSet{{Last: false}},
		}

		mock1.Recv()
		mock1.Recv()

		if !mock1.wasTrailersReceived() {
			t.Error("mock1 should have trailers received")
		}
		if mock2.wasTrailersReceived() {
			t.Error("mock2 should NOT have trailers received")
		}

	})
}

func TestFixComparison(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		implementation         func(*resumableStreamDecoder, *builtinMetricsTracer, gax.Retryer)
		expectContextCancelled bool
		expectTrailersReceived bool
		description            string
	}{
		{
			name:                   "Fixed Implementation",
			implementation:         (*resumableStreamDecoder).tryRecv,
			expectContextCancelled: true,
			expectTrailersReceived: false,
			description:            "Current working implementation",
		},
		{
			name:                   "Broken Implementation (PR #11854)",
			implementation:         brokenTryRecv,
			expectContextCancelled: false,
			expectTrailersReceived: false,
			description:            "Demonstrates the bug from PR #11854",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			cancelled := make(chan bool, 1)
			go func() {
				<-ctx.Done()
				cancelled <- true
			}()

			mockStream := &mockStreamingReceiver{
				responses: []*sppb.PartialResultSet{
					{
						Values: []*structpb.Value{
							{Kind: &structpb.Value_StringValue{StringValue: "test"}},
						},
						Last: true,
					},
				},
			}

			rpc := func(ctx context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
				return mockStream, nil
			}

			decoder := newResumableStreamDecoder(ctx, cancel, nil, rpc, nil, nil)
			mt := &builtinMetricsTracer{}
			decoder.stream, _ = rpc(ctx, nil)

			tt.implementation(decoder, mt, onCodes(DefaultRetryBackoff, codes.Unavailable))

			timeout := time.After(100 * time.Millisecond)
			contextWasCancelled := false

			select {
			case <-cancelled:
				contextWasCancelled = true
			case <-timeout:
			}

			time.Sleep(100 * time.Millisecond)

			if contextWasCancelled != tt.expectContextCancelled {
				t.Errorf("Context cancellation mismatch: got %v, want %v",
					contextWasCancelled, tt.expectContextCancelled)
			}

			if mockStream.wasTrailersReceived() != tt.expectTrailersReceived {
				t.Errorf("Trailer reception mismatch: got %v, want %v",
					mockStream.wasTrailersReceived(), tt.expectTrailersReceived)
			}

		})
	}
}

func brokenTryRecv(d *resumableStreamDecoder, mt *builtinMetricsTracer, retryer gax.Retryer) {
	var res *sppb.PartialResultSet
	res, d.err = d.stream.Recv()
	if d.err == nil {
		d.q.push(res)
		if res.GetLast() {

			d.changeState(finished)
			return
		}
		if d.state == queueingRetryable && !d.isNewResumeToken(res.ResumeToken) {
			d.bytesBetweenResumeTokens += int32(proto.Size(res))
		}
		d.changeState(d.state)
		return
	}

	if d.err == io.EOF {
		d.err = nil

		d.changeState(finished)
		return
	}

	d.changeState(aborted)
}

func TestContextCancellationWithLastFlag_BrokenBehavior(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cancelled := make(chan bool, 1)
	go func() {
		<-ctx.Done()
		cancelled <- true
	}()

	mockStream := &mockStreamingReceiver{
		responses: []*sppb.PartialResultSet{
			{
				Values: []*structpb.Value{
					{Kind: &structpb.Value_StringValue{StringValue: "test"}},
				},
				Last: true,
			},
		},
	}

	rpc := func(ctx context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
		return mockStream, nil
	}

	decoder := newResumableStreamDecoder(ctx, cancel, nil, rpc, nil, nil)
	mt := &builtinMetricsTracer{}
	decoder.stream, _ = rpc(ctx, nil)

	brokenTryRecv(decoder, mt, onCodes(DefaultRetryBackoff, codes.Unavailable))

	timeout := time.After(100 * time.Millisecond)
	contextWasCancelled := false

	select {
	case <-cancelled:
		contextWasCancelled = true
	case <-timeout:
	}

	if contextWasCancelled {
		t.Error("Context was cancelled with broken implementation - this shouldn't happen!")
	}

	if mockStream.wasTrailersReceived() {
		t.Error("Trailers were received in broken implementation - this shouldn't happen!")
	}
}

func TestDetectBrokenBehavior(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	streamCancelledContext := false

	wrappedCancel := func() {
		streamCancelledContext = true
		cancel()
	}

	mockStream := &mockStreamingReceiver{
		responses: []*sppb.PartialResultSet{
			{Last: true},
		},
	}

	rpc := func(ctx context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
		return mockStream, nil
	}

	decoder := newResumableStreamDecoder(ctx, wrappedCancel, nil, rpc, nil, nil)
	mt := &builtinMetricsTracer{}
	decoder.stream, _ = rpc(ctx, nil)

	brokenTryRecv(decoder, mt, onCodes(DefaultRetryBackoff, codes.Unavailable))

	time.Sleep(50 * time.Millisecond)

	if streamCancelledContext {
		t.Error("Broken implementation incorrectly cancelled context")
	}

	decoder2 := newResumableStreamDecoder(ctx, wrappedCancel, nil, rpc, nil, nil)
	decoder2.stream, _ = rpc(ctx, nil)
	streamCancelledContext = false

	decoder2.tryRecv(mt, onCodes(DefaultRetryBackoff, codes.Unavailable))

	time.Sleep(100 * time.Millisecond)

	if !streamCancelledContext {
		t.Error("Correct implementation should have cancelled context")
	}
}
