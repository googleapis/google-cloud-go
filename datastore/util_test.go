// Copyright 2022 Google LLC
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

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	pb "google.golang.org/genproto/googleapis/datastore/v1"
	"google.golang.org/grpc"
)

type (
	myBlob   []byte
	myByte   byte
	myString string
)

func makeMyByteSlice(n int) []myByte {
	b := make([]myByte, n)
	for i := range b {
		b[i] = myByte(i)
	}
	return b
}

func makeInt8Slice(n int) []int8 {
	b := make([]int8, n)
	for i := range b {
		b[i] = int8(i)
	}
	return b
}

func makeUint8Slice(n int) []uint8 {
	b := make([]uint8, n)
	for i := range b {
		b[i] = uint8(i)
	}
	return b
}

func newKey(stringID string, parent *Key) *Key {
	return NameKey("kind", stringID, parent)
}

var (
	testKey0     = newKey("name0", nil)
	testKey1a    = newKey("name1", nil)
	testKey1b    = newKey("name1", nil)
	testKey2a    = newKey("name2", testKey0)
	testKey2b    = newKey("name2", testKey0)
	testGeoPt0   = GeoPoint{Lat: 1.2, Lng: 3.4}
	testGeoPt1   = GeoPoint{Lat: 5, Lng: 10}
	testBadGeoPt = GeoPoint{Lat: 1000, Lng: 34}

	ts = time.Unix(1e9, 0).UTC()
)

type B0 struct {
	B []byte `datastore:",noindex"`
}

type B1 struct {
	B []int8
}

type B2 struct {
	B myBlob `datastore:",noindex"`
}

type B3 struct {
	B []myByte `datastore:",noindex"`
}

type B4 struct {
	B [][]byte
}

type C0 struct {
	I int
	C chan int
}

type C1 struct {
	I int
	C *chan int
}

type C2 struct {
	I int
	C []chan int
}

type C3 struct {
	C string
}

type c4 struct {
	C string
}

type E struct{}

type G0 struct {
	G GeoPoint
}

type G1 struct {
	G []GeoPoint
}

type K0 struct {
	K *Key
}

type K1 struct {
	K []*Key
}

type S struct {
	St string
}

type NoOmit struct {
	A string
	B int  `datastore:"Bb"`
	C bool `datastore:",noindex"`
}

type OmitAll struct {
	A string    `datastore:",omitempty"`
	B int       `datastore:"Bb,omitempty"`
	C bool      `datastore:",omitempty,noindex"`
	D time.Time `datastore:",omitempty"`
	F []int     `datastore:",omitempty"`
}

type Omit struct {
	A string    `datastore:",omitempty"`
	B int       `datastore:"Bb,omitempty"`
	C bool      `datastore:",omitempty,noindex"`
	D time.Time `datastore:",omitempty"`
	F []int     `datastore:",omitempty"`
	S `datastore:",omitempty"`
}

type NoOmits struct {
	No []NoOmit `datastore:",omitempty"`
	S  `datastore:",omitempty"`
	Ss S `datastore:",omitempty"`
}

type N0 struct {
	X0
	Nonymous X0
	Ignore   string `datastore:"-"`
	Other    string
}

type N1 struct {
	X0
	Nonymous []X0
	Ignore   string `datastore:"-"`
	Other    string
}

type N2 struct {
	N1    `datastore:"red"`
	Green N1 `datastore:"green"`
	Blue  N1
	White N1 `datastore:"-"`
}

type N3 struct {
	C3 `datastore:"red"`
}

type N4 struct {
	c4
}

type N5 struct {
	c4 `datastore:"red"`
}

type O0 struct {
	I int64
}

type O1 struct {
	I int32
}

type U0 struct {
	U uint
}

type U1 struct {
	U string
}

type T struct {
	T time.Time
}

type X0 struct {
	S string
	I int
	i int
}

type X1 struct {
	S myString
	I int32
	J int64
}

type X2 struct {
	Z string
}

type X3 struct {
	S bool
	I int
}

type Y0 struct {
	B bool
	F []float64
	G []float64
}

type Y1 struct {
	B bool
	F float64
}

type Y2 struct {
	B bool
	F []int64
}

type Pointers struct {
	Pi *int
	Ps *string
	Pb *bool
	Pf *float64
	Pg *GeoPoint
	Pt *time.Time
}

type PointersOmitEmpty struct {
	Pi *int       `datastore:",omitempty"`
	Ps *string    `datastore:",omitempty"`
	Pb *bool      `datastore:",omitempty"`
	Pf *float64   `datastore:",omitempty"`
	Pg *GeoPoint  `datastore:",omitempty"`
	Pt *time.Time `datastore:",omitempty"`
}

func populatedPointers() *Pointers {
	var (
		i int
		s string
		b bool
		f float64
		g GeoPoint
		t time.Time
	)
	return &Pointers{
		Pi: &i,
		Ps: &s,
		Pb: &b,
		Pf: &f,
		Pg: &g,
		Pt: &t,
	}
}

type Tagged struct {
	A int   `datastore:"a,noindex"`
	B []int `datastore:"b"`
	C int   `datastore:",noindex"`
	D int   `datastore:""`
	E int
	I int `datastore:"-"`
	J int `datastore:",noindex" json:"j"`

	Y0 `datastore:"-"`
	Z  chan int `datastore:"-"`
}

type InvalidTagged1 struct {
	I int `datastore:"\t"`
}

type InvalidTagged2 struct {
	I int
	J int `datastore:"I"`
}

type InvalidTagged3 struct {
	X string `datastore:"-,noindex"`
}

type InvalidTagged4 struct {
	X string `datastore:",garbage"`
}

type Inner1 struct {
	W int32
	X string
}

type Inner2 struct {
	Y float64
}

type Inner3 struct {
	Z bool
}

type Inner5 struct {
	WW int
}

type Inner4 struct {
	X Inner5
}

type Outer struct {
	A int16
	I []Inner1
	J Inner2
	Inner3
}

type OuterFlatten struct {
	A      int16
	I      []Inner1 `datastore:",flatten"`
	J      Inner2   `datastore:",flatten,noindex"`
	Inner3 `datastore:",flatten"`
	K      Inner4  `datastore:",flatten"`
	L      *Inner2 `datastore:",flatten"`
}

type OuterEquivalent struct {
	A     int16
	IDotW []int32  `datastore:"I.W"`
	IDotX []string `datastore:"I.X"`
	JDotY float64  `datastore:"J.Y"`
	Z     bool
}

type Dotted struct {
	A DottedA `datastore:"A0.A1.A2"`
}

