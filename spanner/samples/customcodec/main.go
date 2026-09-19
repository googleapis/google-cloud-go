// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package main demonstrates how to implement and use a tailored, high-performance
// custom gRPC codec and memory pool specifically optimized for Cloud Spanner streaming reads.
package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

// Internal pools for protobuf Value allocations and variants.
var (
	valuePool = sync.Pool{
		New: func() any {
			return &structpb.Value{}
		},
	}
	stringValuePool = sync.Pool{
		New: func() any {
			return &structpb.Value_StringValue{}
		},
	}
	numberValuePool = sync.Pool{
		New: func() any {
			return &structpb.Value_NumberValue{}
		},
	}
	boolValuePool = sync.Pool{
		New: func() any {
			return &structpb.Value_BoolValue{}
		},
	}
	nullValuePool = sync.Pool{
		New: func() any {
			return &structpb.Value_NullValue{}
		},
	}
	partialResultSetPool = sync.Pool{
		New: func() any {
			return &sppb.PartialResultSet{}
		},
	}
)

// CustomPartialResultSetPool implements spanner.PartialResultSetPool using memory recycling.
type CustomPartialResultSetPool struct{}

// Get retrieves a PartialResultSet instance from the memory pool.
func (p *CustomPartialResultSetPool) Get() *sppb.PartialResultSet {
	return partialResultSetPool.Get().(*sppb.PartialResultSet)
}

// Put returns a PartialResultSet and its nested Value instances back to their pools.
func (p *CustomPartialResultSetPool) Put(partialResultSet *sppb.PartialResultSet) {
	if partialResultSet == nil {
		return
	}
	for _, val := range partialResultSet.Values {
		if val != nil {
			switch variant := val.Kind.(type) {
			case *structpb.Value_StringValue:
				variant.StringValue = ""
				stringValuePool.Put(variant)
			case *structpb.Value_NumberValue:
				variant.NumberValue = 0
				numberValuePool.Put(variant)
			case *structpb.Value_BoolValue:
				variant.BoolValue = false
				boolValuePool.Put(variant)
			case *structpb.Value_NullValue:
				variant.NullValue = 0
				nullValuePool.Put(variant)
			}
			val.Kind = nil
			valuePool.Put(val)
		}
	}
	partialResultSet.Values = partialResultSet.Values[:0]
	partialResultSet.ResumeToken = partialResultSet.ResumeToken[:0]
	partialResultSet.ChunkedValue = false
	partialResultSet.Metadata = nil
	partialResultSet.Stats = nil
	partialResultSet.PrecommitToken = nil
	partialResultSetPool.Put(partialResultSet)
}

// SpannerFastCodec is a custom gRPC codec optimized specifically for Cloud Spanner.
// It accelerates PartialResultSet unmarshaling via zero-reflection decoding, and falls back
// to standard proto.Unmarshal for all other message types.
type SpannerFastCodec struct{}

// Name returns the gRPC codec name.
func (SpannerFastCodec) Name() string {
	return "proto"
}

// Marshal marshals a proto.Message.
func (SpannerFastCodec) Marshal(value any) ([]byte, error) {
	protoMessage, ok := value.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("%T is not a proto.Message", value)
	}
	return proto.Marshal(protoMessage)
}

// Unmarshal unmarshals raw wire bytes into the target message.
func (SpannerFastCodec) Unmarshal(data []byte, value any) error {
	if partialResultSet, ok := value.(*sppb.PartialResultSet); ok {
		return FastUnmarshalPartialResultSet(data, partialResultSet)
	}
	protoMessage, ok := value.(proto.Message)
	if !ok {
		return fmt.Errorf("%T is not a proto.Message", value)
	}
	return proto.Unmarshal(data, protoMessage)
}

