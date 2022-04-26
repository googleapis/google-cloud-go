package firestore

import (
	"fmt"

	pb "google.golang.org/genproto/googleapis/firestore/v1"
)

type BulkWriter struct {
	c    *Client
	err  error
	refs []*DocumentRef
}

const (
	MAX_BATCH_SIZE       int = 20 // Maximum number of writes in a single batch
	RETRY_MAX_BATCH_SIZE int = 10 // Maximum number of writes in a batch containing retries
	MAX_RETRY_ATTEMPTS   int = 10 // Maximum number of retries to attempt, with backoff, before stopping
)

// TODO
func (b *BulkWriter) Close() error {
	fmt.Println("Not implemented")
	return nil
}

// TODO
func (b *BulkWriter) Create(ref *DocumentRef, data map[string]interface{}) error {
	fmt.Println("Not implemented")
	return nil
}

// TODO
func (b *BulkWriter) Delete(ref *DocumentRef, pre ...pb.Precondition) error {
	fmt.Println("Not implemented")
	return nil
}

// TODO
func (b *BulkWriter) Flush() error {
	fmt.Println("Not implemented")
	return nil
}

// TODO
func (b *BulkWriter) Set(ref *DocumentRef, data map[string]interface{}, opts ...SetOption) error {
	fmt.Println("Not implemented")
	return nil
}

// TODO
func (b *BulkWriter) Update(ref *DocumentRef, data []Update, opts ...pb.Precondition) error {
	fmt.Println("Not implemented")
	return nil
}
