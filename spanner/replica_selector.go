/*
Copyright 2026 Google LLC

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
	"math"
	"math/rand"
)

type replicaSelector interface {
	selectReplica(candidates []channelEndpoint, scoreLookup func(channelEndpoint) float64) channelEndpoint
}

type powerOfTwoReplicaSelector struct {
	randomIntn func(int) int
}

var _ replicaSelector = (*powerOfTwoReplicaSelector)(nil)

func newPowerOfTwoReplicaSelector() *powerOfTwoReplicaSelector {
	return newPowerOfTwoReplicaSelectorWithRandom(rand.Intn)
}

func newPowerOfTwoReplicaSelectorWithRandom(randomIntn func(int) int) *powerOfTwoReplicaSelector {
	if randomIntn == nil {
		randomIntn = rand.Intn
	}
	return &powerOfTwoReplicaSelector{randomIntn: randomIntn}
}

func (s *powerOfTwoReplicaSelector) selectReplica(candidates []channelEndpoint, scoreLookup func(channelEndpoint) float64) channelEndpoint {
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) == 1 {
		return candidates[0]
	}
	if scoreLookup == nil {
		scoreLookup = func(channelEndpoint) float64 { return math.MaxFloat64 }
	}

	index1 := s.randomIntn(len(candidates))
	index2 := s.randomIntn(len(candidates) - 1)
	if index2 >= index1 {
		index2++
	}

	candidate1 := candidates[index1]
	candidate2 := candidates[index2]
	score1 := sanitizeReplicaScore(scoreLookup(candidate1))
	score2 := sanitizeReplicaScore(scoreLookup(candidate2))
	if score1 <= score2 {
		return candidate1
	}
	return candidate2
}

func sanitizeReplicaScore(score float64) float64 {
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return math.MaxFloat64
	}
	return score
}
