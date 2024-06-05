/*
Copyright 2024 Google LLC

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

package bigtable

import btapb "google.golang.org/genproto/googleapis/bigtable/admin/v2"

// Type wraps the protobuf representation of a type. See the protobuf definition
// for more details on types.
type Type interface {
	proto() *btapb.Type
}

// BytesEncoding represents the encoding of a Bytes type.
type BytesEncoding interface {
	proto() *btapb.Type_Bytes_Encoding
}

// RawBytesEncoding represents a Bytes encoding with no additional encodings.
type RawBytesEncoding struct {
}

func (encoding RawBytesEncoding) proto() *btapb.Type_Bytes_Encoding {
	return &btapb.Type_Bytes_Encoding{
		Encoding: &btapb.Type_Bytes_Encoding_Raw_{
			Raw: &btapb.Type_Bytes_Encoding_Raw{}}}
}

// BytesType represents a string of bytes.
type BytesType struct {
	Encoding BytesEncoding
}

func (bytes BytesType) proto() *btapb.Type {
	var encoding *btapb.Type_Bytes_Encoding
	if bytes.Encoding != nil {
		encoding = bytes.Encoding.proto()
	} else {
		encoding = RawBytesEncoding{}.proto()
	}
	return &btapb.Type{Kind: &btapb.Type_BytesType{BytesType: &btapb.Type_Bytes{Encoding: encoding}}}
}

// StringEncoding represents the encoding of a String.
type StringEncoding interface {
	proto() *btapb.Type_String_Encoding
}

// StringUtf8Encoding represents a string with UTF-8 encoding.
type StringUtf8Encoding struct {
}

func (encoding StringUtf8Encoding) proto() *btapb.Type_String_Encoding {
	return &btapb.Type_String_Encoding{
		Encoding: &btapb.Type_String_Encoding_Utf8Raw_{},
	}
}

// StringType represents a string
type StringType struct {
	Encoding StringEncoding
}

func (str StringType) proto() *btapb.Type {
	var encoding *btapb.Type_String_Encoding
	if str.Encoding != nil {
		encoding = str.Encoding.proto()
	} else {
		encoding = StringUtf8Encoding{}.proto()
	}
	return &btapb.Type{Kind: &btapb.Type_StringType{StringType: &btapb.Type_String{Encoding: encoding}}}
}

// Int64Encoding represents the encoding of an Int64 type.
type Int64Encoding interface {
	proto() *btapb.Type_Int64_Encoding
}

// BigEndianBytesEncoding represents an Int64 encoding where the value is encoded
// as an 8-byte big-endian value.  The byte representation may also have further encoding
// via Bytes.
type BigEndianBytesEncoding struct {
	Bytes BytesType
}

func (beb BigEndianBytesEncoding) proto() *btapb.Type_Int64_Encoding {
	return &btapb.Type_Int64_Encoding{
		Encoding: &btapb.Type_Int64_Encoding_BigEndianBytes_{
			BigEndianBytes: &btapb.Type_Int64_Encoding_BigEndianBytes{
				BytesType: beb.Bytes.proto().GetBytesType(),
			},
		},
	}
}

// Int64Type represents an 8-byte integer.
type Int64Type struct {
	Encoding Int64Encoding
}

func (it Int64Type) proto() *btapb.Type {
	var encoding *btapb.Type_Int64_Encoding
	if it.Encoding != nil {
		encoding = it.Encoding.proto()
	} else {
		// default encoding to BigEndianBytes
		encoding = BigEndianBytesEncoding{}.proto()
	}

	return &btapb.Type{
		Kind: &btapb.Type_Int64Type{
			Int64Type: &btapb.Type_Int64{
				Encoding: encoding,
			},
		},
	}
}

// Aggregator represents an aggregation function for an aggregate type.
type Aggregator interface {
	fillProto(proto *btapb.Type_Aggregate)
}

// SumAggregator is an aggregation function that sums inputs together into its
// accumulator.
type SumAggregator struct{}

func (sum SumAggregator) fillProto(proto *btapb.Type_Aggregate) {
	proto.Aggregator = &btapb.Type_Aggregate_Sum_{Sum: &btapb.Type_Aggregate_Sum{}}
}

// AggregateType represents an aggregate.  See types.proto for more details
// on aggregate types.
type AggregateType struct {
	Input      Type
	Aggregator Aggregator
}

func (agg AggregateType) proto() *btapb.Type {
	protoAgg := &btapb.Type_Aggregate{
		InputType: agg.Input.proto(),
	}

	agg.Aggregator.fillProto(protoAgg)
	return &btapb.Type{Kind: &btapb.Type_AggregateType{AggregateType: protoAgg}}
}
