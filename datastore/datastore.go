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

	"code.google.com/p/goprotobuf/proto"
	pb "google.golang.org/cloud/internal/datastore"
)

var (
	// ErrInvalidEntityType is returned when functions like Get or Next are
	// passed a dst or src argument of invalid type.
	ErrInvalidEntityType = errors.New("datastore: invalid entity type")
	// ErrInvalidKey is returned when an invalid key is presented.
	ErrInvalidKey = errors.New("datastore: invalid key")
	// ErrNoSuchEntity is returned when no entity was found for a given key.
	ErrNoSuchEntity = errors.New("datastore: no such entity")
)

type multiArgType int

const (
	multiArgTypeInvalid multiArgType = iota
	multiArgTypePropertyLoadSaver
	multiArgTypeStruct
	multiArgTypeStructPtr
	multiArgTypeInterface
)

// ErrFieldMismatch is returned when a field is to be loaded into a different
// type than the one it was stored from, or when a field is missing or
// unexported in the destination struct.
// StructType is the type of the struct pointed to by the destination argument
// passed to Get or to Iterator.Next.
type ErrFieldMismatch struct {
	StructType reflect.Type
	FieldName  string
	Reason     string
}

func (e *ErrFieldMismatch) Error() string {
	return fmt.Sprintf("datastore: cannot load field %q into a %q: %s",
		e.FieldName, e.StructType, e.Reason)
}

