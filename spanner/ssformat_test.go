// Copyright 2026 Google LLC
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

package spanner

import (
	"bytes"
	"fmt"
	"math"
	"slices"
	"sort"
	"testing"
)

// buildSignedIntTestValues generates a comprehensive set of signed integer test values.
func buildSignedIntTestValues() []int64 {
	values := make(map[int64]bool)

	// Range of small values
	for i := range 600 {
		values[int64(i-300)] = true
	}

	// Powers of 2 and boundaries
	for i := range 63 {
		powerOf2 := int64(1) << i
		values[powerOf2] = true
		values[powerOf2-1] = true
		values[powerOf2+1] = true
		values[-powerOf2] = true
		values[-powerOf2-1] = true
		values[-powerOf2+1] = true
	}

	// Edge cases
	values[math.MinInt64] = true
	values[math.MaxInt64] = true

	result := make([]int64, 0, len(values))
	for v := range values {
		result = append(result, v)
	}
	slices.Sort(result)
	return result
}

// buildUnsignedIntTestValues generates a comprehensive set of unsigned integer test values.
func buildUnsignedIntTestValues() []uint64 {
	values := make(map[uint64]bool)

	// Range of small values
	for i := range 600 {
		values[uint64(i)] = true
	}

	// Powers of 2 and boundaries (only non-negative values for unsigned)
	for i := range 64 {
		powerOf2 := uint64(1) << i
		values[powerOf2] = true
		if powerOf2 > 0 {
			values[powerOf2-1] = true
		}
		if powerOf2 < math.MaxUint64 {
			values[powerOf2+1] = true
		}
	}

	result := make([]uint64, 0, len(values))
	for v := range values {
		result = append(result, v)
	}
	slices.Sort(result)
	return result
}

// ==================== Prefix Successor Tests ====================

func TestSsFormatMakePrefixSuccessor_EmptyInput(t *testing.T) {
	result := makePrefixSuccessor(nil)
	if result != nil {
		t.Errorf("Expected nil for nil input, got %v", result)
	}

	result = makePrefixSuccessor([]byte{})
	if result != nil {
		t.Errorf("Expected nil for empty input, got %v", result)
	}
}

func TestSsFormatMakePrefixSuccessor_ResultIsGreaterThanOriginal(t *testing.T) {
	original := []byte{0x10, 0x20, 0x30}
	successor := makePrefixSuccessor(original)

	if bytes.Compare(original, successor) >= 0 {
		t.Errorf("Successor should be greater than original")
	}
}

// ==================== Composite Tag Tests ====================

func TestSsFormatAppendCompositeTag_ShortTag(t *testing.T) {
	// Tags 1-15 should fit in 1 byte
	for tag := 1; tag <= 15; tag++ {
		result, err := appendCompositeTag(nil, tag)
		if err != nil {
			t.Errorf("Unexpected error for tag %d: %v", tag, err)
			continue
		}
		if len(result) != 1 {
			t.Errorf("Tag %d should encode to 1 byte, got %d bytes", tag, len(result))
			continue
		}
		expected := byte(tag << 1)
		if result[0] != expected {
			t.Errorf("Tag %d should encode as 0x%02X, got 0x%02X", tag, expected, result[0])
		}
	}
}

func TestSsFormatAppendCompositeTag_MediumTag(t *testing.T) {
	// Tags 16-4095 should fit in 2 bytes
	testTags := []int{16, 100, 1000, 4095}
	for _, tag := range testTags {
		result, err := appendCompositeTag(nil, tag)
		if err != nil {
			t.Errorf("Unexpected error for tag %d: %v", tag, err)
			continue
		}
		if len(result) != 2 {
			t.Errorf("Tag %d should encode to 2 bytes, got %d bytes", tag, len(result))
		}
	}
}

func TestSsFormatAppendCompositeTag_LargeTag(t *testing.T) {
	// Tags 4096-65535 should fit in 3 bytes
	testTags := []int{4096, 10000, 65535}
	for _, tag := range testTags {
		result, err := appendCompositeTag(nil, tag)
		if err != nil {
			t.Errorf("Unexpected error for tag %d: %v", tag, err)
			continue
		}
		if len(result) != 3 {
			t.Errorf("Tag %d should encode to 3 bytes, got %d bytes", tag, len(result))
		}
	}
}

