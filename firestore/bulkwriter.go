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
	MAX_BATCH_SIZE                          = 20            // max number of writes to send in a request
	RETRY_MAX_BATCH_SIZE                    = 10            // max number of writes to send in a retry request
	MAX_RETRY_ATTEMPTS                      = 10            // max number of times to retry a write
	DEFAULT_STARTING_MAXIMUM_OPS_PER_SECOND = 500           // starting max number of requests to the service per second
	RATE_LIMITER_MULTIPLIER                 = 1.5           // amount to increase DEFAULT_STARTING_MAXIMUM_OPS_PER_SECOND every 5 minutes
	RATE_LIMITER_MULTIPLIER_MILLIS          = 5 * 60 * 1000 // 5 minutes in milliseconds
	cooling_period_millis                   = 2.0           // starting time to wait in between requests
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
	ctx             context.Context             // context
	reqs            int64                       // total number of requests sent
	start           time.Time                   // when this BulkWriter was started
	vc              *vkit.Client                // internal client
	isOpen          bool                        // signal that the BulkWriter is closed
	flush           chan bool                   // signal that the BulkWriter needs to flush the queue
	isFlushing      bool                        // determines that a flush has occurred or not
	doneFlushing    chan bool                   // signals that the BulkWriter has flushed the queue
	queue           chan bulkWriterJob          // incoming write requests
	scheduled       chan bulkWriterRequestBatch // scheduled requests
	backlogQueue    []bulkWriterJob             // backlog of requests to send
	maxOpsPerSecond int                         // number of requests that can be sent per second.
	docUpdatePaths  []string                    // document paths with corresponding writes in the queue.
	waitTime        float64                     // time to wait in between requests; increase exponentially
	writesProvided  int64                       // number of writes provided by caller
	writesSent      int64                       // number of writes sent to Firestore
	writesReceived  int64                       // number of writes results received from Firestore
}