type DottedA struct {
	B DottedB `datastore:"B3"`
}

type DottedB struct {
	C int `datastore:"C4.C5"`
}

type SliceOfSlices struct {
	I int
	S []struct {
		J int
		F []float64
	} `datastore:",flatten"`
}

type LastFlattened struct {
	Bs []struct{ IDs []string }
	A  struct{ T time.Time } `datastore:",flatten"`
}

type FirstFlattened struct {
	A  struct{ T time.Time } `datastore:",flatten"`
	Bs []struct{ IDs []string }
}

type Recursive struct {
	I int
	R []Recursive
}

type MutuallyRecursive0 struct {
	I int
	R []MutuallyRecursive1
}

type MutuallyRecursive1 struct {
	I int
	R []MutuallyRecursive0
}

type EntityWithKey struct {
	I int
	S string
	K *Key `datastore:"__key__"`
}

type WithNestedEntityWithKey struct {
	N EntityWithKey
}

type WithNonKeyField struct {
	I int
	K string `datastore:"__key__"`
}

type NestedWithNonKeyField struct {
	N WithNonKeyField
}

type Basic struct {
	A string
}

type PtrToStructField struct {
	B *Basic
	C *Basic `datastore:"c,noindex"`
	*Basic
	D []*Basic
}

type EmbeddedTime struct {
	time.Time
}

type SpecialTime struct {
	MyTime EmbeddedTime
}

type Doubler struct {
	S string
	I int64
	B bool
}

type Repeat struct {
	Key   string
	Value []byte
}

type Repeated struct {
	Repeats []Repeat
}

func (d *Doubler) Load(props []Property) error {
	return LoadStruct(d, props)
}

func (d *Doubler) Save() ([]Property, error) {
	// Save the default Property slice to an in-memory buffer (a PropertyList).
	props, err := SaveStruct(d)
	if err != nil {
		return nil, err
	}
	var list PropertyList
	if err := list.Load(props); err != nil {
		return nil, err
	}

	// Edit that PropertyList, and send it on.
	for i := range list {
		switch v := list[i].Value.(type) {
		case string:
			// + means string concatenation.
			list[i].Value = v + v
		case int64:
			// + means integer addition.
			list[i].Value = v + v
		}
	}
	return list.Save()
}

var _ PropertyLoadSaver = (*Doubler)(nil)

type Deriver struct {
	S, Derived, Ignored string
}

func (e *Deriver) Load(props []Property) error {
	for _, p := range props {
		if p.Name != "S" {
			continue
		}
		e.S = p.Value.(string)
		e.Derived = "derived+" + e.S
	}
	return nil
}

func (e *Deriver) Save() ([]Property, error) {
	return []Property{
		{
			Name:  "S",
			Value: e.S,
		},
	}, nil
}

var _ PropertyLoadSaver = (*Deriver)(nil)

type BadMultiPropEntity struct{}

func (e *BadMultiPropEntity) Load(props []Property) error {
	return errors.New("unimplemented")
}

func (e *BadMultiPropEntity) Save() ([]Property, error) {
	// Write multiple properties with the same name "I".
	var props []Property
	for i := 0; i < 3; i++ {
		props = append(props, Property{
			Name:  "I",
			Value: int64(i),
		})
	}
	return props, nil
}

var _ PropertyLoadSaver = (*BadMultiPropEntity)(nil)

type testCase struct {
	desc   string
	src    interface{}
	want   interface{}
	putErr string
	getErr string
}

