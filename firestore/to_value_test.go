// Copyright 2017 Google LLC
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
	"fmt"
	"reflect"
	"testing"
	"time"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"google.golang.org/genproto/googleapis/type/latlng"
	ts "google.golang.org/protobuf/types/known/timestamppb"
)

type testStruct1 struct {
	B  bool
	I  int
	U  uint32
	F  float64
	S  string
	Y  []byte
	T  time.Time
	Ts *ts.Timestamp
	G  *latlng.LatLng
	L  []int
	M  map[string]int
	P  *int
}

var (
	p = new(int)

	testVal1 = testStruct1{
		B:  true,
		I:  1,
		U:  2,
		F:  3.0,
		S:  "four",
		Y:  []byte{5},
		T:  tm,
		Ts: ptm,
		G:  ll,
		L:  []int{6},
		M:  map[string]int{"a": 7},
		P:  p,
	}

	mapVal1 = mapval(map[string]*pb.Value{
		"B":  boolval(true),
		"I":  intval(1),
		"U":  intval(2),
		"F":  floatval(3),
		"S":  {ValueType: &pb.Value_StringValue{StringValue: "four"}},
		"Y":  bytesval([]byte{5}),
		"T":  tsval(tm),
		"Ts": {ValueType: &pb.Value_TimestampValue{TimestampValue: ptm}},
		"G":  geoval(ll),
		"L":  arrayval(intval(6)),
		"M":  mapval(map[string]*pb.Value{"a": intval(7)}),
		"P":  intval(8),
	})
)

