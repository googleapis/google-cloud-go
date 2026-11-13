package udsthin

import (
	"context"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	pb "poc/gothin/udsthin/gcscorepb"
)

// ObjectIterator mirrors storage.ObjectIterator.
type ObjectIterator struct {
	b        *BucketHandle
	ctx      context.Context
	q        storage.Query
	pageInfo *iterator.PageInfo
	nextFunc func() error
	items    []*storage.ObjectAttrs
}

// Objects returns an iterator over the objects in the bucket matching q.
func (b *BucketHandle) Objects(ctx context.Context, q *storage.Query) *ObjectIterator {
	it := &ObjectIterator{b: b, ctx: ctx}
	if q != nil {
		it.q = *q
	}
	it.pageInfo, it.nextFunc = iterator.NewPageInfo(it.fetch,
		func() int { return len(it.items) },
		func() interface{} { b := it.items; it.items = nil; return b })
	return it
}

// PageInfo returns pagination state for the iterator.
func (it *ObjectIterator) PageInfo() *iterator.PageInfo { return it.pageInfo }

// Next returns the next object or prefix, or iterator.Done.
func (it *ObjectIterator) Next() (*storage.ObjectAttrs, error) {
	if err := it.nextFunc(); err != nil {
		return nil, err
	}
	item := it.items[0]
	it.items = it.items[1:]
	return item, nil
}

func (it *ObjectIterator) fetch(pageSize int, pageToken string) (string, error) {
	page, err := it.b.c.stub().ListObjects(it.ctx, &pb.ListObjectsRequest{
		Bucket:      it.b.name,
		Prefix:      it.q.Prefix,
		Delimiter:   it.q.Delimiter,
		StartOffset: it.q.StartOffset,
		EndOffset:   it.q.EndOffset,
		PageToken:   pageToken,
		PageSize:    int32(pageSize),
		Versions:    it.q.Versions,
	})
	if err != nil {
		return "", mapErr(err)
	}
	for _, p := range page.Prefixes {
		it.items = append(it.items, &storage.ObjectAttrs{Prefix: p})
	}
	for _, o := range page.Items {
		it.items = append(it.items, &storage.ObjectAttrs{
			Bucket:         it.b.name,
			Name:           o.Name,
			Size:           o.Size,
			Generation:     o.Generation,
			Metageneration: o.Metageneration,
			ContentType:    o.ContentType,
			StorageClass:   o.StorageClass,
			CRC32C:         o.Crc32C,
			Updated:        time.Unix(o.UpdatedUnixSecs, 0),
		})
	}
	return page.NextPageToken, nil
}

// BucketIterator mirrors storage.BucketIterator.
type BucketIterator struct {
	Prefix   string
	c        *Client
	ctx      context.Context
	project  string
	pageInfo *iterator.PageInfo
	nextFunc func() error
	items    []*storage.BucketAttrs
}

// Buckets returns an iterator over the buckets in projectID.
func (c *Client) Buckets(ctx context.Context, projectID string) *BucketIterator {
	it := &BucketIterator{c: c, ctx: ctx, project: projectID}
	it.pageInfo, it.nextFunc = iterator.NewPageInfo(it.fetch,
		func() int { return len(it.items) },
		func() interface{} { b := it.items; it.items = nil; return b })
	return it
}

// PageInfo returns pagination state for the bucket iterator.
func (it *BucketIterator) PageInfo() *iterator.PageInfo { return it.pageInfo }

// Next returns the next BucketAttrs or iterator.Done.
func (it *BucketIterator) Next() (*storage.BucketAttrs, error) {
	if err := it.nextFunc(); err != nil {
		return nil, err
	}
	item := it.items[0]
	it.items = it.items[1:]
	return item, nil
}

func (it *BucketIterator) fetch(pageSize int, pageToken string) (string, error) {
	page, err := it.c.stub().ListBuckets(it.ctx, &pb.ListBucketsRequest{
		Project:   it.project,
		Prefix:    it.Prefix,
		PageToken: pageToken,
		PageSize:  int32(pageSize),
	})
	if err != nil {
		return "", mapErr(err)
	}
	for _, b := range page.Items {
		it.items = append(it.items, &storage.BucketAttrs{
			Name:           b.Name,
			Location:       b.Location,
			StorageClass:   b.StorageClass,
			MetaGeneration: b.Metageneration,
			Created:        time.Unix(b.CreatedUnixSecs, 0),
		})
	}
	return page.NextPageToken, nil
}
