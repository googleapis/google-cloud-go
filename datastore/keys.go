// Copyright 2014 Google Inc. All rights reserved.
// Use of this source code is governed by the Apache 2.0
// license that can be found in the LICENSE file.

package datastore

import (
	"bytes"
	"strconv"
)

// Key represents the datastore key for a stored entity, and is immutable.
type Key struct {
	kind     string
	stringID string
	intID    int64
	name     string

	datasetID string // project ID
	namespace string
}

// Kind returns the key's kind (also known as entity type).
func (k *Key) Kind() string {
	return k.kind
}

// StringID returns the key's string ID (also known as an entity name or key
// name), which may be "".
func (k *Key) StringID() string {
	return k.stringID
}

// IntID returns the key's integer ID, which may be 0.
func (k *Key) IntID() int64 {
	return k.intID
}

// Name returns the key's name.
func (k *Key) Name() string {
	return k.name
}

// Parent returns the key's dataset ID.
func (k *Key) DatasetID() string {
	return k.datasetID
}

// Namespace returns the key's namespace.
func (k *Key) Namespace() string {
	return k.namespace
}

// Incomplete returns whether the key does not refer to a stored entity.
// In particular, whether the key has a zero StringID and a zero IntID.
func (k *Key) Incomplete() bool {
	return k.stringID == "" && k.intID == 0
}

// Equal returns whether two keys are equal.
func (k *Key) Equal(o *Key) bool {
	for k != nil && o != nil {
		if k.kind != o.kind || k.stringID != o.stringID || k.intID != o.intID || k.namespace != o.namespace || k.datasetID != o.datasetID {
			return false
		}
	}
	// TODO(jbd): Add name based equals
	return true
}

// marshal marshals the key's string representation to the buffer.
func (k *Key) marshal(b *bytes.Buffer) {
	b.WriteByte('/')
	b.WriteString(k.kind)
	b.WriteByte(',')
	if k.stringID != "" {
		b.WriteString(k.stringID)
	} else {
		b.WriteString(strconv.FormatInt(k.intID, 10))
	}
}

// String returns a string representation of the key.
func (k *Key) String() string {
	if k == nil {
		return ""
	}
	b := bytes.NewBuffer(make([]byte, 0, 512))
	k.marshal(b)
	return b.String()
}

func newIncompleteKey(kind, datasetID, namespace string) *Key {
	return newKey(kind, "", 0, datasetID, namespace)
}

// NewKey creates a new key.
// kind cannot be empty.
// Either one or both of stringID and intID must be zero. If both are zero,
// the key returned is incomplete.
// parent must either be a complete key or nil.
func newKey(kind, stringID string, intID int64, datasetID, namespace string) *Key {
	return &Key{
		kind:      kind,
		stringID:  stringID,
		intID:     intID,
		datasetID: datasetID,
		namespace: namespace,
	}
}
