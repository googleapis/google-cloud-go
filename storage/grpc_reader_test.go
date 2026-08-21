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
	"context"
	"errors"
	"io"
	"testing"

	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	"google.golang.org/grpc/metadata"
)

// fakeReadObjectClient is a minimal implementation of
// storagepb.Storage_ReadObjectClient. Only RecvMsg is exercised by
// gRPCReadObjectReader; the rest are stubs.
type fakeReadObjectClient struct {
	recvMsgErrs []error // scripted errors returned per RecvMsg call
	recvMsgN    int     // count of RecvMsg invocations
	ctx         context.Context
}

func (f *fakeReadObjectClient) RecvMsg(_ any) error {
	f.recvMsgN++
	if len(f.recvMsgErrs) == 0 {
		return io.EOF
	}
	err := f.recvMsgErrs[0]
	f.recvMsgErrs = f.recvMsgErrs[1:]
	return err
}

func (f *fakeReadObjectClient) Recv() (*storagepb.ReadObjectResponse, error) {
	return nil, errors.New("fakeReadObjectClient.Recv not implemented")
}
func (f *fakeReadObjectClient) Header() (metadata.MD, error) { return nil, nil }
func (f *fakeReadObjectClient) Trailer() metadata.MD         { return nil }
func (f *fakeReadObjectClient) CloseSend() error             { return nil }
func (f *fakeReadObjectClient) Context() context.Context {
	if f.ctx == nil {
		return context.Background()
	}
	return f.ctx
}
func (f *fakeReadObjectClient) SendMsg(_ any) error { return nil }

// newReaderForTest constructs a gRPCReadObjectReader in the "all bytes already
// read" state, attached to the given fake stream. Using this state lets us
// drive the EOF early-return path (which is where the drain happens) without
// having to encode realistic ReadObjectResponse wire bytes.
func newReaderForTest(stream storagepb.Storage_ReadObjectClient, size int64, cancel context.CancelFunc) *gRPCReadObjectReader {
	return &gRPCReadObjectReader{
		stream:   stream,
		cancel:   cancel,
		size:     size,
		seen:     size, // already drained
		settings: &settings{}, // nil retry config falls back to default ShouldRetry
	}
}

// TestGRPCReader_ReadAtEOF_DrainsStream verifies that when Read is called
// after all bytes have been delivered, it issues exactly one final RecvMsg
// (consuming the server's EOS) before returning io.EOF, and that re-entry
// does not issue further wire calls. This is the regression guard for
// googleapis/google-cloud-go#14470.
func TestGRPCReader_ReadAtEOF_DrainsStream(t *testing.T) {
	fake := &fakeReadObjectClient{} // returns io.EOF for first RecvMsg
	r := newReaderForTest(fake, 10, func() {})

	// First Read at EOF: should drain exactly once and return io.EOF.
	n, err := r.Read(make([]byte, 4))
	if err != io.EOF {
		t.Fatalf("first Read err = %v; want io.EOF", err)
	}
	if n != 0 {
		t.Errorf("first Read n = %d; want 0", n)
	}
	if fake.recvMsgN != 1 {
		t.Errorf("RecvMsg invocations = %d; want 1 (one drain)", fake.recvMsgN)
	}
	if r.stream != nil {
		t.Errorf("r.stream not cleared after drain")
	}

	// Subsequent Read: must not touch the wire again.
	_, err = r.Read(make([]byte, 4))
	if err != io.EOF {
		t.Fatalf("second Read err = %v; want io.EOF", err)
	}
	if fake.recvMsgN != 1 {
		t.Errorf("RecvMsg invocations after re-entry = %d; want 1 (idempotent)", fake.recvMsgN)
	}
}