func TestSsFormatAppendCompositeTag_InvalidTag(t *testing.T) {
	_, err := appendCompositeTag(nil, 0)
	if err == nil {
		t.Error("Expected error for tag 0")
	}

	_, err = appendCompositeTag(nil, -1)
	if err == nil {
		t.Error("Expected error for tag -1")
	}

	_, err = appendCompositeTag(nil, 65536)
	if err == nil {
		t.Error("Expected error for tag 65536")
	}
}

func TestSsFormatAppendCompositeTag_PreservesOrdering(t *testing.T) {
	// Verify smaller tags encode to lexicographically smaller byte sequences
	for tag1 := 1; tag1 <= 100; tag1++ {
		for tag2 := tag1 + 1; tag2 <= 101 && tag2 <= tag1+10; tag2++ {
			result1, _ := appendCompositeTag(nil, tag1)
			result2, _ := appendCompositeTag(nil, tag2)

			if bytes.Compare(result1, result2) >= 0 {
				t.Errorf("Tag %d should encode smaller than tag %d", tag1, tag2)
			}
		}
	}
}

// ==================== Signed Integer Tests ====================

func TestSsFormatAppendIntIncreasing_PreservesOrdering(t *testing.T) {
	testValues := buildSignedIntTestValues()

	for i := 0; i < len(testValues)-1; i++ {
		v1 := testValues[i]
		v2 := testValues[i+1]

		result1 := appendIntIncreasing(nil, v1)
		result2 := appendIntIncreasing(nil, v2)

		if bytes.Compare(result1, result2) >= 0 {
			t.Errorf("Encoded %d should be less than encoded %d", v1, v2)
		}
	}
}

func TestSsFormatAppendIntDecreasing_ReversesOrdering(t *testing.T) {
	testValues := buildSignedIntTestValues()

	for i := 0; i < len(testValues)-1; i++ {
		v1 := testValues[i]
		v2 := testValues[i+1]

		result1 := appendIntDecreasing(nil, v1)
		result2 := appendIntDecreasing(nil, v2)

		if bytes.Compare(result1, result2) <= 0 {
			t.Errorf("Decreasing encoded %d should be greater than encoded %d", v1, v2)
		}
	}
}

func TestSsFormatAppendIntIncreasing_EdgeCases(t *testing.T) {
	edgeCases := []int64{math.MinInt64, -1, 0, 1, math.MaxInt64}

	for _, value := range edgeCases {
		result := appendIntIncreasing(nil, value)

		if len(result) < 2 {
			t.Errorf("Result should have at least 2 bytes for value %d, got %d", value, len(result))
		}
		if result[0]&0x80 == 0 {
			t.Errorf("IS_KEY bit should be set for value %d", value)
		}
	}
}

// ==================== Unsigned Integer Tests ====================

func TestSsFormatAppendUint64Increasing_PreservesOrdering(t *testing.T) {
	testValues := buildUnsignedIntTestValues()

	for i := 0; i < len(testValues)-1; i++ {
		v1 := testValues[i]
		v2 := testValues[i+1]

		result1 := appendUint64Increasing(nil, v1)
		result2 := appendUint64Increasing(nil, v2)

		if bytes.Compare(result1, result2) >= 0 {
			t.Errorf("Unsigned encoded %d should be less than %d", v1, v2)
		}
	}
}

func TestSsFormatAppendUint64Decreasing_ReversesOrdering(t *testing.T) {
	values := []uint64{0, 1, 100, 1000, math.MaxUint64}

	for i := 0; i < len(values)-1; i++ {
		result1 := appendUint64Decreasing(nil, values[i])
		result2 := appendUint64Decreasing(nil, values[i+1])

		if bytes.Compare(result1, result2) <= 0 {
			t.Errorf("Decreasing unsigned encoded %d should be greater than %d", values[i], values[i+1])
		}
	}
}

// ==================== String Tests ====================

func TestSsFormatAppendStringIncreasing_PreservesOrdering(t *testing.T) {
	strings := []string{"", "a", "aa", "ab", "b", "hello", "world", "\xff"}
	sort.Strings(strings)

	for i := 0; i < len(strings)-1; i++ {
		result1 := appendStringIncreasing(nil, strings[i])
		result2 := appendStringIncreasing(nil, strings[i+1])

		if bytes.Compare(result1, result2) >= 0 {
			t.Errorf("Encoded '%s' should be less than '%s'", strings[i], strings[i+1])
		}
	}
}

