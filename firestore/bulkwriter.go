package firestore

import (
	"context"
	"errors"
	"fmt"
	"math"
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
	// rateLimiterMultiplier is the amount to increase maximum ops (qps) every 5 minutes
	rateLimiterMultiplier = 1.5
	// 5 minutes in milliseconds
	rateLimiterMultiplierMillis = 5 * 60 * 1000
	// Starting time to wait in between requests
	coolingPeriodMillis = 2.0
)

// bulkWriterOperation is used internally to track the status of an individual
// document write.
type bulkWriterOperation struct {
	err      chan error           // send error responses to this channel
	result   chan *pb.WriteResult // send the write results to this channel
	write    *pb.Write            // the writes to apply to the database
	attempts int                  // number of times this write has been attempted
	size     int                  // size of this write, in bytes
}

// BulkWriterStatus provides details about the BulkWriter, including the
// number of writes processed, the number of requests sent to the service, and
// the number of writes in the queue.
type BulkWriterStatus struct {
	// IsOpen shows whether this BulkWriter is open or closed
	IsOpen bool
}

// BulkWriterJob provides read-only access to the results of a BulkWriter write attempt.
type BulkWriterJob struct {
	op bulkWriterOperation // the operation this BulkWriterJob encapsulates
}

// Results gets the results of the BulkWriter write attempt
// Attempting to access the results before the results have been received blocks
// subsequent calls (until the result is received).
func (j *BulkWriterJob) Results() (*WriteResult, error) {
	return processResults(j.op)
}

// A BulkWriter allows multiple document writes in parallel. The BulkWriter
// submits document writes in maximum batches of 20 writes per request. Each
// request can contain many different document writes: create, delete, update,
// and set are all supported. Only one operation per document is allowed.
// Each call to Create, Update, Set, and Delete can return a value and error.
type BulkWriter struct {
	database        string             // the database as resource name: projects/[PROJECT]/databases/[DATABASE]
	start           time.Time          // when this BulkWriter was started
	vc              *vkit.Client       // internal client
	isOpen          bool               // signal that the BulkWriter is closed
	maxOpsPerSecond int                // number of requests that can be sent per second.
	docUpdatePaths  sync.Map           // document paths with corresponding writes in the queue.
	waitTime        float64            // time to wait in between requests; increase exponentially
	cancel          context.CancelFunc // context to send cancel message
	bundler         *bundler.Bundler   // handle bundling up writes to Firestore
	retryBundler    *bundler.Bundler   // handle bundling up retries of writes to Firestore
	ctx             context.Context    // context for canceling BulkWriter operations
}

// newBulkWriter creates a new instance of the BulkWriter. This
// version of BulkWriter is intended to be used within go routines by the
// callers.
func newBulkWriter(ctx context.Context, c *Client, database string) *BulkWriter {

	var dsm sync.Map

	ctx, cancel := context.WithCancel(withResourceHeader(ctx, c.path()))

	bw := &BulkWriter{
		vc:              c.c,
		database:        database,
		isOpen:          true,
		start:           time.Now(),
		maxOpsPerSecond: defaultStartingMaximumOpsPerSecond,
		waitTime:        coolingPeriodMillis,
		cancel:          cancel,
		docUpdatePaths:  dsm,
		ctx:             ctx,
	}

	bw.bundler = bundler.NewBundler(bulkWriterOperation{}, bw.send)
	bw.bundler.HandlerLimit = bw.maxOpsPerSecond
	bw.bundler.BundleCountThreshold = maxBatchSize

	bw.retryBundler = bundler.NewBundler(bulkWriterOperation{}, bw.send)
	bw.retryBundler.BundleCountThreshold = retryMaxBatchSize

	return bw
}

// End sends all enqueued writes in parallel and closes the BulkWriter to new requests.
// After calling End(), calling any additional method automatically returns
// with an error. This method completes when there are no more pending writes
// in the queue.
func (b *BulkWriter) End() {
	b.Flush()
	b.cancel()
	b.isOpen = false
}

// Flush commits all writes that have been enqueued up to this point in parallel.
// This method blocks execution.
func (bw *BulkWriter) Flush() {
	// Ensure that the backlogQueue is empty
	bw.bundler.Flush()
}

// Status gets the current open or closed state of the BulkWriter.
func (bw *BulkWriter) Status() BulkWriterStatus {
	return BulkWriterStatus{
		IsOpen: bw.isOpen,
	}
}

