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

package pscompat

import (
	"context"
	"errors"
	"testing"
	"time"

	pubsub "cloud.google.com/go/internal/pubsub"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/pubsublite/internal/test"
	"cloud.google.com/go/pubsublite/internal/wire"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/sync/errgroup"

	pb "cloud.google.com/go/pubsublite/apiv1/pubsublitepb"
	tspb "google.golang.org/protobuf/types/known/timestamppb"
)

const (
	defaultSubscriberTestTimeout = 30 * time.Second
	activePartition              = 1
)

// mockAckConsumer is a mock implementation of the wire.AckConsumer interface.
type mockAckConsumer struct {
	AckCount int
}

func (ac *mockAckConsumer) Ack() {
	ac.AckCount++
}

// mockWireSubscriber is a mock implementation of the wire.Subscriber interface.
type mockWireSubscriber struct {
	receiver         wire.MessageReceiverFunc
	onReassignment   wire.ReassignmentHandlerFunc
	activePartitions wire.PartitionSet
	msgsC            chan *wire.ReceivedMessage
	stopC            chan struct{}
	err              error
	Stopped          bool
	Terminated       bool
}

// DeliverMessages should be called from the test to simulate a message
// delivery.
func (ms *mockWireSubscriber) DeliverMessages(msgs ...*wire.ReceivedMessage) {
	for _, m := range msgs {
		ms.msgsC <- m
	}
}

// OnReassignment should be called from the test to simulate a partition
// reassignment.
func (ms *mockWireSubscriber) DeliverReassignment(before, after wire.PartitionSet) {
	if err := ms.onReassignment(before, after); err != nil {
		ms.SimulateFatalError(err)
	}
}

// SimulateFatalError should be called from the test to simulate a fatal error
// occurring in the wire subscriber.
func (ms *mockWireSubscriber) SimulateFatalError(err error) {
	ms.err = err
	close(ms.stopC)
}

// wire.Subscriber implementation

func (ms *mockWireSubscriber) Start() {
	go func() {
		for {
			// Ensure stop has higher priority.
			select {
			case <-ms.stopC:
				return // Exit goroutine
			default:
			}

			select {
			case <-ms.stopC:
				return // Exit goroutine
			case msg := <-ms.msgsC:
				ms.receiver(msg)
			}
		}
	}()
}

func (ms *mockWireSubscriber) WaitStarted() error {
	return nil
}

func (ms *mockWireSubscriber) Stop() {
	if !ms.Stopped && !ms.Terminated {
		ms.Stopped = true
		close(ms.stopC)
	}
}

func (ms *mockWireSubscriber) Terminate() {
	if !ms.Stopped && !ms.Terminated {
		ms.Terminated = true
		close(ms.stopC)
	}
}

func (ms *mockWireSubscriber) WaitStopped() error {
	<-ms.stopC // Wait until Stopped
	return ms.err
}

func (ms *mockWireSubscriber) PartitionActive(partition int) bool {
	return ms.activePartitions.Contains(partition)
}

type mockWireSubscriberFactory struct{}

func (f *mockWireSubscriberFactory) New(ctx context.Context, receiver wire.MessageReceiverFunc, onReassignment wire.ReassignmentHandlerFunc) (wire.Subscriber, error) {
	return &mockWireSubscriber{
		receiver:         receiver,
		onReassignment:   onReassignment,
		activePartitions: wire.NewPartitionSet([]int{activePartition}),
		msgsC:            make(chan *wire.ReceivedMessage, 10),
		stopC:            make(chan struct{}),
	}, nil
}

func newTestSubscriberInstance(ctx context.Context, settings ReceiveSettings, receiver messageReceiverFunc) *subscriberInstance {
	sub, _ := newSubscriberInstance(ctx, context.Background(), new(mockWireSubscriberFactory), settings, receiver)
	return sub
}

