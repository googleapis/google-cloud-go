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

package storage

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"hash/crc32"
	"math/rand"
	"testing"
	"time"

	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/mem"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/protoadapt"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestBytesCodecV2(t *testing.T) {
	// Generate some random content.
	content := make([]byte, 1<<10+1) // 1 kib + 1 byte
	rand.New(rand.NewSource(0)).Read(content)

	// Calculate full content hashes.
	crc32c := crc32.Checksum(content, crc32.MakeTable(crc32.Castagnoli))
	hasher := md5.New()
	if _, err := hasher.Write(content); err != nil {
		t.Errorf("hasher.Write: %v", err)
	}
	md5 := hasher.Sum(nil)

	trueBool := true
	metadata := &storagepb.Object{
		Name:               "object-name",
		Bucket:             "bucket-name",
		Etag:               "etag",
		Generation:         100,
		Metageneration:     907,
		StorageClass:       "Standard",
		Size:               1025,
		ContentEncoding:    "none",
		ContentDisposition: "inline",
		CacheControl:       "public, max-age=3600",
		Acl: []*storagepb.ObjectAccessControl{{
			Role:   "role",
			Id:     "id",
			Entity: "allUsers",
			Etag:   "tag",
			Email:  "email@foo.com",
		}},
		ContentLanguage: "mi, en",
		DeleteTime:      toProtoTimestamp(time.Now()),
		ContentType:     "application/octet-stream",
		CreateTime:      toProtoTimestamp(time.Now()),
		ComponentCount:  1,
		Checksums: &storagepb.ObjectChecksums{
			Crc32C:  &crc32c,
			Md5Hash: md5,
		},
		TemporaryHold: true,
		Metadata: map[string]string{
			"a-key": "a-value",
		},
		EventBasedHold: &trueBool,
		Owner: &storagepb.Owner{
			Entity:   "user-1",
			EntityId: "1",
		},
		CustomerEncryption: &storagepb.CustomerEncryption{
			EncryptionAlgorithm: "alg",
			KeySha256Bytes:      []byte("bytes"),
		},
		HardDeleteTime: toProtoTimestamp(time.Now()),
	}

	for _, test := range []struct {
		desc        string
		resp        *storagepb.BidiReadObjectResponse
		wantContent []byte
	}{
		{
			desc: "filled object response",
			resp: &storagepb.BidiReadObjectResponse{
				ObjectDataRanges: []*storagepb.ObjectRangeData{
					{
						ChecksummedData: &storagepb.ChecksummedData{
							Content: content,
							Crc32C:  &crc32c,
						},
						ReadRange: &storagepb.ReadRange{
							ReadOffset: 0,
							ReadLength: 1025,
							ReadId:     1,
						},
						RangeEnd: true,
					},
				},
				Metadata: metadata,
				ReadHandle: &storagepb.BidiReadHandle{
					Handle: []byte("abcde"),
				},
			},
			wantContent: content,
		},
		{
			desc:        "empty object response",
			resp:        &storagepb.BidiReadObjectResponse{},
			wantContent: []byte{},
		},
		{
			desc: "partially empty",
			resp: &storagepb.BidiReadObjectResponse{
				ObjectDataRanges: []*storagepb.ObjectRangeData{
					{
						ChecksummedData: &storagepb.ChecksummedData{},
					},
				},
				Metadata: &storagepb.Object{
					Checksums: &storagepb.ObjectChecksums{
						Md5Hash: md5,
					},
				},
			},
			wantContent: []byte{},
		},
		{
			desc: "empty ObjectDataRanges",
			resp: &storagepb.BidiReadObjectResponse{
				ObjectDataRanges: []*storagepb.ObjectRangeData{},
				Metadata: &storagepb.Object{
					Checksums: &storagepb.ObjectChecksums{
						Md5Hash: md5,
					},
				},
			},
			wantContent: []byte{},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			for _, subtest := range []struct {
				desc        string
				splitBuffer func([]byte) [][]byte // call this to split the message into multiple buffers.
			}{
				{
					desc: "single buffer",
					splitBuffer: func(b []byte) [][]byte {
						return [][]byte{b}
					},
				},
				{
					desc: "split every 100 bytes",
					splitBuffer: func(b []byte) [][]byte {
						var bufs [][]byte
						var i int
						for i = 0; i < len(b)-100; i += 100 {
							bufs = append(bufs, b[i:i+100])
						}
						bufs = append(bufs, b[i:])
						return bufs
					},
				},
				{
					desc: "split every 8 bytes",
					splitBuffer: func(b []byte) [][]byte {
						var bufs [][]byte
						var i int
						for i = 0; i < len(b)-8; i += 8 {
							bufs = append(bufs, b[i:i+8])
						}
						bufs = append(bufs, b[i:])
						return bufs
					},
				},
				{
					desc: "split every byte",
					splitBuffer: func(b []byte) [][]byte {
						var bufs [][]byte
						for i := 0; i < len(b); i++ {
							bufs = append(bufs, b[i:i+1])
						}
						return bufs
					},
				},
			} {
				t.Run(subtest.desc, func(t *testing.T) {
					// Encode the response.
					encodedResp, err := proto.Marshal(test.resp)
					if err != nil {
						t.Fatalf("proto.Marshal: %v", err)
					}
					// Convert response data into mem.BufferSlice, potentially split across multiple buffers.
					var respData mem.BufferSlice
					slices := subtest.splitBuffer(encodedResp)
					for _, s := range slices {
						respData = append(respData, mem.SliceBuffer(s))
					}
					// Unmarshal and decode response using custom decoding.
					var encodedBytes mem.BufferSlice = mem.BufferSlice{}
					if err := bytesCodecV2.Unmarshal(bytesCodecV2{}, respData, &encodedBytes); err != nil {
						t.Fatalf("unmarshal: %v", err)
					}

					decoder := &readResponseDecoder{
						databufs: encodedBytes,
					}

					err = decoder.readFullObjectResponse()
					if err != nil {
						t.Fatalf("readFullObjectResponse: %v", err)
					}

					// Compare the result with the original ReadObjectResponse, without the content
					if diff := cmp.Diff(decoder.msg, test.resp, protocmp.Transform(), protocmp.IgnoreMessages(&storagepb.ObjectRangeData{})); diff != "" {
						t.Errorf("cmp.Diff message: got(-),want(+):\n%s", diff)
					}

					// Read out the data and compare length and content.
					buf := &bytes.Buffer{}
					n, err := decoder.writeToAndUpdateCRC(buf, func([]byte) {})
					if n != int64(len(test.wantContent)) {
						t.Errorf("mismatched content length: got %d, want %d, offsets %+v", n, len(content), decoder.dataOffsets)
					}
					if !bytes.Equal(buf.Bytes(), test.wantContent) {
						t.Errorf("returned message content did not match")
					}

				})
			}
		})
	}
}