// TODO descriptions
// TODO cause the array failure
func TestToProtoValue_Conversions(t *testing.T) {
	*p = 8
	for _, test := range []struct {
		desc string
		in   interface{}
		want *pb.Value
	}{
		{
			desc: "nil",
			in:   nil,
			want: nullValue,
		},
		{
			desc: "nil slice",
			in:   []int(nil),
			want: nullValue,
		},
		{
			desc: "nil map",
			in:   map[string]int(nil),
			want: nullValue,
		},
		{
			desc: "nil struct",
			in:   (*testStruct1)(nil),
			want: nullValue,
		},
		{
			desc: "nil timestamp",
			in:   (*ts.Timestamp)(nil),
			want: nullValue,
		},
		{
			desc: "nil latlng",
			in:   (*latlng.LatLng)(nil),
			want: nullValue,
		},
		{
			desc: "nil docref",
			in:   (*DocumentRef)(nil),
			want: nullValue,
		},
		{
			desc: "bool",
			in:   true,
			want: boolval(true),
		},
		{
			desc: "int",
			in:   3,
			want: intval(3),
		},
		{
			desc: "uint32",
			in:   uint32(3),
			want: intval(3),
		},
		{
			desc: "float",
			in:   1.5,
			want: floatval(1.5),
		},
		{
			desc: "string",
			in:   "str",
			want: strval("str"),
		},
		{
			desc: "byte slice",
			in:   []byte{1, 2},
			want: bytesval([]byte{1, 2}),
		},
		{
			desc: "date time",
			in:   tm,
			want: tsval(tm),
		},
		{
			desc: "pointer to timestamp",
			in:   ptm,
			want: &pb.Value{ValueType: &pb.Value_TimestampValue{TimestampValue: ptm}},
		},
		{
			desc: "pointer to latlng",
			in:   ll,
			want: geoval(ll),
		},
		{
			desc: "populated slice",
			in:   []int{1, 2},
			want: arrayval(intval(1), intval(2)),
		},
		{
			desc: "pointer to populated slice",
			in:   &[]int{1, 2},
			want: arrayval(intval(1), intval(2)),
		},
		{
			desc: "empty slice",
			in:   []int{},
			want: arrayval(),
		},
		{
			desc: "populated map",
			in:   map[string]int{"a": 1, "b": 2},
			want: mapval(map[string]*pb.Value{"a": intval(1), "b": intval(2)}),
		},
		{
			desc: "empty map",
			in:   map[string]int{},
			want: mapval(map[string]*pb.Value{}),
		},
		{
			desc: "int",
			in:   p,
			want: intval(8),
		},
		{
			desc: "pointer to int",
			in:   &p,
			want: intval(8),
		},
		{
			desc: "populated map",
			in:   map[string]interface{}{"a": 1, "p": p, "s": "str"},
			want: mapval(map[string]*pb.Value{"a": intval(1), "p": intval(8), "s": strval("str")}),
		},
		{
			desc: "map with timestamp",
			in:   map[string]fmt.Stringer{"a": tm},
			want: mapval(map[string]*pb.Value{"a": tsval(tm)}),
		},
		{
			desc: "struct",
			in:   testVal1,
			want: mapVal1,
		},
		{
			desc: "array",
			in:   [1]int{7},
			want: arrayval(intval(7)),
		},
		{
			desc: "pointer to docref",
			in: &DocumentRef{
				ID:   "d",
				Path: "projects/P/databases/D/documents/c/d",
				Parent: &CollectionRef{
					ID:         "c",
					parentPath: "projects/P/databases/D",
					Path:       "projects/P/databases/D/documents/c",
					Query:      Query{collectionID: "c", parentPath: "projects/P/databases/D"},
				},
			},
			want: refval("projects/P/databases/D/documents/c/d"),
		},
		{
			desc: "Transforms are removed, which can lead to leaving nil",
			in:   map[string]interface{}{"a": ServerTimestamp},
			want: nil,
		},
		{
			desc: "Transform nested in map is ignored",
			in: map[string]interface{}{
				"a": map[string]interface{}{
					"b": map[string]interface{}{
						"c": ServerTimestamp,
					},
				},
			},
			want: nil,
		},
		{
			desc: "Transforms nested in map are ignored",
			in: map[string]interface{}{
				"a": map[string]interface{}{
					"b": map[string]interface{}{
						"c": ServerTimestamp,
						"d": ServerTimestamp,
					},
				},
			},
			want: nil,
		},
		{
			desc: "int nested in map is kept whilst Transforms are ignored",
			in: map[string]interface{}{
				"a": map[string]interface{}{
					"b": map[string]interface{}{
						"c": ServerTimestamp,
						"d": ServerTimestamp,
						"e": 1,
					},
				},
			},
			want: mapval(map[string]*pb.Value{
				"a": mapval(map[string]*pb.Value{
					"b": mapval(map[string]*pb.Value{"e": intval(1)}),
				}),
			}),
		},

		// Transforms are allowed in maps, but won't show up in the returned proto. Instead, we rely
		// on seeing sawTransforms=true and a call to extractTransforms.
		{
			desc: "Transforms in map are ignored, other values are kept (ServerTimestamp)",
			in:   map[string]interface{}{"a": ServerTimestamp, "b": 5},
			want: mapval(map[string]*pb.Value{"b": intval(5)}),
		},
		{
			desc: "Transforms in map are ignored, other values are kept (ArrayUnion)",
			in:   map[string]interface{}{"a": ArrayUnion(1, 2, 3), "b": 5},
			want: mapval(map[string]*pb.Value{"b": intval(5)}),
		},
		{
			desc: "Transforms in map are ignored, other values are kept (ArrayRemove)",
			in:   map[string]interface{}{"a": ArrayRemove(1, 2, 3), "b": 5},
			want: mapval(map[string]*pb.Value{"b": intval(5)}),
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			got, _, err := toProtoValue(reflect.ValueOf(test.in))
			if err != nil {
				t.Fatalf("%v (%T): %v", test.in, test.in, err)
			}
			if !testEqual(got, test.want) {
				t.Fatalf("%+v (%T):\ngot\n%+v\nwant\n%+v", test.in, test.in, got, test.want)
			}
		})
	}
}

type stringy struct{}

func (stringy) String() string { return "stringy" }

func TestToProtoValue_Errors(t *testing.T) {
	for _, in := range []interface{}{
		uint64(0),                               // a bad fit for int64
		map[int]bool{},                          // map key type is not string
		make(chan int),                          // can't handle type
		map[string]fmt.Stringer{"a": stringy{}}, // only empty interfaces
		ServerTimestamp,                         // ServerTimestamp can only be a field value
		struct{ A interface{} }{A: ServerTimestamp},
		map[string]interface{}{"a": []interface{}{ServerTimestamp}},
		map[string]interface{}{"a": []interface{}{
			map[string]interface{}{"b": ServerTimestamp},
		}},
		Delete, // Delete should never appear
		[]interface{}{Delete},
		map[string]interface{}{"a": Delete},
		map[string]interface{}{"a": []interface{}{Delete}},

		// Transforms are not allowed to occur in an array.
		[]interface{}{ServerTimestamp},
		[]interface{}{ArrayUnion(1, 2, 3)},
		[]interface{}{ArrayRemove(1, 2, 3)},

		// Transforms are not allowed to occur in a struct.
		struct{ A interface{} }{A: ServerTimestamp},
		struct{ A interface{} }{A: ArrayUnion()},
		struct{ A interface{} }{A: ArrayRemove()},
	} {
		_, _, err := toProtoValue(reflect.ValueOf(in))
		if err == nil {
			t.Errorf("%v: got nil, want error", in)
		}
	}
}

