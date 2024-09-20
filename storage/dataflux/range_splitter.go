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
	"fmt"
	"math/big"
	"sort"
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

// minimalIntRange specifies start and end range in base-10 form, along with the
// minimal string length for the split range strings.
type minimalIntRange struct {
	startInteger  *big.Int
	endInteger    *big.Int
	minimalLength int
}

// generateSplitsOpts specifies the parameters needed to generate the split
// range strings.
type generateSplitsOpts struct {
	minimalIntRange *minimalIntRange
	numSplits       int
	startRange      string
	endRange        string
}

// newRangeSplitter creates a new RangeSplitter with the given alphabets.
func newRangeSplitter(alphabet string) (*rangeSplitter, error) {

	// Validate that we do not have empty alphabet passed in.
	if len(alphabet) == 0 {
		return nil, fmt.Errorf("no alphabet specified for the range splitter")
	}
	// Sort the alphabet lexicographically and store a mapping of each alphabet
	// to its index. We need a mapping for efficient index lookup in later operations.
	sortedAlphabet := sortAlphabet([]rune(alphabet))
	alphabetMap := constructAlphabetMap(sortedAlphabet)

	return &rangeSplitter{
		alphabetMap:    alphabetMap,
		sortedAlphabet: sortedAlphabet,
	}, nil
}

// splitRange creates a given number of splits based on a provided start and end range.
func (rs *rangeSplitter) splitRange(startRange, endRange string, numSplits int) ([]string, error) {
	// Number of splits has to be at least one, otherwise it is not splittable.
	if numSplits < 1 {
		return nil, fmt.Errorf("number of splits should be at least 1, got %d", numSplits)
	}

	// End range (if specified) has to be lexicographically greater than the start range
	// for the range to be valid.
	if len(endRange) != 0 && startRange >= endRange {
		return nil, fmt.Errorf("start range %q cannot be lexicographically greater end range %q", startRange, endRange)
	}

	rs.addCharsToAlphabet([]rune(startRange))
	rs.addCharsToAlphabet([]rune(endRange))

	// Validate start range characters and convert into character array form.
	startRangeCharArray, err := rs.convertRangeStringToArray(startRange)
	if err != nil {
		return nil, fmt.Errorf("unable to convert start range %q to array: %v", startRange, err)
	}

	// Validate end range characters and convert into character array form.
	endRangeCharArray, err := rs.convertRangeStringToArray(endRange)
	if err != nil {
		return nil, fmt.Errorf("unable to convert end range %q to array: %v", endRange, err)
	}

	// Construct the final split ranges to be returned.
	var splitPoints []string

	// If the start and end string ranges are equal with padding, no splitting is
	// necessary. In such cases, an empty array of split ranges is returned.
	if rs.isRangeEqualWithPadding(startRangeCharArray, endRangeCharArray) {
		return splitPoints, nil
	}
	// Convert the range strings from base-N to base-10 and employ a greedy approach
	// to determine the smallest splittable integer range difference.
	minimalIntRange, err := rs.convertStringRangeToMinimalIntRange(
		startRangeCharArray, endRangeCharArray, numSplits)
	if err != nil {
		return nil, fmt.Errorf("range splitting with start range %q and end range %q: %v",
			startRange, endRange, err)
	}

	// Generate the split points and return them.
	splitPoints = rs.generateSplits(generateSplitsOpts{
		startRange:      startRange,
		endRange:        endRange,
		numSplits:       numSplits,
		minimalIntRange: minimalIntRange,
	})

	return splitPoints, nil
}

