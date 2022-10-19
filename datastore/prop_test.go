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

	"cloud.google.com/go/internal/testutil"
	pb "google.golang.org/genproto/googleapis/datastore/v1"
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