func TestSubscriberInstanceTransformMessage(t *testing.T) {
	const partition = 3
	ctx := context.Background()
	input := &pb.SequencedMessage{
		Message: &pb.PubSubMessage{
			Data: []byte("data"),
			Key:  []byte("key"),
			Attributes: map[string]*pb.AttributeValues{
				"attr": {Values: [][]byte{[]byte("value")}},
			},
		},
		Cursor: &pb.Cursor{Offset: 123},
		PublishTime: &tspb.Timestamp{
			Seconds: 1577836800,
			Nanos:   900800700,
		},
	}

	for _, tc := range []struct {
		desc string
		// mutateSettings is passed a copy of DefaultReceiveSettings to mutate.
		mutateSettings func(settings *ReceiveSettings)
		want           *pubsub.Message
	}{
		{
			desc:           "default settings",
			mutateSettings: func(settings *ReceiveSettings) {},
			want: &pubsub.Message{
				Data:        []byte("data"),
				OrderingKey: "key",
				Attributes:  map[string]string{"attr": "value"},
				ID:          "3:123",
				PublishTime: time.Unix(1577836800, 900800700),
			},
		},
		{
			desc: "custom message transformer",
			mutateSettings: func(settings *ReceiveSettings) {
				settings.MessageTransformer = func(from *pb.SequencedMessage, to *pubsub.Message) error {
					// Swaps data and key.
					to.OrderingKey = string(from.Message.Data)
					to.Data = from.Message.Key
					return nil
				}
			},
			want: &pubsub.Message{
				Data:        []byte("key"),
				OrderingKey: "data",
				ID:          "3:123",
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			settings := DefaultReceiveSettings
			tc.mutateSettings(&settings)

			ack := &mockAckConsumer{}
			msg := &wire.ReceivedMessage{Msg: input, Ack: ack, Partition: partition}

			cctx, stopSubscriber := context.WithTimeout(ctx, defaultSubscriberTestTimeout)
			messageReceiver := func(ctx context.Context, got *pubsub.Message) {
				if diff := testutil.Diff(got, tc.want, cmpopts.IgnoreUnexported(pubsub.Message{}), cmpopts.EquateEmpty()); diff != "" {
					t.Errorf("Received message got: -, want: +\n%s", diff)
				}
				got.Ack()
				got.Nack() // Should be ignored
				stopSubscriber()
			}
			subInstance := newTestSubscriberInstance(cctx, settings, messageReceiver)
			subInstance.wireSub.(*mockWireSubscriber).DeliverMessages(msg)

			if err := subInstance.Wait(cctx); err != nil {
				t.Errorf("subscriberInstance.Wait() got err: %v", err)
			}
			if got, want := ack.AckCount, 1; got != want {
				t.Errorf("mockAckConsumer.AckCount: got %d, want %d", got, want)
			}
			if got, want := subInstance.recvCtx.Err(), context.Canceled; !test.ErrorEqual(got, want) {
				t.Errorf("subscriberInstance.recvCtx.Err(): got (%v), want (%v)", got, want)
			}
			if got, want := subInstance.wireSub.(*mockWireSubscriber).Stopped, true; got != want {
				t.Errorf("mockWireSubscriber.Stopped: got %v, want %v", got, want)
			}
		})
	}
}

func TestSubscriberInstanceTransformMessageError(t *testing.T) {
	transformErr := errors.New("message could not be converted")

	for _, tc := range []struct {
		desc        string
		transformer ReceiveMessageTransformerFunc
		wantErr     error
	}{
		{
			desc: "returns error",
			transformer: func(_ *pb.SequencedMessage, _ *pubsub.Message) error {
				return transformErr
			},
			wantErr: transformErr,
		},
		{
			desc: "sets message id",
			transformer: func(_ *pb.SequencedMessage, out *pubsub.Message) error {
				out.ID = "should_not_be_set"
				return nil
			},
			wantErr: errMessageIDSet,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			settings := DefaultReceiveSettings
			settings.MessageTransformer = tc.transformer

			ctx := context.Background()
			ack := &mockAckConsumer{}
			msg := &wire.ReceivedMessage{
				Ack: ack,
				Msg: &pb.SequencedMessage{
					Message: &pb.PubSubMessage{Data: []byte("data")},
				},
			}

			cctx, cancel := context.WithTimeout(ctx, defaultSubscriberTestTimeout)
			_ = cancel
			messageReceiver := func(ctx context.Context, got *pubsub.Message) {
				t.Errorf("Received unexpected message: %v", got)
				got.Nack()
			}
			subInstance := newTestSubscriberInstance(cctx, settings, messageReceiver)
			subInstance.wireSub.(*mockWireSubscriber).DeliverMessages(msg)

			if gotErr := subInstance.Wait(cctx); !test.ErrorEqual(gotErr, tc.wantErr) {
				t.Errorf("subscriberInstance.Wait() got err: (%v), want: (%v)", gotErr, tc.wantErr)
			}
			if got, want := ack.AckCount, 0; got != want {
				t.Errorf("mockAckConsumer.AckCount: got %d, want %d", got, want)
			}
			if got, want := subInstance.recvCtx.Err(), context.Canceled; !test.ErrorEqual(got, want) {
				t.Errorf("subscriberInstance.recvCtx.Err(): got (%v), want (%v)", got, want)
			}
			if got, want := subInstance.wireSub.(*mockWireSubscriber).Terminated, true; got != want {
				t.Errorf("mockWireSubscriber.Terminated: got %v, want %v", got, want)
			}
		})
	}
}