var testCases = []testCase{
	{
		"chan save fails",
		&C0{I: -1},
		&E{},
		"unsupported struct field",
		"",
	},
	{
		"*chan save fails",
		&C1{I: -1},
		&E{},
		"unsupported struct field",
		"",
	},
	{
		"[]chan save fails",
		&C2{I: -1, C: make([]chan int, 8)},
		&E{},
		"unsupported struct field",
		"",
	},
	{
		"chan load fails",
		&C3{C: "not a chan"},
		&C0{},
		"",
		"type mismatch",
	},
	{
		"*chan load fails",
		&C3{C: "not a *chan"},
		&C1{},
		"",
		"type mismatch",
	},
	{
		"[]chan load fails",
		&C3{C: "not a []chan"},
		&C2{},
		"",
		"type mismatch",
	},
	{
		"empty struct",
		&E{},
		&E{},
		"",
		"",
	},
	{
		"geopoint",
		&G0{G: testGeoPt0},
		&G0{G: testGeoPt0},
		"",
		"",
	},
	{
		"geopoint invalid",
		&G0{G: testBadGeoPt},
		&G0{},
		"invalid GeoPoint value",
		"",
	},
	{
		"geopoint as props",
		&G0{G: testGeoPt0},
		&PropertyList{
			Property{Name: "G", Value: testGeoPt0, NoIndex: false},
		},
		"",
		"",
	},
	{
		"geopoint slice",
		&G1{G: []GeoPoint{testGeoPt0, testGeoPt1}},
		&G1{G: []GeoPoint{testGeoPt0, testGeoPt1}},
		"",
		"",
	},
	{
		"omit empty, all",
		&OmitAll{},
		new(PropertyList),
		"",
		"",
	},
	{
		"omit empty",
		&Omit{},
		&PropertyList{
			Property{Name: "St", Value: "", NoIndex: false},
		},
		"",
		"",
	},
	{
		"omit empty, fields populated",
		&Omit{
			A: "a",
			B: 10,
			C: true,
			F: []int{11},
		},
		&PropertyList{
			Property{Name: "A", Value: "a", NoIndex: false},
			Property{Name: "Bb", Value: int64(10), NoIndex: false},
			Property{Name: "C", Value: true, NoIndex: true},
			Property{Name: "F", Value: []interface{}{int64(11)}, NoIndex: false},
			Property{Name: "St", Value: "", NoIndex: false},
		},
		"",
		"",
	},
	{
		"omit empty, fields populated",
		&Omit{
			A: "a",
			B: 10,
			C: true,
			F: []int{11},
			S: S{St: "string"},
		},
		&PropertyList{
			Property{Name: "A", Value: "a", NoIndex: false},
			Property{Name: "Bb", Value: int64(10), NoIndex: false},
			Property{Name: "C", Value: true, NoIndex: true},
			Property{Name: "F", Value: []interface{}{int64(11)}, NoIndex: false},
			Property{Name: "St", Value: "string", NoIndex: false},
		},
		"",
		"",
	},
	{
		"omit empty does not propagate",
		&NoOmits{
			No: []NoOmit{
				{},
			},
			S:  S{},
			Ss: S{},
		},
		&PropertyList{
			Property{Name: "No", Value: []interface{}{
				&Entity{
					Properties: []Property{
						{Name: "A", Value: "", NoIndex: false},
						{Name: "Bb", Value: int64(0), NoIndex: false},
						{Name: "C", Value: false, NoIndex: true},
					},
				},
			}, NoIndex: false},
			Property{Name: "Ss", Value: &Entity{
				Properties: []Property{
					{Name: "St", Value: "", NoIndex: false},
				},
			}, NoIndex: false},
			Property{Name: "St", Value: "", NoIndex: false},
		},
		"",
		"",
	},
	{
		"key",
		&K0{K: testKey1a},
		&K0{K: testKey1b},
		"",
		"",
	},
	{
		"key with parent",
		&K0{K: testKey2a},
		&K0{K: testKey2b},
		"",
		"",
	},
	{
		"nil key",
		&K0{},
		&K0{},
		"",
		"",
	},
	{
		"all nil keys in slice",
		&K1{[]*Key{nil, nil}},
		&K1{[]*Key{nil, nil}},
		"",
		"",
	},
	{
		"some nil keys in slice",
		&K1{[]*Key{testKey1a, nil, testKey2a}},
		&K1{[]*Key{testKey1b, nil, testKey2b}},
		"",
		"",
	},
	{
		"overflow",
		&O0{I: 1 << 48},
		&O1{},
		"",
		"overflow",
	},
	{
		"time",
		&T{T: time.Unix(1e9, 0)},
		&T{T: time.Unix(1e9, 0)},
		"",
		"",
	},
	{
		"time as props",
		&T{T: time.Unix(1e9, 0)},
		&PropertyList{
			Property{Name: "T", Value: time.Unix(1e9, 0), NoIndex: false},
		},
		"",
		"",
	},
	{
		"uint save",
		&U0{U: 1},
		&U0{},
		"unsupported struct field",
		"",
	},
	{
		"uint load",
		&U1{U: "not a uint"},
		&U0{},
		"",
		"type mismatch",
	},
	{
		"zero",
		&X0{},
		&X0{},
		"",
		"",
	},
	{
		"basic",
		&X0{S: "one", I: 2, i: 3},
		&X0{S: "one", I: 2},
		"",
		"",
	},
	{
		"save string/int load myString/int32",
		&X0{S: "one", I: 2, i: 3},
		&X1{S: "one", I: 2},
		"",
		"",
	},
	{
		"missing fields",
		&X0{S: "one", I: 2, i: 3},
		&X2{},
		"",
		"no such struct field",
	},
	{
		"save string load bool",
		&X0{S: "one", I: 2, i: 3},
		&X3{I: 2},
		"",
		"type mismatch",
	},
	{
		"basic slice",
		&Y0{B: true, F: []float64{7, 8, 9}},
		&Y0{B: true, F: []float64{7, 8, 9}},
		"",
		"",
	},
	{
		"save []float64 load float64",
		&Y0{B: true, F: []float64{7, 8, 9}},
		&Y1{B: true},
		"",
		"requires a slice",
	},
	{
		"save []float64 load []int64",
		&Y0{B: true, F: []float64{7, 8, 9}},
		&Y2{B: true},
		"",
		"type mismatch",
	},
	{
		"single slice is too long",
		&Y0{F: make([]float64, maxIndexedProperties+1)},
		&Y0{},
		"too many indexed properties",
		"",
	},
	{
		"two slices are too long",
		&Y0{F: make([]float64, maxIndexedProperties), G: make([]float64, maxIndexedProperties)},
		&Y0{},
		"too many indexed properties",
		"",
	},
	{
		"one slice and one scalar are too long",
		&Y0{F: make([]float64, maxIndexedProperties), B: true},
		&Y0{},
		"too many indexed properties",
		"",
	},
	{
		"slice of slices of bytes",
		&Repeated{
			Repeats: []Repeat{
				{
					Key:   "key 1",
					Value: []byte("value 1"),
				},
				{
					Key:   "key 2",
					Value: []byte("value 2"),
				},
			},
		},
		&Repeated{
			Repeats: []Repeat{
				{
					Key:   "key 1",
					Value: []byte("value 1"),
				},
				{
					Key:   "key 2",
					Value: []byte("value 2"),
				},
			},
		},
		"",
		"",
	},
	{
		"long blob",
		&B0{B: makeUint8Slice(maxIndexedProperties + 1)},
		&B0{B: makeUint8Slice(maxIndexedProperties + 1)},
		"",
		"",
	},
	{
		"long []int8 is too long",
		&B1{B: makeInt8Slice(maxIndexedProperties + 1)},
		&B1{},
		"too many indexed properties",
		"",
	},
	{
		"short []int8",
		&B1{B: makeInt8Slice(3)},
		&B1{B: makeInt8Slice(3)},
		"",
		"",
	},
	{
		"long myBlob",
		&B2{B: makeUint8Slice(maxIndexedProperties + 1)},
		&B2{B: makeUint8Slice(maxIndexedProperties + 1)},
		"",
		"",
	},
	{
		"short myBlob",
		&B2{B: makeUint8Slice(3)},
		&B2{B: makeUint8Slice(3)},
		"",
		"",
	},
	{
		"long []myByte",
		&B3{B: makeMyByteSlice(maxIndexedProperties + 1)},
		&B3{B: makeMyByteSlice(maxIndexedProperties + 1)},
		"",
		"",
	},
	{
		"short []myByte",
		&B3{B: makeMyByteSlice(3)},
		&B3{B: makeMyByteSlice(3)},
		"",
		"",
	},
	{
		"slice of blobs",
		&B4{B: [][]byte{
			makeUint8Slice(3),
			makeUint8Slice(4),
			makeUint8Slice(5),
		}},
		&B4{B: [][]byte{
			makeUint8Slice(3),
			makeUint8Slice(4),
			makeUint8Slice(5),
		}},
		"",
		"",
	},
	{
		"[]byte must be noindex",
		&PropertyList{
			Property{Name: "B", Value: makeUint8Slice(1501), NoIndex: false},
		},
		nil,
		"[]byte property too long to index",
		"",
	},
	{
		"string must be noindex",
		&PropertyList{
			Property{Name: "B", Value: strings.Repeat("x", 1501), NoIndex: false},
		},
		nil,
		"string property too long to index",
		"",
	},
	{
		"slice of []byte must be noindex",
		&PropertyList{
			Property{Name: "B", Value: []interface{}{
				[]byte("short"),
				makeUint8Slice(1501),
			}, NoIndex: false},
		},
		nil,
		"[]byte property too long to index",
		"",
	},
	{
		"slice of string must be noindex",
		&PropertyList{
			Property{Name: "B", Value: []interface{}{
				"short",
				strings.Repeat("x", 1501),
			}, NoIndex: false},
		},
		nil,
		"string property too long to index",
		"",
	},
	{
		"save tagged load props",
		&Tagged{A: 1, B: []int{21, 22, 23}, C: 3, D: 4, E: 5, I: 6, J: 7},
		&PropertyList{
			// A and B are renamed to a and b; A and C are noindex, I is ignored.
			// Order is sorted as per byName.
			Property{Name: "C", Value: int64(3), NoIndex: true},
			Property{Name: "D", Value: int64(4), NoIndex: false},
			Property{Name: "E", Value: int64(5), NoIndex: false},
			Property{Name: "J", Value: int64(7), NoIndex: true},
			Property{Name: "a", Value: int64(1), NoIndex: true},
			Property{Name: "b", Value: []interface{}{int64(21), int64(22), int64(23)}, NoIndex: false},
		},
		"",
		"",
	},
	{
		"save tagged load tagged",
		&Tagged{A: 1, B: []int{21, 22, 23}, C: 3, D: 4, E: 5, I: 6, J: 7},
		&Tagged{A: 1, B: []int{21, 22, 23}, C: 3, D: 4, E: 5, J: 7},
		"",
		"",
	},
	{
		"invalid tagged1",
		&InvalidTagged1{I: 1},
		&InvalidTagged1{},
		"struct tag has invalid property name",
		"",
	},
	{
		"invalid tagged2",
		&InvalidTagged2{I: 1, J: 2},
		&InvalidTagged2{J: 2},
		"",
		"",
	},
	{
		"invalid tagged3",
		&InvalidTagged3{X: "hello"},
		&InvalidTagged3{},
		"struct tag has invalid property name: \"-\"",
		"",
	},
	{
		"invalid tagged4",
		&InvalidTagged4{X: "hello"},
		&InvalidTagged4{},
		"struct tag has invalid option: \"garbage\"",
		"",
	},
	{
		"doubler",
		&Doubler{S: "s", I: 1, B: true},
		&Doubler{S: "ss", I: 2, B: true},
		"",
		"",
	},
	{
		"save struct load props",
		&X0{S: "s", I: 1},
		&PropertyList{
			Property{Name: "I", Value: int64(1), NoIndex: false},
			Property{Name: "S", Value: "s", NoIndex: false},
		},
		"",
		"",
	},
	{
		"save props load struct",
		&PropertyList{
			Property{Name: "I", Value: int64(1), NoIndex: false},
			Property{Name: "S", Value: "s", NoIndex: false},
		},
		&X0{S: "s", I: 1},
		"",
		"",
	},
	{
		"nil-value props",
		&PropertyList{
			Property{Name: "I", Value: nil, NoIndex: false},
			Property{Name: "B", Value: nil, NoIndex: false},
			Property{Name: "S", Value: nil, NoIndex: false},
			Property{Name: "F", Value: nil, NoIndex: false},
			Property{Name: "K", Value: nil, NoIndex: false},
			Property{Name: "T", Value: nil, NoIndex: false},
			Property{Name: "J", Value: []interface{}{nil, int64(7), nil}, NoIndex: false},
		},
		&struct {
			I int64
			B bool
			S string
			F float64
			K *Key
			T time.Time
			J []int64
		}{
			J: []int64{0, 7, 0},
		},
		"",
		"",
	},
	{
		"save outer load props flatten",
		&OuterFlatten{
			A: 1,
			I: []Inner1{
				{10, "ten"},
				{20, "twenty"},
				{30, "thirty"},
			},
			J: Inner2{
				Y: 3.14,
			},
			Inner3: Inner3{
				Z: true,
			},
			K: Inner4{
				X: Inner5{
					WW: 12,
				},
			},
			L: &Inner2{
				Y: 2.71,
			},
		},
		&PropertyList{
			Property{Name: "A", Value: int64(1), NoIndex: false},
			Property{Name: "I.W", Value: []interface{}{int64(10), int64(20), int64(30)}, NoIndex: false},
			Property{Name: "I.X", Value: []interface{}{"ten", "twenty", "thirty"}, NoIndex: false},
			Property{Name: "J.Y", Value: float64(3.14), NoIndex: true},
			Property{Name: "K.X.WW", Value: int64(12), NoIndex: false},
			Property{Name: "L.Y", Value: float64(2.71), NoIndex: false},
			Property{Name: "Z", Value: true, NoIndex: false},
		},
		"",
		"",
	},
	{
		"load outer props flatten",
		&PropertyList{
			Property{Name: "A", Value: int64(1), NoIndex: false},
			Property{Name: "I.W", Value: []interface{}{int64(10), int64(20), int64(30)}, NoIndex: false},
			Property{Name: "I.X", Value: []interface{}{"ten", "twenty", "thirty"}, NoIndex: false},
			Property{Name: "J.Y", Value: float64(3.14), NoIndex: true},
			Property{Name: "L.Y", Value: float64(2.71), NoIndex: false},
			Property{Name: "Z", Value: true, NoIndex: false},
		},
		&OuterFlatten{
			A: 1,
			I: []Inner1{
				{10, "ten"},
				{20, "twenty"},
				{30, "thirty"},
			},
			J: Inner2{
				Y: 3.14,
			},
			Inner3: Inner3{
				Z: true,
			},
			L: &Inner2{
				Y: 2.71,
			},
		},
		"",
		"",
	},
	{
		"save outer load props",
		&Outer{
			A: 1,
			I: []Inner1{
				{10, "ten"},
				{20, "twenty"},
				{30, "thirty"},
			},
			J: Inner2{
				Y: 3.14,
			},
			Inner3: Inner3{
				Z: true,
			},
		},
		&PropertyList{
			Property{Name: "A", Value: int64(1), NoIndex: false},
			Property{Name: "I", Value: []interface{}{
				&Entity{
					Properties: []Property{
						{Name: "W", Value: int64(10), NoIndex: false},
						{Name: "X", Value: "ten", NoIndex: false},
					},
				},
				&Entity{
					Properties: []Property{
						{Name: "W", Value: int64(20), NoIndex: false},
						{Name: "X", Value: "twenty", NoIndex: false},
					},
				},
				&Entity{
					Properties: []Property{
						{Name: "W", Value: int64(30), NoIndex: false},
						{Name: "X", Value: "thirty", NoIndex: false},
					},
				},
			}, NoIndex: false},
			Property{Name: "J", Value: &Entity{
				Properties: []Property{
					{Name: "Y", Value: float64(3.14), NoIndex: false},
				},
			}, NoIndex: false},
			Property{Name: "Z", Value: true, NoIndex: false},
		},
		"",
		"",
	},
	{
		"save props load outer-equivalent",
		&PropertyList{
			Property{Name: "A", Value: int64(1), NoIndex: false},
			Property{Name: "I.W", Value: []interface{}{int64(10), int64(20), int64(30)}, NoIndex: false},
			Property{Name: "I.X", Value: []interface{}{"ten", "twenty", "thirty"}, NoIndex: false},
			Property{Name: "J.Y", Value: float64(3.14), NoIndex: false},
			Property{Name: "Z", Value: true, NoIndex: false},
		},
		&OuterEquivalent{
			A:     1,
			IDotW: []int32{10, 20, 30},
			IDotX: []string{"ten", "twenty", "thirty"},
			JDotY: 3.14,
			Z:     true,
		},
		"",
		"",
	},
	{
		"dotted names save",
		&Dotted{A: DottedA{B: DottedB{C: 88}}},
		&PropertyList{
			Property{Name: "A0.A1.A2", Value: &Entity{
				Properties: []Property{
					{Name: "B3", Value: &Entity{
						Properties: []Property{
							{Name: "C4.C5", Value: int64(88), NoIndex: false},
						},
					}, NoIndex: false},
				},
			}, NoIndex: false},
		},
		"",
		"",
	},
	{
		"dotted names load",
		&PropertyList{
			Property{Name: "A0.A1.A2", Value: &Entity{
				Properties: []Property{
					{Name: "B3", Value: &Entity{
						Properties: []Property{
							{Name: "C4.C5", Value: 99, NoIndex: false},
						},
					}, NoIndex: false},
				},
			}, NoIndex: false},
		},
		&Dotted{A: DottedA{B: DottedB{C: 99}}},
		"",
		"",
	},
	{
		"save struct load deriver",
		&X0{S: "s", I: 1},
		&Deriver{S: "s", Derived: "derived+s"},
		"",
		"",
	},
	{
		"save deriver load struct",
		&Deriver{S: "s", Derived: "derived+s", Ignored: "ignored"},
		&X0{S: "s"},
		"",
		"",
	},
	{
		"zero time.Time",
		&T{T: time.Time{}},
		&T{T: time.Time{}},
		"",
		"",
	},
	{
		"time.Time near Unix zero time",
		&T{T: time.Unix(0, 4e3)},
		&T{T: time.Unix(0, 4e3)},
		"",
		"",
	},
	{
		"time.Time, far in the future",
		&T{T: time.Date(99999, 1, 1, 0, 0, 0, 0, time.UTC)},
		&T{T: time.Date(99999, 1, 1, 0, 0, 0, 0, time.UTC)},
		"",
		"",
	},
	{
		"time.Time, very far in the past",
		&T{T: time.Date(-300000, 1, 1, 0, 0, 0, 0, time.UTC)},
		&T{},
		"time value out of range",
		"",
	},
	{
		"time.Time, very far in the future",
		&T{T: time.Date(294248, 1, 1, 0, 0, 0, 0, time.UTC)},
		&T{},
		"time value out of range",
		"",
	},
	{
		"structs",
		&N0{
			X0:       X0{S: "one", I: 2, i: 3},
			Nonymous: X0{S: "four", I: 5, i: 6},
			Ignore:   "ignore",
			Other:    "other",
		},
		&N0{
			X0:       X0{S: "one", I: 2},
			Nonymous: X0{S: "four", I: 5},
			Other:    "other",
		},
		"",
		"",
	},
	{
		"slice of structs",
		&N1{
			X0: X0{S: "one", I: 2, i: 3},
			Nonymous: []X0{
				{S: "four", I: 5, i: 6},
				{S: "seven", I: 8, i: 9},
				{S: "ten", I: 11, i: 12},
				{S: "thirteen", I: 14, i: 15},
			},
			Ignore: "ignore",
			Other:  "other",
		},
		&N1{
			X0: X0{S: "one", I: 2},
			Nonymous: []X0{
				{S: "four", I: 5},
				{S: "seven", I: 8},
				{S: "ten", I: 11},
				{S: "thirteen", I: 14},
			},
			Other: "other",
		},
		"",
		"",
	},
	{
		"structs with slices of structs",
		&N2{
			N1: N1{
				X0: X0{S: "rouge"},
				Nonymous: []X0{
					{S: "rosso0"},
					{S: "rosso1"},
				},
			},
			Green: N1{
				X0: X0{S: "vert"},
				Nonymous: []X0{
					{S: "verde0"},
					{S: "verde1"},
					{S: "verde2"},
				},
			},
			Blue: N1{
				X0: X0{S: "bleu"},
				Nonymous: []X0{
					{S: "blu0"},
					{S: "blu1"},
					{S: "blu2"},
					{S: "blu3"},
				},
			},
		},
		&N2{
			N1: N1{
				X0: X0{S: "rouge"},
				Nonymous: []X0{
					{S: "rosso0"},
					{S: "rosso1"},
				},
			},
			Green: N1{
				X0: X0{S: "vert"},
				Nonymous: []X0{
					{S: "verde0"},
					{S: "verde1"},
					{S: "verde2"},
				},
			},
			Blue: N1{
				X0: X0{S: "bleu"},
				Nonymous: []X0{
					{S: "blu0"},
					{S: "blu1"},
					{S: "blu2"},
					{S: "blu3"},
				},
			},
		},
		"",
		"",
	},
	{
		"save structs load props",
		&N2{
			N1: N1{
				X0: X0{S: "rouge"},
				Nonymous: []X0{
					{S: "rosso0"},
					{S: "rosso1"},
				},
			},
			Green: N1{
				X0: X0{S: "vert"},
				Nonymous: []X0{
					{S: "verde0"},
					{S: "verde1"},
					{S: "verde2"},
				},
			},
			Blue: N1{
				X0: X0{S: "bleu"},
				Nonymous: []X0{
					{S: "blu0"},
					{S: "blu1"},
					{S: "blu2"},
					{S: "blu3"},
				},
			},
		},
		&PropertyList{
			Property{Name: "Blue", Value: &Entity{
				Properties: []Property{
					{Name: "I", Value: int64(0), NoIndex: false},
					{Name: "Nonymous", Value: []interface{}{
						&Entity{
							Properties: []Property{
								{Name: "I", Value: int64(0), NoIndex: false},
								{Name: "S", Value: "blu0", NoIndex: false},
							},
						},
						&Entity{
							Properties: []Property{
								{Name: "I", Value: int64(0), NoIndex: false},
								{Name: "S", Value: "blu1", NoIndex: false},
							},
						},
						&Entity{
							Properties: []Property{
								{Name: "I", Value: int64(0), NoIndex: false},
								{Name: "S", Value: "blu2", NoIndex: false},
							},
						},
						&Entity{
							Properties: []Property{
								{Name: "I", Value: int64(0), NoIndex: false},
								{Name: "S", Value: "blu3", NoIndex: false},
							},
						},
					}, NoIndex: false},
					{Name: "Other", Value: "", NoIndex: false},
					{Name: "S", Value: "bleu", NoIndex: false},
				},
			}, NoIndex: false},
			Property{Name: "green", Value: &Entity{
				Properties: []Property{
					{Name: "I", Value: int64(0), NoIndex: false},
					{Name: "Nonymous", Value: []interface{}{
						&Entity{
							Properties: []Property{
								{Name: "I", Value: int64(0), NoIndex: false},
								{Name: "S", Value: "verde0", NoIndex: false},
							},
						},
						&Entity{
							Properties: []Property{
								{Name: "I", Value: int64(0), NoIndex: false},
								{Name: "S", Value: "verde1", NoIndex: false},
							},
						},
						&Entity{
							Properties: []Property{
								{Name: "I", Value: int64(0), NoIndex: false},
								{Name: "S", Value: "verde2", NoIndex: false},
							},
						},
					}, NoIndex: false},
					{Name: "Other", Value: "", NoIndex: false},
					{Name: "S", Value: "vert", NoIndex: false},
				},
			}, NoIndex: false},
			Property{Name: "red", Value: &Entity{
				Properties: []Property{
					{Name: "I", Value: int64(0), NoIndex: false},
					{Name: "Nonymous", Value: []interface{}{
						&Entity{
							Properties: []Property{
								{Name: "I", Value: int64(0), NoIndex: false},
								{Name: "S", Value: "rosso0", NoIndex: false},
							},
						},
						&Entity{
							Properties: []Property{
								{Name: "I", Value: int64(0), NoIndex: false},
								{Name: "S", Value: "rosso1", NoIndex: false},
							},
						},
					}, NoIndex: false},
					{Name: "Other", Value: "", NoIndex: false},
					{Name: "S", Value: "rouge", NoIndex: false},
				},
			}, NoIndex: false},
		},
		"",
		"",
	},
	{
		"nested entity with key",
		&WithNestedEntityWithKey{
			N: EntityWithKey{
				I: 12,
				S: "abcd",
				K: testKey0,
			},
		},
		&WithNestedEntityWithKey{
			N: EntityWithKey{
				I: 12,
				S: "abcd",
				K: testKey0,
			},
		},
		"",
		"",
	},
	{
		"entity with key at top level",
		&EntityWithKey{
			I: 12,
			S: "abc",
			K: testKey0,
		},
		&EntityWithKey{
			I: 12,
			S: "abc",
			K: testKey0,
		},
		"",
		"",
	},
	{
		"entity with key at top level (key is populated on load)",
		&EntityWithKey{
			I: 12,
			S: "abc",
		},
		&EntityWithKey{
			I: 12,
			S: "abc",
			K: testKey0,
		},
		"",
		"",
	},
	{
		"__key__ field not a *Key",
		&NestedWithNonKeyField{
			N: WithNonKeyField{
				I: 12,
				K: "abcd",
			},
		},
		&NestedWithNonKeyField{
			N: WithNonKeyField{
				I: 12,
				K: "abcd",
			},
		},
		"datastore: __key__ field on struct datastore.WithNonKeyField is not a *datastore.Key",
		"",
	},
	{
		"save struct with ptr to struct fields",
		&PtrToStructField{
			&Basic{
				A: "b",
			},
			&Basic{
				A: "c",
			},
			&Basic{
				A: "anon",
			},
			[]*Basic{
				{
					A: "slice0",
				},
				{
					A: "slice1",
				},
			},
		},
		&PropertyList{
			Property{Name: "A", Value: "anon", NoIndex: false},
			Property{Name: "B", Value: &Entity{
				Properties: []Property{
					{Name: "A", Value: "b", NoIndex: false},
				},
			}},
			Property{Name: "D", Value: []interface{}{
				&Entity{
					Properties: []Property{
						{Name: "A", Value: "slice0", NoIndex: false},
					},
				},
				&Entity{
					Properties: []Property{
						{Name: "A", Value: "slice1", NoIndex: false},
					},
				},
			}, NoIndex: false},
			Property{Name: "c", Value: &Entity{
				Properties: []Property{
					{Name: "A", Value: "c", NoIndex: true},
				},
			}, NoIndex: true},
		},
		"",
		"",
	},
	{
		"save and load struct with ptr to struct fields",
		&PtrToStructField{
			&Basic{
				A: "b",
			},
			&Basic{
				A: "c",
			},
			&Basic{
				A: "anon",
			},
			[]*Basic{
				{
					A: "slice0",
				},
				{
					A: "slice1",
				},
			},
		},
		&PtrToStructField{
			&Basic{
				A: "b",
			},
			&Basic{
				A: "c",
			},
			&Basic{
				A: "anon",
			},
			[]*Basic{
				{
					A: "slice0",
				},
				{
					A: "slice1",
				},
			},
		},
		"",
		"",
	},
	{
		"struct with nil ptr to struct fields",
		&PtrToStructField{
			nil,
			nil,
			nil,
			nil,
		},
		new(PropertyList),
		"",
		"",
	},
	{
		"nested load entity with key",
		&WithNestedEntityWithKey{
			N: EntityWithKey{
				I: 12,
				S: "abcd",
				K: testKey0,
			},
		},
		&PropertyList{
			Property{Name: "N", Value: &Entity{
				Key: testKey0,
				Properties: []Property{
					{Name: "I", Value: int64(12), NoIndex: false},
					{Name: "S", Value: "abcd", NoIndex: false},
				},
			},
				NoIndex: false},
		},
		"",
		"",
	},
	{
		"nested save entity with key",
		&PropertyList{
			Property{Name: "N", Value: &Entity{
				Key: testKey0,
				Properties: []Property{
					{Name: "I", Value: int64(12), NoIndex: false},
					{Name: "S", Value: "abcd", NoIndex: false},
				},
			}, NoIndex: false},
		},

		&WithNestedEntityWithKey{
			N: EntityWithKey{
				I: 12,
				S: "abcd",
				K: testKey0,
			},
		},
		"",
		"",
	},
	{
		"anonymous field with tag",
		&N3{
			C3: C3{C: "s"},
		},
		&PropertyList{
			Property{Name: "red", Value: &Entity{
				Properties: []Property{
					{Name: "C", Value: "s", NoIndex: false},
				},
			}, NoIndex: false},
		},
		"",
		"",
	},
	{
		"unexported anonymous field",
		&N4{
			c4: c4{C: "s"},
		},
		&PropertyList{
			Property{Name: "C", Value: "s", NoIndex: false},
		},
		"",
		"",
	},
	{
		"unexported anonymous field with tag",
		&N5{
			c4: c4{C: "s"},
		},
		new(PropertyList),
		"",
		"",
	},
	{
		"save props load structs with ragged fields",
		&PropertyList{
			Property{Name: "red.S", Value: "rot", NoIndex: false},
			Property{Name: "green.Nonymous.I", Value: []interface{}{int64(10), int64(11), int64(12), int64(13)}, NoIndex: false},
			Property{Name: "Blue.Nonymous.I", Value: []interface{}{int64(20), int64(21)}, NoIndex: false},
			Property{Name: "Blue.Nonymous.S", Value: []interface{}{"blau0", "blau1", "blau2"}, NoIndex: false},
		},
		&N2{
			N1: N1{
				X0: X0{S: "rot"},
			},
			Green: N1{
				Nonymous: []X0{
					{I: 10},
					{I: 11},
					{I: 12},
					{I: 13},
				},
			},
			Blue: N1{
				Nonymous: []X0{
					{S: "blau0", I: 20},
					{S: "blau1", I: 21},
					{S: "blau2"},
				},
			},
		},
		"",
		"",
	},
	{
		"save structs with noindex tags",
		&struct {
			A struct {
				X string `datastore:",noindex"`
				Y string
			} `datastore:",noindex"`
			B struct {
				X string `datastore:",noindex"`
				Y string
			}
		}{},
		&PropertyList{
			Property{Name: "A", Value: &Entity{
				Properties: []Property{
					{Name: "X", Value: "", NoIndex: true},
					{Name: "Y", Value: "", NoIndex: true},
				},
			}, NoIndex: true},
			Property{Name: "B", Value: &Entity{
				Properties: []Property{
					{Name: "X", Value: "", NoIndex: true},
					{Name: "Y", Value: "", NoIndex: false},
				},
			}, NoIndex: false},
		},
		"",
		"",
	},
	{
		"embedded struct with name override",
		&struct {
			Inner1 `datastore:"foo"`
		}{},
		&PropertyList{
			Property{Name: "foo", Value: &Entity{
				Properties: []Property{
					{Name: "W", Value: int64(0), NoIndex: false},
					{Name: "X", Value: "", NoIndex: false},
				},
			}, NoIndex: false},
		},
		"",
		"",
	},
	{
		"last field flattened",
		&LastFlattened{},
		&LastFlattened{},
		"",
		"",
	},
	{
		// Request/Bug: https://github.com/googleapis/google-cloud-go/issues/5026
		// User expected this to work as it worked when the last field. (above test)
		"first field flattened",
		&FirstFlattened{},
		nil,
		"flattening nested structs leads to a slice of slices: field \"IDs\"",
		"",
	},
	{
		"slice of slices",
		&SliceOfSlices{},
		nil,
		"flattening nested structs leads to a slice of slices: field \"F\"",
		"",
	},
	{
		"slice of slices, non-defaults",
		&SliceOfSlices{I: 1, S: []struct {
			J int
			F []float64
		}{{J: 2, F: []float64{3.4, 5.6}}}},
		nil,
		"flattening nested structs leads to a slice of slices: field \"F\"",
		"",
	},
	{
		"recursive struct",
		&Recursive{},
		&Recursive{},
		"",
		"",
	},
	{
		"mutually recursive struct",
		&MutuallyRecursive0{},
		&MutuallyRecursive0{},
		"",
		"",
	},
	{
		"non-exported struct fields",
		&struct {
			i, J int64
		}{i: 1, J: 2},
		&PropertyList{
			Property{Name: "J", Value: int64(2), NoIndex: false},
		},
		"",
		"",
	},
	{
		"json.RawMessage",
		&struct {
			J json.RawMessage
		}{
			J: json.RawMessage("rawr"),
		},
		&PropertyList{
			Property{Name: "J", Value: []byte("rawr"), NoIndex: false},
		},
		"",
		"",
	},
	{
		"json.RawMessage to myBlob",
		&struct {
			B json.RawMessage
		}{
			B: json.RawMessage("rawr"),
		},
		&B2{B: myBlob("rawr")},
		"",
		"",
	},
	{
		"repeated property names",
		&PropertyList{
			Property{Name: "A", Value: ""},
			Property{Name: "A", Value: ""},
		},
		nil,
		"duplicate Property",
		"",
	},
	{
		"embedded time field",
		&SpecialTime{MyTime: EmbeddedTime{ts}},
		&SpecialTime{MyTime: EmbeddedTime{ts}},
		"",
		"",
	},
	{
		"embedded time load",
		&PropertyList{
			Property{Name: "MyTime.Time", Value: ts},
		},
		&SpecialTime{MyTime: EmbeddedTime{ts}},
		"",
		"",
	},
	{
		"pointer fields: nil",
		&Pointers{},
		&Pointers{},
		"",
		"",
	},
	{
		"pointer fields: populated with zeroes",
		populatedPointers(),
		populatedPointers(),
		"",
		"",
	},
}

