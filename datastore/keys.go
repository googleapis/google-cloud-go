// Copyright 2014 Google Inc. All rights reserved.
// Use of this source code is governed by the Apache 2.0
// license that can be found in the LICENSE file.

package datastore

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
)

// Key represents the datastore key for a stored entity, and is immutable.
type Key struct {
	kind     string
	stringID string
	intID    int64

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

func NewIncompleteKey(kind, datasetID, namespace string) *Key {
	return NewKey(kind, "", 0, datasetID, namespace)
}

// NewKey creates a new key.
// kind cannot be empty.
// Either one or both of stringID and intID must be zero. If both are zero,
// the key returned is incomplete.
// parent must either be a complete key or nil.
func NewKey(kind, stringID string, intID int64, datasetID, namespace string) *Key {
	return &Key{
		kind:      kind,
		stringID:  stringID,
		intID:     intID,
		datasetID: datasetID,
		namespace: namespace,
	}
}

// AllocateIDs returns a range of n integer IDs with the given kind and parent
// combination. kind cannot be empty; parent may be nil. The IDs in the range
// returned will not be used by the datastore's automatic ID sequence generator
// and may be used with NewKey without conflict.
//
// The range is inclusive at the low end and exclusive at the high end. In
// other words, valid intIDs x satisfy low <= x && x < high.
//
// If no error is returned, low + n == high.
func AllocateIDs(kind string, parent *Key, n int) (low, high int64, err error) {
	if kind == "" {
		return 0, 0, errors.New("datastore: AllocateIDs given an empty kind")
	}
	if n < 0 {
		return 0, 0, fmt.Errorf("datastore: AllocateIDs given a negative count: %d", n)
	}
	if n == 0 {
		return 0, 0, nil
	}
	// TODO(jbd): Make the request.
	return 0, 0, nil
}
