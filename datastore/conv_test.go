// Copyright 2014 Google Inc. All Rights Reserved.
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

package datastore

import (
	"testing"

	"code.google.com/p/goprotobuf/proto"

	"google.golang.org/cloud/internal/datastore"
)

var (
	protoCircle = &pb.Entity{
		Property: []*pb.Property{
			&pb.Property{
				Name:  proto.String("name"),
				Value: &pb.Value{StringValue: proto.String("circle1")},
			},
			&pb.Property{
				Name:  proto.String("diameter"),
				Value: &pb.Value{IntegerValue: proto.Int64(50)},
			},
		},
	}
	protoSquare = &pb.Entity{
		Property: []*pb.Property{
			&pb.Property{
				Name:  proto.String("name"),
				Value: &pb.Value{StringValue: proto.String("square1")},
			},
			&pb.Property{
				Name:  proto.String("length"),
				Value: &pb.Value{IntegerValue: proto.Int64(50)},
			},
		},
	}
)

type circle struct {
	Name     string
	Diameter int64
}

type square struct {
	Name   string
	Length int64
}

func TestMultiConv_InterfaceSlice(t *testing.T) {
	c := &circle{}
	s := &square{}
	ents := []interface{}{c, s}
	m, err := newMultiConverter(2, ents)
	if err != nil {
		t.Error(err)
	}
	m.set(0, protoCircle)
	m.set(1, protoSquare)

	circle := ents[0].(*circle)
	if circle.Name != "circle1" && circle.Diameter != 50 {
		t.Errorf("Unexpected circle, %v found", circle)
	}
	sq := ents[1].(*square)
	if sq.Name != "square1" && sq.Length != 50 {
		t.Errorf("Unexpected square, %v found", sq)
	}
}

func TestMultiConv_PtrSlice(t *testing.T) {
	ents := make([]*square, 2)
	m, err := newMultiConverter(2, ents)
	if err != nil {
		t.Error(err)
	}
	m.set(0, protoSquare)
	m.set(1, protoSquare)
	for _, sq := range ents {
		if sq.Name != "square1" && sq.Length != 50 {
			t.Errorf("Unexpected square, %v found", sq)
		}
	}
}

func TestMultiConv_NilInterfaceElem(t *testing.T) {
	ents := []interface{}{nil, nil}
	_, err := newMultiConverter(2, ents)
	if err.Error() != "datastore: interface{} slice with nil items are not allowed" {
		t.Errorf("interface slices with nil elems shouldn't be allowed")
	}
}

func TestMutiConv_Length(t *testing.T) {
	ents := make([]*square, 1)
	_, err := newMultiConverter(2, ents)
	if err.Error() != "datastore: dest length is smaller than the number of the results" {
		t.Errorf("Expected to error with length, but found err = %v", err)
	}
}