// checkErr returns the empty string if either both want and err are zero,
// or if want is a non-empty substring of err's string representation.
func checkErr(want string, err error) string {
	if err != nil {
		got := err.Error()
		if want == "" || !strings.Contains(got, want) {
			return got
		}
	} else if want != "" {
		return fmt.Sprintf("want error %q", want)
	}
	return ""
}

func TestRoundTrip(t *testing.T) {
	for _, tc := range testCases {
		p, err := saveEntity(testKey0, tc.src)
		if s := checkErr(tc.putErr, err); s != "" {
			t.Errorf("%s: save: %s", tc.desc, s)
			continue
		}
		if p == nil {
			continue
		}
		var got interface{}
		if _, ok := tc.want.(*PropertyList); ok {
			got = new(PropertyList)
		} else {
			got = reflect.New(reflect.TypeOf(tc.want).Elem()).Interface()
		}
		err = loadEntityProto(got, p)
		if s := checkErr(tc.getErr, err); s != "" {
			t.Errorf("%s: load: %s", tc.desc, s)
			continue
		}
		if pl, ok := got.(*PropertyList); ok {
			// Sort by name to make sure we have a deterministic order.
			sortPL(*pl)
		}

		if !testutil.Equal(got, tc.want, cmp.AllowUnexported(X0{}, X2{})) {
			t.Errorf("%s: compare:\ngot:  %+#v\nwant: %+#v", tc.desc, got, tc.want)
			continue
		}
	}
}

