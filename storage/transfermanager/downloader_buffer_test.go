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
	"bytes"
	crand "crypto/rand"
	"io"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestDownloadBuffer(t *testing.T) {
	t.Parallel()

	// Create without an underlying buffer.
	b := &DownloadBuffer{}

	// Start at an offset.
	firstWrite := []byte("the best of times")
	var firstWriteOff int64 = 7

	n, err := b.WriteAt(firstWrite, firstWriteOff)
	if err != nil {
		t.Fatalf("DonwloadBuffer.WriteAt(%d): %v", firstWriteOff, err)
	}
	if exp := len(firstWrite); exp != n {
		t.Fatalf("expected to write %d, got %d", exp, n)
	}

	if want, got := firstWrite, b.Bytes()[firstWriteOff:]; cmp.Diff(got, want) != "" {
		t.Errorf("got: %q\nwant: %q", b.Bytes(), want)
	}

	// Write the beginning.
	secondWrite := []byte("It was")

	n, err = b.WriteAt(secondWrite, 0)
	if err != nil {
		t.Fatalf("DonwloadBuffer.WriteAt(0): %v", err)
	}
	if exp := len(secondWrite); exp != n {
		t.Fatalf("expected to write %d, got %d", exp, n)
	}

	if want, got := secondWrite, b.Bytes()[:len(secondWrite)]; cmp.Diff(got, want) != "" {
		t.Errorf("got: %q\nwant: %q", b.Bytes(), want)
	}

	// The ending should have stayed the same.
	if want, got := firstWrite, b.Bytes()[firstWriteOff:]; cmp.Diff(got, want) != "" {
		t.Errorf("got: %q\nwant: %q", b.Bytes(), want)
	}

	// Test write in the middle.
	n, err = b.WriteAt([]byte{' '}, int64(len(secondWrite)))
	if err != nil {
		t.Fatalf("DonwloadBuffer.WriteAt(%d): %v", len(secondWrite), err)
	}
	if exp := 1; exp != n {
		t.Fatalf("expected to write %d, got %d", exp, n)
	}

	// Check the full string.
	if want, got := []byte("It was the best of times"), b.Bytes(); cmp.Diff(got, want) != "" {
		t.Errorf("got: %q\nwant: %q", b.Bytes(), want)
	}

	// Test given underlying buffer.
	b = NewDownloadBuffer(b.Bytes())
	if want, got := []byte("It was the best of times"), b.Bytes(); cmp.Diff(got, want) != "" {
		t.Errorf("got: %q\nwant: %q", b.Bytes(), want)
	}

	// Test overwrite.
	overwrite := []byte("worst of times")
	var off int64 = 11

	n, err = b.WriteAt(overwrite, off)
	if err != nil {
		t.Fatalf("DonwloadBuffer.WriteAt(%d): %v", off, err)
	}
	if exp := len(overwrite); exp != n {
		t.Fatalf("expected to write %d, got %d", exp, n)
	}

	if want, got := []byte("It was the worst of times"), b.Bytes(); cmp.Diff(got, want) != "" {
		t.Errorf("got: %q\nwant: %q", b.Bytes(), want)
	}
}

func TestDownloadBufferParallel(t *testing.T) {
	t.Parallel()

	// Set up buffer.
	size := 50
	buf := &bytes.Buffer{}
	if _, err := io.CopyN(buf, crand.Reader, int64(size)); err != nil {
		t.Fatalf("io.CopyN: %v", err)
	}
	want := buf.Bytes()

	b := NewDownloadBuffer(make([]byte, size))

	// Write using 10 parallel goroutines.
	wg := sync.WaitGroup{}
	step := size / 10

	for i := 0; i < size; i = i + step {
		// Schedule a routine.
		i := i
		wg.Add(1)
		go func() {
			time.Sleep(time.Microsecond * time.Duration(rand.Intn(500)))
			n, err := b.WriteAt(want[i:i+step], int64(i))
			if err != nil {
				t.Errorf("b.WriteAt: %v", err)
			}
			if n != 5 {
				t.Errorf("expected to write 5 bytes, got %d", n)
			}
			wg.Done()
		}()
	}

	wg.Wait()

	if diff := cmp.Diff(b.Bytes(), want); diff != "" {
		t.Errorf("diff got(-) vs. want(+): %v", diff)
	}
}
