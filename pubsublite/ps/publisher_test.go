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

package ps

import (
	"context"
	"errors"
	"testing"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsublite/internal/test"
	"cloud.google.com/go/pubsublite/internal/wire"
	"cloud.google.com/go/pubsublite/publish"
	"golang.org/x/xerrors"
	"google.golang.org/api/support/bundler"

	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

// mockWirePublisher is a mock implementation of the wire.Publisher interface.
// It uses test.RPCVerifier to install fake PublishResults for each Publish
// call.
type mockWirePublisher struct {
	Verifier *test.RPCVerifier
	Stopped  bool
	err      error
}

func (mp *mockWirePublisher) Publish(msg *pb.PubSubMessage, onResult wire.PublishResultFunc) {
	resp, err := mp.Verifier.Pop(msg)
	if err != nil {
		mp.err = err
		onResult(nil, err)
		return
	}
	result := resp.(*publish.Metadata)
	onResult(result, nil)
}

func (mp *mockWirePublisher) Start()             {}
func (mp *mockWirePublisher) Stop()              { mp.Stopped = true }
func (mp *mockWirePublisher) WaitStarted() error { return mp.err }
func (mp *mockWirePublisher) WaitStopped() error { return mp.err }
func (mp *mockWirePublisher) Error() error       { return mp.err }

func newTestPublisherClient(verifier *test.RPCVerifier, settings PublishSettings) *PublisherClient {
	return &PublisherClient{
		settings: settings,
		wirePub:  &mockWirePublisher{Verifier: verifier},
	}
}

func TestPublisherClientTransformMessage(t *testing.T) {
	ctx := context.Background()
	input := &pubsub.Message{
		Data:        []byte("data"),
		OrderingKey: "ordering_key",
		Attributes:  map[string]string{"attr": "value"},
	}
	fakeResponse := &publish.Metadata{
		Partition: 2,
		Offset:    42,
	}
	wantResultID := "2:42"

	for _, tc := range []struct {
		desc string
		// mutateSettings is passed a copy of DefaultPublishSettings to mutate.
		mutateSettings func(settings *PublishSettings)
		wantMsg        *pb.PubSubMessage
	}{
		{
			desc:           "default settings",
			mutateSettings: func(settings *PublishSettings) {},
			wantMsg: &pb.PubSubMessage{
				Data: []byte("data"),
				Key:  []byte("ordering_key"),
				Attributes: map[string]*pb.AttributeValues{
					"attr": {Values: [][]byte{[]byte("value")}},
				},
			},
		},
		{
			desc: "custom key extractor",
			mutateSettings: func(settings *PublishSettings) {
				settings.KeyExtractor = func(msg *pubsub.Message) []byte {
					return msg.Data
				}
			},
			wantMsg: &pb.PubSubMessage{
				Data: []byte("data"),
				Key:  []byte("data"),
				Attributes: map[string]*pb.AttributeValues{
					"attr": {Values: [][]byte{[]byte("value")}},
				},
			},
		},
		{
			desc: "custom message transformer",
			mutateSettings: func(settings *PublishSettings) {
				settings.KeyExtractor = func(msg *pubsub.Message) []byte {
					return msg.Data
				}
				settings.MessageTransformer = func(from *pubsub.Message, to *pb.PubSubMessage) error {
					// Swaps data and key.
					to.Data = []byte(from.OrderingKey)
					to.Key = from.Data
					return nil
				}
			},
			wantMsg: &pb.PubSubMessage{
				Data: []byte("ordering_key"),
				Key:  []byte("data"),
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			settings := DefaultPublishSettings
			tc.mutateSettings(&settings)

			verifier := test.NewRPCVerifier(t)
			verifier.Push(tc.wantMsg, fakeResponse, nil)
			defer verifier.Flush()

			pubClient := newTestPublisherClient(verifier, settings)
			result := pubClient.Publish(ctx, input)

			gotID, err := result.Get(ctx)
			if err != nil {
				t.Errorf("Publish() got err: %v", err)
			}
			if gotID != wantResultID {
				t.Errorf("Publish() got id: %q, want: %q", gotID, wantResultID)
			}
		})
	}
}

func TestPublisherClientTransformMessageError(t *testing.T) {
	wantErr := errors.New("message could not be converted")

	settings := DefaultPublishSettings
	settings.MessageTransformer = func(_ *pubsub.Message, _ *pb.PubSubMessage) error {
		return wantErr
	}

	// No publish calls expected.
	verifier := test.NewRPCVerifier(t)
	defer verifier.Flush()

	ctx := context.Background()
	input := &pubsub.Message{
		Data: []byte("data"),
	}
	pubClient := newTestPublisherClient(verifier, settings)
	result := pubClient.Publish(ctx, input)

	_, gotErr := result.Get(ctx)
	if !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("Publish() got err: (%v), want err: (%v)", gotErr, wantErr)
	}
	if !test.ErrorEqual(pubClient.Error(), wantErr) {
		t.Errorf("PublisherClient.Error() got: (%v), want: (%v)", pubClient.Error(), wantErr)
	}
	if got, want := pubClient.wirePub.(*mockWirePublisher).Stopped, true; got != want {
		t.Errorf("Publisher.Stopped: got %v, want %v", got, want)
	}
}

func TestPublisherClientTranslatePublishResultErrors(t *testing.T) {
	ctx := context.Background()
	input := &pubsub.Message{
		Data:        []byte("data"),
		OrderingKey: "ordering_key",
	}
	wantMsg := &pb.PubSubMessage{
		Data: []byte("data"),
		Key:  []byte("ordering_key"),
	}

	for _, tc := range []struct {
		desc    string
		wireErr error
		wantErr error
	}{
		{
			desc:    "oversized message",
			wireErr: wire.ErrOversizedMessage,
			wantErr: bundler.ErrOversizedItem,
		},
		{
			desc:    "oversized message wrapped",
			wireErr: xerrors.Errorf("placeholder error message: %w", wire.ErrOversizedMessage),
			wantErr: bundler.ErrOversizedItem,
		},
		{
			desc:    "buffer overflow",
			wireErr: wire.ErrOverflow,
			wantErr: bundler.ErrOverflow,
		},
		{
			desc:    "service stopped",
			wireErr: wire.ErrServiceStopped,
			wantErr: wire.ErrServiceStopped,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			verifier := test.NewRPCVerifier(t)
			verifier.Push(wantMsg, nil, tc.wireErr)
			defer verifier.Flush()

			pubClient := newTestPublisherClient(verifier, DefaultPublishSettings)
			result := pubClient.Publish(ctx, input)

			_, gotErr := result.Get(ctx)
			if !test.ErrorEqual(gotErr, tc.wantErr) {
				t.Errorf("Publish() got err: (%v), want err: (%v)", gotErr, tc.wantErr)
			}
			if !test.ErrorEqual(pubClient.Error(), tc.wireErr) {
				t.Errorf("PublisherClient.Error() got: (%v), want: (%v)", pubClient.Error(), tc.wireErr)
			}
			if got, want := pubClient.wirePub.(*mockWirePublisher).Stopped, false; got != want {
				t.Errorf("Publisher.Stopped: got %v, want %v", got, want)
			}
		})
	}
}
