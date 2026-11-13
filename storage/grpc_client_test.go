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
	"io"
	"math/rand"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/storage/experimental"
	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/mem"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/protoadapt"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
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
				if !strings.Contains(got, tc.want) {
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
				if !strings.Contains(got, tc.want) {
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
			want:     "force_direct_connectivity=ENFORCED&direct_connectivity_diagnostic=reason",
		},
		{
			desc:     "CloudPath target with NOT ENFORCED",
			enforced: false,
			target:   "dns:///storage.googleapis.com",
			want:     "direct_connectivity_diagnostic=reason",
		},
		{
			desc:     "DirectPath over Interconnect target with ENFORCED",
			enforced: true,
			target:   "google-c2p:///storage-direct.googleapis.com?force-xds",
			want:     "force_direct_connectivity=ENFORCED",
		},
		{
			desc:     "DirectPath over Interconnect target with NOT ENFORCED",
			enforced: false,
			target:   "google-c2p:///storage-direct.googleapis.com?force-xds",
			want:     "",
		},
		{
			desc:     "Empty target with ENFORCED",
			enforced: true,
			target:   "",
			want:     "force_direct_connectivity=ENFORCED&direct_connectivity_diagnostic=reason",
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			c := &grpcStorageClient{
				config: &storageConfig{grpcDirectPathEnforced: tc.enforced},
				dpDiag: "reason",
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
			var headerValue string
			if len(got) > 0 {
				headerValue = got[0]
			}
			if headerValue != tc.want {
				t.Errorf("got metadata %q, want %q", headerValue, tc.want)
			}
		})
	}
}

func TestNewGRPCStorageClient_InterconnectOption(t *testing.T) {
	ctx := context.Background()
	client, err := newGRPCStorageClient(ctx,
		withClientOptions(
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
			experimental.WithDirectPathXdsOverInterconnect(),
		),
	)
	if err != nil {
		t.Fatalf("newGRPCStorageClient: %v", err)
	}
	defer client.Close()

	res, err := internaloption.NewUnsafeResolver(client.settings.clientOption...)
	if err != nil {
		t.Fatalf("NewUnsafeResolver: %v", err)
	}
	if got := res.ResolvedEnableDirectPathXdsOverInterconnect(); !got {
		t.Errorf("res.ResolvedEnableDirectPathXdsOverInterconnect() = %v, want true", got)
	}
}

func TestPrepareDirectPathMetadata_FeatureTracking(t *testing.T) {
	tests := []struct {
		desc            string
		configFeatures  uint32
		contextFeatures []trackedFeature
		wantFeatures    uint32
	}{
		{
			desc:         "no features",
			wantFeatures: 0,
		},
		{
			desc:           "config features only",
			configFeatures: uint32(1 << featurePCU),
			wantFeatures:   uint32(1 << featurePCU),
		},
		{
			desc:            "merged features",
			configFeatures:  uint32(1 << featurePCU),
			contextFeatures: []trackedFeature{featureMultistreamInMRD},
			wantFeatures:    uint32(1<<featurePCU) | uint32(1<<featureMultistreamInMRD),
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			c := &grpcStorageClient{
				config:                  &storageConfig{},
				configFeatureAttributes: tc.configFeatures,
			}

			ctx := context.Background()
			if len(tc.contextFeatures) > 0 {
				ctx = addFeatureAttributes(ctx, tc.contextFeatures...)
			}

			newCtx, err := c.prepareDirectPathMetadata(ctx, directPathEndpointPrefix)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			md, ok := metadata.FromOutgoingContext(newCtx)
			if !ok {
				t.Fatal("metadata not found in context")
			}

			got := md.Get(featureTrackerHeaderName)
			if tc.wantFeatures == 0 {
				if len(got) > 0 {
					t.Errorf("got features %q, want none", got[0])
				}
				return
			}

			if len(got) == 0 {
				t.Fatalf("features header missing, want %d", tc.wantFeatures)
			}

			decoded, err := decodeFeatureMask(got[0])
			if err != nil {
				t.Fatalf("failed to decode features: %v", err)
			}

			if decoded != tc.wantFeatures {
				t.Errorf("got features %d, want %d", decoded, tc.wantFeatures)
			}
		})
	}
}