func TestSubscriberInstanceNack(t *testing.T) {
	nackErr := errors.New("message nacked")

	ctx := context.Background()
	msg := &pb.SequencedMessage{
		Message: &pb.PubSubMessage{
			Data: []byte("data"),
			Key:  []byte("key"),
		},
	}

	for _, tc := range []struct {
		desc string
		// mutateSettings is passed a copy of DefaultReceiveSettings to mutate.
		mutateSettings func(settings *ReceiveSettings)
		msgPartition   int
		wantErr        error
		wantAckCount   int
		wantStopped    bool
		wantTerminated bool
	}{
		{
			desc:           "default settings",
			mutateSettings: func(settings *ReceiveSettings) {},
			msgPartition:   activePartition,
			wantErr:        errNackCalled,
			wantAckCount:   0,
			wantTerminated: true,
		},
		{
			desc:           "message partition inactive",
			mutateSettings: func(settings *ReceiveSettings) {},
			msgPartition:   activePartition + 1,
			wantErr:        nil,
			wantAckCount:   0,
			wantStopped:    true,
		},
		{
			desc: "nack handler returns nil",
			mutateSettings: func(settings *ReceiveSettings) {
				settings.NackHandler = func(_ *pubsub.Message) error {
					return nil
				}
			},
			msgPartition: activePartition,
			wantErr:      nil,
			wantAckCount: 1,
			wantStopped:  true,
		},
		{
			desc: "nack handler returns error",
			mutateSettings: func(settings *ReceiveSettings) {
				settings.NackHandler = func(_ *pubsub.Message) error {
					return nackErr
				}
			},
			msgPartition:   activePartition,
			wantErr:        nackErr,
			wantAckCount:   0,
			wantTerminated: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			settings := DefaultReceiveSettings
			tc.mutateSettings(&settings)

			ack := &mockAckConsumer{}
			msg := &wire.ReceivedMessage{Msg: msg, Ack: ack, Partition: tc.msgPartition}

			cctx, stopSubscriber := context.WithTimeout(ctx, defaultSubscriberTestTimeout)
			messageReceiver := func(ctx context.Context, got *pubsub.Message) {
				got.Nack()

				// Only need to stop the subscriber when the nack handler actually acks
				// the message. For other cases, the subscriber is forcibly terminated.
				if tc.wantErr == nil {
					stopSubscriber()
				}
			}
			subInstance := newTestSubscriberInstance(cctx, settings, messageReceiver)
			subInstance.wireSub.(*mockWireSubscriber).DeliverMessages(msg)

			if gotErr := subInstance.Wait(cctx); !test.ErrorEqual(gotErr, tc.wantErr) {
				t.Errorf("subscriberInstance.Wait() got err: (%v), want: (%v)", gotErr, tc.wantErr)
			}
			if got, want := ack.AckCount, tc.wantAckCount; got != want {
				t.Errorf("mockAckConsumer.AckCount: got %d, want %d", got, want)
			}
			if got, want := subInstance.recvCtx.Err(), context.Canceled; !test.ErrorEqual(got, want) {
				t.Errorf("subscriberInstance.recvCtx.Err(): got (%v), want (%v)", got, want)
			}
			if got, want := subInstance.wireSub.(*mockWireSubscriber).Stopped, tc.wantStopped; got != want {
				t.Errorf("mockWireSubscriber.Stopped: got %v, want %v", got, want)
			}
			if got, want := subInstance.wireSub.(*mockWireSubscriber).Terminated, tc.wantTerminated; got != want {
				t.Errorf("mockWireSubscriber.Terminated: got %v, want %v", got, want)
			}
		})
	}
}

