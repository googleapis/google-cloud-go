/*
Copyright 2017 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/internal/testutil"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	pb "cloud.google.com/go/spanner/testdata/protos"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	proto3 "google.golang.org/protobuf/types/known/structpb"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

var (
	t1 = mustParseTime("2016-11-15T15:04:05.999999999Z")
	// Boundaries
	t2 = mustParseTime("0001-01-01T00:00:00.000000000Z")
	t3 = mustParseTime("9999-12-31T23:59:59.999999999Z")
	// Local timezone
	t4 = time.Now()
	d1 = mustParseDate("2016-11-15")
	d2 = mustParseDate("1678-01-01")
)

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic(err)
	}
	return t
}

func mustParseDate(s string) civil.Date {
	d, err := civil.ParseDate(s)
	if err != nil {
		panic(err)
	}
	return d
}

type customStructToString struct {
	A string
	B string
}

// Convert the customStructToString
func (c customStructToString) EncodeSpanner() (interface{}, error) {
	return "A-B", nil
}

// Convert to customStructToString
func (c *customStructToString) DecodeSpanner(val interface{}) (err error) {
	c.A = "A"
	c.B = "B"
	return nil
}

type customStructToInt struct {
	A int64
	B int64
}

// Convert the customStructToInt
func (c customStructToInt) EncodeSpanner() (interface{}, error) {
	return 123, nil
}

// Convert to customStructToInt
func (c *customStructToInt) DecodeSpanner(val interface{}) (err error) {
	c.A = 1
	c.B = 23
	return nil
}

type customStructToFloat struct {
	A float64
	B float64
}

// Convert the customStructToFloat
func (c customStructToFloat) EncodeSpanner() (interface{}, error) {
	return 123.123, nil
}

// Convert to customStructToFloat
func (c *customStructToFloat) DecodeSpanner(val interface{}) (err error) {
	c.A = 1.23
	c.B = 12.3
	return nil
}

type customStructToBool struct {
	A bool
	B bool
}

// Convert the customStructToBool
func (c customStructToBool) EncodeSpanner() (interface{}, error) {
	return true, nil
}

// Convert to customStructToBool
func (c *customStructToBool) DecodeSpanner(val interface{}) (err error) {
	c.A = true
	c.B = false
	return nil
}

type customStructToBytes struct {
	A []byte
	B []byte
}

// Convert the customStructToBytes
func (c customStructToBytes) EncodeSpanner() (interface{}, error) {
	return []byte("AB"), nil
}

// Convert to customStructToBytes
func (c *customStructToBytes) DecodeSpanner(val interface{}) (err error) {
	c.A = []byte("A")
	c.B = []byte("B")
	return nil
}

type customStructToTime struct {
	A string
	B string
}

// Convert the customStructToTime
func (c customStructToTime) EncodeSpanner() (interface{}, error) {
	return t1, nil
}

// Convert to customStructToTime
func (c *customStructToTime) DecodeSpanner(val interface{}) (err error) {
	c.A = "A"
	c.B = "B"
	return nil
}

type customStructToDate struct {
	A string
	B string
}

// Convert the customStructToDate
func (c customStructToDate) EncodeSpanner() (interface{}, error) {
	return d1, nil
}

// Convert to customStructToDate
func (c *customStructToDate) DecodeSpanner(val interface{}) (err error) {
	c.A = "A"
	c.B = "B"
	return nil
}

type customStructToNull struct {
	val interface{}
}

func (c customStructToNull) EncodeSpanner() (interface{}, error) {
	return c.val, nil
}

func (c *customStructToNull) DecodeSpanner(val interface{}) (err error) {
	if reflect.ValueOf(val).IsNil() {
		return nil
	}
	return fmt.Errorf("val mismatch: expected nil, got %v", val)
}

// e.g. a clock face time HH:MM
type customArray [4]uint8

// Convert to customArray from structpb.ListValue<structpb.StringValue>
func (c *customArray) DecodeSpanner(val interface{}) error {
	listVal, ok := val.(*structpb.ListValue)
	if !ok {
		return fmt.Errorf("failed to decode customArray: unexpected type of %v", val)
	}
	asSlice := listVal.AsSlice()
	if len(asSlice) != 4 {
		return errors.New("failed to decode customArray: expected array of length 4")
	}
	for i, vI := range asSlice {
		vStr, ok := vI.(string)
		if !ok {
			return fmt.Errorf("failed to decode customArray: got non string value: %v", vI)
		}
		vInt, _ := strconv.Atoi(vStr)
		c[i] = uint8(vInt)
	}
	return nil
}

// Test encoding Values.
func TestEncodeValue(t *testing.T) {
	type CustomString string
	type CustomBytes []byte
	type CustomInt64 int64
	type CustomBool bool
	type CustomFloat64 float64
	type CustomTime time.Time
	type CustomDate civil.Date
	type CustomNumeric big.Rat
	type CustomPGNumeric PGNumeric
	type CustomPGJSONB PGJsonB

	type CustomNullString NullString
	type CustomNullInt64 NullInt64
	type CustomNullBool NullBool
	type CustomNullFloat64 NullFloat64
	type CustomNullTime NullTime
	type CustomNullDate NullDate
	type CustomNullNumeric NullNumeric
	type CustomNullJSON NullJSON

	type Message struct {
		Name string
		Body string
		Time int64
	}
	msg := Message{"Alice", "Hello", 1294706395881547000}
	jsonStr := `{"Name":"Alice","Body":"Hello","Time":1294706395881547000}`
	emptyArrayJSONStr := `[]`
	type PtrMessage struct {
		Key *string
	}
	ptrMsg := PtrMessage{}
	nullValueJSONStr := `{"Key":null}`

	sValue := "abc"
	var sNilPtr *string
	iValue := int64(7)
	var iNilPtr *int64
	bValue := true
	var bNilPtr *bool
	fValue := 3.14
	var fNilPtr *float64
	f32Value := float32(3.14)
	var f32NilPtr *float32
	tValue := t1
	var tNilPtr *time.Time
	dValue := d1
	var dNilPtr *civil.Date
	numValuePtr := big.NewRat(12345, 1e3)
	var numNilPtr *big.Rat
	num2ValuePtr := big.NewRat(12345, 1e4)
	maxNumValuePtr, _ := (&big.Rat{}).SetString("99999999999999999999999999999.999999999")
	minNumValuePtr, _ := (&big.Rat{}).SetString("-99999999999999999999999999999.999999999")

	singer1ProtoEnum := pb.Genre_ROCK
	singer1ProtoMsg := &pb.SingerInfo{
		SingerId:    proto.Int64(1),
		BirthDate:   proto.String("January"),
		Nationality: proto.String("Country1"),
		Genre:       &singer1ProtoEnum,
	}

	singer2ProtoEnum := pb.Genre_FOLK
	singer2ProtoMsg := &pb.SingerInfo{
		SingerId:    proto.Int64(2),
		BirthDate:   proto.String("February"),
		Nationality: proto.String("Country2"),
		Genre:       &singer2ProtoEnum,
	}
	protoMessagefqn := "examples.spanner.music.SingerInfo"
	protoEnumfqn := "examples.spanner.music.Genre"

	var (
		tString       = stringType()
		tInt          = intType()
		tBool         = boolType()
		tFloat        = floatType()
		tFloat32      = float32Type()
		tBytes        = bytesType()
		tTime         = timeType()
		tDate         = dateType()
		tNumeric      = numericType()
		tJSON         = jsonType()
		tPGNumeric    = pgNumericType()
		tPGJsonb      = pgJsonbType()
		tProtoMessage = protoMessageType(protoMessagefqn)
		tProtoEnum    = protoEnumType(protoEnumfqn)
	)
	for i, test := range []struct {
		in       interface{}
		want     *proto3.Value
		wantType *sppb.Type
		name     string
	}{
		// STRING / STRING ARRAY:
		{"abc", stringProto("abc"), tString, "string"},
		{NullString{"abc", true}, stringProto("abc"), tString, "NullString with value"},
		{NullString{"abc", false}, nullProto(), tString, "NullString with null"},
		{&sValue, stringProto("abc"), tString, "*string with value"},
		{sNilPtr, nullProto(), tString, "*string with null"},
		{[]string(nil), nullProto(), listType(tString), "null []string"},
		{[]string{"abc", "bcd"}, listProto(stringProto("abc"), stringProto("bcd")), listType(tString), "[]string"},
		{[]NullString{{"abcd", true}, {"xyz", false}}, listProto(stringProto("abcd"), nullProto()), listType(tString), "[]NullString"},
		{[]*string{&sValue, sNilPtr}, listProto(stringProto("abc"), nullProto()), listType(tString), "[]*string"},
		// BYTES / BYTES ARRAY
		{[]byte("foo"), bytesProto([]byte("foo")), tBytes, "[]byte with value"},
		{[]byte(nil), nullProto(), tBytes, "null []byte"},
		{[][]byte{nil, []byte("ab")}, listProto(nullProto(), bytesProto([]byte("ab"))), listType(tBytes), "[][]byte"},
		{[][]byte(nil), nullProto(), listType(tBytes), "null [][]byte"},
		// INT64 / INT64 ARRAY
		{7, intProto(7), tInt, "int"},
		{[]int(nil), nullProto(), listType(tInt), "null []int"},
		{[]int{31, 127}, listProto(intProto(31), intProto(127)), listType(tInt), "[]int"},
		{int64(81), intProto(81), tInt, "int64"},
		{[]int64(nil), nullProto(), listType(tInt), "null []int64"},
		{[]int64{33, 129}, listProto(intProto(33), intProto(129)), listType(tInt), "[]int64"},
		{NullInt64{11, true}, intProto(11), tInt, "NullInt64 with value"},
		{NullInt64{11, false}, nullProto(), tInt, "NullInt64 with null"},
		{&iValue, intProto(7), tInt, "*int64 with value"},
		{iNilPtr, nullProto(), tInt, "*int64 with null"},
		{[]NullInt64{{35, true}, {131, false}}, listProto(intProto(35), nullProto()), listType(tInt), "[]NullInt64"},
		{[]*int64{&iValue, iNilPtr}, listProto(intProto(7), nullProto()), listType(tInt), "[]*int64"},
		// BOOL / BOOL ARRAY
		{true, boolProto(true), tBool, "bool"},
		{NullBool{true, true}, boolProto(true), tBool, "NullBool with value"},
		{NullBool{true, false}, nullProto(), tBool, "NullBool with null"},
		{&bValue, boolProto(true), tBool, "*bool with value"},
		{bNilPtr, nullProto(), tBool, "*bool with null"},
		{[]bool{true, false}, listProto(boolProto(true), boolProto(false)), listType(tBool), "[]bool"},
		{[]NullBool{{true, true}, {true, false}}, listProto(boolProto(true), nullProto()), listType(tBool), "[]NullBool"},
		{[]*bool{&bValue, bNilPtr}, listProto(boolProto(true), nullProto()), listType(tBool), "[]*bool"},
		// FLOAT64 / FLOAT64 ARRAY
		{3.14, floatProto(3.14), tFloat, "float"},
		{NullFloat64{3.1415, true}, floatProto(3.1415), tFloat, "NullFloat64 with value"},
		{NullFloat64{math.Inf(1), true}, floatProto(math.Inf(1)), tFloat, "NullFloat64 with infinity"},
		{NullFloat64{3.14159, false}, nullProto(), tFloat, "NullFloat64 with null"},
		{&fValue, floatProto(3.14), tFloat, "*float64 with value"},
		{fNilPtr, nullProto(), tFloat, "*float64 with null"},
		{[]float64(nil), nullProto(), listType(tFloat), "null []float64"},
		{[]float64{3.141, 0.618, math.Inf(-1)}, listProto(floatProto(3.141), floatProto(0.618), floatProto(math.Inf(-1))), listType(tFloat), "[]float64"},
		{[]NullFloat64{{3.141, true}, {0.618, false}}, listProto(floatProto(3.141), nullProto()), listType(tFloat), "[]NullFloat64"},
		{[]*float64{&fValue, fNilPtr}, listProto(floatProto(3.14), nullProto()), listType(tFloat), "[]NullFloat64"},
		// FLOAT32 / FLOAT32 ARRAY
		{float32(3.14), float32Proto(3.14), tFloat32, "float32"},
		{NullFloat32{3.14, true}, float32Proto(3.14), tFloat32, "NullFloat32 with value"},
		{NullFloat32{float32(math.Inf(1)), true}, float32Proto(float32(math.Inf(1))), tFloat32, "NullFloat32 with infinity"},
		{NullFloat32{3.14, false}, nullProto(), tFloat32, "NullFloat32 with null"},
		{&f32Value, float32Proto(3.14), tFloat32, "*float32 with value"},
		{f32NilPtr, nullProto(), tFloat32, "*float32 with null"},
		{[]float32(nil), nullProto(), listType(tFloat32), "null []float32"},
		{[]float32{3.14, 0.618, float32(math.Inf(-1))}, listProto(float32Proto(3.14), float32Proto(0.618), float32Proto(float32(math.Inf(-1)))), listType(tFloat32), "[]float32"},
		{[]NullFloat32{{3.14, true}, {0.618, false}}, listProto(float32Proto(3.14), nullProto()), listType(tFloat32), "[]NullFloat"},
		{[]*float32{&f32Value, f32NilPtr}, listProto(float32Proto(3.14), nullProto()), listType(tFloat32), "[]NullFloat32"},
		// NUMERIC / NUMERIC ARRAY
		{*numValuePtr, numericProto(numValuePtr), tNumeric, "big.Rat"},
		{numValuePtr, numericProto(numValuePtr), tNumeric, "*big.Rat"},
		{maxNumValuePtr, numericProto(maxNumValuePtr), tNumeric, "max numeric"},
		{minNumValuePtr, numericProto(minNumValuePtr), tNumeric, "min numeric"},
		{numNilPtr, nullProto(), tNumeric, "*big.Rat with null"},
		{NullNumeric{*numValuePtr, true}, numericProto(numValuePtr), tNumeric, "NullNumeric with value"},
		{NullNumeric{*numValuePtr, false}, nullProto(), tNumeric, "NullNumeric with null"},
		{[]big.Rat(nil), nullProto(), listType(tNumeric), "null []big.Rat"},
		{[]big.Rat{*numValuePtr, *num2ValuePtr}, listProto(numericProto(numValuePtr), numericProto(num2ValuePtr)), listType(tNumeric), "[]big.Rat"},
		{[]NullNumeric{{*numValuePtr, true}, {*numValuePtr, false}}, listProto(numericProto(numValuePtr), nullProto()), listType(tNumeric), "[]NullNumeric"},
		{[]*big.Rat{nil, numValuePtr}, listProto(nullProto(), numericProto(numValuePtr)), listType(tNumeric), "[]*big.Rat"},
		{[]*big.Rat(nil), nullProto(), listType(tNumeric), "null []*big.Rat"},
		// JSON
		{NullJSON{msg, true}, stringProto(jsonStr), tJSON, "NullJSON with value"},
		{NullJSON{msg, false}, nullProto(), tJSON, "NullJSON with null"},
		{[]NullJSON(nil), nullProto(), listType(tJSON), "null []NullJSON"},
		{[]NullJSON{{msg, true}, {msg, false}}, listProto(stringProto(jsonStr), nullProto()), listType(tJSON), "[]NullJSON"},
		{NullJSON{[]Message{}, true}, stringProto(emptyArrayJSONStr), tJSON, "a json string with empty array to NullJSON"},
		{NullJSON{ptrMsg, true}, stringProto(nullValueJSONStr), tJSON, "a json string with null value to NullJSON"},
		// PG JSONB
		{PGJsonB{Value: msg, Valid: true}, stringProto(jsonStr), tPGJsonb, "PGJsonB with value"},
		{PGJsonB{Value: msg, Valid: false}, nullProto(), tPGJsonb, "PGJsonB with null"},
		{[]PGJsonB(nil), nullProto(), listType(tPGJsonb), "null []PGJsonB"},
		{[]PGJsonB{{Value: msg, Valid: true}, {Value: msg, Valid: false}}, listProto(stringProto(jsonStr), nullProto()), listType(tPGJsonb), "[]PGJsonB"},
		{PGJsonB{Value: []Message{}, Valid: true}, stringProto(emptyArrayJSONStr), tPGJsonb, "a json string with empty array to PGJsonB"},
		{PGJsonB{Value: ptrMsg, Valid: true}, stringProto(nullValueJSONStr), tPGJsonb, "a json string with null value to PGJsonB"},
		// PG NUMERIC
		{PGNumeric{"123.456", true}, stringProto("123.456"), tPGNumeric, "PG Numeric"},
		{PGNumeric{Valid: false}, nullProto(), tPGNumeric, "PG Numeric with a null value"},
		{[]PGNumeric(nil), nullProto(), listType(tPGNumeric), "null []PGNumeric"},
		{[]PGNumeric{{"123.456", true}, {Valid: false}}, listProto(stringProto("123.456"), nullProto()), listType(tPGNumeric), "[]PGNumeric"},
		// TIMESTAMP / TIMESTAMP ARRAY
		{t1, timeProto(t1), tTime, "time"},
		{NullTime{t1, true}, timeProto(t1), tTime, "NullTime with value"},
		{NullTime{t1, false}, nullProto(), tTime, "NullTime with null"},
		{&tValue, timeProto(t1), tTime, "*time.Time with value"},
		{tNilPtr, nullProto(), tTime, "*time.Time with null"},
		{[]time.Time(nil), nullProto(), listType(tTime), "null []time"},
		{[]time.Time{t1, t2, t3, t4}, listProto(timeProto(t1), timeProto(t2), timeProto(t3), timeProto(t4)), listType(tTime), "[]time"},
		{[]NullTime{{t1, true}, {t1, false}}, listProto(timeProto(t1), nullProto()), listType(tTime), "[]NullTime"},
		{[]*time.Time{&tValue, tNilPtr}, listProto(timeProto(t1), nullProto()), listType(tTime), "[]*time.Time"},
		// DATE / DATE ARRAY
		{d1, dateProto(d1), tDate, "date"},
		{NullDate{d1, true}, dateProto(d1), tDate, "NullDate with value"},
		{NullDate{civil.Date{}, false}, nullProto(), tDate, "NullDate with null"},
		{&dValue, dateProto(d1), tDate, "*civil.Date with value"},
		{dNilPtr, nullProto(), tDate, "*civil.Date with null"},
		{[]civil.Date(nil), nullProto(), listType(tDate), "null []date"},
		{[]civil.Date{d1, d2}, listProto(dateProto(d1), dateProto(d2)), listType(tDate), "[]date"},
		{[]NullDate{{d1, true}, {civil.Date{}, false}}, listProto(dateProto(d1), nullProto()), listType(tDate), "[]NullDate"},
		{[]*civil.Date{&dValue, dNilPtr}, listProto(dateProto(d1), nullProto()), listType(tDate), "[]*civil.Date"},
		// GenericColumnValue
		{GenericColumnValue{tString, stringProto("abc")}, stringProto("abc"), tString, "GenericColumnValue with value"},
		{GenericColumnValue{tString, nullProto()}, nullProto(), tString, "GenericColumnValue with null"},
		// not actually valid (stringProto inside int list), but demonstrates pass-through.
		{
			GenericColumnValue{
				Type:  listType(tInt),
				Value: listProto(intProto(5), nullProto(), stringProto("bcd")),
			},
			listProto(intProto(5), nullProto(), stringProto("bcd")),
			listType(tInt),
			"pass-through",
		},
		// placeholder
		{CommitTimestamp, stringProto(commitTimestampPlaceholderString), tTime, "CommitTimestampPlaceholder"},
		// CUSTOM STRING / CUSTOM STRING ARRAY
		{CustomString("abc"), stringProto("abc"), tString, "CustomString"},
		{CustomNullString{"abc", true}, stringProto("abc"), tString, "CustomNullString with value"},
		{CustomNullString{"abc", false}, nullProto(), tString, "CustomNullString with null"},
		{[]CustomString(nil), nullProto(), listType(tString), "null []CustomString"},
		{[]CustomString{"abc", "bcd"}, listProto(stringProto("abc"), stringProto("bcd")), listType(tString), "[]CustomString"},
		{[]CustomNullString(nil), nullProto(), listType(tString), "null []NullCustomString"},
		{[]CustomNullString{{"abcd", true}, {"xyz", false}}, listProto(stringProto("abcd"), nullProto()), listType(tString), "[]NullCustomString"},
		// CUSTOM BYTES / CUSTOM BYTES ARRAY
		{CustomBytes("foo"), bytesProto([]byte("foo")), tBytes, "CustomBytes with value"},
		{CustomBytes(nil), nullProto(), tBytes, "null CustomBytes"},
		{[]CustomBytes{nil, CustomBytes("ab")}, listProto(nullProto(), bytesProto([]byte("ab"))), listType(tBytes), "[]CustomBytes"},
		{[]CustomBytes(nil), nullProto(), listType(tBytes), "null []CustomBytes"},
		// CUSTOM INT64 / CUSTOM INT64 ARRAY
		{CustomInt64(81), intProto(81), tInt, "CustomInt64"},
		{[]CustomInt64(nil), nullProto(), listType(tInt), "null []CustomInt64"},
		{[]CustomInt64{33, 129}, listProto(intProto(33), intProto(129)), listType(tInt), "[]CustomInt64"},
		{CustomNullInt64{11, true}, intProto(11), tInt, "CustomNullInt64 with value"},
		{CustomNullInt64{11, false}, nullProto(), tInt, "CustomNullInt64 with null"},
		{[]CustomNullInt64(nil), nullProto(), listType(tInt), "null []CustomNullInt64"},
		{[]CustomNullInt64{{35, true}, {131, false}}, listProto(intProto(35), nullProto()), listType(tInt), "[]CustomNullInt64"},
		// CUSTOM BOOL / CUSTOM BOOL ARRAY
		{CustomBool(true), boolProto(true), tBool, "CustomBool"},
		{CustomNullBool{true, true}, boolProto(true), tBool, "CustomNullBool with value"},
		{CustomNullBool{true, false}, nullProto(), tBool, "CustomNullBool with null"},
		{[]CustomBool{true, false}, listProto(boolProto(true), boolProto(false)), listType(tBool), "[]CustomBool"},
		{[]CustomNullBool{{true, true}, {true, false}}, listProto(boolProto(true), nullProto()), listType(tBool), "[]CustomNullBool"},
		// FLOAT64 / FLOAT64 ARRAY
		{CustomFloat64(3.14), floatProto(3.14), tFloat, "CustomFloat64"},
		{CustomNullFloat64{3.1415, true}, floatProto(3.1415), tFloat, "CustomNullFloat64 with value"},
		{CustomNullFloat64{math.Inf(1), true}, floatProto(math.Inf(1)), tFloat, "CustomNullFloat64 with infinity"},
		{CustomNullFloat64{3.14159, false}, nullProto(), tFloat, "CustomNullFloat64 with null"},
		{[]CustomFloat64(nil), nullProto(), listType(tFloat), "null []CustomFloat64"},
		{[]CustomFloat64{3.141, 0.618, CustomFloat64(math.Inf(-1))}, listProto(floatProto(3.141), floatProto(0.618), floatProto(math.Inf(-1))), listType(tFloat), "[]CustomFloat64"},
		{[]CustomNullFloat64(nil), nullProto(), listType(tFloat), "null []CustomNullFloat64"},
		{[]CustomNullFloat64{{3.141, true}, {0.618, false}}, listProto(floatProto(3.141), nullProto()), listType(tFloat), "[]CustomNullFloat64"},
		// CUSTOM TIMESTAMP / CUSTOM TIMESTAMP ARRAY
		{CustomTime(t1), timeProto(t1), tTime, "CustomTime"},
		{CustomNullTime{t1, true}, timeProto(t1), tTime, "CustomNullTime with value"},
		{CustomNullTime{t1, false}, nullProto(), tTime, "CustomNullTime with null"},
		{[]CustomTime(nil), nullProto(), listType(tTime), "null []CustomTime"},
		{[]CustomTime{CustomTime(t1), CustomTime(t2), CustomTime(t3), CustomTime(t4)}, listProto(timeProto(t1), timeProto(t2), timeProto(t3), timeProto(t4)), listType(tTime), "[]CustomTime"},
		{[]CustomNullTime(nil), nullProto(), listType(tTime), "null []CustomNullTime"},
		{[]CustomNullTime{{t1, true}, {t1, false}}, listProto(timeProto(t1), nullProto()), listType(tTime), "[]CustomNullTime"},
		// CUSTOM DATE / CUSTOM DATE ARRAY
		{CustomDate(d1), dateProto(d1), tDate, "CustomDate"},
		{CustomNullDate{d1, true}, dateProto(d1), tDate, "CustomNullDate with value"},
		{CustomNullDate{civil.Date{}, false}, nullProto(), tDate, "CustomNullDate with null"},
		{[]CustomDate(nil), nullProto(), listType(tDate), "null []CustomDate"},
		{[]CustomDate{CustomDate(d1), CustomDate(d2)}, listProto(dateProto(d1), dateProto(d2)), listType(tDate), "[]CustomDate"},
		{[]CustomNullDate(nil), nullProto(), listType(tDate), "null []CustomNullDate"},
		{[]CustomNullDate{{d1, true}, {civil.Date{}, false}}, listProto(dateProto(d1), nullProto()), listType(tDate), "[]NullDate"},
		// CUSTOM STRUCT
		{customStructToString{"A", "B"}, stringProto("A-B"), tString, "a struct to string"},
		{customStructToInt{1, 23}, intProto(123), tInt, "a struct to int"},
		{customStructToFloat{1.23, 12.3}, floatProto(123.123), tFloat, "a struct to float"},
		{customStructToBool{true, false}, boolProto(true), tBool, "a struct to bool"},
		{customStructToBytes{[]byte("A"), []byte("B")}, bytesProto([]byte("AB")), tBytes, "a struct to bytes"},
		{customStructToTime{"A", "B"}, timeProto(tValue), tTime, "a struct to time"},
		{customStructToDate{"A", "B"}, dateProto(dValue), tDate, "a struct to date"},
		{customStructToNull{val: bNilPtr}, nullProto(), tBool, "a struct to null bool"},
		{customStructToNull{val: []byte(nil)}, nullProto(), tBytes, "a struct to null bytes"},
		{customStructToNull{val: sNilPtr}, nullProto(), tString, "a struct to null string"},
		{customStructToNull{val: iNilPtr}, nullProto(), tInt, "a struct to null int"},
		{customStructToNull{val: fNilPtr}, nullProto(), tFloat, "a struct to null float"},
		{customStructToNull{val: numNilPtr}, nullProto(), tNumeric, "a struct to null numeric"},
		{customStructToNull{val: dNilPtr}, nullProto(), tDate, "a struct to null date"},
		{customStructToNull{val: tNilPtr}, nullProto(), tTime, "a struct to null timestamp"},
		// CUSTOM NUMERIC / CUSTOM NUMERIC ARRAY
		{CustomNumeric(*numValuePtr), numericProto(numValuePtr), tNumeric, "CustomNumeric"},
		{CustomNullNumeric{*numValuePtr, true}, numericProto(numValuePtr), tNumeric, "CustomNullNumeric with value"},
		{CustomNullNumeric{*numValuePtr, false}, nullProto(), tNumeric, "CustomNullNumeric with null"},
		{[]CustomNumeric(nil), nullProto(), listType(tNumeric), "null []CustomNumeric"},
		{[]CustomNumeric{CustomNumeric(*numValuePtr), CustomNumeric(*num2ValuePtr)}, listProto(numericProto(numValuePtr), numericProto(num2ValuePtr)), listType(tNumeric), "[]CustomNumeric"},
		{[]CustomNullNumeric(nil), nullProto(), listType(tNumeric), "null []CustomNullNumeric"},
		{[]CustomNullNumeric{{*numValuePtr, true}, {*num2ValuePtr, false}}, listProto(numericProto(numValuePtr), nullProto()), listType(tNumeric), "[]CustomNullNumeric"},
		// CUSTOM JSON
		{CustomNullJSON{msg, true}, stringProto(jsonStr), tJSON, "CustomNullJSON with value"},
		{CustomNullJSON{msg, false}, nullProto(), tJSON, "CustomNullJSON with null"},
		{[]CustomNullJSON(nil), nullProto(), listType(tJSON), "null []CustomNullJSON"},
		{[]CustomNullJSON{{msg, true}, {msg, false}}, listProto(stringProto(jsonStr), nullProto()), listType(tJSON), "[]CustomNullJSON"},
		// CUSTOM PG NUMERIC
		{CustomPGNumeric{"123.456", true}, stringProto("123.456"), tPGNumeric, "PG Numeric"},
		{CustomPGNumeric{Valid: false}, nullProto(), tPGNumeric, "PG Numeric with a null value"},
		{[]CustomPGNumeric(nil), nullProto(), listType(tPGNumeric), "null []PGNumeric"},
		{[]CustomPGNumeric{{"123.456", true}, {Valid: false}}, listProto(stringProto("123.456"), nullProto()), listType(tPGNumeric), "[]PGNumeric"},
		// CUSTOM PG JSONB
		{CustomPGJSONB{Value: msg, Valid: true}, stringProto(jsonStr), tPGJsonb, "CustomPGJSONB with value"},
		{CustomPGJSONB{Value: msg, Valid: false}, nullProto(), tPGJsonb, "CustomPGJSONB with null"},
		{[]CustomPGJSONB(nil), nullProto(), listType(tPGJsonb), "null []CustomPGJSONB"},
		{[]CustomPGJSONB{{Value: msg, Valid: true}, {Value: msg, Valid: false}}, listProto(stringProto(jsonStr), nullProto()), listType(tPGJsonb), "[]CustomPGJSONB"},
		// PROTO MESSAGE AND PROTO ENUM
		{singer1ProtoMsg, protoMessageProto(singer1ProtoMsg), tProtoMessage, "Proto Message"},
		{singer1ProtoEnum, protoEnumProto(singer1ProtoEnum), tProtoEnum, "Proto Enum"},
		{(*pb.SingerInfo)(nil), nullProto(), tProtoMessage, "Proto Message with nil"},
		{(*pb.Genre)(nil), nullProto(), tProtoEnum, "Proto Enum with nil"},
		{NullProtoMessage{singer1ProtoMsg, true}, protoMessageProto(singer1ProtoMsg), tProtoMessage, "NullProto with value"},
		{NullProtoEnum{singer1ProtoEnum, true}, protoEnumProto(singer1ProtoEnum), tProtoEnum, "NullEnum with value"},
		{NullProtoMessage{(*pb.SingerInfo)(nil), true}, nullProto(), tProtoMessage, "NullProto with value nil"},
		{NullProtoEnum{(*pb.Genre)(nil), true}, nullProto(), tProtoEnum, "NullEnum with value nil"},
		// ARRAY OF PROTO MESSAGES AND PROTO ENUM
		{[]*pb.SingerInfo{singer1ProtoMsg, singer2ProtoMsg}, listProto(protoMessageProto(singer1ProtoMsg), protoMessageProto(singer2ProtoMsg)), listType(tProtoMessage), "Array of Proto Message"},
		{[]*pb.SingerInfo{singer1ProtoMsg, singer2ProtoMsg, nil, (*pb.SingerInfo)(nil)}, listProto(protoMessageProto(singer1ProtoMsg), protoMessageProto(singer2ProtoMsg), nullProto(), nullProto()), listType(tProtoMessage), "Array of Proto Message with nil values"},
		{[]*pb.Genre{&singer1ProtoEnum, &singer2ProtoEnum}, listProto(protoEnumProto(singer1ProtoEnum), protoEnumProto(singer2ProtoEnum)), listType(tProtoEnum), "Array of Proto Enum 1"},
		{[]pb.Genre{singer1ProtoEnum, singer2ProtoEnum}, listProto(protoEnumProto(singer1ProtoEnum), protoEnumProto(singer2ProtoEnum)), listType(tProtoEnum), "Array of Proto Enum 2"},
		{[]*pb.Genre{&singer1ProtoEnum, &singer2ProtoEnum, nil, (*pb.Genre)(nil)}, listProto(protoEnumProto(singer1ProtoEnum), protoEnumProto(singer2ProtoEnum), nullProto(), nullProto()), listType(tProtoEnum), "Array of Proto Enum with nil values"},
		{[]*pb.SingerInfo{}, listProto(), listType(tProtoMessage), "Empty Array of Proto Message"},
		{[]*pb.SingerInfo(nil), nullProto(), listType(tProtoMessage), "Nil array of Proto Message"},
		{[]*pb.Genre{}, listProto(), listType(tProtoEnum), "Empty Array of Proto Enum 1"},
		{[]*pb.Genre(nil), nullProto(), listType(tProtoEnum), "Nil Array of Proto Enum 1"},
		{[]pb.Genre{}, listProto(), listType(tProtoEnum), "Empty Array of Proto Enum 2"},
		{[]pb.Genre(nil), nullProto(), listType(tProtoEnum), "Nil Array of Proto Enum 2"},
		// Null elements in ARRAY OF PROTO MESSAGES AND PROTO ENUM
		{[]*pb.SingerInfo{nil, (*pb.SingerInfo)(nil)}, listProto(nullProto(), nullProto()), listType(tProtoMessage), "Array of Proto Message with nil values"},
		{[]*pb.Genre{nil, (*pb.Genre)(nil)}, listProto(nullProto(), nullProto()), listType(tProtoEnum), "Array of Proto Enum with nil values"},
		{[]*pb.SingerInfo{singer1ProtoMsg, singer2ProtoMsg, nil, (*pb.SingerInfo)(nil)}, listProto(protoMessageProto(singer1ProtoMsg), protoMessageProto(singer2ProtoMsg), nullProto(), nullProto()), listType(tProtoMessage), "Array of Proto Message with non-nil and nil values"},
		{[]*pb.Genre{&singer1ProtoEnum, &singer2ProtoEnum, nil, (*pb.Genre)(nil)}, listProto(protoEnumProto(singer1ProtoEnum), protoEnumProto(singer2ProtoEnum), nullProto(), nullProto()), listType(tProtoEnum), "Array of Proto Enum with non-nil and nil values"},
		// PROTO MESSAGE AND ENUM WITH CUSTOM ENCODER
		{&pb.CustomSingerInfo{SingerName: &sValue}, stringProto("abc"), tString, "Proto message with encoder interface to string"},
		{pb.CustomGenre_CUSTOM_ROCK, stringProto("CUSTOM_ROCK"), tString, "Proto Enum with encoder interface to string"},
	} {
		got, gotType, err := encodeValue(test.in)
		if err != nil {
			t.Fatalf("#%d (%s): got error during encoding: %v, want nil", i, test.name, err)
		}
		if !testEqual(got, test.want) {
			t.Errorf("#%d (%s): got encode result: %v, want %v", i, test.name, got, test.want)
		}
		if !testEqual(gotType, test.wantType) {
			t.Errorf("#%d (%s): got encode type: %v, want %v", i, test.name, gotType, test.wantType)
		}
	}
}

// Test encoding invalid values.
func TestEncodeInvalidValues(t *testing.T) {
	type CustomNumeric big.Rat

	invalidNumPtr1 := big.NewRat(11234567891, 1e10)
	invalidNumPtr2, _ := (&big.Rat{}).SetString("199999999999999999999999999999.999999999")

	// Enable error mode.
	oldValue := LossOfPrecisionHandling
	defer func() {
		// Reset the value to pre-test value
		LossOfPrecisionHandling = oldValue
	}()
	LossOfPrecisionHandling = NumericError

	for i, test := range []struct {
		desc   string
		in     interface{}
		errMsg string
	}{
		// NUMERIC
		{desc: "numeric pointer with invalid scale component", in: invalidNumPtr1, errMsg: "max scale for a numeric is 9. The requested numeric has more"},
		{desc: "numeric pointer with invalid whole component", in: invalidNumPtr2, errMsg: "max precision for the whole component of a numeric is 29. The requested numeric has a whole component with precision 30"},
		{desc: "numeric with invalid scale component", in: *invalidNumPtr1, errMsg: "max scale for a numeric is 9. The requested numeric has more"},
		{desc: "numeric with invalid whole component", in: *invalidNumPtr2, errMsg: "max precision for the whole component of a numeric is 29. The requested numeric has a whole component with precision 30"},
		// CUSTOM NUMERIC
		{desc: "custom numeric type with invalid scale component", in: CustomNumeric(*invalidNumPtr1), errMsg: "max scale for a numeric is 9. The requested numeric has more"},
		{desc: "custom numeric type with invalid whole component", in: CustomNumeric(*invalidNumPtr2), errMsg: "max precision for the whole component of a numeric is 29. The requested numeric has a whole component with precision 30"},
		// PROTO MESSAGE AND PROTO ENUM
		{desc: "Invalid Null Proto", in: NullProtoMessage{}, errMsg: "spanner: code = \"InvalidArgument\", desc = \"field \\\"Valid\\\" of spanner.NullProtoMessage cannot be set to false when writing data to Cloud Spanner. Use typed nil in spanner.NullProtoMessage to write null values to Cloud Spanner\""},
		{desc: "Invalid Null Enum", in: NullProtoEnum{}, errMsg: "spanner: code = \"InvalidArgument\", desc = \"field \\\"Valid\\\" of spanner.NullProtoEnum cannot be set to false when writing data to Cloud Spanner. Use typed nil in spanner.NullProtoEnum to write null values to Cloud Spanner\""},
	} {
		_, _, err := encodeValue(test.in)
		if err == nil {
			t.Fatalf("#%d (%s): want error during encoding, but got nil", i, test.desc)
		}
		if err.Error() != test.errMsg {
			t.Errorf("#%d (%s): incorrect error message, got %v, want %v", i, test.desc, err, test.errMsg)
		}
	}
}

type encodeTest struct {
	desc     string
	in       interface{}
	want     *proto3.Value
	wantType *sppb.Type
}

func checkStructEncoding(desc string, got *proto3.Value, gotType *sppb.Type,
	want *proto3.Value, wantType *sppb.Type, t *testing.T) {
	if !testEqual(got, want) {
		t.Errorf("Test %s: got encode result: %v, want %v", desc, got, want)
	}
	if !testEqual(gotType, wantType) {
		t.Errorf("Test %s: got encode type: %v, want %v", desc, gotType, wantType)
	}
}

// Testcase code
func encodeStructValue(test encodeTest, t *testing.T) {
	got, gotType, err := encodeValue(test.in)
	if err != nil {
		t.Fatalf("Test %s: got error during encoding: %v, want nil", test.desc, err)
	}
	checkStructEncoding(test.desc, got, gotType, test.want, test.wantType, t)
}

func TestEncodeStructValuePointers(t *testing.T) {
	type structf struct {
		F int `spanner:"ff2"`
	}
	nestedStructProto := structType(mkField("ff2", intType()))

	type testType struct {
		Stringf    string
		Structf    *structf
		ArrStructf []*structf
	}
	testTypeProto := structType(
		mkField("Stringf", stringType()),
		mkField("Structf", nestedStructProto),
		mkField("ArrStructf", listType(nestedStructProto)))

	for _, test := range []encodeTest{
		{
			"Pointer to Go struct with pointers-to-(array)-struct fields.",
			&testType{"hello", &structf{50}, []*structf{{30}, {40}}},
			listProto(
				stringProto("hello"),
				listProto(intProto(50)),
				listProto(
					listProto(intProto(30)),
					listProto(intProto(40)))),
			testTypeProto,
		},
		{
			"Nil pointer to Go struct representing a NULL struct value.",
			(*testType)(nil),
			nullProto(),
			testTypeProto,
		},
		{
			"Slice of pointers to Go structs with NULL and non-NULL elements.",
			[]*testType{
				(*testType)(nil),
				{"hello", nil, []*structf{nil, {40}}},
				{"world", &structf{70}, nil},
			},
			listProto(
				nullProto(),
				listProto(
					stringProto("hello"),
					nullProto(),
					listProto(nullProto(), listProto(intProto(40)))),
				listProto(
					stringProto("world"),
					listProto(intProto(70)),
					nullProto())),
			listType(testTypeProto),
		},
		{
			"Nil slice of pointers to structs representing a NULL array of structs.",
			[]*testType(nil),
			nullProto(),
			listType(testTypeProto),
		},
		{
			"Empty slice of pointers to structs representing an empty array of structs.",
			[]*testType{},
			listProto(),
			listType(testTypeProto),
		},
	} {
		encodeStructValue(test, t)
	}
}

func TestEncodeStructValueErrors(t *testing.T) {
	type Embedded struct {
		A int
	}
	type embedded struct {
		B bool
	}
	x := 0

	for _, test := range []struct {
		desc    string
		in      interface{}
		wantErr error
	}{
		{
			"Unsupported embedded fields.",
			struct{ Embedded }{Embedded{10}},
			errUnsupportedEmbeddedStructFields("Embedded"),
		},
		{
			"Unsupported pointer to embedded fields.",
			struct{ *Embedded }{&Embedded{10}},
			errUnsupportedEmbeddedStructFields("Embedded"),
		},
		{
			"Unsupported embedded + unexported fields.",
			struct {
				int
				*bool
				embedded
			}{10, nil, embedded{false}},
			errUnsupportedEmbeddedStructFields("int"),
		},
		{
			"Unsupported type.",
			(**struct{})(nil),
			errEncoderUnsupportedType((**struct{})(nil)),
		},
		{
			"Unsupported type.",
			3,
			errEncoderUnsupportedType(3),
		},
		{
			"Unsupported type.",
			&x,
			errEncoderUnsupportedType(&x),
		},
	} {
		_, _, got := encodeStruct(test.in)
		if got == nil || !testEqual(test.wantErr, got) {
			t.Errorf("Test: %s, expected error %v during decoding, got %v", test.desc, test.wantErr, got)
		}
	}
}

func TestEncodeStructValueArrayStructFields(t *testing.T) {
	type structf struct {
		Intff int
	}

	structfType := structType(mkField("Intff", intType()))
	for _, test := range []encodeTest{
		{
			"Unnamed array-of-struct-typed field.",
			struct {
				Intf       int
				ArrStructf []structf `spanner:""`
			}{10, []structf{{1}, {2}}},
			listProto(
				intProto(10),
				listProto(
					listProto(intProto(1)),
					listProto(intProto(2)))),
			structType(
				mkField("Intf", intType()),
				mkField("", listType(structfType))),
		},
		{
			"Null array-of-struct-typed field.",
			struct {
				Intf       int
				ArrStructf []structf
			}{10, []structf(nil)},
			listProto(intProto(10), nullProto()),
			structType(
				mkField("Intf", intType()),
				mkField("ArrStructf", listType(structfType))),
		},
		{
			"Array-of-struct-typed field representing empty array.",
			struct {
				Intf       int
				ArrStructf []structf
			}{10, []structf{}},
			listProto(intProto(10), listProto([]*proto3.Value{}...)),
			structType(
				mkField("Intf", intType()),
				mkField("ArrStructf", listType(structfType))),
		},
		{
			"Array-of-struct-typed field with nullable struct elements.",
			struct {
				Intf       int
				ArrStructf []*structf
			}{
				10,
				[]*structf{(*structf)(nil), {1}},
			},
			listProto(
				intProto(10),
				listProto(
					nullProto(),
					listProto(intProto(1)))),
			structType(
				mkField("Intf", intType()),
				mkField("ArrStructf", listType(structfType))),
		},
	} {
		encodeStructValue(test, t)
	}
}

func TestEncodeStructValueStructFields(t *testing.T) {
	type structf struct {
		Intff int
	}
	structfType := structType(mkField("Intff", intType()))
	for _, test := range []encodeTest{
		{
			"Named struct-type field.",
			struct {
				Intf    int
				Structf structf
			}{10, structf{10}},
			listProto(intProto(10), listProto(intProto(10))),
			structType(
				mkField("Intf", intType()),
				mkField("Structf", structfType)),
		},
		{
			"Unnamed struct-type field.",
			struct {
				Intf    int
				Structf structf `spanner:""`
			}{10, structf{10}},
			listProto(intProto(10), listProto(intProto(10))),
			structType(
				mkField("Intf", intType()),
				mkField("", structfType)),
		},
		{
			"Duplicate struct-typed field.",
			struct {
				Structf1 structf `spanner:""`
				Structf2 structf `spanner:""`
			}{structf{10}, structf{20}},
			listProto(listProto(intProto(10)), listProto(intProto(20))),
			structType(
				mkField("", structfType),
				mkField("", structfType)),
		},
		{
			"Null struct-typed field.",
			struct {
				Intf    int
				Structf *structf
			}{10, nil},
			listProto(intProto(10), nullProto()),
			structType(
				mkField("Intf", intType()),
				mkField("Structf", structfType)),
		},
		{
			"Empty struct-typed field.",
			struct {
				Intf    int
				Structf struct{}
			}{10, struct{}{}},
			listProto(intProto(10), listProto([]*proto3.Value{}...)),
			structType(
				mkField("Intf", intType()),
				mkField("Structf", structType([]*sppb.StructType_Field{}...))),
		},
	} {
		encodeStructValue(test, t)
	}
}

func TestEncodeStructValueFieldNames(t *testing.T) {
	type embedded struct {
		B bool
	}

	for _, test := range []encodeTest{
		{
			"Duplicate fields.",
			struct {
				Field1    int `spanner:"field"`
				DupField1 int `spanner:"field"`
			}{10, 20},
			listProto(intProto(10), intProto(20)),
			structType(
				mkField("field", intType()),
				mkField("field", intType())),
		},
		{
			"Duplicate Fields (different types).",
			struct {
				IntField    int    `spanner:"field"`
				StringField string `spanner:"field"`
			}{10, "abc"},
			listProto(intProto(10), stringProto("abc")),
			structType(
				mkField("field", intType()),
				mkField("field", stringType())),
		},
		{
			"Duplicate unnamed fields.",
			struct {
				Dup  int `spanner:""`
				Dup1 int `spanner:""`
			}{10, 20},
			listProto(intProto(10), intProto(20)),
			structType(
				mkField("", intType()),
				mkField("", intType())),
		},
		{
			"Named and unnamed fields.",
			struct {
				Field  string
				Field1 int    `spanner:""`
				Field2 string `spanner:"field"`
			}{"abc", 10, "def"},
			listProto(stringProto("abc"), intProto(10), stringProto("def")),
			structType(
				mkField("Field", stringType()),
				mkField("", intType()),
				mkField("field", stringType())),
		},
		{
			"Ignored unexported fields.",
			struct {
				Field  int
				field  bool
				Field1 string `spanner:"field"`
			}{10, false, "abc"},
			listProto(intProto(10), stringProto("abc")),
			structType(
				mkField("Field", intType()),
				mkField("field", stringType())),
		},
		{
			"Ignored unexported struct/slice fields.",
			struct {
				a      []*embedded
				b      []embedded
				c      embedded
				d      *embedded
				Field1 string `spanner:"field"`
			}{nil, nil, embedded{}, nil, "def"},
			listProto(stringProto("def")),
			structType(
				mkField("field", stringType())),
		},
	} {
		encodeStructValue(test, t)
	}
}

func TestEncodeStructValueBasicFields(t *testing.T) {
	type CustomString string
	type CustomBytes []byte
	type CustomInt64 int64
	type CustomBool bool
	type CustomFloat64 float64
	type CustomFloat32 float32
	type CustomTime time.Time
	type CustomDate civil.Date

	type CustomNullString NullString
	type CustomNullInt64 NullInt64
	type CustomNullBool NullBool
	type CustomNullFloat64 NullFloat64
	type CustomNullFloat32 NullFloat32
	type CustomNullTime NullTime
	type CustomNullDate NullDate

	sValue := "abc"
	iValue := int64(300)
	bValue := false
	fValue := 3.45
	f32Value := float32(3.14)
	tValue := t1
	dValue := d1

	StructTypeProto := structType(
		mkField("Stringf", stringType()),
		mkField("Intf", intType()),
		mkField("Boolf", boolType()),
		mkField("Floatf", floatType()),
		mkField("Float32f", float32Type()),
		mkField("Bytef", bytesType()),
		mkField("Timef", timeType()),
		mkField("Datef", dateType()))

	for _, test := range []encodeTest{
		{
			"Basic types.",
			struct {
				Stringf  string
				Intf     int
				Boolf    bool
				Floatf   float64
				Float32f float32
				Bytef    []byte
				Timef    time.Time
				Datef    civil.Date
			}{"abc", 300, false, 3.45, float32(3.14), []byte("foo"), t1, d1},
			listProto(
				stringProto("abc"),
				intProto(300),
				boolProto(false),
				floatProto(3.45),
				float32Proto(3.14),
				bytesProto([]byte("foo")),
				timeProto(t1),
				dateProto(d1)),
			StructTypeProto,
		},
		{
			"Pointers to basic types.",
			struct {
				Stringf  *string
				Intf     *int64
				Boolf    *bool
				Floatf   *float64
				Float32f *float32
				Bytef    []byte
				Timef    *time.Time
				Datef    *civil.Date
			}{&sValue, &iValue, &bValue, &fValue, &f32Value, []byte("foo"), &tValue, &dValue},
			listProto(
				stringProto("abc"),
				intProto(300),
				boolProto(false),
				floatProto(3.45),
				float32Proto(3.14),
				bytesProto([]byte("foo")),
				timeProto(t1),
				dateProto(d1)),
			StructTypeProto,
		},
		{
			"Pointers to basic types with null values.",
			struct {
				Stringf  *string
				Intf     *int64
				Boolf    *bool
				Floatf   *float64
				Float32f *float32
				Bytef    []byte
				Timef    *time.Time
				Datef    *civil.Date
			}{nil, nil, nil, nil, nil, nil, nil, nil},
			listProto(
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto()),
			StructTypeProto,
		},
		{
			"Basic custom types.",
			struct {
				Stringf  CustomString
				Intf     CustomInt64
				Boolf    CustomBool
				Floatf   CustomFloat64
				Float32f CustomFloat32
				Bytef    CustomBytes
				Timef    CustomTime
				Datef    CustomDate
			}{"abc", 300, false, 3.45, CustomFloat32(3.14), []byte("foo"), CustomTime(t1), CustomDate(d1)},
			listProto(
				stringProto("abc"),
				intProto(300),
				boolProto(false),
				floatProto(3.45),
				float32Proto(3.14),
				bytesProto([]byte("foo")),
				timeProto(t1),
				dateProto(d1)),
			StructTypeProto,
		},
		{
			"Basic types null values.",
			struct {
				Stringf  NullString
				Intf     NullInt64
				Boolf    NullBool
				Floatf   NullFloat64
				Float32f NullFloat32
				Bytef    []byte
				Timef    NullTime
				Datef    NullDate
			}{
				NullString{"abc", false},
				NullInt64{4, false},
				NullBool{false, false},
				NullFloat64{5.6, false},
				NullFloat32{3.14, false},
				nil,
				NullTime{t1, false},
				NullDate{d1, false},
			},
			listProto(
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto()),
			StructTypeProto,
		},
		{
			"Basic custom types null values.",
			struct {
				Stringf  CustomNullString
				Intf     CustomNullInt64
				Boolf    CustomNullBool
				Floatf   CustomNullFloat64
				Float32f CustomNullFloat32
				Bytef    CustomBytes
				Timef    CustomNullTime
				Datef    CustomNullDate
			}{
				CustomNullString{"abc", false},
				CustomNullInt64{4, false},
				CustomNullBool{false, false},
				CustomNullFloat64{5.6, false},
				CustomNullFloat32{3.14, false},
				nil,
				CustomNullTime{t1, false},
				CustomNullDate{d1, false},
			},
			listProto(
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto()),
			StructTypeProto,
		},
	} {
		encodeStructValue(test, t)
	}
}

func TestEncodeStructValueArrayFields(t *testing.T) {
	type CustomString string
	type CustomBytes []byte
	type CustomInt64 int64
	type CustomBool bool
	type CustomFloat64 float64
	type CustomTime time.Time
	type CustomDate civil.Date

	type CustomNullString NullString
	type CustomNullInt64 NullInt64
	type CustomNullBool NullBool
	type CustomNullFloat64 NullFloat64
	type CustomNullTime NullTime
	type CustomNullDate NullDate

	sValue := "def"
	var sNilPtr *string
	iValue := int64(68)
	var iNilPtr *int64
	bValue := true
	var bNilPtr *bool
	fValue := 3.14
	var fNilPtr *float64
	tValue := t1
	var tNilPtr *time.Time
	dValue := d1
	var dNilPtr *civil.Date

	StructTypeProto := structType(
		mkField("Stringf", listType(stringType())),
		mkField("Intf", listType(intType())),
		mkField("Int64f", listType(intType())),
		mkField("Boolf", listType(boolType())),
		mkField("Floatf", listType(floatType())),
		mkField("Bytef", listType(bytesType())),
		mkField("Timef", listType(timeType())),
		mkField("Datef", listType(dateType())))

	for _, test := range []encodeTest{
		{
			"Arrays of basic types with non-nullable elements",
			struct {
				Stringf []string
				Intf    []int
				Int64f  []int64
				Boolf   []bool
				Floatf  []float64
				Bytef   [][]byte
				Timef   []time.Time
				Datef   []civil.Date
			}{
				[]string{"abc", "def"},
				[]int{4, 67},
				[]int64{5, 68},
				[]bool{false, true},
				[]float64{3.45, 0.93},
				[][]byte{[]byte("foo"), nil},
				[]time.Time{t1, t2},
				[]civil.Date{d1, d2},
			},
			listProto(
				listProto(stringProto("abc"), stringProto("def")),
				listProto(intProto(4), intProto(67)),
				listProto(intProto(5), intProto(68)),
				listProto(boolProto(false), boolProto(true)),
				listProto(floatProto(3.45), floatProto(0.93)),
				listProto(bytesProto([]byte("foo")), nullProto()),
				listProto(timeProto(t1), timeProto(t2)),
				listProto(dateProto(d1), dateProto(d2))),
			StructTypeProto,
		},
		{
			"Arrays of basic custom types with non-nullable elements",
			struct {
				Stringf []CustomString
				Intf    []CustomInt64
				Int64f  []CustomInt64
				Boolf   []CustomBool
				Floatf  []CustomFloat64
				Bytef   []CustomBytes
				Timef   []CustomTime
				Datef   []CustomDate
			}{
				[]CustomString{"abc", "def"},
				[]CustomInt64{4, 67},
				[]CustomInt64{5, 68},
				[]CustomBool{false, true},
				[]CustomFloat64{3.45, 0.93},
				[]CustomBytes{[]byte("foo"), nil},
				[]CustomTime{CustomTime(t1), CustomTime(t2)},
				[]CustomDate{CustomDate(d1), CustomDate(d2)},
			},
			listProto(
				listProto(stringProto("abc"), stringProto("def")),
				listProto(intProto(4), intProto(67)),
				listProto(intProto(5), intProto(68)),
				listProto(boolProto(false), boolProto(true)),
				listProto(floatProto(3.45), floatProto(0.93)),
				listProto(bytesProto([]byte("foo")), nullProto()),
				listProto(timeProto(t1), timeProto(t2)),
				listProto(dateProto(d1), dateProto(d2))),
			StructTypeProto,
		},
		{
			"Arrays of basic types with nullable elements.",
			struct {
				Stringf []NullString
				Intf    []NullInt64
				Int64f  []NullInt64
				Boolf   []NullBool
				Floatf  []NullFloat64
				Bytef   [][]byte
				Timef   []NullTime
				Datef   []NullDate
			}{
				[]NullString{{"abc", false}, {"def", true}},
				[]NullInt64{{4, false}, {67, true}},
				[]NullInt64{{5, false}, {68, true}},
				[]NullBool{{true, false}, {false, true}},
				[]NullFloat64{{3.45, false}, {0.93, true}},
				[][]byte{[]byte("foo"), nil},
				[]NullTime{{t1, false}, {t2, true}},
				[]NullDate{{d1, false}, {d2, true}},
			},
			listProto(
				listProto(nullProto(), stringProto("def")),
				listProto(nullProto(), intProto(67)),
				listProto(nullProto(), intProto(68)),
				listProto(nullProto(), boolProto(false)),
				listProto(nullProto(), floatProto(0.93)),
				listProto(bytesProto([]byte("foo")), nullProto()),
				listProto(nullProto(), timeProto(t2)),
				listProto(nullProto(), dateProto(d2))),
			StructTypeProto,
		},
		{
			"Arrays of pointers to basic types with nullable elements.",
			struct {
				Stringf []*string
				Intf    []*int64
				Int64f  []*int64
				Boolf   []*bool
				Floatf  []*float64
				Bytef   [][]byte
				Timef   []*time.Time
				Datef   []*civil.Date
			}{
				[]*string{sNilPtr, &sValue},
				[]*int64{iNilPtr, &iValue},
				[]*int64{iNilPtr, &iValue},
				[]*bool{bNilPtr, &bValue},
				[]*float64{fNilPtr, &fValue},
				[][]byte{[]byte("foo"), nil},
				[]*time.Time{tNilPtr, &tValue},
				[]*civil.Date{dNilPtr, &dValue},
			},
			listProto(
				listProto(nullProto(), stringProto("def")),
				listProto(nullProto(), intProto(68)),
				listProto(nullProto(), intProto(68)),
				listProto(nullProto(), boolProto(true)),
				listProto(nullProto(), floatProto(3.14)),
				listProto(bytesProto([]byte("foo")), nullProto()),
				listProto(nullProto(), timeProto(t1)),
				listProto(nullProto(), dateProto(d1))),
			StructTypeProto,
		},
		{
			"Arrays of basic custom types with nullable elements.",
			struct {
				Stringf []CustomNullString
				Intf    []CustomNullInt64
				Int64f  []CustomNullInt64
				Boolf   []CustomNullBool
				Floatf  []CustomNullFloat64
				Bytef   []CustomBytes
				Timef   []CustomNullTime
				Datef   []CustomNullDate
			}{
				[]CustomNullString{{"abc", false}, {"def", true}},
				[]CustomNullInt64{{4, false}, {67, true}},
				[]CustomNullInt64{{5, false}, {68, true}},
				[]CustomNullBool{{true, false}, {false, true}},
				[]CustomNullFloat64{{3.45, false}, {0.93, true}},
				[]CustomBytes{[]byte("foo"), nil},
				[]CustomNullTime{{t1, false}, {t2, true}},
				[]CustomNullDate{{d1, false}, {d2, true}},
			},
			listProto(
				listProto(nullProto(), stringProto("def")),
				listProto(nullProto(), intProto(67)),
				listProto(nullProto(), intProto(68)),
				listProto(nullProto(), boolProto(false)),
				listProto(nullProto(), floatProto(0.93)),
				listProto(bytesProto([]byte("foo")), nullProto()),
				listProto(nullProto(), timeProto(t2)),
				listProto(nullProto(), dateProto(d2))),
			StructTypeProto,
		},
		{
			"Null arrays of basic types.",
			struct {
				Stringf []NullString
				Intf    []NullInt64
				Int64f  []NullInt64
				Boolf   []NullBool
				Floatf  []NullFloat64
				Bytef   [][]byte
				Timef   []NullTime
				Datef   []NullDate
			}{
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
			},
			listProto(
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto()),
			StructTypeProto,
		},
		{
			"Null arrays of basic custom types.",
			struct {
				Stringf []CustomNullString
				Intf    []CustomNullInt64
				Int64f  []CustomNullInt64
				Boolf   []CustomNullBool
				Floatf  []CustomNullFloat64
				Bytef   []CustomBytes
				Timef   []CustomNullTime
				Datef   []CustomNullDate
			}{
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
			},
			listProto(
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto()),
			StructTypeProto,
		},
	} {
		encodeStructValue(test, t)
	}
}

// Test decoding Values.
func TestDecodeValue(t *testing.T) {
	type CustomString string
	type CustomBytes []byte
	type CustomInt64 int64
	type CustomBool bool
	type CustomFloat64 float64
	type CustomTime time.Time
	type CustomDate civil.Date
	type CustomNumeric big.Rat

	type CustomNullString NullString
	type CustomNullInt64 NullInt64
	type CustomNullBool NullBool
	type CustomNullFloat64 NullFloat64
	type CustomNullTime NullTime
	type CustomNullDate NullDate
	type CustomNullNumeric NullNumeric
	type CustomNullJSON NullJSON
	type CustomPGNumeric PGNumeric

	jsonStr := `{"Name":"Alice","Body":"Hello","Time":1294706395881547000}`
	var unmarshalledJSONStruct interface{}
	json.Unmarshal([]byte(jsonStr), &unmarshalledJSONStruct)
	invalidJSONStr := `{wrong_json_string}`
	emptyArrayJSONStr := `[]`
	var unmarshalledEmptyJSONArray interface{}
	json.Unmarshal([]byte(emptyArrayJSONStr), &unmarshalledEmptyJSONArray)
	nullValueJSONStr := `{"Key":null}`
	var unmarshalledStructWithNull interface{}
	json.Unmarshal([]byte(nullValueJSONStr), &unmarshalledStructWithNull)
	arrayJSONStr := `[{"Name":"Alice","Body":"Hello","Time":1294706395881547000},null,true]`
	var unmarshalledJSONArray interface{}
	json.Unmarshal([]byte(arrayJSONStr), &unmarshalledJSONArray)

	// Pointer values.
	sValue := "abc"
	var sNilPtr *string
	s2Value := "bcd"

	iValue := int64(15)
	var iNilPtr *int64
	i1Value := int64(91)
	i2Value := int64(87)

	bValue := true
	var bNilPtr *bool
	b2Value := false

	fValue := 3.14
	var fNilPtr *float64
	f2Value := 6.626

	f32Value := float32(3.14)
	var f32NilPtr *float32
	f32Value2 := float32(6.626)

	numValuePtr := big.NewRat(12345, 1e3)
	var numNilPtr *big.Rat
	num2ValuePtr := big.NewRat(12345, 1e4)

	tValue := t1
	var tNilPtr *time.Time
	t2Value := t2

	dValue := d1
	var dNilPtr *civil.Date
	d2Value := d2

	singerEnumValue := pb.Genre_ROCK
	singerProtoMsg := pb.SingerInfo{
		SingerId:    proto.Int64(1),
		BirthDate:   proto.String("January"),
		Nationality: proto.String("Country1"),
		Genre:       &singerEnumValue,
	}
	singer2ProtoEnum := pb.Genre_FOLK
	singer2ProtoMsg := pb.SingerInfo{
		SingerId:    proto.Int64(2),
		BirthDate:   proto.String("February"),
		Nationality: proto.String("Country2"),
		Genre:       &singer2ProtoEnum,
	}
	protoMessagefqn := "examples.spanner.music.SingerInfo"
	protoEnumfqn := "examples.spanner.music.Genre"

	for _, test := range []struct {
		desc      string
		proto     *proto3.Value
		protoType *sppb.Type
		want      interface{}
		wantErr   bool
	}{
		// STRING
		{desc: "decode STRING to string", proto: stringProto("abc"), protoType: stringType(), want: "abc"},
		{desc: "decode NULL to string", proto: nullProto(), protoType: stringType(), want: "abc", wantErr: true},
		{desc: "decode STRING to *string", proto: stringProto("abc"), protoType: stringType(), want: &sValue},
		{desc: "decode NULL to *string", proto: nullProto(), protoType: stringType(), want: sNilPtr},
		{desc: "decode STRING to NullString", proto: stringProto("abc"), protoType: stringType(), want: NullString{"abc", true}},
		{desc: "decode NULL to NullString", proto: nullProto(), protoType: stringType(), want: NullString{}},
		// STRING ARRAY with []NullString
		{desc: "decode ARRAY<STRING> to []NullString", proto: listProto(stringProto("abc"), nullProto(), stringProto("bcd")), protoType: listType(stringType()), want: []NullString{{"abc", true}, {}, {"bcd", true}}},
		{desc: "decode NULL to []NullString", proto: nullProto(), protoType: listType(stringType()), want: []NullString(nil)},
		// STRING ARRAY with []string
		{desc: "decode ARRAY<STRING> to []string", proto: listProto(stringProto("abc"), stringProto("bcd")), protoType: listType(stringType()), want: []string{"abc", "bcd"}},
		// STRING ARRAY with []*string
		{desc: "decode ARRAY<STRING> to []*string", proto: listProto(stringProto("abc"), nullProto(), stringProto("bcd")), protoType: listType(stringType()), want: []*string{&sValue, sNilPtr, &s2Value}},
		{desc: "decode NULL to []*string", proto: nullProto(), protoType: listType(stringType()), want: []*string(nil)},
		// BYTES
		{desc: "decode BYTES to []byte", proto: bytesProto([]byte("ab")), protoType: bytesType(), want: []byte("ab")},
		{desc: "decode NULL to []byte", proto: nullProto(), protoType: bytesType(), want: []byte(nil)},
		// BYTES ARRAY
		{desc: "decode ARRAY<BYTES> to [][]byte", proto: listProto(bytesProto([]byte("ab")), nullProto()), protoType: listType(bytesType()), want: [][]byte{[]byte("ab"), nil}},
		{desc: "decode NULL to [][]byte", proto: nullProto(), protoType: listType(bytesType()), want: [][]byte(nil)},
		//INT64
		{desc: "decode INT64 to int64", proto: intProto(15), protoType: intType(), want: int64(15)},
		{desc: "decode NULL to int64", proto: nullProto(), protoType: intType(), want: int64(0), wantErr: true},
		{desc: "decode INT64 to *int64", proto: intProto(15), protoType: intType(), want: &iValue},
		{desc: "decode NULL to *int64", proto: nullProto(), protoType: intType(), want: iNilPtr},
		{desc: "decode INT64 to NullInt64", proto: intProto(15), protoType: intType(), want: NullInt64{15, true}},
		{desc: "decode NULL to NullInt64", proto: nullProto(), protoType: intType(), want: NullInt64{}},
		// INT64 ARRAY with []NullInt64
		{desc: "decode ARRAY<INT64> to []NullInt64", proto: listProto(intProto(91), nullProto(), intProto(87)), protoType: listType(intType()), want: []NullInt64{{91, true}, {}, {87, true}}},
		{desc: "decode NULL to []NullInt64", proto: nullProto(), protoType: listType(intType()), want: []NullInt64(nil)},
		// INT64 ARRAY with []int64
		{desc: "decode ARRAY<INT64> to []int64", proto: listProto(intProto(91), intProto(87)), protoType: listType(intType()), want: []int64{91, 87}},
		// INT64 ARRAY with []*int64
		{desc: "decode ARRAY<INT64> to []*int64", proto: listProto(intProto(91), nullProto(), intProto(87)), protoType: listType(intType()), want: []*int64{&i1Value, nil, &i2Value}},
		{desc: "decode NULL to []*int64", proto: nullProto(), protoType: listType(intType()), want: []*int64(nil)},
		// BOOL
		{desc: "decode BOOL to bool", proto: boolProto(true), protoType: boolType(), want: true},
		{desc: "decode NULL to bool", proto: nullProto(), protoType: boolType(), want: true, wantErr: true},
		{desc: "decode BOOL to *bool", proto: boolProto(true), protoType: boolType(), want: &bValue},
		{desc: "decode NULL to *bool", proto: nullProto(), protoType: boolType(), want: bNilPtr},
		{desc: "decode BOOL to NullBool", proto: boolProto(true), protoType: boolType(), want: NullBool{true, true}},
		{desc: "decode BOOL to NullBool", proto: nullProto(), protoType: boolType(), want: NullBool{}},
		// BOOL ARRAY with []NullBool
		{desc: "decode ARRAY<BOOL> to []NullBool", proto: listProto(boolProto(true), boolProto(false), nullProto()), protoType: listType(boolType()), want: []NullBool{{true, true}, {false, true}, {}}},
		{desc: "decode NULL to []NullBool", proto: nullProto(), protoType: listType(boolType()), want: []NullBool(nil)},
		// BOOL ARRAY with []bool
		{desc: "decode ARRAY<BOOL> to []bool", proto: listProto(boolProto(true), boolProto(false)), protoType: listType(boolType()), want: []bool{true, false}},
		// BOOL ARRAY with []*bool
		{desc: "decode ARRAY<BOOL> to []*bool", proto: listProto(boolProto(true), nullProto(), boolProto(false)), protoType: listType(boolType()), want: []*bool{&bValue, bNilPtr, &b2Value}},
		{desc: "decode NULL to []*bool", proto: nullProto(), protoType: listType(boolType()), want: []*bool(nil)},
		// FLOAT64
		{desc: "decode FLOAT64 to float64", proto: floatProto(3.14), protoType: floatType(), want: 3.14},
		{desc: "decode NULL to float64", proto: nullProto(), protoType: floatType(), want: 0.00, wantErr: true},
		{desc: "decode FLOAT64 to *float64", proto: floatProto(3.14), protoType: floatType(), want: &fValue},
		{desc: "decode NULL to *float64", proto: nullProto(), protoType: floatType(), want: fNilPtr},
		{desc: "decode FLOAT64 to NullFloat64", proto: floatProto(3.14), protoType: floatType(), want: NullFloat64{3.14, true}},
		{desc: "decode NULL to NullFloat64", proto: nullProto(), protoType: floatType(), want: NullFloat64{}},
		// FLOAT64 ARRAY with []NullFloat64
		{desc: "decode ARRAY<FLOAT64> to []NullFloat64", proto: listProto(floatProto(math.Inf(1)), floatProto(math.Inf(-1)), nullProto(), floatProto(3.1)), protoType: listType(floatType()), want: []NullFloat64{{math.Inf(1), true}, {math.Inf(-1), true}, {}, {3.1, true}}},
		{desc: "decode NULL to []NullFloat64", proto: nullProto(), protoType: listType(floatType()), want: []NullFloat64(nil)},
		// FLOAT64 ARRAY with []float64
		{desc: "decode ARRAY<FLOAT64> to []float64", proto: listProto(floatProto(math.Inf(1)), floatProto(math.Inf(-1)), floatProto(3.1)), protoType: listType(floatType()), want: []float64{math.Inf(1), math.Inf(-1), 3.1}},
		// FLOAT64 ARRAY with []*float64
		{desc: "decode ARRAY<FLOAT64> to []*float64", proto: listProto(floatProto(fValue), nullProto(), floatProto(f2Value)), protoType: listType(floatType()), want: []*float64{&fValue, nil, &f2Value}},
		{desc: "decode NULL to []*float64", proto: nullProto(), protoType: listType(floatType()), want: []*float64(nil)},
		// FLOAT32
		{desc: "decode FLOAT32 to float32", proto: float32Proto(3.14), protoType: float32Type(), want: float32(3.14)},
		{desc: "decode NULL to float32", proto: nullProto(), protoType: float32Type(), want: 0.00, wantErr: true},
		{desc: "decode FLOAT32 to *float32", proto: float32Proto(3.14), protoType: float32Type(), want: &f32Value},
		{desc: "decode NULL to *float32", proto: nullProto(), protoType: float32Type(), want: f32NilPtr},
		{desc: "decode FLOAT32 to NullFloat32", proto: float32Proto(3.14), protoType: float32Type(), want: NullFloat32{3.14, true}},
		{desc: "decode NULL to NullFloat32", proto: nullProto(), protoType: float32Type(), want: NullFloat32{}},
		// FLOAT64 ARRAY with []NullFloat32
		{desc: "decode ARRAY<FLOAT32> to []NullFloat32", proto: listProto(float32Proto(float32(math.Inf(1))), float32Proto(float32(math.Inf(-1))), nullProto(), float32Proto(3.1)), protoType: listType(float32Type()), want: []NullFloat32{{float32(math.Inf(1)), true}, {float32(math.Inf(-1)), true}, {}, {3.1, true}}},
		{desc: "decode NULL to []NullFloat32", proto: nullProto(), protoType: listType(float32Type()), want: []NullFloat32(nil)},
		// FLOAT32 ARRAY with []float32
		{desc: "decode ARRAY<FLOAT32> to []float32", proto: listProto(float32Proto(float32(math.Inf(1))), float32Proto(float32(math.Inf(-1))), float32Proto(3.1)), protoType: listType(float32Type()), want: []float32{float32(math.Inf(1)), float32(math.Inf(-1)), 3.1}},
		// FLOAT64 ARRAY with []*float32
		{desc: "decode ARRAY<FLOAT32> to []*float32", proto: listProto(float32Proto(f32Value), nullProto(), float32Proto(f32Value2)), protoType: listType(float32Type()), want: []*float32{&f32Value, nil, &f32Value2}},
		{desc: "decode NULL to []*float32", proto: nullProto(), protoType: listType(float32Type()), want: []*float32(nil)},
		// NUMERIC
		{desc: "decode NUMERIC to big.Rat", proto: numericProto(numValuePtr), protoType: numericType(), want: *numValuePtr},
		{desc: "decode NUMERIC to NullNumeric", proto: numericProto(numValuePtr), protoType: numericType(), want: NullNumeric{*numValuePtr, true}},
		{desc: "decode NULL to NullNumeric", proto: nullProto(), protoType: numericType(), want: NullNumeric{}},
		{desc: "decode NUMERIC to *big.Rat", proto: numericProto(numValuePtr), protoType: numericType(), want: numValuePtr},
		{desc: "decode NULL to *big.Rat", proto: nullProto(), protoType: numericType(), want: numNilPtr},
		// NUMERIC ARRAY with []NullNumeric
		{desc: "decode ARRAY<Numeric> to []NullNumeric", proto: listProto(numericProto(numValuePtr), numericProto(num2ValuePtr), nullProto()), protoType: listType(numericType()), want: []NullNumeric{{*numValuePtr, true}, {*num2ValuePtr, true}, {}}},
		{desc: "decode NULL to []NullNumeric", proto: nullProto(), protoType: listType(numericType()), want: []NullNumeric(nil)},
		// NUMERIC ARRAY with []big.Rat
		{desc: "decode ARRAY<NUMERIC> to []big.Rat", proto: listProto(numericProto(numValuePtr), numericProto(num2ValuePtr)), protoType: listType(numericType()), want: []big.Rat{*numValuePtr, *num2ValuePtr}},
		// NUMERIC ARRAY with []*big.Rat
		{desc: "decode ARRAY<NUMERIC> to []*big.Rat", proto: listProto(numericProto(numValuePtr), nullProto(), numericProto(num2ValuePtr)), protoType: listType(numericType()), want: []*big.Rat{numValuePtr, nil, num2ValuePtr}},
		{desc: "decode NULL to []*big.Rat", proto: nullProto(), protoType: listType(numericType()), want: []*big.Rat(nil)},
		// JSON
		{desc: "decode json to NullJSON", proto: stringProto(jsonStr), protoType: jsonType(), want: NullJSON{unmarshalledJSONStruct, true}},
		{desc: "decode NULL to NullJSON", proto: nullProto(), protoType: jsonType(), want: NullJSON{}},
		{desc: "decode an invalid json string", proto: stringProto(invalidJSONStr), protoType: jsonType(), want: NullJSON{}, wantErr: true},
		{desc: "decode a json string with empty array to a NullJSON", proto: stringProto(emptyArrayJSONStr), protoType: jsonType(), want: NullJSON{unmarshalledEmptyJSONArray, true}},
		{desc: "decode a json string with null to a NullJSON", proto: stringProto(nullValueJSONStr), protoType: jsonType(), want: NullJSON{unmarshalledStructWithNull, true}},
		// JSON ARRAY with []NullJSON
		{desc: "decode ARRAY<JSON> to []NullJSON", proto: listProto(stringProto(jsonStr), stringProto(jsonStr), nullProto()), protoType: listType(jsonType()), want: []NullJSON{{unmarshalledJSONStruct, true}, {unmarshalledJSONStruct, true}, {}}},
		{desc: "decode ARRAY<JSON> to NullJSON", proto: listProto(stringProto(jsonStr), nullProto(), stringProto("true")), protoType: listType(jsonType()), want: NullJSON{unmarshalledJSONArray, true}},
		{desc: "decode NULL to []NullJSON", proto: nullProto(), protoType: listType(jsonType()), want: []NullJSON(nil)},
		// PG NUMERIC
		{desc: "decode PG NUMERIC to PGNumeric", proto: stringProto("123.456"), protoType: pgNumericType(), want: PGNumeric{"123.456", true}},
		{desc: "decode NaN to PGNumeric", proto: stringProto("NaN"), protoType: pgNumericType(), want: PGNumeric{"NaN", true}},
		{desc: "decode NULL to PGNumeric", proto: nullProto(), protoType: pgNumericType(), want: PGNumeric{}},
		// PG NUMERIC ARRAY with []PGNumeric
		{desc: "decode ARRAY<PG Numeric> to []PGNumeric", proto: listProto(stringProto("123.456"), stringProto("NaN"), nullProto()), protoType: listType(pgNumericType()), want: []PGNumeric{{"123.456", true}, {"NaN", true}, {}}},
		{desc: "decode NULL to []PGNumeric", proto: nullProto(), protoType: listType(pgNumericType()), want: []PGNumeric(nil)},
		// PG OID
		{desc: "decode PG OID to int64", proto: intProto(15), protoType: pgOidType(), want: int64(15)},
		{desc: "decode PG OID NULL to int64", proto: nullProto(), protoType: pgOidType(), want: int64(0), wantErr: true},
		{desc: "decode PG OID to *int64", proto: intProto(15), protoType: pgOidType(), want: &iValue},
		{desc: "decode PG OID NULL to *int64", proto: nullProto(), protoType: pgOidType(), want: iNilPtr},
		{desc: "decode PG OID to NullInt64", proto: intProto(15), protoType: pgOidType(), want: NullInt64{15, true}},
		{desc: "decode PG OID NULL to NullInt64", proto: nullProto(), protoType: pgOidType(), want: NullInt64{}},
		// PG OID ARRAY with []NullInt64
		{desc: "decode ARRAY<PG OID> to []NullInt64", proto: listProto(intProto(91), nullProto(), intProto(87)), protoType: listType(pgOidType()), want: []NullInt64{{91, true}, {}, {87, true}}},
		{desc: "decode PG OID NULL to []NullInt64", proto: nullProto(), protoType: listType(pgOidType()), want: []NullInt64(nil)},
		// PG OID ARRAY with []int64
		{desc: "decode ARRAY<PG OID> to []int64", proto: listProto(intProto(91), intProto(87)), protoType: listType(pgOidType()), want: []int64{91, 87}},
		// PG OID ARRAY with []*int64
		{desc: "decode ARRAY<PG OID> to []*int64", proto: listProto(intProto(91), nullProto(), intProto(87)), protoType: listType(pgOidType()), want: []*int64{&i1Value, nil, &i2Value}},
		{desc: "decode PG OID NULL to []*int64", proto: nullProto(), protoType: listType(pgOidType()), want: []*int64(nil)},
		// TIMESTAMP
		{desc: "decode TIMESTAMP to time.Time", proto: timeProto(t1), protoType: timeType(), want: t1},
		{desc: "decode TIMESTAMP to NullTime", proto: timeProto(t1), protoType: timeType(), want: NullTime{t1, true}},
		{desc: "decode NULL to NullTime", proto: nullProto(), protoType: timeType(), want: NullTime{}},
		{desc: "decode TIMESTAMP to *time.Time", proto: timeProto(t1), protoType: timeType(), want: &tValue},
		{desc: "decode NULL to *time.Time", proto: nullProto(), protoType: timeType(), want: tNilPtr},
		{desc: "decode INT64 to time.Time", proto: intProto(7), protoType: timeType(), want: time.Time{}, wantErr: true},
		// TIMESTAMP ARRAY with []NullTime
		{desc: "decode ARRAY<TIMESTAMP> to []NullTime", proto: listProto(timeProto(t1), timeProto(t2), timeProto(t3), nullProto()), protoType: listType(timeType()), want: []NullTime{{t1, true}, {t2, true}, {t3, true}, {}}},
		{desc: "decode NULL to []NullTime", proto: nullProto(), protoType: listType(timeType()), want: []NullTime(nil)},
		// TIMESTAMP ARRAY with []time.Time
		{desc: "decode ARRAY<TIMESTAMP> to []time.Time", proto: listProto(timeProto(t1), timeProto(t2), timeProto(t3)), protoType: listType(timeType()), want: []time.Time{t1, t2, t3}},
		// TIMESTAMP ARRAY with []*time.Time
		{desc: "decode ARRAY<TIMESTAMP> to []*time.Time", proto: listProto(timeProto(t1), nullProto(), timeProto(t2)), protoType: listType(timeType()), want: []*time.Time{&tValue, nil, &t2Value}},
		{desc: "decode NULL to []*time.Time", proto: nullProto(), protoType: listType(timeType()), want: []*time.Time(nil)},
		// DATE
		{desc: "decode DATE to civil.Date", proto: dateProto(d1), protoType: dateType(), want: d1},
		{desc: "decode DATE to NullDate", proto: dateProto(d1), protoType: dateType(), want: NullDate{d1, true}},
		{desc: "decode NULL to NullDate", proto: nullProto(), protoType: dateType(), want: NullDate{}},
		{desc: "decode DATE to *civil.Date", proto: dateProto(d1), protoType: dateType(), want: &dValue},
		{desc: "decode NULL to *civil.Date", proto: nullProto(), protoType: dateType(), want: dNilPtr},
		// DATE ARRAY with []NullDate
		{desc: "decode ARRAY<DATE> to []NullDate", proto: listProto(dateProto(d1), dateProto(d2), nullProto()), protoType: listType(dateType()), want: []NullDate{{d1, true}, {d2, true}, {}}},
		{desc: "decode NULL to []NullDate", proto: nullProto(), protoType: listType(dateType()), want: []NullDate(nil)},
		// DATE ARRAY with []civil.Date
		{desc: "decode ARRAY<DATE> to []civil.Date", proto: listProto(dateProto(d1), dateProto(d2)), protoType: listType(dateType()), want: []civil.Date{d1, d2}},
		// DATE ARRAY with []NullDate
		{desc: "decode ARRAY<DATE> to []*civil.Date", proto: listProto(dateProto(d1), nullProto(), dateProto(d2)), protoType: listType(dateType()), want: []*civil.Date{&dValue, nil, &d2Value}},
		{desc: "decode NULL to []*civil.Date", proto: nullProto(), protoType: listType(dateType()), want: []*civil.Date(nil)},
		// STRUCT ARRAY
		// STRUCT schema is equal to the following Go struct:
		// type s struct {
		//     Col1 NullInt64
		//     Col2 []struct {
		//         SubCol1 float64
		//         SubCol2 string
		//     }
		// }
		{
			desc: "decode ARRAY<STRUCT> to []NullRow",
			proto: listProto(
				listProto(
					intProto(3),
					listProto(
						listProto(floatProto(3.14), stringProto("this")),
						listProto(floatProto(0.57), stringProto("siht")),
					),
				),
				listProto(
					nullProto(),
					nullProto(),
				),
				nullProto(),
			),
			protoType: listType(
				structType(
					mkField("Col1", intType()),
					mkField(
						"Col2",
						listType(
							structType(
								mkField("SubCol1", floatType()),
								mkField("SubCol2", stringType()),
							),
						),
					),
				),
			),
			want: []NullRow{
				{
					Row: Row{
						fields: []*sppb.StructType_Field{
							mkField("Col1", intType()),
							mkField(
								"Col2",
								listType(
									structType(
										mkField("SubCol1", floatType()),
										mkField("SubCol2", stringType()),
									),
								),
							),
						},
						vals: []*proto3.Value{
							intProto(3),
							listProto(
								listProto(floatProto(3.14), stringProto("this")),
								listProto(floatProto(0.57), stringProto("siht")),
							),
						},
					},
					Valid: true,
				},
				{
					Row: Row{
						fields: []*sppb.StructType_Field{
							mkField("Col1", intType()),
							mkField(
								"Col2",
								listType(
									structType(
										mkField("SubCol1", floatType()),
										mkField("SubCol2", stringType()),
									),
								),
							),
						},
						vals: []*proto3.Value{
							nullProto(),
							nullProto(),
						},
					},
					Valid: true,
				},
				{},
			},
		},
		{
			desc: "decode ARRAY<STRUCT> to []*struct",
			proto: listProto(
				listProto(
					intProto(3),
					listProto(
						listProto(floatProto(3.14), stringProto("this")),
						listProto(floatProto(0.57), stringProto("siht")),
					),
				),
				listProto(
					nullProto(),
					nullProto(),
				),
				nullProto(),
			),
			protoType: listType(
				structType(
					mkField("Col1", intType()),
					mkField(
						"Col2",
						listType(
							structType(
								mkField("SubCol1", floatType()),
								mkField("SubCol2", stringType()),
							),
						),
					),
				),
			),
			want: []*struct {
				Col1      NullInt64
				StructCol []*struct {
					SubCol1 NullFloat64
					SubCol2 string
				} `spanner:"Col2"`
			}{
				{
					Col1: NullInt64{3, true},
					StructCol: []*struct {
						SubCol1 NullFloat64
						SubCol2 string
					}{
						{
							SubCol1: NullFloat64{3.14, true},
							SubCol2: "this",
						},
						{
							SubCol1: NullFloat64{0.57, true},
							SubCol2: "siht",
						},
					},
				},
				{
					Col1: NullInt64{},
					StructCol: []*struct {
						SubCol1 NullFloat64
						SubCol2 string
					}(nil),
				},
				nil,
			},
		},
		// GenericColumnValue
		{desc: "decode STRING to GenericColumnValue", proto: stringProto("abc"), protoType: stringType(), want: GenericColumnValue{stringType(), stringProto("abc")}},
		{desc: "decode NULL to GenericColumnValue", proto: nullProto(), protoType: stringType(), want: GenericColumnValue{stringType(), nullProto()}},
		// not actually valid (stringProto inside int list), but demonstrates pass-through.
		{desc: "decode ARRAY<INT64> to GenericColumnValue", proto: listProto(intProto(5), nullProto(), stringProto("bcd")), protoType: listType(intType()), want: GenericColumnValue{Type: listType(intType()), Value: listProto(intProto(5), nullProto(), stringProto("bcd"))}},

		// Custom base types.
		{desc: "decode STRING to CustomString", proto: stringProto("bar"), protoType: stringType(), want: CustomString("bar")},
		{desc: "decode BYTES to CustomBytes", proto: bytesProto([]byte("ab")), protoType: bytesType(), want: CustomBytes("ab")},
		{desc: "decode INT64 to CustomInt64", proto: intProto(-100), protoType: intType(), want: CustomInt64(-100)},
		{desc: "decode BOOL to CustomBool", proto: boolProto(true), protoType: boolType(), want: CustomBool(true)},
		{desc: "decode FLOAT64 to CustomFloat64", proto: floatProto(6.626), protoType: floatType(), want: CustomFloat64(6.626)},
		{desc: "decode NUMERIC to CustomNumeric", proto: numericProto(numValuePtr), protoType: numericType(), want: CustomNumeric(*numValuePtr)},
		{desc: "decode TIMESTAMP to CustomTimestamp", proto: timeProto(t1), protoType: timeType(), want: CustomTime(t1)},
		{desc: "decode DATE to CustomDate", proto: dateProto(d1), protoType: dateType(), want: CustomDate(d1)},

		{desc: "decode NULL to CustomString", proto: nullProto(), protoType: stringType(), want: CustomString(""), wantErr: true},
		{desc: "decode NULL to CustomBytes", proto: nullProto(), protoType: bytesType(), want: CustomBytes(nil)},
		{desc: "decode NULL to CustomInt64", proto: nullProto(), protoType: intType(), want: CustomInt64(0), wantErr: true},
		{desc: "decode NULL to CustomBool", proto: nullProto(), protoType: boolType(), want: CustomBool(false), wantErr: true},
		{desc: "decode NULL to CustomFloat64", proto: nullProto(), protoType: floatType(), want: CustomFloat64(0), wantErr: true},
		{desc: "decode NULL to CustomNumeric", proto: nullProto(), protoType: numericType(), want: CustomNumeric{}, wantErr: true},
		{desc: "decode NULL to CustomTime", proto: nullProto(), protoType: timeType(), want: CustomTime{}, wantErr: true},
		{desc: "decode NULL to CustomDate", proto: nullProto(), protoType: dateType(), want: CustomDate{}, wantErr: true},

		{desc: "decode STRING to CustomNullString", proto: stringProto("bar"), protoType: stringType(), want: CustomNullString{"bar", true}},
		{desc: "decode INT64 to CustomNullInt64", proto: intProto(-100), protoType: intType(), want: CustomNullInt64{-100, true}},
		{desc: "decode BOOL to CustomNullBool", proto: boolProto(true), protoType: boolType(), want: CustomNullBool{true, true}},
		{desc: "decode FLOAT64 to CustomNullFloat64", proto: floatProto(6.626), protoType: floatType(), want: CustomNullFloat64{6.626, true}},
		{desc: "decode NUMERIC to CustomNullNumeric", proto: numericProto(numValuePtr), protoType: numericType(), want: CustomNullNumeric{*numValuePtr, true}},
		{desc: "decode JSON to CustomNullJSON", proto: stringProto(jsonStr), protoType: jsonType(), want: CustomNullJSON{unmarshalledJSONStruct, true}},
		{desc: "decode TIMESTAMP to CustomNullTime", proto: timeProto(t1), protoType: timeType(), want: CustomNullTime{t1, true}},
		{desc: "decode DATE to CustomNullDate", proto: dateProto(d1), protoType: dateType(), want: CustomNullDate{d1, true}},
		{desc: "decode PG NUMERIC to CustomPGNumeric", proto: stringProto("123.456"), protoType: pgNumericType(), want: CustomPGNumeric{"123.456", true}},
		{desc: "decode PG OID to CustomNullInt64", proto: intProto(-100), protoType: pgOidType(), want: CustomNullInt64{-100, true}},

		{desc: "decode NULL to CustomNullString", proto: nullProto(), protoType: stringType(), want: CustomNullString{}},
		{desc: "decode NULL to CustomNullInt64", proto: nullProto(), protoType: intType(), want: CustomNullInt64{}},
		{desc: "decode NULL to CustomNullBool", proto: nullProto(), protoType: boolType(), want: CustomNullBool{}},
		{desc: "decode NULL to CustomNullFloat64", proto: nullProto(), protoType: floatType(), want: CustomNullFloat64{}},
		{desc: "decode NULL to CustomNullNumeric", proto: nullProto(), protoType: numericType(), want: CustomNullNumeric{}},
		{desc: "decode NULL to CustomNullJSON", proto: nullProto(), protoType: jsonType(), want: CustomNullJSON{}},
		{desc: "decode NULL to CustomNullTime", proto: nullProto(), protoType: timeType(), want: CustomNullTime{}},
		{desc: "decode NULL to CustomNullDate", proto: nullProto(), protoType: dateType(), want: CustomNullDate{}},
		{desc: "decode NULL to CustomPGNumeric", proto: nullProto(), protoType: pgNumericType(), want: CustomPGNumeric{}},
		{desc: "decode PG OID NULL to CustomNullInt64", proto: nullProto(), protoType: pgOidType(), want: CustomNullInt64{}},

		// STRING ARRAY
		{desc: "decode NULL to []CustomString", proto: nullProto(), protoType: listType(stringType()), want: []CustomString(nil)},
		{desc: "decode ARRAY<STRING> to []CustomString", proto: listProto(stringProto("abc"), stringProto("bcd")), protoType: listType(stringType()), want: []CustomString{"abc", "bcd"}},
		{desc: "decode ARRAY<STRING> with NULL values to []CustomString", proto: listProto(stringProto("abc"), nullProto(), stringProto("bcd")), protoType: listType(stringType()), want: []CustomString{}, wantErr: true},
		{desc: "decode NULL to []CustomNullString", proto: nullProto(), protoType: listType(stringType()), want: []CustomNullString(nil)},
		{desc: "decode ARRAY<STRING> to []CustomNullString", proto: listProto(stringProto("abc"), nullProto(), stringProto("bcd")), protoType: listType(stringType()), want: []CustomNullString{{"abc", true}, {}, {"bcd", true}}},
		// BYTES ARRAY
		{desc: "decode NULL to []CustomBytes", proto: nullProto(), protoType: listType(bytesType()), want: []CustomBytes(nil)},
		{desc: "decode ARRAY<BYTES> to []CustomBytes", proto: listProto(bytesProto([]byte("abc")), nullProto(), bytesProto([]byte("bcd"))), protoType: listType(bytesType()), want: []CustomBytes{CustomBytes("abc"), CustomBytes(nil), CustomBytes("bcd")}},
		// INT64 ARRAY
		{desc: "decode NULL to []CustomInt64", proto: nullProto(), protoType: listType(intType()), want: []CustomInt64(nil)},
		{desc: "decode ARRAY<INT64> with NULL values to []CustomInt64", proto: listProto(intProto(-100), nullProto(), intProto(100)), protoType: listType(intType()), want: []CustomInt64{}, wantErr: true},
		{desc: "decode ARRAY<INT64> to []CustomInt64", proto: listProto(intProto(-100), intProto(100)), protoType: listType(intType()), want: []CustomInt64{-100, 100}},
		{desc: "decode NULL to []CustomNullInt64", proto: nullProto(), protoType: listType(intType()), want: []CustomNullInt64(nil)},
		{desc: "decode ARRAY<INT64> to []CustomNullInt64", proto: listProto(intProto(-100), nullProto(), intProto(100)), protoType: listType(intType()), want: []CustomNullInt64{{-100, true}, {}, {100, true}}},
		// BOOL ARRAY
		{desc: "decode NULL to []CustomBool", proto: nullProto(), protoType: listType(boolType()), want: []CustomBool(nil)},
		{desc: "decode ARRAY<BOOL> with NULL values to []CustomBool", proto: listProto(boolProto(false), nullProto(), boolProto(true)), protoType: listType(boolType()), want: []CustomBool{}, wantErr: true},
		{desc: "decode ARRAY<BOOL> to []CustomBool", proto: listProto(boolProto(false), boolProto(true)), protoType: listType(boolType()), want: []CustomBool{false, true}},
		{desc: "decode NULL to []CustomNullBool", proto: nullProto(), protoType: listType(boolType()), want: []CustomNullBool(nil)},
		{desc: "decode ARRAY<BOOL> to []CustomNullBool", proto: listProto(boolProto(false), nullProto(), boolProto(true)), protoType: listType(boolType()), want: []CustomNullBool{{false, true}, {}, {true, true}}},
		// FLOAT64 ARRAY
		{desc: "decode NULL to []CustomFloat64", proto: nullProto(), protoType: listType(floatType()), want: []CustomFloat64(nil)},
		{desc: "decode ARRAY<FLOAT64> with NULL values to []CustomFloat64", proto: listProto(floatProto(3.14), nullProto(), floatProto(6.626)), protoType: listType(floatType()), want: []CustomFloat64{}, wantErr: true},
		{desc: "decode ARRAY<FLOAT64> to []CustomFloat64", proto: listProto(floatProto(3.14), floatProto(6.626)), protoType: listType(floatType()), want: []CustomFloat64{3.14, 6.626}},
		{desc: "decode NULL to []CustomNullFloat64", proto: nullProto(), protoType: listType(floatType()), want: []CustomNullFloat64(nil)},
		{desc: "decode ARRAY<FLOAT64> to []CustomNullFloat64", proto: listProto(floatProto(3.14), nullProto(), floatProto(6.626)), protoType: listType(floatType()), want: []CustomNullFloat64{{3.14, true}, {}, {6.626, true}}},
		// NUMERIC ARRAY
		{desc: "decode NULL to []CustomNumeric", proto: nullProto(), protoType: listType(numericType()), want: []CustomNumeric(nil)},
		{desc: "decode ARRAY<NUMERIC> with NULL values to []CustomNumeric", proto: listProto(numericProto(numValuePtr), nullProto(), numericProto(num2ValuePtr)), protoType: listType(numericType()), want: []CustomNumeric{}, wantErr: true},
		{desc: "decode ARRAY<NUMERIC> to []CustomNumeric", proto: listProto(numericProto(numValuePtr), numericProto(num2ValuePtr)), protoType: listType(numericType()), want: []CustomNumeric{CustomNumeric(*numValuePtr), CustomNumeric(*num2ValuePtr)}},
		{desc: "decode NULL to []CustomNullNumeric", proto: nullProto(), protoType: listType(numericType()), want: []CustomNullNumeric(nil)},
		{desc: "decode ARRAY<NUMERIC> to []CustomNullNumeric", proto: listProto(numericProto(numValuePtr), nullProto(), numericProto(num2ValuePtr)), protoType: listType(numericType()), want: []CustomNullNumeric{{*numValuePtr, true}, {}, {*num2ValuePtr, true}}},
		// JSON ARRAY
		{desc: "decode NULL to []CustomNullJSON", proto: nullProto(), protoType: listType(jsonType()), want: []CustomNullJSON(nil)},
		{desc: "decode ARRAY<JSON> to []CustomNullJSON", proto: listProto(stringProto(jsonStr), stringProto(jsonStr), nullProto()), protoType: listType(jsonType()), want: []CustomNullJSON{{unmarshalledJSONStruct, true}, {unmarshalledJSONStruct, true}, {}}},
		// PG NUMERIC ARRAY
		{desc: "decode NULL to []CustomPGNumeric", proto: nullProto(), protoType: listType(pgNumericType()), want: []CustomPGNumeric(nil)},
		{desc: "decode ARRAY<PG NUMERIC> to []CustomPGNumeric", proto: listProto(stringProto("123.456"), nullProto(), stringProto("1.23456")), protoType: listType(pgNumericType()), want: []CustomPGNumeric{{"123.456", true}, {}, {"1.23456", true}}},
		// TIME ARRAY
		{desc: "decode NULL to []CustomTime", proto: nullProto(), protoType: listType(timeType()), want: []CustomTime(nil)},
		{desc: "decode ARRAY<TIMESTAMP> with NULL values to []CustomTime", proto: listProto(timeProto(t1), nullProto(), timeProto(t2)), protoType: listType(timeType()), want: []CustomTime{}, wantErr: true},
		{desc: "decode ARRAY<TIMESTAMP> to []CustomTime", proto: listProto(timeProto(t1), timeProto(t2)), protoType: listType(timeType()), want: []CustomTime{CustomTime(t1), CustomTime(t2)}},
		{desc: "decode NULL to []CustomNullTime", proto: nullProto(), protoType: listType(timeType()), want: []CustomNullTime(nil)},
		{desc: "decode ARRAY<TIMESTAMP> to []CustomNullTime", proto: listProto(timeProto(t1), nullProto(), timeProto(t2)), protoType: listType(timeType()), want: []CustomNullTime{{t1, true}, {}, {t2, true}}},
		// DATE ARRAY
		{desc: "decode NULL to []CustomDate", proto: nullProto(), protoType: listType(dateType()), want: []CustomDate(nil)},
		{desc: "decode ARRAY<DATE> with NULL values to []CustomDate", proto: listProto(dateProto(d1), nullProto(), dateProto(d2)), protoType: listType(dateType()), want: []CustomDate{}, wantErr: true},
		{desc: "decode ARRAY<DATE> to []CustomDate", proto: listProto(dateProto(d1), dateProto(d2)), protoType: listType(dateType()), want: []CustomDate{CustomDate(d1), CustomDate(d2)}},
		{desc: "decode NULL to []CustomNullDate", proto: nullProto(), protoType: listType(dateType()), want: []CustomNullDate(nil)},
		{desc: "decode ARRAY<DATE> to []CustomNullDate", proto: listProto(dateProto(d1), nullProto(), dateProto(d2)), protoType: listType(dateType()), want: []CustomNullDate{{d1, true}, {}, {d2, true}}},
		// CUSTOM STRUCT
		{desc: "decode STRING to CustomStructToString", proto: stringProto("A-B"), protoType: stringType(), want: customStructToString{"A", "B"}},
		{desc: "decode INT64 to CustomStructToInt", proto: intProto(123), protoType: intType(), want: customStructToInt{1, 23}},
		{desc: "decode FLOAT64 to CustomStructToFloat", proto: floatProto(123.123), protoType: floatType(), want: customStructToFloat{1.23, 12.3}},
		{desc: "decode BOOL to CustomStructToBool", proto: boolProto(true), protoType: boolType(), want: customStructToBool{true, false}},
		{desc: "decode BYTES to CustomStructToBytes", proto: bytesProto([]byte("AB")), protoType: bytesType(), want: customStructToBytes{[]byte("A"), []byte("B")}},
		{desc: "decode TIMESTAMP to CustomStructToTime", proto: timeProto(t1), protoType: timeType(), want: customStructToTime{"A", "B"}},
		{desc: "decode DATE to CustomStructToDate", proto: dateProto(d1), protoType: dateType(), want: customStructToDate{"A", "B"}},
		{desc: "decode NULL bool to CustomStructToNull", proto: nullProto(), protoType: boolType(), want: customStructToNull{}},
		{desc: "decode NULL float to CustomStructToNull", proto: nullProto(), protoType: floatType(), want: customStructToNull{}},
		{desc: "decode NULL string to CustomStructToNull", proto: nullProto(), protoType: stringType(), want: customStructToNull{}},
		{desc: "decode NULL array of bool to CustomStructToNull", proto: nullProto(), protoType: listType(boolType()), want: customStructToNull{}},
		{desc: "decode NULL array of float to CustomStructToNull", proto: nullProto(), protoType: listType(floatType()), want: customStructToNull{}},
		{desc: "decode NULL array of string to CustomStructToNull", proto: nullProto(), protoType: listType(stringType()), want: customStructToNull{}},
		// CUSTOM ARRAY
		{desc: "decode ARRAY<INT64> to CustomArray", proto: listProto(intProto(0), intProto(6), intProto(3), intProto(5)), protoType: listType(intType()), want: customArray([4]uint8{0, 6, 3, 5})},
		// PROTO MESSAGE AND PROTO ENUM
		{desc: "decode PROTO to proto.Message", proto: protoMessageProto(&singerProtoMsg), protoType: protoMessageType(protoMessagefqn),
			want: pb.SingerInfo{
				SingerId:    proto.Int64(1),
				BirthDate:   proto.String("January"),
				Nationality: proto.String("Country1"),
				Genre:       &singerEnumValue,
			},
		},
		{desc: "decode ENUM to protoreflect.Enum", proto: protoEnumProto(pb.Genre_ROCK), protoType: protoEnumType(protoEnumfqn), want: singerEnumValue},
		{desc: "decode PROTO to NullProto", proto: protoMessageProto(&singerProtoMsg), protoType: protoMessageType(protoMessagefqn), want: NullProtoMessage{&singerProtoMsg, true}},
		{desc: "decode PROTO to *pb.SingerInfo", proto: protoMessageProto(&singerProtoMsg), protoType: protoMessageType(protoMessagefqn),
			want: &pb.SingerInfo{
				SingerId:    proto.Int64(1),
				BirthDate:   proto.String("January"),
				Nationality: proto.String("Country1"),
				Genre:       &singerEnumValue,
			},
		},
		{desc: "decode NULL to NullProto", proto: nullProto(), protoType: protoMessageType(protoMessagefqn), want: NullProtoMessage{}},
		{desc: "decode ENUM to NullEnum", proto: protoEnumProto(pb.Genre_ROCK), protoType: protoEnumType(protoEnumfqn), want: NullProtoEnum{&singerEnumValue, true}},
		{desc: "decode NULL to NullEnum", proto: nullProto(), protoType: protoEnumType(protoEnumfqn), want: NullProtoEnum{}},
		// ARRAY OF PROTO MESSAGES AND PROTO ENUM
		{desc: "decode ARRAY<PROTO<>> to []*pb.SingerInfo", proto: listProto(protoMessageProto(&singerProtoMsg), protoMessageProto(&singer2ProtoMsg)), protoType: listType(protoMessageType(protoMessagefqn)), want: []*pb.SingerInfo{&singerProtoMsg, &singer2ProtoMsg}},
		{desc: "decode ARRAY<ENUM<>> to []*pb.Genre", proto: listProto(protoEnumProto(pb.Genre_ROCK), protoEnumProto(pb.Genre_FOLK)), protoType: listType(protoEnumType(protoEnumfqn)), want: []*pb.Genre{&singerEnumValue, &singer2ProtoEnum}},
		{desc: "decode ARRAY<ENUM<>> to []pb.Genre", proto: listProto(protoEnumProto(pb.Genre_ROCK), protoEnumProto(pb.Genre_FOLK)), protoType: listType(protoEnumType(protoEnumfqn)), want: []pb.Genre{singerEnumValue, singer2ProtoEnum}},
		{desc: "decode NULL to []*pb.SingerInfo", proto: nullProto(), protoType: listType(protoMessageType(protoMessagefqn)), want: []*pb.SingerInfo(nil)},
		{desc: "decode NULL to []*pb.Genre", proto: nullProto(), protoType: listType(protoEnumType(protoEnumfqn)), want: []*pb.Genre(nil)},
		{desc: "decode NULL to []pb.Genre", proto: nullProto(), protoType: listType(protoEnumType(protoEnumfqn)), want: []pb.Genre(nil)},
		{desc: "decode empty array to []*pb.SingerInfo", proto: listProto(), protoType: listType(protoMessageType(protoMessagefqn)), want: []*pb.SingerInfo{}},
		{desc: "decode empty array to []*pb.Genre", proto: listProto(), protoType: listType(protoEnumType(protoEnumfqn)), want: []*pb.Genre{}},
		{desc: "decode empty array to []pb.Genre", proto: listProto(), protoType: listType(protoEnumType(protoEnumfqn)), want: []pb.Genre{}},
		// Null elements in ARRAY OF PROTO MESSAGES AND PROTO ENUM
		{desc: "decode ARRAY<PROTO<>> to []*pb.SingerInfo", proto: listProto(nullProto(), protoMessageProto(&singerProtoMsg), protoMessageProto(&singer2ProtoMsg)), protoType: listType(protoMessageType(protoMessagefqn)), want: []*pb.SingerInfo{nil, &singerProtoMsg, &singer2ProtoMsg}},
		{desc: "decode all NULL elements in ARRAY<PROTO<>> to []*pb.SingerInfo", proto: listProto(nullProto(), nullProto()), protoType: listType(protoMessageType(protoMessagefqn)), want: []*pb.SingerInfo{nil, nil}},
		{desc: "decode ARRAY<ENUM<>> to []*pb.Genre", proto: listProto(nullProto(), protoEnumProto(pb.Genre_ROCK), protoEnumProto(pb.Genre_FOLK)), protoType: listType(protoEnumType(protoEnumfqn)), want: []*pb.Genre{nil, &singerEnumValue, &singer2ProtoEnum}},
		{desc: "decode all NULL elements in ARRAY<ENUM<>> to []*pb.Genre", proto: listProto(nullProto(), nullProto()), protoType: listType(protoEnumType(protoEnumfqn)), want: []*pb.Genre{nil, nil}},
		// PROTO MESSAGE WITH CUSTOM DECODER
		{desc: "decode STRING to Proto message", proto: stringProto("abc"), protoType: stringType(), want: pb.CustomSingerInfo{SingerName: proto.String("abc")}},
	} {
		gotp := reflect.New(reflect.TypeOf(test.want))
		v := gotp.Interface()
		// Initialize the input to a non-zero value to ensure that the decode
		// method will override this with the actual value, or a zero value in
		// case of a NULL.
		switch nullValue := v.(type) {
		case *NullString:
			nullValue.StringVal = "foo"
		case *NullInt64:
			nullValue.Int64 = -100
		case *NullFloat64:
			nullValue.Float64 = 3.14
		case *NullBool:
			nullValue.Bool = true
		case *NullTime:
			nullValue.Time = time.Unix(100, 100)
		case *NullDate:
			nullValue.Date = civil.DateOf(time.Unix(100, 200))
		case *NullProtoMessage:
			nullValue.ProtoMessageVal = &pb.SingerInfo{}
		case *NullProtoEnum:
			var singerProtoEnumDefault pb.Genre
			nullValue.ProtoEnumVal = &singerProtoEnumDefault
		default:
		}
		err := decodeValue(test.proto, test.protoType, v)
		if test.wantErr {
			if err == nil {
				t.Errorf("%s: missing expected decode failure for %v(%v)", test.desc, test.proto, test.protoType)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: cannot decode %v(%v): %v", test.desc, test.proto, test.protoType, err)
			continue
		}
		got := reflect.Indirect(gotp).Interface()
		switch v.(type) {
		case proto.Message:
			if diff := cmp.Diff(got, test.want, protocmp.Transform()); diff != "" {
				t.Errorf("unexpected difference in proto message :\n%v", diff)
			}
		default:
			if !testutil.Equal(got, test.want, cmp.AllowUnexported(CustomNumeric{}, CustomTime{}, CustomDate{}, Row{}, big.Rat{}, big.Int{}, customStructToNull{})) {
				t.Errorf("%s: unexpected decoding result - got %v (%T), want %v (%T)", test.desc, got, got, test.want, test.want)
			}
		}
	}
}

// Test error cases for decodeValue.
func TestDecodeValueErrors(t *testing.T) {
	var s string
	for i, test := range []struct {
		in *proto3.Value
		t  *sppb.Type
		v  interface{}
	}{
		{nullProto(), stringType(), nil},
		{nullProto(), stringType(), 1},
		{timeProto(t1), timeType(), &s},
	} {
		err := decodeValue(test.in, test.t, test.v)
		if err == nil {
			t.Errorf("#%d: want error, got nil", i)
		}
	}
}

func TestGetDecodableSpannerType(t *testing.T) {
	type CustomString string
	type CustomInt64 int64
	type CustomBool bool
	type CustomFloat64 float64
	type CustomFloat32 float32
	type CustomTime time.Time
	type CustomDate civil.Date
	type CustomNumeric big.Rat

	type CustomNullString NullString
	type CustomNullInt64 NullInt64
	type CustomNullBool NullBool
	type CustomNullFloat64 NullFloat64
	type CustomNullFloat32 NullFloat32
	type CustomNullTime NullTime
	type CustomNullDate NullDate
	type CustomNullNumeric NullNumeric

	type StringEmbedded struct {
		string
	}
	type NullStringEmbedded struct {
		NullString
	}

	for i, test := range []struct {
		in   interface{}
		want decodableSpannerType
	}{
		{"foo", spannerTypeNonNullString},
		{[]byte("ab"), spannerTypeByteArray},
		{[]byte(nil), spannerTypeByteArray},
		{int64(123), spannerTypeNonNullInt64},
		{true, spannerTypeNonNullBool},
		{3.14, spannerTypeNonNullFloat64},
		{float32(3.14), spannerTypeNonNullFloat32},
		{time.Now(), spannerTypeNonNullTime},
		{civil.DateOf(time.Now()), spannerTypeNonNullDate},
		{NullString{}, spannerTypeNullString},
		{NullInt64{}, spannerTypeNullInt64},
		{NullBool{}, spannerTypeNullBool},
		{NullFloat64{}, spannerTypeNullFloat64},
		{NullFloat32{}, spannerTypeNullFloat32},
		{NullTime{}, spannerTypeNullTime},
		{NullDate{}, spannerTypeNullDate},
		{*big.NewRat(1234, 1000), spannerTypeNonNullNumeric},
		{big.Rat{}, spannerTypeNonNullNumeric},
		{NullNumeric{}, spannerTypeNullNumeric},

		{[]string{"foo", "bar"}, spannerTypeArrayOfNonNullString},
		{[][]byte{{1, 2, 3}, {3, 2, 1}}, spannerTypeArrayOfByteArray},
		{[][]byte{}, spannerTypeArrayOfByteArray},
		{[]int64{int64(123)}, spannerTypeArrayOfNonNullInt64},
		{[]bool{true}, spannerTypeArrayOfNonNullBool},
		{[]float64{3.14}, spannerTypeArrayOfNonNullFloat64},
		{[]float32{3.14}, spannerTypeArrayOfNonNullFloat32},
		{[]time.Time{time.Now()}, spannerTypeArrayOfNonNullTime},
		{[]civil.Date{civil.DateOf(time.Now())}, spannerTypeArrayOfNonNullDate},
		{[]NullString{}, spannerTypeArrayOfNullString},
		{[]NullInt64{}, spannerTypeArrayOfNullInt64},
		{[]NullBool{}, spannerTypeArrayOfNullBool},
		{[]NullFloat64{}, spannerTypeArrayOfNullFloat64},
		{[]NullFloat32{}, spannerTypeArrayOfNullFloat32},
		{[]NullTime{}, spannerTypeArrayOfNullTime},
		{[]NullDate{}, spannerTypeArrayOfNullDate},
		{[]big.Rat{}, spannerTypeArrayOfNonNullNumeric},
		{[]big.Rat{*big.NewRat(1234, 1000), *big.NewRat(1234, 100)}, spannerTypeArrayOfNonNullNumeric},
		{[]NullNumeric{}, spannerTypeArrayOfNullNumeric},

		{CustomString("foo"), spannerTypeNonNullString},
		{CustomInt64(-100), spannerTypeNonNullInt64},
		{CustomBool(true), spannerTypeNonNullBool},
		{CustomFloat64(3.141592), spannerTypeNonNullFloat64},
		{CustomFloat32(3.141592), spannerTypeNonNullFloat32},
		{CustomTime(time.Now()), spannerTypeNonNullTime},
		{CustomDate(civil.DateOf(time.Now())), spannerTypeNonNullDate},
		{CustomNumeric(*big.NewRat(1234, 1000)), spannerTypeNonNullNumeric},

		{[]CustomString{}, spannerTypeArrayOfNonNullString},
		{[]CustomInt64{}, spannerTypeArrayOfNonNullInt64},
		{[]CustomBool{}, spannerTypeArrayOfNonNullBool},
		{[]CustomFloat64{}, spannerTypeArrayOfNonNullFloat64},
		{[]CustomFloat32{}, spannerTypeArrayOfNonNullFloat32},
		{[]CustomTime{}, spannerTypeArrayOfNonNullTime},
		{[]CustomDate{}, spannerTypeArrayOfNonNullDate},
		{[]CustomNumeric{}, spannerTypeArrayOfNonNullNumeric},

		{CustomNullString{}, spannerTypeNullString},
		{CustomNullInt64{}, spannerTypeNullInt64},
		{CustomNullBool{}, spannerTypeNullBool},
		{CustomNullFloat64{}, spannerTypeNullFloat64},
		{CustomNullFloat32{}, spannerTypeNullFloat32},
		{CustomNullTime{}, spannerTypeNullTime},
		{CustomNullDate{}, spannerTypeNullDate},
		{CustomNullNumeric{}, spannerTypeNullNumeric},

		{[]CustomNullString{}, spannerTypeArrayOfNullString},
		{[]CustomNullInt64{}, spannerTypeArrayOfNullInt64},
		{[]CustomNullBool{}, spannerTypeArrayOfNullBool},
		{[]CustomNullFloat64{}, spannerTypeArrayOfNullFloat64},
		{[]CustomNullFloat32{}, spannerTypeArrayOfNullFloat32},
		{[]CustomNullTime{}, spannerTypeArrayOfNullTime},
		{[]CustomNullDate{}, spannerTypeArrayOfNullDate},
		{[]CustomNullNumeric{}, spannerTypeArrayOfNullNumeric},

		{StringEmbedded{}, spannerTypeUnknown},
		{NullStringEmbedded{}, spannerTypeUnknown},
	} {
		// Pass a pointer to the original value.
		gotp := reflect.New(reflect.TypeOf(test.in))
		got := getDecodableSpannerType(gotp.Interface(), true)
		if got != test.want {
			t.Errorf("%d: unexpected decodable type from a pointer - got %v, want %v", i, got, test.want)
		}

		// Pass the original value.
		got = getDecodableSpannerType(test.in, false)
		if got != test.want {
			t.Errorf("%d: unexpected decodable type from a value - got %v, want %v", i, got, test.want)
		}
	}
}

// Test NaN encoding/decoding.
func TestNaN(t *testing.T) {
	// Decode NaN value.
	f := 0.0
	nf := NullFloat64{}
	// To float64
	if err := decodeValue(floatProto(math.NaN()), floatType(), &f); err != nil {
		t.Errorf("decodeValue returns %q for %v, want nil", err, floatProto(math.NaN()))
	}
	if !math.IsNaN(f) {
		t.Errorf("f = %v, want %v", f, math.NaN())
	}
	// To NullFloat64
	if err := decodeValue(floatProto(math.NaN()), floatType(), &nf); err != nil {
		t.Errorf("decodeValue returns %q for %v, want nil", err, floatProto(math.NaN()))
	}
	if !math.IsNaN(nf.Float64) || !nf.Valid {
		t.Errorf("f = %v, want %v", f, NullFloat64{math.NaN(), true})
	}
	// Encode NaN value
	// From float64
	v, _, err := encodeValue(math.NaN())
	if err != nil {
		t.Errorf("encodeValue returns %q for NaN, want nil", err)
	}
	x, ok := v.GetKind().(*proto3.Value_NumberValue)
	if !ok {
		t.Errorf("incorrect type for v.GetKind(): %T, want *proto3.Value_NumberValue", v.GetKind())
	}
	if !math.IsNaN(x.NumberValue) {
		t.Errorf("x.NumberValue = %v, want %v", x.NumberValue, math.NaN())
	}
	// From NullFloat64
	v, _, err = encodeValue(NullFloat64{math.NaN(), true})
	if err != nil {
		t.Errorf("encodeValue returns %q for NaN, want nil", err)
	}
	x, ok = v.GetKind().(*proto3.Value_NumberValue)
	if !ok {
		t.Errorf("incorrect type for v.GetKind(): %T, want *proto3.Value_NumberValue", v.GetKind())
	}
	if !math.IsNaN(x.NumberValue) {
		t.Errorf("x.NumberValue = %v, want %v", x.NumberValue, math.NaN())
	}
}

// Test Float32 NaN encoding/decoding.
func TestFloat32NaN(t *testing.T) {
	// Decode NaN value.
	f := float32(0.0)
	nf := NullFloat32{}
	// To float32
	if err := decodeValue(float32Proto(float32(math.NaN())), float32Type(), &f); err != nil {
		t.Errorf("decodeValue returns %q for %v, want nil", err, float32Proto(float32(math.NaN())))
	}
	if !math.IsNaN(float64(f)) {
		t.Errorf("f = %v, want %v", f, math.NaN())
	}
	// To NullFloat32
	if err := decodeValue(float32Proto(float32(math.NaN())), float32Type(), &nf); err != nil {
		t.Errorf("decodeValue returns %q for %v, want nil", err, float32Proto(float32(math.NaN())))
	}
	if !math.IsNaN(float64(nf.Float32)) || !nf.Valid {
		t.Errorf("f = %v, want %v", f, NullFloat32{float32(math.NaN()), true})
	}
	// Encode NaN value
	// From float32
	v, _, err := encodeValue(float32(math.NaN()))
	if err != nil {
		t.Errorf("encodeValue returns %q for NaN, want nil", err)
	}
	x, ok := v.GetKind().(*proto3.Value_NumberValue)
	if !ok {
		t.Errorf("incorrect type for v.GetKind(): %T, want *proto3.Value_NumberValue", v.GetKind())
	}
	if !math.IsNaN(x.NumberValue) {
		t.Errorf("x.NumberValue = %v, want %v", x.NumberValue, math.NaN())
	}
	// From NullFloat32
	v, _, err = encodeValue(NullFloat32{float32(math.NaN()), true})
	if err != nil {
		t.Errorf("encodeValue returns %q for NaN, want nil", err)
	}
	x, ok = v.GetKind().(*proto3.Value_NumberValue)
	if !ok {
		t.Errorf("incorrect type for v.GetKind(): %T, want *proto3.Value_NumberValue", v.GetKind())
	}
	if !math.IsNaN(x.NumberValue) {
		t.Errorf("x.NumberValue = %v, want %v", x.NumberValue, math.NaN())
	}
}

func TestGenericColumnValue(t *testing.T) {
	for _, test := range []struct {
		in   GenericColumnValue
		want interface{}
		fail bool
	}{
		{GenericColumnValue{stringType(), stringProto("abc")}, "abc", false},
		{GenericColumnValue{stringType(), stringProto("abc")}, 5, true},
		{GenericColumnValue{listType(intType()), listProto(intProto(91), nullProto(), intProto(87))}, []NullInt64{{91, true}, {}, {87, true}}, false},
		{GenericColumnValue{intType(), intProto(42)}, GenericColumnValue{intType(), intProto(42)}, false}, // trippy! :-)
	} {
		gotp := reflect.New(reflect.TypeOf(test.want))
		if err := test.in.Decode(gotp.Interface()); err != nil {
			if !test.fail {
				t.Errorf("cannot decode %v to %v: %v", test.in, test.want, err)
			}
			continue
		}
		if test.fail {
			t.Errorf("decoding %v to %v succeeds unexpectedly", test.in, test.want)
		}

		// Test we can go backwards as well.
		v, err := newGenericColumnValue(test.want)
		if err != nil {
			t.Errorf("NewGenericColumnValue failed: %v", err)
			continue
		}
		if !testEqual(*v, test.in) {
			t.Errorf("unexpected encode result - got %v, want %v", v, test.in)
		}
	}
}

func TestDecodeStruct(t *testing.T) {
	type CustomString string
	type CustomTime time.Time
	stype := &sppb.StructType{Fields: []*sppb.StructType_Field{
		{Name: "Id", Type: stringType()},
		{Name: "Time", Type: timeType()},
	}}
	lv := listValueProto(stringProto("id"), timeProto(t1))

	type (
		S1 struct {
			ID   string
			Time time.Time
		}
		S2 struct {
			ID   string
			Time string
		}
		S3 struct {
			ID   CustomString
			Time CustomTime
		}
		S4 struct {
			ID   CustomString
			Time CustomString
		}
		S5 struct {
			NullString
			Time CustomTime
		}
	)
	var (
		s1 S1
		s2 S2
		s3 S3
		s4 S4
		s5 S5
	)

	for _, test := range []struct {
		desc    string
		lenient bool
		ptr     interface{}
		want    interface{}
		fail    bool
	}{
		{
			desc:    "decode to S1 with lenient enabled",
			ptr:     &s1,
			want:    &S1{ID: "id", Time: t1},
			lenient: true,
		},
		{
			desc:    "decode to S1 with lenient disabled",
			ptr:     &s1,
			want:    &S1{ID: "id", Time: t1},
			lenient: false,
		},
		{
			desc:    "decode to S2 with lenient enabled",
			ptr:     &s2,
			fail:    true,
			lenient: true,
		},
		{
			desc:    "decode to S2 with lenient disabled",
			ptr:     &s2,
			fail:    true,
			lenient: false,
		},
		{
			desc:    "decode to S3 with lenient enabled",
			ptr:     &s3,
			want:    &S3{ID: CustomString("id"), Time: CustomTime(t1)},
			lenient: true,
		},
		{
			desc:    "decode to S3 with lenient disabled",
			ptr:     &s3,
			want:    &S3{ID: CustomString("id"), Time: CustomTime(t1)},
			lenient: false,
		},
		{
			desc:    "decode to S4 with lenient enabled",
			ptr:     &s4,
			fail:    true,
			lenient: true,
		},
		{
			desc:    "decode to S4 with lenient disabled",
			ptr:     &s4,
			fail:    true,
			lenient: false,
		},
		{
			desc:    "decode to S5 with lenient enabled",
			ptr:     &s5,
			want:    &S5{NullString: NullString{}, Time: CustomTime(t1)},
			lenient: true,
		},
		{
			desc:    "decode to S5 with lenient disabled",
			ptr:     &s5,
			fail:    true,
			lenient: false,
		},
	} {
		err := decodeStruct(stype, lv, test.ptr, test.lenient)
		if (err != nil) != test.fail {
			t.Errorf("%s: got error %v, wanted fail: %v", test.desc, err, test.fail)
		}
		if err == nil {
			if !testutil.Equal(test.ptr, test.want, cmp.AllowUnexported(CustomTime{})) {
				t.Errorf("%s: got %+v, want %+v", test.desc, test.ptr, test.want)
			}
		}
	}
}

func TestDecodeStructWithPointers(t *testing.T) {
	stype := &sppb.StructType{Fields: []*sppb.StructType_Field{
		{Name: "Str", Type: stringType()},
		{Name: "Int", Type: intType()},
		{Name: "Bool", Type: boolType()},
		{Name: "Float", Type: floatType()},
		{Name: "Time", Type: timeType()},
		{Name: "Date", Type: dateType()},
		{Name: "StrArray", Type: listType(stringType())},
		{Name: "IntArray", Type: listType(intType())},
		{Name: "BoolArray", Type: listType(boolType())},
		{Name: "FloatArray", Type: listType(floatType())},
		{Name: "TimeArray", Type: listType(timeType())},
		{Name: "DateArray", Type: listType(dateType())},
	}}
	lv := []*proto3.ListValue{
		listValueProto(
			stringProto("id"),
			intProto(15),
			boolProto(true),
			floatProto(3.14),
			timeProto(t1),
			dateProto(d1),
			listProto(stringProto("id1"), nullProto(), stringProto("id2")),
			listProto(intProto(16), nullProto(), intProto(17)),
			listProto(boolProto(true), nullProto(), boolProto(false)),
			listProto(floatProto(3.14), nullProto(), floatProto(6.626)),
			listProto(timeProto(t1), nullProto(), timeProto(t2)),
			listProto(dateProto(d1), nullProto(), dateProto(d2)),
		),
		listValueProto(
			nullProto(),
			nullProto(),
			nullProto(),
			nullProto(),
			nullProto(),
			nullProto(),
			nullProto(),
			nullProto(),
			nullProto(),
			nullProto(),
			nullProto(),
			nullProto(),
		),
	}

	type S1 struct {
		Str        *string
		Int        *int64
		Bool       *bool
		Float      *float64
		Time       *time.Time
		Date       *civil.Date
		StrArray   []*string
		IntArray   []*int64
		BoolArray  []*bool
		FloatArray []*float64
		TimeArray  []*time.Time
		DateArray  []*civil.Date
	}
	var s1 S1
	sValue := "id"
	iValue := int64(15)
	bValue := true
	fValue := 3.14
	tValue := t1
	dValue := d1
	sArrayValue1 := "id1"
	sArrayValue2 := "id2"
	sArrayValue := []*string{&sArrayValue1, nil, &sArrayValue2}
	iArrayValue1 := int64(16)
	iArrayValue2 := int64(17)
	iArrayValue := []*int64{&iArrayValue1, nil, &iArrayValue2}
	bArrayValue1 := true
	bArrayValue2 := false
	bArrayValue := []*bool{&bArrayValue1, nil, &bArrayValue2}
	f1Value := 3.14
	f2Value := 6.626
	fArrayValue := []*float64{&f1Value, nil, &f2Value}
	t1Value := t1
	t2Value := t2
	tArrayValue := []*time.Time{&t1Value, nil, &t2Value}
	d1Value := d1
	d2Value := d2
	dArrayValue := []*civil.Date{&d1Value, nil, &d2Value}

	for i, test := range []struct {
		desc string
		ptr  *S1
		want *S1
		fail bool
	}{
		{
			desc: "decode values to S1",
			ptr:  &s1,
			want: &S1{Str: &sValue, Int: &iValue, Bool: &bValue, Float: &fValue, Time: &tValue, Date: &dValue, StrArray: sArrayValue, IntArray: iArrayValue, BoolArray: bArrayValue, FloatArray: fArrayValue, TimeArray: tArrayValue, DateArray: dArrayValue},
		},
		{
			desc: "decode nulls to S1",
			ptr:  &s1,
			want: &S1{Str: nil, Int: nil, Bool: nil, Float: nil, Time: nil, Date: nil, StrArray: nil, IntArray: nil, BoolArray: nil, FloatArray: nil, TimeArray: nil, DateArray: nil},
		},
	} {
		err := decodeStruct(stype, lv[i], test.ptr, false)
		if (err != nil) != test.fail {
			t.Errorf("%s: got error %v, wanted fail: %v", test.desc, err, test.fail)
		}
		if err == nil {
			if !testutil.Equal(test.ptr, test.want) {
				t.Errorf("%s: got %+v, want %+v", test.desc, test.ptr, test.want)
			}
		}
	}
}

func TestDecodeStructArray(t *testing.T) {
	stype := &sppb.StructType{Fields: []*sppb.StructType_Field{
		{Name: "C", Type: &sppb.Type{Code: sppb.TypeCode_ARRAY,
			ArrayElementType: &sppb.Type{
				Code: sppb.TypeCode_STRUCT,
				StructType: &sppb.StructType{Fields: []*sppb.StructType_Field{
					{Name: "A", Type: intType()},
					{Name: "B", Type: intType()},
				}},
			},
		},
		},
	},
	}
	lv := listValueProto(listProto(listProto(intProto(1), intProto(2))))

	type (
		// inner struct
		S2 struct {
			A int64 `spanner:"A"`
		}

		S1 struct {
			C []*S2 `spanner:"C"`
		}
	)

	var (
		test1 S1
		test2 S1
	)
	for _, test := range []struct {
		desc    string
		lenient bool
		ptr     interface{}
		want    interface{}
		fail    bool
	}{
		{
			// when the Spanner returns more fields in inner struct compared to Go inner struct
			desc:    "decode to S1 with lenient enabled",
			ptr:     &test1,
			want:    &S1{C: []*S2{{A: 1}}},
			lenient: true,
		},
		{
			desc:    "decode to S1 with lenient disabled",
			ptr:     &test2,
			fail:    true,
			lenient: false,
		},
	} {
		err := decodeStruct(stype, lv, test.ptr, test.lenient)
		if (err != nil) != test.fail {
			t.Errorf("%s: got error %v, wanted fail: %v", test.desc, err, test.fail)
		}
		if err == nil {
			if !testutil.Equal(test.ptr, test.want) {
				t.Errorf("%s: got %+v, want %+v", test.desc, test.ptr, test.want)
			}
		}
	}
}

func TestEncodeStructValueDynamicStructs(t *testing.T) {
	dynStructType := reflect.StructOf([]reflect.StructField{
		{Name: "A", Type: reflect.TypeOf(0), Tag: `spanner:"a"`},
		{Name: "B", Type: reflect.TypeOf(""), Tag: `spanner:"b"`},
	})
	dynNullableStructType := reflect.PtrTo(dynStructType)
	dynStructArrType := reflect.SliceOf(dynStructType)
	dynNullableStructArrType := reflect.SliceOf(dynNullableStructType)

	dynStructValue := reflect.New(dynStructType)
	dynStructValue.Elem().Field(0).SetInt(10)
	dynStructValue.Elem().Field(1).SetString("abc")

	dynStructArrValue := reflect.MakeSlice(dynNullableStructArrType, 2, 2)
	dynStructArrValue.Index(0).Set(reflect.Zero(dynNullableStructType))
	dynStructArrValue.Index(1).Set(dynStructValue)

	structProtoType := structType(
		mkField("a", intType()),
		mkField("b", stringType()))

	arrProtoType := listType(structProtoType)

	for _, test := range []encodeTest{
		{
			"Dynanic non-NULL struct value.",
			dynStructValue.Elem().Interface(),
			listProto(intProto(10), stringProto("abc")),
			structProtoType,
		},
		{
			"Dynanic NULL struct value.",
			reflect.Zero(dynNullableStructType).Interface(),
			nullProto(),
			structProtoType,
		},
		{
			"Empty array of dynamic structs.",
			reflect.MakeSlice(dynStructArrType, 0, 0).Interface(),
			listProto([]*proto3.Value{}...),
			arrProtoType,
		},
		{
			"NULL array of non-NULL-able dynamic structs.",
			reflect.Zero(dynStructArrType).Interface(),
			nullProto(),
			arrProtoType,
		},
		{
			"NULL array of NULL-able(nil) dynamic structs.",
			reflect.Zero(dynNullableStructArrType).Interface(),
			nullProto(),
			arrProtoType,
		},
		{
			"Array containing NULL(nil) dynamic-typed struct elements.",
			dynStructArrValue.Interface(),
			listProto(
				nullProto(),
				listProto(intProto(10), stringProto("abc"))),
			arrProtoType,
		},
	} {
		encodeStructValue(test, t)
	}
}

func TestEncodeStructValueEmptyStruct(t *testing.T) {
	emptyListValue := listProto([]*proto3.Value{}...)
	emptyStructType := structType([]*sppb.StructType_Field{}...)
	emptyStruct := struct{}{}
	nullEmptyStruct := (*struct{})(nil)

	dynamicEmptyStructType := reflect.StructOf(make([]reflect.StructField, 0, 0))
	dynamicStructArrType := reflect.SliceOf(reflect.PtrTo((dynamicEmptyStructType)))

	dynamicEmptyStruct := reflect.New(dynamicEmptyStructType)
	dynamicNullEmptyStruct := reflect.Zero(reflect.PtrTo(dynamicEmptyStructType))

	dynamicStructArrValue := reflect.MakeSlice(dynamicStructArrType, 2, 2)
	dynamicStructArrValue.Index(0).Set(dynamicNullEmptyStruct)
	dynamicStructArrValue.Index(1).Set(dynamicEmptyStruct)

	for _, test := range []encodeTest{
		{
			"Go empty struct.",
			emptyStruct,
			emptyListValue,
			emptyStructType,
		},
		{
			"Dynamic empty struct.",
			dynamicEmptyStruct.Interface(),
			emptyListValue,
			emptyStructType,
		},
		{
			"Go NULL empty struct.",
			nullEmptyStruct,
			nullProto(),
			emptyStructType,
		},
		{
			"Dynamic NULL empty struct.",
			dynamicNullEmptyStruct.Interface(),
			nullProto(),
			emptyStructType,
		},
		{
			"Non-empty array of dynamic NULL and non-NULL empty structs.",
			dynamicStructArrValue.Interface(),
			listProto(nullProto(), emptyListValue),
			listType(emptyStructType),
		},
		{
			"Non-empty array of nullable empty structs.",
			[]*struct{}{nullEmptyStruct, &emptyStruct},
			listProto(nullProto(), emptyListValue),
			listType(emptyStructType),
		},
		{
			"Empty array of empty struct.",
			[]struct{}{},
			emptyListValue,
			listType(emptyStructType),
		},
		{
			"Null array of empty structs.",
			[]struct{}(nil),
			nullProto(),
			listType(emptyStructType),
		},
	} {
		encodeStructValue(test, t)
	}
}

func TestEncodeStructValueMixedStructTypes(t *testing.T) {
	type staticStruct struct {
		F int `spanner:"fStatic"`
	}
	s1 := staticStruct{10}
	s2 := (*staticStruct)(nil)

	var f float64
	dynStructType := reflect.StructOf([]reflect.StructField{
		{Name: "A", Type: reflect.TypeOf(f), Tag: `spanner:"fDynamic"`},
	})
	s3 := reflect.New(dynStructType)
	s3.Elem().Field(0).SetFloat(3.14)

	for _, test := range []encodeTest{
		{
			"'struct' with static and dynamic *struct, []*struct, []struct fields",
			struct {
				A []staticStruct
				B []*staticStruct
				C interface{}
			}{
				[]staticStruct{s1, s1},
				[]*staticStruct{&s1, s2},
				s3.Interface(),
			},
			listProto(
				listProto(listProto(intProto(10)), listProto(intProto(10))),
				listProto(listProto(intProto(10)), nullProto()),
				listProto(floatProto(3.14))),
			structType(
				mkField("A", listType(structType(mkField("fStatic", intType())))),
				mkField("B", listType(structType(mkField("fStatic", intType())))),
				mkField("C", structType(mkField("fDynamic", floatType())))),
		},
	} {
		encodeStructValue(test, t)
	}
}

func TestBindParamsDynamic(t *testing.T) {
	// Verify Statement.bindParams generates correct values and types.
	st := Statement{
		SQL:    "SELECT id from t_foo WHERE col = @var",
		Params: map[string]interface{}{"var": nil},
	}
	want := &sppb.ExecuteSqlRequest{
		Params: &proto3.Struct{
			Fields: map[string]*proto3.Value{"var": nil},
		},
		ParamTypes: map[string]*sppb.Type{"var": nil},
	}
	var (
		t1, _ = time.Parse(time.RFC3339Nano, "2016-11-15T15:04:05.999999999Z")
		// Boundaries
		t2, _ = time.Parse(time.RFC3339Nano, "0001-01-01T00:00:00.000000000Z")
	)
	dynamicStructType := reflect.StructOf([]reflect.StructField{
		{Name: "A", Type: reflect.TypeOf(t1), Tag: `spanner:"field"`},
		{Name: "B", Type: reflect.TypeOf(3.14), Tag: `spanner:""`},
	})
	dynamicStructArrType := reflect.SliceOf(reflect.PtrTo(dynamicStructType))
	dynamicEmptyStructType := reflect.StructOf(make([]reflect.StructField, 0, 0))

	dynamicStructTypeProto := structType(
		mkField("field", timeType()),
		mkField("", floatType()))

	s3 := reflect.New(dynamicStructType)
	s3.Elem().Field(0).Set(reflect.ValueOf(t1))
	s3.Elem().Field(1).SetFloat(1.4)

	s4 := reflect.New(dynamicStructType)
	s4.Elem().Field(0).Set(reflect.ValueOf(t2))
	s4.Elem().Field(1).SetFloat(-13.3)

	dynamicStructArrayVal := reflect.MakeSlice(dynamicStructArrType, 2, 2)
	dynamicStructArrayVal.Index(0).Set(s3)
	dynamicStructArrayVal.Index(1).Set(s4)

	for _, test := range []struct {
		val       interface{}
		wantField *proto3.Value
		wantType  *sppb.Type
	}{
		{
			s3.Interface(),
			listProto(timeProto(t1), floatProto(1.4)),
			structType(
				mkField("field", timeType()),
				mkField("", floatType())),
		},
		{
			reflect.Zero(reflect.PtrTo(dynamicEmptyStructType)).Interface(),
			nullProto(),
			structType([]*sppb.StructType_Field{}...),
		},
		{
			dynamicStructArrayVal.Interface(),
			listProto(
				listProto(timeProto(t1), floatProto(1.4)),
				listProto(timeProto(t2), floatProto(-13.3))),
			listType(dynamicStructTypeProto),
		},
		{
			[]*struct {
				F1 time.Time `spanner:"field"`
				F2 float64   `spanner:""`
			}{
				nil,
				{t1, 1.4},
			},
			listProto(
				nullProto(),
				listProto(timeProto(t1), floatProto(1.4))),
			listType(dynamicStructTypeProto),
		},
	} {
		st.Params["var"] = test.val
		want.Params.Fields["var"] = test.wantField
		want.ParamTypes["var"] = test.wantType
		gotParams, gotParamTypes, gotErr := st.convertParams()
		if gotErr != nil {
			t.Error(gotErr)
			continue
		}
		gotParamField := gotParams.Fields["var"]
		if !proto.Equal(gotParamField, test.wantField) {
			// handle NaN
			gotParamFieldText, err := prototext.Marshal(gotParamField)
			if err != nil {
				t.Fatal(err)
			}
			wantParamFieldText, err := prototext.Marshal(test.wantField)
			if err != nil {
				t.Fatal(err)
			}
			if test.wantType.Code == floatType().Code && bytes.Equal(gotParamFieldText, wantParamFieldText) {
				continue
			}
			t.Errorf("%#v: got %v, want %v\n", test.val, gotParamField, test.wantField)
		}
		gotParamType := gotParamTypes["var"]
		if !proto.Equal(gotParamType, test.wantType) {
			t.Errorf("%#v: got %v, want %v\n", test.val, gotParamType, test.wantField)
		}
	}
}

// Test converting nullable types to json strings.
func TestJSONMarshal_NullTypes(t *testing.T) {
	type Message struct {
		Name string
		Body string
		Time int64
	}
	msg := Message{"Alice", "Hello", 1294706395881547000}
	jsonStr := `{"Name":"Alice","Body":"Hello","Time":1294706395881547000}`

	singerProtoEnum := pb.Genre_ROCK
	singerProtoMessage := pb.SingerInfo{
		SingerId:    proto.Int64(1),
		BirthDate:   proto.String("January"),
		Nationality: proto.String("Country1"),
		Genre:       &singerProtoEnum,
	}
	singerProtoMessageJSONStr := `{"singer_id":1,"birth_date":"January","nationality":"Country1","genre":3}`

	type testcase struct {
		input  interface{}
		expect string
	}

	for _, test := range []struct {
		name  string
		cases []testcase
	}{
		{
			"NullString",
			[]testcase{
				{input: NullString{"this is a test string", true}, expect: `"this is a test string"`},
				{input: &NullString{"this is a test string", true}, expect: `"this is a test string"`},
				{input: &NullString{"this is a test string", false}, expect: "null"},
				{input: NullString{}, expect: "null"},
			},
		},
		{
			"NullInt64",
			[]testcase{
				{input: NullInt64{int64(123), true}, expect: "123"},
				{input: &NullInt64{int64(123), true}, expect: "123"},
				{input: &NullInt64{int64(123), false}, expect: "null"},
				{input: NullInt64{}, expect: "null"},
			},
		},
		{
			"NullFloat64",
			[]testcase{
				{input: NullFloat64{float64(123.123), true}, expect: "123.123"},
				{input: &NullFloat64{float64(123.123), true}, expect: "123.123"},
				{input: &NullFloat64{float64(123.123), false}, expect: "null"},
				{input: NullFloat64{}, expect: "null"},
			},
		},
		{
			"NullFloat32",
			[]testcase{
				{input: NullFloat32{float32(3.14), true}, expect: "3.14"},
				{input: &NullFloat32{float32(123.123), true}, expect: "123.123"},
				{input: &NullFloat32{float32(123.123), false}, expect: "null"},
				{input: NullFloat32{}, expect: "null"},
			},
		},
		{
			"NullBool",
			[]testcase{
				{input: NullBool{true, true}, expect: "true"},
				{input: &NullBool{true, true}, expect: "true"},
				{input: &NullBool{true, false}, expect: "null"},
				{input: NullBool{}, expect: "null"},
			},
		},
		{
			"NullTime",
			[]testcase{
				{input: NullTime{time.Date(2009, 11, 17, 20, 34, 58, 651387237, time.UTC), true}, expect: `"2009-11-17T20:34:58.651387237Z"`},
				{input: &NullTime{time.Date(2009, 11, 17, 20, 34, 58, 651387237, time.UTC), true}, expect: `"2009-11-17T20:34:58.651387237Z"`},
				{input: &NullTime{time.Date(2009, 11, 17, 20, 34, 58, 651387237, time.UTC), false}, expect: "null"},
				{input: NullTime{}, expect: "null"},
			},
		},
		{
			"NullDate",
			[]testcase{
				{input: NullDate{civil.Date{Year: 2009, Month: time.November, Day: 17}, true}, expect: `"2009-11-17"`},
				{input: &NullDate{civil.Date{Year: 2009, Month: time.November, Day: 17}, true}, expect: `"2009-11-17"`},
				{input: &NullDate{civil.Date{Year: 2009, Month: time.November, Day: 17}, false}, expect: "null"},
				{input: NullDate{}, expect: "null"},
			},
		},
		{
			"NullNumeric",
			[]testcase{
				{input: NullNumeric{*big.NewRat(1234123456789, 1e9), true}, expect: `"1234.123456789"`},
				{input: &NullNumeric{*big.NewRat(1234123456789, 1e9), true}, expect: `"1234.123456789"`},
				{input: &NullNumeric{*big.NewRat(1234123456789, 1e9), false}, expect: "null"},
				{input: NullNumeric{}, expect: "null"},
			},
		},
		{
			"NullJSON",
			[]testcase{
				{input: NullJSON{msg, true}, expect: jsonStr},
				{input: &NullJSON{msg, true}, expect: jsonStr},
				{input: &NullJSON{msg, false}, expect: "null"},
				{input: NullJSON{}, expect: "null"},
			},
		},
		{
			"PGNumeric",
			[]testcase{
				{input: PGNumeric{"123.456", true}, expect: `"123.456"`},
				{input: PGNumeric{"NaN", true}, expect: `"NaN"`},
				{input: &PGNumeric{"123.456", true}, expect: `"123.456"`},
				{input: &PGNumeric{"123.456", false}, expect: "null"},
				{input: PGNumeric{}, expect: "null"},
			},
		},
		{
			"NullProtoMessage",
			[]testcase{
				{input: NullProtoMessage{&singerProtoMessage, true}, expect: singerProtoMessageJSONStr},
				{input: &NullProtoMessage{&singerProtoMessage, true}, expect: singerProtoMessageJSONStr},
				{input: &NullProtoMessage{&singerProtoMessage, false}, expect: "null"},
				{input: NullProtoMessage{}, expect: "null"},
			},
		},
		{
			"NullProtoEnum",
			[]testcase{
				{input: NullProtoEnum{singerProtoEnum, true}, expect: "3"},
				{input: NullProtoEnum{&singerProtoEnum, true}, expect: "3"},
				{input: &NullProtoEnum{singerProtoEnum, true}, expect: "3"},
				{input: &NullProtoEnum{&singerProtoEnum, true}, expect: "3"},
				{input: NullProtoEnum{singerProtoEnum, false}, expect: "null"},
				{input: NullProtoEnum{nil, true}, expect: "null"},
				{input: NullProtoEnum{}, expect: "null"},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, tc := range test.cases {
				bytes, _ := json.Marshal(tc.input)
				got := string(bytes)
				if got != tc.expect {
					t.Fatalf("Incorrect marshalling to json strings: got %v, want %v", got, tc.expect)
				}
			}
		})
	}
}

// Test converting json strings to nullable types.
func TestJSONUnmarshal_NullTypes(t *testing.T) {
	jsonStr := `{"Body":"Hello","Name":"Alice","Time":1294706395881547000}`
	singerProtoEnum := pb.Genre_ROCK
	singerProtoMessage := pb.SingerInfo{
		SingerId:    proto.Int64(1),
		BirthDate:   proto.String("January"),
		Nationality: proto.String("Country1"),
		Genre:       &singerProtoEnum,
	}
	singerProtoMessageJSONStr := `{"singer_id":1,"birth_date":"January","nationality":"Country1","genre":3}`

	type testcase struct {
		input       []byte
		got         interface{}
		isNull      bool
		expect      string
		expectError bool
	}

	for _, test := range []struct {
		name  string
		cases []testcase
	}{
		{
			"NullString",
			[]testcase{
				{input: []byte(`"this is a test string"`), got: NullString{}, isNull: false, expect: "this is a test string", expectError: false},
				{input: []byte(`""`), got: NullString{}, isNull: false, expect: "", expectError: false},
				{input: []byte("null"), got: NullString{}, isNull: true, expect: nullString, expectError: false},
				{input: []byte(`"{\"sub_a\": \"value_1\"}"`), got: NullString{}, isNull: false, expect: `{"sub_a": "value_1"}`, expectError: false},
				{input: nil, got: NullString{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(""), got: NullString{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(`"hello`), got: NullString{}, isNull: true, expect: nullString, expectError: true},
			},
		},
		{
			"NullInt64",
			[]testcase{
				{input: []byte("123"), got: NullInt64{}, isNull: false, expect: "123", expectError: false},
				{input: []byte("null"), got: NullInt64{}, isNull: true, expect: nullString, expectError: false},
				{input: nil, got: NullInt64{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(""), got: NullInt64{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(`"hello`), got: NullInt64{}, isNull: true, expect: nullString, expectError: true},
			},
		},
		{
			"NullFloat64",
			[]testcase{
				{input: []byte("123.123"), got: NullFloat64{}, isNull: false, expect: "123.123", expectError: false},
				{input: []byte("null"), got: NullFloat64{}, isNull: true, expect: nullString, expectError: false},
				{input: nil, got: NullFloat64{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(""), got: NullFloat64{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(`"hello`), got: NullFloat64{}, isNull: true, expect: nullString, expectError: true},
			},
		},
		{
			"NullFloat32",
			[]testcase{
				{input: []byte("3.14"), got: NullFloat32{}, isNull: false, expect: "3.14", expectError: false},
				{input: []byte("null"), got: NullFloat32{}, isNull: true, expect: nullString, expectError: false},
				{input: nil, got: NullFloat32{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(""), got: NullFloat32{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(`"hello`), got: NullFloat32{}, isNull: true, expect: nullString, expectError: true},
			},
		},
		{
			"NullBool",
			[]testcase{
				{input: []byte("true"), got: NullBool{}, isNull: false, expect: "true", expectError: false},
				{input: []byte("null"), got: NullBool{}, isNull: true, expect: nullString, expectError: false},
				{input: nil, got: NullBool{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(""), got: NullBool{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(`"hello`), got: NullBool{}, isNull: true, expect: nullString, expectError: true},
			},
		},
		{
			"NullTime",
			[]testcase{
				{input: []byte(`"2009-11-17T20:34:58.651387237Z"`), got: NullTime{}, isNull: false, expect: "2009-11-17T20:34:58.651387237Z", expectError: false},
				{input: []byte("null"), got: NullTime{}, isNull: true, expect: nullString, expectError: false},
				{input: nil, got: NullTime{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(""), got: NullTime{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(`"hello`), got: NullTime{}, isNull: true, expect: nullString, expectError: true},
			},
		},
		{
			"NullDate",
			[]testcase{
				{input: []byte(`"2009-11-17"`), got: NullDate{}, isNull: false, expect: "2009-11-17", expectError: false},
				{input: []byte("null"), got: NullDate{}, isNull: true, expect: nullString, expectError: false},
				{input: nil, got: NullDate{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(""), got: NullDate{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(`"hello`), got: NullDate{}, isNull: true, expect: nullString, expectError: true},
			},
		},
		{
			"NullNumeric",
			[]testcase{
				{input: []byte(`"1234.123456789"`), got: NullNumeric{}, isNull: false, expect: "1234.123456789", expectError: false},
				{input: []byte("null"), got: NullNumeric{}, isNull: true, expect: nullString, expectError: false},
				{input: nil, got: NullNumeric{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(""), got: NullNumeric{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(`"1234.123456789`), got: NullNumeric{}, isNull: true, expect: nullString, expectError: true},
			},
		},
		{
			"NullJSON",
			[]testcase{
				{input: []byte(jsonStr), got: NullJSON{}, isNull: false, expect: jsonStr, expectError: false},
				{input: []byte("null"), got: NullJSON{}, isNull: true, expect: nullString, expectError: false},
				{input: nil, got: NullJSON{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(""), got: NullJSON{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(`{invalid_json_string}`), got: NullJSON{}, isNull: true, expect: nullString, expectError: true},
			},
		},
		{
			"PGNumeric",
			[]testcase{
				{input: []byte(`"123.456"`), got: PGNumeric{}, isNull: false, expect: "123.456", expectError: false},
				{input: []byte(`"NaN"`), got: PGNumeric{}, isNull: false, expect: "NaN", expectError: false},
				{input: []byte("null"), got: PGNumeric{}, isNull: true, expect: nullString, expectError: false},
				{input: nil, got: PGNumeric{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(""), got: PGNumeric{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(`"123.456`), got: PGNumeric{}, isNull: true, expect: nullString, expectError: true},
			},
		},
		{
			"NullProtoMessage",
			[]testcase{
				{input: []byte(singerProtoMessageJSONStr), got: NullProtoMessage{&pb.SingerInfo{}, true}, isNull: false, expect: singerProtoMessage.String(), expectError: false},
				{input: []byte("null"), got: NullProtoMessage{&pb.SingerInfo{}, true}, isNull: true, expect: nullString, expectError: false},
				{input: nil, got: NullProtoMessage{&pb.SingerInfo{}, true}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(""), got: NullProtoMessage{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(`{invalid_json_string}`), got: NullProtoMessage{}, isNull: true, expect: nullString, expectError: true},
			},
		},
		{
			"NullProtoEnum",
			[]testcase{
				{input: []byte("3"), got: NullProtoEnum{&singerProtoEnum, true}, isNull: false, expect: singerProtoEnum.String(), expectError: false},
				{input: []byte("null"), got: NullProtoEnum{}, isNull: true, expect: nullString, expectError: false},
				{input: nil, got: NullProtoEnum{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(""), got: NullProtoEnum{}, isNull: true, expect: nullString, expectError: true},
				{input: []byte(`"hello`), got: NullProtoEnum{}, isNull: true, expect: nullString, expectError: true},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, tc := range test.cases {
				switch v := tc.got.(type) {
				case NullString:
					err := json.Unmarshal(tc.input, &v)
					expectUnmarshalNullableTypes(t, err, v, tc.isNull, tc.expect, tc.expectError)
				case NullInt64:
					err := json.Unmarshal(tc.input, &v)
					expectUnmarshalNullableTypes(t, err, v, tc.isNull, tc.expect, tc.expectError)
				case NullFloat64:
					err := json.Unmarshal(tc.input, &v)
					expectUnmarshalNullableTypes(t, err, v, tc.isNull, tc.expect, tc.expectError)
				case NullFloat32:
					err := json.Unmarshal(tc.input, &v)
					expectUnmarshalNullableTypes(t, err, v, tc.isNull, tc.expect, tc.expectError)
				case NullBool:
					err := json.Unmarshal(tc.input, &v)
					expectUnmarshalNullableTypes(t, err, v, tc.isNull, tc.expect, tc.expectError)
				case NullTime:
					err := json.Unmarshal(tc.input, &v)
					expectUnmarshalNullableTypes(t, err, v, tc.isNull, tc.expect, tc.expectError)
				case NullDate:
					err := json.Unmarshal(tc.input, &v)
					expectUnmarshalNullableTypes(t, err, v, tc.isNull, tc.expect, tc.expectError)
				case NullNumeric:
					err := json.Unmarshal(tc.input, &v)
					expectUnmarshalNullableTypes(t, err, v, tc.isNull, tc.expect, tc.expectError)
				case NullJSON:
					err := json.Unmarshal(tc.input, &v)
					expectUnmarshalNullableTypes(t, err, v, tc.isNull, tc.expect, tc.expectError)
				case PGNumeric:
					err := json.Unmarshal(tc.input, &v)
					expectUnmarshalNullableTypes(t, err, v, tc.isNull, tc.expect, tc.expectError)
				case NullProtoMessage:
					err := json.Unmarshal(tc.input, &v)
					expectUnmarshalNullableTypes(t, err, v, tc.isNull, tc.expect, tc.expectError)
				case NullProtoEnum:
					err := json.Unmarshal(tc.input, &v)
					expectUnmarshalNullableTypes(t, err, v, tc.isNull, tc.expect, tc.expectError)
				default:
					t.Fatalf("Unknown type: %T", v)
				}
			}
		})
	}
}

func expectUnmarshalNullableTypes(t *testing.T, err error, v interface{}, isNull bool, expect string, expectError bool) {
	if expectError {
		if err == nil {
			t.Fatalf("Expect to get an error, but got a nil")
		}
		return
	}

	if err != nil {
		t.Fatalf("Got an error when unmarshalling a valid json string: %q", err)
	}
	if s, ok := v.(NullableValue); !ok || s.IsNull() != isNull {
		t.Fatalf("Incorrect unmarshalling a json string to nullable types: got %q, want %q", v, expect)
	}
	if s, ok := v.(fmt.Stringer); !ok || s.String() != expect {
		t.Fatalf("Incorrect unmarshalling a json string to nullable types: got %q, want %q", v, expect)
	}
}

func TestNullJson(t *testing.T) {
	v, _ := nulljson(false, nil)
	v[0] = 'X'
	v, _ = nulljson(false, nil)
	if string(v) != "null" {
		t.Fatalf("expected null, got %s", v)
	}
}

// Test decode for PROTO type when custom type is a variant of a base type
func TestDecodeProtoUsingBaseVariant(t *testing.T) {
	// nullBytes is custom type from []byte base type.
	type nullBytes []byte

	var b []byte
	var nb nullBytes

	gcv := &GenericColumnValue{
		Type: &sppb.Type{
			Code:         sppb.TypeCode_PROTO,
			ProtoTypeFqn: "examples.ProtoType",
		},
		Value: structpb.NewStringValue("Zm9vCg=="),
	}
	if err := gcv.Decode(&nb); err != nil {
		t.Error(err)
	}
	if err := gcv.Decode(&b); err != nil {
		t.Error(err)
	}

	// Convert []byte and nullBytes to base64 encoding and then compare the contents.
	if !testutil.Equal(base64.StdEncoding.EncodeToString(b), base64.StdEncoding.EncodeToString(nb)) {
		t.Errorf("%s: got %+v, want %+v", "Test PROTO decode to []byte custom type", nb, b)
	}
}

// Test decode for PROTO type when custom type is a variant of a base type
func TestDecodeProtoArrayUsingBaseVariant(t *testing.T) {
	// nullBytes is custom type from []byte base type.
	type nullBytes [][]byte

	var b [][]byte
	var nb nullBytes

	gcv := &GenericColumnValue{
		Type: &sppb.Type{
			Code: sppb.TypeCode_ARRAY,
			ArrayElementType: &sppb.Type{
				Code:         sppb.TypeCode_PROTO,
				ProtoTypeFqn: "examples.ProtoType",
			},
		},
		Value: structpb.NewListValue(
			&structpb.ListValue{
				Values: []*structpb.Value{
					structpb.NewStringValue("Zm9vCg=="),
				},
			}),
	}
	if err := gcv.Decode(&nb); err != nil {
		t.Error(err)
	}
	if err := gcv.Decode(&b); err != nil {
		t.Error(err)
	}

	if len(b) != 1 {
		t.Errorf("Expected length to be 1")
	}

	if len(nb) != 1 {
		t.Errorf("Expected length to be 1")
	}
	// Convert to base64 encoding and then compare the contents.
	if !testutil.Equal(base64.StdEncoding.EncodeToString(b[0]), base64.StdEncoding.EncodeToString(nb[0])) {
		t.Errorf("%s: got %+v, want %+v", "Test PROTO decode to [][]byte custom type", nb, b)
	}
}