type aPtrPLS struct {
	Count int
}

func (pls *aPtrPLS) Load([]Property) error {
	pls.Count++
	return nil
}

func (pls *aPtrPLS) Save() ([]Property, error) {
	return []Property{{Name: "Count", Value: 4}}, nil
}

type aValuePLS struct {
	Count int
}

func (pls aValuePLS) Load([]Property) error {
	pls.Count += 2
	return nil
}

func (pls aValuePLS) Save() ([]Property, error) {
	return []Property{{Name: "Count", Value: 8}}, nil
}

type aValuePtrPLS struct {
	Count int
}

func (pls *aValuePtrPLS) Load([]Property) error {
	pls.Count = 11
	return nil
}

func (pls *aValuePtrPLS) Save() ([]Property, error) {
	return []Property{{Name: "Count", Value: 12}}, nil
}

type aNotPLS struct {
	Count int
}

type plsString string

func (s *plsString) Load([]Property) error {
	*s = "LOADED"
	return nil
}

func (s *plsString) Save() ([]Property, error) {
	return []Property{{Name: "SS", Value: "SAVED"}}, nil
}

func ptrToplsString(s string) *plsString {
	plsStr := plsString(s)
	return &plsStr
}

type aSubPLS struct {
	Foo string
	Bar *aPtrPLS
	Baz aValuePtrPLS
	S   plsString
}

