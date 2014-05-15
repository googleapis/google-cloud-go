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
	ErrNotFound = errors.New("datastore: no entity with the provided key has been found")
)

type Dataset struct {
	ID string // project ID, value could be obtained from the Developer Console.

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
	return &Dataset{ID: projectID, transport: conf.NewTransport()}, nil
}

func (d *Dataset) NewIncompleteKey(kind string) *Key {
	return newIncompleteKey(kind, d.ID, "default")
}

func (d *Dataset) NewIncompleteKeyWithNs(namespace, kind string) *Key {
	return newIncompleteKey(kind, d.ID, namespace)
}

func (d *Dataset) NewKey(kind string, ID int64) *Key {
	return d.NewKeyWithNs("default", kind, ID)
}

func (d *Dataset) NewKeyWithNs(namespace, kind string, ID int64) *Key {
	return newKey(kind, strconv.FormatInt(ID, 10), ID, d.ID, namespace)
}

func (d *Dataset) Get(key *Key, dest interface{}) (err error) {
	req := &pb.LookupRequest{
		Key: []*pb.Key{keyToPbKey(key)},
	}
	resp := &pb.LookupResponse{}
	client := gcloud.Client{Transport: d.transport}
	if err = client.Call(d.newUrl("lookup"), req, resp); err != nil {
		return
	}
	if len(resp.Found) == 0 {
		return ErrNotFound
	}
	entityFromPbEntity(resp.Found[0].Entity, dest)
	return
}

func (d *Dataset) Put(key *Key, src interface{}) (*Key, error) {
	panic("not yet implemented")
}

func (d *Dataset) Delete(key *Key) (err error) {
	mode := pb.CommitRequest_NON_TRANSACTIONAL
	req := &pb.CommitRequest{
		Mutation: &pb.Mutation{Delete: []*pb.Key{keyToPbKey(key)}},
		Mode:     &mode,
	}
	resp := &pb.CommitResponse{}
	client := gcloud.Client{Transport: d.transport}
	return client.Call(d.newUrl("commit"), req, resp)
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
	client := gcloud.Client{Transport: d.transport}
	if err = client.Call(d.newUrl("allocateIds"), req, resp); err != nil {
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
			d.ID,
			created.GetPartitionId().GetNamespace())
	}
	return
}

func (d *Dataset) RunInTransaction(fn func() error) error {
	panic("not yet implemented")
}

func (d *Dataset) newUrl(method string) string {
	return "https://www.googleapis.com/datastore/v1beta2/datasets/" + d.ID + "/" + method
}
