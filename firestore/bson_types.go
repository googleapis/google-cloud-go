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

// ObjectID represents a BSON ObjectID (12 bytes).
type ObjectID [12]byte

// String returns the 24-character lowercase hex string representation of the ObjectID.
func (id ObjectID) String() string {
	return hex.EncodeToString(id[:])
}

// ParseObjectID parses a 24-character lowercase hex string into an ObjectID.
func ParseObjectID(s string) (ObjectID, error) {
	var id ObjectID
	if len(s) != 24 {
		return id, fmt.Errorf("firestore: invalid ObjectID length: %d (expected 24)", len(s))
	}
	// Check if all characters are lowercase hex.
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return id, fmt.Errorf("firestore: invalid ObjectID character at %d: %c", i, c)
		}
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return id, err
	}
	copy(id[:], b)
	return id, nil
}

// Regex represents a BSON Regular Expression.
type Regex struct {
	Pattern string
	Options string
}

// Validate validates the Regex.
// - Pattern must not contain null bytes.
// - Options must only contain characters 'i', 'm', 's', 'u', 'x', sorted alphabetically, no repeats.
func (r Regex) Validate() error {
	if strings.ContainsRune(r.Pattern, 0) {
		return fmt.Errorf("firestore: Regex Pattern cannot contain null bytes")
	}
	// Validate options.
	validOptions := map[rune]bool{'i': true, 'm': true, 's': true, 'u': true, 'x': true}
	seen := map[rune]bool{}
	for _, c := range r.Options {
		if !validOptions[c] {
			return fmt.Errorf("firestore: invalid Regex Option: %c", c)
		}
		if seen[c] {
			return fmt.Errorf("firestore: duplicate Regex Option: %c", c)
		}
		seen[c] = true
	}
	// Check if sorted.
	runes := []rune(r.Options)
	if !sort.SliceIsSorted(runes, func(i, j int) bool { return runes[i] < runes[j] }) {
		return fmt.Errorf("firestore: Regex Options must be sorted alphabetically: %s", r.Options)
	}
	return nil
}

// BSONTimestamp represents a BSON Timestamp.
type BSONTimestamp struct {
	Seconds   uint32
	Increment uint32
}

// Decimal128 represents a BSON Decimal128.
type Decimal128 struct {
	String string
}

// Validate validates the Decimal128 string representation.
// For now we just check it is not empty. Real validation might need a library,
// but the backend will validate it anyway.
func (d Decimal128) Validate() error {
	if d.String == "" {
		return fmt.Errorf("firestore: Decimal128 string cannot be empty")
	}
	// Basic regex check for decimal128-like string if possible, or just trust the backend.
	// BSON decimal128 spec is complex.
	return nil
}

// MinKey represents BSON MinKey.
type MinKey struct{}

// MaxKey represents BSON MaxKey.
type MaxKey struct{}

// Binary represents BSON Binary data with subtype != 0.
type Binary struct {
	Subtype byte
	Data    []byte
}

// BSONInt32 represents a BSON 32-bit integer.
type BSONInt32 int32
