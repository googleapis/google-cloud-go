// Copyright 2022 Google LLC
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

package types

import (
	"bytes"
	"fmt"
	"strconv"
	"time"
)

// IntervalValue is a go type for representing BigQuery INTERVAL values.
// Intervals are encoded in three parts:
// * Years and Months
// * Days
// * Time (Hours/Mins/Seconds/Fractional Seconds).
//
// It is EXPERIMENTAL and subject to change or removal without notice.
type IntervalValue struct {
	// In canonical form, Years and Months share a consistent sign and reduced
	// to avoid large month values.
	Years  int32
	Months int32

	// In canonical form, Days are independent of the other parts and can have it's
	// own sign.  There is no attempt to reduce larger Day values into the Y-M part.
	Days int32

	// In canonical form, the time parts all share a consistent sign and are reduced.
	Hours      int32
	Minutes    int32
	Seconds    int32
	SubSeconds int32
}

// String returns string representation of the interval value using the canonical format.
// The canonical format is as follows:
// [sign]Y-M [sign]D [sign]H:M:S[.F]
func (iv *IntervalValue) String() string {
	src := iv
	if !iv.isCanonical() {
		src = iv.Canonicalize()
	}
	out := fmt.Sprintf("%d-%d %d %d:%d:%d", src.Years, int32abs(src.Months), src.Days, src.Hours, int32abs(src.Minutes), int32abs(src.Seconds))
	if src.SubSeconds != 0 {
		out = fmt.Sprintf("%s.%d", out, int32abs(src.SubSeconds))
	}
	return out
}

type intervalPart int

const (
	yearsPart = iota
	monthsPart
	daysPart
	hoursPart
	minutesPart
	secondsPart
	subsecsPart
)

func (i intervalPart) String() string {
	knownParts := []string{"YEARS", "MONTHS", "DAYS", "HOURS", "MINUTES", "SECONDS", "SUBSECONDS"}
	if i < 0 || int(i) > len(knownParts) {
		return fmt.Sprintf("UNKNOWN(%d)", i)
	}
	return knownParts[i]
}

var canonicalParts = []intervalPart{yearsPart, monthsPart, daysPart, hoursPart, minutesPart, secondsPart, subsecsPart}

// ParseInterval parses an interval in it's canonical string format into the IntervalType it represents.
func ParseInterval(value string) (*IntervalValue, error) {
	iVal := &IntervalValue{}
	for _, part := range canonicalParts {
		remaining, v, err := getPartValue(part, value)
		if err != nil {
			return nil, err
		}
		switch part {
		case yearsPart:
			iVal.Years = v
		case monthsPart:
			iVal.Months = v
			if iVal.Years < 0 {
				iVal.Months = -v
			}
		case daysPart:
			iVal.Days = v
		case hoursPart:
			iVal.Hours = v
		case minutesPart:
			iVal.Minutes = v
			if iVal.Hours < 0 {
				iVal.Minutes = -v
			}
		case secondsPart:
			iVal.Seconds = v
			if iVal.Hours < 0 {
				iVal.Seconds = -v
			}
		case subsecsPart:
			iVal.SubSeconds = v
			if iVal.Hours < 0 {
				iVal.SubSeconds = -v
			}
		default:
			return nil, fmt.Errorf("encountered invalid part %s during parse", part)
		}
		value = remaining
	}
	return iVal, nil
}

func getPartValue(part intervalPart, s string) (string, int32, error) {
	s = trimPrefix(part, s)
	return getNumVal(part, s)
}

// trimPrefix removes formatting prefix relevant to the given type.
func trimPrefix(part intervalPart, s string) string {
	var trimByte byte
	switch part {
	case yearsPart, daysPart, hoursPart:
		trimByte = byte(' ')
	case monthsPart:
		trimByte = byte('-')
	case minutesPart, secondsPart:
		trimByte = byte(':')
	case subsecsPart:
		trimByte = byte('.')
	}
	for len(s) > 0 && s[0] == trimByte {
		s = s[1:]
	}
	return s
}

