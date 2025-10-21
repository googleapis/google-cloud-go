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
	"hash/crc32"
	"testing"

	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	"google.golang.org/protobuf/proto"
)

func Test_UpdateAndGetChecksums(t *testing.T) {
	oldBytes := []byte("old ")
	oldChecksum := crc32.Checksum(oldBytes, crc32cTable)
	checksumOfTest := crc32.Checksum([]byte("test"), crc32cTable)
	testCases := []struct {
		name              string
		buf               []byte
		oldChecksum       uint32
		finishWrite       bool
		sendCRC32C        bool
		disableCRC32C     bool
		attrs             *ObjectAttrs
		wantChecksums     *storagepb.ObjectChecksums
		wantChecksum      uint32
		wantChecksumofBuf *uint32
	}{
		{
			name:              "finishWrite is false",
			buf:               []byte("test"),
			oldChecksum:       0,
			finishWrite:       false,
			sendCRC32C:        true,
			attrs:             &ObjectAttrs{},
			wantChecksum:      checksumOfTest,
			wantChecksumofBuf: proto.Uint32(checksumOfTest),
		},
		{
			name:        "finishWrite is true, sendCRC32C is true",
			buf:         []byte("test"),
			oldChecksum: 0,
			finishWrite: true,
			sendCRC32C:  true,
			attrs:       &ObjectAttrs{CRC32C: 12345},
			wantChecksums: &storagepb.ObjectChecksums{
				Crc32C: proto.Uint32(12345),
			},
			wantChecksum:      checksumOfTest,
			wantChecksumofBuf: proto.Uint32(checksumOfTest),
		},
		{
			name:        "finishWrite is true, sendCRC32C is false",
			buf:         []byte("test"),
			oldChecksum: 0,
			finishWrite: true,
			sendCRC32C:  false,
			attrs:       &ObjectAttrs{},
			wantChecksums: &storagepb.ObjectChecksums{
				Crc32C: proto.Uint32(checksumOfTest),
			},
			wantChecksum:      crc32.Checksum([]byte("test"), crc32cTable),
			wantChecksumofBuf: proto.Uint32(checksumOfTest),
		},
		{
			name:        "finishWrite is true, sendCRC32C is false with old checksum",
			buf:         []byte("test"),
			oldChecksum: oldChecksum,
			finishWrite: true,
			sendCRC32C:  false,
			attrs:       &ObjectAttrs{},
			wantChecksums: &storagepb.ObjectChecksums{
				Crc32C: proto.Uint32(crc32.Update(oldChecksum, crc32cTable, []byte("test"))),
			},
			wantChecksum:      crc32.Checksum([]byte("old test"), crc32cTable),
			wantChecksumofBuf: proto.Uint32(checksumOfTest),
		},
		{
			name:              "do not calculate CRC32C when disableCRC32C is true",
			buf:               []byte("test"),
			oldChecksum:       0,
			disableCRC32C:     true,
			attrs:             &ObjectAttrs{},
			wantChecksums:     nil,
			wantChecksum:      0,
			wantChecksumofBuf: nil,
		},
		{
			name:          "send crc32c if disableCRC32C is true and user provides crc32 on finishWrite",
			buf:           []byte("test"),
			oldChecksum:   0,
			disableCRC32C: true,
			sendCRC32C:    true,
			finishWrite:   true,
			attrs:         &ObjectAttrs{CRC32C: 12345},
			wantChecksums: &storagepb.ObjectChecksums{
				Crc32C: proto.Uint32(12345),
			},
			wantChecksum:      0,
			wantChecksumofBuf: nil,
		},
		{
			name:              "send nil if disableCRC32C is true and user provides crc32 but not finishWrite",
			buf:               []byte("test"),
			oldChecksum:       0,
			disableCRC32C:     true,
			sendCRC32C:        true,
			attrs:             &ObjectAttrs{CRC32C: 12345},
			wantChecksums:     nil,
			wantChecksum:      0,
			wantChecksumofBuf: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotChecksums, gotChecksumofBuf, gotChecksum := updateAndGetChecksums(tc.buf, tc.oldChecksum, tc.finishWrite, tc.sendCRC32C, tc.disableCRC32C, tc.attrs)

			if gotChecksum != tc.wantChecksum {
				t.Errorf("got checksum %d, want %d", gotChecksum, tc.wantChecksum)
			}
			if !proto.Equal(gotChecksums, tc.wantChecksums) {
				t.Errorf("got checksums %v, want %v", gotChecksums, tc.wantChecksums)
			}
			if (tc.wantChecksumofBuf == nil) != (gotChecksumofBuf == nil) {
				t.Errorf("got checksumofBuf %v, want %v", gotChecksumofBuf, tc.wantChecksumofBuf)
				return
			}
			if tc.wantChecksumofBuf == nil {
				return
			}
			if *gotChecksumofBuf != *tc.wantChecksumofBuf {
				t.Errorf("got *checksumofBuf %d, want %d", gotChecksumofBuf, tc.wantChecksumofBuf)
			}

		})
	}
}
