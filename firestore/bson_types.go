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

package firestore

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// BSONObjectID represents a BSON ObjectID (12 bytes).
type BSONObjectID [12]byte

// String returns the 24-character lowercase hex string representation of the BSONObjectID.
func (id BSONObjectID) String() string {
	return hex.EncodeToString(id[:])
}

// ParseBSONObjectID parses a 24-character lowercase hex string into a BSONObjectID.
func ParseBSONObjectID(s string) (BSONObjectID, error) {
	var id BSONObjectID
	if len(s) != 24 {
		return id, fmt.Errorf("firestore: invalid BSONObjectID length: %d (expected 24)", len(s))
	}
	// Check if all characters are lowercase hex.
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return id, fmt.Errorf("firestore: invalid BSONObjectID character at %d: %c", i, c)
		}
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return id, err
	}
	copy(id[:], b)
	return id, nil
}

// BSONRegex represents a BSON Regular Expression.
type BSONRegex struct {
	Pattern string
	Options string
}

// Validate validates the BSONRegex.
// - Pattern must not contain null bytes.
// - Options must only contain characters 'i', 'm', 's', 'u', 'x', sorted alphabetically, no repeats.
func (r BSONRegex) Validate() error {
	if strings.ContainsRune(r.Pattern, 0) {
		return fmt.Errorf("firestore: BSONRegex Pattern cannot contain null bytes")
	}
	// Validate options.
	validOptions := map[rune]bool{'i': true, 'm': true, 's': true, 'u': true, 'x': true}
	seen := map[rune]bool{}
	for _, c := range r.Options {
		if !validOptions[c] {
			return fmt.Errorf("firestore: invalid BSONRegex Option: %c", c)
		}
		if seen[c] {
			return fmt.Errorf("firestore: duplicate BSONRegex Option: %c", c)
		}
		seen[c] = true
	}
	// Check if sorted.
	runes := []rune(r.Options)
	if !sort.SliceIsSorted(runes, func(i, j int) bool { return runes[i] < runes[j] }) {
		return fmt.Errorf("firestore: BSONRegex Options must be sorted alphabetically: %s", r.Options)
	}
	return nil
}

// BSONTimestamp represents a BSON Timestamp.
type BSONTimestamp struct {
	Seconds   uint32
	Increment uint32
}

// BSONDecimal128 represents a BSON Decimal128.
type BSONDecimal128 string

// Validate validates the BSONDecimal128 string representation.
func (d BSONDecimal128) Validate() error {
	if d == "" {
		return fmt.Errorf("firestore: BSONDecimal128 string cannot be empty")
	}
	return nil
}

// BSONMinKey represents BSON MinKey.
type BSONMinKey struct{}

// BSONMaxKey represents BSON MaxKey.
type BSONMaxKey struct{}

// BSONBinary represents BSON Binary data with subtype != 0.
type BSONBinary struct {
	Subtype byte
	Data    []byte
}

// BSONInt32 represents a BSON 32-bit integer.
type BSONInt32 int32