func TestSsFormatAppendStringDecreasing_ReversesOrdering(t *testing.T) {
	strings := []string{"", "a", "b", "hello"}

	for i := 0; i < len(strings)-1; i++ {
		result1 := appendStringDecreasing(nil, strings[i])
		result2 := appendStringDecreasing(nil, strings[i+1])

		if bytes.Compare(result1, result2) <= 0 {
			t.Errorf("Decreasing encoded '%s' should be greater than '%s'", strings[i], strings[i+1])
		}
	}
}

// ==================== Bytes Tests ====================

func TestSsFormatAppendBytesIncreasing_PreservesOrdering(t *testing.T) {
	testBytes := [][]byte{
		{},
		{0x00},
		{0x01},
		{0x01, 0x02},
		{0xFF},
	}

	for i := 0; i < len(testBytes)-1; i++ {
		result1 := appendBytesIncreasing(nil, testBytes[i])
		result2 := appendBytesIncreasing(nil, testBytes[i+1])

		if bytes.Compare(result1, result2) >= 0 {
			t.Errorf("Encoded bytes should maintain lexicographic order")
		}
	}
}

func TestSsFormatAppendBytesDecreasing_ReversesOrdering(t *testing.T) {
	testBytes := [][]byte{
		{},
		{0x00},
		{0x01},
		{0x01, 0x02},
		{0xFF},
	}

	for i := 0; i < len(testBytes)-1; i++ {
		result1 := appendBytesDecreasing(nil, testBytes[i])
		result2 := appendBytesDecreasing(nil, testBytes[i+1])

		if bytes.Compare(result1, result2) <= 0 {
			t.Errorf("Decreasing encoded bytes should reverse lexicographic order")
		}
	}
}

func TestSsFormatAppendBytesDecreasing_EscapesSpecialBytes(t *testing.T) {
	result := appendBytesDecreasing(nil, []byte{0x00, 0xFF, 0x42})

	// Result should be longer due to escaping
	if len(result) <= 5 {
		t.Errorf("Result should include escape sequences, got length %d", len(result))
	}
}

func TestSsFormatAppendBytesDecreasing_EmptyArray(t *testing.T) {
	result := appendBytesDecreasing(nil, []byte{})

	// Empty bytes should still have header + terminator
	if len(result) < 3 {
		t.Errorf("Empty bytes encoding should have at least 3 bytes, got %d", len(result))
	}
	if result[0]&0x80 == 0 {
		t.Errorf("IS_KEY bit should be set")
	}
}

func TestSsFormatAppendBytesIncreasing_vs_Decreasing_DifferentOutput(t *testing.T) {
	input := []byte{0x01, 0x02, 0x03}

	resultInc := appendBytesIncreasing(nil, input)
	resultDec := appendBytesDecreasing(nil, input)

	if bytes.Equal(resultInc, resultDec) {
		t.Errorf("Increasing and decreasing encodings should differ")
	}
}

// ==================== Double Tests ====================

func TestSsFormatAppendDoubleDecreasing_ReversesOrdering(t *testing.T) {
	values := []float64{-math.MaxFloat64, -1.0, 0.0, 1.0, math.MaxFloat64}

	for i := 0; i < len(values)-1; i++ {
		result1 := appendDoubleDecreasing(nil, values[i])
		result2 := appendDoubleDecreasing(nil, values[i+1])

		if bytes.Compare(result1, result2) <= 0 {
			t.Errorf("Decreasing encoded %v should be greater than %v", values[i], values[i+1])
		}
	}
}

func TestSsFormatAppendDoubleIncreasing_NegativeZeroEqualsPositiveZero(t *testing.T) {
	resultNegZero := appendDoubleIncreasing(nil, math.Copysign(0, -1))
	resultPosZero := appendDoubleIncreasing(nil, 0.0)

	if !bytes.Equal(resultNegZero, resultPosZero) {
		t.Errorf("-0.0 and 0.0 should encode identically")
	}
}

