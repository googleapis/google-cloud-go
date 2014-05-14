package datastore

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	gcloud "github.com/googlecloudplatform/gcloud-golang"
	"github.com/googlecloudplatform/gcloud-golang/datastore/pb"
	"google.golang.org/oauth2"
	"google.golang.org/oauth2/google"
)

const (
	endpointLookup = "https://www.googleapis.com/datastore/v1beta2/datasets/{datasetId}/lookup"
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
	tr, err := conf.NewTransport()
	if err != nil {
		return
	}
	return &Dataset{ID: projectID, transport: tr}, nil
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
	if err = client.Call(d.newUrl(endpointLookup), req, resp); err != nil {
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

func (d *Dataset) Delete(key *Key) error {
	panic("not yet implemented")
}

func (d *Dataset) AllocateIDs(kind string, n int) ([]*Key, error) {
	panic("not yet implemented")
}

func (d *Dataset) RunInTransaction(fn func() error) error {
	panic("not yet implemented")
}

func (d *Dataset) newUrl(template string) string {
	return strings.Replace(template, "{datasetId}", d.ID, 1)
}
