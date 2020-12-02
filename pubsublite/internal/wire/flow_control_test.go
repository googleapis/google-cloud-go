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
	"math"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/pubsublite/internal/test"
	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"

	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

func TestTokenCounterAdd(t *testing.T) {
	// Note: tests are applied to this counter instance in order.
	counter := tokenCounter{}

	for _, tc := range []struct {
		desc  string
		delta flowControlTokens
		want  tokenCounter
	}{
		{
			desc:  "Initialize",
			delta: flowControlTokens{Bytes: 9876543, Messages: 1234},
			want:  tokenCounter{Bytes: 9876543, Messages: 1234},
		},
		{
			desc:  "Add delta",
			delta: flowControlTokens{Bytes: 1, Messages: 2},
			want:  tokenCounter{Bytes: 9876544, Messages: 1236},
		},
		{
			desc:  "Overflow",
			delta: flowControlTokens{Bytes: math.MaxInt64, Messages: math.MaxInt64},
			want:  tokenCounter{Bytes: math.MaxInt64, Messages: math.MaxInt64},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			counter.Add(tc.delta)
			if !testutil.Equal(counter, tc.want) {
				t.Errorf("tokenCounter.Add(%v): got %v, want %v", tc.delta, counter, tc.want)
			}
		})
	}
}

func TestTokenCounterSub(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		counter tokenCounter
		delta   flowControlTokens
		want    tokenCounter
		wantErr error
	}{
		{
			desc:    "Result zero",
			counter: tokenCounter{Bytes: 9876543, Messages: 1234},
			delta:   flowControlTokens{Bytes: 9876543, Messages: 1234},
			want:    tokenCounter{Bytes: 0, Messages: 0},
		},
		{
			desc:    "Result non-zero",
			counter: tokenCounter{Bytes: 9876543, Messages: 1234},
			delta:   flowControlTokens{Bytes: 9876500, Messages: 1200},
			want:    tokenCounter{Bytes: 43, Messages: 34},
		},
		{
			desc:    "Bytes negative",
			counter: tokenCounter{Bytes: 9876543, Messages: 1234},
			delta:   flowControlTokens{Bytes: 9876544, Messages: 1234},
			want:    tokenCounter{Bytes: 9876543, Messages: 1234},
			wantErr: errTokenCounterBytesNegative,
		},
		{
			desc:    "Messages negative",
			counter: tokenCounter{Bytes: 9876543, Messages: 1234},
			delta:   flowControlTokens{Bytes: 9876543, Messages: 1235},
			want:    tokenCounter{Bytes: 9876543, Messages: 1234},
			wantErr: errTokenCounterMessagesNegative,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			gotErr := tc.counter.Sub(tc.delta)
			if !testutil.Equal(tc.counter, tc.want) {
				t.Errorf("tokenCounter.Sub(%v): got %v, want %v", tc.delta, tc.counter, tc.want)
			}
			if !test.ErrorEqual(gotErr, tc.wantErr) {
				t.Errorf("tokenCounter.Sub(%v) error: got %v, want %v", tc.delta, gotErr, tc.wantErr)
			}
		})
	}
}

func TestTokenCounterToFlowControlRequest(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		counter tokenCounter
		want    *pb.FlowControlRequest
	}{
		{
			desc:    "Uninitialized counter",
			counter: tokenCounter{},
			want:    nil,
		},
		{
			desc:    "Bytes non-zero",
			counter: tokenCounter{Bytes: 1},
			want:    &pb.FlowControlRequest{AllowedBytes: 1},
		},
		{
			desc:    "Messages non-zero",
			counter: tokenCounter{Messages: 1},
			want:    &pb.FlowControlRequest{AllowedMessages: 1},
		},
		{
			desc:    "Messages and bytes",
			counter: tokenCounter{Bytes: 56, Messages: 32},
			want:    &pb.FlowControlRequest{AllowedBytes: 56, AllowedMessages: 32},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			got := tc.counter.ToFlowControlRequest()
			if !proto.Equal(got, tc.want) {
				t.Errorf("tokenCounter(%v).ToFlowControlRequest(): got %v, want %v", tc.counter, got, tc.want)
			}
		})
	}
}

