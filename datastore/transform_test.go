// Copyright 2025 Google LLC
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
	"testing"

	pb "cloud.google.com/go/datastore/apiv1/datastorepb"
	"cloud.google.com/go/internal/testutil"
	"google.golang.org/protobuf/proto"
)

func TestIncrement(t *testing.T) {
	testCases := []struct {
		name    string
		value   interface{}
		wantErr bool
		want    *pb.Value
	}{
		{"int", int(5), false, &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 5}}},
		{"int8", int8(5), false, &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 5}}},
		{"int16", int16(5), false, &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 5}}},
		{"int32", int32(5), false, &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 5}}},
		{"int64", int64(5), false, &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 5}}},
		{"uint8", uint8(5), false, &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 5}}},
		{"uint16", uint16(5), false, &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 5}}},
		{"uint32", uint32(5), false, &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 5}}},
		{"float32", float32(5.5), false, &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(float32(5.5))}}},
		{"float64", float64(5.5), false, &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: 5.5}}},
		{"string", "not-a-number", true, nil},
		{"bool", true, true, nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pt := Increment("fieldName", tc.value)
			if (pt.err != nil) != tc.wantErr {
				t.Errorf("Increment() error = %v, wantErr %v", pt.err, tc.wantErr)
				return
			}
			if pt.err == nil {
				got := pt.pb.GetIncrement()
				if !proto.Equal(got, tc.want) {
					t.Errorf("Increment() got = %v, want %v", got, tc.want)
				}
				if pt.pb.GetProperty() != "fieldName" {
					t.Errorf("Increment() propertyName got = %s, want fieldName", pt.pb.GetProperty())
				}
			}
		})
	}
}

func TestSetToServerTime(t *testing.T) {
	pt := SetToServerTime("fieldName")
	if pt.pb.GetProperty() != "fieldName" {
		t.Errorf("SetToServerTime() propertyName got = %s, want fieldName", pt.pb.GetProperty())
	}
	if pt.pb.GetSetToServerValue() != pb.PropertyTransform_REQUEST_TIME {
		t.Errorf("SetToServerTime() value got = %v, want REQUEST_TIME", pt.pb.GetSetToServerValue())
	}
}

func TestMaximum(t *testing.T) {
	testCases := []struct {
		name    string
		value   interface{}
		wantErr bool
		want    *pb.Value
	}{
		{"int", int(-2), false, &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: -2}}},
		{"int64", int64(-2), false, &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: -2}}},
		{"float32", float32(-2.5), false, &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(float32(-2.5))}}},
		{"float64", float64(-2.5), false, &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: -2.5}}},
		{"string", "not-a-number", true, nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pt := Maximum("fieldName", tc.value)
			if (pt.err != nil) != tc.wantErr {
				t.Errorf("Maximum() error = %v, wantErr %v", pt.err, tc.wantErr)
				return
			}
			if pt.err == nil {
				got := pt.pb.GetMaximum()
				if !proto.Equal(got, tc.want) {
					t.Errorf("Maximum() got = %v, want %v", got, tc.want)
				}
				if pt.pb.GetProperty() != "fieldName" {
					t.Errorf("Maximum() propertyName got = %s, want fieldName", pt.pb.GetProperty())
				}
			}
		})
	}
}

func TestMinimum(t *testing.T) {
	testCases := []struct {
		name    string
		value   interface{}
		wantErr bool
		want    *pb.Value
	}{
		{"int", int(100), false, &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 100}}},
		{"int64", int64(100), false, &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 100}}},
		{"float32", float32(100.1), false, &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(float32(100.1))}}},
		{"float64", float64(100.1), false, &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: 100.1}}},
		{"string", "not-a-number", true, nil},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pt := Minimum("fieldName", tc.value)
			if (pt.err != nil) != tc.wantErr {
				t.Errorf("Minimum() error = %v, wantErr %v", pt.err, tc.wantErr)
				return
			}
			if pt.err == nil {
				got := pt.pb.GetMinimum()
				if !proto.Equal(got, tc.want) {
					t.Errorf("Minimum() got = %v, want %v", got, tc.want)
				}
				if pt.pb.GetProperty() != "fieldName" {
					t.Errorf("Minimum() propertyName got = %s, want fieldName", pt.pb.GetProperty())
				}
			}
		})
	}
}

