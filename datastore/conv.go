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
	"strconv"
	"strings"
	"time"

	"code.google.com/p/goprotobuf/proto"

	pb "google.golang.org/cloud/internal/datastore"
)

const (
	tagKeyDatastore = "datastore"
)

var operatorToProto = map[operator]*pb.PropertyFilter_Operator{
	lessThan:    pb.PropertyFilter_LESS_THAN.Enum(),
	lessEq:      pb.PropertyFilter_LESS_THAN_OR_EQUAL.Enum(),
	equal:       pb.PropertyFilter_EQUAL.Enum(),
	greaterEq:   pb.PropertyFilter_GREATER_THAN_OR_EQUAL.Enum(),
	greaterThan: pb.PropertyFilter_GREATER_THAN.Enum(),
}

var sortDirectionToProto = map[sortDirection]*pb.PropertyOrder_Direction{
	ascending:  pb.PropertyOrder_ASCENDING.Enum(),
	descending: pb.PropertyOrder_ASCENDING.Enum(),
}

type fieldMeta struct {
	field   *reflect.StructField
	name    string
	indexed bool
}

var (
	typeOfByteSlice = reflect.TypeOf([]byte{})
	typeOfTime      = reflect.TypeOf(time.Time{})
	typeOfKeyPtr    = reflect.TypeOf(&Key{})
)

var entityMeta map[reflect.Type](map[string]*fieldMeta) = make(map[reflect.Type](map[string]*fieldMeta))

func registerEntityMeta(typ reflect.Type) map[string]*fieldMeta {
	// TODO(jbd): Should be thread safe.
	entityMeta[typ] = make(map[string]*fieldMeta)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get(tagKeyDatastore)

		name := ""
		indexed := true // by default

		// name_of_the_field
		// name_of_the_field,noindex
		if strings.Contains(tag, "noindex") {
			indexed = false
			name = strings.Replace(tag, ",noindex", "", -1)
		} else {
			name = tag
		}

		if tag == "" {
			// TODO(jbd): CamelCase to camel_case
			name = strings.ToLower(field.Name)
		}

		entityMeta[typ][name] = &fieldMeta{
			field: &field, name: name, indexed: indexed,
		}
	}
	return entityMeta[typ]
}

func keyToPbKey(k *Key) *pb.Key {
	pathEl := &pb.Key_PathElement{Kind: &k.kind}
	if k.intID > 0 {
		pathEl.Id = &k.intID
	} else if k.name != "" {
		pathEl.Name = &k.name
	}
	key := &pb.Key{
		PartitionId: &pb.PartitionId{
			DatasetId: &k.datasetID,
		},
		PathElement: []*pb.Key_PathElement{pathEl},
	}
	if k.namespace != "" {
		key.PartitionId.Namespace = proto.String(k.namespace)
	}
	return key
}

func keyFromKeyProto(datasetID string, p *pb.Key) *Key {
	return newKey(
		p.GetPathElement()[0].GetKind(),
		strconv.FormatInt(p.GetPathElement()[0].GetId(), 10),
		p.GetPathElement()[0].GetId(),
		datasetID,
		p.GetPartitionId().GetNamespace())
}

func queryToQueryProto(q *Query) *pb.Query {
	p := &pb.Query{}
	p.Kind = []*pb.KindExpression{&pb.KindExpression{Name: proto.String(q.kind)}}
	if len(q.projection) > 0 {
		p.Projection = make([]*pb.PropertyExpression, len(q.projection))
		for i, fieldName := range q.projection {
			p.Projection[i] = &pb.PropertyExpression{
				Property: &pb.PropertyReference{Name: proto.String(fieldName)},
			}
		}
	}
	// filters
	if len(q.filter) > 0 {
		p.Filter = &pb.Filter{
			CompositeFilter: &pb.CompositeFilter{},
		}

		filters := make([]*pb.Filter, len(q.filter))
		for i, f := range q.filter {
			filters[i] = &pb.Filter{
				PropertyFilter: &pb.PropertyFilter{
					Property: &pb.PropertyReference{Name: &f.FieldName},
					Operator: operatorToProto[f.Op],
					Value:    objToValue(f.Value),
				},
			}
		}
		p.Filter.CompositeFilter.Filter = filters
		p.Filter.CompositeFilter.Operator = pb.CompositeFilter_AND.Enum()
	}
	// group-by
	if len(q.groupBy) > 0 {
		p.GroupBy = make([]*pb.PropertyReference, len(q.groupBy))
		for i, fieldName := range q.groupBy {
			p.GroupBy[i] = &pb.PropertyReference{Name: &fieldName}
		}
	}
	// pagination
	if len(q.start) > 0 {
		p.StartCursor = q.start
	}
	if q.limit > 0 {
		p.Limit = &q.limit

	}
	if q.offset > 0 {
		p.Offset = &q.offset
	}
	return p
}