// Create adds a document creation write to the queue of writes to send.
func (bw *BulkWriter) Create(doc *DocumentRef, datum interface{}) (*BulkWriterJob, error) {
	err := bw.checkWriteConditions(doc)
	if err != nil {
		return nil, err
	}

	w, err := doc.newCreateWrites(datum)
	if err != nil {
		return nil, fmt.Errorf("firestore: cannot create %v with %v", doc.ID, datum)
	}

	return &BulkWriterJob{
		op: bw.write(w[0]),
	}, nil
}

// Delete adds a document deletion write to the queue of writes to send.
func (bw *BulkWriter) Delete(doc *DocumentRef, preconds ...Precondition) (*BulkWriterJob, error) {
	err := bw.checkWriteConditions(doc)
	if err != nil {
		return nil, err
	}

	w, err := doc.newDeleteWrites(preconds)
	if err != nil {
		return nil, fmt.Errorf("firestore: cannot delete doc %v", doc.ID)
	}

	return &BulkWriterJob{
		op: bw.write(w[0]),
	}, nil
}

// Set adds a document set write to the queue of writes to send.
func (bw *BulkWriter) Set(doc *DocumentRef, datum interface{}, opts ...SetOption) (*BulkWriterJob, error) {
	err := bw.checkWriteConditions(doc)
	if err != nil {
		return nil, err
	}

	w, err := doc.newSetWrites(datum, opts)
	if err != nil {
		return nil, fmt.Errorf("firestore: cannot set %v on doc %v", datum, doc.ID)
	}

	return &BulkWriterJob{
		op: bw.write(w[0]),
	}, nil
}

// Update adds a document update write to the queue of writes to send.
func (bw *BulkWriter) Update(doc *DocumentRef, updates []Update, preconds ...Precondition) (*BulkWriterJob, error) {
	err := bw.checkWriteConditions(doc)
	if err != nil {
		return nil, err
	}

	w, err := doc.newUpdatePathWrites(updates, preconds)
	if err != nil {
		return nil, fmt.Errorf("firestore: cannot update doc %v", doc.ID)
	}
	return &BulkWriterJob{
		op: bw.write(w[0]),
	}, nil
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

	_, loaded := bw.docUpdatePaths.LoadOrStore(doc.shortPath, struct{}{})
	if loaded {
		return fmt.Errorf("firestore: bulkwriter: received duplicate write for path: %v", doc.shortPath)
	}

	return nil
}

// write packages up write requests into bulkWriterJob objects.
func (bw *BulkWriter) write(w *pb.Write) bulkWriterOperation {
	r := make(chan *pb.WriteResult, 1)
	e := make(chan error, 1)

	j := bulkWriterOperation{
		result: r,
		write:  w,
		err:    e,
		size:   20, // TODO: Compute actual payload size
	}

	bw.bundler.Add(j, j.size)
	return j
}

// processResults checks for errors returned from send() and packages up the
// results as WriteResult objects
func processResults(o bulkWriterOperation) (*WriteResult, error) {
	wpb := <-o.result
	err := <-o.err

	if err != nil {
		return nil, err
	}

	wr, err := writeResultFromProto(wpb)
	if err != nil {
		return nil, err
	}
	return wr, nil
}

// send transmits writes to the service and matches response results to job channels.
func (bw *BulkWriter) send(i interface{}) {
	bwj := i.([]bulkWriterOperation)

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
			s := resp.Status[i]

			c := s.GetCode()

			if c != 0 { // Should we do an explicit check against rpc.Code enum?
				bw.retryBundler.Add(bwj[i], bwj[i].size)
				continue
			}

			bwj[i].result <- res
			bwj[i].err <- nil
		}
		// This means the writes are now finalized, all retries completed
	}

	// Check whether we need to increase the rate of requests to the service
	// after each result
	bw.increaseRate()
}

// increaseRate updates the number of bundler requests that can be concurrently open
func (bw *BulkWriter) increaseRate() {
	elapsed := time.Now().Sub(bw.start).Seconds()

	// Ideally, we would determine the qps before increasing, but instead
	// we simply increase the number of bundler handler invocations we have
	// running at once.
	mins := elapsed / 60
	if math.Mod(mins, 5) <= 0.1 {
		newOps := float64(bw.maxOpsPerSecond) * rateLimiterMultiplier
		bw.maxOpsPerSecond = int(newOps)
		bw.bundler.HandlerLimit = int(newOps)
	}
}
