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
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"code.google.com/p/goprotobuf/proto"

	"github.com/golang/oauth2"
	"github.com/golang/oauth2/google"

	pb "google.golang.org/cloud/internal/datastore"
)

var (
	ErrNotFound = errors.New("datastore: no entity with the provided key has been found")
)

var requiredScopes = []string{
	"https://www.googleapis.com/auth/datastore",
	"https://www.googleapis.com/auth/userinfo.email",
}

type Dataset struct {
	defaultTransaction *Transaction
}

type Transaction struct {
	id        []byte
	datasetID string
	transport http.RoundTripper

	rolledback bool
	mu         sync.RWMutex
}

func NewDataset(projectID, clientEmail, pemFilename string) (dataset *Dataset, err error) {
	if !strings.HasPrefix(projectID, "s~") && !strings.HasPrefix(projectID, "e~") {
		projectID = "s~" + projectID
	}
	conf, err := google.NewServiceAccountConfig(&oauth2.JWTOptions{
		Email:       clientEmail,
		PemFilename: pemFilename,
		Scopes:      requiredScopes,
	})
	if err != nil {
		return
	}
	dataset = &Dataset{
		defaultTransaction: &Transaction{
			datasetID: projectID,
			transport: conf.NewTransport(),
		},
	}
	return
}

func (d *Dataset) NewIncompleteKey(kind string) *Key {
	return newIncompleteKey(kind, d.defaultTransaction.datasetID, "")
}

func (d *Dataset) NewIncompleteKeyWithNS(namespace, kind string) *Key {
	return newIncompleteKey(kind, d.defaultTransaction.datasetID, namespace)
}

func (d *Dataset) NewKey(kind string, ID int64) *Key {
	return d.NewKeyWithNS("", kind, ID)
}

func (d *Dataset) NewKeyWithNS(namespace, kind string, ID int64) *Key {
	return newKey(kind, strconv.FormatInt(ID, 10), ID, d.defaultTransaction.datasetID, namespace)
}

func (d *Dataset) Get(key *Key, dest interface{}) (err error) {
	return d.defaultTransaction.Get(key, dest)
}

func (d *Dataset) Put(key *Key, src interface{}) (k *Key, err error) {
	return d.defaultTransaction.Put(key, src)
}

// Delete deletes the object identified with the provided key.
func (d *Dataset) Delete(key *Key) (err error) {
	return d.defaultTransaction.Delete(key)
}

// AllocateIDs allocates n new IDs from the specified namespace and of
// the provided kind. If no namespace provided, default is used.
func (d *Dataset) AllocateIDs(namespace, kind string, n int) (keys []*Key, err error) {
	if n <= 0 {
		err = errors.New("datastore: n should be bigger than zero")
		return
	}
	incompleteKeys := make([]*pb.Key, n)
	for i := 0; i < n; i++ {
		incompleteKeys[i] = keyToPbKey(d.NewIncompleteKeyWithNS(namespace, kind))
	}
	req := &pb.AllocateIdsRequest{Key: incompleteKeys}
	resp := &pb.AllocateIdsResponse{}

	url := d.defaultTransaction.newUrl("allocateIds")
	if err = d.defaultTransaction.newClient().call(url, req, resp); err != nil {
		return
	}
	// TODO(jbd): Return error if response doesn't include enough keys.
	keys = make([]*Key, n)
	for i := 0; i < n; i++ {
		created := resp.GetKey()[i]
		keys[i] = keyFromKeyProto(d.defaultTransaction.datasetID, created)
	}
	return
}

func (d *Dataset) RunQuery(q *Query, dest interface{}) (keys []*Key, nextQuery *Query, err error) {
	return d.defaultTransaction.RunQuery(q, dest)
}

// RunInTransaction starts a new transaction, runs the provided function
// and automatically commits the transaction if created transaction
// hasn't rolled back. The following example gets an object, modifies
// its Name field and puts it back to datastore in the same transaction.
// If any error occurs, the transaction is rolled back. Otherwise,
// transaction is committed.
//
// 		err := ds.RunInTransaction(func(t *datastore.Transaction) {
// 			a := &someType{}
//			if err := t.Get(k, &a); err != nil {
//				t.Rollback();
//				return
//			}
//			a.Name = "new name"
//			if err := t.Put(k, &a); err != nil {
//				t.Rollback();
//				return
//			}
// 		})
//
func (d *Dataset) RunInTransaction(fn func(t *Transaction)) (err error) {
	t, err := d.NewTransaction()
	if err != nil {
		return
	}
	fn(t)
	// if not rolled back, commit the
	// transaction automatically
	if !t.IsRolledBack() {
		return t.Commit()
	}
	return
}

// NewTransaction begins a transaction and returns a Transaction instance.
func (d *Dataset) NewTransaction() (*Transaction, error) {
	transaction := &Transaction{
		transport: d.defaultTransaction.transport,
		datasetID: d.defaultTransaction.datasetID,
	}

	req := &pb.BeginTransactionRequest{}
	resp := &pb.BeginTransactionResponse{}
	url := d.defaultTransaction.newUrl("beginTransaction")
	if err := d.defaultTransaction.newClient().call(url, req, resp); err != nil {
		return nil, err
	}
	transaction.id = resp.GetTransaction()
	return transaction, nil
}

// IsTransactional returns true if the transaction has a non-zero
// transaction ID.
func (t *Transaction) IsTransactional() bool {
	return len(t.id) > 0
}

// IsRolledBack returns true if transaction is rolled back.
func (t *Transaction) IsRolledBack() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.rolledback
}

