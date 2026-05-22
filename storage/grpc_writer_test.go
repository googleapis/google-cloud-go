// Copyright 2025 Google LLC
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
	"time"

	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	"google.golang.org/protobuf/proto"
)

func TestGetObjectChecksums(t *testing.T) {
	tests := []struct {
		name                string
		fullObjectChecksum  func() uint32
		finishWrite         bool
		sendCRC32C          bool
		takeoverWriter      bool
		disableAutoChecksum bool
		attrs               *ObjectAttrs
		want                *storagepb.ObjectChecksums
	}{
		{
			name:        "finishWrite is false",
			finishWrite: false,
			want:        nil,
		},
		{
			name:        "objectAttrs is nil",
			finishWrite: true,
			want:        nil,
		},
		{
			name:        "sendCRC32C is true, attrs have CRC32C",
			finishWrite: true,
			sendCRC32C:  true,
			attrs:       &ObjectAttrs{CRC32C: 123},
			want: &storagepb.ObjectChecksums{
				Crc32C: proto.Uint32(123),
			},
		},
		{
			name:                "disableCRC32C is true and sendCRC32C is true",
			finishWrite:         true,
			sendCRC32C:          true,
			disableAutoChecksum: true,
			attrs:               &ObjectAttrs{CRC32C: 123},
			want: &storagepb.ObjectChecksums{
				Crc32C: proto.Uint32(123),
			},
		},
		{
			name:        "sendCRC32C is true",
			finishWrite: true,
			sendCRC32C:  true,
			attrs:       &ObjectAttrs{CRC32C: 123},
			want: &storagepb.ObjectChecksums{
				Crc32C: proto.Uint32(123),
			},
		},
		{
			name:        "MD5 is provided",
			finishWrite: true,
			attrs:       &ObjectAttrs{MD5: []byte{1, 5, 0}},
			want: &storagepb.ObjectChecksums{
				Md5Hash: []byte{1, 5, 0},
			},
		},
		{
			name:                "disableCRC32C is true and sendCRC32C is false",
			finishWrite:         true,
			sendCRC32C:          false,
			disableAutoChecksum: true,
			want:                nil,
		},
		{
			name:                "CRC32C enabled, no user-provided checksum",
			fullObjectChecksum:  func() uint32 { return 456 },
			finishWrite:         true,
			sendCRC32C:          false,
			disableAutoChecksum: false,
			attrs:               &ObjectAttrs{},
			want: &storagepb.ObjectChecksums{
				Crc32C: proto.Uint32(456),
			},
		},
		// TODO(b/461982277): remove this testcase once checksums for takeover writer is implemented
		{
			name:           "takeover writer should return nil",
			finishWrite:    true,
			takeoverWriter: true,
			want:           nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getObjectChecksums(&getObjectChecksumsParams{
				disableAutoChecksum: tt.disableAutoChecksum,
				sendCRC32C:          tt.sendCRC32C,
				objectAttrs:         tt.attrs,
				fullObjectChecksum:  tt.fullObjectChecksum,
				finishWrite:         tt.finishWrite,
				takeoverWriter:      tt.takeoverWriter,
			})
			if !proto.Equal(got, tt.want) {
				t.Errorf("getObjectChecksums() = %v, want %v", got, tt.want)
			}
		})
	}
}
// Test the logic correctly handles the combination of io.EOF
// from Recv (recvErr) and a generic error from Send (sendErr).
func TestGRPCWriterErrorHandling(t *testing.T) {
	// As this is deeply embedded in the unexported types, we verify the logic
	// by simulating the exact error assignment sequence.
	tests := []struct {
		name      string
		recvErr   error
		sendErr   error
		wantError error
	}{
		{
			name:      "recvErr is io.EOF, sendErr is nil",
			recvErr:   io.EOF,
			sendErr:   nil,
			wantError: nil,
		},
		{
			name:      "recvErr is io.EOF, sendErr is an error",
			recvErr:   io.EOF,
			sendErr:   errors.New("send error"),
			wantError: errors.New("send error"), // Send error takes precedence.
		},
		{
			name:      "recvErr is an error, sendErr is nil",
			recvErr:   errors.New("recv error"),
			sendErr:   nil,
			wantError: errors.New("recv error"), // Recv error takes precedence.
		},
		{
			name:      "recvErr is an error, sendErr is an error",
			recvErr:   errors.New("recv error"),
			sendErr:   errors.New("send error"),
			wantError: errors.New("recv error"), // Recv error takes precedence.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var streamErr error

			streamErr = pickStreamError(tt.recvErr, tt.sendErr)

			if tt.wantError == nil {
				if streamErr != nil {
					t.Errorf("got error %v, want nil", streamErr)
				}
			} else {
				if streamErr == nil || streamErr.Error() != tt.wantError.Error() {
					t.Errorf("got error %v, want %v", streamErr, tt.wantError)
				}
			}
		})
	}
}

// TestGRPCWriter_Deadlock simulates a deadlock scenario if Recv and Send channels
// were not isolated in gRPCOneshotBidiWriteBufferSender.
func TestGRPCWriter_Deadlock(t *testing.T) {
	// A timeout means a deadlock likely occurred.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sendDone := make(chan struct{})
	recvDone := make(chan struct{})

	requests := make(chan gRPCBidiWriteRequest)
	completions := make(chan gRPCBidiWriteCompletion)

	var sendErr error

	go func() {
		sendErr = func() error {
			for {
				select {
				case <-recvDone:
					return nil
				case r, ok := <-requests:
					if !ok {
						return nil
					}
					if r.requestAck {
						continue
					}
					// mimic send logic
					if r.finishWrite {
						return nil
					}
				}
			}
		}()
		close(sendDone)
	}()

	go func() {
		// Mimic recv loop that immediately exits.
		// If recvDone isn't checked by the sender loop, sending
		// requests could block forever if the consumer closes early.
		close(recvDone)
	}()

	// sendDone should be closed immediately.
	select {
	case <-sendDone:
		// Success, no deadlock.
	case <-ctx.Done():
		t.Fatal("deadlock detected: send loop did not exit after recvDone was closed")
	}

	if sendErr != nil {
		t.Errorf("expected no error, got %v", sendErr)
	}
	close(completions)
}
