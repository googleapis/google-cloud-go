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

package storage

import (
	"errors"
	"net/http"

	"golang.org/x/net/context"
	"google.golang.org/api/googleapi"
	raw "google.golang.org/api/storage/v1"
)

// Create creates the Bucket in the project.
// If attrs is nil the API defaults will be used.
func (b *BucketHandle) Create(ctx context.Context, projectID string, attrs *BucketAttrs) error {
	var bkt *raw.Bucket
	if attrs != nil {
		bkt = attrs.toRawBucket()
	} else {
		bkt = &raw.Bucket{}
	}
	bkt.Name = b.name
	req := b.c.raw.Buckets.Insert(projectID, bkt)
	_, err := req.Context(ctx).Do()
	return err
}

// Delete deletes the Bucket.
func (b *BucketHandle) Delete(ctx context.Context) error {
	req := b.c.raw.Buckets.Delete(b.name)
	return req.Context(ctx).Do()
}

// ACL returns an ACLHandle, which provides access to the bucket's access control list.
// This controls who can list, create or overwrite the objects in a bucket.
// This call does not perform any network operations.
func (c *BucketHandle) ACL() *ACLHandle {
	return c.acl
}

// DefaultObjectACL returns an ACLHandle, which provides access to the bucket's default object ACLs.
// These ACLs are applied to newly created objects in this bucket that do not have a defined ACL.
// This call does not perform any network operations.
func (c *BucketHandle) DefaultObjectACL() *ACLHandle {
	return c.defaultObjectACL
}

// Object returns an ObjectHandle, which provides operations on the named object.
// This call does not perform any network operations.
//
// name must consist entirely of valid UTF-8-encoded runes. The full specification
// for valid object names can be found at:
//   https://cloud.google.com/storage/docs/bucket-naming
func (b *BucketHandle) Object(name string) *ObjectHandle {
	return &ObjectHandle{
		c:      b.c,
		bucket: b.name,
		object: name,
		acl: &ACLHandle{
			c:      b.c,
			bucket: b.name,
			object: name,
		},
	}
}

// TODO(jba): Add storage.buckets.list.
// TODO(jbd): Add storage.buckets.update.

// TODO(jbd): Add storage.objects.watch.

// Attrs returns the metadata for the bucket.
func (b *BucketHandle) Attrs(ctx context.Context) (*BucketAttrs, error) {
	resp, err := b.c.raw.Buckets.Get(b.name).Projection("full").Context(ctx).Do()
	if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
		return nil, ErrBucketNotExist
	}
	if err != nil {
		return nil, err
	}
	return newBucket(resp), nil
}

// ObjectList represents a list of objects returned from a bucket List call.
type ObjectList struct {
	// Results represent a list of object results.
	Results []*ObjectAttrs

	// Next is the continuation query to retrieve more
	// results with the same filtering criteria. If there
	// are no more results to retrieve, it is nil.
	Next *Query

	// Prefixes represents prefixes of objects
	// matching-but-not-listed up to and including
	// the requested delimiter.
	Prefixes []string
}

// List lists objects from the bucket. You can specify a query
// to filter the results. If q is nil, no filtering is applied.
//
// Deprecated. Use BucketHandle.Objects instead.
func (b *BucketHandle) List(ctx context.Context, q *Query) (*ObjectList, error) {
	it := b.Objects(ctx, q)
	attrs, pres, err := it.NextPage()
	if err != nil && err != Done {
		return nil, err
	}
	objects := &ObjectList{
		Results:  attrs,
		Prefixes: pres,
	}
	if it.NextPageToken() != "" {
		objects.Next = &it.query
	}
	return objects, nil
}

func (b *BucketHandle) Objects(ctx context.Context, q *Query) *ObjectIterator {
	it := &ObjectIterator{
		ctx:    ctx,
		bucket: b,
	}
	if q != nil {
		it.query = *q
	}
	return it
}

type ObjectIterator struct {
	ctx      context.Context
	bucket   *BucketHandle
	query    Query
	pageSize int32
	objs     []*ObjectAttrs
	prefixes []string
	err      error
}