func TestAppendMissingElements(t *testing.T) {
	k := NameKey("Kind", "name", nil)
	testCases := []struct {
		name     string
		elements []interface{}
		wantErr  bool
		want     *pb.ArrayValue
	}{
		{
			name:     "various types",
			elements: []interface{}{int64(1), "two", true, k},
			want: &pb.ArrayValue{Values: []*pb.Value{
				{ValueType: &pb.Value_IntegerValue{IntegerValue: 1}, ExcludeFromIndexes: false},
				{ValueType: &pb.Value_StringValue{StringValue: "two"}, ExcludeFromIndexes: false},
				{ValueType: &pb.Value_BooleanValue{BooleanValue: true}, ExcludeFromIndexes: false},
				{ValueType: &pb.Value_KeyValue{KeyValue: keyToProto(k)}, ExcludeFromIndexes: false},
			}},
		},
		{
			name:     "empty",
			elements: []interface{}{},
			want:     &pb.ArrayValue{Values: []*pb.Value{}},
		},
		{
			name:     "unsupported type",
			elements: []interface{}{make(chan int)},
			wantErr:  true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pt := AppendMissingElements("fieldName", tc.elements...)
			if (pt.err != nil) != tc.wantErr {
				t.Errorf("AppendMissingElements() error = %v, wantErr %v", pt.err, tc.wantErr)
				return
			}
			if pt.err == nil {
				got := pt.pb.GetAppendMissingElements()
				if !testutil.Equal(got, tc.want) {
					t.Errorf("AppendMissingElements() got = %v, want %v", got, tc.want)
				}
				if pt.pb.GetProperty() != "fieldName" {
					t.Errorf("AppendMissingElements() propertyName got = %s, want fieldName", pt.pb.GetProperty())
				}
			}
		})
	}
}

func TestRemoveAllFromArray(t *testing.T) {
	k := IDKey("Kind", 123, nil)
	testCases := []struct {
		name     string
		elements []interface{}
		wantErr  bool
		want     *pb.ArrayValue
	}{
		{
			name:     "various types",
			elements: []interface{}{int64(10), "twenty", false, k},
			want: &pb.ArrayValue{Values: []*pb.Value{
				{ValueType: &pb.Value_IntegerValue{IntegerValue: 10}, ExcludeFromIndexes: false},
				{ValueType: &pb.Value_StringValue{StringValue: "twenty"}, ExcludeFromIndexes: false},
				{ValueType: &pb.Value_BooleanValue{BooleanValue: false}, ExcludeFromIndexes: false},
				{ValueType: &pb.Value_KeyValue{KeyValue: keyToProto(k)}, ExcludeFromIndexes: false},
			}},
		},
		{
			name:     "empty",
			elements: []interface{}{},
			want:     &pb.ArrayValue{Values: []*pb.Value{}},
		},
		{
			name:     "unsupported type",
			elements: []interface{}{reflect.ValueOf(nil)},
			wantErr:  true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pt := RemoveAllFromArray("fieldName", tc.elements...)
			if (pt.err != nil) != tc.wantErr {
				t.Errorf("RemoveAllFromArray() error = %v, wantErr %v", pt.err, tc.wantErr)
				return
			}
			if pt.err == nil {
				got := pt.pb.GetRemoveAllFromArray()
				if !testutil.Equal(got, tc.want) {
					t.Errorf("RemoveAllFromArray() got = %v, want %v", got, tc.want)
				}
				if pt.pb.GetProperty() != "fieldName" {
					t.Errorf("RemoveAllFromArray() propertyName got = %s, want fieldName", pt.pb.GetProperty())
				}
			}
		})
	}
}
