// Copyright 2026 Google LLC
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

package spanner

import (
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	proto3 "google.golang.org/protobuf/types/known/structpb"
)

func TestDecodeStructArrayNilStructType(t *testing.T) {
	for i, test := range []struct {
		in      *proto3.ListValue
		wantErr error
	}{
		{
			// Empty ARRAY<STRUCT> with nil StructType.
			in:      listProto().GetListValue(),
			wantErr: errNilSpannerStructType(),
		},
		{
			// ARRAY<STRUCT> with NULL element and nil StructType.
			in:      listProto(nullProto()).GetListValue(),
			wantErr: errNilSpannerStructType(),
		},
		{
			// ARRAY<STRUCT> with non-null struct element and nil StructType.
			in:      listProto(listProto(intProto(1), floatProto(2.0), stringProto("3"))).GetListValue(),
			wantErr: errDecodeArrayElement(0, listProto(intProto(1), floatProto(2.0), stringProto("3")), "STRUCT", errNilSpannerStructType()),
		},
	} {
		var got []*struct {
			Col1 int64
			Col2 float64
			Col3 string
		}
		err := decodeStructArray(nil, test.in, &got, false)
		if !testEqual(err, test.wantErr) {
			t.Errorf("#%d: got error %v, want %v", i, err, test.wantErr)
		}
	}
}

func TestDecodeValueStructArrayNilStructType(t *testing.T) {
	for i, test := range []struct {
		in      *proto3.Value
		wantErr error
	}{
		{
			// Empty ARRAY<STRUCT> with nil StructType.
			in:      listProto(),
			wantErr: errNilSpannerStructType(),
		},
		{
			// ARRAY<STRUCT> with NULL element and nil StructType.
			in:      listProto(nullProto()),
			wantErr: errNilSpannerStructType(),
		},
		{
			// ARRAY<STRUCT> with non-null struct element and nil StructType.
			in:      listProto(listProto(intProto(1), floatProto(2.0), stringProto("3"))),
			wantErr: errDecodeArrayElement(0, listProto(intProto(1), floatProto(2.0), stringProto("3")), "STRUCT", errNilSpannerStructType()),
		},
	} {
		var got []*struct {
			Col1 int64
			Col2 float64
			Col3 string
		}
		err := decodeValue(test.in, listType(&sppb.Type{Code: sppb.TypeCode_STRUCT}), &got)
		if !testEqual(err, test.wantErr) {
			t.Errorf("#%d: got error %v, want %v", i, err, test.wantErr)
		}
	}
}