func TestFlowControlBatcher(t *testing.T) {
	var batcher flowControlBatcher

	t.Run("Uninitialized", func(t *testing.T) {
		if got, want := batcher.ShouldExpediteBatchRequest(), false; got != want {
			t.Errorf("flowControlBatcher.ShouldExpediteBatchRequest(): got %v, want %v", got, want)
		}
		if got, want := batcher.ReleasePendingRequest(), (*pb.FlowControlRequest)(nil); !proto.Equal(got, want) {
			t.Errorf("flowControlBatcher.ReleasePendingRequest(): got %v, want %v", got, want)
		}
		if got, want := batcher.RequestForRestart(), (*pb.FlowControlRequest)(nil); !proto.Equal(got, want) {
			t.Errorf("flowControlBatcher.RequestForRestart(): got %v, want %v", got, want)
		}
	})

	t.Run("OnClientFlow-1", func(t *testing.T) {
		deltaTokens := flowControlTokens{Bytes: 500, Messages: 10}
		batcher.OnClientFlow(deltaTokens)

		if got, want := batcher.ShouldExpediteBatchRequest(), true; got != want {
			t.Errorf("flowControlBatcher.ShouldExpediteBatchRequest(): got %v, want %v", got, want)
		}
		if got, want := batcher.ReleasePendingRequest(), flowControlReq(deltaTokens); !proto.Equal(got, want) {
			t.Errorf("flowControlBatcher.ReleasePendingRequest(): got %v, want %v", got, want)
		}
		if got, want := batcher.RequestForRestart(), flowControlReq(deltaTokens); !proto.Equal(got, want) {
			t.Errorf("flowControlBatcher.RequestForRestart(): got %v, want %v", got, want)
		}
	})

	t.Run("OnClientFlow-2", func(t *testing.T) {
		deltaTokens := flowControlTokens{Bytes: 100, Messages: 1}
		batcher.OnClientFlow(deltaTokens)

		if got, want := batcher.ShouldExpediteBatchRequest(), false; got != want {
			t.Errorf("flowControlBatcher.ShouldExpediteBatchRequest(): got %v, want %v", got, want)
		}
		if got, want := batcher.ReleasePendingRequest(), flowControlReq(deltaTokens); !proto.Equal(got, want) {
			t.Errorf("flowControlBatcher.ReleasePendingRequest(): got %v, want %v", got, want)
		}
		if got, want := batcher.RequestForRestart(), flowControlReq(flowControlTokens{Bytes: 600, Messages: 11}); !proto.Equal(got, want) {
			t.Errorf("flowControlBatcher.RequestForRestart(): got %v, want %v", got, want)
		}
	})

	t.Run("OnMessages-Valid", func(t *testing.T) {
		msgs := []*pb.SequencedMessage{seqMsgWithSizeBytes(10), seqMsgWithSizeBytes(20)}
		if gotErr := batcher.OnMessages(msgs); gotErr != nil {
			t.Errorf("flowControlBatcher.OnMessages(): got err (%v), want err <nil>", gotErr)
		}

		if got, want := batcher.ShouldExpediteBatchRequest(), false; got != want {
			t.Errorf("flowControlBatcher.ShouldExpediteBatchRequest(): got %v, want %v", got, want)
		}
		if got, want := batcher.ReleasePendingRequest(), (*pb.FlowControlRequest)(nil); !proto.Equal(got, want) {
			t.Errorf("flowControlBatcher.ReleasePendingRequest(): got %v, want %v", got, want)
		}
		if got, want := batcher.RequestForRestart(), flowControlReq(flowControlTokens{Bytes: 570, Messages: 9}); !proto.Equal(got, want) {
			t.Errorf("flowControlBatcher.RequestForRestart(): got %v, want %v", got, want)
		}
	})

	t.Run("OnMessages-Underflow", func(t *testing.T) {
		msgs := []*pb.SequencedMessage{seqMsgWithSizeBytes(400), seqMsgWithSizeBytes(200)}
		if gotErr, wantErr := batcher.OnMessages(msgs), errTokenCounterBytesNegative; !test.ErrorEqual(gotErr, wantErr) {
			t.Errorf("flowControlBatcher.OnMessages(): got err (%v), want err (%v)", gotErr, wantErr)
		}

		if got, want := batcher.ShouldExpediteBatchRequest(), false; got != want {
			t.Errorf("flowControlBatcher.ShouldExpediteBatchRequest(): got %v, want %v", got, want)
		}
		if got, want := batcher.ReleasePendingRequest(), (*pb.FlowControlRequest)(nil); !proto.Equal(got, want) {
			t.Errorf("flowControlBatcher.ReleasePendingRequest(): got %v, want %v", got, want)
		}
		if got, want := batcher.RequestForRestart(), flowControlReq(flowControlTokens{Bytes: 570, Messages: 9}); !proto.Equal(got, want) {
			t.Errorf("flowControlBatcher.RequestForRestart(): got %v, want %v", got, want)
		}
	})
}

