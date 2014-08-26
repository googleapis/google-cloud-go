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

	"github.com/golang/oauth2"
	"github.com/golang/oauth2/google"

	pb "google.golang.org/cloud/internal/datastore"
)

var (
	ErrNotFound = errors.New("datastore: no entity with the specified key has been found")
)

var requiredScopes = []string{
	"https://www.googleapis.com/auth/datastore",
	"https://www.googleapis.com/auth/userinfo.email",
}

type Dataset struct {
	tx        *Tx
	namespace string
}

func NewDataset(projectID, email string, privateKey []byte) (*Dataset, error) {
	return NewDatasetWithNS(projectID, "", email, privateKey)
}

func NewDatasetWithNS(projectID, ns, email string, privateKey []byte) (*Dataset, error) {
	conf, err := google.NewServiceAccountConfig(&oauth2.JWTOptions{
		Email:      email,
		PrivateKey: privateKey,
		Scopes:     requiredScopes,
	})
	if err != nil {
		return nil, err
	}
	return &Dataset{
		namespace: ns,
		tx: &Tx{
			datasetID: projectID,
			client:    &client{transport: conf.NewTransport()},
		},
	}, nil
}

func (d *Dataset) NewNamedKey(kind string, name string) *Key {
	return &Key{namespace: d.namespace, kind: kind, name: name}
}

func (d *Dataset) NewKey(kind string, id int64) *Key {
	return &Key{namespace: d.namespace, kind: kind, id: id}
}

func (d *Dataset) NewIncompleteKey(kind string) *Key {
	return &Key{namespace: d.namespace, kind: kind}
}

// NewQuery creates a new Query for a specific entity kind.
//
// An empty kind means to return all entities, including entities created and
// managed by other App Engine features, and is called a kindless query.
// Kindless queries cannot include filters or sort orders on property values.
func (d *Dataset) NewQuery(kinds ...string) *Query {
	return &Query{
		namespace: d.namespace,
		kinds:     kinds,
		limit:     -1,
	}
}

func (d *Dataset) Get(keys []*Key, dest interface{}) (err error) {
	return d.tx.Get(keys, dest)
}

// Put upserts the objects identified with provided keys. If one or
// more keys are incomplete, backend generates unique numeric identifiers.
func (d *Dataset) Put(keys []*Key, src interface{}) ([]*Key, error) {
	return d.tx.Put(keys, src)
}

// Delete deletes the object identified with the provided key.
func (d *Dataset) Delete(keys []*Key) (err error) {
	return d.tx.Delete(keys)
}

// AllocateIDs allocates n new IDs from the dataset's namespace and of
// the provided kind. If no namespace provided, default is used.
func (d *Dataset) AllocateIDs(kind string, n int) (keys []*Key, err error) {
	if n <= 0 {
		err = errors.New("datastore: n should be bigger than zero")
		return
	}
	key := keyToProto(d.NewIncompleteKey(kind))
	incompleteKeys := make([]*pb.Key, n)
	for i := 0; i < n; i++ {
		incompleteKeys[i] = key
	}
	req := &pb.AllocateIdsRequest{Key: incompleteKeys}
	resp := &pb.AllocateIdsResponse{}

	url := d.tx.newUrl("allocateIds")
	if err = d.tx.client.call(url, req, resp); err != nil {
		return
	}
	// TODO(jbd): Return error if response doesn't include enough keys.
	keys = make([]*Key, n)
	for i := 0; i < n; i++ {
		created := resp.GetKey()[i]
		keys[i] = protoToKey(created)
	}
	return
}

func (d *Dataset) RunQuery(q *Query, dest interface{}) (keys []*Key, nextQuery *Query, err error) {
	return d.tx.RunQuery(q, dest)
}

// NewTx begins a transaction.
func (d *Dataset) NewTx() (*Tx, error) {
	req := &pb.BeginTransactionRequest{}
	resp := &pb.BeginTransactionResponse{}
	url := d.tx.newUrl("beginTransaction")
	if err := d.tx.client.call(url, req, resp); err != nil {
		return nil, err
	}
	tx := &Tx{
		datasetID: d.tx.datasetID,
		client:    d.tx.client,
		id:        resp.GetTransaction(),
	}
	return tx, nil
}
