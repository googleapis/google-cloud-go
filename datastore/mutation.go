// Copyright 2018 Google LLC
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

	pb "cloud.google.com/go/datastore/apiv1/datastorepb"
)

// A Mutation represents a change to a Datastore entity.
type Mutation struct {
	key *Key // needed for transaction PendingKeys and to dedup deletions
	mut *pb.Mutation

	// err is set to a Datastore or gRPC error, if Mutation is not valid
	// (see https://godoc.org/google.golang.org/grpc/codes).
	err error
}

func (m *Mutation) isDelete() bool {
	_, ok := m.mut.Operation.(*pb.Mutation_Delete)
	return ok
}

// WithTransforms adds one or more server-side property transformations to the mutation.
// It can be called multiple times to add more transforms.
// The order of transforms is preserved, first by the order of calls to WithTransforms,
// and then by the order of transforms within a single call.
//
// Usage with NewUpsert: By default, NewUpsert replaces the entire entity.
// To apply transforms without replacing the existing entity (e.g. only incrementing a counter),
// call WithPropertyMask on the mutation with an empty slice.
//
// Note: Transforms are applied *after* the Upsert/Update operation.
// The PropertyMask controls what the Upsert/Update writes (the "base" for the transform).
//
// Transforms are applied to this base entity even if the property was excluded from
// the mask.
//
// - nil mask: Base is the new entity (full replacement).
// - empty mask: Base is the existing entity (no-op update).
func (m *Mutation) WithTransforms(transforms ...PropertyTransform) *Mutation {
	if m.err != nil {
		return m
	}
	if m.mut == nil {
		m.err = errors.New("datastore: WithTransforms called on uninitialized mutation")
		return m
	}
	if m.isDelete() {
		m.err = errors.New("datastore: property transforms cannot be applied to a delete mutation")
		return m
	}

	for _, transform := range transforms {
		if transform.pb == nil {
			m.err = errors.New("datastore: WithTransforms called with an uninitialized PropertyTransform")
			return m
		}
		m.mut.PropertyTransforms = append(m.mut.PropertyTransforms, transform.pb)
	}
	return m
}

// WithPropertyMask specifies which properties of the entity should be updated.
//
// If paths is empty, no properties will be updated (and the entity will not be replaced).
// This is useful when performing a "transform-only" operation where you only want to
// apply PropertyTransforms without modifying other parts of the entity.
//
// If WithPropertyMask is not called, the default behavior for Upsert and Update
// is to replace the entire entity.
func (m *Mutation) WithPropertyMask(paths ...string) *Mutation {
	if m.err != nil {
		return m
	}
	if m.mut == nil {
		m.err = errors.New("datastore: WithPropertyMask called on uninitialized mutation")
		return m
	}
	if _, isInsert := m.mut.GetOperation().(*pb.Mutation_Insert); isInsert || m.isDelete() {
		m.err = errors.New("datastore: property mask can only be applied to update or upsert mutations")
		return m
	}

	// We permit an empty slice for paths, which means "update no properties".
	// However, we must ensure m.mut.PropertyMask is not nil in that case.
	if m.mut.PropertyMask == nil {
		m.mut.PropertyMask = &pb.PropertyMask{}
	}
	m.mut.PropertyMask.Paths = append(m.mut.PropertyMask.Paths, paths...)
	return m
}

// NewInsert creates a Mutation that will save the entity src into the
// datastore with key k. If k already exists, calling Mutate with the
// Mutation will lead to a gRPC codes.AlreadyExists error.
func NewInsert(k *Key, src interface{}) *Mutation {
	if !k.valid() {
		return &Mutation{err: ErrInvalidKey}
	}
	p, err := saveEntity(k, src)
	if err != nil {
		return &Mutation{err: err}
	}
	return &Mutation{
		key: k,
		mut: &pb.Mutation{Operation: &pb.Mutation_Insert{Insert: p}},
	}
}

// NewUpsert creates a Mutation that saves the entity src into the datastore with key
// k, whether or not k exists. See Client.Put for valid values of src.
//
// By default, NewUpsert replaces the entire entity. To perform a partial update
// or a "transform-only" operation (e.g. only incrementing a counter),
// call WithPropertyMask on the returned Mutation.
func NewUpsert(k *Key, src interface{}) *Mutation {
	if !k.valid() {
		return &Mutation{err: ErrInvalidKey}
	}
	p, err := saveEntity(k, src)
	if err != nil {
		return &Mutation{err: err}
	}
	return &Mutation{
		key: k,
		mut: &pb.Mutation{Operation: &pb.Mutation_Upsert{Upsert: p}},
	}
}

// NewUpdate creates a Mutation that replaces the entity in the datastore with
// key k. If k does not exist, calling Mutate with the Mutation will lead to a
// gRPC codes.NotFound error.
// See Client.Put for valid values of src.
func NewUpdate(k *Key, src interface{}) *Mutation {
	if !k.valid() {
		return &Mutation{err: ErrInvalidKey}
	}
	if k.Incomplete() {
		return &Mutation{err: fmt.Errorf("datastore: can't update the incomplete key: %v", k)}
	}
	p, err := saveEntity(k, src)
	if err != nil {
		return &Mutation{err: err}
	}
	return &Mutation{
		key: k,
		mut: &pb.Mutation{Operation: &pb.Mutation_Update{Update: p}},
	}
}

// NewDelete creates a Mutation that deletes the entity with key k.
func NewDelete(k *Key) *Mutation {
	if !k.valid() {
		return &Mutation{err: ErrInvalidKey}
	}
	if k.Incomplete() {
		return &Mutation{err: fmt.Errorf("datastore: can't delete the incomplete key: %v", k)}
	}
	return &Mutation{
		key: k,
		mut: &pb.Mutation{Operation: &pb.Mutation_Delete{Delete: keyToProto(k)}},
	}
}

func mutationProtos(muts []*Mutation) ([]*pb.Mutation, error) {
	// If any of the mutations have errors, collect and return them.
	var merr MultiError
	for i, m := range muts {
		if m.err != nil {
			if merr == nil {
				merr = make(MultiError, len(muts))
			}
			merr[i] = m.err
		}
	}
	if merr != nil {
		return nil, merr
	}

	var protos []*pb.Mutation
	// Collect protos. Remove duplicate deletions (see deleteMutations).
	seen := map[string]bool{}
	for _, m := range muts {
		if m.isDelete() {
			ks := m.key.stringInternal()
			if seen[ks] {
				continue
			}
			seen[ks] = true
		}
		protos = append(protos, m.mut)
	}
	return protos, nil
}