// generateSplits generates the split points using the specified options.
func (rs *rangeSplitter) generateSplits(opts generateSplitsOpts) []string {

	startInteger := opts.minimalIntRange.startInteger
	endInteger := opts.minimalIntRange.endInteger
	minimalLength := opts.minimalIntRange.minimalLength

	rangeDifference := new(big.Int).Sub(endInteger, startInteger)

	var splitPoints []string

	// The number of intervals is one more than the number of split points.
	rangeInterval := new(big.Int).SetInt64(int64(opts.numSplits + 1))

	for i := 1; i <= opts.numSplits; i++ {
		// Combine the range interval and index to determine the split point in base-10 form.
		rangeDiffWithIdx := new(big.Int).Mul(rangeDifference, big.NewInt(int64(i)))
		rangeInterval := new(big.Int).Div(rangeDiffWithIdx, rangeInterval)
		splitPoint := new(big.Int).Add(rangeInterval, startInteger)

		// Convert the split point back from base-10 to base-N.
		splitString := rs.convertIntToString(splitPoint, minimalLength)

		// Due to the approximate nature on how the minimal int range is derived, we need to perform
		// another validation to check to ensure each split point falls in valid range.
		isGreaterThanStart := len(splitString) > 0 && splitString > opts.startRange
		isLessThanEnd := len(opts.endRange) == 0 || (len(splitString) > 0 && splitString < opts.endRange)
		if isGreaterThanStart && isLessThanEnd {
			splitPoints = append(splitPoints, splitString)
		}
	}
	return splitPoints
}

// sortAlphabet sorts the alphabets string lexicographically and returns a pointer to the sorted string.
func sortAlphabet(unsortedAlphabet []rune) *[]rune {
	sortedAlphabet := unsortedAlphabet
	sort.Slice(sortedAlphabet, func(i, j int) bool {
		return sortedAlphabet[i] < sortedAlphabet[j]
	})
	return &sortedAlphabet
}

// constructAlphabetMap constructs a mapping from each character in the
// alphabets to its index in the alphabet array.
func constructAlphabetMap(alphabet *[]rune) map[rune]int {
	alphabetMap := make(map[rune]int)
	for i, char := range *alphabet {
		alphabetMap[char] = i
	}
	return alphabetMap
}

// addCharsToAlphabet adds a character to the known alphabet.
func (rs *rangeSplitter) addCharsToAlphabet(characters []rune) {
	rs.mu.Lock()         // Acquire the lock
	defer rs.mu.Unlock() // Release the lock when the function exits
	allAlphabet := *rs.sortedAlphabet
	newChars := false
	for _, char := range characters {
		if _, exists := rs.alphabetMap[char]; exists {
			continue
		}
		allAlphabet = append(allAlphabet, char)
		newChars = true
		rs.alphabetMap[char] = 0
	}
	if newChars {
		rs.sortedAlphabet = sortAlphabet(allAlphabet)
		rs.alphabetMap = constructAlphabetMap(rs.sortedAlphabet)
	}
}

// isRangeEqualWithPadding checks if two range strings are identical. Equality
// encompasses any padding using the smallest alphabet character from the set.
func (rs *rangeSplitter) isRangeEqualWithPadding(startRange, endRange *[]rune) bool {

	sortedAlphabet := rs.sortedAlphabet

	// When the end range is unspecified, it's interpreted as a sequence of the
	// highest possible characters. Consequently, they are not deemed equal.
	if len(*endRange) == 0 {
		return false
	}

	// Get the longer length of the two range strings.
	maxLength := len(*startRange)
	if len(*endRange) > maxLength {
		maxLength = len(*endRange)
	}

	smallestChar := (*sortedAlphabet)[0]

	// Loop through the string range.
	for i := 0; i < maxLength; i++ {

		// In cases where a character is absent at a specific position (due to a length
		// difference), the position is padded with the smallest character in the alphabet.
		charStart := charAtOrDefault(startRange, i, smallestChar)
		charEnd := charAtOrDefault(endRange, i, smallestChar)

		// As soon as we find a difference, we conclude the two strings are different.
		if charStart != charEnd {
			return false
		}
	}
	// Otherwise, we conclude the two strings are equal.
	return true
}

