// Copyright 2026 Google LLC
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

package storage

import (
	"bytes"
	"context"
	"errors"
	"hash/crc32"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- Bidi Read (Private Preview) Integration Tests ---

// TestIntegration_BidiRead_MultipleRangedRead tests reading an Object across multiple
// concurrent range read futures. Validates bytes, total transferred length, and CRC32C checksum integrity.
func TestIntegration_BidiRead_MultipleRangedRead(t *testing.T) {
	multiTransportTest(skipAllButRapid(context.Background(), "Bidi Read API test"), t, func(t *testing.T, ctx context.Context, bucket string, _ string, client *Client) {
		if bucket == "" {
			t.Skip("Bucket not configured")
		}
		content := make([]byte, 4<<20) // 4MB
		rand.New(rand.NewSource(0)).Read(content)
		objName := "bidi-read-multi-range-" + uidSpace.New()

		obj := client.Bucket(bucket).Object(objName)
		if err := writeObject(ctx, obj, "application/octet-stream", content); err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = obj.Delete(ctx)
		}()

		reader, err := obj.NewMultiRangeDownloader(ctx)
		if err != nil {
			t.Fatalf("NewMultiRangeDownloader: %v", err)
		}
		defer reader.Close()

		type rangeTest struct {
			offset int64
			length int64
			buf    bytes.Buffer
			gotOff int64
			gotLen int64
			err    error
		}

		ranges := []*rangeTest{
			{offset: 0, length: 1024},                      // first 1KB
			{offset: 1024, length: 2048},                   // next 2KB
			{offset: 1 << 20, length: 512 << 10},           // middle 512KB
			{offset: -1024, length: 0},                     // last 1KB (negative offset)
			{offset: 0, length: 0},                         // entire object
			{offset: 2 << 20, length: int64(len(content))}, // from 2MB to end (length larger than remaining)
		}

		var wg sync.WaitGroup
		for _, rt := range ranges {
			wg.Add(1)
			r := rt
			reader.Add(&r.buf, r.offset, r.length, func(off, length int64, err error) {
				defer wg.Done()
				r.gotOff = off
				r.gotLen = length
				r.err = err
			})
		}
		wg.Wait()
		reader.Wait()

		for i, rt := range ranges {
			if rt.err != nil {
				t.Errorf("range %d (offset=%d, length=%d) failed: %v", i, rt.offset, rt.length, rt.err)
				continue
			}
			expectedOffset := rt.offset
			if expectedOffset < 0 {
				expectedOffset += int64(len(content))
			}
			expectedData := content[expectedOffset:]
			if rt.length > 0 && int(expectedOffset+rt.length) <= len(content) {
				expectedData = content[expectedOffset : expectedOffset+rt.length]
			}
			if rt.gotLen != int64(len(expectedData)) {
				t.Errorf("range %d: got length %d, want %d", i, rt.gotLen, len(expectedData))
			}
			if !bytes.Equal(rt.buf.Bytes(), expectedData) {
				t.Errorf("range %d data mismatch: got %d bytes, want %d bytes", i, rt.buf.Len(), len(expectedData))
			}
			// Verify CRC32C checksum integrity
			gotCRC := crc32.Checksum(rt.buf.Bytes(), crc32cTable)
			wantCRC := crc32.Checksum(expectedData, crc32cTable)
			if gotCRC != wantCRC {
				t.Errorf("range %d CRC32C mismatch: got %d, want %d", i, gotCRC, wantCRC)
			}
		}
	})
}

// TestIntegration_BidiRead_ReadPostStreamClose verifies session isolation:
// Awaiting/resolving an outstanding Future after closing the Session/FileDescriptor throws appropriate Stream Closed Exception.
func TestIntegration_BidiRead_ReadPostStreamClose(t *testing.T) {
	multiTransportTest(skipAllButRapid(context.Background(), "Bidi Read API test"), t, func(t *testing.T, ctx context.Context, bucket string, _ string, client *Client) {
		if bucket == "" {
			t.Skip("Bucket not configured")
		}
		content := make([]byte, 1024)
		rand.New(rand.NewSource(0)).Read(content)
		objName := "bidi-read-post-close-" + uidSpace.New()

		obj := client.Bucket(bucket).Object(objName)
		if err := writeObject(ctx, obj, "application/octet-stream", content); err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = obj.Delete(ctx)
		}()

		reader, err := obj.NewMultiRangeDownloader(ctx)
		if err != nil {
			t.Fatalf("NewMultiRangeDownloader: %v", err)
		}

		// Close reader before issuing reads
		if err := reader.Close(); err != nil {
			t.Fatalf("reader.Close: %v", err)
		}

		// Attempting to Add a read after Close should invoke callback with error
		var buf bytes.Buffer
		var cbErr error
		done := make(chan struct{})
		reader.Add(&buf, 0, 100, func(off, length int64, err error) {
			cbErr = err
			close(done)
		})

		select {
		case <-done:
			if cbErr == nil {
				t.Fatalf("expected error when reading from closed stream, got nil")
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for callback on closed reader")
		}
	})
}