// TestPrepareDirectPathMetadata_OwnsFeatureHeader verifies that the client
// always rewrites the feature header in outgoing metadata from its own
// bitmask: foreign or malformed values (e.g. set by a caller for a different
// client) are replaced by the merged mask, or dropped when there is nothing
// to report.
func TestPrepareDirectPathMetadata_OwnsFeatureHeader(t *testing.T) {
	foreign := strings.Repeat("Zm9v", 130) // ~390 bytes of non-mask payload.
	tests := []struct {
		desc           string
		configFeatures uint32
		ctxFeatures    []trackedFeature
		incomingMD     []string
		want           []string
	}{
		{
			desc: "no features, no header: untouched",
			want: nil,
		},
		{
			desc:       "no features, zero mask header: dropped",
			incomingMD: []string{encodeFeatureMask(0)},
			want:       nil,
		},
		{
			desc:       "no features, foreign value: dropped",
			incomingMD: []string{foreign},
			want:       nil,
		},
		{
			desc:           "features, foreign value: replaced by mask",
			configFeatures: uint32(1 << featurePCU),
			incomingMD:     []string{foreign},
			want:           []string{encodeFeatureMask(uint32(1 << featurePCU))},
		},
		{
			desc:           "legacy standard-alphabet value is merged and re-encoded",
			configFeatures: uint32(1 << featurePCU),
			ctxFeatures:    []trackedFeature{featureMultistreamInMRD},
			incomingMD:     []string{"AAAA+A=="}, // 0xF8 in standard base64 (bits 0 and 1 unset).
			want:           []string{encodeFeatureMask(0xF8 | uint32(1<<featurePCU) | uint32(1<<featureMultistreamInMRD))},
		},
		{
			desc:       "multiple values collapse to one",
			incomingMD: []string{encodeFeatureMask(1), encodeFeatureMask(2), foreign},
			want:       []string{encodeFeatureMask(3)},
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			c := &grpcStorageClient{
				config:                  &storageConfig{},
				configFeatureAttributes: tc.configFeatures,
			}
			ctx := context.Background()
			if len(tc.ctxFeatures) > 0 {
				ctx = addFeatureAttributes(ctx, tc.ctxFeatures...)
			}
			var callerMD metadata.MD
			if len(tc.incomingMD) > 0 {
				callerMD = metadata.MD{}
				callerMD.Set(featureTrackerHeaderName, tc.incomingMD...)
				ctx = metadata.NewOutgoingContext(ctx, callerMD)
			}

			newCtx, err := c.prepareDirectPathMetadata(ctx, directPathEndpointPrefix)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			md, _ := metadata.FromOutgoingContext(newCtx)
			got := md.Get(featureTrackerHeaderName)
			if diff := cmp.Diff(tc.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("metadata %s mismatch (-want +got):\n%s", featureTrackerHeaderName, diff)
			}
			if diff := cmp.Diff(tc.incomingMD, callerMD.Get(featureTrackerHeaderName), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("caller metadata %s modified (-want +got):\n%s", featureTrackerHeaderName, diff)
			}
		})
	}
}

// TestListObjects_UserProject verifies that ListObjects correctly applies the
// withUserProject storageOption to the outgoing context metadata (x-goog-user-project)
// via setUserProjectMetadata, without initiating actual RPC calls.
func TestListObjects_UserProject(t *testing.T) {
	ctx := context.Background()
	c := &grpcStorageClient{
		settings: &settings{},
	}
	it := c.ListObjects(ctx, "bucket", nil, withUserProject("project-id"))

	md, ok := metadata.FromOutgoingContext(it.ctx)
	if !ok {
		t.Fatalf("Expected outgoing metadata in context")
	}

	if got := md.Get("x-goog-user-project"); len(got) == 0 || got[0] != "project-id" {
		t.Errorf("Expected x-goog-user-project to be project-id, got %v", got)
	}
}

