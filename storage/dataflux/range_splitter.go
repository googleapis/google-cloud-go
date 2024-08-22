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
func newRangeSplitter(alphabet string) (*rangeSplitter, error) {

	return &rangeSplitter{}, nil
}

// splitRange creates a given number of splits based on a provided start and end range.
func (rs *rangeSplitter) splitRange(startRange, endRange string, numSplits int) ([]string, error) {
	return nil, nil
}