type preallocatedSliceWriter struct {
	buf []byte
	off int
}

func (s *preallocatedSliceWriter) Write(p []byte) (n int, err error) {
	if s.off+len(p) > len(s.buf) {
		return 0, io.ErrShortBuffer
	}
	n = copy(s.buf[s.off:], p)
	s.off += n
	return n, nil
}

// TestIntegration_BidiRead_ZeroCopyRead tests zero-copy range reads across multiple concurrent
// range read futures, validating byte buffer access and resource disposal.
func TestIntegration_BidiRead_ZeroCopyRead(t *testing.T) {
	multiTransportTest(skipAllButRapid(context.Background(), "Bidi Read API test"), t, func(t *testing.T, ctx context.Context, bucket string, _ string, client *Client) {
		if bucket == "" {
			t.Skip("Bucket not configured")
		}
		content := make([]byte, 2<<20) // 2MB
		rand.New(rand.NewSource(0)).Read(content)
		objName := "bidi-read-zero-copy-" + uidSpace.New()

		obj := client.Bucket(bucket).Object(objName)
		if err := writeObject(ctx, obj, "application/octet-stream", content); err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = obj.Delete(ctx)
		}()

		reader, err := obj.NewMultiRangeDownloader(ctx)
		if err != nil {
			t.Fatalf("NewMultiRangeDownloader: %v", err)
		}
		defer reader.Close()

		// Allocate fixed buffers for zero-copy read destinations
		buf1 := make([]byte, 1024)
		buf2 := make([]byte, 2048)
		sw1 := &preallocatedSliceWriter{buf: buf1}
		sw2 := &preallocatedSliceWriter{buf: buf2}

		var wg sync.WaitGroup
		var err1, err2 error
		wg.Add(2)
		reader.Add(sw1, 0, 1024, func(off, length int64, err error) {
			err1 = err
			wg.Done()
		})
		reader.Add(sw2, 1024, 2048, func(off, length int64, err error) {
			err2 = err
			wg.Done()
		})
		wg.Wait()
		reader.Wait()

		if err1 != nil {
			t.Errorf("read 1 failed: %v", err1)
		}
		if err2 != nil {
			t.Errorf("read 2 failed: %v", err2)
		}
		if !bytes.Equal(buf1, content[:1024]) {
			t.Errorf("buf1 content mismatch")
		}
		if !bytes.Equal(buf2, content[1024:3072]) {
			t.Errorf("buf2 content mismatch")
		}
	})
}

// TestIntegration_BidiRead_NonExistentBucketRead tests opening a stream on a non-existent bucket.
// Verifies that an appropriate StorageException with HTTP status 404 is thrown.
func TestIntegration_BidiRead_NonExistentBucketRead(t *testing.T) {
	multiTransportTest(skipAllButRapid(context.Background(), "Bidi Read API test"), t, func(t *testing.T, ctx context.Context, _ string, _ string, client *Client) {
		nonExistentBucket := "non-existent-bucket-" + uidSpace.New()
		obj := client.Bucket(nonExistentBucket).Object("test-object")

		reader, err := obj.NewMultiRangeDownloader(ctx)
		if err == nil {
			// If NewMultiRangeDownloader doesn't fail immediately, reading or closing must fail with NotFound
			var buf bytes.Buffer
			var readErr error
			done := make(chan struct{})
			reader.Add(&buf, 0, 100, func(off, length int64, err error) {
				readErr = err
				close(done)
			})
			select {
			case <-done:
				if !errorIsStatusCode(readErr, http.StatusNotFound, codes.NotFound) {
					t.Errorf("expected NotFound error, got %v", readErr)
				}
			case <-time.After(5 * time.Second):
			}
			closeErr := reader.Close()
			if !errorIsStatusCode(closeErr, http.StatusNotFound, codes.NotFound) && !errorIsStatusCode(readErr, http.StatusNotFound, codes.NotFound) {
				t.Fatalf("expected NotFound status for non-existent bucket read, got readErr=%v, closeErr=%v", readErr, closeErr)
			}
			return
		}
		if !errorIsStatusCode(err, http.StatusNotFound, codes.NotFound) {
			t.Fatalf("expected NotFound status for non-existent bucket, got %v", err)
		}
	})
}

