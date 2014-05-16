package datastore

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/golang/oauth2"
	"github.com/golang/oauth2/google"
	gcloud "github.com/googlecloudplatform/gcloud-golang"
	"github.com/googlecloudplatform/gcloud-golang/datastore/pb"
)

var requiredScopes = []string{
	"https://www.googleapis.com/auth/datastore",
	"https://www.googleapis.com/auth/userinfo.email",
}

var (
	ErrRollbackNonTransactional = errors.New("datastore: cannot rollback non-transactional operation")
	ErrAlreadyRolledBack        = errors.New("datastore: transaction already rolled back")
	ErrNotFound                 = errors.New("datastore: no entity with the provided key has been found")
)

type Dataset struct {
	defaultTransaction *Transaction
}

type Transaction struct {
	id        []byte
	datasetID string
	transport http.RoundTripper
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
	return newIncompleteKey(kind, d.defaultTransaction.datasetID, "default")
}

func (d *Dataset) NewIncompleteKeyWithNs(namespace, kind string) *Key {
	return newIncompleteKey(kind, d.defaultTransaction.datasetID, namespace)
}

func (d *Dataset) NewKey(kind string, ID int64) *Key {
	return d.NewKeyWithNs("default", kind, ID)
}

func (d *Dataset) NewKeyWithNs(namespace, kind string, ID int64) *Key {
	return newKey(kind, strconv.FormatInt(ID, 10), ID, d.defaultTransaction.datasetID, namespace)
}

func (d *Dataset) Get(key *Key, dest interface{}) (err error) {
	return d.defaultTransaction.Get(key, dest)
}

func (d *Dataset) Put(key *Key, src interface{}) (k *Key, err error) {
	return d.defaultTransaction.Put(key, src)
}

func (d *Dataset) Delete(key *Key) (err error) {
	return d.defaultTransaction.Delete(key)
}

func (d *Dataset) AllocateIDs(namespace, kind string, n int) (keys []*Key, err error) {
	if namespace == "" {
		namespace = "default"
	}
	incompleteKeys := make([]*pb.Key, n)
	for i := 0; i < n; i++ {
		incompleteKeys[i] = keyToPbKey(d.NewIncompleteKeyWithNs(namespace, kind))
	}
	req := &pb.AllocateIdsRequest{Key: incompleteKeys}
	resp := &pb.AllocateIdsResponse{}

	url := d.defaultTransaction.newUrl("allocateIds")
	if err = d.defaultTransaction.newClient().Call(url, req, resp); err != nil {
		return
	}
	// TODO(jbd): Return error if response doesn't include enough keys.
	keys = make([]*Key, n)
	for i := 0; i < n; i++ {
		created := resp.GetKey()[i]
		keys[i] = newKey(
			created.GetPathElement()[0].GetKind(),
			strconv.FormatInt(created.GetPathElement()[0].GetId(), 10),
			created.GetPathElement()[0].GetId(),
			d.defaultTransaction.datasetID,
			created.GetPartitionId().GetNamespace())
	}
	return
}

func (d *Dataset) NewTransaction() (*Transaction, error) {
	transaction := &Transaction{
		transport: d.defaultTransaction.transport,
		datasetID: d.defaultTransaction.datasetID,
	}

	req := &pb.BeginTransactionRequest{}
	resp := &pb.BeginTransactionResponse{}
	url := d.defaultTransaction.newUrl("beginTransaction")
	if err := d.defaultTransaction.newClient().Call(url, req, resp); err != nil {
		return nil, err
	}
	transaction.id = resp.GetTransaction()
	return transaction, nil
}

func (t *Transaction) IsTransactional() bool {
	return len(t.id) > 0
}

func (t *Transaction) Commit() error {
	if t.IsTransactional() {
		return nil
	}
	req := &pb.CommitRequest{
		Transaction: t.id,
	}
	resp := &pb.CommitResponse{}
	if err := t.newClient().Call(t.newUrl("commit"), req, resp); err != nil {
		return err
	}
	return nil
}

func (t *Transaction) Rollback() error {
	if t.IsTransactional() {
		return ErrRollbackNonTransactional
	}
	req := &pb.RollbackRequest{
		Transaction: t.id,
	}
	resp := &pb.RollbackResponse{}
	if err := t.newClient().Call(t.newUrl("rollback"), req, resp); err != nil {
		return err
	}
	return nil
}

func (t *Transaction) Get(key *Key, dest interface{}) (err error) {
	// TODO: add transactional impl
	req := &pb.LookupRequest{
		Key: []*pb.Key{keyToPbKey(key)},
	}
	resp := &pb.LookupResponse{}
	if err = t.newClient().Call(t.newUrl("lookup"), req, resp); err != nil {
		return
	}
	if len(resp.Found) == 0 {
		return ErrNotFound
	}
	entityFromPbEntity(resp.Found[0].Entity, dest)
	return
}

func (t *Transaction) Put(key *Key, src interface{}) (k *Key, err error) {
	// TODO: add transactional impl
	mode := pb.CommitRequest_NON_TRANSACTIONAL
	req := &pb.CommitRequest{
		Mode: &mode,
		Mutation: &pb.Mutation{
			Upsert: []*pb.Entity{entityToPbEntity(key, src)},
		},
	}
	resp := &pb.CommitResponse{}
	if err = t.newClient().Call(t.newUrl("commit"), req, resp); err != nil {
		return
	}
	panic("not yet implemented")
}

func (t *Transaction) Delete(key *Key) (err error) {
	// TODO: add transactional impl
	mode := pb.CommitRequest_NON_TRANSACTIONAL
	req := &pb.CommitRequest{
		Mutation: &pb.Mutation{Delete: []*pb.Key{keyToPbKey(key)}},
		Mode:     &mode,
	}
	resp := &pb.CommitResponse{}
	return t.newClient().Call(t.newUrl("commit"), req, resp)
}

func (t *Transaction) newClient() *gcloud.Client {
	return &gcloud.Client{Transport: t.transport}
}

func (t *Transaction) newUrl(method string) string {
	return "https://www.googleapis.com/datastore/v1beta2/datasets/" + t.datasetID + "/" + method
}
