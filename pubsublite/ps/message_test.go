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
	"log"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/pubsub"
	"github.com/google/go-cmp/cmp/cmpopts"

	tspb "github.com/golang/protobuf/ptypes/timestamp"
	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

func encodeTimestamp(seconds int64, nanos int32) string {
	val, err := encodeEventTimestamp(&tspb.Timestamp{
		Seconds: seconds,
		Nanos:   nanos,
	})
	if err != nil {
		log.Fatal(err)
	}
	return val
}

func TestMessageTransforms(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		wireMsg *pb.SequencedMessage
		wantMsg *pubsub.Message
		wantErr bool
	}{
		{
			desc: "valid: full message",
			wireMsg: &pb.SequencedMessage{
				Cursor: &pb.Cursor{Offset: 10},
				PublishTime: &tspb.Timestamp{
					Seconds: 1577836800,
					Nanos:   900800700,
				},
				Message: &pb.PubSubMessage{
					Data: []byte("foo"),
					Key:  []byte("bar"),
					EventTime: &tspb.Timestamp{
						Seconds: 1577836800,
						Nanos:   500400300,
					},
					Attributes: map[string]*pb.AttributeValues{
						"attr1": {Values: [][]byte{[]byte("hello")}},
						"attr2": {Values: [][]byte{[]byte("world")}},
					},
				},
			},
			wantMsg: &pubsub.Message{
				ID:          "10",
				PublishTime: time.Unix(1577836800, 900800700),
				Data:        []byte("foo"),
				OrderingKey: "bar",
				Attributes: map[string]string{
					"attr1": "hello",
					"attr2": "world",
					"x-goog-pubsublite-event-time-timestamp-proto": encodeTimestamp(1577836800, 500400300),
				},
			},
		},
		{
			desc: "valid: minimum",
			wireMsg: &pb.SequencedMessage{
				Message: &pb.PubSubMessage{},
			},
			wantMsg: &pubsub.Message{},
		},
		{
			desc:    "invalid: sequenced message nil",
			wantErr: true,
		},
		{
			desc:    "invalid: pubsubmessage nil",
			wireMsg: &pb.SequencedMessage{},
			wantErr: true,
		},
		{
			desc: "invalid: multiple attribute values",
			wireMsg: &pb.SequencedMessage{
				Message: &pb.PubSubMessage{
					Attributes: map[string]*pb.AttributeValues{
						"attr1": {Values: [][]byte{[]byte("hello"), []byte("bar")}},
					},
				},
			},
			wantErr: true,
		},
		{
			desc: "invalid: event time is attribute",
			wireMsg: &pb.SequencedMessage{
				Message: &pb.PubSubMessage{
					Attributes: map[string]*pb.AttributeValues{
						"x-goog-pubsublite-event-time-timestamp-proto": {
							Values: [][]byte{[]byte(encodeTimestamp(1577836800, 500400300))},
						},
					},
				},
			},
			wantErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			gotRecvMsg := new(pubsub.Message)
			gotErr := transformReceivedMessage(tc.wireMsg, gotRecvMsg)
			if (gotErr != nil) != tc.wantErr {
				t.Errorf("transformReceivedMessage() err = (%v), want err=%v", gotErr, tc.wantErr)
			}

			if tc.wantMsg != nil {
				if diff := testutil.Diff(gotRecvMsg, tc.wantMsg, cmpopts.IgnoreUnexported(pubsub.Message{}), cmpopts.EquateEmpty()); diff != "" {
					t.Errorf("transformReceivedMessage() got: -, want: +\n%s", diff)
				}

				// Check reverse conversion equals input.
				gotPubMsg := new(pb.PubSubMessage)
				gotErr := transformPublishedMessage(tc.wantMsg, gotPubMsg, extractOrderingKey)
				if gotErr != nil {
					t.Errorf("transformPublishedMessage() err = (%v)", gotErr)
				}
				if diff := testutil.Diff(gotPubMsg, tc.wireMsg.Message, cmpopts.EquateEmpty()); diff != "" {
					t.Errorf("transformPublishedMessage() got: -, want: +\n%s", diff)
				}
			}
		})
	}
}