func TestSsFormatAppendDoubleIncreasing_NaN(t *testing.T) {
	result := appendDoubleIncreasing(nil, math.NaN())

	if len(result) < 2 {
		t.Errorf("NaN encoding should have at least 2 bytes, got %d", len(result))
	}
	if result[0]&0x80 == 0 {
		t.Errorf("IS_KEY bit should be set for NaN")
	}
}

// ==================== Null Marker Tests ====================

func TestSsFormatNullOrderedFirst_SortsBeforeValues(t *testing.T) {
	nullResult := appendNullOrderedFirst(nil)

	valueResult := appendNotNullMarkerNullOrderedFirst(nil)
	valueResult = appendIntIncreasing(valueResult, math.MinInt64)

	if bytes.Compare(nullResult, valueResult) >= 0 {
		t.Errorf("Null (ordered first) should sort before any value")
	}
}

func TestSsFormatNullOrderedLast_SortsAfterValues(t *testing.T) {
	nullResult := appendNullOrderedLast(nil)

	valueResult := appendNotNullMarkerNullOrderedLast(nil)
	valueResult = appendIntIncreasing(valueResult, math.MaxInt64)

	if bytes.Compare(nullResult, valueResult) <= 0 {
		t.Errorf("Null (ordered last) should sort after any value")
	}
}

// ==================== Timestamp Tests ====================

func TestSsFormatEncodeTimestamp_Length(t *testing.T) {
	result := encodeTimestamp(0, 0)
	if len(result) != 12 {
		t.Errorf("Timestamp should encode to 12 bytes, got %d", len(result))
	}
}

func TestSsFormatEncodeTimestamp_PreservesOrdering(t *testing.T) {
	timestamps := [][2]int64{
		{0, 0},
		{0, 1},
		{0, 999999999},
		{1, 0},
		{100, 500000000},
		{math.MaxInt64 / 2, 0},
	}

	for i := 0; i < len(timestamps)-1; i++ {
		t1 := encodeTimestamp(timestamps[i][0], int32(timestamps[i][1]))
		t2 := encodeTimestamp(timestamps[i+1][0], int32(timestamps[i+1][1]))

		if bytes.Compare(t1, t2) >= 0 {
			t.Errorf("Earlier timestamp should encode smaller")
		}
	}
}

// ==================== UUID Tests ====================

func TestSsFormatEncodeUUID_Length(t *testing.T) {
	result := encodeUUID(0, 0)
	if len(result) != 16 {
		t.Errorf("UUID should encode to 16 bytes, got %d", len(result))
	}
}

func TestSsFormatEncodeUUID_BigEndianEncoding(t *testing.T) {
	result := encodeUUID(0x0102030405060708, 0x090A0B0C0D0E0F10)

	// Verify big-endian encoding of high bits
	expectedHigh := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	for i, expected := range expectedHigh {
		if result[i] != expected {
			t.Errorf("Byte %d should be 0x%02X, got 0x%02X", i, expected, result[i])
		}
	}

	// Verify big-endian encoding of low bits
	expectedLow := []byte{0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10}
	for i, expected := range expectedLow {
		if result[8+i] != expected {
			t.Errorf("Byte %d should be 0x%02X, got 0x%02X", 8+i, expected, result[8+i])
		}
	}
}

func TestSsFormatEncodeUUID_PreservesOrdering(t *testing.T) {
	uuids := [][2]int64{
		{0, 0},
		{0, 1},
		{0, math.MaxInt64},
		{1, 0},
		{math.MaxInt64, math.MaxInt64},
	}

	for i := 0; i < len(uuids)-1; i++ {
		u1 := encodeUUID(uuids[i][0], uuids[i][1])
		u2 := encodeUUID(uuids[i+1][0], uuids[i+1][1])

		if bytes.Compare(u1, u2) >= 0 {
			t.Errorf("UUID ordering should be preserved")
		}
	}
}

// ==================== Composite Key Tests ====================

func TestSsFormatCompositeKey_TagPlusIntPreservesOrdering(t *testing.T) {
	tag := 5
	values := []int64{math.MinInt64, -1, 0, 1, math.MaxInt64}

	for i := 0; i < len(values)-1; i++ {
		result1, _ := appendCompositeTag(nil, tag)
		result1 = appendIntIncreasing(result1, values[i])

		result2, _ := appendCompositeTag(nil, tag)
		result2 = appendIntIncreasing(result2, values[i+1])

		if bytes.Compare(result1, result2) >= 0 {
			t.Errorf("Composite key with %d should be less than with %d", values[i], values[i+1])
		}
	}
}

