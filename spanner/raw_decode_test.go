/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"encoding/base64"
	"math"
	"math/big"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/uuid"
	"google.golang.org/grpc/mem"
	"google.golang.org/protobuf/proto"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

func TestUnmarshalRawPartialResultSetLeavesValuesOnWire(t *testing.T) {
	want := &sppb.PartialResultSet{
		Metadata: &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
			{Name: "s", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
			{Name: "i", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
		}}},
		Values: []*structpb.Value{
			structpb.NewStringValue("value"),
			structpb.NewStringValue("42"),
			structpb.NewBoolValue(true),
			structpb.NewNumberValue(1.25),
			structpb.NewNullValue(),
		},
		ChunkedValue: true,
		ResumeToken:  []byte("resume"),
		Last:         true,
	}
	wire, err := proto.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}

	got := new(sppb.PartialResultSet)
	rawValues, err := unmarshalRawPartialResultSet(wire, got, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Values) != 0 {
		t.Fatalf("decoded %d structpb values, want none", len(got.Values))
	}
	wantEnvelope := proto.Clone(want).(*sppb.PartialResultSet)
	wantEnvelope.Values = nil
	if !proto.Equal(got, wantEnvelope) {
		t.Fatalf("envelope mismatch\n got: %v\nwant: %v", got, wantEnvelope)
	}
	if len(rawValues) != len(want.Values) {
		t.Fatalf("raw value count = %d, want %d", len(rawValues), len(want.Values))
	}
	for i, raw := range rawValues {
		decoded := new(structpb.Value)
		if err := proto.Unmarshal(raw, decoded); err != nil {
			t.Fatalf("value %d: %v", i, err)
		}
		if !proto.Equal(decoded, want.Values[i]) {
			t.Fatalf("value %d = %v, want %v", i, decoded, want.Values[i])
		}
	}
}

func TestDecodeRawValueMatchesDecodeValueForScalarTypes(t *testing.T) {
	protoBytes, err := proto.Marshal(&sppb.Type{Code: sppb.TypeCode_STRING})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		typeCode    sppb.TypeCode
		value       *structpb.Value
		newDst      func() any
		nullDst     func() any
		nullAllowed bool
	}{
		{"string", sppb.TypeCode_STRING, structpb.NewStringValue("hello"), func() any { return new(string) }, func() any { return new(NullString) }, true},
		{"bytes", sppb.TypeCode_BYTES, structpb.NewStringValue(base64.StdEncoding.EncodeToString([]byte("bytes"))), func() any { return new([]byte) }, func() any { return new([]byte) }, true},
		{"int64", sppb.TypeCode_INT64, structpb.NewStringValue("-9223372036854775808"), func() any { return new(int64) }, func() any { return new(NullInt64) }, true},
		{"enum", sppb.TypeCode_ENUM, structpb.NewStringValue("7"), func() any { return new(int64) }, func() any { return new(NullInt64) }, true},
		{"bool", sppb.TypeCode_BOOL, structpb.NewBoolValue(true), func() any { return new(bool) }, func() any { return new(NullBool) }, true},
		{"float64", sppb.TypeCode_FLOAT64, structpb.NewNumberValue(1.25), func() any { return new(float64) }, func() any { return new(NullFloat64) }, true},
		{"float32", sppb.TypeCode_FLOAT32, structpb.NewNumberValue(2.5), func() any { return new(float32) }, func() any { return new(NullFloat32) }, true},
		{"numeric", sppb.TypeCode_NUMERIC, structpb.NewStringValue("123.456"), func() any { return new(big.Rat) }, func() any { return new(NullNumeric) }, true},
		{"timestamp", sppb.TypeCode_TIMESTAMP, structpb.NewStringValue("2024-01-02T03:04:05.123456789Z"), func() any { return new(time.Time) }, func() any { return new(NullTime) }, true},
		{"date", sppb.TypeCode_DATE, structpb.NewStringValue("2024-01-02"), func() any { return new(civil.Date) }, func() any { return new(NullDate) }, true},
		{"json", sppb.TypeCode_JSON, structpb.NewStringValue(`{"key":"value"}`), func() any { return new(NullJSON) }, func() any { return new(NullJSON) }, true},
		{"uuid", sppb.TypeCode_UUID, structpb.NewStringValue("d4c3b2a1-1111-2222-3333-444455556666"), func() any { return new(uuid.UUID) }, func() any { return new(NullUUID) }, true},
		{"interval", sppb.TypeCode_INTERVAL, structpb.NewStringValue("P1Y2M3DT4H5M6S"), func() any { return new(Interval) }, func() any { return new(NullInterval) }, true},
		{"proto", sppb.TypeCode_PROTO, structpb.NewStringValue(base64.StdEncoding.EncodeToString(protoBytes)), func() any { return new([]byte) }, func() any { return new([]byte) }, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			typ := &sppb.Type{Code: test.typeCode}
			assertRawDecodeMatches(t, test.value, typ, test.newDst)
			if test.nullAllowed {
				assertRawDecodeMatches(t, structpb.NewNullValue(), typ, test.nullDst)
			}
		})
	}
}

