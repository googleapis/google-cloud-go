package dataflux

import (
	"context"

	"cloud.google.com/go/storage"
)

// newObjectListerOpts specifies options for instantiating the NewObjectLister.
type newObjectListerOpts struct {
	// startRange is the start offset of the objects to be listed.
	startRange string
	// endRange is the end offset of the objects to be listed.
	endRange string
	// bucketHandle is the bucket handle of the bucket to be listed.
	bucketHandle *storage.BucketHandle
	// query is the storage.Query to filter objects for listing.
	query storage.Query
	// skipDirectoryObjects is to indicate whether to list directory objects.
	skipDirectoryObjects bool
	// generation is the generation number of the last object in the page.
	generation int64
}

// nextPageResult holds the next page of object names and indicates whether the
// lister has completed listing (no more objects to retrieve).
type nextPageResult struct {
	// items is the list of objects listed.
	items []*storage.ObjectAttrs
	// doneListing indicates whether the lister has completed listing.
	doneListing bool
	// nextStartRange is the start offset of the next page of objects to be listed.
	nextStartRange string
	// generation is the generation number of the last object in the page.
	generation int64
}

// newObjectLister creates a new ObjectLister using the given lister options.
func newObjectLister(ctx context.Context, opts newObjectListerOpts) (*nextPageResult, error) {
	return &nextPageResult{}, nil
}

func addPrefix(name, prefix string) string {
	if name != "" {
		return prefix + name
	}
	return name
}
