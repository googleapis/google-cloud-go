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
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"

	tspb "github.com/golang/protobuf/ptypes/timestamp"
	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

type fakeSource struct {
	ret int64
}

func (f *fakeSource) Int63() int64    { return f.ret }
func (f *fakeSource) Seed(seed int64) {}

type fakeMsgRouter struct {
	multiplier     int
	partitionCount int
}

func (f *fakeMsgRouter) SetPartitionCount(count int) {
	f.partitionCount = count
}

func (f *fakeMsgRouter) Route(orderingKey []byte) int {
	return f.partitionCount * f.multiplier
}

func TestMessageToProto(t *testing.T) {
	for _, tc := range []struct {
		desc string
		msg  *Message
		want *pb.PubSubMessage
	}{
		{
			desc: "valid: minimal",
			msg: &Message{
				Data: []byte("Hello world"),
			},
			want: &pb.PubSubMessage{
				Data: []byte("Hello world"),
			},
		},
		{
			desc: "valid: filled",
			msg: &Message{
				Data: []byte("foo"),
				Attributes: map[string]AttributeValues{
					"attr1": [][]byte{
						[]byte("val1"),
						[]byte("val2"),
					},
				},
				EventTime:   time.Unix(1555593697, 154358*1000),
				OrderingKey: []byte("order"),
			},
			want: &pb.PubSubMessage{
				Data: []byte("foo"),
				Attributes: map[string]*pb.AttributeValues{
					"attr1": {
						Values: [][]byte{
							[]byte("val1"),
							[]byte("val2"),
						},
					},
				},
				EventTime: &tspb.Timestamp{
					Seconds: 1555593697,
					Nanos:   154358 * 1000,
				},
				Key: []byte("order"),
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := tc.msg.toProto()
			if err != nil {
				t.Errorf("toProto() err = %v", err)
			} else if !proto.Equal(got, tc.want) {
				t.Errorf("toProto() got = %v\nwant = %v", got, tc.want)
			}
		})
	}
}

func TestRoundRobinMsgRouter(t *testing.T) {
	// Using the same msgRouter for each test run ensures that it reinitializes
	// when the partition count changes.
	source := &fakeSource{}
	msgRouter := &roundRobinMsgRouter{rng: rand.New(source)}

	for _, tc := range []struct {
		partitionCount int
		source         int64
		want           []int
	}{
		{
			partitionCount: 8,
			source:         9,
			want:           []int{1, 2, 3, 4, 5, 6, 7, 0, 1},
		},
		{
			partitionCount: 5,
			source:         2,
			want:           []int{2, 3, 4, 0, 1, 2},
		},
	} {
		t.Run(fmt.Sprintf("partitionCount=%d", tc.partitionCount), func(t *testing.T) {
			source.ret = tc.source
			msgRouter.SetPartitionCount(tc.partitionCount)
			for i, want := range tc.want {
				got := msgRouter.Route([]byte("IGNORED"))
				if got != want {
					t.Errorf("i=%d: Route() = %d, want = %d", i, got, want)
				}
			}
		})
	}
}

func TestHashingMsgRouter(t *testing.T) {
	// Using the same msgRouter for each test run ensures that it reinitializes
	// when the partition count changes.
	msgRouter := &hashingMsgRouter{}

	keys := [][]byte{
		[]byte("foo1"),
		[]byte("foo2"),
		[]byte("foo3"),
		[]byte("foo4"),
		[]byte("foo5"),
	}

	for _, tc := range []struct {
		partitionCount int
	}{
		{partitionCount: 10},
		{partitionCount: 5},
	} {
		t.Run(fmt.Sprintf("partitionCount=%d", tc.partitionCount), func(t *testing.T) {
			msgRouter.SetPartitionCount(tc.partitionCount)
			for _, key := range keys {
				p1 := msgRouter.Route(key)
				p2 := msgRouter.Route(key)
				if p1 != p2 {
					t.Errorf("Route() returned different partitions for same key %v", key)
				}
				if p1 < 0 || p1 >= tc.partitionCount {
					t.Errorf("Route() returned partition out of range: %v", p1)
				}
			}
		})
	}
}

func TestCompositeMsgRouter(t *testing.T) {
	keyedRouter := &fakeMsgRouter{multiplier: 10}
	keylessRouter := &fakeMsgRouter{multiplier: 100}
	msgRouter := &compositeMsgRouter{
		keyedRouter:   keyedRouter,
		keylessRouter: keylessRouter,
	}

	for _, tc := range []struct {
		desc           string
		partitionCount int
		key            []byte
		want           int
	}{
		{
			desc:           "key",
			partitionCount: 2,
			key:            []byte("foo"),
			want:           20,
		},
		{
			desc:           "nil key",
			partitionCount: 8,
			key:            nil,
			want:           800,
		},
		{
			desc:           "empty key",
			partitionCount: 5,
			key:            []byte{},
			want:           500,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			msgRouter.SetPartitionCount(tc.partitionCount)
			if got := msgRouter.Route(tc.key); got != tc.want {
				t.Errorf("Route() = %d, want = %d", got, tc.want)
			}
		})
	}
}