func TestNewGRPCStorageClient_NoGlobalTimeout(t *testing.T) {
	ctx := context.Background()
	client, err := newGRPCStorageClient(ctx,
		withClientOptions(
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		),
	)
	if err != nil {
		t.Fatalf("newGRPCStorageClient: %v", err)
	}
	defer client.Close()

	// Verify that default/global settings do NOT contain any timeout override,
	// preventing indefinite hangs on metadata operations.
	for _, opt := range client.settings.gax {
		if strings.Contains(fmt.Sprintf("%T", opt), "timeoutOpt") {
			t.Errorf("expected default/global settings to not contain a timeout option, but found: %T (%v)", opt, opt)
		}
	}

	// Helper to verify that a list of CallOptions has a timeout of 0s
	verifyTimeoutIsZero := func(method string, opts []gax.CallOption) {
		var cs gax.CallSettings
		// Apply a dummy non-zero timeout first
		gax.WithTimeout(1 * time.Hour).Resolve(&cs)

		// Apply all options of interest
		for _, opt := range opts {
			opt.Resolve(&cs)
		}

		val := reflect.ValueOf(cs)
		field := val.FieldByName("timeout")
		if !field.IsValid() {
			t.Errorf("method %q: gax.CallSettings structure changed, field 'timeout' not found", method)
			return
		}

		timeout := time.Duration(field.Int())
		if timeout == 1*time.Hour {
			t.Errorf("method %q: expected explicit WithTimeout(0) to keep streams unbounded, but none was found", method)
		} else if timeout != 0 {
			t.Errorf("method %q: expected timeout of 0s, got %v", method, timeout)
		}
	}

	// Verify that all generated streaming CallOptions explicitly contain WithTimeout(0)
	// to keep payload streams (Reads/Writes) completely timeout-free.
	opts := client.raw.CallOptions
	verifyTimeoutIsZero("ReadObject", opts.ReadObject)
	verifyTimeoutIsZero("WriteObject", opts.WriteObject)
	verifyTimeoutIsZero("BidiReadObject", opts.BidiReadObject)
	verifyTimeoutIsZero("BidiWriteObject", opts.BidiWriteObject)
	verifyTimeoutIsZero("CancelResumableWrite", opts.CancelResumableWrite)
}

