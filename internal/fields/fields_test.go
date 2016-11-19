// Copyright 2016 Google Inc. All Rights Reserved.
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

package fields

import (
	"reflect"
	"testing"
)

type embed1 struct {
	Em     int
	Dup    int
	Shadow int
}

type embed2 struct {
	Dup int
}

type S struct {
	Exported   int
	unexported int
	Shadow     int // shadows S1.Shadow
	embed1
	*embed2
}

func TestFieldByNameFunc(t *testing.T) {
	for _, test := range []struct {
		name      string
		wantIndex []int
	}{
		{"Exported", []int{0}},
		{"unexported", nil},
		{"Shadow", []int{2}},
		{"Em", []int{3, 0}}, // field in embedded struct
		{"Dup", nil},        // duplicate fields at the same level annihilate each other
	} {
		got, _ := fieldByNameFunc(reflect.TypeOf(S{}), func(s string) bool { return s == test.name })
		if !reflect.DeepEqual(got.Index, test.wantIndex) {
			t.Errorf("%s: got %v, want %v", test.name, got.Index, test.wantIndex)
		}
	}
}
