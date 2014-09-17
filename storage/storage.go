// Package storage is a Google Cloud Storage client.
package storage

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	raw "code.google.com/p/google-api-go-client/storage/v1beta2"
)

const (
	// ScopeFullControl grants permissions to manage your
	// data and permissions in Google Cloud Storage.
	ScopeFullControl = raw.DevstorageFull_controlScope

	// ScopeReadOnly grants permissions to
	// view your data in Google Cloud Storage.
	ScopeReadOnly = raw.DevstorageRead_onlyScope

	// ScopeReadWrite grants permissions to manage your
	// data in Google Cloud Storage.
	ScopeReadWrite = raw.DevstorageRead_writeScope
)

const (
	templUrlMedia = "https://storage.googleapis.com/%s/%s"
)

type BucketInfo struct {
	// Name is the name of the bucket.
	Name string `json:"name,omitempty"`
}

type conn struct {
	c *http.Client
	s *raw.Service
}

type Bucket struct {
	name string
	conn *conn
}

func (b *Bucket) String() string {
	return fmt.Sprintf("<bucket: %v>", b.name)
}

// Client represents a Google Cloud Storage client.
type Client struct {
	conn *conn
}

// New returns a new Google Cloud Storage client. The provided
// RoundTripper should be authorized and authenticated to make
// calls to Google Cloud Storage API.
func New(tr http.RoundTripper) *Client {
	return NewWithClient(&http.Client{Transport: tr})
}

// NewWithClient returns a new Google Cloud Storage client that
// uses the provided http.Client. Provided http.Client is responsible
// to authorize and authenticate the requests made to the
// Google Cloud Storage API.
func NewWithClient(c *http.Client) *Client {
	s, _ := raw.New(c)
	return &Client{conn: &conn{s: s, c: c}}
}

// TODO(jbd): Add storage.buckets.list.
// TODO(jbd): Add storage.buckets.insert.
// TODO(jbd): Add storage.buckets.update.
// TODO(jbd): Add storage.buckets.delete.

// TODO(jbd): Add storage.objects.list.
// TODO(jbd): Add storage.objects.watch.

// BucketInfo returns the metadata for the specified bucket.
func (c *Client) BucketInfo(name string) (*BucketInfo, error) {
	panic("not yet implemented")
}

// Bucket returns a named bucket to perform object operations on.
func (c *Client) Bucket(name string) *Bucket {
	return &Bucket{name: name, conn: c.conn}
}

// DefaultBucket returns the default bucket assigned to your
// project if you are running on Google Compute Engine or
// Google App Engine Managed VMs. It will return nil if your
// code is not running on Google Compute Engine or App Engine.
func (c *Client) DefaultBucket() *Bucket {
	panic("not yet implemented")
}

// Stat returns meta information about the specified object.
func (b *Bucket) Stat(name string) (*ObjectInfo, error) {
	o, err := b.conn.s.Objects.Get(b.name, name).Do()
	if err != nil {
		return nil, err
	}
	return newObjectInfo(o), nil
}

// Put inserts/updates an object with the provided meta information.
func (b *Bucket) Put(name string, info *ObjectInfo) (*ObjectInfo, error) {
	o, err := b.conn.s.Objects.Insert(b.name, info.toRawObject()).Do()
	if err != nil {
		return nil, err
	}
	return newObjectInfo(o), nil
}

// Delete deletes the specified object.
func (b *Bucket) Delete(name string) error {
	return b.conn.s.Objects.Delete(b.name, name).Do()
}

// Copy copies the source object to the destination with the new
// meta information provided.
// The destination object is inserted into the source bucket
// if the destination object doesn't specify another bucket name.
func (b *Bucket) Copy(name string, dest *ObjectInfo) (*ObjectInfo, error) {
	if dest.Name == "" {
		return nil, errors.New("storage: missing dest name")
	}
	destBucket := dest.Bucket
	if destBucket == "" {
		destBucket = b.name
	}
	o, err := b.conn.s.Objects.Copy(
		b.name, name, destBucket, dest.Name, dest.toRawObject()).Do()
	if err != nil {
		return nil, err
	}
	return newObjectInfo(o), nil
}

// NewReader creates a new io.ReadCloser to read the contents
// of the object.
func (b *Bucket) NewReader(name string) (io.ReadCloser, error) {
	resp, err := b.conn.c.Get(fmt.Sprintf(templUrlMedia, b.name, name))
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// NewWriter creates a new io.WriteCloser to write to the GCS object
// identified by the specified object name.
// If such object doesn't exist, it creates one. If info is not nil,
// write operation also modifies the meta information of the object.
func (b *Bucket) NewWriter(name string, info *ObjectInfo) io.WriteCloser {
	i := ObjectInfo{}
	if info != nil {
		i = *info
	}
	i.Bucket = b.name
	i.Name = name
	return newObjectWriter(b.conn, &i)
}
