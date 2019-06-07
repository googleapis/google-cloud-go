// Copyright 2019 Google LLC
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

package testutil

import (
	"fmt"

	"github.com/google/go-cmp/cmp"
)

// OrderedKeyMsg is a message with key and data.
type OrderedKeyMsg struct {
	Key  string
	Data string
}

// VerifyKeyOrdering verifies that received data was published and received in
// key order.
//
// TODO(deklerk): account for consistent redelivery.
func VerifyKeyOrdering(publishData, receiveData []OrderedKeyMsg) error {
	publishKeyMap := make(map[string][]string)
	receiveKeyMap := make(map[string][]string)

	for _, d := range publishData {
		if d.Key == "" {
			continue
		}
		publishKeyMap[d.Key] = append(publishKeyMap[d.Key], d.Data)
	}

	for _, d := range receiveData {
		if d.Key == "" {
			continue
		}
		receiveKeyMap[d.Key] = append(receiveKeyMap[d.Key], d.Data)
	}

	if len(publishKeyMap) != len(receiveKeyMap) {
		return fmt.Errorf("published %d keys, received %d - expected them to be equal but they were not", len(publishKeyMap), len(receiveKeyMap))
	}

	for k, pb := range publishKeyMap {
		rd, ok := receiveKeyMap[k]
		if !ok {
			// Should never happen unless we're using a topic that's got some
			// data in it already / topic test pollution / etc. So, basically,
			// make sure a fresh topic is always used for integration tests.
			return fmt.Errorf("saw key %s, but we never published this key", k)
		}

		if diff := cmp.Diff(pb, rd); diff != "" {
			return fmt.Errorf("%s: got -, want +\n\t%s", k, diff)
		}
	}

	return nil
}
