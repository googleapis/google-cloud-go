// Copyright 2021 Google LLC
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
package managedwriter

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// RowSerializer is an interface for handling message conversions.
//
// This API transmits row data using arbitrary protocol buffers.
// A RowSerializer must be able to describe the format of the proto
// messages it will transmit, and it must support the conversion of
// incoming messages.
type RowSerializer interface {
	// Describe provides the descriptor proto for the serialized messages.
	//
	Describe() *descriptorpb.DescriptorProto

	// Convert is used to convert an input into one or more rows, serialized
	// in the the protobuf message's binary format.
	Convert(in interface{}) ([][]byte, error)
}

// simpleRowSerializer simplifies serializer construction.
type simpleRowSerializer struct {
	DescFn    func() *descriptorpb.DescriptorProto
	ConvertFn func(in interface{}) ([][]byte, error)
}

// Describe satisfies the method for RowSerializer
func (srs *simpleRowSerializer) Describe() *descriptorpb.DescriptorProto {
	return srs.DescFn()
}

// Convert satisfies the method for RowSerializer
func (srs *simpleRowSerializer) Convert(in interface{}) ([][]byte, error) {
	return srs.ConvertFn(in)
}

func staticDescFn(in *descriptorpb.DescriptorProto) func() *descriptorpb.DescriptorProto {
	return func() *descriptorpb.DescriptorProto {
		return in
	}
}

// marshalConvert is a converter function that will serialize either a protobuf
// message or a slice of messages.
func marshalConvert(in interface{}) ([][]byte, error) {
	if msg, ok := in.(proto.Message); ok {
		b, err := proto.Marshal(msg)
		if err != nil {
			return nil, err
		}
		return [][]byte{b}, nil
	}
	return nil, fmt.Errorf("not a proto message")
	/*
		// Non-proto case.  Try reflecting into a slice.
		s := reflect.ValueOf(in)
		if s.Kind() != reflect.Slice {
			return nil, fmt.Errorf("input of unknown type (%T)", in)
		}
		if s.IsNil() {
			return nil, fmt.Errorf("empty slice of data")
		}

		messageType := reflect.TypeOf((*proto.Message)(nil)).Elem()
		for _, element := s.Elem() {
			if !s.Type().Implements(messageType) {
				return nil, fmt.Errorf("slice element is not a proto.Message")
			}
		}
	*/
}

// passthroughConvert is for cases where you've pre-serialized the data
func passthroughConvert(in interface{}) ([][]byte, error) {
	if bs, ok := in.([][]byte); ok {
		return bs, nil
	}
	if b, ok := in.([]byte); ok {
		return [][]byte{b}, nil
	}
	return nil, fmt.Errorf("not bytes")
}
