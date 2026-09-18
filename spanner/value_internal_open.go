// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !spanner_opaque

package spanner

import (
	"fmt"

	proto3 "google.golang.org/protobuf/types/known/structpb"
)

type internalValue = proto3.Value
type internalListValue = proto3.ListValue

type internalValueKind int

const (
	internalValueUnset internalValueKind = iota
	internalValueNull
	internalValueNumber
	internalValueString
	internalValueBool
	internalValueStruct
	internalValueList
)

func internalValueKindOf(v *internalValue) internalValueKind {
	switch v.GetKind().(type) {
	case *proto3.Value_NullValue:
		return internalValueNull
	case *proto3.Value_NumberValue:
		return internalValueNumber
	case *proto3.Value_StringValue:
		return internalValueString
	case *proto3.Value_BoolValue:
		return internalValueBool
	case *proto3.Value_StructValue:
		return internalValueStruct
	case *proto3.Value_ListValue:
		return internalValueList
	default:
		return internalValueUnset
	}
}

func internalValueKindName(v *internalValue) string {
	if v == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%T", v.Kind)
}
func internalValueIsNull(v *internalValue) bool {
	return internalValueKindOf(v) == internalValueNull
}
func internalValueForError(v *internalValue) string { return fmt.Sprint(v) }

func internalGetStringValue(v *internalValue) (string, bool) {
	x, ok := v.GetKind().(*proto3.Value_StringValue)
	if !ok || x == nil {
		return "", false
	}
	return x.StringValue, true
}
func internalGetBoolValue(v *internalValue) (bool, bool) {
	x, ok := v.GetKind().(*proto3.Value_BoolValue)
	if !ok || x == nil {
		return false, false
	}
	return x.BoolValue, true
}
func internalGetNumberValue(v *internalValue) (float64, bool) {
	x, ok := v.GetKind().(*proto3.Value_NumberValue)
	if !ok || x == nil {
		return 0, false
	}
	return x.NumberValue, true
}
func internalGetListValue(v *internalValue) (*internalListValue, bool) {
	x, ok := v.GetKind().(*proto3.Value_ListValue)
	if !ok || x == nil {
		return nil, false
	}
	return x.ListValue, true
}

func internalListValues(v *internalListValue) []*internalValue {
	if v == nil {
		return nil
	}
	return v.Values
}

func internalSetListValues(v *internalListValue, values []*internalValue) { v.Values = values }
func internalListValueFromValues(values []*internalValue) *internalListValue {
	return &proto3.ListValue{Values: values}
}
func internalNewStringValue(v string) *internalValue {
	return &proto3.Value{Kind: &proto3.Value_StringValue{StringValue: v}}
}
func internalNewListValue(v []*internalValue) *internalValue {
	return &proto3.Value{Kind: &proto3.Value_ListValue{ListValue: &proto3.ListValue{Values: v}}}
}
func internalValueToPublic(v *internalValue) *proto3.Value                   { return v }
func publicValueToInternal(v *proto3.Value) *internalValue                   { return v }
func internalListValueToPublic(v *internalListValue) *proto3.ListValue       { return v }
func internalValuesFromPublic(values []*proto3.Value) []*internalValue       { return values }
func internalListValueFromPublic(value *proto3.ListValue) *internalListValue { return value }
