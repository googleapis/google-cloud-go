// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and

package wire

import (
	"encoding/base64"
	"errors"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/pubsublite/internal/test"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	spb "google.golang.org/genproto/googleapis/rpc/status"
)

func makeStreamResetSignal() error {
	statuspb := &spb.Status{
		Code: int32(codes.Aborted),
		Details: []*anypb.Any{test.MakeAny(&errdetails.ErrorInfo{
			Reason: "RESET", Domain: "pubsublite.googleapis.com",
		})},
	}
	return status.ErrorProto(statuspb)
}

func TestIsStreamResetSignal(t *testing.T) {
	for _, tc := range []struct {
		desc string
		err  error
		want bool
	}{
		{
			desc: "reset signal",
			err:  makeStreamResetSignal(),
			want: true,
		},
		{
			desc: "non-retryable code",
			err: status.ErrorProto(&spb.Status{
				Code:    int32(codes.FailedPrecondition),
				Details: []*anypb.Any{test.MakeAny(&errdetails.ErrorInfo{Reason: "RESET", Domain: "pubsublite.googleapis.com"})},
			}),
			want: false,
		},
		{
			desc: "wrong domain",
			err: status.ErrorProto(&spb.Status{
				Code:    int32(codes.Aborted),
				Details: []*anypb.Any{test.MakeAny(&errdetails.ErrorInfo{Reason: "RESET"})},
			}),
			want: false,
		},
		{
			desc: "wrong reason",
			err: status.ErrorProto(&spb.Status{
				Code:    int32(codes.Aborted),
				Details: []*anypb.Any{test.MakeAny(&errdetails.ErrorInfo{Domain: "pubsublite.googleapis.com"})},
			}),
			want: false,
		},
		{
			desc: "missing details",
			err:  status.ErrorProto(&spb.Status{Code: int32(codes.Aborted)}),
			want: false,
		},
		{
			desc: "nil error",
			err:  nil,
			want: false,
		},
		{
			desc: "generic error",
			err:  errors.New(""),
			want: false,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			if got := isStreamResetSignal(tc.err); got != tc.want {
				t.Errorf("isStreamResetSignal() got: %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPubsubMetadataAddClientInfo(t *testing.T) {
	for _, tc := range []struct {
		desc           string
		framework      FrameworkType
		libraryVersion string
		wantClientInfo *structpb.Struct
	}{
		{
			desc:           "minimal",
			libraryVersion: "",
			wantClientInfo: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"language": stringValue("GOLANG"),
				},
			},
		},
		{
			desc:           "cps shim",
			framework:      FrameworkCloudPubSubShim,
			libraryVersion: "",
			wantClientInfo: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"language":  stringValue("GOLANG"),
					"framework": stringValue("CLOUD_PUBSUB_SHIM"),
				},
			},
		},
		{
			desc:           "version valid",
			framework:      FrameworkCloudPubSubShim,
			libraryVersion: "1.2.3",
			wantClientInfo: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"language":      stringValue("GOLANG"),
					"framework":     stringValue("CLOUD_PUBSUB_SHIM"),
					"major_version": numberValue(1),
					"minor_version": numberValue(2),
				},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			metadata := newPubsubMetadata()
			metadata.doAddClientInfo(tc.framework, tc.libraryVersion)

			b, err := base64.StdEncoding.DecodeString(metadata["x-goog-pubsub-context"])
			if err != nil {
				t.Errorf("Failed to decode base64 pubsub context header: %v", err)
				return
			}
			gotClientInfo := new(structpb.Struct)
			if err := proto.Unmarshal(b, gotClientInfo); err != nil {
				t.Errorf("Failed to unmarshal pubsub context structpb: %v", err)
				return
			}
			if diff := testutil.Diff(gotClientInfo, tc.wantClientInfo); diff != "" {
				t.Errorf("Pubsub context structpb: got: -, want: +\n%s", diff)
			}
		})
	}
}
