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

package storage

import (
	"context"
	"strings"
	"testing"
)

func TestAddFeatureAttributes(t *testing.T) {
	ctx := context.Background()

	// Initial features should be 0.
	if got := featureAttributes(ctx); got != 0 {
		t.Errorf("getFeatureAttributes(empty) = %d; want 0", got)
	}

	// Add a single feature.
	ctx = addFeatureAttributes(ctx, featureMultistreamInMRD)
	if got := featureAttributes(ctx); got != uint32(1<<featureMultistreamInMRD) {
		t.Errorf("getFeatureAttributes(MultiStream) = %d; want %d", got, featureMultistreamInMRD)
	}

	// Add another feature (merge).
	ctx = addFeatureAttributes(ctx, featurePCU)
	want := uint32(1<<featureMultistreamInMRD) | uint32(1<<featurePCU)
	if got := featureAttributes(ctx); got != want {
		t.Errorf("getFeatureAttributes(MultiStream | PCU) = %d; want %d", got, want)
	}

	// Adding same feature should be idempotent.
	ctx = addFeatureAttributes(ctx, featurePCU)
	if got := featureAttributes(ctx); got != want {
		t.Errorf("getFeatureAttributes(MultiStream | PCU | PCU) = %d; want %d", got, want)
	}
}

func TestMergeFeatureAttributes(t *testing.T) {
	tests := []struct {
		name string
		vals []string
		want uint32
	}{
		{
			name: "empty",
			vals: []string{},
			want: 0,
		},
		{
			name: "single value",
			vals: []string{encodeFeatureMask(1)},
			want: 1,
		},
		{
			name: "multiple values",
			vals: []string{encodeFeatureMask(1), encodeFeatureMask(2), encodeFeatureMask(4)},
			want: 7,
		},
		{
			name: "overlapping values",
			vals: []string{encodeFeatureMask(3), encodeFeatureMask(6)},
			want: 7, // 011 | 110 = 111 (7)
		},
		{
			name: "invalid values ignored",
			vals: []string{encodeFeatureMask(1), "invalid", encodeFeatureMask(8)},
			want: 9,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeFeatureAttributes(tc.vals)
			if got != tc.want {
				t.Errorf("mergeFeatureAttributes(%v) = %d; want %d", tc.vals, got, tc.want)
			}
		})
	}
}

func TestEncodeFeatureMaskIsWebSafe(t *testing.T) {
	// The server decodes this header with a strict web-safe base64 decoder,
	// so the encoding must never contain '+' or '/' for any bitmask.
	masks := []uint32{0, 1, 3, 0xF8, 0xFB, 0x3E00, 0x3F00, 0xFFFFFFFF}
	for _, m := range masks {
		got := encodeFeatureMask(m)
		if strings.ContainsAny(got, "+/") {
			t.Errorf("encodeFeatureMask(%#x) = %q; contains standard-alphabet characters", m, got)
		}
		decoded, err := decodeFeatureMask(got)
		if err != nil {
			t.Fatalf("decodeFeatureMask(%q) error: %v", got, err)
		}
		if decoded != m {
			t.Errorf("round trip of %#x = %#x", m, decoded)
		}
	}
}

func TestDecodeFeatureMask(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    uint32
		wantErr bool
	}{
		{name: "url-safe padded", in: "AAAA-w==", want: 0xFB},
		{name: "url-safe unpadded", in: "AAAA-w", want: 0xFB},
		{name: "standard padded (legacy)", in: "AAAA+w==", want: 0xFB},
		{name: "standard unpadded (legacy)", in: "AAAA+w", want: 0xFB},
		{name: "not base64", in: "invalid!", wantErr: true},
		{name: "wrong length", in: "AQID", wantErr: true},
		{name: "oversized payload", in: strings.Repeat("A", 520), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeFeatureMask(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("decodeFeatureMask(%q) error = %v; wantErr %v", tc.in, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("decodeFeatureMask(%q) = %#x; want %#x", tc.in, got, tc.want)
			}
		})
	}
}
