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

	"google.golang.org/cloud/internal"

	"code.google.com/p/go.net/context"
	"code.google.com/p/google-api-go-client/googleapi"
	raw "code.google.com/p/google-api-go-client/storage/v1"
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

// TODO(jbd): Add storage.buckets.list.
// TODO(jbd): Add storage.buckets.insert.
// TODO(jbd): Add storage.buckets.update.
// TODO(jbd): Add storage.buckets.delete.

// TODO(jbd): Add storage.objects.watch.

// BucketInfo returns the metadata for the specified bucket.
func BucketInfo(ctx context.Context, name string) (*Bucket, error) {
	resp, err := rawService(ctx).Buckets.Get(name).Do()
	if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
		return nil, ErrBucketNotExists
	}
	if err != nil {
		return nil, err
	}
	return newBucket(resp), nil
}

// List lists objects from the bucket. You can specify a query
// to filter the results. If q is nil, no filtering is applied.
func List(ctx context.Context, bucket string, q *Query) (*Objects, error) {
	c := rawService(ctx).Objects.List(bucket)
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
func Stat(ctx context.Context, bucket, name string) (*Object, error) {
	o, err := rawService(ctx).Objects.Get(bucket, name).Do()
	if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
		return nil, ErrObjectNotExists
	}
	if err != nil {
		return nil, err
	}
	return newObject(o), nil
}

// Put inserts/updates an object with the provided meta information.
func Put(ctx context.Context, bucket, name string, info *Object) (*Object, error) {
	o, err := rawService(ctx).Objects.Insert(bucket, info.toRawObject()).Do()
	if err != nil {
		return nil, err
	}
	return newObject(o), nil
}

// Delete deletes the specified object.
func Delete(ctx context.Context, bucket, name string) error {
	return rawService(ctx).Objects.Delete(bucket, name).Do()
}

// Copy copies the source object to the destination with the new
// meta information provided.
// The destination object is inserted into the source bucket
// if the destination object doesn't specify another bucket name.
func Copy(ctx context.Context, bucket, name string, dest *Object) (*Object, error) {
	if dest.Name == "" {
		return nil, errors.New("storage: missing dest name")
	}
	if dest.Bucket == "" {
		// Make a copy of the dest object instead of mutating it.
		dest2 := *dest
		dest2.Bucket = bucket
		dest = &dest2
	}
	o, err := rawService(ctx).Objects.Copy(
		bucket, name, dest.Bucket, dest.Name, dest.toRawObject()).Do()
	if err != nil {
		return nil, err
	}
	return newObject(o), nil
}

// NewReader creates a new io.ReadCloser to read the contents
// of the object.
func NewReader(ctx context.Context, bucket, name string) (io.ReadCloser, error) {
	c := ctx.Value(internal.Key(0)).(map[string]interface{})["http_client"].(*http.Client)
	resp, err := c.Get(fmt.Sprintf(templURLMedia, bucket, name))
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
func NewWriter(ctx context.Context, bucket, name string, info *Object) *ObjectWriter {
	i := Object{}
	if info != nil {
		i = *info
	}
	i.Bucket = bucket
	i.Name = name
	return newObjectWriter(ctx, &i)
}

func projID(ctx context.Context) string {
	return ctx.Value(internal.Key(0)).(map[string]interface{})["project_id"].(string)
}

func rawService(ctx context.Context) *raw.Service {
	return ctx.Value(internal.Key(0)).(map[string]interface{})["storage_service"].(*raw.Service)
}
