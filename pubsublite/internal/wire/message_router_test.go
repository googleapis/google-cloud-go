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

	"cloud.google.com/go/pubsublite/internal/test"
)

func TestRoundRobinMsgRouter(t *testing.T) {
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
			source := &test.FakeSource{Ret: tc.source}
			msgRouter := newRoundRobinMsgRouter(rand.New(source), tc.partitionCount)

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
			msgRouter := newHashingMsgRouter(tc.partitionCount)
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

type fakeMsgRouter struct {
	multiplier     int
	partitionCount int
}

func (f *fakeMsgRouter) Route(orderingKey []byte) int {
	return f.partitionCount * f.multiplier
}

func TestCompositeMsgRouter(t *testing.T) {
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
			msgRouter := &compositeMsgRouter{
				keyedRouter: &fakeMsgRouter{
					multiplier:     10,
					partitionCount: tc.partitionCount,
				},
				keylessRouter: &fakeMsgRouter{
					multiplier:     100,
					partitionCount: tc.partitionCount,
				},
			}

			if got := msgRouter.Route(tc.key); got != tc.want {
				t.Errorf("Route() = %d, want = %d", got, tc.want)
			}
		})
	}
}
