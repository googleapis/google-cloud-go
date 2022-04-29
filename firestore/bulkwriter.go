package firestore

import (
	"context"
	"fmt"

	"cloud.google.com/go/internal/trace"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
)

type BulkWriter struct {
	c      *Client
	err    error
	writes []*pb.Write
	status BulkWriterStatus
}

const (
	MAX_BATCH_SIZE       int = 20 // Maximum number of writes in a single batch
	RETRY_MAX_BATCH_SIZE int = 10 // Maximum number of writes in a batch containing retries
	MAX_RETRY_ATTEMPTS   int = 10 // Maximum number of retries to attempt, with backoff, before stopping
)

type BulkWriterStatus int

const (
	SUCCESS  BulkWriterStatus = iota // All writes to the database were successful.
	OPEN                             // Writes have not yet been sent to the database.
	SENDING                          // Writes are being sent to the database.
	RETRYING                         // Some writes to the database failed; some failures are being retried.
	FAILED                           // Some writes to the database weren't sent to the database.
)

// Close sends all enqueued writes in parallel.
// After calling Close(), calling any additional method automatically returns
// with a nil error. This method completes when there are no more pending writes
// in the queue.
func (b *BulkWriter) Close() (err error) {
	ctx := trace.StartSpan(context.Background(), "cloud.google.com/go/firestore.BulkWriter.Close")
	defer func() { trace.EndSpan(ctx, err) }()
	fmt.Println("Not implemented")
	return nil
}

// Create creates a new document at the specified `DocumentReference` path with
// the provided data.
func (b *BulkWriter) Create(ref *DocumentRef, data map[string]interface{}) (wr *pb.WriteResult, err error) {
	fmt.Println("Not implemented")
	return nil, nil
}

// Delete removes the specified document from the database. You can provide
// `Precondition` objects to enforce for this deletion.
func (b *BulkWriter) Delete(ref *DocumentRef, preconds
	 ...pb.Precondition) (wr *pb.WriteResult, err error) {
	fmt.Println("Not implemented")
	return nil, nil
}

// Flush commits all writes that have been enqueued up to this point in parallel.
func (b *BulkWriter) Flush() (err error) {
	ctx := trace.StartSpan(context.Background(), "cloud.google.com/go/firestore.BulkWriter.Flush")
	defer func() { trace.EndSpan(ctx, err) }()
	fmt.Println("Not implemented")
	return nil
}

// TODO
func (b *BulkWriter) Set(ref *DocumentRef, data map[string]interface{}, opts ...SetOption) (wr *pb.WriteResult, err error) {
	fmt.Println("Not implemented")
	return nil, nil
}

// Status gets the status of the BulkWriter
func (b *BulkWriter) Status() BulkWriterStatus {
	return b.status
}

// TODO
func (b *BulkWriter) Update(ref *DocumentRef, data []Update, preconds ...pb.Precondition) (wr *pb.WriteResult, err error) {
	fmt.Println("Not implemented")
	return nil, nil
}
