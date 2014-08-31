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

// Key represents the datastore key for a stored entity, and is immutable.
type Key struct {
	kind   string
	id     int64
	name   string
	parent *Key

	namespace string
}

func (k *Key) Kind() string {
	return k.kind
}

func (k *Key) ID() int64 {
	return k.id
}

func (k *Key) Name() string {
	return k.name
}

func (k *Key) Parent() *Key {
	return k.parent
}

func (k *Key) SetParent(v *Key) {
	if !v.IsComplete() {
		panic("can't set an incomplete key as parent")
	}
	k.parent = v
}

func (k *Key) Namespace() string {
	return k.namespace
}

// Complete returns whether the key does not refer to a stored entity.
func (k *Key) IsComplete() bool {
	return k.name != "" || k.id != 0
}

func (k *Key) IsEqual(o *Key) bool {
	for {
		if k == nil || o == nil {
			return k == o // if either is nil, both must be nil
		}
		if k.namespace != o.namespace || k.name != o.name || k.id != o.id || k.kind != o.kind {
			return false
		}
		if k.parent == nil && o.parent == nil {
			return true
		}
		k = k.parent
		o = o.parent
	}
}
