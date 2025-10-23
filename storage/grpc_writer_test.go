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
	"testing"

	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	"google.golang.org/protobuf/proto"
)

func TestGetObjectChecksums(t *testing.T) {
	tests := []struct {
		name               string
		fullObjectChecksum func() uint32
		finishWrite        bool
		sendCRC32C         bool
		disableCRC32C      bool
		attrs              *ObjectAttrs
		want               *storagepb.ObjectChecksums
	}{
		{
			name:        "finishWrite is false",
			finishWrite: false,
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
			name:          "disableCRC32C is true and sendCRC32C is true",
			finishWrite:   true,
			sendCRC32C:    true,
			disableCRC32C: true,
			attrs:         &ObjectAttrs{CRC32C: 123},
			want: &storagepb.ObjectChecksums{
				Crc32C: proto.Uint32(123),
			},
		},
		{
			name:          "disableCRC32C is true and sendCRC32C is false",
			finishWrite:   true,
			sendCRC32C:    false,
			disableCRC32C: true,
			want:          nil,
		},
		{
			name:               "CRC32C enabled, no user-provided checksum",
			fullObjectChecksum: func() uint32 { return 456 },
			finishWrite:        true,
			sendCRC32C:         false,
			disableCRC32C:      false,
			attrs:              &ObjectAttrs{},
			want: &storagepb.ObjectChecksums{
				Crc32C: proto.Uint32(456),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getObjectChecksums(tt.fullObjectChecksum, tt.finishWrite, tt.sendCRC32C, tt.disableCRC32C, tt.attrs)
			if !proto.Equal(got, tt.want) {
				t.Errorf("getObjectChecksums() = %v, want %v", got, tt.want)
			}
		})
	}
}