// charAtOrDefault returns the character at the specified position, or the default character if
// the position is out of bounds.
func charAtOrDefault(charArray *[]rune, position int, defaultChar rune) rune {
	if position < 0 || position >= len(*charArray) {
		return defaultChar
	}
	return (*charArray)[position]
}

// convertStringRangeToMinimalIntRange gradually extends the start and end string
// range in base-10 representation, until the difference reaches a threshold
// suitable for splitting.
func (rs *rangeSplitter) convertStringRangeToMinimalIntRange(
	startRange, endRange *[]rune, numSplits int) (*minimalIntRange, error) {

	startInteger := big.NewInt(0)
	endInteger := big.NewInt(0)

	alphabetLength := len(*rs.sortedAlphabet)
	startChar := (*rs.sortedAlphabet)[0]
	endChar := (*rs.sortedAlphabet)[alphabetLength-1]

	endDefaultChar := startChar
	if len(*endRange) == 0 {
		endDefaultChar = endChar
	}

	for i := 0; ; i++ {

		// Convert each character of the start range string into a big integer
		// based on the alphabet system.
		startPosition, err := rs.charPosition(charAtOrDefault(startRange, i, startChar))
		if err != nil {
			return nil, err
		}
		startInteger.Mul(startInteger, big.NewInt(int64(alphabetLength)))
		startInteger.Add(startInteger, big.NewInt(int64(startPosition)))

		// Convert each character of the end range string into a big integer
		// based on the alphabet system.
		endPosition, err := rs.charPosition(charAtOrDefault(endRange, i, endDefaultChar))
		if err != nil {
			return nil, err
		}
		endInteger.Mul(endInteger, big.NewInt(int64(alphabetLength)))
		endInteger.Add(endInteger, big.NewInt(int64(endPosition)))

		// Calculate the difference between the start and end range in big integer representation.
		difference := new(big.Int).Sub(endInteger, startInteger)

		// If the difference is bigger than the number of split points, we are done.
		// In particular, the minimal length is one greater than the index (due to zero indexing).
		if difference.Cmp(big.NewInt(int64(numSplits))) > 0 {
			return &minimalIntRange{
				startInteger:  startInteger,
				endInteger:    endInteger,
				minimalLength: i + 1,
			}, nil
		}
	}
}

// charPosition returns the index of the character in the alphabet set.
func (rs *rangeSplitter) charPosition(ch rune) (int, error) {
	if idx, ok := rs.alphabetMap[ch]; ok {
		return idx, nil
	}
	return -1, fmt.Errorf("character %c is not found in the alphabet map %v", ch, rs.alphabetMap)
}

// convertRangeStringToArray transforms the range string into a rune slice while
// verifying the presence of each character in the alphabets.
func (rs *rangeSplitter) convertRangeStringToArray(rangeString string) (*[]rune, error) {
	for _, char := range rangeString {
		if _, exists := rs.alphabetMap[char]; !exists {
			return nil, fmt.Errorf("character %c in range string %q is not found in the alphabet array", char, rangeString)
		}
	}
	characterArray := []rune(rangeString)
	return &characterArray, nil
}

// convertIntToString converts the split point from base-10 to base-N.
func (rs *rangeSplitter) convertIntToString(splitPoint *big.Int, stringLength int) string {

	remainder := new(big.Int)

	var splitChar []rune
	alphabetSize := big.NewInt(int64(len(*rs.sortedAlphabet)))

	// Iterate through the split point and convert alphabet by alphabet.
	for i := 0; i < stringLength; i++ {
		remainder.Mod(splitPoint, alphabetSize)
		splitPoint.Div(splitPoint, alphabetSize)
		splitChar = append(splitChar, (*rs.sortedAlphabet)[(int)(remainder.Int64())])
	}

	// Reverse the converted alphabet order because we originally processed from right to left.
	for i, j := 0, len(splitChar)-1; i < j; i, j = i+1, j-1 {
		splitChar[i], splitChar[j] = splitChar[j], splitChar[i]
	}

	return string(splitChar)
}
