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
	err      chan error           // send error responses to this channel
	result   chan *pb.WriteResult // send the write results to this channel
	write    *pb.Write            // the writes to apply to the database
	attempts int                  // number of times this write has been attempted
	size     int                  // size of this write, in bytes
	wr       *WriteResult         // (cached) result from the operation
	e        error                // (cached) any errors that occurred
}

// Results gets the results of the BulkWriter write attempt
// Attempting to access the results before the results have been received blocks
// subsequent calls (until the result is received).
func (j *BulkWriterJob) Results() (*WriteResult, error) {
	if j.wr == nil && j.e == nil {
		j.wr, j.e = j.processResults() // cache the results for additional calls
	}
	return j.wr, j.e
}

// processResults checks for errors returned from send() and packages up the
// results as WriteResult objects
func (o *BulkWriterJob) processResults() (*WriteResult, error) {
	//  TODO(telpirion): Refactor this to use select/case
	wpb := <-o.result
	err := <-o.err

	if err != nil {
		return nil, err
	}

	wr, err := writeResultFromProto(wpb)

	if err != nil {
		return nil, err
	}
	return wr, err
}

// A BulkWriter allows multiple document writes in parallel. The BulkWriter
// submits document writes in maximum batches of 20 writes per request. Each
// request can contain many different document writes: create, delete, update,
// and set are all supported. Only one operation per document is allowed.
// Each call to Create, Update, Set, and Delete can return a value and error.
type BulkWriter struct {
	database        string             // the database as resource name: projects/[PROJECT]/databases/[DATABASE]
	start           time.Time          // when this BulkWriter was started; used to calculate qps and rate increases
	vc              *vkit.Client       // internal client
	isOpen          bool               // flag that the BulkWriter is closed
	maxOpsPerSecond int                // number of requests that can be sent per second
	docUpdatePaths  map[string]bool    // document paths with corresponding writes in the queue
	cancel          context.CancelFunc // context to send cancel message
	bundler         *bundler.Bundler   // handle bundling up writes to Firestore
	ctx             context.Context    // context for canceling BulkWriter operations
	openLock        sync.Mutex         // guards against setting isOpen concurrently
}

// newBulkWriter creates a new instance of the BulkWriter. This
// version of BulkWriter is intended to be used within go routines by the
// callers.
func newBulkWriter(ctx context.Context, c *Client, database string) *BulkWriter {
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
	bw.bundler.BundleByteLimit = 0 // unlimited size

	return bw
}

// End sends all enqueued writes in parallel and closes the BulkWriter to new requests.
// After calling End(), calling any additional method automatically returns
// with an error. This method completes when there are no more pending writes
// in the queue.
func (b *BulkWriter) End() {
	b.Flush()
	b.cancel()
	b.openLock.Lock()
	b.isOpen = false
	b.openLock.Unlock()
}

// Flush commits all writes that have been enqueued up to this point in parallel.
// This method blocks execution.
func (bw *BulkWriter) Flush() {
	// Ensure that the backlogQueue is empty
	bw.bundler.Flush()
}

// IsOpen gets the current open or closed state of the BulkWriter.
func (bw *BulkWriter) IsOpen() bool {
	bw.openLock.Lock()
	defer bw.openLock.Unlock()
	return bw.isOpen
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
		return nil, fmt.Errorf("firestore: cannot create %v with %v", doc.ID, datum)
	}

	if len(w) > 1 {
		return nil, fmt.Errorf("firestore: too many document writes sent to bulkwriter")
	}

	j := bw.write(w[0])
	return &j, nil
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
		return nil, fmt.Errorf("firestore: cannot delete doc %v", doc.ID)
	}

	if len(w) > 1 {
		return nil, fmt.Errorf("firestore: too many document writes sent to bulkwriter")
	}

	j := bw.write(w[0])
	return &j, nil
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
		return nil, fmt.Errorf("firestore: cannot set %v on doc %v", datum, doc.ID)
	}

	if len(w) > 1 {
		return nil, fmt.Errorf("firestore: too many writes sent to bulkwriter")
	}

	j := bw.write(w[0])
	return &j, nil
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
		return nil, fmt.Errorf("firestore: cannot update doc %v", doc.ID)
	}

	if len(w) > 1 {
		return nil, fmt.Errorf("firestore: too many writes sent to bulkwriter")
	}

	j := bw.write(w[0])
	return &j, nil
}

// checkConditions determines whether this write attempt is valid. It returns
// an error if either the BulkWriter has already been closed or if it
// receives a nil document reference.
func (bw *BulkWriter) checkWriteConditions(doc *DocumentRef) error {
	if !bw.isOpen {
		return errors.New("firestore: BulkWriter has been closed")
	}

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
func (bw *BulkWriter) write(w *pb.Write) BulkWriterJob {
	r := make(chan *pb.WriteResult, 1)
	e := make(chan error, 1)

	j := BulkWriterJob{
		result: r,
		write:  w,
		err:    e,
		size:   0, // ignore operation size constraints; can't be inferred at compile time
	}

	err := bw.bundler.Add(j, j.size)
	if err != nil {
		j.err <- err
		j.result <- nil
	}
	return j
}

// send transmits writes to the service and matches response results to job channels.
func (bw *BulkWriter) send(i interface{}) {
	bwj := i.([]BulkWriterJob)

	if len(bwj) == 0 {
		return
	}

	var ws []*pb.Write
	for _, w := range bwj {
		ws = append(ws, w.write)
	}

	bwr := pb.BatchWriteRequest{
		Database: bw.database,
		Writes:   ws,
		Labels:   map[string]string{},
	}

	select {
	case <-bw.ctx.Done():
		break
	default:
		resp, err := bw.vc.BatchWrite(bw.ctx, &bwr)
		if err != nil {
			// Do we need to be selective about what kind of errors we send?
			for _, j := range bwj {
				j.result <- nil
				j.err <- err
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
					bw.bundler.Add(j, j.size)
				}
				continue
			}

			bwj[i].result <- res
			bwj[i].err <- nil
		}
		// This means the writes are now finalized, all retries completed
	}
}
