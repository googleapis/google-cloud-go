package firestore

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	vkit "cloud.google.com/go/firestore/apiv1"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
)

const (
	// MaxBatchSize is the max number of writes to send in a request
	MaxBatchSize = 20
	// RetryMaxBatchSize is the max number of writes to send in a retry request
	RetryMaxBatchSize = 10
	// MaxRetryAttempts is the max number of times to retry a write
	MaxRetryAttempts = 10
	// DefaultStartingMaximumOpsPerSecond is the starting max number of requests to the service per second
	DefaultStartingMaximumOpsPerSecond = 500
	// RateLimiterMultiplier is the amount to increase maximum ops (qps) every 5 minutes
	RateLimiterMultiplier       = 1.5
	rateLimiterMultiplierMillis = 5 * 60 * 1000 // 5 minutes in milliseconds
	coolingPeriodMillis         = 2.0           // starting time to wait in between requests
)

type bulkWriterJob struct {
	err      chan error           // send error responses to this channel
	result   chan *pb.WriteResult // send the write results to this channel
	write    *pb.Write            // the writes to apply to the database
	attempts int                  // number of times this write has been attempted
}

type bulkWriterBatch struct {
	bwr *pb.BatchWriteRequest // request to send to the service
	bwj []bulkWriterJob       // corresponding jobs to return response
}

type bulkWriterRequestBatch struct {
	isFlushing bool              // flag that indicates that BulkWriter needs to schedule a flush of the queue
	bwb        []bulkWriterBatch // all the bulkWriterBatch objects to schedule
}

// The BulkWriterStatus provides details about the BulkWriter, including the
// number of writes processed, the number of requests sent to the service, and
// the number of writes in the queue.
type BulkWriterStatus struct {
	WritesProvidedCount int  // number of write requests provided by caller
	IsOpen              bool // whether this BulkWriter is open or closed
	WritesInQueueCount  int  // number of write requests in the queue
	WritesSentCount     int  // number of requests sent to the service
	WritesReceivedCount int  // number of WriteResults received from the service
}

// A BulkWriter allows multiple document writes in parallel. The BulkWriter
// submits document writes in maximum batches of 20 writes per request. Each
// request can contain many different document writes: create, delete, update,
// and set are all supported. Only one operation per document is allowed.
// Each call to Create, Update, Set, and Delete can return a value and error.
type BulkWriter struct {
	database        string                      // the database as resource name: projects/[PROJECT]/databases/[DATABASE]
	reqs            int64                       // total number of requests sent
	start           time.Time                   // when this BulkWriter was started
	vc              *vkit.Client                // internal client
	isOpen          bool                        // signal that the BulkWriter is closed
	flush           chan struct{}               // signal the beginning/end of flushing operation
	isFlushing      bool                        // determines whether we're in a flush state
	queue           chan bulkWriterJob          // incoming write requests
	scheduled       chan bulkWriterRequestBatch // scheduled requests
	sendingQueue    []bulkWriterJob             // queue of requests to send
	backlogQueue    []bulkWriterJob             // queue of requests to store in memory during flush operation
	maxOpsPerSecond int                         // number of requests that can be sent per second.
	docUpdatePaths  []string                    // document paths with corresponding writes in the queue.
	waitTime        float64                     // time to wait in between requests; increase exponentially
	writesProvided  int64                       // number of writes provided by caller
	writesSent      int64                       // number of writes sent to Firestore
	writesReceived  int64                       // number of writes results received from Firestore
}

// newBulkWriter creates a new instance of the BulkWriter. This
// version of BulkWriter is intended to be used within go routines by the
// callers.
func newBulkWriter(ctx context.Context, c *Client, database string) *BulkWriter {
	bw := BulkWriter{
		vc:              c.c,
		database:        database,
		isOpen:          true,
		flush:           make(chan struct{}),
		isFlushing:      false,
		queue:           make(chan bulkWriterJob),
		scheduled:       make(chan bulkWriterRequestBatch),
		start:           time.Now(),
		maxOpsPerSecond: DefaultStartingMaximumOpsPerSecond,
		waitTime:        coolingPeriodMillis,
	}

	// Start the call receiving thread and request sending thread
	// enocom: We are using memory to get more speed, be aware of memory usage
	go bw.receiver(withResourceHeader(ctx, c.path()))
	go bw.scheduler(withResourceHeader(ctx, c.path()))

	// TODO(telpirion): Create a retry thread?

	return &bw
}

// End sends all enqueued writes in parallel and closes the BulkWriter to new requests.
// CANNOT BE DEFERRED. Deferring a call to End() can cause a deadlock.
// After calling End(), calling any additional method automatically returns
// with an error. This method completes when there are no more pending writes
// in the queue.
func (b *BulkWriter) End() {
	// Make sure that flushing actually happens before isOpen changes values.
	b.Flush()
	b.isOpen = false
}

