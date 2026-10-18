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
	"reflect"
	"strings"
	"testing"

	pb "cloud.google.com/go/datastore/apiv1/datastorepb"
	"cloud.google.com/go/internal/testutil"
)

func TestLoadSavePLS(t *testing.T) {
	type testCase struct {
		desc     string
		src      interface{}
		wantSave *pb.Entity
		wantLoad interface{}
		saveErr  string
		loadErr  string
	}

	testCases := []testCase{
		{
			desc: "non-struct implements PLS (top-level)",
			src:  ptrToplsString("hello"),
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"SS": {ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
				},
			},
			wantLoad: ptrToplsString("LOADED"),
		},
		{
			desc: "substructs do implement PLS",
			src:  &aSubPLS{Foo: "foo", Bar: &aPtrPLS{Count: 2}, Baz: aValuePtrPLS{Count: 15}, S: "something"},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Foo": {ValueType: &pb.Value_StringValue{StringValue: "foo"}},
					"Bar": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"Count": {ValueType: &pb.Value_IntegerValue{IntegerValue: 4}},
							},
						},
					}},
					"Baz": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"Count": {ValueType: &pb.Value_IntegerValue{IntegerValue: 12}},
							},
						},
					}},
					"S": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"SS": {ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
							},
						},
					}},
				},
			},
			wantLoad: &aSubPLS{Foo: "foo", Bar: &aPtrPLS{Count: 1}, Baz: aValuePtrPLS{Count: 11}, S: "LOADED"},
		},
		{
			desc: "substruct (ptr) does implement PLS, nil valued substruct",
			src:  &aSubPLS{Foo: "foo", S: "something"},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Foo": {ValueType: &pb.Value_StringValue{StringValue: "foo"}},
					"Baz": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"Count": {ValueType: &pb.Value_IntegerValue{IntegerValue: 12}},
							},
						},
					}},
					"S": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"SS": {ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
							},
						},
					}},
				},
			},
			wantLoad: &aSubPLS{Foo: "foo", Baz: aValuePtrPLS{Count: 11}, S: "LOADED"},
		},
		{
			desc: "substruct (ptr) does not implement PLS",
			src:  &aSubNotPLS{Foo: "foo", Bar: &aNotPLS{Count: 2}},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Foo": {ValueType: &pb.Value_StringValue{StringValue: "foo"}},
					"Bar": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"Count": {ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
							},
						},
					}},
				},
			},
			wantLoad: &aSubNotPLS{Foo: "foo", Bar: &aNotPLS{Count: 2}},
		},
		{
			desc:     "substruct (value) does implement PLS, error on save",
			src:      &aSubPLSErr{Foo: "foo", Bar: aValuePLS{Count: 2}},
			wantSave: (*pb.Entity)(nil),
			wantLoad: &aSubPLSErr{},
			saveErr:  "PropertyLoadSaver methods must be implemented on a pointer",
		},
		{
			desc: "substruct (value) does implement PLS, error on load",
			src:  &aSubPLSNoErr{Foo: "foo", Bar: aPtrPLS{Count: 2}},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Foo": {ValueType: &pb.Value_StringValue{StringValue: "foo"}},
					"Bar": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"Count": {ValueType: &pb.Value_IntegerValue{IntegerValue: 4}},
							},
						},
					}},
				},
			},
			wantLoad: &aSubPLSErr{},
			loadErr:  "PropertyLoadSaver methods must be implemented on a pointer",
		},

		{
			desc: "parent does not have flatten option, child impl PLS",
			src: &Grandparent{
				Parent: Parent{
					Child: Child{
						I: 9,
						Grandchild: Grandchild{
							S: "BAD",
						},
					},
					String: plsString("something"),
				},
			},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Parent": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"Child": {ValueType: &pb.Value_EntityValue{
									EntityValue: &pb.Entity{
										Properties: map[string]*pb.Value{
											"I":            {ValueType: &pb.Value_IntegerValue{IntegerValue: 10}},
											"Grandchild.S": {ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 10"}},
										},
									},
								}},
								"String": {ValueType: &pb.Value_EntityValue{
									EntityValue: &pb.Entity{
										Properties: map[string]*pb.Value{
											"SS": {ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
										},
									},
								}},
							},
						},
					}},
				},
			},
			wantLoad: &Grandparent{
				Parent: Parent{
					Child: Child{
						I: 1,
						Grandchild: Grandchild{
							S: "grandchild loaded",
						},
					},
					String: "LOADED",
				},
			},
		},
		{
			desc: "parent has flatten option enabled, child impl PLS",
			src: &GrandparentFlatten{
				Parent: Parent{
					Child: Child{
						I: 7,
						Grandchild: Grandchild{
							S: "BAD",
						},
					},
					String: plsString("something"),
				},
			},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Parent.Child.I":            {ValueType: &pb.Value_IntegerValue{IntegerValue: 8}},
					"Parent.Child.Grandchild.S": {ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 8"}},
					"Parent.String.SS":          {ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
				},
			},
			wantLoad: &GrandparentFlatten{
				Parent: Parent{
					Child: Child{
						I: 1,
						Grandchild: Grandchild{
							S: "grandchild loaded",
						},
					},
					String: "LOADED",
				},
			},
		},

		{
			desc: "parent has flatten option enabled, child (ptr to) impl PLS",
			src: &GrandparentOfPtrFlatten{
				Parent: ParentOfPtr{
					Child: &Child{
						I: 7,
						Grandchild: Grandchild{
							S: "BAD",
						},
					},
					String: ptrToplsString("something"),
				},
			},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Parent.Child.I":            {ValueType: &pb.Value_IntegerValue{IntegerValue: 8}},
					"Parent.Child.Grandchild.S": {ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 8"}},
					"Parent.String.SS":          {ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
				},
			},
			wantLoad: &GrandparentOfPtrFlatten{
				Parent: ParentOfPtr{
					Child: &Child{
						I: 1,
						Grandchild: Grandchild{
							S: "grandchild loaded",
						},
					},
					String: ptrToplsString("LOADED"),
				},
			},
		},
		{
			desc: "children (slice of) impl PLS",
			src: &GrandparentOfSlice{
				Parent: ParentOfSlice{
					Children: []Child{
						{
							I: 7,
							Grandchild: Grandchild{
								S: "BAD",
							},
						},
						{
							I: 9,
							Grandchild: Grandchild{
								S: "BAD2",
							},
						},
					},
					Strings: []plsString{
						"something1",
						"something2",
					},
				},
			},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Parent": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"Children": {ValueType: &pb.Value_ArrayValue{
									ArrayValue: &pb.ArrayValue{Values: []*pb.Value{
										{ValueType: &pb.Value_EntityValue{
											EntityValue: &pb.Entity{
												Properties: map[string]*pb.Value{
													"I":            {ValueType: &pb.Value_IntegerValue{IntegerValue: 8}},
													"Grandchild.S": {ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 8"}},
												},
											},
										}},
										{ValueType: &pb.Value_EntityValue{
											EntityValue: &pb.Entity{
												Properties: map[string]*pb.Value{
													"I":            {ValueType: &pb.Value_IntegerValue{IntegerValue: 10}},
													"Grandchild.S": {ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 10"}},
												},
											},
										}},
									}},
								}},
								"Strings": {ValueType: &pb.Value_ArrayValue{
									ArrayValue: &pb.ArrayValue{Values: []*pb.Value{
										{ValueType: &pb.Value_EntityValue{
											EntityValue: &pb.Entity{
												Properties: map[string]*pb.Value{
													"SS": {ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
												},
											},
										}},
										{ValueType: &pb.Value_EntityValue{
											EntityValue: &pb.Entity{
												Properties: map[string]*pb.Value{
													"SS": {ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
												},
											},
										}},
									}},
								}},
							},
						},
					}},
				},
			},
			wantLoad: &GrandparentOfSlice{
				Parent: ParentOfSlice{
					Children: []Child{
						{
							I: 1,
							Grandchild: Grandchild{
								S: "grandchild loaded",
							},
						},
						{
							I: 1,
							Grandchild: Grandchild{
								S: "grandchild loaded",
							},
						},
					},
					Strings: []plsString{
						"LOADED",
						"LOADED",
					},
				},
			},
		},
		{
			desc: "children (slice of ptrs) impl PLS",
			src: &GrandparentOfSlicePtrs{
				Parent: ParentOfSlicePtrs{
					Children: []*Child{
						{
							I: 7,
							Grandchild: Grandchild{
								S: "BAD",
							},
						},
						{
							I: 9,
							Grandchild: Grandchild{
								S: "BAD2",
							},
						},
					},
					Strings: []*plsString{
						ptrToplsString("something1"),
						ptrToplsString("something2"),
					},
				},
			},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Parent": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"Children": {ValueType: &pb.Value_ArrayValue{
									ArrayValue: &pb.ArrayValue{Values: []*pb.Value{
										{ValueType: &pb.Value_EntityValue{
											EntityValue: &pb.Entity{
												Properties: map[string]*pb.Value{
													"I":            {ValueType: &pb.Value_IntegerValue{IntegerValue: 8}},
													"Grandchild.S": {ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 8"}},
												},
											},
										}},
										{ValueType: &pb.Value_EntityValue{
											EntityValue: &pb.Entity{
												Properties: map[string]*pb.Value{
													"I":            {ValueType: &pb.Value_IntegerValue{IntegerValue: 10}},
													"Grandchild.S": {ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 10"}},
												},
											},
										}},
									}},
								}},
								"Strings": {ValueType: &pb.Value_ArrayValue{
									ArrayValue: &pb.ArrayValue{Values: []*pb.Value{
										{ValueType: &pb.Value_EntityValue{
											EntityValue: &pb.Entity{
												Properties: map[string]*pb.Value{
													"SS": {ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
												},
											},
										}},
										{ValueType: &pb.Value_EntityValue{
											EntityValue: &pb.Entity{
												Properties: map[string]*pb.Value{
													"SS": {ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
												},
											},
										}},
									}},
								}},
							},
						},
					}},
				},
			},
			wantLoad: &GrandparentOfSlicePtrs{
				Parent: ParentOfSlicePtrs{
					Children: []*Child{
						{
							I: 1,
							Grandchild: Grandchild{
								S: "grandchild loaded",
							},
						},
						{
							I: 1,
							Grandchild: Grandchild{
								S: "grandchild loaded",
							},
						},
					},
					Strings: []*plsString{
						ptrToplsString("LOADED"),
						ptrToplsString("LOADED"),
					},
				},
			},
		},
		{
			desc: "parent has flatten option, children (slice of) impl PLS",
			src: &GrandparentOfSliceFlatten{
				Parent: ParentOfSlice{
					Children: []Child{
						{
							I: 7,
							Grandchild: Grandchild{
								S: "BAD",
							},
						},
						{
							I: 9,
							Grandchild: Grandchild{
								S: "BAD2",
							},
						},
					},
					Strings: []plsString{
						"something1",
						"something2",
					},
				},
			},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Parent.Children.I": {ValueType: &pb.Value_ArrayValue{ArrayValue: &pb.ArrayValue{
						Values: []*pb.Value{
							{ValueType: &pb.Value_IntegerValue{IntegerValue: 8}},
							{ValueType: &pb.Value_IntegerValue{IntegerValue: 10}},
						},
					},
					}},
					"Parent.Children.Grandchild.S": {ValueType: &pb.Value_ArrayValue{ArrayValue: &pb.ArrayValue{
						Values: []*pb.Value{
							{ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 8"}},
							{ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 10"}},
						},
					},
					}},
					"Parent.Strings.SS": {ValueType: &pb.Value_ArrayValue{ArrayValue: &pb.ArrayValue{
						Values: []*pb.Value{
							{ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
							{ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
						},
					},
					}},
				},
			},
			wantLoad: &GrandparentOfSliceFlatten{
				Parent: ParentOfSlice{
					Children: []Child{
						{
							I: 1,
							Grandchild: Grandchild{
								S: "grandchild loaded",
							},
						},
						{
							I: 1,
							Grandchild: Grandchild{
								S: "grandchild loaded",
							},
						},
					},
					Strings: []plsString{
						"LOADED",
						"LOADED",
					},
				},
			},
		},
		{
			desc: "parent has flatten option, children (slice of ptrs) impl PLS",
			src: &GrandparentOfSlicePtrsFlatten{
				Parent: ParentOfSlicePtrs{
					Children: []*Child{
						{
							I: 7,
							Grandchild: Grandchild{
								S: "BAD",
							},
						},
						{
							I: 9,
							Grandchild: Grandchild{
								S: "BAD2",
							},
						},
					},
					Strings: []*plsString{
						ptrToplsString("something1"),
						ptrToplsString("something1"),
					},
				},
			},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Parent.Children.I": {ValueType: &pb.Value_ArrayValue{ArrayValue: &pb.ArrayValue{
						Values: []*pb.Value{
							{ValueType: &pb.Value_IntegerValue{IntegerValue: 8}},
							{ValueType: &pb.Value_IntegerValue{IntegerValue: 10}},
						},
					},
					}},
					"Parent.Children.Grandchild.S": {ValueType: &pb.Value_ArrayValue{ArrayValue: &pb.ArrayValue{
						Values: []*pb.Value{
							{ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 8"}},
							{ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 10"}},
						},
					},
					}},
					"Parent.Strings.SS": {ValueType: &pb.Value_ArrayValue{ArrayValue: &pb.ArrayValue{
						Values: []*pb.Value{
							{ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
							{ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
						},
					},
					}},
				},
			},
			wantLoad: &GrandparentOfSlicePtrsFlatten{
				Parent: ParentOfSlicePtrs{
					Children: []*Child{
						{
							I: 1,
							Grandchild: Grandchild{
								S: "grandchild loaded",
							},
						},
						{
							I: 1,
							Grandchild: Grandchild{
								S: "grandchild loaded",
							},
						},
					},
					Strings: []*plsString{
						ptrToplsString("LOADED"),
						ptrToplsString("LOADED"),
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		e, err := saveEntity(testKey0, tc.src)
		if tc.saveErr == "" { // Want no error.
			if err != nil {
				t.Errorf("%s: save: %v", tc.desc, err)
				continue
			}
			if !testutil.Equal(e, tc.wantSave) {
				t.Errorf("%s: save: \ngot:  %+v\nwant: %+v", tc.desc, e, tc.wantSave)
				continue
			}
		} else { // Want error.
			if err == nil {
				t.Errorf("%s: save: want err", tc.desc)
				continue
			}
			if !strings.Contains(err.Error(), tc.saveErr) {
				t.Errorf("%s: save: \ngot err  '%s'\nwant err '%s'", tc.desc, err.Error(), tc.saveErr)
			}
			continue
		}

		gota := reflect.New(reflect.TypeOf(tc.wantLoad).Elem()).Interface()
		err = loadEntityProto(gota, e)
		if tc.loadErr == "" { // Want no error.
			if err != nil {
				t.Errorf("%s: load: %v", tc.desc, err)
				continue
			}
			if !testutil.Equal(gota, tc.wantLoad) {
				t.Errorf("%s: load: \ngot:  %+v\nwant: %+v", tc.desc, gota, tc.wantLoad)
				continue
			}
		} else { // Want error.
			if err == nil {
				t.Errorf("%s: load: want err", tc.desc)
				continue
			}
			if !strings.Contains(err.Error(), tc.loadErr) {
				t.Errorf("%s: load: \ngot err  '%s'\nwant err '%s'", tc.desc, err.Error(), tc.loadErr)
			}
		}
	}
}

func TestFlattenDoesNotLeakToSibling(t *testing.T) {
	type Reason struct {
		Code string `datastore:"code"`
	}
	type TimeRange struct {
		Reasons []*Reason `datastore:"reasons"`
	}
	type Retail struct {
		ID string `datastore:"id"`
	}
	type Entity struct {
		Retail     Retail       `datastore:"retail,flatten"`
		TimeRanges []*TimeRange `datastore:"timeRanges"`
	}

	props, err := SaveStruct(&Entity{
		Retail: Retail{ID: "example"},
		TimeRanges: []*TimeRange{
			{Reasons: []*Reason{{Code: "UNAVAILABLE"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var sawID, sawRanges bool
	for _, p := range props {
		if p.Name == "retail.id" && p.Value == "example" {
			sawID = true
		}
		if p.Name == "timeRanges" {
			sawRanges = true
		}
	}
	if !sawID || !sawRanges {
		t.Fatalf("saved properties = %#v, want retail.id and timeRanges", props)
	}
}

func TestValidateChildTypeSharedStruct(t *testing.T) {
	type Item struct {
		Tags []string `datastore:"tags"`
	}
	type Group struct {
		Items []Item `datastore:"items"`
	}
	type FlattenedFirst struct {
		Flattened   Group `datastore:"flattened,flatten"`
		Unflattened Group `datastore:"unflattened"`
	}
	type UnflattenedFirst struct {
		Unflattened Group `datastore:"unflattened"`
		Flattened   Group `datastore:"flattened,flatten"`
	}
	type UnflattenedFirstPopulated struct {
		Unflattened Group `datastore:"unflattened"`
		Flattened   Group `datastore:"flattened,flatten"`
	}
	type SingleThenSliceFlatten struct {
		Single   Item   `datastore:"single,flatten"`
		Multiple []Item `datastore:"multiple,flatten"`
	}
	type RecursiveSliceFlatten struct {
		Children []RecursiveSliceFlatten `datastore:"children,flatten"`
	}

	testCases := []struct {
		desc    string
		src     any
		wantErr string
	}{
		{
			desc:    "flattened field before unflattened field sharing type",
			src:     &FlattenedFirst{},
			wantErr: `datastore: flattening nested structs leads to a slice of slices: field "Tags"`,
		},
		{
			desc:    "unflattened field before flattened field sharing type",
			src:     &UnflattenedFirst{},
			wantErr: `datastore: flattening nested structs leads to a slice of slices: field "Tags"`,
		},
		{
			desc: "populated unflattened field before flattened field sharing type",
			src: &UnflattenedFirstPopulated{
				Flattened: Group{Items: []Item{{Tags: []string{"a", "b"}}}},
			},
			wantErr: `datastore: flattening nested structs leads to a slice of slices: field "Tags"`,
		},
		{
			desc:    "single flattened struct before slice of flattened struct sharing type",
			src:     &SingleThenSliceFlatten{},
			wantErr: `datastore: flattening nested structs leads to a slice of slices: field "Tags"`,
		},
		{
			desc:    "recursive slice of flattened struct",
			src:     &RecursiveSliceFlatten{},
			wantErr: `datastore: flattening nested structs leads to a slice of slices: field "Children"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			_, err := SaveStruct(tc.src)
			if err == nil {
				t.Fatalf("SaveStruct(%T) = nil, want error containing %q", tc.src, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("SaveStruct(%T) error = %q, want error containing %q", tc.src, err.Error(), tc.wantErr)
			}
		})
	}
}

func TestValidateChildTypeUnflattenedSliceWithFlattenedSubfield(t *testing.T) {
	type Item struct {
		Tags []string `datastore:"tags"`
	}
	type ChildWithFlat struct {
		Meta Item `datastore:"meta,flatten"`
	}
	type ParentWithSharedChild struct {
		Primary  ChildWithFlat   `datastore:"primary"`
		Children []ChildWithFlat `datastore:"children"`
	}
	type RecursiveNode struct {
		Children []RecursiveNode `datastore:"children"`
		Meta     Item            `datastore:"meta,flatten"`
	}

	testCases := []struct {
		desc string
		src  any
		dst  any
	}{
		{
			desc: "shared child across single and unflattened slice fields",
			src: &ParentWithSharedChild{
				Primary:  ChildWithFlat{Meta: Item{Tags: []string{"a", "b"}}},
				Children: []ChildWithFlat{{Meta: Item{Tags: []string{"c", "d"}}}},
			},
			dst: &ParentWithSharedChild{},
		},
		{
			desc: "recursive node with unflattened slice and flattened subfield",
			src: &RecursiveNode{
				Meta:     Item{Tags: []string{"root"}},
				Children: []RecursiveNode{{Meta: Item{Tags: []string{"child"}}}},
			},
			dst: &RecursiveNode{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			props, err := SaveStruct(tc.src)
			if err != nil {
				t.Fatalf("SaveStruct(%T) = %v, want nil", tc.src, err)
			}
			if err := LoadStruct(tc.dst, props); err != nil {
				t.Fatalf("LoadStruct(%T) = %v, want nil", tc.dst, err)
			}
			if !testutil.Equal(tc.dst, tc.src) {
				t.Errorf("LoadStruct(%T) = %+v, want %+v", tc.dst, tc.dst, tc.src)
			}
		})
	}
}
