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
	"errors"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"code.google.com/p/goprotobuf/proto"
	pb "google.golang.org/cloud/internal/datastore"
)

const (
	tagKeyDatastore = "datastore"
)

var (
	reCCtoUnderscore = regexp.MustCompile("(.)([A-Z][a-z]+)")
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

var (
	typeOfByteSlice = reflect.TypeOf([]byte{})
	typeOfTime      = reflect.TypeOf(time.Time{})
	typeOfKey       = reflect.TypeOf(Key{})
)

type fieldMeta struct {
	field   *reflect.StructField
	name    string
	indexed bool
}

var (
	mu         sync.Mutex
	entityMeta map[reflect.Type](map[string]*fieldMeta) = make(map[reflect.Type](map[string]*fieldMeta))
)

func camelCaseToUnderscore(name string) string {
	v := reCCtoUnderscore.ReplaceAllLiteralString(name, "_$2")
	return strings.ToLower(v)
}

func registerEntityMeta(typ reflect.Type) map[string]*fieldMeta {
	mu.Lock()
	defer mu.Unlock()

	metadata, ok := entityMeta[typ]
	if ok {
		return metadata
	}
	entityMeta[typ] = make(map[string]*fieldMeta)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get(tagKeyDatastore)
		name := ""
		indexed := true // by default TODO(jbd): If list, don't set.

		// name_of_the_field
		// name_of_the_field,noindex
		if strings.Contains(tag, "noindex") {
			indexed = false
			name = strings.Replace(tag, ",noindex", "", -1)
		} else {
			name = tag
		}
		if name == "" {
			name = camelCaseToUnderscore(field.Name)
		}

		entityMeta[typ][name] = &fieldMeta{
			field: &field, name: name, indexed: indexed,
		}
	}
	return entityMeta[typ]
}

func keyToProto(k *Key) *pb.Key {
	// TODO(jbd): Eliminate unrequired allocations.
	path := []*pb.Key_PathElement(nil)
	for {
		el := &pb.Key_PathElement{
			Kind: proto.String(k.kind),
		}
		if k.id != 0 {
			el.Id = proto.Int64(k.id)
		}
		if k.name != "" {
			el.Name = proto.String(k.name)
		}
		path = append([]*pb.Key_PathElement{el}, path...)
		if k.parent == nil {
			break
		}
		k = k.parent
	}
	key := &pb.Key{
		PathElement: path,
	}
	if k.namespace != "" {
		key.PartitionId = &pb.PartitionId{
			Namespace: proto.String(k.namespace),
		}
	}
	return key
}

func protoToKey(p *pb.Key) *Key {
	keys := make([]*Key, len(p.GetPathElement()))
	for i, el := range p.GetPathElement() {
		keys[i] = &Key{
			namespace: p.GetPartitionId().GetNamespace(),
			kind:      el.GetKind(),
			id:        el.GetId(),
			name:      el.GetName(),
		}
	}
	for i := 0; i < len(keys)-1; i++ {
		keys[i+1].parent = keys[i]
	}
	return keys[len(keys)-1]
}

func queryToProto(q *Query) *pb.Query {
	// TODO(jbd): Support ancestry queries.
	p := &pb.Query{}
	p.Kind = make([]*pb.KindExpression, len(q.kinds))
	for i, kind := range q.kinds {
		p.Kind[i] = &pb.KindExpression{Name: proto.String(kind)}
	}
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
	if len(q.end) > 0 {
		p.EndCursor = q.end
	}
	if q.limit > 0 {
		p.Limit = &q.limit

	}
	if q.offset > 0 {
		p.Offset = &q.offset
	}
	return p
}

func entityToProto(key *Key, val reflect.Value) *pb.Entity {
	typ := val.Type()
	metadata := registerEntityMeta(typ)
	entityProto := &pb.Entity{
		Key:      keyToProto(key),
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

func protoToEntity(src *pb.Entity, dest interface{}) {
	typ := reflect.TypeOf(dest).Elem()
	val := reflect.ValueOf(dest).Elem()
	metadata := registerEntityMeta(typ)
	for _, p := range src.GetProperty() {
		f, ok := metadata[p.GetName()]
		if !ok {
			// skip if not presented in the struct
			continue
		}
		fv := val.FieldByName(f.field.Name)
		pv := p.GetValue()
		switch f.field.Type.Kind() {
		case reflect.Int, reflect.Int16, reflect.Int32, reflect.Int64:
			fv.SetInt(pv.GetIntegerValue())
		case reflect.Bool:
			fv.SetBool(pv.GetBooleanValue())
		case reflect.Float32, reflect.Float64:
			fv.SetFloat(pv.GetDoubleValue())
		case reflect.String:
			fv.SetString(pv.GetStringValue())
		case typeOfByteSlice.Kind():
			fv.SetBytes(pv.GetBlobValue())
		case typeOfKeyPtr.Kind():
			key := protoToKey(pv.GetKeyValue())
			fv.Set(reflect.ValueOf(key))
		case typeOfTime.Kind():
			sec := pv.GetTimestampMicrosecondsValue() / (1000 * 1000)
			us := pv.GetTimestampMicrosecondsValue() % (1000 * 1000)
			t := time.Unix(sec, us*1000)
			fv.Set(reflect.ValueOf(t))
			// TODO(jbd): Handle lists and composites
		case reflect.Slice:
			panic("not yet implemented")
		case reflect.Struct:
			panic("not yet implemented")
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
		us := t.UnixNano() / 1000
		return &pb.Value{TimestampMicrosecondsValue: proto.Int64(us)}
	case *Key:
		return &pb.Value{KeyValue: keyToProto(src.(*Key))}
	case string:
		return &pb.Value{StringValue: proto.String(src.(string))}
	case []byte:
		return &pb.Value{BlobValue: src.([]byte)}
	}
	// TODO(jbd): Support Composite types and lists.
	return nil
}

type multiConverter struct {
	dest interface{}
	size int

	sliceTyp reflect.Type
	elemType reflect.Type
	sliceVal reflect.Value
}

func newMultiConverter(size int, dest interface{}) (*multiConverter, error) {
	if reflect.TypeOf(dest).Kind() != reflect.Slice {
		return nil, errors.New("datastore: dest should be a slice")
	}
	c := &multiConverter{
		dest:     dest,
		size:     size,
		sliceTyp: reflect.TypeOf(dest).Elem(),
		sliceVal: reflect.ValueOf(dest),
	}
	if c.sliceVal.Len() < size {
		return nil, errors.New("datastore: dest length is smaller than the number of the results")
	}
	// pre-init the item values if nil
	for i := 0; i < size; i++ {
		v := c.sliceVal.Index(i)
		if v.IsNil() && c.sliceTyp.Kind() == reflect.Interface {
			return nil, errors.New("datastore: interface{} slice with nil items are not allowed")
		}
		if v.IsNil() {
			v.Set(reflect.New(c.typeOfElem(i)))
		}
	}
	return c, nil
}

func (c *multiConverter) set(i int, proto *pb.Entity) {
	if i < 0 || i >= c.size {
		return
	}
	protoToEntity(proto, c.sliceVal.Index(i).Interface())
}

func (c *multiConverter) typeOfElem(i int) reflect.Type {
	if c.sliceTyp.Kind() == reflect.Interface {
		return c.sliceVal.Index(i).Elem().Type().Elem()
	}
	return c.sliceVal.Index(i).Type().Elem()
}