// Flush commits all writes that have been enqueued up to this point in parallel.
// CANNOT BE DEFERRED. Deferring a call to Flush() can cause a deadlock.
// This method blocks execution.
func (b *BulkWriter) Flush() {
	b.flush <- struct{}{}

	// Ensure that the backlogQueue is empty
	b.backlogQueue = []bulkWriterJob{}

	// Block until we get the signal that we're done flushing.
	<-b.flush

	// Repopulate sending queue now that flush operation is done.
	b.sendingQueue = b.backlogQueue
}

// Status returns the current state of the BulkWriter, including whether it is open and
// the number of writes provided by the caller, writes in the queue, writes sent, and
// the writes received.
func (bw *BulkWriter) Status() BulkWriterStatus {
	return BulkWriterStatus{
		IsOpen:              bw.isOpen,
		WritesInQueueCount:  len(bw.sendingQueue),
		WritesProvidedCount: int(bw.writesProvided),
		WritesSentCount:     int(bw.writesSent),
		WritesReceivedCount: int(bw.writesReceived),
	}
}

// Create adds a document creation write to the queue of writes to send.
func (bw *BulkWriter) Create(doc *DocumentRef, datum interface{}) (*WriteResult, error) {
	err := bw.checkWriteConditions(doc)
	if err != nil {
		return nil, err
	}

	w, err := doc.newCreateWrites(datum)
	if err != nil {
		return nil, fmt.Errorf("firestore: cannot create %v with %v", doc.ID, datum)
	}

	wc, ec := bw.write(w[0])
	wr, err := bw.processResults(wc, ec)
	return wr, err
}

// Delete adds a document deletion write to the queue of writes to send.
func (bw *BulkWriter) Delete(doc *DocumentRef, preconds ...Precondition) (*WriteResult, error) {
	err := bw.checkWriteConditions(doc)
	if err != nil {
		return nil, err
	}

	w, err := doc.newDeleteWrites(preconds)
	if err != nil {
		return nil, fmt.Errorf("firestore: cannot delete doc %v", doc.ID)
	}

	wd, ec := bw.write(w[0])
	wr, err := bw.processResults(wd, ec)
	return wr, err
}

// Set adds a document set write to the queue of writes to send.
func (bw *BulkWriter) Set(doc *DocumentRef, datum interface{}, opts ...SetOption) (*WriteResult, error) {
	err := bw.checkWriteConditions(doc)
	if err != nil {
		return nil, err
	}

	w, err := doc.newSetWrites(datum, opts)
	if err != nil {
		return nil, fmt.Errorf("firestore: cannot set %v on doc %v", datum, doc.ID)
	}

	ws, ec := bw.write(w[0])
	wr, err := bw.processResults(ws, ec)
	return wr, err
}

// Update adds a document update write to the queue of writes to send.
func (bw *BulkWriter) Update(doc *DocumentRef, updates []Update, preconds ...Precondition) (*WriteResult, error) {
	err := bw.checkWriteConditions(doc)
	if err != nil {
		return nil, err
	}

	w, err := doc.newUpdatePathWrites(updates, preconds)
	if err != nil {
		return nil, fmt.Errorf("firestore: cannot update doc %v", doc.ID)
	}
	wu, ec := bw.write(w[0])
	wr, err := bw.processResults(wu, ec)
	return wr, err
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

	for _, e := range bw.docUpdatePaths {
		if doc.shortPath == e {
			return fmt.Errorf("firestore: BulkWriter received duplicate write for path: %v", doc.shortPath)
		}
	}

	bw.docUpdatePaths = append(bw.docUpdatePaths, doc.shortPath)
	return nil
}

// write packages up write requests into bulkWriterJob objects.
func (bw *BulkWriter) write(w *pb.Write) (chan *pb.WriteResult, chan error) {
	r := make(chan *pb.WriteResult, 1)
	e := make(chan error, 1)

	j := bulkWriterJob{
		result: r,
		write:  w,
		err:    e,
	}

	// Write successfully created, increment count
	bw.writesProvided++

	bw.queue <- j
	return r, e
}

// processResults checks for errors returned from send() and packages up the
// results as WriteResult objects
func (bw *BulkWriter) processResults(w chan *pb.WriteResult, e chan error) (*WriteResult, error) {
	wrpb := <-w
	err := <-e

	if err != nil {
		return nil, err
	}

	wr, err := writeResultFromProto(wrpb)
	if err != nil {
		return nil, err
	}
	return wr, nil
}

