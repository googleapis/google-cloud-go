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

//go:build spanner_opaque

package spanner

import (
	"fmt"

	"cloud.google.com/go/spanner/internal/opaquepb"
	proto3 "google.golang.org/protobuf/types/known/structpb"
)

type internalValue = opaquepb.Value
type internalListValue = opaquepb.ListValue

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
	if v == nil {
		return internalValueUnset
	}
	switch v.WhichKind() {
	case opaquepb.Value_NullValue_case:
		return internalValueNull
	case opaquepb.Value_NumberValue_case:
		return internalValueNumber
	case opaquepb.Value_StringValue_case:
		return internalValueString
	case opaquepb.Value_BoolValue_case:
		return internalValueBool
	case opaquepb.Value_StructValue_case:
		return internalValueStruct
	case opaquepb.Value_ListValue_case:
		return internalValueList
	default:
		return internalValueUnset
	}
}

// Opaque Value cannot represent a typed-nil oneof wrapper. Conversion from
// such a public value therefore produces an unset opaque value. Tagged decode
// treats unset as NULL and reports it as <nil> in errors. A list or struct
// wrapper with a nil message payload likewise converts to unset.
func internalValueIsNull(v *internalValue) bool {
	kind := internalValueKindOf(v)
	return kind == internalValueNull || kind == internalValueUnset
}

func internalValueKindName(v *internalValue) string {
	switch internalValueKindOf(v) {
	case internalValueNull:
		return "null"
	case internalValueNumber:
		return "number"
	case internalValueString:
		return "string"
	case internalValueBool:
		return "bool"
	case internalValueStruct:
		return "struct"
	case internalValueList:
		return "list"
	default:
		return "<nil>"
	}
}
func internalValueForError(v *internalValue) string {
	return fmt.Sprint(v)
}

func internalGetStringValue(v *internalValue) (string, bool) {
	return v.GetStringValue(), v != nil && v.WhichKind() == opaquepb.Value_StringValue_case
}
func internalGetBoolValue(v *internalValue) (bool, bool) {
	return v.GetBoolValue(), v != nil && v.WhichKind() == opaquepb.Value_BoolValue_case
}
func internalGetNumberValue(v *internalValue) (float64, bool) {
	return v.GetNumberValue(), v != nil && v.WhichKind() == opaquepb.Value_NumberValue_case
}
func internalGetListValue(v *internalValue) (*internalListValue, bool) {
	if v != nil && v.WhichKind() == opaquepb.Value_ListValue_case {
		return v.GetListValue(), true
	}
	return nil, false
}

func internalListValues(v *internalListValue) []*internalValue {
	if v == nil {
		return nil
	}
	return v.GetValues()
}

func internalSetListValues(v *internalListValue, values []*internalValue) { v.SetValues(values) }
func internalListValueFromValues(values []*internalValue) *internalListValue {
	list := new(opaquepb.ListValue)
	list.SetValues(values)
	return list
}
func internalNewStringValue(v string) *internalValue {
	value := new(opaquepb.Value)
	value.SetStringValue(v)
	return value
}
func internalNewListValue(v []*internalValue) *internalValue {
	list := new(opaquepb.ListValue)
	list.SetValues(v)
	value := new(opaquepb.Value)
	value.SetListValue(list)
	return value
}

// internalValueToPublic performs the compatibility conversion required by
// public APIs that expose structpb.Value. Streaming decode never calls it.
func internalValueToPublic(v *internalValue) *proto3.Value {
	if v == nil {
		return nil
	}
	switch internalValueKindOf(v) {
	case internalValueNull:
		return proto3.NewNullValue()
	case internalValueNumber:
		return proto3.NewNumberValue(v.GetNumberValue())
	case internalValueString:
		return proto3.NewStringValue(v.GetStringValue())
	case internalValueBool:
		return proto3.NewBoolValue(v.GetBoolValue())
	case internalValueStruct:
		value := v.GetStructValue()
		if value == nil {
			return &proto3.Value{Kind: &proto3.Value_StructValue{}}
		}
		fields := make(map[string]*proto3.Value, len(value.GetFields()))
		for name, field := range value.GetFields() {
			fields[name] = internalValueToPublic(field)
		}
		return proto3.NewStructValue(&proto3.Struct{Fields: fields})
	case internalValueList:
		list := v.GetListValue()
		if list == nil {
			return &proto3.Value{Kind: &proto3.Value_ListValue{}}
		}
		values := internalListValues(list)
		converted := make([]*proto3.Value, len(values))
		for i, value := range values {
			converted[i] = internalValueToPublic(value)
		}
		return proto3.NewListValue(&proto3.ListValue{Values: converted})
	default:
		return &proto3.Value{}
	}
}

func internalListValueToPublic(v *internalListValue) *proto3.ListValue {
	if v == nil {
		return nil
	}
	values := internalListValues(v)
	converted := make([]*proto3.Value, len(values))
	for i, value := range values {
		converted[i] = internalValueToPublic(value)
	}
	return &proto3.ListValue{Values: converted}
}

// publicValueToInternal is used only by public constructors and compatibility
// entry points. Values received from Spanner are already opaque.
func publicValueToInternal(v *proto3.Value) *internalValue {
	if v == nil {
		return nil
	}
	dst := new(opaquepb.Value)
	switch kind := v.GetKind().(type) {
	case *proto3.Value_NullValue:
		if kind == nil {
			break
		}
		dst.SetNullValue(opaquepb.NullValue(kind.NullValue))
	case *proto3.Value_NumberValue:
		if kind == nil {
			break
		}
		dst.SetNumberValue(kind.NumberValue)
	case *proto3.Value_StringValue:
		if kind == nil {
			break
		}
		dst.SetStringValue(kind.StringValue)
	case *proto3.Value_BoolValue:
		if kind == nil {
			break
		}
		dst.SetBoolValue(kind.BoolValue)
	case *proto3.Value_StructValue:
		if kind == nil {
			break
		}
		if kind.StructValue == nil {
			break
		}
		fields := make(map[string]*opaquepb.Value, len(kind.StructValue.GetFields()))
		for name, field := range kind.StructValue.GetFields() {
			fields[name] = publicValueToInternal(field)
		}
		value := new(opaquepb.Struct)
		value.SetFields(fields)
		dst.SetStructValue(value)
	case *proto3.Value_ListValue:
		if kind == nil {
			break
		}
		if kind.ListValue == nil {
			break
		}
		values := make([]*opaquepb.Value, len(kind.ListValue.GetValues()))
		for i, value := range kind.ListValue.GetValues() {
			values[i] = publicValueToInternal(value)
		}
		list := new(opaquepb.ListValue)
		list.SetValues(values)
		dst.SetListValue(list)
	}
	return dst
}

func internalValuesFromPublic(values []*proto3.Value) []*internalValue {
	converted := make([]*internalValue, len(values))
	for i, value := range values {
		converted[i] = publicValueToInternal(value)
	}
	return converted
}

func internalListValueFromPublic(value *proto3.ListValue) *internalListValue {
	if value == nil {
		return nil
	}
	return internalListValueFromValues(internalValuesFromPublic(value.GetValues()))
}
