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
	"context"
	"crypto/md5"
	"fmt"
	"hash/crc32"
	"math/rand"
	"testing"
	"time"

	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/mem"
	"google.golang.org/grpc/metadata"
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
					n, found, err := decoder.writeToAndUpdateCRC(buf, 1, func([]byte) {})
					if err != nil {
						t.Fatalf("decoder.writeToAndUpdateCRC: %v", err)
					}
					if len(test.resp.ObjectDataRanges) > 0 && test.resp.ObjectDataRanges[0].ReadRange != nil && !found {
						t.Fatalf("decoder.writeToAndUpdateCRC: range not found")
					}
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

func TestRoutingInterceptors(t *testing.T) {
	tests := []struct {
		desc          string
		enforced      bool
		initialParams string
		want          string
	}{
		{
			desc:     "enforced new header",
			enforced: true,
			want:     "force_direct_connectivity=ENFORCED",
		},
		{
			desc:     "default when not enforced",
			enforced: false,
			want:     "",
		},
		{
			desc:          "enforced append to existing",
			enforced:      true,
			initialParams: "bucket=my-bucket",
			want:          "bucket=my-bucket&force_direct_connectivity=ENFORCED",
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			c := &grpcStorageClient{
				config: &storageConfig{grpcDirectPathEnforced: tc.enforced},
			}
			unary, stream := c.routingInterceptors()

			ctx := context.Background()
			if tc.initialParams != "" {
				ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(requestParamsHeaderKey, tc.initialParams))
			}
			cc := &grpc.ClientConn{}
			// Unary Interceptor
			t.Run("unary", func(t *testing.T) {
				var got string
				invoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
					md, _ := metadata.FromOutgoingContext(ctx)
					if vals := md.Get(requestParamsHeaderKey); len(vals) > 0 {
						got = vals[0]
					}
					return nil
				}

				if err := unary(ctx, "/test/method", nil, nil, cc, invoker); err != nil {
					t.Errorf("unary error: %v", err)
				}
				if got != tc.want {
					t.Errorf("got %q, want %q", got, tc.want)
				}
			})

			// Stream Interceptor
			t.Run("stream", func(t *testing.T) {
				var got string
				streamer := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
					md, _ := metadata.FromOutgoingContext(ctx)
					if vals := md.Get(requestParamsHeaderKey); len(vals) > 0 {
						got = vals[0]
					}
					return nil, nil
				}

				if _, err := stream(ctx, nil, cc, "/test/stream", streamer); err != nil {
					t.Errorf("stream error: %v", err)
				}
				if got != tc.want {
					t.Errorf("got %q, want %q", got, tc.want)
				}
			})
		})
	}
}

func TestPrepareDirectPathMetadata(t *testing.T) {
	tests := []struct {
		desc          string
		enforced      bool
		target        string
		initialParams string
		want          string
	}{
		{
			desc:     "DirectPath target with ENFORCED",
			enforced: true,
			target:   "google-c2p:///storage.googleapis.com",
			want:     "force_direct_connectivity=ENFORCED",
		},
		{
			desc:     "DirectPath target with NOT ENFORCED",
			enforced: false,
			target:   "google-c2p:///storage.googleapis.com",
			want:     "",
		},
		{
			desc:     "CloudPath target with ENFORCED",
			enforced: true,
			target:   "dns:///storage.googleapis.com",
			want:     "force_direct_connectivity=OPTED_OUT",
		},
		{
			desc:     "CloudPath target with NOT ENFORCED",
			enforced: false,
			target:   "dns:///storage.googleapis.com",
			want:     "force_direct_connectivity=OPTED_OUT",
		},
		{
			desc:     "Empty target with ENFORCED",
			enforced: true,
			target:   "",
			want:     "force_direct_connectivity=ENFORCED",
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			c := &grpcStorageClient{
				config: &storageConfig{grpcDirectPathEnforced: tc.enforced},
			}

			ctx := context.Background()
			if tc.initialParams != "" {
				ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(requestParamsHeaderKey, tc.initialParams))
			}

			newCtx, err := c.prepareDirectPathMetadata(ctx, tc.target)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			md, ok := metadata.FromOutgoingContext(newCtx)
			if !ok {
				t.Fatal("metadata not found in context")
			}

			got := md.Get(requestParamsHeaderKey)
			if len(got) == 0 {
				if tc.want != "" {
					t.Fatal("request params header not found")
				}
			} else if got[0] != tc.want {
				t.Errorf("got metadata %q, want %q", got[0], tc.want)
			}
		})
	}
}