func TestVerifyChecksums(t *testing.T) {
	helloWorldCRC := crc32.Checksum([]byte("hello world"), crc32cTable)

	tests := []struct {
		desc        string
		msg         *storagepb.BidiReadObjectResponse
		databufs    mem.BufferSlice
		dataOffsets map[int64]bufferSliceOffsets
		initialErrs map[int64]error
		wantErrs    map[int64]string // readID -> substring of error message, or empty if no error expected
	}{
		{
			desc: "nil msg returns early and does not initialize crcErrs",
			msg:  nil,
			databufs: mem.BufferSlice{
				mem.SliceBuffer([]byte("hello world")),
			},
			wantErrs: nil, // stays nil
		},
		{
			desc: "empty databufs with checksum return error",
			msg: &storagepb.BidiReadObjectResponse{
				ObjectDataRanges: []*storagepb.ObjectRangeData{
					{
						ChecksummedData: &storagepb.ChecksummedData{
							Crc32C: &helloWorldCRC,
						},
						ReadRange: &storagepb.ReadRange{ReadId: 1},
					},
				},
			},
			databufs: mem.BufferSlice{},
			wantErrs: map[int64]string{
				1: "storage: checksum mismatch: bad CRC on chunk read: got",
			},
		},
		{
			desc: "missing checksummed data is skipped",
			msg: &storagepb.BidiReadObjectResponse{
				ObjectDataRanges: []*storagepb.ObjectRangeData{
					{
						ReadRange: &storagepb.ReadRange{ReadId: 1},
					},
				},
			},
			databufs: mem.BufferSlice{
				mem.SliceBuffer([]byte("hello world")),
			},
			dataOffsets: map[int64]bufferSliceOffsets{
				1: {startBuf: 0, endBuf: 0, startOff: 0, endOff: 11},
			},
			wantErrs: map[int64]string{}, // initialized but empty
		},
		{
			desc: "missing Crc32C pointer is skipped",
			msg: &storagepb.BidiReadObjectResponse{
				ObjectDataRanges: []*storagepb.ObjectRangeData{
					{
						ChecksummedData: &storagepb.ChecksummedData{},
						ReadRange:       &storagepb.ReadRange{ReadId: 1},
					},
				},
			},
			databufs: mem.BufferSlice{
				mem.SliceBuffer([]byte("hello world")),
			},
			dataOffsets: map[int64]bufferSliceOffsets{
				1: {startBuf: 0, endBuf: 0, startOff: 0, endOff: 11},
			},
			wantErrs: map[int64]string{},
		},
		{
			desc: "single buffer correct checksum",
			msg: &storagepb.BidiReadObjectResponse{
				ObjectDataRanges: []*storagepb.ObjectRangeData{
					{
						ChecksummedData: &storagepb.ChecksummedData{
							Crc32C: &helloWorldCRC,
						},
						ReadRange: &storagepb.ReadRange{ReadId: 1},
					},
				},
			},
			databufs: mem.BufferSlice{
				mem.SliceBuffer([]byte("hello world")),
			},
			dataOffsets: map[int64]bufferSliceOffsets{
				1: {startBuf: 0, endBuf: 0, startOff: 0, endOff: 11},
			},
			wantErrs: map[int64]string{},
		},
		{
			desc: "single buffer incorrect checksum",
			msg: &storagepb.BidiReadObjectResponse{
				ObjectDataRanges: []*storagepb.ObjectRangeData{
					{
						ChecksummedData: &storagepb.ChecksummedData{
							Crc32C: proto.Uint32(12345),
						},
						ReadRange: &storagepb.ReadRange{ReadId: 1},
					},
				},
			},
			databufs: mem.BufferSlice{
				mem.SliceBuffer([]byte("hello world")),
			},
			dataOffsets: map[int64]bufferSliceOffsets{
				1: {startBuf: 0, endBuf: 0, startOff: 0, endOff: 11},
			},
			wantErrs: map[int64]string{
				1: "storage: checksum mismatch: bad CRC on chunk read: got",
			},
		},
		{
			desc: "multiple buffers correct checksum",
			msg: &storagepb.BidiReadObjectResponse{
				ObjectDataRanges: []*storagepb.ObjectRangeData{
					{
						ChecksummedData: &storagepb.ChecksummedData{
							Crc32C: &helloWorldCRC,
						},
						ReadRange: &storagepb.ReadRange{ReadId: 1},
					},
				},
			},
			databufs: mem.BufferSlice{
				mem.SliceBuffer([]byte("prefix hello")),
				mem.SliceBuffer([]byte(" world diff message")),
				mem.SliceBuffer([]byte("suffix")),
			},
			dataOffsets: map[int64]bufferSliceOffsets{
				1: {startBuf: 0, endBuf: 1, startOff: 7, endOff: 6},
			},
			wantErrs: map[int64]string{},
		},
		{
			desc: "multiple buffers incorrect checksum",
			msg: &storagepb.BidiReadObjectResponse{
				ObjectDataRanges: []*storagepb.ObjectRangeData{
					{
						ChecksummedData: &storagepb.ChecksummedData{
							Crc32C: proto.Uint32(12345),
						},
						ReadRange: &storagepb.ReadRange{ReadId: 1},
					},
				},
			},
			databufs: mem.BufferSlice{
				mem.SliceBuffer([]byte("prefix_hello")),
				mem.SliceBuffer([]byte("_world_")),
				mem.SliceBuffer([]byte("suffix")),
			},
			dataOffsets: map[int64]bufferSliceOffsets{
				1: {startBuf: 0, endBuf: 1, startOff: 7, endOff: 6},
			},
			wantErrs: map[int64]string{
				1: "storage: checksum mismatch: bad CRC on chunk read: got",
			},
		},
		{
			desc: "startOff >= endOff empty segment",
			msg: &storagepb.BidiReadObjectResponse{
				ObjectDataRanges: []*storagepb.ObjectRangeData{
					{
						ChecksummedData: &storagepb.ChecksummedData{
							Crc32C: proto.Uint32(0),
						},
						ReadRange: &storagepb.ReadRange{ReadId: 1},
					},
				},
			},
			databufs: mem.BufferSlice{
				mem.SliceBuffer([]byte("hello world")),
			},
			dataOffsets: map[int64]bufferSliceOffsets{
				1: {startBuf: 0, endBuf: 0, startOff: 5, endOff: 5},
			},
			wantErrs: map[int64]string{},
		},
		{
			desc: "multiple ranges mix of results",
			msg: &storagepb.BidiReadObjectResponse{
				ObjectDataRanges: []*storagepb.ObjectRangeData{
					{
						ChecksummedData: &storagepb.ChecksummedData{
							Crc32C: &helloWorldCRC,
						},
						ReadRange: &storagepb.ReadRange{ReadId: 10},
					},
					{
						ChecksummedData: &storagepb.ChecksummedData{
							Crc32C: proto.Uint32(9999),
						},
						ReadRange: &storagepb.ReadRange{ReadId: 20},
					},
					{
						ReadRange: &storagepb.ReadRange{ReadId: 30},
					},
				},
			},
			databufs: mem.BufferSlice{
				mem.SliceBuffer([]byte("hello world")),
			},
			dataOffsets: map[int64]bufferSliceOffsets{
				10: {startBuf: 0, endBuf: 0, startOff: 0, endOff: 11},
				20: {startBuf: 0, endBuf: 0, startOff: 0, endOff: 11},
				30: {startBuf: 0, endBuf: 0, startOff: 0, endOff: 11},
			},
			wantErrs: map[int64]string{
				20: "storage: checksum mismatch: bad CRC on chunk read: got",
			},
		},
		{
			desc: "pre-existing errors are preserved",
			msg: &storagepb.BidiReadObjectResponse{
				ObjectDataRanges: []*storagepb.ObjectRangeData{
					{
						ChecksummedData: &storagepb.ChecksummedData{
							Crc32C: &helloWorldCRC,
						},
						ReadRange: &storagepb.ReadRange{ReadId: 10},
					},
				},
			},
			databufs: mem.BufferSlice{
				mem.SliceBuffer([]byte("hello world")),
			},
			dataOffsets: map[int64]bufferSliceOffsets{
				10: {startBuf: 0, endBuf: 0, startOff: 0, endOff: 11},
			},
			initialErrs: map[int64]error{
				99: fmt.Errorf("some other error"),
			},
			wantErrs: map[int64]string{
				99: "some other error",
				10: "",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			decoder := &readResponseDecoder{
				msg:         tc.msg,
				databufs:    tc.databufs,
				dataOffsets: tc.dataOffsets,
				crcErrs:     tc.initialErrs,
			}

			decoder.verifyChecksums()

			if tc.wantErrs == nil {
				if decoder.crcErrs != nil {
					t.Errorf("expected crcErrs to be nil, got: %v", decoder.crcErrs)
				}
				return
			}

			// Count expected errors (we don't expect errors for empty strings in wantErrs, i.e. 10 is correct)
			var expectedCount int
			for _, wantSubstr := range tc.wantErrs {
				if wantSubstr != "" {
					expectedCount++
				}
			}

			var gotCount int
			for _, err := range decoder.crcErrs {
				if err != nil {
					gotCount++
				}
			}

			if gotCount != expectedCount {
				t.Errorf("mismatched error count: got %v, want %v", decoder.crcErrs, tc.wantErrs)
			}

			for readID, wantSubstr := range tc.wantErrs {
				err, ok := decoder.crcErrs[readID]
				if wantSubstr == "" {
					if ok && err != nil {
						t.Errorf("unexpected error for read ID %d: %v", readID, err)
					}
					continue
				}
				if !ok {
					t.Errorf("expected error for read ID %d, but none was found", readID)
					continue
				}
				if !strings.Contains(err.Error(), wantSubstr) {
					t.Errorf("read ID %d error message: got %q, want it to contain %q", readID, err.Error(), wantSubstr)
				}
			}
		})
	}
}

func TestBidiReadStreamSessionGracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fakeStream := &fakeBidiReadObjectClient{
		closeSendCh:   make(chan struct{}),
		msgsBeforeEOF: 2,
		streamCtx:     ctx,
	}
	session := &bidiReadStreamSession{
		reqC:       make(chan *storagepb.BidiReadObjectRequest, 5),
		ctx:        ctx,
		managerCtx: ctx,
		cancel:     cancel,
		stream:     fakeStream,
		params:     &newMultiRangeDownloaderParams{disableMRDReadChecksum: true},
	}
	session.wg.Add(2)
	go session.sendLoop()
	go session.receiveLoop()

	// Initial shutdown should close reqC, set manualShutdown, and drain
	// remaining messages on the stream until EOF before cancelling ctx.
	session.Shutdown()

	if !session.manualShutdown {
		t.Errorf("got session.manualShutdown = false, want true")
	}
	if !fakeStream.closeSendCalled {
		t.Errorf("got fakeStream.closeSendCalled = false, want true")
	}
	if got, want := fakeStream.recvCount, 3; got != want {
		t.Errorf("got fakeStream.recvCount = %d, want %d (2 in-flight messages + EOF)", got, want)
	}
	if !fakeStream.eofBeforeCancel {
		t.Errorf("got fakeStream.eofBeforeCancel = false, want true")
	}

	select {
	case _, ok := <-session.reqC:
		if ok {
			t.Errorf("expected reqC to be closed")
		}
	default:
		t.Errorf("expected reqC to be closed and readable")
	}

	// Calling Shutdown again should be idempotent and not panic.
	session.Shutdown()
}

type fakeRangeReadServer struct {
	storagepb.UnimplementedStorageServer
	data             []byte
	omitContentRange bool
}

func (s *fakeRangeReadServer) sliceRange(offset, limit int64) (start, end int64, slice []byte) {
	end = int64(len(s.data))
	start = offset
	if start < 0 {
		start = max(end+start, 0)
	}
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	return start, end, s.data[start:end]
}