func keyToProto(k *Key) *pb.Key {
	if k == nil {
		return nil
	}

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

// multiKeyToProto is a batch version of keyToProto.
func multiKeyToProto(keys []*Key) []*pb.Key {
	ret := make([]*pb.Key, len(keys))
	for i, k := range keys {
		ret[i] = keyToProto(k)
	}
	return ret
}

// multiKeyToProto is a batch version of keyToProto.
func multiProtoToKey(keys []*pb.Key) []*Key {
	ret := make([]*Key, len(keys))
	for i, k := range keys {
		ret[i] = protoToKey(k)
	}
	return ret
}

// multiValid is a batch version of Key.valid. It returns an error, not a
// []bool.
func multiValid(key []*Key) error {
	invalid := false
	for _, k := range key {
		if !k.valid() {
			invalid = true
			break
		}
	}
	if !invalid {
		return nil
	}
	err := make(MultiError, len(key))
	for i, k := range key {
		if !k.valid() {
			err[i] = ErrInvalidKey
		}
	}
	return err
}

// checkMultiArg checks that v has type []S, []*S, []I, or []P, for some struct
// type S, for some interface type I, or some non-interface non-pointer type P
// such that P or *P implements PropertyLoadSaver.
//
// It returns what category the slice's elements are, and the reflect.Type
// that represents S, I or P.
//
// As a special case, PropertyList is an invalid type for v.
func checkMultiArg(v reflect.Value) (m multiArgType, elemType reflect.Type) {
	if v.Kind() != reflect.Slice {
		return multiArgTypeInvalid, nil
	}
	if v.Type() == typeOfPropertyList {
		return multiArgTypeInvalid, nil
	}
	elemType = v.Type().Elem()
	if reflect.PtrTo(elemType).Implements(typeOfPropertyLoadSaver) {
		return multiArgTypePropertyLoadSaver, elemType
	}
	switch elemType.Kind() {
	case reflect.Struct:
		return multiArgTypeStruct, elemType
	case reflect.Interface:
		return multiArgTypeInterface, elemType
	case reflect.Ptr:
		elemType = elemType.Elem()
		if elemType.Kind() == reflect.Struct {
			return multiArgTypeStructPtr, elemType
		}
	}
	return multiArgTypeInvalid, nil
}

// Get loads the entity stored for k into dst, which must be a struct pointer
// or implement PropertyLoadSaver. If there is no such entity for the key, Get
// returns ErrNoSuchEntity.
//
// The values of dst's unmatched struct fields are not modified, and matching
// slice-typed fields are not reset before appending to them. In particular, it
// is recommended to pass a pointer to a zero valued struct on each Get call.
//
// ErrFieldMismatch is returned when a field is to be loaded into a different
// type than the one it was stored from, or when a field is missing or
// unexported in the destination struct. ErrFieldMismatch is only returned if
// dst is a struct pointer.
func Get(c *Client, key *Key, dst interface{}) error {
	err := GetMulti(c, []*Key{key}, []interface{}{dst})
	if me, ok := err.(MultiError); ok {
		return me[0]
	}
	return err
}

// GetMulti is a batch version of Get.
//
// dst must be a []S, []*S, []I or []P, for some struct type S, some interface
// type I, or some non-interface non-pointer type P such that P or *P
// implements PropertyLoadSaver. If an []I, each element must be a valid dst
// for Get: it must be a struct pointer or implement PropertyLoadSaver.
//
// As a special case, PropertyList is an invalid type for dst, even though a
// PropertyList is a slice of structs. It is treated as invalid to avoid being
// mistakenly passed when []PropertyList was intended.
func GetMulti(c *Client, key []*Key, dst interface{}) error {
	v := reflect.ValueOf(dst)
	multiArgType, _ := checkMultiArg(v)
	if multiArgType == multiArgTypeInvalid {
		return errors.New("datastore: dst has invalid type")
	}
	if len(key) != v.Len() {
		return errors.New("datastore: key and dst slices have different length")
	}
	if len(key) == 0 {
		return nil
	}
	if err := multiValid(key); err != nil {
		return err
	}
	req := &pb.LookupRequest{
		Key: multiKeyToProto(key),
	}
	if c.transaction != nil {
		req.ReadOptions = &pb.ReadOptions{Transaction: c.transaction}
	}

	res := &pb.LookupResponse{}
	if err := c.call("Lookup", req, res); err != nil {
		return err
	}
	if len(key) != len(res.Found) {
		return errors.New("datastore: internal error: server returned the wrong number of entities")
	}
	multiErr, any := make(MultiError, len(key)), false
	for i, e := range res.Found {
		if e.Entity == nil {
			multiErr[i] = ErrNoSuchEntity
		} else {
			elem := v.Index(i)
			if multiArgType == multiArgTypePropertyLoadSaver || multiArgType == multiArgTypeStruct {
				elem = elem.Addr()
			}
			multiErr[i] = loadEntity(elem.Interface(), e.Entity)
		}
		if multiErr[i] != nil {
			any = true
		}
	}
	if any {
		return multiErr
	}
	return nil
}

// Put saves the entity src into the datastore with key k. src must be a struct
// pointer or implement PropertyLoadSaver; if a struct pointer then any
// unexported fields of that struct will be skipped. If k is an incomplete key,
// the returned key will be a unique key generated by the datastore.
func Put(c *Client, key *Key, src interface{}) (*Key, error) {
	k, err := PutMulti(c, []*Key{key}, []interface{}{src})
	if err != nil {
		if me, ok := err.(MultiError); ok {
			return nil, me[0]
		}
		return nil, err
	}
	return k[0], nil
}

// PutMulti is a batch version of Put.
//
// src must satisfy the same conditions as the dst argument to GetMulti.
func PutMulti(c *Client, keys []*Key, src interface{}) ([]*Key, error) {
	v := reflect.ValueOf(src)
	multiArgType, _ := checkMultiArg(v)
	if multiArgType == multiArgTypeInvalid {
		return nil, errors.New("datastore: src has invalid type")
	}
	if len(keys) != v.Len() {
		return nil, errors.New("datastore: key and src slices have different length")
	}
	if len(keys) == 0 {
		return nil, nil
	}

	if err := multiValid(keys); err != nil {
		return nil, err
	}

	autoIdIndex := []int{}
	autoId := []*pb.Entity(nil)
	upsert := []*pb.Entity(nil)
	for i, k := range keys {
		val := reflect.ValueOf(src).Index(i)
		// If src is an interface slice []interface{}{ent1, ent2}
		if val.Kind() == reflect.Interface {
			val = val.Elem()
		}
		// If src is a slice of ptrs []*T{ent1, ent2}
		if val.Kind() == reflect.Ptr {
			val = val.Elem()
		}

		p, err := saveEntity(k, val.Interface())
		if err != nil {
			return nil, fmt.Errorf("datastore: Error while saving %v: %v", k.String(), err)
		}

		if k.Incomplete() {
			autoIdIndex = append(autoIdIndex, i)
			autoId = append(autoId, p)
		} else {
			upsert = append(upsert, p)
		}
	}

	req := &pb.CommitRequest{
		Mutation: &pb.Mutation{
			InsertAutoId: autoId,
			Upsert:       upsert,
		}}

	if c.transaction != nil {
		req.Transaction = c.transaction
		req.Mode = pb.CommitRequest_TRANSACTIONAL.Enum()
	} else {
		req.Mode = pb.CommitRequest_NON_TRANSACTIONAL.Enum()
	}

	res := &pb.CommitResponse{}
	if err := c.call("Commit", req, res); err != nil {
		return nil, err
	}
	if len(autoId) != len(res.MutationResult.InsertAutoIdKey) {
		return nil, errors.New("datastore: internal error: server returned the wrong number of keys")
	}
	ret := make([]*Key, len(keys))
	for i := range ret {
		ret[i] = protoToKey(res.MutationResult.InsertAutoIdKey[i])
		if ret[i].Incomplete() {
			return nil, errors.New("datastore: internal error: server returned an invalid key")
		}
	}
	return ret, nil
}

// Delete deletes the entity for the given key.
func Delete(c *Client, key *Key) error {
	err := DeleteMulti(c, []*Key{key})
	if me, ok := err.(MultiError); ok {
		return me[0]
	}
	return err
}

// DeleteMulti is a batch version of Delete.
func DeleteMulti(c *Client, keys []*Key) error {
	protoKeys := make([]*pb.Key, len(keys))
	for i, k := range keys {
		protoKeys[i] = keyToProto(k)
	}

	req := &pb.CommitRequest{
		Mutation: &pb.Mutation{
			Delete: protoKeys,
		},
	}

	if c.transaction != nil {
		req.Transaction = c.transaction
		req.Mode = pb.CommitRequest_TRANSACTIONAL.Enum()
	} else {
		req.Mode = pb.CommitRequest_NON_TRANSACTIONAL.Enum()
	}

	resp := &pb.CommitResponse{}
	return c.call("Commit", req, resp)
}
