package firestore

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	vkit "cloud.google.com/go/firestore/apiv1"
	"google.golang.org/api/support/bundler"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
)

const (
	// maxBatchSize is the max number of writes to send in a request
	maxBatchSize = 20
	// retryMaxBatchSize is the max number of writes to send in a retry request
	retryMaxBatchSize = 10
	// maxRetryAttempts is the max number of times to retry a write
	maxRetryAttempts = 10
	// defaultStartingMaximumOpsPerSecond is the starting max number of requests to the service per second
	defaultStartingMaximumOpsPerSecond = 500
)

// BulkWriterJob provides read-only access to the results of a BulkWriter write attempt.
type BulkWriterJob struct {
	err        chan error           // send error responses to this channel
	result     chan *pb.WriteResult // send the write results to this channel
	write      *pb.Write            // the writes to apply to the database
	attempts   int                  // number of times this write has been attempted
	resultLock sync.Mutex           // guards writes to wr and e fields
	wr         *WriteResult         // (cached) result from the operation
	e          error                // (cached) any errors that occurred
	ctx        context.Context      // context for canceling/timing out results
}

// Results gets the results of the BulkWriter write attempt.
// This method blocks if the results for this BulkWriterJob haven't been
// received.
func (j *BulkWriterJob) Results() (*WriteResult, error) {
	j.resultLock.Lock()
	defer j.resultLock.Unlock()
	if j.wr == nil && j.e == nil {
		j.wr, j.e = j.processResults() // cache the results for additional calls
	}
	return j.wr, j.e
}

// processResults checks for errors returned from send() and packages up the
// results as WriteResult objects
func (j *BulkWriterJob) processResults() (*WriteResult, error) {
	select {
	case <-j.ctx.Done():
		return nil, fmt.Errorf("bulkwriter: early write cancellation")
	case wpb := <-j.result:
		return writeResultFromProto(wpb)
	case err := <-j.err:
		return nil, err
	}
}

// setError ensures that an error is returned on the error channel of BulkWriterJob.
func (j *BulkWriterJob) setError(e error) {
	j.err <- e
	close(j.result)
}

// setSuccess ensures that a WriteResult is returned to the result channel of BulkWriterJob.
func (j *BulkWriterJob) setResult(r *pb.WriteResult) {
	j.result <- r
	close(j.err)
}

// A BulkWriter supports concurrent writes to multiple documents. The BulkWriter
// submits document writes in maximum batches of 20 writes per request. Each
// request can contain many different document writes: create, delete, update,
// and set are all supported.
//
// Only one operation (create, set, update, delete) per document is allowed.
// BulkWriter cannot promise atomicity: individual writes can fail or succeed
// independent of each other. Bulkwriter does not apply writes in any set order;
// thus a document can't have set on it immediately after creation.
type BulkWriter struct {
	database        string             // the database as resource name: projects/[PROJECT]/databases/[DATABASE]
	start           time.Time          // when this BulkWriter was started; used to calculate qps and rate increases
	vc              *vkit.Client       // internal client
	maxOpsPerSecond int                // number of requests that can be sent per second
	docUpdatePaths  map[string]bool    // document paths with corresponding writes in the queue
	cancel          context.CancelFunc // context to send cancel message
	bundler         *bundler.Bundler   // handle bundling up writes to Firestore
	ctx             context.Context    // context for canceling all BulkWriter operations
	openLock        sync.Mutex         // guards against setting isOpen concurrently
	isOpen          bool               // flag that the BulkWriter is closed
}

// newBulkWriter creates a new instance of the BulkWriter. This
// version of BulkWriter is intended to be used within go routines by the
// callers.
func newBulkWriter(ctx context.Context, c *Client, database string) *BulkWriter {
	// Although typically we shouldn't store Context objects, in this case we
	// need to pass this Context through to the Bundler
	ctx, cancel := context.WithCancel(withResourceHeader(ctx, c.path()))

	bw := &BulkWriter{
		database:        database,
		start:           time.Now(),
		vc:              c.c,
		isOpen:          true,
		maxOpsPerSecond: defaultStartingMaximumOpsPerSecond,
		docUpdatePaths:  make(map[string]bool),
		cancel:          cancel,
		ctx:             ctx,
	}

	// can't initialize with struct above; need instance reference to BulkWriter.send()
	bw.bundler = bundler.NewBundler(BulkWriterJob{}, bw.send)
	bw.bundler.HandlerLimit = bw.maxOpsPerSecond
	bw.bundler.BundleCountThreshold = maxBatchSize

	return bw
}

// End sends all enqueued writes in parallel and closes the BulkWriter to new requests.
// After calling End(), calling any additional method automatically returns
// with an error. This method completes when there are no more pending writes
// in the queue.
func (b *BulkWriter) End() {
	b.Flush()
	b.openLock.Lock()
	b.isOpen = false
	b.openLock.Unlock()
}

// Flush commits all writes that have been enqueued up to this point in parallel.
// This method blocks execution.
func (bw *BulkWriter) Flush() {
	bw.bundler.Flush()
}