func TestDecodeRawValueNestedFallback(t *testing.T) {
	value := structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
		structpb.NewStringValue("one"),
		structpb.NewNullValue(),
	}})
	typ := &sppb.Type{Code: sppb.TypeCode_ARRAY, ArrayElementType: &sppb.Type{Code: sppb.TypeCode_STRING}}
	assertRawDecodeMatches(t, value, typ, func() any { return new([]NullString) })
}

func TestDecodeRawValueStructFallback(t *testing.T) {
	value := structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
		structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
			structpb.NewStringValue("one"),
		}}),
		structpb.NewNullValue(),
	}})
	typ := &sppb.Type{Code: sppb.TypeCode_ARRAY, ArrayElementType: &sppb.Type{
		Code: sppb.TypeCode_STRUCT,
		StructType: &sppb.StructType{Fields: []*sppb.StructType_Field{
			{Name: "value", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
		}},
	}}
	raw, err := proto.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var got []NullRow
	if err := decodeRawValue(raw, typ, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[0].Valid || got[1].Valid {
		t.Fatalf("decoded struct array validity = %#v", got)
	}
	var decoded string
	if err := got[0].Row.Column(0, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != "one" {
		t.Fatalf("decoded struct value = %q, want one", decoded)
	}
}

func TestDecodeRawValuePostgreSQLTypes(t *testing.T) {
	for _, test := range []struct {
		name   string
		value  *structpb.Value
		typ    *sppb.Type
		newDst func() any
	}{
		{"numeric", structpb.NewStringValue("123.456"), pgNumericType(), func() any { return new(PGNumeric) }},
		{"numeric null", structpb.NewNullValue(), pgNumericType(), func() any { return new(PGNumeric) }},
		{"jsonb", structpb.NewStringValue(`{"key":"value"}`), pgJsonbType(), func() any { return new(PGJsonB) }},
		{"jsonb null", structpb.NewNullValue(), pgJsonbType(), func() any { return new(PGJsonB) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertRawDecodeMatches(t, test.value, test.typ, test.newDst)
		})
	}
}

func TestDecodeRawValueSpecialFloats(t *testing.T) {
	for _, test := range []struct {
		name string
		wire string
		want float64
	}{
		{"nan", "NaN", math.NaN()},
		{"positive infinity", "Infinity", math.Inf(1)},
		{"negative infinity", "-Infinity", math.Inf(-1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, value := range []*structpb.Value{
				structpb.NewNumberValue(test.want),
				structpb.NewStringValue(test.wire),
			} {
				raw, err := proto.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				var got64 float64
				if err := decodeRawValue(raw, &sppb.Type{Code: sppb.TypeCode_FLOAT64}, &got64); err != nil {
					t.Fatal(err)
				}
				if math.IsNaN(test.want) {
					if !math.IsNaN(got64) {
						t.Fatalf("float64 = %v, want NaN", got64)
					}
				} else if got64 != test.want {
					t.Fatalf("float64 = %v, want %v", got64, test.want)
				}

				var got32 float32
				if err := decodeRawValue(raw, &sppb.Type{Code: sppb.TypeCode_FLOAT32}, &got32); err != nil {
					t.Fatal(err)
				}
				if math.IsNaN(test.want) {
					if !math.IsNaN(float64(got32)) {
						t.Fatalf("float32 = %v, want NaN", got32)
					}
				} else if math.IsInf(test.want, 1) && !math.IsInf(float64(got32), 1) {
					t.Fatalf("float32 = %v, want +Inf", got32)
				} else if math.IsInf(test.want, -1) && !math.IsInf(float64(got32), -1) {
					t.Fatalf("float32 = %v, want -Inf", got32)
				}
			}
		})
	}
}

func TestDecodeRawValueGenericFallback(t *testing.T) {
	value := structpb.NewStringValue("generic")
	typ := &sppb.Type{Code: sppb.TypeCode_STRING}
	raw, err := proto.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var got GenericColumnValue
	if err := decodeRawValue(raw, typ, &got); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(got.Type, typ) || !proto.Equal(got.Value, value) {
		t.Fatalf("decoded generic value = %#v", got)
	}
}

func TestDecodeRawValueProtoMessageFallback(t *testing.T) {
	want := &sppb.Type{Code: sppb.TypeCode_UUID}
	wire, err := proto.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	value := structpb.NewStringValue(base64.StdEncoding.EncodeToString(wire))
	raw, err := proto.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	got := new(sppb.Type)
	if err := decodeRawValue(raw, &sppb.Type{Code: sppb.TypeCode_PROTO}, got); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(got, want) {
		t.Fatalf("decoded proto = %v, want %v", got, want)
	}
}

func TestDecodeRawValueNullErrorsMatch(t *testing.T) {
	null := structpb.NewNullValue()
	tests := []struct {
		name   string
		typ    *sppb.Type
		newDst func() any
	}{
		{"typed nil", &sppb.Type{Code: sppb.TypeCode_STRING}, func() any { return (*string)(nil) }},
		{"type mismatch", &sppb.Type{Code: sppb.TypeCode_STRING}, func() any { return new(NullInt64) }},
		{"pg numeric annotation", &sppb.Type{Code: sppb.TypeCode_NUMERIC}, func() any { return new(PGNumeric) }},
		{"pg json annotation", &sppb.Type{Code: sppb.TypeCode_JSON}, func() any { return new(PGJsonB) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertRawDecodeMatches(t, null, test.typ, test.newDst)
		})
	}
}

func assertRawDecodeMatches(t *testing.T, value *structpb.Value, typ *sppb.Type, newDst func() any) {
	t.Helper()
	raw, err := proto.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	want := newDst()
	wantErr := decodeValue(value, typ, want)
	got := newDst()
	gotErr := decodeRawValue(raw, typ, got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded value = %#v, want %#v", got, want)
	}
	if (gotErr == nil) != (wantErr == nil) || gotErr != nil && gotErr.Error() != wantErr.Error() {
		t.Fatalf("decoded error = %v, want %v", gotErr, wantErr)
	}
}

func TestRawChunkedValueAcrossReceiveBuffers(t *testing.T) {
	pool := new(countingBufferPool)
	left := strings.Repeat("a", 1200)
	right := strings.Repeat("b", 1200)
	metadata := &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
		{Name: "value", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
	}}}
	firstPRS, firstBuffer := rawTestBuffer(t, pool, &sppb.PartialResultSet{
		Metadata:     metadata,
		Values:       []*structpb.Value{structpb.NewStringValue(left)},
		ChunkedValue: true,
	})
	secondPRS, secondBuffer := rawTestBuffer(t, pool, &sppb.PartialResultSet{
		Values: []*structpb.Value{structpb.NewStringValue(right)},
		Last:   true,
	})

	decoder := new(partialResultSetDecoder)
	row, _, err := decoder.addReusable(firstPRS, firstBuffer)
	if err != nil {
		t.Fatal(err)
	}
	if row != nil {
		t.Fatal("first chunk yielded a row")
	}
	row, _, err = decoder.addReusable(secondPRS, secondBuffer)
	if err != nil {
		t.Fatal(err)
	}
	if row == nil {
		t.Fatal("second chunk did not yield a row")
	}
	firstPRS, secondPRS = nil, nil
	if got := pool.puts.Load(); got != 0 {
		t.Fatalf("buffers freed while row is live: %d", got)
	}
	var got string
	if err := row.Column(0, &got); err != nil {
		t.Fatal(err)
	}
	if want := left + right; got != want {
		t.Fatalf("chunked value length/content mismatch: got %d bytes", len(got))
	}
	releaseRawRow(row)
	if got := pool.puts.Load(); got != 2 {
		t.Fatalf("freed buffers = %d, want 2", got)
	}
}

