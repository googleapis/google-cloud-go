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
	"runtime"
	"sync"
	"sync/atomic"
	"weak"

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
		return internalMalformedKind(v)
	}
}

type malformedValueState struct {
	kind     internalValueKind
	typedNil bool
}

// malformedValues preserves open oneof states that opaque messages cannot
// represent (for example, a typed-nil oneof wrapper). Valid protobuf wire
// values never enter this registry. Finalizers keep its lifetime bounded by
// the compatibility value that owns the state.
var malformedValues sync.Map
var malformedValueCount atomic.Int64

func internalMalformedState(v *internalValue) (malformedValueState, bool) {
	if v == nil || malformedValueCount.Load() == 0 {
		return malformedValueState{}, false
	}
	state, ok := malformedValues.Load(weak.Make(v))
	if !ok {
		return malformedValueState{}, false
	}
	return state.(malformedValueState), true
}

func internalMalformedKind(v *internalValue) internalValueKind {
	if state, ok := internalMalformedState(v); ok {
		return state.kind
	}
	return internalValueUnset
}

func internalMalformedTypedNil(v *internalValue) bool {
	state, ok := internalMalformedState(v)
	return ok && state.typedNil
}

func internalSetMalformedKind(v *internalValue, kind internalValueKind, typedNil bool) {
	malformedValues.Range(func(key, _ interface{}) bool {
		if key.(weak.Pointer[opaquepb.Value]).Value() == nil {
			if _, loaded := malformedValues.LoadAndDelete(key); loaded {
				malformedValueCount.Add(-1)
			}
		}
		return true
	})
	key := weak.Make(v)
	malformedValues.Store(key, malformedValueState{kind: kind, typedNil: typedNil})
	malformedValueCount.Add(1)
	runtime.SetFinalizer(v, func(*opaquepb.Value) {
		if _, loaded := malformedValues.LoadAndDelete(key); loaded {
			malformedValueCount.Add(-1)
		}
	})
}

func internalValueKindName(v *internalValue) string {
	switch internalValueKindOf(v) {
	case internalValueNull:
		return "*structpb.Value_NullValue"
	case internalValueNumber:
		return "*structpb.Value_NumberValue"
	case internalValueString:
		return "*structpb.Value_StringValue"
	case internalValueBool:
		return "*structpb.Value_BoolValue"
	case internalValueStruct:
		return "*structpb.Value_StructValue"
	case internalValueList:
		return "*structpb.Value_ListValue"
	default:
		return "<nil>"
	}
}
func internalValueForError(v *internalValue) string {
	if internalMalformedKind(v) != internalValueUnset {
		return ""
	}
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
	if internalMalformedKind(v) == internalValueList {
		return nil, !internalMalformedTypedNil(v)
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
	if kind := internalMalformedKind(v); kind != internalValueUnset {
		switch kind {
		case internalValueNull:
			return &proto3.Value{Kind: (*proto3.Value_NullValue)(nil)}
		case internalValueNumber:
			return &proto3.Value{Kind: (*proto3.Value_NumberValue)(nil)}
		case internalValueString:
			return &proto3.Value{Kind: (*proto3.Value_StringValue)(nil)}
		case internalValueBool:
			return &proto3.Value{Kind: (*proto3.Value_BoolValue)(nil)}
		case internalValueStruct:
			if internalMalformedTypedNil(v) {
				return &proto3.Value{Kind: (*proto3.Value_StructValue)(nil)}
			}
			return &proto3.Value{Kind: &proto3.Value_StructValue{}}
		case internalValueList:
			if internalMalformedTypedNil(v) {
				return &proto3.Value{Kind: (*proto3.Value_ListValue)(nil)}
			}
			return &proto3.Value{Kind: &proto3.Value_ListValue{}}
		}
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
			internalSetMalformedKind(dst, internalValueNull, true)
			break
		}
		dst.SetNullValue(opaquepb.NullValue(kind.NullValue))
	case *proto3.Value_NumberValue:
		if kind == nil {
			internalSetMalformedKind(dst, internalValueNumber, true)
			break
		}
		dst.SetNumberValue(kind.NumberValue)
	case *proto3.Value_StringValue:
		if kind == nil {
			internalSetMalformedKind(dst, internalValueString, true)
			break
		}
		dst.SetStringValue(kind.StringValue)
	case *proto3.Value_BoolValue:
		if kind == nil {
			internalSetMalformedKind(dst, internalValueBool, true)
			break
		}
		dst.SetBoolValue(kind.BoolValue)
	case *proto3.Value_StructValue:
		if kind == nil {
			internalSetMalformedKind(dst, internalValueStruct, true)
			break
		}
		if kind.StructValue == nil {
			internalSetMalformedKind(dst, internalValueStruct, false)
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
			internalSetMalformedKind(dst, internalValueList, true)
			break
		}
		if kind.ListValue == nil {
			internalSetMalformedKind(dst, internalValueList, false)
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
