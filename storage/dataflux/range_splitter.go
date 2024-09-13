// Copyright 2024 Google LLC
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

package dataflux

import (
	"sync"
)

// rangeSplitter specifies the a list and a map of sorted alphabets.
type rangeSplitter struct {
	mu             sync.Mutex
	sortedAlphabet *[]rune
	alphabetMap    map[rune]int
}

// listRange specifies the start and end range for the range splitter.
type listRange struct {
	startRange string
	endRange   string
}

// newRangeSplitter creates a new RangeSplitter with the given alphabets.
func newRangeSplitter(alphabet string) *rangeSplitter {
	return &rangeSplitter{}
}

// splitRange creates a given number of splits based on a provided start and end range.
func (rs *rangeSplitter) splitRange(startRange, endRange string, numSplits int) ([]string, error) {
	return nil, nil
}