// Helper struct for IsZero() method testing
type StructWithIsZero struct {
	ID     int
	Data   string
	isZero bool // internal flag to control IsZero()
}

func (s StructWithIsZero) IsZero() bool {
	return s.isZero
}

// Example test struct
type TestStructForOmitZero struct {
	// Primitives
	IntVal   int       `firestore:"IntVal,omitzero"`
	StrVal   string    `firestore:"StrVal,omitzero"`
	BoolVal  bool      `firestore:"BoolVal,omitzero"`
	FloatVal float64   `firestore:"FloatVal,omitzero"`
	TimeVal  time.Time `firestore:"TimeVal,omitzero"`
	// Pointers
	PtrIntVal    *int              `firestore:"PtrIntVal,omitzero"`
	PtrStructVal *StructWithIsZero `firestore:"PtrStructVal,omitzero"`
	// Slices/Maps
	SliceVal []string          `firestore:"SliceVal,omitzero"`
	MapVal   map[string]string `firestore:"MapVal,omitzero"`
	// IsZero method
	CustomIsZero    StructWithIsZero  `firestore:"CustomIsZero,omitzero"`
	PtrCustomIsZero *StructWithIsZero `firestore:"PtrCustomIsZero,omitzero"`
	// omitempty and omitzero
	BothEmptyZeroStr string `firestore:"BothEmptyZeroStr,omitempty,omitzero"`
	OnlyOmitEmptyStr string `firestore:"OnlyOmitEmptyStr,omitempty"`
	// No omitzero
	NoOmitZeroInt int `firestore:"NoOmitZeroInt"`
}