func (s *fakeRangeReadServer) ReadObject(req *storagepb.ReadObjectRequest, stream storagepb.Storage_ReadObjectServer) error {
	start, end, slice := s.sliceRange(req.GetReadOffset(), req.GetReadLimit())
	var cr *storagepb.ContentRange
	if !s.omitContentRange {
		cr = &storagepb.ContentRange{Start: start, End: end, CompleteLength: int64(len(s.data))}
	}
	return stream.Send(&storagepb.ReadObjectResponse{
		ChecksummedData: &storagepb.ChecksummedData{Content: slice},
		ContentRange:    cr,
		Metadata:        &storagepb.Object{Size: int64(len(s.data)), FinalizeTime: timestamppb.Now()},
	})
}

func (s *fakeRangeReadServer) BidiReadObject(stream storagepb.Storage_BidiReadObjectServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}
	rr := req.GetReadRanges()[0]
	start, end, slice := s.sliceRange(rr.GetReadOffset(), rr.GetReadLength())
	return stream.Send(&storagepb.BidiReadObjectResponse{
		Metadata: &storagepb.Object{Size: int64(len(s.data)), FinalizeTime: timestamppb.Now()},
		ObjectDataRanges: []*storagepb.ObjectRangeData{{
			ChecksummedData: &storagepb.ChecksummedData{Content: slice},
			ReadRange:       &storagepb.ReadRange{ReadOffset: start, ReadLength: end - start, ReadId: rr.GetReadId()},
			RangeEnd:        true,
		}},
	})
}

func newFakeRangeReadStorageClient(t *testing.T, srv storagepb.StorageServer) *grpcStorageClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	storagepb.RegisterStorageServer(gs, srv)
	go gs.Serve(lis)
	t.Cleanup(gs.Stop)

	ctx := context.Background()
	c, err := newGRPCStorageClient(ctx, withClientOptions(
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		})),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		WithDisabledClientMetrics(),
		WithGRPCBidiReads(),
	))
	if err != nil {
		t.Fatalf("newGRPCStorageClient: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

type rangeDrainTestCase struct {
	name             string
	offset           int64
	length           int64
	readBytes        int
	omitContentRange bool
	wantDrained      bool
}

var rangeDrainTestCases = []rangeDrainTestCase{
	{name: "full read", offset: 0, length: -1, readBytes: 100, wantDrained: true},
	{name: "completed partial range read", offset: 20, length: 10, readBytes: 10, wantDrained: true},
	{name: "open-ended range from offset", offset: 20, length: -1, readBytes: 80, wantDrained: true},
	{name: "negative-offset suffix range", offset: -15, length: -1, readBytes: 15, wantDrained: true},
	{name: "range length past object end", offset: 80, length: 50, readBytes: 20, wantDrained: true},
	{name: "incomplete partial range read", offset: 20, length: 10, readBytes: 4, wantDrained: false},
}

func TestGRPCReaderCloseDrainOnCompletion(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789"), 10) // 100 bytes
	srv := &fakeRangeReadServer{data: data}
	c := newFakeRangeReadStorageClient(t, srv)

	for _, tc := range rangeDrainTestCases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := c.NewRangeReader(context.Background(), &newRangeReaderParams{
				bucket: "b",
				object: "o",
				gen:    -1,
				offset: tc.offset,
				length: tc.length,
			})
			if err != nil {
				t.Fatalf("NewRangeReader: %v", err)
			}
			gr := r.reader.(*gRPCReader)
			fakeStream := &fakeBidiReadObjectClient{ClientStream: gr.stream}
			gr.stream = fakeStream
			var canceled bool
			origCancel := gr.cancel
			gr.cancel = func() {
				canceled = true
				origCancel()
			}

			if _, err := io.ReadFull(r, make([]byte, tc.readBytes)); err != nil {
				t.Fatalf("io.ReadFull: %v", err)
			}
			if err := r.Close(); err != nil {
				t.Fatalf("r.Close: %v", err)
			}
			if got := fakeStream.recvd; got != tc.wantDrained {
				t.Errorf("got stream drained = %v, want %v", got, tc.wantDrained)
			}
			if !canceled {
				t.Errorf("got canceled = false, want true")
			}
			if gr.stream != nil {
				t.Errorf("got gr.stream = %v, want nil after Close", gr.stream)
			}
		})
	}
}

