package dataflux

import (
	"context"
	"sync"

	"cloud.google.com/go/storage"
)

// WorkerStatus indicates the status of a worker.
type WorkerStatus int

const (
	// Idle status shows that the worker is currently not listing.
	Idle WorkerStatus = iota
	// Active status shows that the worker is currently listing objects within assigned range.
	Active
)

type listerResult struct {
	mu      sync.Mutex
	objects []*storage.ObjectAttrs
}

type worker struct {
	goroutineID   int
	startRange    string
	endRange      string
	status        WorkerStatus
	rangesplitter *rangeSplitter
	idleChannel   chan int
	result        *listerResult
	generation    int64
}

// workstealListing is the main entry point of the worksteal algorithm.
// It performs worksteal to achieve highly dynamic object listing.
func workstealListing(ctx context.Context, opts Lister) ([]*storage.ObjectAttrs, error) {
	return nil, nil
}
