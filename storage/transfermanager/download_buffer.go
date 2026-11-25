// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package transfermanager

import (
	"errors"
	"sync"
)

// NewDownloadBuffer initializes a DownloadBuffer using buf as the underlying
// buffer. Preferred way to create a DownloadBuffer as it does not need to grow
// the buffer if len(buf) is larger than or equal to the object length or range
// being downloaded to.
func NewDownloadBuffer(buf []byte) *DownloadBuffer {
	return &DownloadBuffer{bytes: buf}
}

// DownloadBuffer satisfies the io.WriterAt interface, allowing you to use it as
// a buffer to download to when using [Downloader]. DownloadBuffer is thread-safe
// as long as the ranges being written to do not overlap.
type DownloadBuffer struct {
	bytes []byte
	mu    sync.Mutex
}

// WriteAt writes len(p) bytes from p to the underlying buffer at offset off,
// growing the buffer if needed. It returns the number of bytes written from p
// and any error encountered that caused the write to stop early.
// WriteAt is thread-safe as long as the ranges being written to do not overlap.
// The supplied slice p is not retained.
func (db *DownloadBuffer) WriteAt(p []byte, off int64) (n int, err error) {
	if off < 0 {
		return 0, errors.New("transfermanager: negative offset")
	}
	requiredLength := int64(len(p)) + off

	db.mu.Lock()
	defer db.mu.Unlock()

	// Grow the buffer while holding db.mu so concurrent writes cannot race on
	// capacity or overwrite a larger buffer with a smaller allocation.
	if int64(len(db.bytes)) < requiredLength {
		if int64(cap(db.bytes)) < requiredLength {
			expandedBuff := make([]byte, requiredLength)
			copy(expandedBuff, db.bytes)
			db.bytes = expandedBuff
		} else {
			db.bytes = db.bytes[:requiredLength]
		}
	}

	copy(db.bytes[off:], p)

	return len(p), nil
}

// Bytes returns the slice of bytes written to DownloadBuffer. The slice aliases
// the buffer content at least until the next buffer modification, so
// immediate changes to the slice will affect the result of future reads.
func (db *DownloadBuffer) Bytes() []byte {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.bytes
}
