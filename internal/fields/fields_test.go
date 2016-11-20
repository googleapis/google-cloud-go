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
	"fmt"
	"reflect"
	"testing"
)

type embed1 struct {
	Em1    int
	Dup    int // annihilates with embed2.Dup
	Shadow int
	embed3
}

type embed2 struct {
	Dup int
	embed3
	embed4
}

type embed3 struct {
	Em3 int // annihilated because embed3 is in both embed1 and embed2
	embed5
}

type embed4 struct {
	Em4     int
	Dup     int // annihilation of Dup in embed1, embed2 hides this Dup
	*embed1     // ignored because it occurs at a higher level
}

type embed5 struct {
	x int
}

type Anonymous int

type S struct {
	Exported   int
	unexported int
	Shadow     int // shadows S1.Shadow
	embed1
	*embed2
	Anonymous
}

func TestFields(t *testing.T) {
	got := Fields(reflect.TypeOf(S{}))
	intType := reflect.TypeOf(int(0))
	want := []struct {
		name  string
		index []int
		typ   reflect.Type
	}{
		{"Anonymous", []int{5}, reflect.TypeOf(Anonymous(0))},
		{"Em1", []int{3, 0}, intType},
		{"Em4", []int{4, 2, 0}, intType},
		{"Exported", []int{0}, intType},
		{"Shadow", []int{2}, intType},
	}
	if len(got) != len(want) {
		fmt.Println(got)
		t.Fatalf("got %d fields, want %d", len(got), len(want))
	}
	for i, g := range got {
		w := want[i]
		if g.Name != w.name {
			t.Errorf("name: got %q, want %q", g.Name, w.name)
		}
		if !reflect.DeepEqual(g.Index, w.index) {
			t.Errorf("index: got %v, want %v", g.Index, w.index)
		}
		if g.Type != w.typ {
			t.Errorf("type: got %s, want %s", g.Type, w.typ)
		}
	}
}