type aSubNotPLS struct {
	Foo string
	Bar *aNotPLS
}

type aSubPLSErr struct {
	Foo string
	Bar aValuePLS
}

type aSubPLSNoErr struct {
	Foo string
	Bar aPtrPLS
}

type GrandparentFlatten struct {
	Parent Parent `datastore:",flatten"`
}

type GrandparentOfPtrFlatten struct {
	Parent ParentOfPtr `datastore:",flatten"`
}

type GrandparentOfSlice struct {
	Parent ParentOfSlice
}

type GrandparentOfSlicePtrs struct {
	Parent ParentOfSlicePtrs
}

type GrandparentOfSliceFlatten struct {
	Parent ParentOfSlice `datastore:",flatten"`
}

type GrandparentOfSlicePtrsFlatten struct {
	Parent ParentOfSlicePtrs `datastore:",flatten"`
}

type Grandparent struct {
	Parent Parent
}

type Parent struct {
	Child  Child
	String plsString
}

type ParentOfPtr struct {
	Child  *Child
	String *plsString
}

type ParentOfSlice struct {
	Children []Child
	Strings  []plsString
}

type ParentOfSlicePtrs struct {
	Children []*Child
	Strings  []*plsString
}

type Child struct {
	I          int
	Grandchild Grandchild
}

