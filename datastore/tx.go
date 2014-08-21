package datastore

import (
	"errors"
	"net/http"
	"reflect"

	"code.google.com/p/goprotobuf/proto"
	pb "google.golang.org/cloud/internal/datastore"
)

var (
	errKeyIncomplete = errors.New("datastore: key is incomplete, provide a complete key")
)

type Tx struct {
	id        []byte
	datasetID string
	transport http.RoundTripper
}

// IsTransactional returns true if the transaction has a non-zero
// transaction ID.
func (t *Tx) IsTransactional() bool {
	return len(t.id) > 0
}

func (t *Tx) RunQuery(q *Query, dest interface{}) (keys []*Key, nextQuery *Query, err error) {
	if q.err != nil {
		return nil, nil, q.err
	}
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
		keys[i] = keyFromKeyProto(e.GetEntity().GetKey())
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
func (t *Tx) Commit() error {
	if !t.IsTransactional() {
		return errors.New("datastore: non-transactional operation")
	}
	req := &pb.CommitRequest{
		Mode:        pb.CommitRequest_TRANSACTIONAL.Enum(),
		Transaction: t.id,
	}
	resp := &pb.CommitResponse{}
	if err := t.newClient().call(t.newUrl("commit"), req, resp); err != nil {
		return err
	}
	return nil
}

// Rollback rollbacks the transaction.
func (t *Tx) Rollback() error {
	if !t.IsTransactional() {
		return errors.New("datastore: non-transactional operation")
	}
	req := &pb.RollbackRequest{
		Transaction: t.id,
	}
	resp := &pb.RollbackResponse{}
	if err := t.newClient().call(t.newUrl("rollback"), req, resp); err != nil {
		return err
	}
	return nil
}

// TODO(jbd): Implement GetAll, PutAll and DeleteAll.

// Get looks up for the object identified with the provided key
// in the scope of the current transaction.
func (t *Tx) Get(key *Key, dest interface{}) (err error) {
	if !key.IsComplete() {
		return errKeyIncomplete
	}
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

// Put upserts the object identified with key in the scope
// of the current transaction.
// It returns the complete key if key is incomplete.
func (t *Tx) Put(key *Key, src interface{}) (k *Key, err error) {
	if !isPtrOfStruct(src) {
		err = errors.New("datastore: dest should be a pointer of a struct")
		return
	}
	// Determine mod depending on if this is the default
	// transaction or not.
	mode := pb.CommitRequest_NON_TRANSACTIONAL.Enum()
	if t.IsTransactional() {
		mode = pb.CommitRequest_TRANSACTIONAL.Enum()
	}

	// TODO(jbd): Handle indexes.
	entity := []*pb.Entity{entityToEntityProto(key, reflect.ValueOf(src).Elem())}
	req := &pb.CommitRequest{
		Transaction: t.id,
		Mode:        mode,
		Mutation:    &pb.Mutation{},
	}

	if !key.IsComplete() {
		req.Mutation.InsertAutoId = entity
	} else {
		req.Mutation.Upsert = entity
	}

	resp := &pb.CommitResponse{}
	if err = t.newClient().call(t.newUrl("commit"), req, resp); err != nil {
		return
	}

	autoKey := resp.GetMutationResult().GetInsertAutoIdKey()
	if len(autoKey) > 0 {
		k = keyFromKeyProto(autoKey[0])
	} else {
		k = key
	}
	return
}

// Delete deletes the object identified with the specified key in
// the transaction.
func (t *Tx) Delete(key *Key) (err error) {
	if !key.IsComplete() {
		return errKeyIncomplete
	}
	// Determine mod depending on if this is the default
	// transaction or not.
	mode := pb.CommitRequest_NON_TRANSACTIONAL.Enum()
	if t.IsTransactional() {
		mode = pb.CommitRequest_TRANSACTIONAL.Enum()
	}

	req := &pb.CommitRequest{
		Transaction: t.id,
		Mutation: &pb.Mutation{
			Delete: []*pb.Key{keyToPbKey(key)},
		},
		Mode: mode,
	}
	resp := &pb.CommitResponse{}
	return t.newClient().call(t.newUrl("commit"), req, resp)
}

func (t *Tx) newClient() *client {
	return &client{transport: t.transport}
}

func (t *Tx) newUrl(method string) string {
	// TODO(jbd): Provide support for non-prod instances.
	return "https://www.googleapis.com/datastore/v1beta2/datasets/" + t.datasetID + "/" + method
}
