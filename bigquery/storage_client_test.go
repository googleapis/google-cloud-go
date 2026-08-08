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

package bigquery

import (
	"context"
	"testing"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	gax "github.com/googleapis/gax-go/v2"
)

func TestReadSessionStart_ArrowCompression(t *testing.T) {
	ctx := context.Background()
	testCases := []struct {
		desc      string
		codec     ArrowCompressionCodec
		wantCodec storagepb.ArrowSerializationOptions_CompressionCodec
	}{
		{
			desc:      "unspecified",
			codec:     ArrowCompressionUnspecified,
			wantCodec: storagepb.ArrowSerializationOptions_COMPRESSION_UNSPECIFIED,
		},
		{
			desc:      "lz4",
			codec:     ArrowCompressionLZ4Frame,
			wantCodec: storagepb.ArrowSerializationOptions_LZ4_FRAME,
		},
		{
			desc:      "zstd",
			codec:     ArrowCompressionZSTD,
			wantCodec: storagepb.ArrowSerializationOptions_ZSTD,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			settings := defaultReadClientSettings()
			settings.arrowCompressionCodec = tc.codec

			var capturedReq *storagepb.CreateReadSessionRequest
			rs := &readSession{
				ctx:      ctx,
				settings: settings,
				createReadSessionFunc: func(ctx context.Context, req *storagepb.CreateReadSessionRequest, opts ...gax.CallOption) (*storagepb.ReadSession, error) {
					capturedReq = req
					return &storagepb.ReadSession{}, nil
				},
			}

			if err := rs.start(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if capturedReq == nil {
				t.Fatal("request was not captured")
			}

			if tc.wantCodec == storagepb.ArrowSerializationOptions_COMPRESSION_UNSPECIFIED {
				if capturedReq.ReadSession.ReadOptions != nil && capturedReq.ReadSession.ReadOptions.GetArrowSerializationOptions() != nil {
					t.Errorf("expected no arrow serialization options for unspecified codec, got %v", capturedReq.ReadSession.ReadOptions.GetArrowSerializationOptions())
				}
			} else {
				opts := capturedReq.ReadSession.ReadOptions.GetArrowSerializationOptions()
				if opts == nil {
					t.Fatal("expected arrow serialization options, got nil")
				}
				if opts.BufferCompression != tc.wantCodec {
					t.Errorf("expected codec %v, got %v", tc.wantCodec, opts.BufferCompression)
				}
			}
		})
	}
}