// TestGRPCReader_ReadAtEOF_ExhaustiveDrain verifies that Read loops until
// the stream is fully exhausted (receives EOF), even if multiple messages
// are required.
func TestGRPCReader_ReadAtEOF_ExhaustiveDrain(t *testing.T) {
	fake := &fakeReadObjectClient{
		// Two successes (e.g., metadata messages) followed by EOF.
		recvMsgErrs: []error{nil, nil},
	}
	r := newReaderForTest(fake, 10, func() {})

	_, err := r.Read(make([]byte, 4))
	if err != io.EOF {
		t.Fatalf("Read err = %v; want io.EOF", err)
	}
	// Total calls = 2 (for the nils) + 1 (for the final io.EOF) = 3
	if fake.recvMsgN != 3 {
		t.Errorf("RecvMsg invocations = %d; want 3 (exhaustive drain)", fake.recvMsgN)
	}
	if r.stream != nil {
		t.Errorf("r.stream not cleared after exhaustive drain")
	}
}

// TestGRPCReader_ReadAtEOF_DrainSwallowsNonEOF pins the deliberate behavior
// choice: trailer errors observed during the EOS drain are dropped, matching
// the prior contract that Read users do not see post-success trailer errors.
// If we ever decide to surface them (mirroring WriteTo), this test should
// flip and document the change.
func TestGRPCReader_ReadAtEOF_DrainSwallowsNonEOF(t *testing.T) {
	fake := &fakeReadObjectClient{
		recvMsgErrs: []error{errors.New("post-success trailer error")},
	}
	r := newReaderForTest(fake, 10, func() {})

	n, err := r.Read(make([]byte, 4))
	if err != io.EOF {
		t.Fatalf("Read err = %v; want io.EOF (non-EOF drain error must be dropped)", err)
	}
	if n != 0 {
		t.Errorf("Read n = %d; want 0", n)
	}
}

// TestGRPCReader_ReadZeroRange_NoDrain verifies that a zero-range reader
// (constructor closes the stream eagerly) takes the early-return path
// without attempting a drain.
func TestGRPCReader_ReadZeroRange_NoDrain(t *testing.T) {
	fake := &fakeReadObjectClient{}
	r := &gRPCReadObjectReader{
		zeroRange: true,
		stream:    nil, // zero-range closes the stream in the constructor
		settings:  &settings{},
	}

	_, err := r.Read(make([]byte, 4))
	if err != io.EOF {
		t.Fatalf("Read err = %v; want io.EOF", err)
	}
	if fake.recvMsgN != 0 {
		t.Errorf("RecvMsg invocations = %d; want 0 (no drain on zero-range)", fake.recvMsgN)
	}
}

// TestGRPCReader_PartialReadClose_RunsCancel verifies that the intentional-
// abort path is preserved: Close on a reader that has *not* fully drained
// the object still invokes cancel(), so otelgrpc records the span as
// Cancelled (which is the correct outcome for a real abort).
func TestGRPCReader_PartialReadClose_RunsCancel(t *testing.T) {
	cancelled := false
	cancel := func() { cancelled = true }
	r := &gRPCReadObjectReader{
		stream:   &fakeReadObjectClient{},
		cancel:   cancel,
		size:     100,
		seen:     42, // partial
		settings: &settings{},
	}

	if err := r.Close(); err != nil {
		t.Fatalf("Close err = %v; want nil", err)
	}
	if !cancelled {
		t.Errorf("cancel not invoked on partial-read Close")
	}
	if r.stream != nil {
		t.Errorf("r.stream not cleared by Close")
	}
}

// TestGRPCReader_FullReadClose_CancelStillRuns documents that Close still
// calls cancel() even after Read has drained the EOS. This is harmless
// (the gRPC stream is already finalized) and intentionally unchanged so
// that callers who rely on cancel propagating to their own ctx-derived
// resources are not affected.
func TestGRPCReader_FullReadClose_CancelStillRuns(t *testing.T) {
	cancelled := false
	cancel := func() { cancelled = true }
	r := newReaderForTest(&fakeReadObjectClient{}, 10, cancel)

	if _, err := r.Read(make([]byte, 4)); err != io.EOF {
		t.Fatalf("Read err = %v; want io.EOF", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close err = %v; want nil", err)
	}
	if !cancelled {
		t.Errorf("cancel not invoked by Close after full read")
	}
}