func TestMergeRawValuesNestedListFallback(t *testing.T) {
	left := structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
		structpb.NewStringValue("a"),
		structpb.NewStringValue("b"),
	}})
	right := structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
		structpb.NewStringValue("c"),
		structpb.NewStringValue("d"),
	}})
	want := structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
		structpb.NewStringValue("a"),
		structpb.NewStringValue("bc"),
		structpb.NewStringValue("d"),
	}})
	leftWire, err := proto.Marshal(left)
	if err != nil {
		t.Fatal(err)
	}
	rightWire, err := proto.Marshal(right)
	if err != nil {
		t.Fatal(err)
	}
	mergedWire, err := mergeRawValues(leftWire, rightWire)
	if err != nil {
		t.Fatal(err)
	}
	got, err := materializeRawValue(mergedWire)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(got, want) {
		t.Fatalf("merged value = %v, want %v", got, want)
	}
}

func TestRawRowOutlivesPartialResultSet(t *testing.T) {
	pool := new(countingBufferPool)
	want := strings.Repeat("row-owned", 200)
	prs, buffer := rawTestBuffer(t, pool, &sppb.PartialResultSet{
		Metadata: &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
			{Name: "value", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
		}}},
		Values: []*structpb.Value{structpb.NewStringValue(want)},
	})
	decoder := new(partialResultSetDecoder)
	row, _, err := decoder.addReusable(prs, buffer)
	if err != nil {
		t.Fatal(err)
	}
	prs = nil
	if got := pool.puts.Load(); got != 0 {
		t.Fatalf("buffer freed with live row: %d", got)
	}
	var got string
	if err := row.Column(0, &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("row value mismatch: got %q, want %q", got, want)
	}
	releaseRawRow(row)
	if got := pool.puts.Load(); got != 1 {
		t.Fatalf("freed buffers = %d, want 1", got)
	}
	if got != want {
		t.Fatalf("decoded string changed after buffer release: got %q, want %q", got, want)
	}
}

