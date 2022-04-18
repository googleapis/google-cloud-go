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
type IntervalValue struct {
	// Years and Months shares a single common sign.
	Years  int32
	Months int32

	// Days can be signed independently.
	Days int32

	// The time parts also share a single common sign.
	Hours      int32
	Mins       int32
	Seconds    int32
	SubSeconds int32
}

// String returns string representation of the interval value using the canonical format.
// The canonical format is as follows:
// [sign]Y-M [sign]D [sign]H:M:S[.F]
func (it *IntervalValue) String() string {
	// TODO: we need to normalize mixed sign values in the y-m group, and the h:m:s.f group.
	out := fmt.Sprintf("%d-%d %d %d:%d:%d", it.Years, it.Months, it.Days, it.Hours, it.Mins, it.Seconds)
	if it.SubSeconds != 0 {
		out = fmt.Sprintf("%s.%d", out, it.SubSeconds)
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
		case daysPart:
			iVal.Days = v
		case hoursPart:
			iVal.Hours = v
		case minutesPart:
			iVal.Mins = v
		case secondsPart:
			iVal.Seconds = v
		case subsecsPart:
			iVal.SubSeconds = v
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
func (it *IntervalValue) ToDuration() time.Duration {
	return time.Duration(0)
}
