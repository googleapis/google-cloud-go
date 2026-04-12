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

func TestPowerOfTwoReplicaSelector_SelectReplica(t *testing.T) {
	selector := newPowerOfTwoReplicaSelectorWithRandom(sequenceIntn(0, 1))
	replicaA := &passthroughChannelEndpoint{address: "replica-a"}
	replicaB := &passthroughChannelEndpoint{address: "replica-b"}

	selected := selector.selectReplica(
		[]channelEndpoint{replicaA, replicaB},
		func(endpoint channelEndpoint) float64 {
			if endpoint.Address() == "replica-a" {
				return 50
			}
			return 10
		},
	)

	if selected != replicaB {
		t.Fatalf("selected = %v, want replica-b", selected)
	}
}

func TestPowerOfTwoReplicaSelector_SelectReplicaEmpty(t *testing.T) {
	selector := newPowerOfTwoReplicaSelector()
	if got := selector.selectReplica(nil, nil); got != nil {
		t.Fatalf("selectReplica(nil) = %v, want nil", got)
	}
}

func sequenceIntn(values ...int) func(int) int {
	index := 0
	return func(n int) int {
		value := values[index]
		index++
		if n <= 0 {
			return 0
		}
		return value % n
	}
}
