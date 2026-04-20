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
	"testing"
)

func TestConstantOf_SlicesAndArrays(t *testing.T) {
	tests := []struct {
		name  string
		input any
	}{
		{
			name:  "slice of ints",
			input: []int{1, 2, 3},
		},
		{
			name:  "array of ints",
			input: [3]int{1, 2, 3},
		},
		{
			name:  "slice of strings",
			input: []string{"a", "b", "c"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expr := ConstantOf(tc.input)
			if expr == nil {
				t.Fatalf("ConstantOf returned nil")
			}

			pbVal, err := expr.toProto()
			if err != nil {
				t.Fatalf("toProto() failed with error: %v", err)
			}
			if pbVal == nil {
				t.Fatalf("expected non-nil pb.Value")
			}
		})
	}
}