// NewBulkWriter creates a new instance of the BulkWriter. This
// version of BulkWriter is intended to be used within go routines by the
// callers.
func NewBulkWriter(ctx context.Context, database string) (*BulkWriter, error) {
	v, err := vkit.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	f := make(chan bool)
	q := make(chan bulkWriterJob)
	d := make(chan bool)
	s := make(chan bulkWriterRequestBatch)
	bw := BulkWriter{
		ctx:             ctx,
		vc:              v,
		database:        database,
		isOpen:          true,
		flush:           f,
		isFlushing:      false,
		doneFlushing:    d,
		queue:           q,
		scheduled:       s,
		start:           time.Now(),
		maxOpsPerSecond: DEFAULT_STARTING_MAXIMUM_OPS_PER_SECOND,
		waitTime:        cooling_period_millis,
	}

	// Start the call receiving thread and request sending thread
	go bw.receiver()
	go bw.scheduler()

	// TODO(telpirion): Create a retry thread?

	return &bw, nil
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
// This method blocks execution.
func (b *BulkWriter) Flush() {
	b.flush <- true
	// Block until we get the signal that we're done flushing.
	<-b.doneFlushing
}

// QueueCount returns the number of document writes that are currently in the queue.
func (bw *BulkWriter) Status() BulkWriterStatus {
	return BulkWriterStatus{
		IsOpen:              bw.isOpen,
		WritesInQueueCount:  len(bw.backlogQueue),
		WritesProvidedCount: int(bw.writesProvided),
		WritesSentCount:     int(bw.writesSent),
	}
}

// Create adds a document creation write to the queue of writes to send.
func (bw *BulkWriter) Create(doc *DocumentRef, datum interface{}) (*pb.WriteResult, error) {
	err := bw.checkWriteConditions(doc)
	if err != nil {
		return nil, err
	}

	w, err := doc.newCreateWrites(datum)
	if err != nil {
		return nil, fmt.Errorf("firestore: cannot create %v with %v", doc.ID, datum)
	}

	wc, ec := bw.write(w[0])
	return <-wc, <-ec
}

// Delete adds a document deletion write to the queue of writes to send.
func (bw *BulkWriter) Delete(doc *DocumentRef, preconds ...Precondition) (*pb.WriteResult, error) {
	err := bw.checkWriteConditions(doc)
	if err != nil {
		return nil, err
	}

	w, err := doc.newDeleteWrites(preconds)
	if err != nil {
		return nil, fmt.Errorf("firestore: cannot delete doc %v", doc.ID)
	}

	wc, ec := bw.write(w[0])
	return <-wc, <-ec
}

// Set adds a document set write to the queue of writes to send.
func (bw *BulkWriter) Set(doc *DocumentRef, datum interface{}, opts ...SetOption) (*pb.WriteResult, error) {
	err := bw.checkWriteConditions(doc)
	if err != nil {
		return nil, err
	}

	w, err := doc.newSetWrites(datum, opts)
	if err != nil {
		return nil, fmt.Errorf("firestore: cannot set %v on doc %v", datum, doc.ID)
	}

	wc, ec := bw.write(w[0])
	return <-wc, <-ec
}

func (bw *BulkWriter) Update(doc *DocumentRef, updates []Update, preconds ...Precondition) (*pb.WriteResult, error) {
	err := bw.checkWriteConditions(doc)
	if err != nil {
		return nil, err
	}

	w, err := doc.newUpdatePathWrites(updates, preconds)
	if err != nil {
		return nil, fmt.Errorf("firestore: cannot update doc %v", doc.ID)
	}
	wc, ec := bw.write(w[0])
	return <-wc, <-ec
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

// receiver gets write requests from the caller and sends BatchWriteRequests to the scheduler.
// The receiver method is the main "event loop" of the BulkWriter. It maintains
// the communication routes in between the caller and calls to the service.
func (bw *BulkWriter) receiver() {
	for {
		log.Println("receiver: receiving")
		var flushOp bool
		var bs []bulkWriterBatch

		select {
		case bwj := <-bw.queue:
			bw.backlogQueue = append(bw.backlogQueue, bwj)
			if len(bw.backlogQueue) > MAX_BATCH_SIZE {
				bs = bw.buildRequests(false)
			}
			// should we block from adding to the queue until a flushing job is complete?
		case <-bw.flush:
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

	qs := len(bw.backlogQueue)
	var b []bulkWriterJob

	// Don't index outside-of-bounds
	if qs < MAX_BATCH_SIZE {
		b = bw.backlogQueue[:qs]
		bw.backlogQueue = []bulkWriterJob{}

	} else {
		b = bw.backlogQueue[:MAX_BATCH_SIZE]
		bw.backlogQueue = bw.backlogQueue[MAX_BATCH_SIZE:]
	}

	// Get the writes out of the jobs
	var ws []*pb.Write
	for _, j := range b {
		if j.attempts < MAX_RETRY_ATTEMPTS {
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
		for len(bw.backlogQueue) > 0 {
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
func (bw *BulkWriter) scheduler() {
	for bwr := range bw.scheduled { // bw.scheduled is a channel of bulkWriterRequestBatch objects

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

			if qps < int64(bw.maxOpsPerSecond) {
				go bw.send(b.bwr, b.bwj)
			} else {
				// TODO(erschmid): Decide on a back off strategy
				time.Sleep(time.Duration(bw.waitTime))
				bw.waitTime = math.Pow(float64(bw.waitTime), 2)

				go bw.send(b.bwr, b.bwj)

				// Increase number of requests per second at the five minute mark
				mins := elapsed / 60
				if mins%5 == 0 {
					newOps := float64(bw.maxOpsPerSecond) * RATE_LIMITER_MULTIPLIER
					bw.maxOpsPerSecond = int(newOps)
				}
			}
		}

		// if this bulkWriterRequestBatch is a flush job, report that it is now done.
		if bwr.isFlushing {
			bw.isFlushing = false
			bw.doneFlushing <- true
		}
	}
}

// send transmits writes to the service and matches response results to job channels.
func (bw *BulkWriter) send(bwr *pb.BatchWriteRequest, bwj []bulkWriterJob) {
	bw.reqs++
	resp, err := bw.vc.BatchWrite(bw.ctx, bwr)
	if err != nil {
		// Do we need to be selective about what kind of errors we send?
		for _, j := range bwj {
			j.result <- nil
			j.err <- err
		}
	}

	// Iterate over the response. Match successful requests with unsuccessful
	// requests.
	for i, res := range resp.WriteResults {
		s := resp.Status[i]

		c := s.GetCode()

		if c != 0 { // Should we do an explicit check against rpc.Code enum?
			bw.backlogQueue = append(bw.backlogQueue, bwj[i])
			continue
		}

		bw.writesReceived++ // This means the writes are now finalized, all retries completed
		bwj[i].result <- res
		bwj[i].err <- nil
	}
}