func TestToProtoValue_OmitZeroTag(t *testing.T) {
	zeroIntVal := 0
	nonZeroIntVal := 42
	nonZeroTimeVal := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		instance TestStructForOmitZero
		want     *pb.Value
	}{
		{
			name: "All zero values with omitzero",
			instance: TestStructForOmitZero{
				IntVal:           0,
				StrVal:           "",
				BoolVal:          false,
				FloatVal:         0.0,
				TimeVal:          time.Time{},
				PtrIntVal:        nil,
				PtrStructVal:     nil,
				SliceVal:         nil,
				MapVal:           nil,
				CustomIsZero:     StructWithIsZero{ID: 1, Data: "not zero but IsZero returns true", isZero: true},
				PtrCustomIsZero:  &StructWithIsZero{ID: 2, Data: "also not zero but IsZero true", isZero: true},
				BothEmptyZeroStr: "",
				OnlyOmitEmptyStr: "",
				NoOmitZeroInt:    0, // This should be included
			},
			want: mapval(map[string]*pb.Value{
				"NoOmitZeroInt": intval(0), // Only this and OnlyOmitEmptyStr (if non-empty, which it is not here) should remain
			}),
		},
		{
			name: "All zero values with omitzero - empty non-nil slice and map",
			instance: TestStructForOmitZero{
				IntVal:           0,
				StrVal:           "",
				BoolVal:          false,
				FloatVal:         0.0,
				TimeVal:          time.Time{},
				PtrIntVal:        nil,
				PtrStructVal:     nil,
				SliceVal:         []string{},
				MapVal:           map[string]string{},
				CustomIsZero:     StructWithIsZero{ID: 1, Data: "not zero but IsZero returns true", isZero: true},
				PtrCustomIsZero:  &StructWithIsZero{ID: 2, Data: "also not zero but IsZero true", isZero: true},
				BothEmptyZeroStr: "",
				OnlyOmitEmptyStr: "",
				NoOmitZeroInt:    0,
			},
			want: mapval(map[string]*pb.Value{
				"NoOmitZeroInt": intval(0),
			}),
		},
		{
			name: "Non-zero values with omitzero",
			instance: TestStructForOmitZero{
				IntVal:           1,
				StrVal:           "foo",
				BoolVal:          true,
				FloatVal:         1.1,
				TimeVal:          nonZeroTimeVal,
				PtrIntVal:        &nonZeroIntVal,
				PtrStructVal:     &StructWithIsZero{ID: 3, Data: "non-zero struct", isZero: false},
				SliceVal:         []string{"a"},
				MapVal:           map[string]string{"k": "v"},
				CustomIsZero:     StructWithIsZero{ID: 4, Data: "I am not zero", isZero: false},
				PtrCustomIsZero:  &StructWithIsZero{ID: 5, Data: "I am also not zero", isZero: false},
				BothEmptyZeroStr: "not empty or zero",
				OnlyOmitEmptyStr: "not empty",
				NoOmitZeroInt:    5,
			},
			want: mapval(map[string]*pb.Value{
				"IntVal":           intval(1),
				"StrVal":           strval("foo"),
				"BoolVal":          boolval(true),
				"FloatVal":         floatval(1.1),
				"TimeVal":          tsval(nonZeroTimeVal),
				"PtrIntVal":        intval(nonZeroIntVal),
				"PtrStructVal":     mapval(map[string]*pb.Value{"ID": intval(3), "Data": strval("non-zero struct")}),
				"SliceVal":         arrayval(strval("a")),
				"MapVal":           mapval(map[string]*pb.Value{"k": strval("v")}),
				"CustomIsZero":     mapval(map[string]*pb.Value{"ID": intval(4), "Data": strval("I am not zero")}),
				"PtrCustomIsZero":  mapval(map[string]*pb.Value{"ID": intval(5), "Data": strval("I am also not zero")}),
				"BothEmptyZeroStr": strval("not empty or zero"),
				"OnlyOmitEmptyStr": strval("not empty"),
				"NoOmitZeroInt":    intval(5),
			}),
		},
		{
			name: "Pointer to zero value",
			instance: TestStructForOmitZero{
				PtrIntVal:     &zeroIntVal, // Pointer to 0
				NoOmitZeroInt: 0,
			},
			want: mapval(map[string]*pb.Value{
				"NoOmitZeroInt": intval(0), // PtrIntVal should be omitted
			}),
		},
		{
			name: "Pointer to zero value struct (IsZero returns true)",
			instance: TestStructForOmitZero{
				PtrStructVal:  &StructWithIsZero{ID: 0, Data: "", isZero: true},
				NoOmitZeroInt: 0,
			},
			want: mapval(map[string]*pb.Value{
				"NoOmitZeroInt": intval(0), // PtrStructVal should be omitted
			}),
		},
		{
			name: "Pointer to non-zero value struct (IsZero returns false)",
			instance: TestStructForOmitZero{
				PtrStructVal:  &StructWithIsZero{ID: 1, Data: "data", isZero: false},
				NoOmitZeroInt: 0,
			},
			want: mapval(map[string]*pb.Value{
				"PtrStructVal":  mapval(map[string]*pb.Value{"ID": intval(1), "Data": strval("data")}),
				"NoOmitZeroInt": intval(0),
			}),
		},
		{
			name: "IsZero returns false for a Go zero value struct",
			instance: TestStructForOmitZero{
				CustomIsZero:  StructWithIsZero{ID: 0, Data: "", isZero: false}, // Go zero, but IsZero() is false
				NoOmitZeroInt: 0,
			},
			want: mapval(map[string]*pb.Value{
				"CustomIsZero":  mapval(map[string]*pb.Value{"ID": intval(0), "Data": strval("")}), // Should be included
				"NoOmitZeroInt": intval(0),
			}),
		},
		{
			name: "IsZero returns true for a non-Go zero value struct",
			instance: TestStructForOmitZero{
				CustomIsZero:  StructWithIsZero{ID: 1, Data: "stuff", isZero: true}, // Not Go zero, but IsZero() is true
				NoOmitZeroInt: 0,
			},
			want: mapval(map[string]*pb.Value{
				"NoOmitZeroInt": intval(0), // CustomIsZero should be omitted
			}),
		},
		{
			name: "BothEmptyZeroStr: empty string (omitted by omitempty and omitzero)",
			instance: TestStructForOmitZero{
				BothEmptyZeroStr: "",
				NoOmitZeroInt:    0,
			},
			want: mapval(map[string]*pb.Value{
				"NoOmitZeroInt": intval(0),
			}),
		},
		{
			name: "BothEmptyZeroStr: non-empty string (included)",
			instance: TestStructForOmitZero{
				BothEmptyZeroStr: "text",
				NoOmitZeroInt:    0,
			},
			want: mapval(map[string]*pb.Value{
				"BothEmptyZeroStr": strval("text"),
				"NoOmitZeroInt":    intval(0),
			}),
		},
		{
			name: "OnlyOmitEmptyStr: empty string (omitted by omitempty)",
			instance: TestStructForOmitZero{
				OnlyOmitEmptyStr: "",
				NoOmitZeroInt:    0,
			},
			want: mapval(map[string]*pb.Value{
				"NoOmitZeroInt": intval(0),
			}),
		},
		{
			name: "OnlyOmitEmptyStr: non-empty string (included)",
			instance: TestStructForOmitZero{
				OnlyOmitEmptyStr: "text",
				NoOmitZeroInt:    0,
			},
			want: mapval(map[string]*pb.Value{
				"OnlyOmitEmptyStr": strval("text"),
				"NoOmitZeroInt":    intval(0),
			}),
		},
		{
			name: "PtrCustomIsZero: nil pointer",
			instance: TestStructForOmitZero{
				PtrCustomIsZero: nil,
				NoOmitZeroInt:   0,
			},
			want: mapval(map[string]*pb.Value{
				"NoOmitZeroInt": intval(0),
			}),
		},
		{
			name: "PtrCustomIsZero: non-nil, IsZero returns true",
			instance: TestStructForOmitZero{
				PtrCustomIsZero: &StructWithIsZero{ID: 7, Data: "non-zero but IsZero true", isZero: true},
				NoOmitZeroInt:   0,
			},
			want: mapval(map[string]*pb.Value{
				"NoOmitZeroInt": intval(0),
			}),
		},
		{
			name: "PtrCustomIsZero: non-nil, IsZero returns false",
			instance: TestStructForOmitZero{
				PtrCustomIsZero: &StructWithIsZero{ID: 8, Data: "actually not zero", isZero: false},
				NoOmitZeroInt:   0,
			},
			want: mapval(map[string]*pb.Value{
				"PtrCustomIsZero": mapval(map[string]*pb.Value{"ID": intval(8), "Data": strval("actually not zero")}),
				"NoOmitZeroInt":   intval(0),
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := toProtoValue(reflect.ValueOf(tt.instance))
			if err != nil {
				t.Fatalf("toProtoValue() error = %v", err)
			}
			if !testEqual(got, tt.want) {
				t.Errorf("toProtoValue() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToProtoValue_SawTransform(t *testing.T) {
	for i, in := range []interface{}{
		map[string]interface{}{"a": ServerTimestamp},
		map[string]interface{}{"a": ArrayUnion()},
		map[string]interface{}{"a": ArrayRemove()},
	} {
		_, sawTransform, err := toProtoValue(reflect.ValueOf(in))
		if err != nil {
			t.Fatalf("%d %v: got err %v\nexpected nil", i, in, err)
		}
		if !sawTransform {
			t.Errorf("%d %v: got sawTransform=false, expected sawTransform=true", i, in)
		}
	}
}

type testStruct2 struct {
	Ignore        int       `firestore:"-"`
	Rename        int       `firestore:"a"`
	OmitEmpty     int       `firestore:",omitempty"`
	OmitEmptyTime time.Time `firestore:",omitempty"`
}

func TestToProtoValue_Tags(t *testing.T) {
	in := &testStruct2{
		Ignore:        1,
		Rename:        2,
		OmitEmpty:     3,
		OmitEmptyTime: aTime,
	}
	got, _, err := toProtoValue(reflect.ValueOf(in))
	if err != nil {
		t.Fatal(err)
	}
	want := mapval(map[string]*pb.Value{
		"a":             intval(2),
		"OmitEmpty":     intval(3),
		"OmitEmptyTime": tsval(aTime),
	})
	if !testEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	got, _, err = toProtoValue(reflect.ValueOf(testStruct2{}))
	if err != nil {
		t.Fatal(err)
	}
	want = mapval(map[string]*pb.Value{"a": intval(0)})
	if !testEqual(got, want) {
		t.Errorf("got\n%+v\nwant\n%+v", got, want)
	}
}

func TestToProtoValue_Embedded(t *testing.T) {
	// Embedded time.Time, LatLng, or Timestamp should behave like non-embedded.
	type embed struct {
		time.Time
		*latlng.LatLng
		*ts.Timestamp
	}

	got, _, err := toProtoValue(reflect.ValueOf(embed{tm, ll, ptm}))
	if err != nil {
		t.Fatal(err)
	}
	want := mapval(map[string]*pb.Value{
		"Time":      tsval(tm),
		"LatLng":    geoval(ll),
		"Timestamp": {ValueType: &pb.Value_TimestampValue{TimestampValue: ptm}},
	})
	if !testEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestIsEmpty(t *testing.T) {
	for _, e := range []interface{}{int(0), float32(0), false, "", []int{}, []int(nil), (*int)(nil)} {
		if !isEmptyValue(reflect.ValueOf(e)) {
			t.Errorf("%v (%T): want true, got false", e, e)
		}
	}
	i := 3
	for _, n := range []interface{}{int(1), float32(1), true, "x", []int{1}, &i} {
		if isEmptyValue(reflect.ValueOf(n)) {
			t.Errorf("%v (%T): want false, got true", n, n)
		}
	}
}
