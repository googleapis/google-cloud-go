// Copyright 2014 Google Inc. All Rights Reserved.
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

package datastore

import "testing"

func TestKeyEqual(t *testing.T) {
	tests := []struct {
		k0, k1 *Key
		eq     bool
	}{
		{nil, nil, true},
		{&Key{}, nil, false},
		{&Key{}, &Key{}, true},
		{newKey("a", "b", 42, "c", "d"), newKey("a", "b", 42, "c", "d"), true},
		{newKey("", "b", 42, "c", "d"), newKey("a", "b", 42, "c", "d"), false},
		{newKey("a", "", 42, "c", "d"), newKey("a", "b", 42, "c", "d"), false},
		{newKey("a", "b", 0, "c", "d"), newKey("a", "b", 42, "c", "d"), false},
		{newKey("a", "b", 42, "", "d"), newKey("a", "b", 42, "c", "d"), false},
		{newKey("a", "b", 42, "c", ""), newKey("a", "b", 42, "c", "d"), false},
	}
	for _, test := range tests {
		if test.k0.Equal(test.k1) != test.eq {
			t.Errorf("%#v == %#v\n\tgot: %v; want: %v\n", test.k0, test.k1, !test.eq, test.eq)
		} else if test.k1.Equal(test.k0) != test.eq {
			t.Errorf("%#v == %#v\n\tgot: %v; want: %v\n", test.k1, test.k0, !test.eq, test.eq)
		}
	}
}
