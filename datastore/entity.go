package datastore

import (
	"reflect"
	"strings"

	"github.com/googlecloudplatform/gcloud-golang/datastore/pb"
)

const (
	tagKeyDatastore = "datastore"
)

func keyToPbKey(k *Key) *pb.Key {
	// TODO(jbd): Panic if dataset ID is not provided.
	pathEl := &pb.Key_PathElement{Kind: &k.kind}
	if k.intID > 0 {
		pathEl.Id = &k.intID
	} else if k.name != "" {
		pathEl.Name = &k.name
	}
	return &pb.Key{
		PartitionId: &pb.PartitionId{
			DatasetId: &k.datasetID,
			Namespace: &k.namespace,
		},
		PathElement: []*pb.Key_PathElement{pathEl},
	}
}

// TODO(jbd): Minimize reflect, cache conversion method for
// known types.
func entityToPbEntity(key *Key, src interface{}) *pb.Entity {
	panic("not yet implemented")
	return &pb.Entity{}
}

func entityFromPbEntity(e *pb.Entity, dest interface{}) {
	fieldsByDatastoreName := make(map[string]reflect.StructField)
	typ := reflect.TypeOf(dest).Elem()
	val := reflect.ValueOf(dest).Elem()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name := field.Tag.Get(tagKeyDatastore)
		if name == "" {
			name = strings.ToLower(field.Name)
		}
		// TODO(jbd): Check if name is valid
		// TODO(jbd): Handle type mismatches.
		fieldsByDatastoreName[name] = field
	}
	// TODO(jbd): Cache fieldsByDatastoreName by type
	for _, p := range e.GetProperty() {
		f, ok := fieldsByDatastoreName[p.GetName()]
		if !ok {
			// skip if not presented in the struct
			continue
		}
		// set the value
		fieldVal := val.FieldByName(f.Name)
		dsVal := p.GetValue()
		switch f.Type.String() {
		case "int":
			fieldVal.SetInt(dsVal.GetIntegerValue())
		case "bool":
			fieldVal.SetBool(dsVal.GetBooleanValue())
		case "float":
			fieldVal.SetFloat(dsVal.GetDoubleValue())
		case "string":
			fieldVal.SetString(dsVal.GetStringValue())
		case "[]byte":
			fieldVal.SetBytes(dsVal.GetBlobValue())
			// TODO(jbd): Handle Key, lists, time, other composites
		}
	}
}
