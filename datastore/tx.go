package datastore

import (
	"errors"
	"net/http"
	"reflect"

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
	// if !isSlicePtr(dest) {
	// 	err = errors.New("datastore: dest should be a slice pointer")
	// 	return
	// }
	// req := &pb.RunQueryRequest{
	// 	ReadOptions: &pb.ReadOptions{
	// 		Transaction: t.id,
	// 	},
	// 	PartitionId: &pb.PartitionId{
	// 		DatasetId: proto.String(t.datasetID),
	// 	},
	// 	Query: queryToProto(q),
	// }
	// if q.namespace != "" {
	// 	req.PartitionId.Namespace = proto.String(q.namespace)
	// }

	// resp := &pb.RunQueryResponse{}
	// if err = t.newClient().call(t.newUrl("runQuery"), req, resp); err != nil {
	// 	return
	// }

	// results := resp.GetBatch().GetEntityResult()
	// keys = make([]*Key, len(results))

	// typ := reflect.TypeOf(dest).Elem() // type of slice
	// v := reflect.MakeSlice(typ, len(results), len(results))
	// for i, e := range results {
	// 	keys[i] = protoToKey(e.GetEntity().GetKey())
	// 	obj := reflect.New(typ.Elem().Elem()).Elem()
	// 	entityFromEntityProto(e.GetEntity(), obj)

	// 	v.Index(i).Set(reflect.New(typ.Elem().Elem())) // dest[i] = new(elType)
	// 	v.Index(i).Elem().Set(obj)                     // dest[i] = el
	// }
	// reflect.ValueOf(dest).Elem().Set(v)
	// if string(resp.GetBatch().GetEndCursor()) != string(q.start) {
	// 	// next page is available
	// 	nextQuery = q.Start(resp.GetBatch().GetEndCursor())
	// }
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

func (t *Tx) Get(keys []*Key, dest interface{}) error {
	if len(keys) == 0 {
		return nil
	}
	converter, err := newMultiConverter(len(keys), dest)
	if err != nil {
		return err
	}
	protoKeys := make([]*pb.Key, len(keys))
	for i, k := range keys {
		protoKeys[i] = keyToProto(k)
	}
	req := &pb.LookupRequest{
		ReadOptions: &pb.ReadOptions{
			Transaction: t.id,
		},
		Key: protoKeys,
	}
	resp := &pb.LookupResponse{}
	if err := t.newClient().call(t.newUrl("lookup"), req, resp); err != nil {
		return err
	}
	for i, result := range resp.Found {
		converter.set(i, result)
	}
	return nil
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
		k = protoToKey(autoKey[0])
	} else {
		k = key
	}
	return
}

// Delete deletes the object identified with the specified key in
// the transaction.
func (t *Tx) Delete(keys ...*Key) (err error) {
	protoKeys := make([]*pb.Key, len(keys))
	for i, k := range keys {
		protoKeys[i] = keyToProto(k)
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
			Delete: protoKeys,
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

type multiConverter struct {
	dest interface{}
	size int

	sliceTyp reflect.Type
	elemType reflect.Type
	sliceVal reflect.Value
}

func newMultiConverter(size int, dest interface{}) (*multiConverter, error) {
	if reflect.TypeOf(dest).Kind() != reflect.Slice {
		return nil, errors.New("datastore: dest should be a slice")
	}
	c := &multiConverter{
		dest:     dest,
		size:     size,
		sliceTyp: reflect.TypeOf(dest).Elem(),
		sliceVal: reflect.ValueOf(dest),
	}
	return c, nil
}

func (c *multiConverter) set(i int, proto *pb.EntityResult) {
	if i < 0 || i >= c.size {
		return
	}
	obj := protoToEntity(c.elemTypeOf(i), proto.Entity)
	c.sliceVal.Index(i).Set(reflect.ValueOf(obj))
}

func (c *multiConverter) elemTypeOf(i int) reflect.Type {
	if c.sliceTyp.Kind() == reflect.Interface {
		return c.sliceVal.Index(i).Elem().Type().Elem()
	}
	return c.sliceVal.Index(i).Type().Elem()
}