func TestSsFormatCompositeKey_DifferentTagsSortByTag(t *testing.T) {
	value := int64(100)

	result1, _ := appendCompositeTag(nil, 5)
	result1 = appendIntIncreasing(result1, value)

	result2, _ := appendCompositeTag(nil, 10)
	result2 = appendIntIncreasing(result2, value)

	if bytes.Compare(result1, result2) >= 0 {
		t.Errorf("Key with smaller tag should sort first")
	}
}

func TestSsFormatCompositeKey_MultipleKeyParts(t *testing.T) {
	// Simulate encoding a composite key with multiple parts: tag + int + string
	result1, _ := appendCompositeTag(nil, 1)
	result1 = appendIntIncreasing(result1, 100)
	result1 = appendStringIncreasing(result1, "alice")

	result2, _ := appendCompositeTag(nil, 1)
	result2 = appendIntIncreasing(result2, 100)
	result2 = appendStringIncreasing(result2, "bob")

	if bytes.Compare(result1, result2) >= 0 {
		t.Errorf("Keys with same prefix but different strings should order by string")
	}
}

// ==================== Order Preservation Summary Test ====================

func TestSsFormatOrderPreservation_ComprehensiveIntTest(t *testing.T) {
	testValues := buildSignedIntTestValues()

	// Take a sample of values to avoid O(n^2) test time
	step := max(1, len(testValues)/100)
	var sample []int64
	for i := 0; i < len(testValues); i += step {
		sample = append(sample, testValues[i])
	}

	// Encode all values
	var encoded [][]byte
	for _, v := range sample {
		encoded = append(encoded, appendIntIncreasing(nil, v))
	}

	// Verify the encoded values are in the same order as the original values
	for i := 0; i < len(sample)-1; i++ {
		if bytes.Compare(encoded[i], encoded[i+1]) >= 0 {
			t.Errorf("Order should be preserved: %d < %d", sample[i], sample[i+1])
		}
	}
}

// ==================== TargetRange Tests ====================

func TestSsFormatTargetRange_IsPoint(t *testing.T) {
	pointRange := newTargetRange([]byte{0x01, 0x02}, nil, false)
	if !pointRange.isPoint() {
		t.Errorf("Expected isPoint() to return true for empty limit")
	}

	rangeRange := newTargetRange([]byte{0x01}, []byte{0x02}, false)
	if rangeRange.isPoint() {
		t.Errorf("Expected isPoint() to return false for non-empty limit")
	}
}

func TestSsFormatTargetRange_MergeFrom(t *testing.T) {
	r1 := newTargetRange([]byte{0x10}, []byte{0x30}, false)
	r2 := newTargetRange([]byte{0x05}, []byte{0x20}, false)

	r1.mergeFrom(r2)

	// Should take minimum start
	if !bytes.Equal(r1.start, []byte{0x05}) {
		t.Errorf("Expected start to be 0x05, got %v", r1.start)
	}

	// Should keep maximum limit
	if !bytes.Equal(r1.limit, []byte{0x30}) {
		t.Errorf("Expected limit to be 0x30, got %v", r1.limit)
	}
}

func TestSsFormatTargetRange_MergeFrom_Approximate(t *testing.T) {
	r1 := newTargetRange([]byte{0x10}, []byte{0x30}, false)
	r2 := newTargetRange([]byte{0x15}, []byte{0x25}, true)

	r1.mergeFrom(r2)

	if !r1.approximate {
		t.Errorf("Expected approximate to be true after merging with approximate range")
	}
}

