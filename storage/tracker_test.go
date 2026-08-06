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
			vals: []string{encodeUint32(1)},
			want: 1,
		},
		{
			name: "multiple values",
			vals: []string{encodeUint32(1), encodeUint32(2), encodeUint32(4)},
			want: 7,
		},
		{
			name: "overlapping values",
			vals: []string{encodeUint32(3), encodeUint32(6)},
			want: 7, // 011 | 110 = 111 (7)
		},
		{
			name: "invalid values ignored",
			vals: []string{encodeUint32(1), "invalid", encodeUint32(8)},
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