type Grandchild struct {
	S string
}

func (c *Child) Load(props []Property) error {
	for _, p := range props {
		if p.Name == "I" {
			c.I++
		} else if p.Name == "Grandchild.S" {
			c.Grandchild.S = "grandchild loaded"
		}
	}

	return nil
}

func (c *Child) Save() ([]Property, error) {
	v := c.I + 1
	return []Property{
		{Name: "I", Value: v},
		{Name: "Grandchild.S", Value: fmt.Sprintf("grandchild saved %d", v)},
	}, nil
}

// DEPRECATED. Please use newMock for new unit tests.
// See https://github.com/googleapis/google-cloud-go/issues/6856.
type fakeDatastoreClient struct {
	pb.DatastoreClient

	// Optional handlers for the datastore methods.
	// Any handlers left undefined will return an error.
	lookup           func(*pb.LookupRequest) (*pb.LookupResponse, error)
	runQuery         func(*pb.RunQueryRequest) (*pb.RunQueryResponse, error)
	beginTransaction func(*pb.BeginTransactionRequest) (*pb.BeginTransactionResponse, error)
	commit           func(*pb.CommitRequest) (*pb.CommitResponse, error)
	rollback         func(*pb.RollbackRequest) (*pb.RollbackResponse, error)
	allocateIds      func(*pb.AllocateIdsRequest) (*pb.AllocateIdsResponse, error)
}

