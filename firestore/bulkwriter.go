package firestore

import (
	"context"
	"errors"
	"time"

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

type bulkWriterOperation int16

const (
	CREATE bulkWriterOperation = iota
	UPDATE
	SET
	DELETE
)

type bulkWriterJob struct {
	err      chan error
	result   chan *pb.WriteResult
	write    *pb.Write
	attempts int
}

type CallersBulkWriter struct {
	database     string          // the database as resource name: projects/[PROJECT]/databases/[DATABASE]
	ctx          context.Context // context -- unneeded?
	reqs         int             // current number of requests open
	vc           *vkit.Client    // internal client
	isOpen       bool            // semaphore
	backlogQueue []bulkWriterJob // backlog of requests to send
}

// NewCallersBulkWriter creates a new instance of the CallersBulkWriter. This
// version of BulkWriter is intended to be used within go routines by the
// callers.
func NewCallersBulkWriter(ctx context.Context, database string) (*CallersBulkWriter, error) {
	v, err := vkit.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return &CallersBulkWriter{ctx: ctx, vc: v, database: database, isOpen: true}, nil
}

// Close sends all enqueued writes in parallel.
// After calling Close(), calling any additional method automatically returns
// with a nil error. This method completes when there are no more pending writes
// in the queue.
func (b *CallersBulkWriter) Close() {
	b.isOpen = false
	b.Flush()
}

// Flush commits all writes that have been enqueued up to this point in parallel.
func (b *CallersBulkWriter) Flush() {
	b.execute(true)
	for len(b.backlogQueue) > 0 {
		time.Sleep(time.Millisecond * 5) // TODO: Pick a number not out of thin air; exp back off?
		b.execute(true)
	}
}

// Do holds the place of all four required operations: create, update, set, delete.
// Only do one write per call to Do(), as you can only write to the same document 1x per batch.
// This method signature is a bad design--be sure to fix
func (bw *CallersBulkWriter) Do(dr *DocumentRef, op bulkWriterOperation, v interface{}) (*pb.WriteResult, error) {

	if !bw.isOpen {
		return nil, errors.New("firestore: BulkWriter has been closed")
	}

	if dr == nil {
		return nil, errors.New("firestore: nil document contents")
	}

	if op != DELETE && v == nil {
		return nil, errors.New("firestore: too few parameters passed in to BulkWriter operation")
	}

	var w []*pb.Write
	var err error
	r := make(chan *pb.WriteResult, 1)
	e := make(chan error, 1)

	// We can only do one write per document. The new*Writes methods return
	// an array of Write objects. FOR NOW, just take the first write.
	switch op {
	case CREATE:
		w, err = dr.newCreateWrites(v)
	}

	if err != nil {
		return nil, err
	}

	j := bulkWriterJob{
		result: r,
		write:  w[0],
		err:    e,
	}

	bw.backlogQueue = append(bw.backlogQueue, j)

	// NOTE: The space complexity of this pattern is linear. Is that okay?
	go bw.execute(false)

	return <-r, <-e
}

func (bw *CallersBulkWriter) makeBatch() []bulkWriterJob {

	qs := len(bw.backlogQueue)
	var b []bulkWriterJob

	if qs < MAX_BATCH_SIZE {

		// We're ready to send or flushing out the queue. Send all the remaining
		// requests to Firestore.
		b = bw.backlogQueue[:qs]
		bw.backlogQueue = []bulkWriterJob{}

	} else {
		// We have a full batch; send it.
		b = bw.backlogQueue[:MAX_BATCH_SIZE]
		bw.backlogQueue = bw.backlogQueue[MAX_BATCH_SIZE:]
	}
	return b
}

func (bw *CallersBulkWriter) execute(isFlushing bool) {

	// Guardrail -- Check whether too many reqs open right now
	if bw.reqs >= DEFAULT_STARTING_MAXIMUM_OPS_PER_SECOND {
		return
	}

	// Get the writes out of the jobs
	b := bw.makeBatch()
	var ws []*pb.Write
	for _, j := range b {
		if j.attempts < MAX_RETRY_ATTEMPTS {
			ws = append(ws, j.write)
		}
	}

	// Guardrail -- check whether no writes to apply
	if len(ws) == 0 {
		return
	}

	// Compose our request
	bwr := *&pb.BatchWriteRequest{
		Database: bw.database,
		Writes:   ws,
	}

	// Send it!
	bw.reqs++
	resp, err := bw.vc.BatchWrite(bw.ctx, &bwr)
	if err != nil {
		// Do we need to be selective about what kind of errors we send?
		for _, j := range b {
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
			bw.backlogQueue = append(bw.backlogQueue, b[i])
			continue
		}

		b[i].result <- res
		b[i].err <- nil
	}
}
