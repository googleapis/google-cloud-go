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

package genai

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	pb "cloud.google.com/go/aiplatform/apiv1beta1/aiplatformpb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/iterator"
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

func testCaching(t *testing.T, client *Client) {
	ctx := context.Background()
	const model = "gemini-1.5-pro-001"

	t.Run("CRUD", func(t *testing.T) {
		must := func(cc *CachedContent, err error) *CachedContent {
			t.Helper()
			if err != nil {
				t.Fatal(err)
			}
			return cc
		}

		want := &CachedContent{
			Model: "projects/" + os.Getenv("VERTEX_PROJECT_ID") +
				"/locations/us-central1/publishers/google/models/" + model,
			CreateTime: time.Now().UTC(),
			UpdateTime: time.Now().UTC(),
		}

		compare := func(got *CachedContent, expireTime time.Time) {
			t.Helper()
			want.Expiration.ExpireTime = expireTime
			if diff := cmp.Diff(want, got,
				cmpopts.EquateApproxTime(10*time.Second),
				cmpopts.IgnoreFields(CachedContent{}, "Name", "UpdateTime")); diff != "" {
				t.Errorf("mismatch (-want, +got):\n%s", diff)
			}
		}

		ttl := 30 * time.Minute
		wantExpireTime := time.Now().Add(ttl)
		argcc := &CachedContent{
			Model:      model,
			Expiration: ExpireTimeOrTTL{TTL: ttl},
			Contents: []*Content{{Role: "user", Parts: []Part{
				FileData{MIMEType: "text/plain", FileURI: "gs://0002-test-multimodal/embeddings/xray-embedding.json"},
			}}},
		}
		cc := must(client.CreateCachedContent(ctx, argcc))
		compare(cc, wantExpireTime)
		name := cc.Name
		cc2 := must(client.GetCachedContent(ctx, name))
		compare(cc2, wantExpireTime)
		gotList := listAll(t, client.ListCachedContents(ctx))
		var cc3 *CachedContent
		for _, cc := range gotList {
			if cc.Name == name {
				cc3 = cc
				break
			}
		}
		if cc3 == nil {
			t.Fatal("did not find created in list")
		}
		compare(cc3, wantExpireTime)

		// Update using expire time.
		newExpireTime := cc3.Expiration.ExpireTime.Add(15 * time.Minute)
		cc4 := must(client.UpdateCachedContent(ctx, cc3, &CachedContentToUpdate{
			Expiration: &ExpireTimeOrTTL{ExpireTime: newExpireTime},
		}))
		compare(cc4, newExpireTime)

		t.Run("update-ttl", func(t *testing.T) {
			// Update using TTL.
			cc5 := must(client.UpdateCachedContent(ctx, cc4, &CachedContentToUpdate{
				Expiration: &ExpireTimeOrTTL{TTL: ttl},
			}))
			compare(cc5, time.Now().Add(ttl))
		})

		if err := client.DeleteCachedContent(ctx, name); err != nil {
			t.Fatal(err)
		}

		if err := client.DeleteCachedContent(ctx, "bad name"); err == nil {
			t.Fatal("want error, got nil")
		}
	})
	t.Run("generation", func(t *testing.T) {
		txt := strings.Repeat("George Washington was the first president of the United States. ", 3000)
		argcc := &CachedContent{
			Model:    model,
			Contents: []*Content{{Role: "user", Parts: []Part{Text(txt)}}},
		}
		cc, err := client.CreateCachedContent(ctx, argcc)
		if err != nil {
			t.Fatal(err)
		}
		defer client.DeleteCachedContent(ctx, cc.Name)
		m := client.GenerativeModelFromCachedContent(cc)
		res, err := m.GenerateContent(ctx, Text("Who was the first US president?"))
		if err != nil {
			t.Fatal(err)
		}
		got := responseString(res)
		const want = "Washington"
		if !strings.Contains(got, want) {
			t.Errorf("got %q, want string containing %q", got, want)
		}
	})
}

func listAll(t *testing.T, iter *CachedContentIterator) []*CachedContent {
	var ccs []*CachedContent
	for {
		cc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		ccs = append(ccs, cc)
	}
	return ccs
}