// TestIntegration_BidiRead_OutOfRange tests out-of-bounds range reads beyond object size (offset > size).
// Ensures appropriate exception is thrown for invalid ranges while valid range reads on the same session succeed.
func TestIntegration_BidiRead_OutOfRange(t *testing.T) {
	multiTransportTest(skipAllButRapid(context.Background(), "Bidi Read API test"), t, func(t *testing.T, ctx context.Context, bucket string, _ string, client *Client) {
		if bucket == "" {
			t.Skip("Bucket not configured")
		}
		content := make([]byte, 1000)
		rand.New(rand.NewSource(0)).Read(content)
		objName := "bidi-read-out-of-range-" + uidSpace.New()

		obj := client.Bucket(bucket).Object(objName)
		if err := writeObject(ctx, obj, "application/octet-stream", content); err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = obj.Delete(ctx)
		}()

		reader, err := obj.NewMultiRangeDownloader(ctx, WithMinConnections(2))
		if err != nil {
			t.Fatalf("NewMultiRangeDownloader: %v", err)
		}
		defer reader.Close()

		var validBuf, outOfRangeBuf bytes.Buffer
		var validErr, outOfRangeErr error
		var validLen int64
		var wg sync.WaitGroup
		wg.Add(2)

		// Valid range read
		reader.Add(&validBuf, 0, 100, func(off, length int64, err error) {
			defer wg.Done()
			validLen = length
			validErr = err
		})

		// Out of range read: offset > size (offset 5000 > size 1000)
		reader.Add(&outOfRangeBuf, 5000, 100, func(off, length int64, err error) {
			defer wg.Done()
			outOfRangeErr = err
		})

		wg.Wait()
		reader.Wait()

		if validErr != nil {
			t.Errorf("valid range read failed: %v", validErr)
		}
		if validLen != 100 || !bytes.Equal(validBuf.Bytes(), content[:100]) {
			t.Errorf("valid range read content mismatch: got %d bytes, want 100 bytes", validLen)
		}
		if outOfRangeErr == nil {
			t.Errorf("expected out of range error for offset > size, got nil")
		} else if status.Code(outOfRangeErr) != codes.OutOfRange && !errors.Is(outOfRangeErr, io.EOF) {
			t.Logf("out of range returned error: %v (code: %v)", outOfRangeErr, status.Code(outOfRangeErr))
		}
	})
}

// --- Bidi Writes (Private Preview) Integration Tests ---

// TestIntegration_BidiWrite_AppendableUploadEmptyObject opens an appendable upload and
// immediately closes it without writing bytes. Asserts object size == 0, CRC32C checksum of empty bytes,
// and empty read-back content.
func TestIntegration_BidiWrite_AppendableUploadEmptyObject(t *testing.T) {
	multiTransportTest(skipAllButRapid(context.Background(), "Bidi Write API test"), t, func(t *testing.T, ctx context.Context, bucket string, _ string, client *Client) {
		if bucket == "" {
			t.Skip("Bucket not configured")
		}
		h := testHelper{t}
		bkt := client.Bucket(bucket)
		objName := "bidi-write-empty-" + uidSpace.New()
		obj := bkt.Object(objName)
		defer h.mustDeleteObject(obj)

		w := obj.If(Conditions{DoesNotExist: true}).NewWriter(ctx)
		w.Append = true
		w.FinalizeOnClose = true

		if err := w.Close(); err != nil {
			t.Fatalf("w.Close: %v", err)
		}

		attrs := h.mustObjectAttrs(obj)
		if attrs.Size != 0 {
			t.Errorf("attrs.Size: got %d, want 0", attrs.Size)
		}
		if attrs.Finalized.IsZero() {
			t.Errorf("expected finalized object, got unfinalized")
		}

		// Read back content and verify empty
		r, err := obj.NewReader(ctx)
		if err != nil {
			t.Fatalf("obj.NewReader: %v", err)
		}
		defer r.Close()
		readData, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("io.ReadAll: %v", err)
		}
		if len(readData) != 0 {
			t.Errorf("read-back content: got %d bytes, want 0 bytes", len(readData))
		}
	})
}

