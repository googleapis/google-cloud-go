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

import (
	"math"
	"testing"
)

func TestPowerOfTwoReplicaSelectorChooseIndex(t *testing.T) {
	selector := powerOfTwoReplicaSelector{
		intn: func(max int) int {
			switch max {
			case 3:
				return 0
			case 2:
				return 0
			default:
				t.Fatalf("unexpected intn bound %d", max)
				return 0
			}
		},
	}
	scores := []float64{10, 5, 1}

	selected := selector.chooseIndex(len(scores), func(index int) float64 {
		return scores[index]
	})
	if got, want := selected, 1; got != want {
		t.Fatalf("chooseIndex() = %d, want %d", got, want)
	}
}

func TestPowerOfTwoReplicaSelectorChooseIndexTreatsNaNAsWorst(t *testing.T) {
	selector := powerOfTwoReplicaSelector{
		intn: func(max int) int {
			switch max {
			case 2:
				return 0
			case 1:
				return 0
			default:
				t.Fatalf("unexpected intn bound %d", max)
				return 0
			}
		},
	}
	scores := []float64{math.NaN(), 7}

	selected := selector.chooseIndex(len(scores), func(index int) float64 {
		return scores[index]
	})
	if got, want := selected, 1; got != want {
		t.Fatalf("chooseIndex() = %d, want %d", got, want)
	}
}