func TestRawRowIteratorStopReleasesLiveBuffersOnce(t *testing.T) {
	pool := new(countingBufferPool)
	firstPRS, firstBuffer := rawTestBuffer(t, pool, &sppb.PartialResultSet{
		Metadata: &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
			{Name: "value", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
		}}},
		Values: []*structpb.Value{
			structpb.NewStringValue(strings.Repeat("live", 400)),
			structpb.NewStringValue(strings.Repeat("queued", 400)),
		},
	})
	decoder := new(partialResultSetDecoder)
	rows, _, err := decoder.add(firstPRS, firstBuffer)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("completed rows = %d, want 2", len(rows))
	}
	releaseVTUnsafeBuffer(firstBuffer)

	secondPRS, secondBuffer := rawTestBuffer(t, pool, &sppb.PartialResultSet{
		Values:       []*structpb.Value{structpb.NewStringValue(strings.Repeat("pending", 400))},
		ChunkedValue: true,
	})
	pendingRows, _, err := decoder.add(secondPRS, secondBuffer)
	if err != nil {
		t.Fatal(err)
	}
	if len(pendingRows) != 0 {
		t.Fatalf("pending chunk yielded %d rows, want none", len(pendingRows))
	}
	releaseVTUnsafeBuffer(secondBuffer)
	if got := pool.puts.Load(); got != 0 {
		t.Fatalf("buffers freed before Stop = %d, want 0", got)
	}

	iter := &RowIterator{rowd: decoder, lastRawRow: rows[0], rows: rows[1:]}
	iter.Stop()
	iter.Stop()
	if got := pool.puts.Load(); got != 2 {
		t.Fatalf("freed buffers after Stop twice = %d, want 2", got)
	}
}

type countingBufferPool struct {
	puts atomic.Int32
}

func (*countingBufferPool) Get(length int) *[]byte {
	b := make([]byte, length)
	return &b
}

func (p *countingBufferPool) Put(buffer *[]byte) {
	for i := range *buffer {
		(*buffer)[i] = 0xff
	}
	p.puts.Add(1)
}

func rawTestBuffer(t *testing.T, pool mem.BufferPool, source *sppb.PartialResultSet) (*sppb.PartialResultSet, *vtUnsafeBuffer) {
	t.Helper()
	wire, err := proto.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) <= 1024 {
		t.Fatalf("test wire length = %d; must exceed pooling threshold", len(wire))
	}
	buf := mem.Copy(wire, pool)
	prs := new(sppb.PartialResultSet)
	rawValues, err := unmarshalRawPartialResultSet(buf.ReadOnlyData(), prs, true)
	if err != nil {
		buf.Free()
		t.Fatal(err)
	}
	retained := newVTUnsafeBuffer(buf, nil)
	retained.rawValues = rawValues
	retained.raw = true
	return prs, retained
}
