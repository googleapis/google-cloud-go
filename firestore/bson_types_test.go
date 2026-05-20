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

func TestObjectID_ParseErrors(t *testing.T) {
	_, err := ParseObjectID("0123456789abcdef0123456") // too short
	if err == nil {
		t.Errorf("expected error for too short ObjectID string")
	}

	_, err = ParseObjectID("0123456789abcdef012345678") // too long
	if err == nil {
		t.Errorf("expected error for too long ObjectID string")
	}

	_, err = ParseObjectID("0123456789abcdef0123456g") // invalid char 'g'
	if err == nil {
		t.Errorf("expected error for invalid character in ObjectID string")
	}

	_, err = ParseObjectID("0123456789ABCDEF01234567") // uppercase
	if err == nil {
		t.Errorf("expected error for uppercase character in ObjectID string")
	}
}