// ==================== Golden Tests ====================
func TestSsFormatGolden_MakePrefixSuccessor(t *testing.T) {
	// Any valid key has the LSB of zero. Thus, setting the LSB of key K to one
	// will create a string that's larger than K, but smaller than any valid key
	// that does not start with K.
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{"zero byte", []byte{0x00}, []byte{0x01}},
		{"even byte", []byte{0x10}, []byte{0x11}},
		{"already odd (valid ssformat keys should not have odd LSB)", []byte{0x11}, []byte{0x11}}, // No change since LSB already 1
		{"multi-byte key", []byte{0x81, 0x02, 0x58}, []byte{0x81, 0x02, 0x59}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := makePrefixSuccessor(tt.input)
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("makePrefixSuccessor(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSsFormatGolden_CompositeTag(t *testing.T) {
	// Verify composite tag encoding produces expected bytes.
	tests := []struct {
		tag      int
		expected []byte
	}{
		// Short tags (1-15): encode as (tag << 1)
		{1, []byte{0x02}},  // 1 << 1 = 2
		{5, []byte{0x0A}},  // 5 << 1 = 10
		{15, []byte{0x1E}}, // 15 << 1 = 30
		// Medium tags (16-4095): 2 bytes, header is 001xxxxx (0x20 | tag_bits)
		{16, []byte{0x20, 0x20}},   // shiftedTag=32, header=0x20|(32>>8)=0x20, second=32&0xFF=0x20
		{100, []byte{0x20, 0xC8}},  // shiftedTag=200, header=0x20, second=0xC8
		{4095, []byte{0x3F, 0xFE}}, // shiftedTag=8190, header=0x20|(8190>>8)=0x3F, second=0xFE
		// Large tags (4096-65535): 3 bytes, header is 010xxxxx (0x40 | tag_bits)
		{4096, []byte{0x40, 0x20, 0x00}},  // shiftedTag=8192=0x2000
		{10000, []byte{0x40, 0x4E, 0x20}}, // shiftedTag=20000=0x4E20
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("tag_%d", tt.tag), func(t *testing.T) {
			result, err := appendCompositeTag(nil, tt.tag)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("appendCompositeTag(%d) = %v, want %v", tt.tag, result, tt.expected)
			}
		})
	}
}

func TestSsFormatGolden_UnsignedInt(t *testing.T) {
	// Verify unsigned integer encoding produces expected bytes.
	// Example: 0x1234 = 4660
	// LSB byte: (4660 & 0x7F) << 1 = (52) << 1 = 104 = 0x68
	// Remaining: 4660 >> 7 = 36 = 0x24
	// Type: TYPE_UINT_1 + 2 - 1 = 1, header = 0x80 | 1 = 0x81
	tests := []struct {
		name     string
		val      uint64
		expected []byte
	}{
		{"zero", 0, []byte{0x80, 0x00}},              // TYPE_UINT_1, payload = 0
		{"one", 1, []byte{0x80, 0x02}},               // TYPE_UINT_1, payload = 1<<1 = 2
		{"127", 127, []byte{0x80, 0xFE}},             // TYPE_UINT_1, payload = 127<<1 = 254
		{"128", 128, []byte{0x81, 0x01, 0x00}},       // TYPE_UINT_2, 128 = (1<<7), so 1 in high, 0 in low<<1
		{"0x1234", 0x1234, []byte{0x81, 0x24, 0x68}}, // 0x1234ULL => 81 24 68
		{"300", 300, []byte{0x81, 0x02, 0x58}},       // 300 = 0x12C, shifted payload
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := appendUint64Increasing(nil, tt.val)
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("appendUint64Increasing(%d) = %x, want %x", tt.val, result, tt.expected)
			}
		})
	}
}

func TestSsFormatGolden_SignedInt(t *testing.T) {
	// Verify signed integer encoding produces expected bytes.
	tests := []struct {
		name     string
		val      int64
		expected []byte
	}{
		{"zero", 0, []byte{0x91, 0x00}},        // TYPE_POS_INT_1 = 17, header = 0x80|17 = 0x91
		{"positive_1", 1, []byte{0x91, 0x02}},  // 1 << 1 = 2
		{"negative_1", -1, []byte{0x90, 0xFE}}, // TYPE_NEG_INT_1 = 16, header = 0x90
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := appendIntIncreasing(nil, tt.val)
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("appendIntIncreasing(%d) = %x, want %x", tt.val, result, tt.expected)
			}
		})
	}
}

