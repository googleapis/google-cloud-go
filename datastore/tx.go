package datastore

import (
	"errors"
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

	// TODO(jbd): Export client and provide a way to provide custom clients.
	client *client
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
	req := &pb.RunQueryRequest{
		ReadOptions: &pb.ReadOptions{
			Transaction: t.id,
		},
		Query: queryToProto(q),
	}
	if q.namespace != "" {
		req.PartitionId = &pb.PartitionId{
			Namespace: proto.String(q.namespace),
		}
	}
	resp := &pb.RunQueryResponse{}
	if err = t.client.call(t.newUrl("runQuery"), req, resp); err != nil {
		return
	}
	results := resp.GetBatch().GetEntityResult()
	keys = make([]*Key, len(results))

	var conv *multiConverter
	if dest != nil {
		if conv, err = newMultiConverter(len(keys), dest); err != nil {
			return
		}
	}

	for i, r := range results {
		keys[i] = protoToKey(r.Entity.Key)
		if conv != nil {
			conv.set(i, r.Entity)
		}
	}
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
	if err := t.client.call(t.newUrl("commit"), req, resp); err != nil {
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
	if err := t.client.call(t.newUrl("rollback"), req, resp); err != nil {
		return err
	}
	return nil
}

// Get gets multiple entities by key. Destination argument only
// allows a slice of pointers or an interface{} slice with pointers.
// Examples:
// 		ptr1 := &T{} //...
// 		items := []interface{}{ptr1, ptr1}
// 		ds.Get([]*datastore.Key{key1, key2}, items)
// 		fmt.Println(ptr1, ptr2)
//
//		// or alternatively
//		items = make([]*T, 2)
// 		ds.Get([]*datastore.Key{key1, key2}, items)
// 		fmt.Println(items[0], items[1])
//
// 		 // or alternatively
// 		items = []*T{ptr1, ptr2}
// 		ds.Get([]*datastore.Key{key1, key2}, items)
// 		fmt.Println(ptr1, ptr2)
//
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
	if err := t.client.call(t.newUrl("lookup"), req, resp); err != nil {
		return err
	}
	for i, result := range resp.Found {
		converter.set(i, result.Entity)
	}
	return nil
}

// Put upserts the objects identified with provided keys. If one or
// more keys are incomplete, backend generates unique numeric identifiers.
func (t *Tx) Put(keys []*Key, src interface{}) ([]*Key, error) {
	// TODO(jbd): Validate src type.
	// Determine mod depending on if this is the default
	// transaction or not.
	mode := pb.CommitRequest_NON_TRANSACTIONAL.Enum()
	if t.IsTransactional() {
		mode = pb.CommitRequest_TRANSACTIONAL.Enum()
	}

	req := &pb.CommitRequest{
		Transaction: t.id,
		Mode:        mode,
		Mutation:    &pb.Mutation{},
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
		if !k.IsComplete() {
			autoIdIndex = append(autoIdIndex, i)
			autoId = append(autoId, entityToProto(k, val))
		} else {
			upsert = append(upsert, entityToProto(k, val))
		}
	}
	req.Mutation.InsertAutoId = autoId
	req.Mutation.Upsert = upsert

	resp := &pb.CommitResponse{}
	if err := t.client.call(t.newUrl("commit"), req, resp); err != nil {
		return nil, err
	}

	// modify keys list with the newly created keys.
	createdIDs := resp.GetMutationResult().GetInsertAutoIdKey()
	for i, index := range autoIdIndex {
		keys[index] = protoToKey(createdIDs[i])
	}
	return keys, nil
}

// Delete deletes the object identified with the specified key in
// the transaction.
func (t *Tx) Delete(keys []*Key) (err error) {
	protoKeys := make([]*pb.Key, len(keys))
	for i, k := range keys {
		protoKeys[i] = keyToProto(k)
	}
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
	return t.client.call(t.newUrl("commit"), req, resp)
}

func (t *Tx) newUrl(method string) string {
	// TODO(jbd): Provide support for non-prod instances.
	return "https://www.googleapis.com/datastore/v1beta2/datasets/" + t.datasetID + "/" + method
}
