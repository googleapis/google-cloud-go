// Copyright 2016 Google LLC
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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	pb "cloud.google.com/go/datastore/apiv1/datastorepb"
	"cloud.google.com/go/internal/testutil"
)

func TestInterfaceToProtoNil(t *testing.T) {
	// A nil *Key, or a nil value of any other pointer type, should convert to a NullValue.
	for _, in := range []interface{}{
		(*Key)(nil),
		(*int)(nil),
		(*string)(nil),
		(*bool)(nil),
		(*float64)(nil),
		(*GeoPoint)(nil),
		(*time.Time)(nil),
	} {
		got, err := interfaceToProto(in, false)
		if err != nil {
			t.Fatalf("%T: %v", in, err)
		}
		_, ok := got.ValueType.(*pb.Value_NullValue)
		if !ok {
			t.Errorf("%T: got: %T\nwant: %T", in, got.ValueType, &pb.Value_NullValue{})
		}
	}
}

func TestSaveEntityNested(t *testing.T) {
	type WithKey struct {
		X string
		I int
		K *Key `datastore:"__key__"`
	}

	type NestedWithKey struct {
		Y string
		N WithKey
	}

	type WithoutKey struct {
		X string
		I int
	}

	type NestedWithoutKey struct {
		Y string
		N WithoutKey
	}

	type a struct {
		S string
	}

	type UnexpAnonym struct {
		a
	}

	testCases := []struct {
		desc string
		src  interface{}
		key  *Key
		want *pb.Entity
	}{
		{
			desc: "nested entity with key",
			src: &NestedWithKey{
				Y: "yyy",
				N: WithKey{
					X: "two",
					I: 2,
					K: testKey1a,
				},
			},
			key: testKey0,
			want: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Y": {ValueType: &pb.Value_StringValue{StringValue: "yyy"}},
					"N": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Key: keyToProto(testKey1a),
							Properties: map[string]*pb.Value{
								"X": {ValueType: &pb.Value_StringValue{StringValue: "two"}},
								"I": {ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
							},
						},
					}},
				},
			},
		},
		{
			desc: "nested entity with incomplete key",
			src: &NestedWithKey{
				Y: "yyy",
				N: WithKey{
					X: "two",
					I: 2,
					K: incompleteKey,
				},
			},
			key: testKey0,
			want: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Y": {ValueType: &pb.Value_StringValue{StringValue: "yyy"}},
					"N": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Key: keyToProto(incompleteKey),
							Properties: map[string]*pb.Value{
								"X": {ValueType: &pb.Value_StringValue{StringValue: "two"}},
								"I": {ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
							},
						},
					}},
				},
			},
		},
		{
			desc: "nested entity without key",
			src: &NestedWithoutKey{
				Y: "yyy",
				N: WithoutKey{
					X: "two",
					I: 2,
				},
			},
			key: testKey0,
			want: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Y": {ValueType: &pb.Value_StringValue{StringValue: "yyy"}},
					"N": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"X": {ValueType: &pb.Value_StringValue{StringValue: "two"}},
								"I": {ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
							},
						},
					}},
				},
			},
		},
		{
			desc: "key at top level",
			src: &WithKey{
				X: "three",
				I: 3,
				K: testKey0,
			},
			key: testKey0,
			want: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"X": {ValueType: &pb.Value_StringValue{StringValue: "three"}},
					"I": {ValueType: &pb.Value_IntegerValue{IntegerValue: 3}},
				},
			},
		},
		{
			desc: "nested unexported anonymous struct field",
			src: &UnexpAnonym{
				a{S: "hello"},
			},
			key: testKey0,
			want: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"S": {ValueType: &pb.Value_StringValue{StringValue: "hello"}},
				},
			},
		},
	}

	for _, tc := range testCases {
		got, err := saveEntity(tc.key, tc.src)
		if err != nil {
			t.Errorf("saveEntity: %s: %v", tc.desc, err)
			continue
		}

		if !testutil.Equal(tc.want, got) {
			t.Errorf("%s: compare:\ngot:  %#v\nwant: %#v", tc.desc, got, tc.want)
		}
	}
}

