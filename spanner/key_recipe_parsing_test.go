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

import "testing"

func TestParseTimestamp_TruncatesFractionalSeconds(t *testing.T) {
	secondsA, nanosA, err := parseTimestamp("0000-01-01T00:00:00.123456789Z")
	if err != nil {
		t.Fatalf("parseTimestamp returned error: %v", err)
	}
	secondsB, nanosB, err := parseTimestamp("0000-01-01T00:00:00.1234567890Z")
	if err != nil {
		t.Fatalf("parseTimestamp returned error for >9 digits: %v", err)
	}
	if secondsA != secondsB || nanosA != nanosB {
		t.Fatalf("expected truncation to preserve value; got (%d,%d) vs (%d,%d)", secondsA, nanosA, secondsB, nanosB)
	}
}

func TestParseTimestamp_RejectsNonUTCOffset(t *testing.T) {
	if _, _, err := parseTimestamp("2025-01-01T00:00:00+07:00"); err == nil {
		t.Fatal("expected error for non-Z timezone")
	}
}

func TestParseDate_AllowsYearZero(t *testing.T) {
	if _, err := parseDate("0000-01-01"); err != nil {
		t.Fatalf("expected year 0000 to be accepted, got error: %v", err)
	}
}

func TestParseUUID_CanonicalAndOptionalBraces(t *testing.T) {
	valid := []string{
		"123e4567-e89b-12d3-a456-426614174000",
		"{123e4567-e89b-12d3-a456-426614174000}",
		"123E4567-E89B-12D3-A456-426614174000",
	}
	for _, in := range valid {
		if _, _, err := parseUUID(in); err != nil {
			t.Fatalf("expected UUID %q to be valid, got: %v", in, err)
		}
	}

	invalid := []string{
		"123e4567e89b12d3a456426614174000",
		"123e4567-e89b-12d3-a456-42661417400",
		"{123e4567-e89b-12d3-a456-426614174000",
		"123e4567-e89b-12d3-a456-426614174000}",
		"123e4567-e89b-12d3-a456-42661417400g",
	}
	for _, in := range invalid {
		if _, _, err := parseUUID(in); err == nil {
			t.Fatalf("expected UUID %q to be rejected", in)
		}
	}
}