func TestGRPCReadObjectReaderCloseDrainOnCompletion(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789"), 10) // 100 bytes
	srv := &fakeRangeReadServer{data: data}
	c := newFakeRangeReadStorageClient(t, srv)

	cases := append([]rangeDrainTestCase(nil), rangeDrainTestCases...)
	cases = append(cases, rangeDrainTestCase{
		name:             "completed partial range read without ContentRange",
		offset:           20,
		length:           10,
		readBytes:        10,
		omitContentRange: true,
		wantDrained:      true,
	})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv.omitContentRange = tc.omitContentRange
			r, err := c.NewRangeReaderReadObject(context.Background(), &newRangeReaderParams{
				bucket: "b",
				object: "o",
				gen:    -1,
				offset: tc.offset,
				length: tc.length,
			})
			if err != nil {
				t.Fatalf("NewRangeReaderReadObject: %v", err)
			}
			gr := r.reader.(*gRPCReadObjectReader)
			fakeStream := &fakeReadObjectClient{ClientStream: gr.stream}
			gr.stream = fakeStream
			var canceled bool
			origCancel := gr.cancel
			gr.cancel = func() {
				canceled = true
				origCancel()
			}

			if _, err := io.ReadFull(r, make([]byte, tc.readBytes)); err != nil {
				t.Fatalf("io.ReadFull: %v", err)
			}
			if err := r.Close(); err != nil {
				t.Fatalf("r.Close: %v", err)
			}
			if got := fakeStream.recvd; got != tc.wantDrained {
				t.Errorf("got stream drained = %v, want %v", got, tc.wantDrained)
			}
			if !canceled {
				t.Errorf("got canceled = false, want true")
			}
			if gr.stream != nil {
				t.Errorf("got gr.stream = %v, want nil after Close", gr.stream)
			}
		})
	}
}

type fakeBidiReadObjectClient struct {
	grpc.ClientStream
	recvd           bool
	recvCount       int
	msgsBeforeEOF   int
	closeSendCh     chan struct{}
	closeSendCalled bool
	eofBeforeCancel bool
	streamCtx       context.Context
}

func (f *fakeBidiReadObjectClient) RecvMsg(m interface{}) error {
	f.recvd = true
	if f.closeSendCh != nil {
		<-f.closeSendCh
	}
	f.recvCount++
	if f.recvCount <= f.msgsBeforeEOF {
		return nil
	}
	if f.streamCtx != nil && f.streamCtx.Err() == nil {
		f.eofBeforeCancel = true
	}
	if f.ClientStream != nil {
		return f.ClientStream.RecvMsg(m)
	}
	return io.EOF
}

func (f *fakeBidiReadObjectClient) Send(*storagepb.BidiReadObjectRequest) error {
	return nil
}

func (f *fakeBidiReadObjectClient) Recv() (*storagepb.BidiReadObjectResponse, error) {
	f.recvd = true
	return nil, io.EOF
}

func (f *fakeBidiReadObjectClient) CloseSend() error {
	if !f.closeSendCalled {
		f.closeSendCalled = true
		if f.closeSendCh != nil {
			close(f.closeSendCh)
		}
	}
	if f.ClientStream != nil {
		return f.ClientStream.CloseSend()
	}
	return nil
}

type fakeReadObjectClient struct {
	grpc.ClientStream
	recvd bool
}

func (f *fakeReadObjectClient) RecvMsg(m interface{}) error {
	f.recvd = true
	if f.ClientStream != nil {
		return f.ClientStream.RecvMsg(m)
	}
	return io.EOF
}

func (f *fakeReadObjectClient) Recv() (*storagepb.ReadObjectResponse, error) {
	f.recvd = true
	return nil, io.EOF
}

func TestRangeReaderRemain(t *testing.T) {
	const size = 100
	for _, tc := range []struct {
		desc        string
		startOffset int64
		length      int64
		want        int64
	}{
		{desc: "entire object", startOffset: 0, length: -1, want: size},
		{desc: "entire object with exact length", startOffset: 0, length: size, want: size},
		{desc: "prefix", startOffset: 0, length: 10, want: 10},
		{desc: "range in the middle", startOffset: 40, length: 20, want: 20},
		{desc: "range in the middle with length larger than offset", startOffset: 10, length: 30, want: 30},
		{desc: "range in the middle with offset larger than length", startOffset: 60, length: 10, want: 10},
		{desc: "range ending exactly at end of object", startOffset: 60, length: 40, want: 40},
		{desc: "length past end of object", startOffset: 60, length: 80, want: 40},
		{desc: "length larger than object", startOffset: 60, length: 2 * size, want: 40},
		{desc: "rest of object from offset", startOffset: 60, length: -1, want: 40},
		{desc: "zero length", startOffset: 60, length: 0, want: 0},
		{desc: "offset at end of object", startOffset: size, length: 10, want: 0},
		{desc: "offset past end of object", startOffset: size + 10, length: 10, want: 0},
		{desc: "offset past end of object, rest of object", startOffset: size + 10, length: -1, want: 0},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			if got := rangeReaderRemain(size, tc.startOffset, tc.length); got != tc.want {
				t.Errorf("rangeReaderRemain(%d, %d, %d) = %d, want %d", size, tc.startOffset, tc.length, got, tc.want)
			}
		})
	}
}

