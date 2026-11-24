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

package adapters

import "testing"

func TestAdaptersExist(t *testing.T) {
	if DefaultReadRowRequestAdapter == nil {
		t.Error("expected DefaultReadRowRequestAdapter to be non-nil")
	}
	if DefaultReadRowResponseAdapter == nil {
		t.Error("expected DefaultReadRowResponseAdapter to be non-nil")
	}
	if DefaultMutateRowRequestAdapter == nil {
		t.Error("expected DefaultMutateRowRequestAdapter to be non-nil")
	}
	if DefaultMutateRowResponseAdapter == nil {
		t.Error("expected DefaultMutateRowResponseAdapter to be non-nil")
	}
}