// Next returns the next result. Its second return value is Done if there are
// no more results. Once Next returns Done, all subsequent calls will return
// Done.
//
// Internally, Next retrieves results in bulk. You can call SetPageSize as a
// performance hint to affect how many results are retrieved in a single RPC.
//
// SetPageToken should not be called when using Next.
//
// Next and NextPage should not be used with the same iterator.
//
// If Query.Delimiter is non-empty, Next returns an error. Use NextPage when using delimiters.
func (it *ObjectIterator) Next() (*ObjectAttrs, error) {
	if it.query.Delimiter != "" {
		return nil, errors.New("cannot use ObjectIterator.Next with a delimiter")
	}
	for len(it.objs) == 0 { // "for", not "if", to handle empty pages
		if it.err != nil {
			return nil, it.err
		}
		it.nextPage()
		if it.err != nil {
			it.objs = nil
			return nil, it.err
		}
		if it.query.Cursor == "" {
			it.err = Done
		}
	}
	o := it.objs[0]
	it.objs = it.objs[1:]
	return o, nil
}

const DefaultPageSize = 1000

// NextPage returns the next page of results, both objects (as *ObjectAttrs)
// and prefixes. Prefixes will be nil if query.Delimiter is empty.
//
// NextPage will return exactly the number of results (the total of objects and
// prefixes) specified by the last call to SetPageSize, unless there are not
// enough results available. If no page size was specified, it uses
// DefaultPageSize.
//
// NextPage may return a second return value of Done along with the last page
// of results.
//
// After NextPage returns Done, all subsequent calls to NextPage will return
// (nil, Done).
//
// Next and NextPage should not be used with the same iterator.
func (it *ObjectIterator) NextPage() (objs []*ObjectAttrs, prefixes []string, err error) {
	defer it.SetPageSize(it.pageSize) // restore value at entry
	if it.pageSize <= 0 {
		it.pageSize = DefaultPageSize
	}
	for len(objs)+len(prefixes) < int(it.pageSize) {
		it.pageSize -= int32(len(objs) + len(prefixes))
		it.nextPage()
		if it.err != nil {
			return nil, nil, it.err
		}
		objs = append(objs, it.objs...)
		prefixes = append(prefixes, it.prefixes...)
		if it.query.Cursor == "" {
			it.err = Done
			return objs, prefixes, it.err
		}
	}
	return objs, prefixes, it.err
}

// nextPage gets the next page of results by making a single call to the underlying method.
// It sets it.objs, it.prefixes, it.query.Cursor, and it.err. It never sets it.err to Done.
func (it *ObjectIterator) nextPage() {
	if it.err != nil {
		return
	}
	req := it.bucket.c.raw.Objects.List(it.bucket.name)
	req.Projection("full")
	req.Delimiter(it.query.Delimiter)
	req.Prefix(it.query.Prefix)
	req.Versions(it.query.Versions)
	req.PageToken(it.query.Cursor)
	if it.pageSize > 0 {
		req.MaxResults(int64(it.pageSize))
	}
	resp, err := req.Context(it.ctx).Do()
	if err != nil {
		it.err = err
		return
	}
	it.query.Cursor = resp.NextPageToken
	it.objs = nil
	for _, item := range resp.Items {
		it.objs = append(it.objs, newObject(item))
	}
	it.prefixes = resp.Prefixes
}

// SetPageSize sets the page size for all subsequent calls to NextPage.
// NextPage will return exactly this many items if they are present.
func (it *ObjectIterator) SetPageSize(pageSize int32) {
	it.pageSize = pageSize
}

// SetPageToken sets the page token for the next call to NextPage, to resume
// the iteration from a previous point.
func (it *ObjectIterator) SetPageToken(t string) {
	it.query.Cursor = t
}

// NextPageToken returns a page token that can be used with SetPageToken to
// resume iteration from the next page. It returns the empty string if there
// are no more pages. For an example, see SetPageToken.
func (it *ObjectIterator) NextPageToken() string {
	return it.query.Cursor
}