func TestSavePointers(t *testing.T) {
	for _, test := range []struct {
		desc string
		in   interface{}
		want []Property
	}{
		{
			desc: "nil pointers save as nil-valued properties",
			in:   &Pointers{},
			want: []Property{
				{Name: "Pi", Value: nil},
				{Name: "Ps", Value: nil},
				{Name: "Pb", Value: nil},
				{Name: "Pf", Value: nil},
				{Name: "Pg", Value: nil},
				{Name: "Pt", Value: nil},
			},
		},
		{
			desc: "nil omitempty pointers not saved",
			in:   &PointersOmitEmpty{},
			want: []Property(nil),
		},
		{
			desc: "non-nil omitempty zero-valued pointers are saved",
			in:   func() *PointersOmitEmpty { pi := 0; return &PointersOmitEmpty{Pi: &pi} }(),
			want: []Property{{Name: "Pi", Value: int64(0)}},
		},
		{
			desc: "non-nil zero-valued pointers save as zero values",
			in:   populatedPointers(),
			want: []Property{
				{Name: "Pi", Value: int64(0)},
				{Name: "Ps", Value: ""},
				{Name: "Pb", Value: false},
				{Name: "Pf", Value: 0.0},
				{Name: "Pg", Value: GeoPoint{}},
				{Name: "Pt", Value: time.Time{}},
			},
		},
		{
			desc: "non-nil non-zero-valued pointers save as the appropriate values",
			in: func() *Pointers {
				p := populatedPointers()
				*p.Pi = 1
				*p.Ps = "x"
				*p.Pb = true
				*p.Pf = 3.14
				*p.Pg = GeoPoint{Lat: 1, Lng: 2}
				*p.Pt = time.Unix(100, 0)
				return p
			}(),
			want: []Property{
				{Name: "Pi", Value: int64(1)},
				{Name: "Ps", Value: "x"},
				{Name: "Pb", Value: true},
				{Name: "Pf", Value: 3.14},
				{Name: "Pg", Value: GeoPoint{Lat: 1, Lng: 2}},
				{Name: "Pt", Value: time.Unix(100, 0)},
			},
		},
	} {
		got, err := SaveStruct(test.in)
		if err != nil {
			t.Fatalf("%s: %v", test.desc, err)
		}
		if !testutil.Equal(got, test.want) {
			t.Errorf("%s\ngot  %#v\nwant %#v\n", test.desc, got, test.want)
		}
	}
}

func TestSaveEmptySlice(t *testing.T) {
	// Zero-length slice fields are not saved.
	for _, slice := range [][]string{nil, {}} {
		got, err := SaveStruct(&struct{ S []string }{S: slice})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("%#v: got %d properties, wanted zero", slice, len(got))
		}
	}
}

func TestSaveSliceOmitempty(t *testing.T) {

	type S struct {
		G []string `datastore:",noindex,omitempty"`
	}
	testCases := []struct {
		desc string
		in   []string
		want []string // Expected values in the saved property
	}{
		{
			desc: "1st-item-is-empty-string",
			in:   []string{"", "s1", "s2"},
			want: []string{"s1", "s2"},
		},
		{
			desc: "2nd-item-is-empty-string",
			in:   []string{"s0", "", "s2"},
			want: []string{"s0", "s2"},
		},
		{
			desc: "all-empty-strings",
			in:   []string{"", "", ""},
			want: nil, // Should not save the property at all
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := SaveStruct(&S{G: tc.in})
			if err != nil {
				t.Fatalf("SaveStruct failed: %v", err)
			}

			if len(tc.want) == 0 {
				if len(got) != 0 {
					t.Errorf("got %d properties, wanted zero", len(got))
				}
				return
			}

			if len(got) != 1 {
				t.Fatalf("got %d properties, wanted 1", len(got))
			}

			prop := got[0]
			if prop.Name != "G" {
				t.Errorf("got property name %q, want %q", prop.Name, "G")
			}

			gotVals, ok := prop.Value.([]interface{})
			if !ok {
				t.Fatalf("got value type %T, want []interface{}", prop.Value)
			}

			wantVals := make([]interface{}, len(tc.want))
			for i, v := range tc.want {
				wantVals[i] = v
			}

			if !reflect.DeepEqual(gotVals, wantVals) {
				t.Errorf("values do not match:\ngot:  %v\nwant: %v", gotVals, wantVals)
			}
		})
	}
}

// Map is used by TestSaveFieldsWithInterface
// to test a custom type property save.
type Map map[int]int

func (*Map) Load(_ []Property) error {
	return nil
}

func (*Map) Save() ([]Property, error) {
	return []Property{}, nil
}

// Struct is used by TestSaveFieldsWithInterface
// to test a custom type property save.
type Struct struct {
	Map Map
}

