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
	"encoding/json"
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
	got := Fields(reflect.TypeOf(S1{}), nil)
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

func TestAgainstJSONEncodingNoTags(t *testing.T) {
	// Demonstrates that this package produces the same set of fields as encoding/json.
	s1 := S1{
		Exported:   1,
		unexported: 2,
		Shadow:     3,
		embed1: embed1{
			Em1:    4,
			Dup:    5,
			Shadow: 6,
			embed3: embed3{
				Em3:    7,
				embed5: embed5{x: 8},
			},
		},
		embed2: &embed2{
			Dup: 9,
			embed3: embed3{
				Em3:    10,
				embed5: embed5{x: 11},
			},
			embed4: embed4{
				Em4:    12,
				Dup:    13,
				embed1: &embed1{Em1: 14},
			},
		},
		Anonymous: Anonymous(15),
	}
	bytes, err := json.Marshal(s1)
	if err != nil {
		t.Fatal(err)
	}
	var want S1
	if err := json.Unmarshal(bytes, &want); err != nil {
		t.Fatal(err)
	}

	var got S1
	got.embed2 = &embed2{} // need this because reflection won't create it
	fields := Fields(reflect.TypeOf(got), nil)
	setFields(fields, &got, s1)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%+v\nwant\n%+v", got, want)
	}
}

type S2 struct {
	NoTag      int
	XXX        int           `json:"tag"` // tag name takes precedence
	Anonymous  `json:"anon"` // anonymous non-structs also get their name from the tag
	unexported int           `json:"tag"`
	Embed      `json:"em"`   // embedded structs with tags become fields
	tEmbed1
	tEmbed2
}

type Embed struct {
	Em int
}

type tEmbed1 struct {
	Dup int
	X   int `json:"Dup2"`
}

type tEmbed2 struct {
	Y int `json:"Dup"`  // takes precedence over tEmbed1.Dup because it is tagged
	Z int `json:"Dup2"` // same name as tEmbed1.X and both tagged, so ignored
}

func jsonTagParser(t reflect.StructTag) string { return t.Get("json") }

func TestFieldsWithTags(t *testing.T) {
	got := Fields(reflect.TypeOf(S2{}), jsonTagParser)
	want := []Field{
		{Name: "Dup", NameFromTag: true, Index: []int{6, 0}, Type: intType},
		{Name: "NoTag", Index: []int{0}, Type: intType},
		{Name: "anon", NameFromTag: true, Index: []int{2}, Type: reflect.TypeOf(Anonymous(0))},
		{Name: "em", NameFromTag: true, Index: []int{4}, Type: reflect.TypeOf(Embed{})},
		{Name: "tag", NameFromTag: true, Index: []int{1}, Type: intType},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d fields, want %d", len(got), len(want))
	}
	for i, g := range got {
		w := want[i]
		if !reflect.DeepEqual(g, w) {
			t.Errorf("got %+v, want %+v", g, w)
		}
	}
}

func TestAgainstJSONEncodingWithTags(t *testing.T) {
	// Demonstrates that this package produces the same set of fields as encoding/json.
	s2 := S2{
		NoTag:     1,
		XXX:       2,
		Anonymous: 3,
		Embed: Embed{
			Em: 4,
		},
		tEmbed1: tEmbed1{
			Dup: 5,
			X:   6,
		},
		tEmbed2: tEmbed2{
			Y: 7,
			Z: 8,
		},
	}
	bytes, err := json.Marshal(s2)
	if err != nil {
		t.Fatal(err)
	}
	var want S2
	if err := json.Unmarshal(bytes, &want); err != nil {
		t.Fatal(err)
	}

	var got S2
	fields := Fields(reflect.TypeOf(got), jsonTagParser)
	setFields(fields, &got, s2)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%+v\nwant\n%+v", got, want)
	}
}

func TestUnexportedAnonymousNonStruct(t *testing.T) {
	// An unexported anonymous non-struct field should not be recorded.
	// This is currently a bug in encoding/json.
	// https://github.com/golang/go/issues/18009
	type (
		u int
		v int
		S struct {
			u
			v `json:"x"`
			int
		}
	)

	got := Fields(reflect.TypeOf(S{}), jsonTagParser)
	if len(got) != 0 {
		t.Errorf("got %d fields, want 0", len(got))
	}
}

func TestUnexportedAnonymousStruct(t *testing.T) {
	// An unexported anonymous struct with a tag is ignored.
	// This is currently a bug in encoding/json.
	// https://github.com/golang/go/issues/18009
	type (
		s1 struct{ X int }
		S2 struct {
			s1 `json:"Y"`
		}
	)
	got := Fields(reflect.TypeOf(S2{}), jsonTagParser)
	if len(got) != 0 {
		t.Errorf("got %d fields, want 0", len(got))
	}
}

// Set the fields of dst from those of src.
// dst must be a pointer to a struct value.
// src must be a struct value.
func setFields(fields []Field, dst, src interface{}) {
	vsrc := reflect.ValueOf(src)
	vdst := reflect.ValueOf(dst).Elem()
	for _, f := range fields {
		fdst := vdst.FieldByIndex(f.Index)
		fsrc := vsrc.FieldByIndex(f.Index)
		fdst.Set(fsrc)
	}
}