func (c *fakeDatastoreClient) Lookup(ctx context.Context, in *pb.LookupRequest, opts ...grpc.CallOption) (*pb.LookupResponse, error) {
	if c.lookup == nil {
		return nil, errors.New("no lookup handler defined")
	}
	return c.lookup(in)
}
func (c *fakeDatastoreClient) RunQuery(ctx context.Context, in *pb.RunQueryRequest, opts ...grpc.CallOption) (*pb.RunQueryResponse, error) {
	if c.runQuery == nil {
		return nil, errors.New("no runQuery handler defined")
	}
	return c.runQuery(in)
}
func (c *fakeDatastoreClient) BeginTransaction(ctx context.Context, in *pb.BeginTransactionRequest, opts ...grpc.CallOption) (*pb.BeginTransactionResponse, error) {
	if c.beginTransaction == nil {
		return nil, errors.New("no beginTransaction handler defined")
	}
	return c.beginTransaction(in)
}
func (c *fakeDatastoreClient) Commit(ctx context.Context, in *pb.CommitRequest, opts ...grpc.CallOption) (*pb.CommitResponse, error) {
	if c.commit == nil {
		return nil, errors.New("no commit handler defined")
	}
	return c.commit(in)
}
func (c *fakeDatastoreClient) Rollback(ctx context.Context, in *pb.RollbackRequest, opts ...grpc.CallOption) (*pb.RollbackResponse, error) {
	if c.rollback == nil {
		return nil, errors.New("no rollback handler defined")
	}
	return c.rollback(in)
}
func (c *fakeDatastoreClient) AllocateIds(ctx context.Context, in *pb.AllocateIdsRequest, opts ...grpc.CallOption) (*pb.AllocateIdsResponse, error) {
	if c.allocateIds == nil {
		return nil, errors.New("no allocateIds handler defined")
	}
	return c.allocateIds(in)
}
