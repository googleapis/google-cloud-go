// Copyright 2021 Google LLC
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

package internal

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestToProtoStruct(t *testing.T) {
	v := struct {
		Foo string                 `json:"foo"`
		Bar int                    `json:"bar,omitempty"`
		Baz []float64              `json:"baz"`
		Moo map[string]interface{} `json:"moo"`
	}{
		Foo: "foovalue",
		Baz: []float64{1.1},
		Moo: map[string]interface{}{
			"a": 1,
			"b": "two",
			"c": true,
		},
	}

	got, err := ToProtoStruct(v)
	if err != nil {
		t.Fatal(err)
	}
	want := &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"foo": {Kind: &structpb.Value_StringValue{StringValue: v.Foo}},
			"baz": {Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: []*structpb.Value{
				{Kind: &structpb.Value_NumberValue{NumberValue: 1.1}},
			}}}},
			"moo": {Kind: &structpb.Value_StructValue{
				StructValue: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"a": {Kind: &structpb.Value_NumberValue{NumberValue: 1}},
						"b": {Kind: &structpb.Value_StringValue{StringValue: "two"}},
						"c": {Kind: &structpb.Value_BoolValue{BoolValue: true}},
					},
				},
			}},
		},
	}
	if !proto.Equal(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}

	// Non-structs should fail to convert.
	for v := range []interface{}{3, "foo", []int{1, 2, 3}} {
		_, err := ToProtoStruct(v)
		if err == nil {
			t.Errorf("%v: got nil, want error", v)
		}
	}

	// Test fast path.
	got, err = ToProtoStruct(want)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Error("got and want should be identical, but are not")
	}
}
