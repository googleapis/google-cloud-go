//go:build !linux

package udsthin

import (
	"context"
	"errors"

	"cloud.google.com/go/storage"
)

// ErrShmUnsupported is returned on platforms without the memfd/SCM_RIGHTS
// shared-memory data plane (the POC implements it for Linux only).
var ErrShmUnsupported = errors.New("udsthin: shared-memory data plane is Linux-only")

// ShmRegion is a stub on non-Linux platforms.
type ShmRegion struct {
	ID  uint64
	Mem []byte
}

// ShmReader is a stub on non-Linux platforms.
type ShmReader struct {
	Attrs storage.ReaderObjectAttrs
}

// OpenShm always fails on non-Linux platforms.
func (c *Client) OpenShm() (*ShmRegion, error) { return nil, ErrShmUnsupported }

// Close is a no-op on non-Linux platforms.
func (r *ShmRegion) Close() {}

// NewShmReader always fails on non-Linux platforms.
func (o *ObjectHandle) NewShmReader(ctx context.Context, region *ShmRegion, base, capacity uint64) (*ShmReader, error) {
	return nil, ErrShmUnsupported
}

// Next is unreachable on non-Linux platforms (NewShmReader never succeeds).
func (r *ShmReader) Next() ([]byte, error) { return nil, ErrShmUnsupported }

// Read is unreachable on non-Linux platforms.
func (r *ShmReader) Read(p []byte) (int, error) { return 0, ErrShmUnsupported }

// Close is a no-op on non-Linux platforms.
func (r *ShmReader) Close() error { return nil }