func (t *Transaction) RunQuery(q *Query, dest interface{}) (keys []*Key, nextQuery *Query, err error) {
	if !isSlicePtr(dest) {
		err = errors.New("datastore: dest should be a slice pointer")
		return
	}
	req := &pb.RunQueryRequest{
		ReadOptions: &pb.ReadOptions{
			Transaction: t.id,
		},
		PartitionId: &pb.PartitionId{
			DatasetId: proto.String(t.datasetID),
		},
		Query: queryToQueryProto(q),
	}
	if q.namespace != "" {
		req.PartitionId.Namespace = proto.String(q.namespace)
	}

	resp := &pb.RunQueryResponse{}
	if err = t.newClient().call(t.newUrl("runQuery"), req, resp); err != nil {
		return
	}

	results := resp.GetBatch().GetEntityResult()
	keys = make([]*Key, len(results))

	typ := reflect.TypeOf(dest).Elem() // type of slice
	v := reflect.MakeSlice(typ, len(results), len(results))
	for i, e := range results {
		keys[i] = keyFromKeyProto(t.datasetID, e.GetEntity().GetKey())
		obj := reflect.New(typ.Elem().Elem()).Elem()
		entityFromEntityProto(t.datasetID, e.GetEntity(), obj)

		v.Index(i).Set(reflect.New(typ.Elem().Elem())) // dest[i] = new(elType)
		v.Index(i).Elem().Set(obj)                     // dest[i] = el
	}
	reflect.ValueOf(dest).Elem().Set(v)
	if string(resp.GetBatch().GetEndCursor()) != string(q.start) {
		// next page is available
		nextQuery = q.Start(resp.GetBatch().GetEndCursor())
	}
	return
}

// Commit commits the transaction.
func (t *Transaction) Commit() error {
	if t.IsTransactional() {
		return nil
	}
	req := &pb.CommitRequest{
		Transaction: t.id,
	}
	resp := &pb.CommitResponse{}
	if err := t.newClient().call(t.newUrl("commit"), req, resp); err != nil {
		return err
	}
	return nil
}

// Rollback rollbacks the transaction.
func (t *Transaction) Rollback() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.IsTransactional() {
		return errors.New("datastore: non-transactional operation")
	}
	req := &pb.RollbackRequest{
		Transaction: t.id,
	}
	resp := &pb.RollbackResponse{}
	if err := t.newClient().call(t.newUrl("rollback"), req, resp); err != nil {
		return err
	}
	t.rolledback = true
	return nil
}

// Get looks up for the object identified with the provided key
// in the transaction.
func (t *Transaction) Get(key *Key, dest interface{}) (err error) {
	if !isPtrOfStruct(dest) {
		err = errors.New("datastore: dest should be a pointer of a struct")
		return
	}
	req := &pb.LookupRequest{
		ReadOptions: &pb.ReadOptions{
			Transaction: t.id,
		},
		Key: []*pb.Key{keyToPbKey(key)},
	}
	resp := &pb.LookupResponse{}
	if err = t.newClient().call(t.newUrl("lookup"), req, resp); err != nil {
		return
	}
	if len(resp.Found) == 0 {
		return ErrNotFound
	}

	val := reflect.ValueOf(dest).Elem()
	entityFromEntityProto(t.datasetID, resp.Found[0].Entity, val)
	return
}

// Put upserts the object identified with key in the transaction.
// Returns the complete key if key is incomplete.
func (t *Transaction) Put(key *Key, src interface{}) (k *Key, err error) {
	if !isPtrOfStruct(src) {
		err = errors.New("datastore: dest should be a pointer of a struct")
		return
	}

	// Determine mod depending on if this is the default
	// transaction or not.
	mode := pb.CommitRequest_NON_TRANSACTIONAL.Enum()
	if len(t.id) > 0 {
		mode = pb.CommitRequest_TRANSACTIONAL.Enum()
	}

	// TODO(jbd): Handle indexes.
	entity := []*pb.Entity{entityToEntityProto(key, reflect.ValueOf(src).Elem())}
	req := &pb.CommitRequest{
		Transaction: t.id,
		Mode:        mode,
		Mutation:    &pb.Mutation{},
	}
	if key.Incomplete() {
		req.Mutation.InsertAutoId = entity
	} else {
		req.Mutation.Update = entity
	}

	resp := &pb.CommitResponse{}
	if err = t.newClient().call(t.newUrl("commit"), req, resp); err != nil {
		return
	}

	autoKey := resp.GetMutationResult().GetInsertAutoIdKey()
	if len(autoKey) > 0 {
		k = keyFromKeyProto(t.datasetID, autoKey[0])
	}
	return
}

// Delete deletes the object identified with the specified key in
// the transaction.
func (t *Transaction) Delete(key *Key) (err error) {
	// Determine mod depending on if this is the default
	// transaction or not.
	mode := pb.CommitRequest_NON_TRANSACTIONAL.Enum()
	if len(t.id) > 0 {
		mode = pb.CommitRequest_TRANSACTIONAL.Enum()
	}

	req := &pb.CommitRequest{
		Transaction: t.id,
		Mutation:    &pb.Mutation{Delete: []*pb.Key{keyToPbKey(key)}},
		Mode:        mode,
	}
	resp := &pb.CommitResponse{}
	return t.newClient().call(t.newUrl("commit"), req, resp)
}

func (t *Transaction) newClient() *client {
	return &client{transport: t.transport}
}

func (t *Transaction) newUrl(method string) string {
	return "https://www.googleapis.com/datastore/v1beta2/datasets/" + t.datasetID + "/" + method
}