func str(s *status.Status) string {
	if s == nil {
		return "nil"
	}
	if s.Proto() == nil {
		return "<Code=OK>"
	}
	return fmt.Sprintf("<Code=%v, Message=%q, Details=%+v>", s.Code(), s.Message(), s.Details())
}

func TestErrorGenerationAndRetrival(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		code    codes.Code
		details []protoadapt.MessageV1
	}{
		{
			desc: "redirect",
			code: codes.Unavailable,
			details: []protoadapt.MessageV1{
				&storagepb.BidiReadObjectRedirectedError{
					ReadHandle: &storagepb.BidiReadHandle{
						Handle: []byte{1, 2, 3},
					},
					RoutingToken: proto.String("redirect-routing-1234"),
				},
			},
		},
		{
			desc: "read-range",
			code: codes.NotFound,
			details: []protoadapt.MessageV1{
				&storagepb.BidiReadObjectError{
					ReadRangeErrors: []*storagepb.ReadRangeError{{ReadId: 4}},
				},
			},
		},
	} {
		initialStatus := status.New(tc.code, tc.desc)
		newStatus, err := initialStatus.WithDetails(tc.details...)
		if err != nil {
			t.Fatalf("(%v).WithDetails(%+v) failed: %v", str(newStatus), tc.details, err)
		}
		errorReceived := newStatus.Err()
		finalStatus := status.Convert(errorReceived)
		if finalStatus.Code() != tc.code {
			t.Fatalf("status code expected: %v, received: %v", tc.code, finalStatus.Code())
		}
		detail := finalStatus.Details()
		for i := range detail {
			if !proto.Equal(detail[i].(protoreflect.ProtoMessage), tc.details[i].(protoreflect.ProtoMessage)) {
				t.Fatalf("(%v).Details()[%d] = %+v, want %+v", str(finalStatus), i, detail[i], tc.details[i])
			}
		}
		if finalStatus.Message() != tc.desc {
			t.Fatalf("(%v)message()= %v, want %v", str(finalStatus), finalStatus.Message(), tc.desc)
		}
	}
}

func TestErrorExtension(t *testing.T) {
	// Create an initial BidiReadObjectRedirectedError.
	initialStatus := status.New(codes.Unavailable, "redirect")
	reqDetails := &storagepb.BidiReadObjectRedirectedError{
		ReadHandle: &storagepb.BidiReadHandle{
			Handle: []byte{1, 2, 3},
		},
		RoutingToken: proto.String("redirect-routing-1234"),
	}
	newStatus, err := initialStatus.WithDetails(reqDetails)
	if err != nil {
		t.Fatalf("(%v).WithDetails(%+v) failed: %v", str(newStatus), reqDetails, err)
	}
	// Decode the above error extension to get BidiReadObjectRedirectedError.
	errorReceived := newStatus.Err()
	rpcStatus := status.Convert(errorReceived)
	respDetails := rpcStatus.Details()
	for _, detail := range respDetails {
		if bidiError, ok := detail.(*storagepb.BidiReadObjectRedirectedError); ok {
			// Compare the result with the original BidiReadObjectRedirectedError.
			if diff := cmp.Diff(bidiError, reqDetails, protocmp.Transform()); diff != "" {
				t.Errorf("cmp.Diff got(-),want(+):\n%s", diff)
			}
		}
	}
}
