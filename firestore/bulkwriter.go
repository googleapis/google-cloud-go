package firestore

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	vkit "cloud.google.com/go/firestore/apiv1"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
)

const (
	MAX_BATCH_SIZE                          = 20
	RETRY_MAX_BATCH_SIZE                    = 10 // Do we even need this?
	MAX_RETRY_ATTEMPTS                      = 10
	DEFAULT_STARTING_MAXIMUM_OPS_PER_SECOND = 500
	RATE_LIMITER_MULTIPLIER                 = 1.5
	RATE_LIMITER_MULTIPLIER_MILLIS          = 5 * 60 * 1000
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

type BulkWriter struct {
	database        string               // the database as resource name: projects/[PROJECT]/databases/[DATABASE]
	ctx             context.Context      // context
	reqs            int64                // total number of requests sent
	start           time.Time            // when this BulkWriter was started
	vc              *vkit.Client         // internal client
	isOpen          bool                 // signal that the BulkWriter is closed
	flush           chan bool            // signal that the BulkWriter needs to flush the queue
	isFlushing      bool                 // determines that a flush has occurred or not
	doneFlushing    chan bool            // signals that the BulkWriter has flushed the queue
	queue           chan bulkWriterJob   // incoming write requests
	scheduled       chan bulkWriterBatch // scheduled requests
	backlogQueue    []bulkWriterJob      // backlog of requests to send
	maxOpsPerSecond int                  // Number of requests that can be sent per second.
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
	scheduled := make(chan bulkWriterBatch)
	bw := BulkWriter{
		ctx:             ctx,
		vc:              v,
		database:        database,
		isOpen:          true,
		flush:           f,
		isFlushing:      false,
		doneFlushing:    d,
		queue:           q,
		scheduled:       scheduled,
		start:           time.Now(),
		maxOpsPerSecond: DEFAULT_STARTING_MAXIMUM_OPS_PER_SECOND,
	}

	// Start the call receiving thread and request sending thread
	go bw.receiver()
	go bw.scheduler()

	// Should we have a retry-er?

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

// Create adds a document creation write to the queue of writes to send.
func (bw *BulkWriter) Create(doc *DocumentRef, datum interface{}) (*pb.WriteResult, error) {
	r := make(chan *pb.WriteResult, 1)
	e := make(chan error, 1)

	if !bw.isOpen {
		return nil, errors.New("firestore: BulkWriter has been closed")
	}

	if doc == nil {
		return nil, errors.New("firestore: nil document contents")
	}

	w, err := doc.newCreateWrites(datum)
	if err != nil {
		r <- nil
		e <- errors.New(fmt.Sprintf("firestore: cannot create %v with %v", doc.ID, datum))
	}

	j := bulkWriterJob{
		result: r,
		write:  w[0],
		err:    e,
	}

	bw.queue <- j
	// Note: This will increase the space complexity of this feature
	return <-r, <-e
}

// Create adds a document deletion write to the queue of writes to send.
func (bw *BulkWriter) Delete(doc *DocumentRef, preconds ...Precondition) (*pb.WriteResult, error) {
	r := make(chan *pb.WriteResult, 1)
	e := make(chan error, 1)

	if !bw.isOpen {
		return nil, errors.New("firestore: BulkWriter has been closed")
	}

	if doc == nil {
		return nil, errors.New("firestore: nil document contents")
	}

	w, err := doc.newDeleteWrites(preconds)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("firestore: cannot delete doc %v", doc.ID))
	}

	j := bulkWriterJob{
		result: r,
		write:  w[0],
		err:    e,
	}

	bw.queue <- j
	return <-r, <-e
}

// receiver gets write requests from the caller and sends BatchWriteRequests to the scheduler.
// The receiver method is the main "event loop" of the BulkWriter. It maintains
// the communication routes in between the caller and calls to the service.
func (bw *BulkWriter) receiver() {
	for {
		log.Println("receiver: receiving")
		var bs []bulkWriterBatch

		select {
		case bwj := <-bw.queue:
			bw.backlogQueue = append(bw.backlogQueue, bwj)
			if len(bw.backlogQueue) > MAX_BATCH_SIZE {
				bs = bw.buildRequests(false)
			} else if bw.isFlushing {
				bs = bw.buildRequests(true)
			}

		case <-bw.flush:
			log.Println("receiver: got call to flush")
			bw.isFlushing = true
			bs = bw.buildRequests(true)
		}

		for _, bwb := range bs {

			// TODO: Tag a collection of bwjs as a Flush job.

			// Send signal that flushed queue objects are being sent.
			// This will only happen if we have items to send while flushing.
			if bw.isFlushing {
				bw.isFlushing = false
				bw.doneFlushing <- true
			}
			log.Println("receiver: sending job to scheduler")
			bw.scheduled <- bwb
		}

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
	for b := range bw.scheduled { // bw.scheduled is a channel of bulkWriterBatch objects
		elapsed := (bw.start.UnixMilli() - time.Now().UnixMilli()) / 1000
		var qps int64
		// Don't divide by 0!
		if elapsed == 0 {
			qps = 0
		} else {
			qps = bw.reqs / elapsed
		}

		if qps < int64(bw.maxOpsPerSecond) {
			go bw.send(b.bwr, b.bwj)
		} else {
			time.Sleep(time.Duration(RATE_LIMITER_MULTIPLIER_MILLIS))
			go bw.send(b.bwr, b.bwj)

			// Increase number of requests per second at the five minute mark
			mins := elapsed / 60
			if mins%5 == 0 {
				newOps := float64(bw.maxOpsPerSecond) * RATE_LIMITER_MULTIPLIER
				bw.maxOpsPerSecond = int(newOps)
			}
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

		bwj[i].result <- res
		bwj[i].err <- nil
	}
}
