package firestore

import (
	"context"
	"errors"
	"fmt"

	vkit "cloud.google.com/go/firestore/apiv1"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
)

const (
	MAX_BATCH_SIZE                          = 20
	RETRY_MAX_BATCH_SIZE                    = 10
	MAX_RETRY_ATTEMPTS                      = 10
	DEFAULT_STARTING_MAXIMUM_OPS_PER_SECOND = 500
	RATE_LIMITER_MULTIPLIER                 = 1.5
	RATE_LIMITER_MULTIPLIER_MILLIS          = 5 * 60 * 1000
)

type bulkWriterJob struct {
	err      chan error
	result   chan *pb.WriteResult
	write    *pb.Write
	attempts int
}

type bulkWriterBatch struct {
	bwr *pb.BatchWriteRequest // Request to send to the service
	bwj []bulkWriterJob       // Corresponding jobs to return response
}

type BulkWriter struct {
	database     string             // the database as resource name: projects/[PROJECT]/databases/[DATABASE]
	ctx          context.Context    // context -- unneeded?
	reqs         int                // current number of requests open
	vc           *vkit.Client       // internal client
	isOpen       bool               // signal that the BulkWriter is closed
	isFlushing   chan bool          // signal that the BulkWriter needs to flush the queue
	queue        chan bulkWriterJob // incoming write requests
	backlogQueue []bulkWriterJob    // backlog of requests to send
}

// NewCallersBulkWriter creates a new instance of the CallersBulkWriter. This
// version of BulkWriter is intended to be used within go routines by the
// callers.
func NewBulkWriter(ctx context.Context, database string) (*BulkWriter, error) {
	v, err := vkit.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	isFlushing := make(chan bool)
	queue := make(chan bulkWriterJob)
	bw := BulkWriter{
		ctx:        ctx,
		vc:         v,
		database:   database,
		isOpen:     true,
		isFlushing: isFlushing,
		queue:      queue,
	}

	go bw.executor()
	return &bw, nil
}

// Close sends all enqueued writes in parallel.
// CANNOT BE DEFERRED. Deferring a call to Close() can cause a deadlock.
// After calling Close(), calling any additional method automatically returns
// with a nil error. This method completes when there are no more pending writes
// in the queue.
func (b *BulkWriter) Close() {
	// Make sure that flushing actually happens before isOpen changes values.
	b.Flush()
	b.isOpen = false
}

// Flush commits all writes that have been enqueued up to this point in parallel.
// This method blocks execution.
func (b *BulkWriter) Flush() {
	b.isFlushing <- true
}

// Create adds a document creation write to the queue of writes to send.
func (bw *BulkWriter) Create(doc *DocumentRef, datum interface{}) (*pb.WriteResult, error) {
	if !bw.isOpen {
		return nil, errors.New("firestore: BulkWriter has been closed")
	}

	if doc == nil {
		return nil, errors.New("firestore: nil document contents")
	}

	w, err := doc.newCreateWrites(datum)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("firestore: cannot create %v with %v", doc.ID, datum))
	}

	r := make(chan *pb.WriteResult, 1)
	e := make(chan error, 1)

	j := bulkWriterJob{
		result: r,
		write:  w[0],
		err:    e,
	}

	bw.queue <- j
	return <-r, <-e
}

// Create adds a document deletion write to the queue of writes to send.
func (bw *BulkWriter) Delete(doc *DocumentRef, preconds ...Precondition) (*pb.WriteResult, error) {
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

	r := make(chan *pb.WriteResult, 1)
	e := make(chan error, 1)

	j := bulkWriterJob{
		result: r,
		write:  w[0],
		err:    e,
	}

	bw.queue <- j
	return <-r, <-e
}

// executor stores up BulkWriter jobs to send and starts execution threads.
// The executor method is the main "event loop" of the BulkWriter. It maintains
// the communication routes in between the caller and calls to the service.
func (bw *BulkWriter) executor() {
	for bwj := range bw.queue {
		// Store the new bulkWriterJob in the BW queue
		bw.backlogQueue = append(bw.backlogQueue, bwj)

		// Determine whether we need to send a batch or flush the queue
		select {
		case <-bw.isFlushing:
			go bw.execute(true)
			bw.isFlushing <- false
		default:
			if len(bw.backlogQueue) > MAX_BATCH_SIZE {
				go bw.execute(false)
			}
		}

		if bw.isOpen == false {
			break
		}

		// TODO: Check for context.Done()
	}

	// Send final call to flush the queue
	go bw.execute(true)
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

// execute creates batches of writes to send, observes timing, and sends writes.
func (bw *BulkWriter) execute(isFlushing bool) {
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

	for _, bwb := range bs {
		// TODO: Timing loop, checking number of requests in flight
		// IDEA: Create a schedule() function that handles timing
		//    + go routine with channel of bulkWriterBatch objects as param
		//    + listens to channel, sends requests when channel is populated
		bw.send(bwb.bwr, bwb.bwj)
	}
}

/*


func (bw *BulkWriter) schedule() {

	// NEED TO KEEP TRACK OF TIME ELAPSED AND QPS
	timeElapsed := bw.startTime - time.Now()
	qps := bw.totalRequests / timeElapsed


	waitTime := RATE_LIMITER_MULTIPLIER_MILLIS
	for b := range bw.scheduled { // bw.scheduled is a channel of bulkWriterBatch objects
		if qps < DEFAULT_STARTING_MAXIMUM_OPS_PER_SECOND {
			bw.send(b.bwr, b.bwj)
		} else {
			time.Sleep(time.Duration(waitTime))
			bw.send(b.bwr, b.bwj)

			// Increase wait time?

			// Increase number of requests per second at the five minute mark
		}
	}
}

*/

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

	bw.reqs--

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