func entityToEntityProto(key *Key, val reflect.Value) *pb.Entity {
	typ := val.Type()
	metadata, ok := entityMeta[typ]
	if !ok {
		metadata = registerEntityMeta(typ)
	}
	entityProto := &pb.Entity{
		Key:      keyToPbKey(key),
		Property: make([]*pb.Property, 0),
	}

	for name, f := range metadata {
		p := &pb.Property{}
		v := val.FieldByName(f.field.Name)
		p.Name = proto.String(name)
		p.Value = objToValue(v.Interface())
		entityProto.Property = append(entityProto.GetProperty(), p)
	}
	return entityProto
}

// val should has a struct type
func entityFromEntityProto(datasetId string, e *pb.Entity, val reflect.Value) {
	typ := val.Type()
	if typ.Kind() != reflect.Struct {
		panic("datastore: val should have a struct type")
	}

	metadata, ok := entityMeta[typ]
	if !ok {
		metadata = registerEntityMeta(typ)
	}

	for _, p := range e.GetProperty() {
		f, ok := metadata[p.GetName()]
		if !ok {
			// skip if not presented in the struct
			continue
		}

		fieldVal := val.FieldByName(f.field.Name)
		dsVal := p.GetValue()

		switch f.field.Type.Kind() {
		case reflect.Int, reflect.Int16, reflect.Int32, reflect.Int64:
			fieldVal.SetInt(dsVal.GetIntegerValue())
		case reflect.Bool:
			fieldVal.SetBool(dsVal.GetBooleanValue())
		case reflect.Float32, reflect.Float64:
			fieldVal.SetFloat(dsVal.GetDoubleValue())
		case reflect.String:
			fieldVal.SetString(dsVal.GetStringValue())
		case typeOfByteSlice.Kind():
			fieldVal.SetBytes(dsVal.GetBlobValue())
		case typeOfKeyPtr.Kind():
			key := keyFromKeyProto(datasetId, dsVal.GetKeyValue())
			fieldVal.Set(reflect.ValueOf(key))
		case typeOfTime.Kind():
			// TODO(jbd): Add more precision
			t := time.Unix(dsVal.GetTimestampMicrosecondsValue()/1000*1000, 0)
			fieldVal.Set(reflect.ValueOf(t))
			// TODO(jbd): Handle lists, time, other composites
		}
	}
}

func objToValue(src interface{}) *pb.Value {
	switch src.(type) {
	case bool:
		return &pb.Value{BooleanValue: proto.Bool(src.(bool))}
	case int16, int32, int64, int:
		return &pb.Value{IntegerValue: proto.Int64(src.(int64))}
	case float32, float64:
		return &pb.Value{DoubleValue: proto.Float64(src.(float64))}
	case time.Time:
		t := src.(time.Time)
		// TODO(jbd): Unix time in ms? No.
		return &pb.Value{TimestampMicrosecondsValue: proto.Int64(t.Unix())}
	case *Key:
		pKey := keyToPbKey(src.(*Key))
		return &pb.Value{KeyValue: pKey}
	case string:
		return &pb.Value{StringValue: proto.String(src.(string))}
	case []byte:
		return &pb.Value{BlobValue: src.([]byte)}
	}
	// TODO(jbd): Support Composite types and lists.
	return nil
}

func isPtrOfStruct(src interface{}) bool {
	return reflect.TypeOf(src).Kind() == reflect.Ptr && reflect.TypeOf(src).Elem().Kind() == reflect.Struct
}

func isSlicePtr(src interface{}) bool {
	return reflect.TypeOf(src).Elem().Kind() == reflect.Slice
}