func TestSsFormatGolden_Double(t *testing.T) {
	// Verify the double transformation for sortable encoding.
	// The transformation maps:
	// - Positive doubles (bit 63 = 0) stay as-is
	// - Negative doubles (bit 63 = 1) get transformed to maintain ordering

	// Test that ordering is preserved across the transformation
	orderedDoubles := []float64{
		math.Inf(-1),
		-math.MaxFloat64,
		-1e100,
		-1.0,
		-1e-100,
		-math.SmallestNonzeroFloat64,
		0.0,
		math.SmallestNonzeroFloat64,
		1e-100,
		1.0,
		1e100,
		math.MaxFloat64,
		math.Inf(1),
	}

	var encodings [][]byte
	for _, d := range orderedDoubles {
		encodings = append(encodings, appendDoubleIncreasing(nil, d))
	}

	for i := 0; i < len(encodings)-1; i++ {
		if bytes.Compare(encodings[i], encodings[i+1]) >= 0 {
			t.Errorf("Ordering violated: encoded(%v) >= encoded(%v)", orderedDoubles[i], orderedDoubles[i+1])
		}
	}
}

func TestSsFormatGolden_String(t *testing.T) {
	// Verify string encoding produces expected bytes.
	// Header: IS_KEY | TYPE_STRING = 0x80 | 25 = 0x99
	// Terminator: 0x00 0x78
	// Escape: 0x00 -> 0x00 0xF0, 0xFF -> 0xFF 0x10
	tests := []struct {
		name     string
		val      string
		expected []byte
	}{
		{"empty", "", []byte{0x99, 0x00, 0x78}},   // header + terminator
		{"a", "a", []byte{0x99, 'a', 0x00, 0x78}}, // header + 'a' + terminator
		{"hello", "hello", []byte{0x99, 'h', 'e', 'l', 'l', 'o', 0x00, 0x78}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := appendStringIncreasing(nil, tt.val)
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("appendStringIncreasing(%q) = %x, want %x", tt.val, result, tt.expected)
			}
		})
	}
}

func TestSsFormatGolden_BytesEscaping(t *testing.T) {
	// Verify escape sequences produce expected bytes.
	// 0x00 => 0x00 0xF0
	// 0xFF => 0xFF 0x10
	tests := []struct {
		name     string
		val      []byte
		expected []byte
	}{
		{"null_byte", []byte{0x00}, []byte{0x99, 0x00, 0xF0, 0x00, 0x78}},
		{"ff_byte", []byte{0xFF}, []byte{0x99, 0xFF, 0x10, 0x00, 0x78}},
		{"mixed", []byte{0x00, 0x42, 0xFF}, []byte{0x99, 0x00, 0xF0, 0x42, 0xFF, 0x10, 0x00, 0x78}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := appendBytesIncreasing(nil, tt.val)
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("appendBytesIncreasing(%v) = %x, want %x", tt.val, result, tt.expected)
			}
		})
	}
}

func TestSsFormatGolden_NullMarkers(t *testing.T) {
	// Verify NULL marker encoding produces expected bytes.
	// TYPE_NULL_ORDERED_FIRST = 27, header = 0x80 | 27 = 0x9B
	// TYPE_NULL_ORDERED_LAST = 60, header = 0x80 | 60 = 0xBC
	// TYPE_NULLABLE_NOT_NULL_NULL_ORDERED_FIRST = 28, header = 0x80 | 28 = 0x9C
	// TYPE_NULLABLE_NOT_NULL_NULL_ORDERED_LAST = 59, header = 0x80 | 59 = 0xBB

	if result := appendNullOrderedFirst(nil); !bytes.Equal(result, []byte{0x9B, 0x00}) {
		t.Errorf("appendNullOrderedFirst() = %x, want %x", result, []byte{0x9B, 0x00})
	}

	if result := appendNullOrderedLast(nil); !bytes.Equal(result, []byte{0xBC, 0x00}) {
		t.Errorf("appendNullOrderedLast() = %x, want %x", result, []byte{0xBC, 0x00})
	}

	if result := appendNotNullMarkerNullOrderedFirst(nil); !bytes.Equal(result, []byte{0x9C}) {
		t.Errorf("appendNotNullMarkerNullOrderedFirst() = %x, want %x", result, []byte{0x9C})
	}

	if result := appendNotNullMarkerNullOrderedLast(nil); !bytes.Equal(result, []byte{0xBB}) {
		t.Errorf("appendNotNullMarkerNullOrderedLast() = %x, want %x", result, []byte{0xBB})
	}
}
