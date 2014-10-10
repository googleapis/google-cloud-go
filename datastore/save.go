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
	"fmt"
	"reflect"
	"time"

	"code.google.com/p/goprotobuf/proto"

	pb "google.golang.org/cloud/internal/datastore"
)

// valueToProto converts a named value to a newly allocated Property.
// The returned error string is empty on success.
func valueToProto(v reflect.Value) (p *pb.Value, errStr string) {
	var (
		pv          pb.Value
		unsupported bool
	)
	switch v.Kind() {
	case reflect.Invalid:
		// No-op.
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		pv.IntegerValue = proto.Int64(v.Int())
	case reflect.Bool:
		pv.BooleanValue = proto.Bool(v.Bool())
	case reflect.String:
		pv.StringValue = proto.String(v.String())
	case reflect.Float32, reflect.Float64:
		pv.DoubleValue = proto.Float64(v.Float())
	case reflect.Ptr:
		if k, ok := v.Interface().(*Key); ok {
			if k != nil {
				pv.KeyValue = keyToProto(k)
			}
		} else {
			unsupported = true
		}
	case reflect.Struct:
		switch t := v.Interface().(type) {
		case time.Time:
			if t.Before(minTime) || t.After(maxTime) {
				return nil, "time value out of range"
			}
			pv.TimestampMicrosecondsValue = proto.Int64(toUnixMicro(t))
		default:
			unsupported = true
		}
	case reflect.Slice:
		if b, ok := v.Interface().([]byte); ok {
			pv.BlobValue = b
		} else {
			// nvToProto should already catch slice values.
			// If we get here, we have a slice of slice values.
			unsupported = true
		}
	default:
		unsupported = true
	}
	if unsupported {
		return nil, "unsupported datastore value type: " + v.Type().String()
	}

	return &pv, ""
}

// saveEntity saves an EntityProto into a PropertyLoadSaver or struct pointer.
func saveEntity(key *Key, src interface{}) (*pb.Entity, error) {
	var err error
	var props []Property
	if e, ok := src.(PropertyLoadSaver); ok {
		props, err = e.Save()
	} else {
		props, err = SaveStruct(src)
	}
	if err != nil {
		return nil, err
	}
	return propertiesToProto(key, props)
}

func saveStructProperty(props *[]Property, name string, noIndex bool, v reflect.Value) error {
	p := Property{
		Name:    name,
		NoIndex: noIndex,
	}
	switch x := v.Interface().(type) {
	case *Key, time.Time, BlobKey:
		p.Value = x
	default:
		switch v.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			p.Value = v.Int()
		case reflect.Bool:
			p.Value = v.Bool()
		case reflect.String:
			p.Value = v.String()
		case reflect.Float32, reflect.Float64:
			p.Value = v.Float()
		case reflect.Slice:
			if v.Type().Elem().Kind() == reflect.Uint8 {
				p.NoIndex = true
				p.Value = v.Bytes()
			}
		case reflect.Struct:
			if !v.CanAddr() {
				return fmt.Errorf("datastore: unsupported struct field: value is unaddressable")
			}
			sub, err := newStructPLS(v.Addr().Interface())
			if err != nil {
				return fmt.Errorf("datastore: unsupported struct field: %v", err)
			}
			return sub.(structPLS).save(props, name, noIndex)
		}
	}
	if p.Value == nil {
		return fmt.Errorf("datastore: unsupported struct field type: %v", v.Type())
	}
	*props = append(*props, p)
	return nil
}

func (s structPLS) Save() ([]Property, error) {
	var props []Property
	if err := s.save(&props, "", false); err != nil {
		return nil, err
	}
	return props, nil
}

func (s structPLS) save(props *[]Property, prefix string, noIndex bool) error {
	for i, t := range s.codec.byIndex {
		if t.name == "-" {
			continue
		}
		name := t.name
		if prefix != "" {
			name = prefix + name
		}
		v := s.v.Field(i)
		if !v.IsValid() || !v.CanSet() {
			continue
		}
		noIndex1 := noIndex || t.noIndex
		// For slice fields that aren't []byte, save each element.
		if v.Kind() == reflect.Slice && v.Type().Elem().Kind() != reflect.Uint8 {
			for j := 0; j < v.Len(); j++ {
				if err := saveStructProperty(props, name, noIndex1, v.Index(j)); err != nil {
					return err
				}
			}
			continue
		}
		// Otherwise, save the field itself.
		if err := saveStructProperty(props, name, noIndex1, v); err != nil {
			return err
		}
	}
	return nil
}

func propertiesToProto(key *Key, props []Property) (*pb.Entity, error) {
	e := &pb.Entity{
		Key: keyToProto(key),
	}

	indexedProps := 0
	for _, p := range props {
		x := &pb.Property{
			Name:  proto.String(p.Name),
			Value: new(pb.Value),
		}
		switch v := p.Value.(type) {
		case int64:
			x.Value.IntegerValue = proto.Int64(v)
		case bool:
			x.Value.BooleanValue = proto.Bool(v)
		case string:
			x.Value.StringValue = proto.String(v)
		case float64:
			x.Value.DoubleValue = proto.Float64(v)
		case *Key:
			if v != nil {
				x.Value.KeyValue = keyToProto(v)
			}
		case time.Time:
			if v.Before(minTime) || v.After(maxTime) {
				return nil, fmt.Errorf("datastore: time value out of range")
			}
			x.Value.TimestampMicrosecondsValue = proto.Int64(toUnixMicro(v))
		case BlobKey:
			x.Value.BlobKeyValue = proto.String(string(v))
		case []byte:
			x.Value.BlobValue = v
			if !p.NoIndex {
				return nil, fmt.Errorf("datastore: cannot index a []byte valued Property with Name %q", p.Name)
			}
		default:
			if p.Value != nil {
				return nil, fmt.Errorf("datastore: invalid Value type for a Property with Name %q", p.Name)
			}
		}

		if !p.NoIndex {
			rVal := reflect.ValueOf(p.Value)
			if rVal.Kind() == reflect.Slice {
				indexedProps += rVal.Len()
			} else {
				indexedProps++
			}
		}

		x.Value.Indexed = proto.Bool(!p.NoIndex)

		if indexedProps > maxIndexedProperties {
			return nil, errors.New("datastore: too many indexed properties")
		}

		e.Property = append(e.Property, x)
	}

	return e, nil
}