// FastUnmarshalPartialResultSet fast-decodes Protobuf wire bytes directly into a PartialResultSet.
func FastUnmarshalPartialResultSet(data []byte, target *sppb.PartialResultSet) error {
	index := 0
	length := len(data)
	target.Values = target.Values[:0]

	for index < length {
		var tag uint64
		if data[index] < 0x80 {
			tag = uint64(data[index])
			index++
		} else {
			value, bytesRead := binary.Uvarint(data[index:])
			if bytesRead <= 0 {
				return fmt.Errorf("invalid varint at index %d", index)
			}
			tag = value
			index += bytesRead
		}

		fieldNumber := tag >> 3
		wireType := tag & 7

		switch fieldNumber {
		case 2: // Values: repeated google.protobuf.Value
			if wireType != 2 {
				return fmt.Errorf("unexpected wire type %d for field 2", wireType)
			}
			valueLength, bytesRead := binary.Uvarint(data[index:])
			if bytesRead <= 0 {
				return fmt.Errorf("invalid value length at index %d", index)
			}
			index += bytesRead
			valueEnd := index + int(valueLength)
			if valueEnd > length {
				return fmt.Errorf("unexpected EOF reading protobuf Value")
			}

			valObj := valuePool.Get().(*structpb.Value)
			if err := fastUnmarshalValue(data[index:valueEnd], valObj); err != nil {
				return err
			}
			target.Values = append(target.Values, valObj)
			index = valueEnd

		case 4: // ResumeToken: bytes
			if wireType != 2 {
				return fmt.Errorf("unexpected wire type %d for field 4", wireType)
			}
			tokenLength, bytesRead := binary.Uvarint(data[index:])
			if bytesRead <= 0 {
				return fmt.Errorf("invalid token length at index %d", index)
			}
			index += bytesRead
			tokenEnd := index + int(tokenLength)
			if tokenEnd > length {
				return fmt.Errorf("unexpected EOF reading resume token")
			}
			target.ResumeToken = append(target.ResumeToken[:0], data[index:tokenEnd]...)
			index = tokenEnd

		case 3: // ChunkedValue: bool
			if wireType != 0 {
				return fmt.Errorf("unexpected wire type %d for field 3", wireType)
			}
			value, bytesRead := binary.Uvarint(data[index:])
			if bytesRead <= 0 {
				return fmt.Errorf("invalid varint at index %d", index)
			}
			target.ChunkedValue = (value != 0)
			index += bytesRead

		case 1: // Metadata: ResultSetMetadata (fallback to proto.Unmarshal)
			if wireType != 2 {
				return fmt.Errorf("unexpected wire type %d for field 1", wireType)
			}
			metadataLength, bytesRead := binary.Uvarint(data[index:])
			if bytesRead <= 0 {
				return fmt.Errorf("invalid metadata length at index %d", index)
			}
			index += bytesRead
			metadataEnd := index + int(metadataLength)
			if metadataEnd > length {
				return fmt.Errorf("unexpected EOF reading metadata")
			}
			if target.Metadata == nil {
				target.Metadata = &sppb.ResultSetMetadata{}
			}
			if err := proto.Unmarshal(data[index:metadataEnd], target.Metadata); err != nil {
				return err
			}
			index = metadataEnd

		case 5: // Stats: ResultSetStats (fallback to proto.Unmarshal)
			if wireType != 2 {
				return fmt.Errorf("unexpected wire type %d for field 5", wireType)
			}
			statsLength, bytesRead := binary.Uvarint(data[index:])
			if bytesRead <= 0 {
				return fmt.Errorf("invalid stats length at index %d", index)
			}
			index += bytesRead
			statsEnd := index + int(statsLength)
			if statsEnd > length {
				return fmt.Errorf("unexpected EOF reading stats")
			}
			if target.Stats == nil {
				target.Stats = &sppb.ResultSetStats{}
			}
			if err := proto.Unmarshal(data[index:statsEnd], target.Stats); err != nil {
				return err
			}
			index = statsEnd

		default:
			// Skip unrecognized fields based on wire type
			switch wireType {
			case 0: // Varint
				_, bytesRead := binary.Uvarint(data[index:])
				if bytesRead <= 0 {
					return fmt.Errorf("invalid varint skipping field %d", fieldNumber)
				}
				index += bytesRead
			case 1: // 64-bit fixed
				index += 8
			case 2: // Length-delimited
				fieldLength, bytesRead := binary.Uvarint(data[index:])
				if bytesRead <= 0 {
					return fmt.Errorf("invalid length skipping field %d", fieldNumber)
				}
				index += bytesRead + int(fieldLength)
			case 5: // 32-bit fixed
				index += 4
			default:
				return fmt.Errorf("unsupported wire type %d for field %d", wireType, fieldNumber)
			}
		}
	}
	return nil
}

