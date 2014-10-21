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

// Package storage is a Google Cloud Storage client.
package storage

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"code.google.com/p/google-api-go-client/googleapi"
	raw "code.google.com/p/google-api-go-client/storage/v1"
	"google.golang.org/cloud/internal"
)

var (
	ErrBucketNotExists = errors.New("storage: bucket doesn't exist")
	ErrObjectNotExists = errors.New("storage: object doesn't exist")
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
	templURLMedia = "https://storage.googleapis.com/%s/%s"
)

type conn struct {
	c *http.Client
	s *raw.Service
}

// BucketClient is a client to perform object operations on.
type BucketClient struct {
	name string
	conn *conn
}

// String returns a string representation of the bucket client.
// E.g. <bucket: my-project-bucket>
func (b *BucketClient) String() string {
	return fmt.Sprintf("<bucket: %v>", b.name)
}

// Client represents a Google Cloud Storage client.
type Client struct {
	projID string
	conn   *conn
}

// New returns a new Google Cloud Storage client. The provided
// RoundTripper should be authorized and authenticated to make
// calls to Google Cloud Storage API.
// You can obtain the project ID from the Google Developers Console,
// https://console.developers.google.com.
func New(projID string, tr http.RoundTripper) *Client {
	return NewWithClient(projID, &http.Client{Transport: tr})
}

// NewWithClient returns a new Google Cloud Storage client that
// uses the provided http.Client. Provided http.Client is responsible
// to authorize and authenticate the requests made to the
// Google Cloud Storage API.
// It mutates the client's original Transport to append the cloud
// package's user-agent to the outgoing requests.
// You can obtain the project ID from the Google Developers Console,
// https://console.developers.google.com.
func NewWithClient(projID string, c *http.Client) *Client {
	c.Transport = &internal.UATransport{Base: c.Transport}
	s, _ := raw.New(c)
	return &Client{projID: projID, conn: &conn{s: s, c: c}}
}

// TODO(jbd): Add storage.buckets.list.
// TODO(jbd): Add storage.buckets.insert.
// TODO(jbd): Add storage.buckets.update.
// TODO(jbd): Add storage.buckets.delete.

// TODO(jbd): Add storage.objects.watch.

// Bucket returns the metadata for the specified bucket.
func (c *Client) Bucket(name string) (*Bucket, error) {
	resp, err := c.conn.s.Buckets.Get(name).Do()
	if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
		return nil, ErrBucketNotExists
	}
	if err != nil {
		return nil, err
	}
	return newBucket(resp), nil
}

// BucketClient returns a bucket client to perform object operations on.
func (c *Client) BucketClient(bucketname string) *BucketClient {
	return &BucketClient{name: bucketname, conn: c.conn}
}

// List lists objects from the bucket. You can specify a query
// to filter the results. If q is nil, no filtering is applied.
func (b *BucketClient) List(q *Query) (*Objects, error) {
	c := b.conn.s.Objects.List(b.name)
	if q != nil {
		c.Delimiter(q.Delimiter)
		c.Prefix(q.Prefix)
		c.Versions(q.Versions)
		c.PageToken(q.Cursor)
		if q.MaxResults > 0 {
			c.MaxResults(int64(q.MaxResults))
		}
	}
	resp, err := c.Do()
	if err != nil {
		return nil, err
	}
	objects := &Objects{
		Results:  make([]*Object, len(resp.Items)),
		Prefixes: make([]string, len(resp.Prefixes)),
	}
	for i, item := range resp.Items {
		objects.Results[i] = newObject(item)
	}
	for i, prefix := range resp.Prefixes {
		objects.Prefixes[i] = prefix
	}
	if resp.NextPageToken != "" {
		next := Query{}
		if q != nil {
			// keep the other filtering
			// criteria if there is a query
			next = *q
		}
		next.Cursor = resp.NextPageToken
		objects.Next = &next
	}
	return objects, nil
}

// Stat returns meta information about the specified object.
func (b *BucketClient) Stat(name string) (*Object, error) {
	o, err := b.conn.s.Objects.Get(b.name, name).Do()
	if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
		return nil, ErrObjectNotExists
	}
	if err != nil {
		return nil, err
	}
	return newObject(o), nil
}

// Put inserts/updates an object with the provided meta information.
func (b *BucketClient) Put(name string, info *Object) (*Object, error) {
	o, err := b.conn.s.Objects.Insert(b.name, info.toRawObject()).Do()
	if err != nil {
		return nil, err
	}
	return newObject(o), nil
}

// Delete deletes the specified object.
func (b *BucketClient) Delete(name string) error {
	return b.conn.s.Objects.Delete(b.name, name).Do()
}

// Copy copies the source object to the destination with the new
// meta information provided.
// The destination object is inserted into the source bucket
// if the destination object doesn't specify another bucket name.
func (b *BucketClient) Copy(name string, dest *Object) (*Object, error) {
	if dest.Name == "" {
		return nil, errors.New("storage: missing dest name")
	}
	if dest.Bucket == "" {
		// Make a copy of the dest object instead of mutating it.
		dest2 := *dest
		dest2.Bucket = b.name
		dest = &dest2
	}
	o, err := b.conn.s.Objects.Copy(
		b.name, name, dest.Bucket, dest.Name, dest.toRawObject()).Do()
	if err != nil {
		return nil, err
	}
	return newObject(o), nil
}

// NewReader creates a new io.ReadCloser to read the contents
// of the object.
func (b *BucketClient) NewReader(name string) (io.ReadCloser, error) {
	resp, err := b.conn.c.Get(fmt.Sprintf(templUrlMedia, b.name, name))
	if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
		return nil, ErrObjectNotExists
	}
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// NewWriter returns a new ObjectWriter to write to the GCS object
// identified by the specified object name.
// If such object doesn't exist, it creates one. If info is not nil,
// write operation also modifies the meta information of the object.
// All read-only fields are ignored during metadata updates.
func (b *BucketClient) NewWriter(name string, info *Object) *ObjectWriter {
	i := Object{}
	if info != nil {
		i = *info
	}
	i.Bucket = b.name
	i.Name = name
	return newObjectWriter(b.conn, &i)
}