func TestSubscriberInstanceReassignmentHandler(t *testing.T) {
	reassignmentErr := errors.New("reassignment failure")
	before := wire.NewPartitionSet([]int{3, 2, 1})
	after := wire.NewPartitionSet([]int{4, 5, 3})
	ctx := context.Background()

	for _, tc := range []struct {
		desc string
		// mutateSettings is passed a copy of DefaultReceiveSettings to mutate.
		mutateSettings func(settings *ReceiveSettings)
		wantErr        error
	}{
		{
			desc:           "default settings",
			mutateSettings: func(settings *ReceiveSettings) {},
		},
		{
			desc: "reassignment handler returns nil",
			mutateSettings: func(settings *ReceiveSettings) {
				settings.ReassignmentHandler = func(before, after []int) error {
					if got, want := before, []int{1, 2, 3}; !testutil.Equal(got, want) {
						t.Errorf("before: got %d, want %d", got, want)
					}
					if got, want := after, []int{3, 4, 5}; !testutil.Equal(got, want) {
						t.Errorf("after: got %d, want %d", got, want)
					}
					return nil
				}
			},
		},
		{
			desc: "reassignment handler returns error",
			mutateSettings: func(settings *ReceiveSettings) {
				settings.ReassignmentHandler = func(before, after []int) error {
					return reassignmentErr
				}
			},
			wantErr: reassignmentErr,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			settings := DefaultReceiveSettings
			tc.mutateSettings(&settings)

			cctx, stopSubscriber := context.WithTimeout(ctx, defaultSubscriberTestTimeout)
			// make vet happy, we need to explicitly control calling this for failure cases
			_ = stopSubscriber
			messageReceiver := func(ctx context.Context, got *pubsub.Message) {
				t.Error("Message receiver should not be called")
			}
			subInstance := newTestSubscriberInstance(cctx, settings, messageReceiver)
			subInstance.wireSub.(*mockWireSubscriber).DeliverReassignment(before, after)
			if tc.wantErr == nil {
				stopSubscriber()
			}

			if gotErr := subInstance.Wait(cctx); !test.ErrorEqual(gotErr, tc.wantErr) {
				t.Errorf("subscriberInstance.Wait() got err: (%v), want err: (%v)", gotErr, tc.wantErr)
			}
		})
	}
}

func TestSubscriberInstanceWireSubscriberFails(t *testing.T) {
	fatalErr := errors.New("server error")

	ctx := context.Background()
	msg := &wire.ReceivedMessage{
		Ack: &mockAckConsumer{},
		Msg: &pb.SequencedMessage{
			Message: &pb.PubSubMessage{Data: []byte("data")},
		},
	}

	cctx, cancel := context.WithTimeout(ctx, defaultSubscriberTestTimeout)
	_ = cancel
	messageReceiver := func(ctx context.Context, got *pubsub.Message) {
		// Verifies that receivers are notified via ctx.Done when the subscriber is
		// shutting down.
		select {
		case <-time.After(defaultSubscriberTestTimeout):
			t.Errorf("MessageReceiverFunc context not closed within %v", defaultSubscriberTestTimeout)
		case <-ctx.Done():
		}
	}
	subInstance := newTestSubscriberInstance(cctx, DefaultReceiveSettings, messageReceiver)
	subInstance.wireSub.(*mockWireSubscriber).DeliverMessages(msg)
	time.AfterFunc(100*time.Millisecond, func() {
		// Simulates a fatal server error that causes the wire subscriber to
		// terminate from within.
		subInstance.wireSub.(*mockWireSubscriber).SimulateFatalError(fatalErr)
	})

	if gotErr := subInstance.Wait(cctx); !test.ErrorEqual(gotErr, fatalErr) {
		t.Errorf("subscriberInstance.Wait() got err: (%v), want: (%v)", gotErr, fatalErr)
	}
	if got, want := subInstance.recvCtx.Err(), context.Canceled; !test.ErrorEqual(got, want) {
		t.Errorf("subscriberInstance.recvCtx.Err(): got (%v), want (%v)", got, want)
	}
	if got, want := subInstance.wireSub.(*mockWireSubscriber).Stopped, false; got != want {
		t.Errorf("mockWireSubscriber.Stopped: got %v, want %v", got, want)
	}
	if got, want := subInstance.wireSub.(*mockWireSubscriber).Terminated, false; got != want {
		t.Errorf("mockWireSubscriber.Terminated: got %v, want %v", got, want)
	}
}

func TestSubscriberClientDuplicateReceive(t *testing.T) {
	ctx := context.Background()
	subClient := &SubscriberClient{
		settings:       DefaultReceiveSettings,
		wireSubFactory: new(mockWireSubscriberFactory),
	}

	messageReceiver := func(_ context.Context, got *pubsub.Message) {
		t.Errorf("No messages expected, got: %v", got)
	}

	g, gctx := errgroup.WithContext(ctx)
	for i := 0; i < 3; i++ {
		// Receive() is blocking, so we must start them in goroutines. Passing gctx
		// to Receive will stop the subscribers once the first error occurs.
		g.Go(func() error {
			return subClient.Receive(gctx, messageReceiver)
		})
	}
	if gotErr, wantErr := g.Wait(), errDuplicateReceive; !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("SubscriberClient.Receive() got err: (%v), want: (%v)", gotErr, wantErr)
	}
}
