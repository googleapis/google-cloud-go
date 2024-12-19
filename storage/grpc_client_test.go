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
	"errors"
	"hash/crc32"
	"io"
	"math/rand"
	"testing"
	"time"

	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/mem"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
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
		resp        *storagepb.ReadObjectResponse
		wantContent []byte
	}{
		{
			desc: "filled object response",
			resp: &storagepb.ReadObjectResponse{
				ChecksummedData: &storagepb.ChecksummedData{
					Content: content,
					Crc32C:  &crc32c,
				},
				ObjectChecksums: &storagepb.ObjectChecksums{
					Crc32C:  &crc32c,
					Md5Hash: md5,
				},
				ContentRange: &storagepb.ContentRange{
					Start:          0,
					End:            1025,
					CompleteLength: 1025,
				},
				Metadata: metadata,
			},
			wantContent: content,
		},
		{
			desc:        "empty object response",
			resp:        &storagepb.ReadObjectResponse{},
			wantContent: []byte{},
		},
		{
			desc: "partially empty",
			resp: &storagepb.ReadObjectResponse{
				ChecksummedData: &storagepb.ChecksummedData{},
				ObjectChecksums: &storagepb.ObjectChecksums{Md5Hash: md5},
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
					if diff := cmp.Diff(decoder.msg, test.resp, protocmp.Transform(), protocmp.IgnoreMessages(&storagepb.ChecksummedData{})); diff != "" {
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

func TestWriteBackoff(t *testing.T) {
	ctx := context.Background()

	writeStream := &MockWriteStream{}
	streamInterceptor := grpc.WithStreamInterceptor(
		func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			return writeStream, nil
		})

	c, err := NewGRPCClient(ctx, option.WithoutAuthentication(), option.WithGRPCDialOption(streamInterceptor))
	if err != nil {
		t.Fatalf("NewGRPCClient: %v", err)
	}

	w := gRPCWriter{
		c:         (c.tc).(*grpcStorageClient),
		buf:       []byte("012345689abcdefg"),
		chunkSize: 10,
		ctx:       ctx,
		attrs:     &ObjectAttrs{},
	}

	maxAttempts := 4 // be careful changing this value; some tests depend on it

	retriableErr := status.Errorf(codes.Internal, "a retriable error")
	nonretriableErr := status.Errorf(codes.PermissionDenied, "a non-retriable error")
	testCases := []struct {
		desc            string
		recvErrs        []error
		sendErrs        []error
		closeSend       bool // if true, will mimic finishing the upload so CloseSend will be called
		expectedRetries int
		expectedErr     error // final err from uploadBuffer should wrap this err
	}{
		{
			desc:            "retriable error on receive",
			recvErrs:        []error{retriableErr, retriableErr, retriableErr, retriableErr},
			expectedRetries: maxAttempts - 1,
			expectedErr:     retriableErr,
		},
		{
			desc:            "retriable error on send",
			sendErrs:        []error{io.EOF},
			recvErrs:        []error{retriableErr, retriableErr, retriableErr, retriableErr},
			expectedRetries: maxAttempts - 1,
			expectedErr:     retriableErr,
		},
		{
			desc:            "non-retriable error on send",
			sendErrs:        []error{io.EOF},
			recvErrs:        []error{nonretriableErr},
			expectedRetries: 0,
			expectedErr:     nonretriableErr,
		},
		{
			desc:            "non-retriable err after send closed",
			recvErrs:        []error{nil, nonretriableErr},
			expectedRetries: 0,
			expectedErr:     nonretriableErr,
			closeSend:       true,
		},
		{
			desc:            "retriable err after send closed",
			recvErrs:        []error{nil, retriableErr, retriableErr, retriableErr, retriableErr},
			expectedRetries: maxAttempts - 1,
			expectedErr:     retriableErr,
			closeSend:       true,
		},
	}
	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			backoff := &mockBackoff{}
			retryConfig := newUploadBufferRetryConfig(&settings{})
			retryConfig.config.maxAttempts = &maxAttempts
			retryConfig.config.backoff = backoff

			writeStream.RecvMsgErrToReturn = test.recvErrs
			writeStream.SendMsgErrToReturn = test.sendErrs

			_, _, err = w.uploadBuffer(0, 0, test.closeSend, retryConfig)
			if !errors.Is(err, test.expectedErr) {
				t.Fatalf("uploadBuffer, want err to wrap: %v, got err: %v", test.expectedErr, err)
			}

			if got, want := backoff.pauseCallsCount, test.expectedRetries; got != want {
				t.Errorf("backoff.Pause was called %d times, expected %d", got, want)
			}
		})
	}
}

type MockWriteStream struct {
	grpc.ClientStream
	SendMsgErrToReturn []error // if the array is empty, returns nil; otherwise, pops the first value
	RecvMsgErrToReturn []error // if the array is empty, returns nil; otherwise, pops the first value
}

func (s *MockWriteStream) SendMsg(m any) error {
	if len(s.SendMsgErrToReturn) < 1 {
		return nil
	}
	err := s.SendMsgErrToReturn[0]
	s.SendMsgErrToReturn = s.SendMsgErrToReturn[1:]
	return err
}

// Retriable errors mean we should start over and attempt to
// resend the entire buffer via a new stream.
// If not retriable, falling through will return the error received.
func (s *MockWriteStream) RecvMsg(m any) error {
	if len(s.RecvMsgErrToReturn) < 1 {
		return nil
	}
	err := s.RecvMsgErrToReturn[0]
	s.RecvMsgErrToReturn = s.RecvMsgErrToReturn[1:]
	return err
}

func (s *MockWriteStream) CloseSend() error {
	return nil
}

// mockBackoff keeps count of the number of time Pause was called. Not thread-safe.
type mockBackoff struct {
	Initial         time.Duration
	Max             time.Duration
	Multiplier      float64
	pauseCallsCount int
}

func (b *mockBackoff) Pause() time.Duration {
	b.pauseCallsCount++
	return 0
}

func (b *mockBackoff) SetInitial(i time.Duration) {
	b.Initial = i
}

func (b *mockBackoff) SetMax(m time.Duration) {
	b.Max = m
}

func (b *mockBackoff) SetMultiplier(m float64) {
	b.Multiplier = m
}

func (b *mockBackoff) GetInitial() time.Duration {
	return b.Initial
}

func (b *mockBackoff) GetMax() time.Duration {
	return b.Max
}

func (b *mockBackoff) GetMultiplier() float64 {
	return b.Multiplier
}
