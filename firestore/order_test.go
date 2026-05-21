// Copyright 2018 Google LLC
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

package firestore

import (
	"math"
	"testing"
	"time"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"google.golang.org/genproto/googleapis/type/latlng"
)

func TestCompareValues(t *testing.T) {
	// Ordered list of values.
	vals := []*pb.Value{
		nullValue,
		bsonMinKey(), // BSON MinKey
		boolval(false),
		boolval(true),
		floatval(math.NaN()),
		floatval(math.Inf(-1)),
		floatval(-math.MaxFloat64),
		int64val(math.MinInt64),
		bsonInt32val(-2), // BSON Int32
		floatval(-1.1),
		intval(-1),
		intval(0),
		floatval(math.SmallestNonzeroFloat64),
		bsonDecimal128val("0.5"), // BSON Decimal128
		intval(1),
		floatval(1.1),
		intval(2),
		int64val(math.MaxInt64),
		floatval(math.MaxFloat64),
		floatval(math.Inf(1)),
		tsval(time.Date(2016, 5, 20, 10, 20, 0, 0, time.UTC)),
		tsval(time.Date(2016, 10, 21, 15, 32, 0, 0, time.UTC)),
		bsonTimestampval(1477063920, 0), // BSON Timestamp (2016-10-21 15:32:00 UTC)
		strval(""),
		strval("\u0000\ud7ff\ue000\uffff"),
		strval("(╯°□°）╯︵ ┻━┻"),
		strval("a"),
		strval("abc def"),
		strval("e\u0301b"),
		strval("æ"),
		strval("\u00e9a"),
		bytesval([]byte{}),
		bytesval([]byte{0}),
		bytesval([]byte{0, 1, 2, 3, 4}),
		bytesval([]byte{0, 1, 2, 4, 3}),
		bytesval([]byte{255}),
		bsonBinaryval(2, []byte{1}), // BSON Binary (subtype 2)
		refval("projects/p1/databases/d1/documents/c1/doc1"),
		refval("projects/p1/databases/d1/documents/c1/doc2"),
		refval("projects/p1/databases/d1/documents/c1/doc2/c2/doc1"),
		refval("projects/p1/databases/d1/documents/c1/doc2/c2/doc2"),
		refval("projects/p1/databases/d1/documents/c10/doc1"),
		refval("projects/p1/databases/dkkkkklkjnjkkk1/documents/c2/doc1"),
		refval("projects/p2/databases/d2/documents/c1/doc1"),
		refval("projects/p2/databases/d2/documents/c1-/doc1"),
		bsonObjectIDval("0123456789abcdef01234567"), // BSON ObjectID
		geopoint(-90, -180),
		geopoint(-90, 0),
		geopoint(-90, 180),
		geopoint(0, -180),
		geopoint(0, 0),
		geopoint(0, 180),
		geopoint(1, -180),
		geopoint(1, 0),
		geopoint(1, 180),
		geopoint(90, -180),
		geopoint(90, 0),
		geopoint(90, 180),
		bsonRegexval("foo", "im"), // BSON Regex
		arrayval(),
		arrayval(strval("bar")),
		arrayval(strval("foo")),
		arrayval(strval("foo"), intval(1)),
		arrayval(strval("foo"), intval(2)),
		arrayval(strval("foo"), strval("0")),
		mapval(map[string]*pb.Value{"bar": intval(0)}),
		mapval(map[string]*pb.Value{"bar": intval(0), "foo": intval(1)}),
		mapval(map[string]*pb.Value{"foo": intval(1)}),
		mapval(map[string]*pb.Value{"foo": intval(2)}),
		mapval(map[string]*pb.Value{"foo": strval("0")}),
		bsonMaxKey(), // BSON MaxKey
	}

	for i, v1 := range vals {
		if got := compareValues(v1, v1); got != 0 {
			t.Errorf("compare(%v, %v) == %d, want 0", v1, v1, got)
		}
		for _, v2 := range vals[i+1:] {
			if got := compareValues(v1, v2); got != -1 {
				t.Errorf("compare(%v, %v) == %d, want -1", v1, v2, got)
			}
			if got := compareValues(v2, v1); got != 1 {
				t.Errorf("compare(%v, %v) == %d, want 1", v2, v1, got)
			}
		}
	}

	// Integers and Doubles order the same.
	n1 := intval(17)
	n2 := floatval(17)
	if got := compareValues(n1, n2); got != 0 {
		t.Errorf("compare(%v, %v) == %d, want 0", n1, n2, got)
	}

	// BSON Int32 and Decimal128 order same as other numbers.
	bn1 := bsonInt32val(17)
	bn2 := bsonDecimal128val("17.0")
	if got := compareValues(bn1, n1); got != 0 {
		t.Errorf("compare(%v, %v) == %d, want 0", bn1, n1, got)
	}
	if got := compareValues(bn2, n2); got != 0 {
		t.Errorf("compare(%v, %v) == %d, want 0", bn2, n2, got)
	}
	if got := compareValues(bn1, bn2); got != 0 {
		t.Errorf("compare(%v, %v) == %d, want 0", bn1, bn2, got)
	}

	// Decimal128 NaN orders same as float NaN
	nan1 := floatval(math.NaN())
	nan2 := bsonDecimal128val("NaN")
	if got := compareValues(nan1, nan2); got != 0 {
		t.Errorf("compare(%v, %v) == %d, want 0", nan1, nan2, got)
	}
}

func geopoint(lat, lng float64) *pb.Value {
	return geoval(&latlng.LatLng{Latitude: lat, Longitude: lng})
}

func bsonMinKey() *pb.Value {
	return &pb.Value{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: map[string]*pb.Value{
		"__min__": nullValue,
	}}}}
}

func bsonMaxKey() *pb.Value {
	return &pb.Value{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: map[string]*pb.Value{
		"__max__": nullValue,
	}}}}
}

func bsonInt32val(i int32) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: map[string]*pb.Value{
		"__int__": int64val(int64(i)),
	}}}}
}

func bsonDecimal128val(s string) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: map[string]*pb.Value{
		"__decimal128__": strval(s),
	}}}}
}

func bsonTimestampval(seconds, increment int64) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: map[string]*pb.Value{
		"__request_timestamp__": mapval(map[string]*pb.Value{
			"seconds":   int64val(seconds),
			"increment": int64val(increment),
		}),
	}}}}
}

func bsonBinaryval(subtype byte, data []byte) *pb.Value {
	payload := make([]byte, len(data)+1)
	payload[0] = subtype
	copy(payload[1:], data)
	return &pb.Value{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: map[string]*pb.Value{
		"__binary__": bytesval(payload),
	}}}}
}

func bsonObjectIDval(hexStr string) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: map[string]*pb.Value{
		"__oid__": strval(hexStr),
	}}}}
}

func bsonRegexval(pattern, options string) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: map[string]*pb.Value{
		"__regex__": mapval(map[string]*pb.Value{
			"pattern": strval(pattern),
			"options": strval(options),
		}),
	}}}}
}
