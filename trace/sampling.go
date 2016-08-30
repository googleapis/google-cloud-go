// Copyright 2016 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package trace

import (
	crand "crypto/rand"
	"encoding/binary"
	"math/rand"
	"sync"

	"golang.org/x/time/rate"
)

type Policy interface {
	// Sample determines whether to sample the next request.  If so, it
	// also returns a string and rate describing the policy by which the
	// request was chosen.
	Sample() (sample bool, policy string, rate float64)
}

type sampler struct {
	fraction float64
	*rate.Limiter
	*rand.Rand
	sync.Mutex
}

func (s *sampler) Sample() (sample bool, policy string, rate float64) {
	s.Lock()
	x := s.Float64()
	s.Unlock()
	if x >= s.fraction || !s.Allow() {
		return false, "", 0.0
	}
	if s.fraction < 1.0 {
		return true, "fraction", s.fraction
	}
	return true, "qps", float64(s.Limit())
}

// NewSampler returns a sampling policy that traces a given fraction of
// requests, and enforces a limit on the number of traces per second.
func NewSampler(fraction, maxqps float64) Policy {
	if !(fraction > 0) || !(maxqps > 0) {
		return nil
	}
	maxTokens := 100
	if maxqps < 99.0 {
		maxTokens = 1 + int(maxqps)
	}
	var seed int64
	binary.Read(crand.Reader, binary.LittleEndian, &seed)
	s := sampler{
		fraction: fraction,
		Limiter:  rate.NewLimiter(rate.Limit(maxqps), maxTokens),
		Rand:     rand.New(rand.NewSource(seed)),
	}
	return &s
}