func fastUnmarshalValue(data []byte, target *structpb.Value) error {
	if len(data) == 0 {
		return nil
	}
	tag := data[0]
	switch tag {
	case 0x1A: // StringValue: field 3, wire 2
		stringLength, bytesRead := binary.Uvarint(data[1:])
		if bytesRead <= 0 {
			return fmt.Errorf("invalid string length in Value")
		}
		stringStart := 1 + bytesRead
		stringEnd := stringStart + int(stringLength)
		if stringEnd > len(data) {
			return fmt.Errorf("unexpected EOF reading string value")
		}
		stringValue, ok := target.Kind.(*structpb.Value_StringValue)
		if !ok {
			stringValue = stringValuePool.Get().(*structpb.Value_StringValue)
		}
		stringValue.StringValue = string(data[stringStart:stringEnd])
		target.Kind = stringValue

	case 0x11: // NumberValue: field 2, wire 1 (8 bytes fixed64)
		if len(data) < 9 {
			return fmt.Errorf("unexpected EOF reading number value")
		}
		bits := binary.LittleEndian.Uint64(data[1:9])
		numberValue, ok := target.Kind.(*structpb.Value_NumberValue)
		if !ok {
			numberValue = numberValuePool.Get().(*structpb.Value_NumberValue)
		}
		numberValue.NumberValue = math.Float64frombits(bits)
		target.Kind = numberValue

	case 0x20: // BoolValue: field 4, wire 0
		boolValue, ok := target.Kind.(*structpb.Value_BoolValue)
		if !ok {
			boolValue = boolValuePool.Get().(*structpb.Value_BoolValue)
		}
		boolValue.BoolValue = (data[1] != 0)
		target.Kind = boolValue

	case 0x08: // NullValue: field 1, wire 0
		nullValue, ok := target.Kind.(*structpb.Value_NullValue)
		if !ok {
			nullValue = nullValuePool.Get().(*structpb.Value_NullValue)
		}
		nullValue.NullValue = structpb.NullValue(data[1])
		target.Kind = nullValue

	default:
		// Complex types (struct, list) fall back to standard protobuf unmarshal
		return proto.Unmarshal(data, target)
	}
	return nil
}

// NewCustomOptimizedSpannerClient creates a standard Spanner client configured
// with the custom fast-path gRPC codec and memory pooler.
func NewCustomOptimizedSpannerClient(ctx context.Context, database string, opts ...option.ClientOption) (*spanner.Client, error) {
	customCodecOption := option.WithGRPCDialOption(
		grpc.WithDefaultCallOptions(
			grpc.ForceCodec(SpannerFastCodec{}),
		),
	)

	allOpts := append([]option.ClientOption{customCodecOption}, opts...)
	return spanner.NewClientWithConfig(ctx, database,
		spanner.ClientConfig{
			SessionPoolConfig:    spanner.DefaultSessionPoolConfig,
			PartialResultSetPool: &CustomPartialResultSetPool{},
		},
		allOpts...,
	)
}

func main() {
	ctx := context.Background()

	database := os.Getenv("SPANNER_DATABASE")
	if database == "" {
		database = "projects/my-project/instances/my-instance/databases/my-database"
	}

	fmt.Printf("Initializing Spanner client with custom fast codec for: %s\n", database)

	client, err := NewCustomOptimizedSpannerClient(ctx, database)
	if err != nil {
		log.Fatalf("Failed to create Spanner client: %v", err)
	}
	defer client.Close()

	statement := spanner.Statement{
		SQL: "SELECT 1 AS col_int, 'Hello from custom fast codec' AS col_str, CURRENT_TIMESTAMP() AS col_ts",
	}

	iteratorInstance := client.Single().Query(ctx, statement)
	defer iteratorInstance.Stop()

	for {
		row, err := iteratorInstance.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("Query failed: %v", err)
		}

		var colInt int64
		var colStr string
		var colTs time.Time

		if err := row.Columns(&colInt, &colStr, &colTs); err != nil {
			log.Fatalf("Failed to read columns: %v", err)
		}

		fmt.Printf("Row received -> col_int: %d, col_str: %q, col_ts: %v\n", colInt, colStr, colTs)
	}

	fmt.Println("Done! Streaming query decoded successfully using custom Spanner fast codec.")
}