// TestRangeReaderRemainLargeObject covers fixed-size range reads deep into a
// multi-GiB object.
func TestRangeReaderRemainLargeObject(t *testing.T) {
	const (
		size   = 4 * 1024 * MiB
		length = 16 * MiB
	)
	for _, offset := range []int64{0, length, 2214592512, 3338665984, size - length} {
		if got := rangeReaderRemain(size, offset, length); got != length {
			t.Errorf("rangeReaderRemain(%d, %d, %d) = %d, want %d", int64(size), offset, int64(length), got, int64(length))
		}
	}
}

// authCredentialsOptions returns the option.WithAuthCredentials options in opts.
func authCredentialsOptions(opts []option.ClientOption) []option.ClientOption {
	credsOptType := reflect.TypeOf(option.WithAuthCredentials(nil))
	var matched []option.ClientOption
	for _, o := range opts {
		if reflect.TypeOf(o) == credsOptType {
			matched = append(matched, o)
		}
	}
	return matched
}

func TestNewGRPCStorageClient_AuthTracingCustomCredentials(t *testing.T) {
	ctx := context.Background()
	t.Setenv(storageOtelTracingDevVar, "true")
	t.Setenv("STORAGE_EMULATOR_HOST", "")
	t.Setenv("STORAGE_EMULATOR_HOST_GRPC", "")
	// Keep the test hermetic: skip DirectPath and GCE metadata probing.
	t.Setenv(directPathDisableEnvVar, "true")

	baseProvider := &staticAuthTokenProvider{token: &auth.Token{Value: "fake-token"}}
	baseCreds := &auth.Credentials{TokenProvider: baseProvider}
	client, err := newGRPCStorageClient(ctx,
		withClientOptions(
			option.WithAuthCredentials(baseCreds),
			WithDisabledClientMetrics(),
		),
	)
	if err != nil {
		t.Fatalf("newGRPCStorageClient: %v", err)
	}
	defer client.Close()

	if got := len(authCredentialsOptions(client.settings.clientOption)); got != 2 {
		t.Errorf("got %d WithAuthCredentials options, want 2 (caller's and traced)", got)
	}
	resolved, err := internaloption.AuthCreds(ctx, client.settings.clientOption)
	if err != nil {
		t.Fatalf("internaloption.AuthCreds: %v", err)
	}
	if resolved == baseCreds {
		t.Fatal("resolved credentials alias the caller's credentials; want a wrapped copy")
	}
	traced, ok := resolved.TokenProvider.(*tracedTokenProvider)
	if !ok {
		t.Fatalf("got resolved TokenProvider %T, want *tracedTokenProvider", resolved.TokenProvider)
	}
	if traced.base != auth.TokenProvider(baseProvider) {
		t.Errorf("got traced base TokenProvider %T, want the caller's provider", traced.base)
	}
	// Caller-owned credentials must not be mutated by client construction.
	if baseCreds.TokenProvider != auth.TokenProvider(baseProvider) {
		t.Errorf("caller-owned credentials were mutated: got TokenProvider %T, want %T", baseCreds.TokenProvider, baseProvider)
	}
}

func TestNewGRPCStorageClient_AuthTracingWithoutAuthentication(t *testing.T) {
	ctx := context.Background()
	t.Setenv(storageOtelTracingDevVar, "true")
	t.Setenv("STORAGE_EMULATOR_HOST", "")
	t.Setenv("STORAGE_EMULATOR_HOST_GRPC", "")

	client, err := newGRPCStorageClient(ctx,
		withClientOptions(
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
			WithDisabledClientMetrics(),
		),
	)
	if err != nil {
		t.Fatalf("newGRPCStorageClient: %v", err)
	}
	defer client.Close()

	if got := len(authCredentialsOptions(client.settings.clientOption)); got != 0 {
		t.Errorf("got %d WithAuthCredentials options, want 0 with WithoutAuthentication", got)
	}
}
