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

type S1 struct {
	Exported   int
	unexported int
	Shadow     int // shadows S1.Shadow
	embed1
	*embed2
	Anonymous
}

var intType = reflect.TypeOf(int(0))

func TestFieldsNoTags(t *testing.T) {
	got := Fields(reflect.TypeOf(S1{}))
	want := []Field{
		{Name: "Anonymous", Index: []int{5}, Type: reflect.TypeOf(Anonymous(0))},
		{Name: "Em1", Index: []int{3, 0}, Type: intType},
		{Name: "Em4", Index: []int{4, 2, 0}, Type: intType},
		{Name: "Exported", Index: []int{0}, Type: intType},
		{Name: "Shadow", Index: []int{2}, Type: intType},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d fields, want %d", len(got), len(want))
	}
	for i, g := range got {
		w := want[i]
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %+v, want %+v", g, w)
		}
	}
}

type S2 struct {
	NoTag     int
	XXX       int           `test:"tag"` // tag name takes precedence
	Anonymous `test:"anon"` // anonymous non-structs also get their name from the tag
	embed1    `test:"em"`   // embedded structs with tags become fields
	tEmbed1
	tEmbed2
}

type tEmbed1 struct {
	Dup int
	X   int `test:"Dup2"`
}

type tEmbed2 struct {
	Y int `test:"Dup"`  // takes precedence over tEmbed1.Dup because it is tagged
	Z int `test:"Dup2"` // same name as tEmbed1.X and both tagged, so ignored
}

func TestFieldsWithTags(t *testing.T) {
	got := Fields(reflect.TypeOf(S2{}))
	want := []Field{
		{Name: "Dup", NameFromTag: true, Index: []int{5, 0}, Type: intType},
		{Name: "NoTag", Index: []int{0}, Type: intType},
		{Name: "anon", NameFromTag: true, Index: []int{2}, Type: reflect.TypeOf(Anonymous(0))},
		{Name: "em", NameFromTag: true, Index: []int{3}, Type: reflect.TypeOf(embed1{})},
		{Name: "tag", NameFromTag: true, Index: []int{1}, Type: intType},
	}
	if len(got) != len(want) {
		fmt.Println(got)
		t.Fatalf("got %d fields, want %d", len(got), len(want))
	}
	for i, g := range got {
		w := want[i]
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %+v, want %+v", g, w)
		}
	}
}
