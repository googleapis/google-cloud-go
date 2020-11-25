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
	"crypto/sha256"
	"math/big"
	"math/rand"
)

// messageRouter outputs a partition number, given an ordering key (which it may
// ignore).
type messageRouter interface {
	Route(orderingKey []byte) int
}

// roundRobinMsgRouter sequentially cycles through partition numbers, starting
// from a random partition.
type roundRobinMsgRouter struct {
	rng            *rand.Rand
	partitionCount int
	nextPartition  int
}

func newRoundRobinMsgRouter(rng *rand.Rand, count int) *roundRobinMsgRouter {
	return &roundRobinMsgRouter{
		rng:            rng,
		partitionCount: count,
		nextPartition:  int(rng.Int63n(int64(count))),
	}
}

func (r *roundRobinMsgRouter) Route(_ []byte) (partition int) {
	partition = r.nextPartition
	r.nextPartition = (partition + 1) % r.partitionCount
	return
}

// hashingMsgRouter hashes an ordering key using SHA256 to obtain a partition
// number. It should only be used for messages with an ordering key.
//
// Matches implementation at:
// https://github.com/googleapis/java-pubsublite/blob/master/google-cloud-pubsublite/src/main/java/com/google/cloud/pubsublite/internal/DefaultRoutingPolicy.java
type hashingMsgRouter struct {
	partitionCount *big.Int
}

func newHashingMsgRouter(count int) *hashingMsgRouter {
	return &hashingMsgRouter{
		partitionCount: big.NewInt(int64(count)),
	}
}

func (r *hashingMsgRouter) Route(orderingKey []byte) int {
	if len(orderingKey) == 0 {
		return -1
	}
	h := sha256.Sum256(orderingKey)
	num := new(big.Int).SetBytes(h[:])
	partition := new(big.Int).Mod(num, r.partitionCount)
	return int(partition.Int64())
}

// compositeMsgRouter delegates to different message routers for messages
// with/without ordering keys.
type compositeMsgRouter struct {
	keyedRouter   messageRouter
	keylessRouter messageRouter
}

func (r *compositeMsgRouter) Route(orderingKey []byte) int {
	if len(orderingKey) > 0 {
		return r.keyedRouter.Route(orderingKey)
	}
	return r.keylessRouter.Route(orderingKey)
}

type messageRouterFactory struct {
	rng *rand.Rand
}

func newMessageRouterFactory(rng *rand.Rand) *messageRouterFactory {
	return &messageRouterFactory{rng: rng}
}

// New returns a compositeMsgRouter that uses hashingMsgRouter for messages with
// ordering key and roundRobinMsgRouter for messages without.
func (f *messageRouterFactory) New(partitionCount int) messageRouter {
	return &compositeMsgRouter{
		keyedRouter:   newHashingMsgRouter(partitionCount),
		keylessRouter: newRoundRobinMsgRouter(f.rng, partitionCount),
	}
}