// Create adds a document creation write to the queue of writes to send.
// Note: You cannot write to (Create, Update, Set, or Delete) the same document more than once.
func (bw *BulkWriter) Create(doc *DocumentRef, datum interface{}) (*BulkWriterJob, error) {
	err := bw.checkWriteConditions(doc)
	if err != nil {
		return nil, err
	}

	// TODO(telpirion): Talk to FS team about why we're doing this
	w, err := doc.newCreateWrites(datum)
	if err != nil {
		return nil, fmt.Errorf("firestore: cannot create %v with %v; reason: %v", doc.ID, datum, err)
	}

	if len(w) > 1 {
		return nil, fmt.Errorf("firestore: too many document writes sent to bulkwriter")
	}

	j := bw.write(w[0])
	return j, nil
}

// Delete adds a document deletion write to the queue of writes to send.
// Note: You cannot write to (Create, Update, Set, or Delete) the same document more than once.
func (bw *BulkWriter) Delete(doc *DocumentRef, preconds ...Precondition) (*BulkWriterJob, error) {
	err := bw.checkWriteConditions(doc)
	if err != nil {
		return nil, err
	}

	w, err := doc.newDeleteWrites(preconds)
	if err != nil {
		return nil, fmt.Errorf("firestore: cannot delete doc %v; reason: %v", doc.ID, err)
	}

	if len(w) > 1 {
		return nil, fmt.Errorf("firestore: too many document writes sent to bulkwriter")
	}

	j := bw.write(w[0])
	return j, nil
}

// Set adds a document set write to the queue of writes to send.
// Note: You cannot write to (Create, Update, Set, or Delete) the same document more than once.
func (bw *BulkWriter) Set(doc *DocumentRef, datum interface{}, opts ...SetOption) (*BulkWriterJob, error) {
	err := bw.checkWriteConditions(doc)
	if err != nil {
		return nil, err
	}

	w, err := doc.newSetWrites(datum, opts)
	if err != nil {
		return nil, fmt.Errorf("firestore: cannot set %v on doc %v; reason: %v", datum, doc.ID, err)
	}

	if len(w) > 1 {
		return nil, fmt.Errorf("firestore: too many writes sent to bulkwriter")
	}

	j := bw.write(w[0])
	return j, nil
}

// Update adds a document update write to the queue of writes to send.
// Note: You cannot write to (Create, Update, Set, or Delete) the same document more than once.
func (bw *BulkWriter) Update(doc *DocumentRef, updates []Update, preconds ...Precondition) (*BulkWriterJob, error) {
	err := bw.checkWriteConditions(doc)
	if err != nil {
		return nil, err
	}

	w, err := doc.newUpdatePathWrites(updates, preconds)
	if err != nil {
		return nil, fmt.Errorf("firestore: cannot update doc %v; reason: %v", doc.ID, err)
	}

	if len(w) > 1 {
		return nil, fmt.Errorf("firestore: too many writes sent to bulkwriter")
	}

	j := bw.write(w[0])
	return j, nil
}

// checkConditions determines whether this write attempt is valid. It returns
// an error if either the BulkWriter has already been closed or if it
// receives a nil document reference.
func (bw *BulkWriter) checkWriteConditions(doc *DocumentRef) error {
	bw.openLock.Lock()
	if !bw.isOpen {
		return errors.New("firestore: BulkWriter has been closed")
	}
	bw.openLock.Unlock()

	if doc == nil {
		return errors.New("firestore: nil document contents")
	}

	_, havePath := bw.docUpdatePaths[doc.shortPath]
	if havePath {
		return fmt.Errorf("firestore: bulkwriter: received duplicate write for path: %v", doc.shortPath)
	}

	bw.docUpdatePaths[doc.shortPath] = true

	return nil
}

// write packages up write requests into bulkWriterJob objects.
func (bw *BulkWriter) write(w *pb.Write) *BulkWriterJob {

	j := &BulkWriterJob{
		result: make(chan *pb.WriteResult, 1),
		write:  w,
		err:    make(chan error, 1),
		ctx:    bw.ctx,
	}

	err := bw.bundler.Add(j, 0) // ignore operation size constraints; can't be inferred at compile time
	if err != nil {
		j.setError(err)
	}
	return j
}

// send transmits writes to the service and matches response results to job channels.
// This method takes an empty interface to conform to the contract of the Bundler event handler.
func (bw *BulkWriter) send(i interface{}) {
	bwj := i.([]BulkWriterJob)

	if len(bwj) == 0 {
		return
	}

	var ws []*pb.Write
	for _, w := range bwj {
		ws = append(ws, w.write)
	}

	bwr := &pb.BatchWriteRequest{
		Database: bw.database,
		Writes:   ws,
		Labels:   map[string]string{},
	}

	select {
	case <-bw.ctx.Done():
		return
	default:
		resp, err := bw.vc.BatchWrite(bw.ctx, bwr)
		if err != nil {
			// Do we need to be selective about what kind of errors we send?
			for _, j := range bwj {
				j.setError(err)
			}
			return
		}
		// Iterate over the response. Match successful requests with unsuccessful
		// requests.
		for i, res := range resp.WriteResults {
			// Get the status code for this WriteResult
			s := resp.Status[i]
			c := s.GetCode()
			if c != 0 { // Should we do an explicit check against rpc.Code enum?
				j := bwj[i]
				j.attempts++

				// Do we need separate retry bundler
				if j.attempts < maxRetryAttempts {
					bw.bundler.Add(j, 0) // ignore operation size constraints; can't be inferred at compile time
				}
				continue
			}

			bwj[i].setResult(res)
		}
		// This means the writes are now finalized, all retries completed
	}
}
