// Copyright 2023 Google LLC
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

package genai

import (
	"context"
	"fmt"
	"testing"
	"time"

	pb "cloud.google.com/go/aiplatform/apiv1beta1/aiplatformpb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

func TestPopulateCachedContent(t *testing.T) {
	tm := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	cmpOpt := cmpopts.IgnoreUnexported(
		timestamppb.Timestamp{},
		durationpb.Duration{},
	)
	for _, test := range []struct {
		proto  *pb.CachedContent
		veneer *CachedContent
	}{
		{&pb.CachedContent{}, &CachedContent{}},
		{
			&pb.CachedContent{Expiration: &pb.CachedContent_ExpireTime{ExpireTime: timestamppb.New(tm)}},
			&CachedContent{Expiration: ExpireTimeOrTTL{ExpireTime: tm}},
		},
		{
			&pb.CachedContent{Expiration: &pb.CachedContent_Ttl{durationpb.New(time.Hour)}},
			&CachedContent{Expiration: ExpireTimeOrTTL{TTL: time.Hour}},
		},
	} {
		var gotp pb.CachedContent
		populateCachedContentTo(&gotp, test.veneer)
		if g, w := gotp.Expiration, test.proto.Expiration; !cmp.Equal(g, w, cmpOpt) {
			t.Errorf("from %v to proto: got  %v, want %v", test.veneer.Expiration, g, w)
		}

		var gotv CachedContent
		populateCachedContentFrom(&gotv, test.proto)
		if g, w := gotv.Expiration, test.veneer.Expiration; !cmp.Equal(g, w) {
			t.Errorf("from %v to veneer: got  %v, want %v", test.proto.Expiration, g, w)
		}
	}
}

// called from client_test.go:TestLive.
func testCaching(t *testing.T, client *Client) {
	t.Skip("caching not yet working")
	ctx := context.Background()
	must := func(cc *CachedContent, err error) *CachedContent {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return cc
	}

	argcc := &CachedContent{
		Name:              "vertex-caching-test",
		Model:             "gemini-1.5-pro",
		SystemInstruction: &Content{Parts: []Part{Text("si")}},
	}
	cc := must(client.CreateCachedContent(ctx, argcc))
	fmt.Println(cc)
}