func TestOffsetTrackerRequestForRestart(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		tracker subscriberOffsetTracker
		want    *pb.SeekRequest
	}{
		{
			desc:    "Uninitialized tracker",
			tracker: subscriberOffsetTracker{},
			want:    nil,
		},
		{
			desc:    "Next offset positive",
			tracker: subscriberOffsetTracker{minNextOffset: 1},
			want: &pb.SeekRequest{
				Target: &pb.SeekRequest_Cursor{
					Cursor: &pb.Cursor{Offset: 1},
				},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			got := tc.tracker.RequestForRestart()
			if !proto.Equal(got, tc.want) {
				t.Errorf("subscriberOffsetTracker(%v).RequestForRestart(): got %v, want %v", tc.tracker, got, tc.want)
			}
		})
	}
}

func TestOffsetTrackerOnMessages(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		tracker subscriberOffsetTracker
		msgs    []*pb.SequencedMessage
		want    subscriberOffsetTracker
		wantErr bool
	}{
		{
			desc:    "Uninitialized tracker",
			tracker: subscriberOffsetTracker{},
			msgs:    []*pb.SequencedMessage{seqMsgWithOffset(0)},
			want:    subscriberOffsetTracker{minNextOffset: 1},
		},
		{
			desc:    "Consecutive message offsets",
			tracker: subscriberOffsetTracker{minNextOffset: 5},
			msgs:    []*pb.SequencedMessage{seqMsgWithOffset(5), seqMsgWithOffset(6), seqMsgWithOffset(7)},
			want:    subscriberOffsetTracker{minNextOffset: 8},
		},
		{
			desc:    "Skip message offsets",
			tracker: subscriberOffsetTracker{minNextOffset: 5},
			msgs:    []*pb.SequencedMessage{seqMsgWithOffset(10), seqMsgWithOffset(15)},
			want:    subscriberOffsetTracker{minNextOffset: 16},
		},
		{
			desc:    "Start offset before minNextOffset",
			tracker: subscriberOffsetTracker{minNextOffset: 5},
			msgs:    []*pb.SequencedMessage{seqMsgWithOffset(4)},
			want:    subscriberOffsetTracker{minNextOffset: 5},
			wantErr: true,
		},
		{
			desc:    "Unordered messages",
			tracker: subscriberOffsetTracker{minNextOffset: 5},
			msgs:    []*pb.SequencedMessage{seqMsgWithOffset(5), seqMsgWithOffset(10), seqMsgWithOffset(9)},
			want:    subscriberOffsetTracker{minNextOffset: 5},
			wantErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			err := tc.tracker.OnMessages(tc.msgs)
			if !testutil.Equal(tc.tracker, tc.want, cmp.AllowUnexported(subscriberOffsetTracker{})) {
				t.Errorf("subscriberOffsetTracker().OnMessages(): got %v, want %v", tc.tracker, tc.want)
			}
			if gotErr := err != nil; gotErr != tc.wantErr {
				t.Errorf("subscriberOffsetTracker().OnMessages() error: got (%v), want err=%v", err, tc.wantErr)
			}
		})
	}
}