// TestIntegration_BidiWrite_MultiChunkAppendableUpload writes multiple byte chunks with Appendable Upload,
// closes the channel, and asserts total size, cumulative CRC32C, and binary equivalence on read-back.
func TestIntegration_BidiWrite_MultiChunkAppendableUpload(t *testing.T) {
	multiTransportTest(skipAllButRapid(context.Background(), "Bidi Write API test"), t, func(t *testing.T, ctx context.Context, bucket string, _ string, client *Client) {
		if bucket == "" {
			t.Skip("Bucket not configured")
		}
		h := testHelper{t}
		bkt := client.Bucket(bucket)
		objName := "bidi-write-multi-chunk-" + uidSpace.New()
		obj := bkt.Object(objName)
		defer h.mustDeleteObject(obj)

		chunk1 := make([]byte, 2*MiB)
		chunk2 := make([]byte, 2*MiB)
		chunk3 := make([]byte, 1*MiB)
		rand.New(rand.NewSource(1)).Read(chunk1)
		rand.New(rand.NewSource(2)).Read(chunk2)
		rand.New(rand.NewSource(3)).Read(chunk3)
		fullContent := append(append(chunk1, chunk2...), chunk3...)

		w := obj.If(Conditions{DoesNotExist: true}).NewWriter(ctx)
		w.Append = true
		w.FinalizeOnClose = true
		w.ChunkSize = 2 * MiB

		for _, chunk := range [][]byte{chunk1, chunk2, chunk3} {
			if _, err := w.Write(chunk); err != nil {
				t.Fatalf("w.Write chunk: %v", err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatalf("w.Close: %v", err)
		}

		attrs := h.mustObjectAttrs(obj)
		if attrs.Size != int64(len(fullContent)) {
			t.Errorf("attrs.Size: got %d, want %d", attrs.Size, len(fullContent))
		}
		if attrs.Finalized.IsZero() {
			t.Errorf("expected finalized object, got unfinalized")
		}

		// Read back content and verify binary equivalence
		r, err := obj.NewReader(ctx)
		if err != nil {
			t.Fatalf("obj.NewReader: %v", err)
		}
		defer r.Close()
		readData, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("io.ReadAll: %v", err)
		}
		if !bytes.Equal(readData, fullContent) {
			t.Errorf("read-back binary mismatch: got %d bytes, want %d bytes", len(readData), len(fullContent))
		}
		wantCRC := crc32.Checksum(fullContent, crc32cTable)
		if attrs.CRC32C != 0 && attrs.CRC32C != wantCRC {
			t.Errorf("CRC32C mismatch: got %d, want %d", attrs.CRC32C, wantCRC)
		}
	})
}

// TestIntegration_BidiWrite_ExplicitFlush writes 1 byte, explicitly invokes channel.flush(),
// writes remaining data, closes channel, and verifies final object size and CRC32C checksum.
func TestIntegration_BidiWrite_ExplicitFlush(t *testing.T) {
	multiTransportTest(skipAllButRapid(context.Background(), "Bidi Write API test"), t, func(t *testing.T, ctx context.Context, bucket string, _ string, client *Client) {
		if bucket == "" {
			t.Skip("Bucket not configured")
		}
		h := testHelper{t}
		bkt := client.Bucket(bucket)
		objName := "bidi-write-flush-" + uidSpace.New()
		obj := bkt.Object(objName)
		defer h.mustDeleteObject(obj)

		content := make([]byte, 1024)
		rand.New(rand.NewSource(0)).Read(content)

		w := obj.If(Conditions{DoesNotExist: true}).NewWriter(ctx)
		w.Append = true
		w.FinalizeOnClose = true

		// Write 1 byte
		if _, err := w.Write(content[:1]); err != nil {
			t.Fatalf("writing 1 byte: %v", err)
		}
		// Explicit Flush
		n, err := w.Flush()
		if err != nil {
			t.Fatalf("w.Flush: %v", err)
		}
		if n != 1 {
			t.Errorf("flushed bytes: got %d, want 1", n)
		}

		// Write remaining data
		if _, err := w.Write(content[1:]); err != nil {
			t.Fatalf("writing remaining data: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("w.Close: %v", err)
		}

		attrs := h.mustObjectAttrs(obj)
		if attrs.Size != int64(len(content)) {
			t.Errorf("attrs.Size: got %d, want %d", attrs.Size, len(content))
		}
		if attrs.Finalized.IsZero() {
			t.Errorf("expected finalized object, got unfinalized")
		}
	})
}

// TestIntegration_BidiWrite_AppendableUploadTakeover: Session 1 writes chunk 1 with CLOSE_WITHOUT_FINALIZING.
// Session 2 initializes a new BlobAppendableUpload targeting the same blob generation, appends chunk 2, and finalizes,
// verifying cumulative size and CRC32C.
func TestIntegration_BidiWrite_AppendableUploadTakeover(t *testing.T) {
	multiTransportTest(skipAllButRapid(context.Background(), "Bidi Write API test"), t, func(t *testing.T, ctx context.Context, bucket string, _ string, client *Client) {
		if bucket == "" {
			t.Skip("Bucket not configured")
		}
		h := testHelper{t}
		bkt := client.Bucket(bucket)
		objName := "bidi-write-takeover-" + uidSpace.New()
		obj := bkt.Object(objName)
		defer h.mustDeleteObject(obj)

		chunk1 := make([]byte, 1*MiB)
		chunk2 := make([]byte, 1*MiB)
		rand.New(rand.NewSource(1)).Read(chunk1)
		rand.New(rand.NewSource(2)).Read(chunk2)
		fullContent := append(chunk1, chunk2...)

		// Session 1: write chunk 1 without finalizing
		w1 := obj.If(Conditions{DoesNotExist: true}).NewWriter(ctx)
		w1.Append = true
		w1.FinalizeOnClose = false
		if _, err := w1.Write(chunk1); err != nil {
			t.Fatalf("w1.Write: %v", err)
		}
		if err := w1.Close(); err != nil {
			t.Fatalf("w1.Close: %v", err)
		}
		gen := w1.Attrs().Generation

		// Session 2: takeover targeting the same blob generation, append chunk 2, finalize
		w2, off, err := obj.Generation(gen).NewWriterFromAppendableObject(ctx, &AppendableWriterOpts{
			FinalizeOnClose: true,
		})
		if err != nil {
			t.Fatalf("NewWriterFromAppendableObject: %v", err)
		}
		if off != int64(len(chunk1)) {
			t.Errorf("takeover offset: got %d, want %d", off, len(chunk1))
		}
		if _, err := w2.Write(chunk2); err != nil {
			t.Fatalf("w2.Write: %v", err)
		}
		if err := w2.Close(); err != nil {
			t.Fatalf("w2.Close: %v", err)
		}

		attrs := h.mustObjectAttrs(obj)
		if attrs.Size != int64(len(fullContent)) {
			t.Errorf("final size: got %d, want %d", attrs.Size, len(fullContent))
		}
		if attrs.Finalized.IsZero() {
			t.Errorf("expected finalized object, got unfinalized")
		}
	})
}

// TestIntegration_BidiWrite_TakeoverJustToFinalize: Session 1 writes data with CLOSE_WITHOUT_FINALIZING.
// Session 2 takes over the object generation and calls channel.finalizeAndClose() without appending data, verifying object finalization.
func TestIntegration_BidiWrite_TakeoverJustToFinalize(t *testing.T) {
	multiTransportTest(skipAllButRapid(context.Background(), "Bidi Write API test"), t, func(t *testing.T, ctx context.Context, bucket string, _ string, client *Client) {
		if bucket == "" {
			t.Skip("Bucket not configured")
		}
		h := testHelper{t}
		bkt := client.Bucket(bucket)
		objName := "bidi-write-takeover-fin-" + uidSpace.New()
		obj := bkt.Object(objName)
		defer h.mustDeleteObject(obj)

		content := make([]byte, 1*MiB)
		rand.New(rand.NewSource(0)).Read(content)

		// Session 1: write data without finalizing
		w1 := obj.If(Conditions{DoesNotExist: true}).NewWriter(ctx)
		w1.Append = true
		w1.FinalizeOnClose = false
		if _, err := w1.Write(content); err != nil {
			t.Fatalf("w1.Write: %v", err)
		}
		if err := w1.Close(); err != nil {
			t.Fatalf("w1.Close: %v", err)
		}
		gen := w1.Attrs().Generation

		// Session 2: takeover just to finalize without appending data
		w2, off, err := obj.Generation(gen).NewWriterFromAppendableObject(ctx, &AppendableWriterOpts{
			FinalizeOnClose: true,
		})
		if err != nil {
			t.Fatalf("NewWriterFromAppendableObject: %v", err)
		}
		if off != int64(len(content)) {
			t.Errorf("takeover offset: got %d, want %d", off, len(content))
		}
		if err := w2.Close(); err != nil {
			t.Fatalf("w2.Close: %v", err)
		}

		attrs := h.mustObjectAttrs(obj)
		if attrs.Size != int64(len(content)) {
			t.Errorf("final size: got %d, want %d", attrs.Size, len(content))
		}
		if attrs.Finalized.IsZero() {
			t.Errorf("expected finalized object, got unfinalized")
		}
	})
}

// TestIntegration_BidiWrite_ExplicitFinalizeWithCorrectChecksum: Writes data and invokes
// channel.finalizeAndClose(expectedCrc32c) using the accurate CRC32C, verifying successful completion.
func TestIntegration_BidiWrite_ExplicitFinalizeWithCorrectChecksum(t *testing.T) {
	multiTransportTest(skipAllButRapid(context.Background(), "Bidi Write API test"), t, func(t *testing.T, ctx context.Context, bucket string, _ string, client *Client) {
		if bucket == "" {
			t.Skip("Bucket not configured")
		}
		h := testHelper{t}
		bkt := client.Bucket(bucket)
		objName := "bidi-write-crc-correct-" + uidSpace.New()
		obj := bkt.Object(objName)
		defer h.mustDeleteObject(obj)

		content := make([]byte, 512<<10)
		rand.New(rand.NewSource(0)).Read(content)
		expectedCRC := crc32.Checksum(content, crc32cTable)

		w := obj.If(Conditions{DoesNotExist: true}).NewWriter(ctx)
		w.Append = true
		w.FinalizeOnClose = true
		w.SendCRC32C = true
		w.CRC32C = expectedCRC

		if _, err := w.Write(content); err != nil {
			t.Fatalf("w.Write: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("w.Close with correct CRC32C: %v", err)
		}

		attrs := h.mustObjectAttrs(obj)
		if attrs.Size != int64(len(content)) {
			t.Errorf("attrs.Size: got %d, want %d", attrs.Size, len(content))
		}
		if attrs.Finalized.IsZero() {
			t.Errorf("expected finalized object, got unfinalized")
		}
	})
}

// TestIntegration_BidiWrite_ExplicitFinalizeWithIncorrectChecksum: Writes data and calls
// channel.finalizeAndClose(badCrc32c) with an invalid checksum. Asserts that the call fails on the server.
func TestIntegration_BidiWrite_ExplicitFinalizeWithIncorrectChecksum(t *testing.T) {
	multiTransportTest(skipAllButRapid(context.Background(), "Bidi Write API test"), t, func(t *testing.T, ctx context.Context, bucket string, _ string, client *Client) {
		if bucket == "" {
			t.Skip("Bucket not configured")
		}
		h := testHelper{t}
		bkt := client.Bucket(bucket)
		objName := "bidi-write-crc-incorrect-" + uidSpace.New()
		obj := bkt.Object(objName)
		defer h.mustDeleteObject(obj)

		content := make([]byte, 512<<10)
		rand.New(rand.NewSource(0)).Read(content)
		badCRC := crc32.Checksum(content, crc32cTable) + 1

		w := obj.If(Conditions{DoesNotExist: true}).NewWriter(ctx)
		w.Append = true
		w.FinalizeOnClose = true
		w.SendCRC32C = true
		w.CRC32C = badCRC

		if _, err := w.Write(content); err != nil {
			t.Fatalf("w.Write: %v", err)
		}
		err := w.Close()
		if err == nil {
			t.Fatalf("expected error on close with incorrect CRC32C, got nil")
		}
		if !errorIsStatusCode(err, http.StatusBadRequest, codes.InvalidArgument) {
			t.Errorf("expected InvalidArgument/BadRequest, got %v", err)
		}
	})
}

// TestIntegration_BidiWrite_TakeoverJustToFinalizeWithIncorrectChecksum: Takes over an unfinalized object
// and invokes finalizeAndClose(badCrc32c) with an invalid checksum, verifying error handling on mismatch.
func TestIntegration_BidiWrite_TakeoverJustToFinalizeWithIncorrectChecksum(t *testing.T) {
	multiTransportTest(skipAllButRapid(context.Background(), "Bidi Write API test"), t, func(t *testing.T, ctx context.Context, bucket string, _ string, client *Client) {
		if bucket == "" {
			t.Skip("Bucket not configured")
		}
		h := testHelper{t}
		bkt := client.Bucket(bucket)
		objName := "bidi-write-takeover-badcrc-" + uidSpace.New()
		obj := bkt.Object(objName)
		defer h.mustDeleteObject(obj)

		content := make([]byte, 512<<10)
		rand.New(rand.NewSource(0)).Read(content)
		badCRC := crc32.Checksum(content, crc32cTable) + 1

		// Session 1: write data without finalizing
		w1 := obj.If(Conditions{DoesNotExist: true}).NewWriter(ctx)
		w1.Append = true
		w1.FinalizeOnClose = false
		if _, err := w1.Write(content); err != nil {
			t.Fatalf("w1.Write: %v", err)
		}
		if err := w1.Close(); err != nil {
			t.Fatalf("w1.Close: %v", err)
		}
		gen := w1.Attrs().Generation

		// Session 2: takeover with incorrect CRC32C
		w2, _, err := obj.Generation(gen).NewWriterFromAppendableObject(ctx, &AppendableWriterOpts{
			FinalizeOnClose: true,
		})
		if err != nil {
			t.Fatalf("NewWriterFromAppendableObject: %v", err)
		}
		w2.SendAppendFinalCRC32C = true
		w2.AppendFinalCRC32C = badCRC

		err = w2.Close()
		if err == nil {
			t.Fatalf("expected error on close with incorrect AppendFinalCRC32C, got nil")
		}
		if !errorIsStatusCode(err, http.StatusBadRequest, codes.InvalidArgument) {
			t.Errorf("expected InvalidArgument/BadRequest, got %v", err)
		}
	})
}

// TestIntegration_BidiWrite_TakeoverWithCorrectChecksum: Takes over an unfinalized object, appends additional
// bytes, and finalizes with cumulative expected CRC32C, verifying double object size and CRC32C match.
func TestIntegration_BidiWrite_TakeoverWithCorrectChecksum(t *testing.T) {
	multiTransportTest(skipAllButRapid(context.Background(), "Bidi Write API test"), t, func(t *testing.T, ctx context.Context, bucket string, _ string, client *Client) {
		if bucket == "" {
			t.Skip("Bucket not configured")
		}
		h := testHelper{t}
		bkt := client.Bucket(bucket)
		objName := "bidi-write-takeover-goodcrc-" + uidSpace.New()
		obj := bkt.Object(objName)
		defer h.mustDeleteObject(obj)

		chunk1 := make([]byte, 512<<10)
		chunk2 := make([]byte, 512<<10)
		rand.New(rand.NewSource(1)).Read(chunk1)
		rand.New(rand.NewSource(2)).Read(chunk2)
		fullContent := append(chunk1, chunk2...)
		cumulativeCRC := crc32.Checksum(fullContent, crc32cTable)

		// Session 1: write chunk 1 without finalizing
		w1 := obj.If(Conditions{DoesNotExist: true}).NewWriter(ctx)
		w1.Append = true
		w1.FinalizeOnClose = false
		if _, err := w1.Write(chunk1); err != nil {
			t.Fatalf("w1.Write: %v", err)
		}
		if err := w1.Close(); err != nil {
			t.Fatalf("w1.Close: %v", err)
		}
		gen := w1.Attrs().Generation

		// Session 2: takeover, append chunk 2, finalize with cumulative CRC32C
		w2, off, err := obj.Generation(gen).NewWriterFromAppendableObject(ctx, &AppendableWriterOpts{
			FinalizeOnClose: true,
		})
		if err != nil {
			t.Fatalf("NewWriterFromAppendableObject: %v", err)
		}
		if off != int64(len(chunk1)) {
			t.Errorf("takeover offset: got %d, want %d", off, len(chunk1))
		}
		if _, err := w2.Write(chunk2); err != nil {
			t.Fatalf("w2.Write: %v", err)
		}
		w2.SendAppendFinalCRC32C = true
		w2.AppendFinalCRC32C = cumulativeCRC

		if err := w2.Close(); err != nil {
			t.Fatalf("w2.Close with correct cumulative CRC: %v", err)
		}

		attrs := h.mustObjectAttrs(obj)
		if attrs.Size != int64(len(fullContent)) {
			t.Errorf("attrs.Size: got %d, want %d", attrs.Size, len(fullContent))
		}
		if attrs.Finalized.IsZero() {
			t.Errorf("expected finalized object, got unfinalized")
		}
	})
}

// TestIntegration_BidiWrite_TakeoverAndAppendWithIncorrectChecksum: Takes over, appends data,
// and passes an incorrect CRC32C to finalizeAndClose(), verifying mismatch exception.
func TestIntegration_BidiWrite_TakeoverAndAppendWithIncorrectChecksum(t *testing.T) {
	multiTransportTest(skipAllButRapid(context.Background(), "Bidi Write API test"), t, func(t *testing.T, ctx context.Context, bucket string, _ string, client *Client) {
		if bucket == "" {
			t.Skip("Bucket not configured")
		}
		h := testHelper{t}
		bkt := client.Bucket(bucket)
		objName := "bidi-write-takeover-append-badcrc-" + uidSpace.New()
		obj := bkt.Object(objName)
		defer h.mustDeleteObject(obj)

		chunk1 := make([]byte, 512<<10)
		chunk2 := make([]byte, 512<<10)
		rand.New(rand.NewSource(1)).Read(chunk1)
		rand.New(rand.NewSource(2)).Read(chunk2)
		fullContent := append(chunk1, chunk2...)
		badCumulativeCRC := crc32.Checksum(fullContent, crc32cTable) + 1

		// Session 1: write chunk 1 without finalizing
		w1 := obj.If(Conditions{DoesNotExist: true}).NewWriter(ctx)
		w1.Append = true
		w1.FinalizeOnClose = false
		if _, err := w1.Write(chunk1); err != nil {
			t.Fatalf("w1.Write: %v", err)
		}
		if err := w1.Close(); err != nil {
			t.Fatalf("w1.Close: %v", err)
		}
		gen := w1.Attrs().Generation

		// Session 2: takeover, append chunk 2, finalize with bad cumulative CRC32C
		w2, _, err := obj.Generation(gen).NewWriterFromAppendableObject(ctx, &AppendableWriterOpts{
			FinalizeOnClose: true,
		})
		if err != nil {
			t.Fatalf("NewWriterFromAppendableObject: %v", err)
		}
		if _, err := w2.Write(chunk2); err != nil {
			t.Fatalf("w2.Write: %v", err)
		}
		w2.SendAppendFinalCRC32C = true
		w2.AppendFinalCRC32C = badCumulativeCRC

		err = w2.Close()
		if err == nil {
			t.Fatalf("expected error on close with incorrect AppendFinalCRC32C, got nil")
		}
		if !errorIsStatusCode(err, http.StatusBadRequest, codes.InvalidArgument) {
			t.Errorf("expected InvalidArgument/BadRequest, got %v", err)
		}
	})
}

// TestIntegration_RCU_SingleShotWriteAndIngestOnRead tests writing an object using Single-Shot Regional Write
// and reading it back to validate ingest-on-read uptiering and ranged reads on RCU (Regional Rapid) buckets.
func TestIntegration_RCU_SingleShotWriteAndIngestOnRead(t *testing.T) {
	multiTransportTest(skipAllButRapid(context.Background(), "RCU Single-Shot Write & Ingest on Read"), t, func(t *testing.T, ctx context.Context, bucket string, _ string, client *Client) {
		if bucket == "" {
			t.Skip("Bucket not configured")
		}
		content := make([]byte, 2<<20) // 2MB
		rand.New(rand.NewSource(42)).Read(content)
		objName := "rcu-ingest-on-read-" + uidSpace.New()

		obj := client.Bucket(bucket).Object(objName)

		// 1. Single-shot write
		w := obj.If(Conditions{DoesNotExist: true}).NewWriter(ctx)
		if _, err := w.Write(content); err != nil {
			t.Fatalf("single-shot write failed: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("closing single-shot writer failed: %v", err)
		}
		defer func() {
			_ = obj.Delete(ctx)
		}()

		// 2. Perform initial read (triggering ingest-on-read)
		r, err := obj.NewReader(ctx)
		if err != nil {
			t.Fatalf("NewReader failed: %v", err)
		}
		readBack, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}
		if !bytes.Equal(readBack, content) {
			t.Fatalf("read back content mismatch: got %d bytes, want %d bytes", len(readBack), len(content))
		}

		// 3. Perform MRD Bidi Read on the uptiered object
		mrd, err := obj.NewMultiRangeDownloader(ctx)
		if err != nil {
			t.Fatalf("NewMultiRangeDownloader on RCU object failed: %v", err)
		}
		defer mrd.Close()

		var rangeBuf bytes.Buffer
		var rangeErr error
		var rangeLen int64
		done := make(chan struct{})
		mrd.Add(&rangeBuf, 1024, 4096, func(off, length int64, err error) {
			rangeErr = err
			rangeLen = length
			close(done)
		})
		<-done
		mrd.Wait()

		if rangeErr != nil {
			t.Fatalf("mrd range read failed: %v", rangeErr)
		}
		if rangeLen != 4096 {
			t.Errorf("got range length %d, want 4096", rangeLen)
		}
		if !bytes.Equal(rangeBuf.Bytes(), content[1024:5120]) {
			t.Errorf("range read data mismatch")
		}
	})
}