func TestSaveFieldsWithInterface(t *testing.T) {
	// We should be able to extract the underlying value behind an interface.
	// See issue https://github.com/googleapis/google-cloud-go/issues/1474.

	type n1 struct {
		Inner interface{}
	}

	type n2 struct {
		Inner2 *n1
	}
	type n3 struct {
		N2 interface{}
	}

	civDateVal := civil.Date{
		Year:  2020,
		Month: 11,
		Day:   10,
	}
	civTimeValNano := civil.Time{
		Hour:       1,
		Minute:     1,
		Second:     1,
		Nanosecond: 1,
	}
	civTimeVal := civil.Time{
		Hour:   1,
		Minute: 1,
		Second: 1,
	}
	timeValNano, _ := time.Parse("15:04:05.000000000", civTimeValNano.String())
	timeVal, _ := time.Parse("15:04:05", civTimeVal.String())
	dateTimeStr := fmt.Sprintf("%v %v", civDateVal.String(), civTimeVal.String())
	dateTimeVal, _ := time.ParseInLocation("2006-01-02 15:04:05", dateTimeStr, time.UTC)

	cases := []struct {
		name string
		in   interface{}
		want []Property
	}{
		{
			name: "Non-Nil value",
			in: &struct {
				Value interface{}
				ID    int
				key   interface{}
			}{
				Value: "this is a string",
				ID:    17,
				key:   "key1",
			},
			want: []Property{
				{Name: "Value", Value: "this is a string"},
				{Name: "ID", Value: int64(17)},
			},
		},
		{
			name: "Nil value",
			in: &struct {
				foo interface{}
			}{
				foo: (*string)(nil),
			},
			want: nil,
		},
		{
			name: "Nil interface",
			in: &struct {
				Value interface{}
				key   interface{}
			}{
				Value: nil,
				key:   "key1",
			},
			want: []Property{
				{Name: "Value", Value: nil},
			},
		},
		{
			name: "Nil map",
			in:   &Struct{},
			want: []Property{
				{Name: "Map", Value: &Entity{Properties: []Property{}}},
			},
		},
		{
			name: "Non-nil map",
			in:   &Struct{Map: Map{1: 2}},
			want: []Property{
				{Name: "Map", Value: &Entity{Properties: []Property{}}},
			},
		},
		{
			name: "civil.Date",
			in: &struct {
				CivDate civil.Date
			}{
				CivDate: civDateVal,
			},
			want: []Property{
				{
					Name:  "CivDate",
					Value: civDateVal.In(time.UTC),
				},
			},
		},
		{
			name: "civil.Time-nano",
			in: &struct {
				CivTimeNano civil.Time
			}{
				CivTimeNano: civTimeValNano,
			},
			want: []Property{
				{
					Name:  "CivTimeNano",
					Value: timeValNano,
				},
			},
		},
		{
			name: "civil.Time",
			in: &struct {
				CivTime civil.Time
			}{
				CivTime: civTimeVal,
			},
			want: []Property{
				{
					Name:  "CivTime",
					Value: timeVal,
				},
			},
		},
		{
			name: "civil.DateTime",
			in: &struct {
				CivDateTime civil.DateTime
			}{
				CivDateTime: civil.DateTime{
					Date: civDateVal,
					Time: civTimeVal,
				},
			},
			want: []Property{
				{
					Name:  "CivDateTime",
					Value: dateTimeVal,
				},
			},
		},
		{
			name: "Nested",
			in: &n3{
				N2: &n2{
					Inner2: &n1{
						Inner: "Innest",
					},
				},
			},
			want: []Property{
				{
					Name: "N2",
					Value: &Entity{
						Properties: []Property{{
							Name: "Inner2",
							Value: &Entity{
								Properties: []Property{{
									Name: "Inner", Value: "Innest",
								}},
							},
						}},
					},
				},
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SaveStruct(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if diff := testutil.Diff(got, tt.want); diff != "" {
				t.Fatalf("got - want +\n%s", diff)
			}
		})
	}
}

// plsMap is used by TestSaveMapPropertyLoadSaver to test saving of custom
// map types that implement PropertyLoadSaver.
type plsMap map[string]string

func (m *plsMap) Load(props []Property) error {
	for _, p := range props {
		if p.Name == "JSON" {
			return json.Unmarshal(p.Value.([]byte), m)
		}
	}
	return nil
}

func (m *plsMap) Save() ([]Property, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return []Property{{Name: "JSON", Value: b, NoIndex: true}}, nil
}

// plsMapErr is used by TestSaveMapPropertyLoadSaver to test Save errors.
type plsMapErr map[string]string

func (m *plsMapErr) Load(_ []Property) error {
	return nil
}

func (m *plsMapErr) Save() ([]Property, error) {
	return nil, errors.New("save failed")
}

func TestSaveMapPropertyLoadSaver(t *testing.T) {
	type mapEnt struct {
		A string `datastore:"a"`
		M plsMap `datastore:"m"`
	}
	type mapEntFlatten struct {
		A string `datastore:"a"`
		M plsMap `datastore:"m,flatten"`
	}
	type mapEntOmit struct {
		A string `datastore:"a"`
		M plsMap `datastore:"m,omitempty"`
	}
	type mapEntSlice struct {
		Ms []plsMap `datastore:"ms"`
	}
	type mapEntErr struct {
		M plsMapErr `datastore:"m"`
	}

	entity := func(json string) *Entity {
		return &Entity{Properties: []Property{{Name: "JSON", Value: []byte(json), NoIndex: true}}}
	}

	cases := []struct {
		name    string
		in      interface{}
		want    []Property
		wantErr string
	}{
		{
			name: "nil map",
			in:   &mapEnt{A: "a"},
			want: []Property{
				{Name: "a", Value: "a"},
				{Name: "m", Value: entity("null")},
			},
		},
		{
			name: "non-nil map",
			in:   &mapEnt{A: "a", M: plsMap{"k": "v"}},
			want: []Property{
				{Name: "a", Value: "a"},
				{Name: "m", Value: entity(`{"k":"v"}`)},
			},
		},
		{
			name: "empty map",
			in:   &mapEnt{A: "a", M: plsMap{}},
			want: []Property{
				{Name: "a", Value: "a"},
				{Name: "m", Value: entity("{}")},
			},
		},
		{
			name: "nil map with flatten",
			in:   &mapEntFlatten{A: "a"},
			want: []Property{
				{Name: "a", Value: "a"},
				{Name: "m.JSON", Value: []byte("null"), NoIndex: true},
			},
		},
		{
			name: "non-nil map with flatten",
			in:   &mapEntFlatten{A: "a", M: plsMap{"k": "v"}},
			want: []Property{
				{Name: "a", Value: "a"},
				{Name: "m.JSON", Value: []byte(`{"k":"v"}`), NoIndex: true},
			},
		},
		{
			name: "nil map with omitempty",
			in:   &mapEntOmit{A: "a"},
			want: []Property{{Name: "a", Value: "a"}},
		},
		{
			name: "empty map with omitempty",
			in:   &mapEntOmit{A: "a", M: plsMap{}},
			want: []Property{{Name: "a", Value: "a"}},
		},
		{
			name: "slice of maps with nil element",
			in:   &mapEntSlice{Ms: []plsMap{{"k": "v"}, nil}},
			want: []Property{
				{Name: "ms", Value: []interface{}{entity(`{"k":"v"}`), entity("null")}},
			},
		},
		{
			name:    "save error on nil map",
			in:      &mapEntErr{},
			wantErr: "field save error: save failed",
		},
		{
			name:    "save error on non-nil map",
			in:      &mapEntErr{M: plsMapErr{"k": "v"}},
			wantErr: "save failed",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SaveStruct(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("got error %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if diff := testutil.Diff(got, tt.want); diff != "" {
				t.Fatalf("got - want +\n%s", diff)
			}
			// The saved properties must be usable by the Put path.
			if _, err := propertiesToProto(testKey0, got, false); err != nil {
				t.Fatalf("propertiesToProto: %v", err)
			}
		})
	}
}

func TestSaveLoadMapPropertyLoadSaver(t *testing.T) {
	type mapEnt struct {
		A string `datastore:"a"`
		M plsMap `datastore:"m"`
	}

	for _, in := range []*mapEnt{
		{A: "a", M: plsMap{"k": "v"}},
		{A: "a"},
	} {
		props, err := SaveStruct(in)
		if err != nil {
			t.Fatal(err)
		}
		var out mapEnt
		if err := LoadStruct(&out, props); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(&out, in) {
			t.Errorf("round trip mismatch: got %+v, want %+v", &out, in)
		}
	}
}

type NestedNoIndexChild struct {
	Value string `datastore:"v"`
}

type NestedNoIndexParent struct {
	B []NestedNoIndexChild `datastore:"bs,noindex"`
}

func (b *NestedNoIndexChild) Load(ps []Property) error {
	return LoadStruct(b, ps)
}

func (b *NestedNoIndexChild) Save() ([]Property, error) {
	return SaveStruct(b)
}

func TestSaveNestedNoIndexPropertyLoadSaver(t *testing.T) {
	m := &NestedNoIndexParent{
		B: []NestedNoIndexChild{
			{Value: strings.Repeat("a", 2000)},
		},
	}
	key := NameKey("A", "test", nil)
	_, err := saveEntity(key, m)
	if err != nil {
		t.Fatalf("saveEntity failed: %v", err)
	}
}