func getNumVal(part intervalPart, s string) (string, int32, error) {

	allowedVals := []byte("0123456789")
	var allowedSign bool
	captured := ""
	switch part {
	case yearsPart, daysPart, hoursPart:
		allowedSign = true
	}
	// capture sign prefix +/-
	if len(s) > 0 && allowedSign {
		switch s[0] {
		case '-':
			captured = "-"
			s = s[1:]
		case '+':
			s = s[1:]
		}
	}
	for len(s) > 0 && bytes.IndexByte(allowedVals, s[0]) >= 0 {
		captured = captured + string(s[0])
		s = s[1:]
	}

	if len(captured) == 0 {
		if part == subsecsPart {
			// Subseconds is optional, treat no value as zero.
			return s, 0, nil
		}
		// Otherwise this is an error.
		return "", 0, fmt.Errorf("no value captured for part %s", part.String())
	}
	parsed, err := strconv.ParseInt(captured, 10, 32)
	if err != nil {
		return "", 0, fmt.Errorf("error parsing value %s for %s: %v", captured, part.String(), err)
	}
	return s, int32(parsed), nil
}

// FromDuration converts a time.Duration to an IntervalType representation.
// TODO: nanos
// TODO: do we need an error return?
func FromDuration(in time.Duration) (*IntervalValue, error) {
	return nil, fmt.Errorf("Unimplemented")
}

// ToDuration converts an interval to a time.Duration value.
func (iv *IntervalValue) ToDuration() time.Duration {
	return time.Duration(0)
}

// Canonicalize returns a normalized IntervalValue, where signs for elements in the
// Y-M and H:M:S.F are made consistent and values are normalized.
func (iv *IntervalValue) Canonicalize() *IntervalValue {
	newIV := &IntervalValue{iv.Years, iv.Months, iv.Days, iv.Hours, iv.Minutes, iv.Seconds, iv.SubSeconds}
	// canonicalize Y-M part
	totalMonths := iv.Years*12 + iv.Months
	newIV.Years = totalMonths / 12
	totalMonths = totalMonths - (newIV.Years * 12)
	newIV.Months = totalMonths % 12

	// TODO: do we canonicalize days?

	// canonicalize time part
	totalNanos := int64(iv.Hours)*3600*1e9 +
		int64(iv.Minutes)*60*1e9 +
		int64(iv.Seconds)*1e9
	switch {
	case iv.SubSeconds < 1000:
		// millis
		totalNanos = totalNanos + int64(iv.SubSeconds)*1e6
	case iv.SubSeconds >= 1000 && iv.SubSeconds < 1e6:
		// micros
		totalNanos = totalNanos + int64(iv.SubSeconds)*1000
	case iv.SubSeconds >= 1e6 && iv.SubSeconds < 1e9:
		// nanos
		totalNanos = totalNanos + int64(iv.SubSeconds)
	default:
		// TODO do we truncate, error, etc if our fraction is too long?
	}
	// Reduce to parts.
	newIV.Hours = int32(totalNanos / 3600 / 1e9)
	totalNanos = totalNanos - (int64(newIV.Hours) * 3600 * 1e9)
	newIV.Minutes = int32(totalNanos / 60 / 1e9)
	totalNanos = totalNanos - (int64(newIV.Minutes) * 60 * 1e9)
	newIV.Seconds = int32(totalNanos / 1e9)
	totalNanos = totalNanos - (int64(newIV.Seconds) * 1e9)
	// TODO: discard trailing zeros
	newIV.SubSeconds = int32(totalNanos)
	return newIV
}

func (iv *IntervalValue) isCanonical() bool {
	if !sameSign(iv.Years, iv.Months) ||
		!sameSign(iv.Hours, iv.Minutes) {
		return false
	}
	if int32abs(iv.Months) > 12 ||
		int32abs(iv.Hours) > 24 ||
		int32abs(iv.Minutes) > 60 ||
		int32abs(iv.Seconds) > 60 {
		return false
	}
	// TODO: validate boundarys like 10k years?
	return true
}

func int32abs(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
}

func sameSign(nums ...int32) bool {
	var pos, neg int
	for _, n := range nums {
		if n > 0 {
			pos = pos + 1
		}
		if n < 0 {
			neg = neg + 1
		}
	}
	if pos > 0 && neg > 0 {
		return false
	}
	return true
}
