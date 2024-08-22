package dataflux

import (
	"context"

	"cloud.google.com/go/storage"
)

// Listing performs a sequential listing on the given bucket.
// It returns a list of objects and the next token to use to continue listing.
// If the next token is empty, then listing is complete.
func Listing(ctx context.Context, opts Lister) ([]*storage.ObjectAttrs, string, error) {
	return nil, "", nil
}
