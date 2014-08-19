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

type Path struct {
	Kind string
	ID   int64
	Name string
}

// Key represents the datastore key for a stored entity, and is immutable.
type Key struct {
	fullPath  []*Path
	namespace string
}

// TODO(jbd): Does the path needs to be immutable?

func (k *Key) Path() []*Path {
	return k.fullPath
}

func (k *Key) Namespace() string {
	return k.namespace
}

// Complete returns whether the key does not refer to a stored entity.
func (k *Key) Complete() bool {
	for _, p := range k.fullPath {
		if p.Name == "" && p.ID == 0 {
			return false
		}
	}
	return true
}

func (k *Key) Equal(o *Key) bool {
	if k == nil || o == nil {
		return k == o // if either is nil, both must be nil
	}
	if k.namespace != o.namespace || len(k.fullPath) != len(o.fullPath) {
		return false
	}
	for i, p := range k.fullPath {
		oPath := o.fullPath[i]
		if p.ID != oPath.ID || p.Name != oPath.Name || p.Kind != oPath.Kind {
			return false
		}
	}
	return true
}