// receiver gets write requests from the caller and sends BatchWriteRequests to the scheduler.
// The receiver method is the main "event loop" of the BulkWriter. It maintains
// the communication routes in between the caller and calls to the service.
func (bw *BulkWriter) receiver(ctx context.Context) {
	for {
		log.Println("receiver: receiving")
		var flushOp bool
		var bs []bulkWriterBatch

		select {
		case bwj := <-bw.queue:
			bw.sendingQueue = append(bw.sendingQueue, bwj)
			if bw.isFlushing {
				bw.backlogQueue = append(bw.backlogQueue, bwj)
			} else if len(bw.sendingQueue) > MaxBatchSize {
				bs = bw.buildRequests(false)
			}
			// should we block from adding to the queue until a flushing job is complete?
		case <-bw.flush: // enocom: Need a mutex around here, the bools aren't thread safe
			log.Println("receiver: got call to flush")
			bw.isFlushing = true
			bs = bw.buildRequests(true)
			flushOp = true
		}

		bwb := bulkWriterRequestBatch{
			isFlushing: flushOp,
			bwb:        bs,
		}

		// Send batch of requests to scheduler
		bw.scheduled <- bwb

		// BulkWriter has received call to Close()
		if !bw.isOpen {
			break
		}
		log.Println("receiver: end receiving")
	}
	close(bw.queue)
	close(bw.scheduled)
}

// makeBatch creates MAX_BATCH_SIZE (or smaller) bundles of bulkWriterJobs for sending.
func (bw *BulkWriter) makeBatch() (bulkWriterBatch, error) {

	qs := len(bw.sendingQueue)
	var b []bulkWriterJob

	// Don't index outside-of-bounds
	if qs < MaxBatchSize {
		b = bw.sendingQueue[:qs]
		bw.sendingQueue = []bulkWriterJob{}

	} else {
		b = bw.sendingQueue[:MaxBatchSize]
		bw.sendingQueue = bw.sendingQueue[MaxBatchSize:]
	}

	// Get the writes out of the jobs
	var ws []*pb.Write
	for _, j := range b {
		if j.attempts < MaxRetryAttempts {
			ws = append(ws, j.write)
		}
	}

	// Guardrail -- check whether no writes to apply
	if len(ws) == 0 {
		return bulkWriterBatch{}, fmt.Errorf("no writes to apply")
	}

	// Compose our request
	bwr := pb.BatchWriteRequest{
		Database: bw.database,
		Writes:   ws,
		Labels:   map[string]string{},
	}

	return bulkWriterBatch{bwr: &bwr, bwj: b}, nil
}

// buildRequests bundles batches of writes into a series of batches
func (bw *BulkWriter) buildRequests(isFlushing bool) []bulkWriterBatch {
	// Build up the group of batches to send.
	var bs []bulkWriterBatch
	if isFlushing {
		for len(bw.sendingQueue) > 0 {
			b, err := bw.makeBatch()
			if err == nil {
				bs = append(bs, b)
			}
		}
	} else {
		b, err := bw.makeBatch()
		if err == nil {
			bs = append(bs, b)
		}
	}
	return bs
}

// scheduler manages the timing and rate multiplier logic for sending requests to the service.
func (bw *BulkWriter) scheduler(ctx context.Context) {
	for bwr := range bw.scheduled { // bw.scheduled is a channel of bulkWriterRequestBatch objects

		// Check for Context.Done()

		for _, b := range bwr.bwb {
			elapsed := (bw.start.UnixMilli() - time.Now().UnixMilli()) / 1000
			var qps int64
			// Don't divide by 0!
			if elapsed == 0 {
				qps = 0
			} else {
				qps = bw.reqs / elapsed
			}

			wpb := len(b.bwr.Writes)
			bw.writesSent += int64(wpb)

			// TODO(telpirion): Decide on a back off strategy
			if qps >= int64(bw.maxOpsPerSecond) {
				time.Sleep(time.Duration(bw.waitTime))
				bw.waitTime = math.Pow(float64(bw.waitTime), 2)

				// Increase number of requests per second at the five minute mark
				mins := elapsed / 60
				if mins%5 == 0 {
					newOps := float64(bw.maxOpsPerSecond) * RateLimiterMultiplier
					bw.maxOpsPerSecond = int(newOps)
				}
			}

			go bw.send(ctx, b.bwr, b.bwj)
		}

		// if this bulkWriterRequestBatch is a flush job, report that it is now done.
		if bwr.isFlushing {
			bw.isFlushing = false
			bw.flush <- struct{}{}
		}
	}
}

// send transmits writes to the service and matches response results to job channels.
func (bw *BulkWriter) send(ctx context.Context, bwr *pb.BatchWriteRequest, bwj []bulkWriterJob) {
	bw.reqs++
	resp, err := bw.vc.BatchWrite(ctx, bwr)
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
			bw.sendingQueue = append(bw.sendingQueue, bwj[i])
			continue
		}

		bw.writesReceived++ // This means the writes are now finalized, all retries completed
		bwj[i].result <- res
		bwj[i].err <- nil
	}
}
