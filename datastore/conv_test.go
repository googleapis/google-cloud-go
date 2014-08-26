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
	"reflect"
	"testing"
	"time"

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

type someType struct {
	Name      string
	Blob      []byte
	Done      bool
	Size      int64
	Total     float64
	OtherKey  *Key
	CreatedAt time.Time
}

func TestEntityToProto(t *testing.T) {
	key := &Key{kind: "Kind1", name: "entity1"}
	now := time.Now()
	s := someType{
		Name:      "name",
		Blob:      []byte("hello world"),
		Done:      true,
		Size:      56,
		Total:     120.45,
		OtherKey:  &Key{kind: "Kind2", id: 123},
		CreatedAt: now,
	}
	proto := entityToProto(key, reflect.ValueOf(s))
	if proto.Key.PartitionId != nil {
		t.Error("Partition should have been nil")
	}
	if proto.Key.PathElement[0].GetKind() != "Kind1" {
		t.Errorf("proto key kind found as %v unexpectedly", *proto.Key.PathElement[0].Kind)
	}
	if proto.Key.PathElement[0].GetName() != "entity1" {
		t.Errorf("proto key name found as %v unexpectedly", *proto.Key.PathElement[0].Name)
	}
	if proto.Key.PathElement[0].Id != nil {
		t.Errorf("proto key id should have been nil")
	}

	for _, prop := range proto.Property {
		switch prop.GetName() {
		case "name":
			if prop.GetValue().GetStringValue() != s.Name {
				t.Errorf("Unexpected name property is found: %v", prop)
			}
		case "blob":
			if string(prop.GetValue().GetBlobValue()) != string(s.Blob) {
				t.Errorf("Unexpected blob property is found: %v", prop)
			}
		case "done":
			if prop.GetValue().GetBooleanValue() != s.Done {
				t.Errorf("Unexpected done property is found: %v", prop)
			}
		case "size":
			if prop.GetValue().GetIntegerValue() != s.Size {
				t.Errorf("Unexpected size property is found: %v", prop)
			}
		case "total":
			if prop.GetValue().GetDoubleValue() != s.Total {
				t.Errorf("Unexpected total property is found: %v", prop)
			}
		case "otherkey":
			protoKey := protoToKey(prop.GetValue().GetKeyValue())
			if !protoKey.IsEqual(s.OtherKey) {
				t.Errorf("Unexpected otherkey property is found: %v", prop)
			}
		case "createdat":
			if prop.GetValue().GetTimestampMicrosecondsValue() != now.UnixNano()/1000 {
				t.Errorf("Unexpected createdat property is found: %v", prop)
			}
		default:
			t.Errorf("Unexpected property name")
		}
	}
}

func TestProtoToEntity(t *testing.T) {
	p := &pb.Entity{
		Property: []*pb.Property{
			&pb.Property{
				Name:  proto.String("name"),
				Value: &pb.Value{StringValue: proto.String("name-value")},
			},
			&pb.Property{
				Name:  proto.String("blob"),
				Value: &pb.Value{BlobValue: []byte("blob-value")},
			},
			&pb.Property{
				Name:  proto.String("done"),
				Value: &pb.Value{BooleanValue: proto.Bool(true)},
			},
			&pb.Property{
				Name:  proto.String("size"),
				Value: &pb.Value{IntegerValue: proto.Int64(500)},
			},
			&pb.Property{
				Name:  proto.String("total"),
				Value: &pb.Value{DoubleValue: proto.Float64(100.15)},
			},
			&pb.Property{
				Name:  proto.String("createdat"),
				Value: &pb.Value{TimestampMicrosecondsValue: proto.Int64(1409090080287871)},
			},
		},
	}

	s := &someType{}
	protoToEntity(p, s)

	if s.Name != "name-value" {
		t.Errorf("Unexpected name, %v is found", s.Name)
	}
	if string(s.Blob) != string([]byte("blob-value")) {
		t.Errorf("Unexpected blob, %v is found", s.Blob)
	}
	if s.Done != true {
		t.Errorf("Unexpected done, %v is found", s.Done)
	}
	if s.Size != 500 {
		t.Errorf("Unexpected size, %v is found", s.Size)
	}
	if s.Total != 100.15 {
		t.Errorf("Unexpected total, %v is found", s.Total)
	}
	if s.CreatedAt != time.Unix(1409090080, 287871000) {
		t.Errorf("Unexpected createdat, %v is found", s.CreatedAt)
	}
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
