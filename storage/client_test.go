// Copyright 2022 Google LLC
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
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/iam/apiv1/iampb"
	"cloud.google.com/go/storage/experimental"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/gax-go/v2"
	"github.com/googleapis/gax-go/v2/apierror"
	"github.com/googleapis/gax-go/v2/callctx"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var emulatorClients map[string]storageClient
var veneerClient *Client

func TestCreateBucketEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		want := &BucketAttrs{
			Name: bucket,
			Logging: &BucketLogging{
				LogBucket: bucket,
			},
		}
		got, err := client.CreateBucket(ctx, project, want.Name, want, nil)
		if err != nil {
			t.Fatal(err)
		}
		want.Location = "US"
		if diff := cmp.Diff(got.Name, want.Name); diff != "" {
			t.Errorf("Name got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.Location, want.Location); diff != "" {
			t.Errorf("Location got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.Logging.LogBucket, want.Logging.LogBucket); diff != "" {
			t.Errorf("LogBucket got(-),want(+):\n%s", diff)
		}
	})
}

func TestDeleteBucketEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		b := &BucketAttrs{
			Name: bucket,
		}
		// Create the bucket that will be deleted.
		_, err := client.CreateBucket(ctx, project, b.Name, b, nil)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		// Delete the bucket that was just created.
		err = client.DeleteBucket(ctx, b.Name, nil)
		if err != nil {
			t.Fatalf("client.DeleteBucket: %v", err)
		}
	})
}

func TestGetBucketEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		want := &BucketAttrs{
			Name: bucket,
		}
		// Create the bucket that will be retrieved.
		_, err := client.CreateBucket(ctx, project, want.Name, want, nil)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		got, err := client.GetBucket(ctx, want.Name, &BucketConditions{MetagenerationMatch: 1})
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(got.Name, want.Name); diff != "" {
			t.Errorf("got(-),want(+):\n%s", diff)
		}
	})
}

func TestUpdateBucketEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		bkt := &BucketAttrs{
			Name: bucket,
		}
		// Create the bucket that will be updated.
		_, err := client.CreateBucket(ctx, project, bkt.Name, bkt, nil)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}

		ua := &BucketAttrsToUpdate{
			VersioningEnabled:     false,
			RequesterPays:         false,
			DefaultEventBasedHold: false,
			Encryption:            &BucketEncryption{DefaultKMSKeyName: "key2"},
			Lifecycle: &Lifecycle{
				Rules: []LifecycleRule{
					{
						Action:    LifecycleAction{Type: "Delete"},
						Condition: LifecycleCondition{AgeInDays: 30},
					},
				},
			},
			Logging:      &BucketLogging{LogBucket: "lb", LogObjectPrefix: "p"},
			Website:      &BucketWebsite{MainPageSuffix: "mps", NotFoundPage: "404"},
			StorageClass: "NEARLINE",
			RPO:          RPOAsyncTurbo,
		}
		want := &BucketAttrs{
			Name:                  bucket,
			VersioningEnabled:     false,
			RequesterPays:         false,
			DefaultEventBasedHold: false,
			Encryption:            &BucketEncryption{DefaultKMSKeyName: "key2"},
			Lifecycle: Lifecycle{
				Rules: []LifecycleRule{
					{
						Action:    LifecycleAction{Type: "Delete"},
						Condition: LifecycleCondition{AgeInDays: 30},
					},
				},
			},
			Logging:      &BucketLogging{LogBucket: "lb", LogObjectPrefix: "p"},
			Website:      &BucketWebsite{MainPageSuffix: "mps", NotFoundPage: "404"},
			StorageClass: "NEARLINE",
			RPO:          RPOAsyncTurbo,
		}

		got, err := client.UpdateBucket(ctx, bucket, ua, &BucketConditions{MetagenerationMatch: 1})
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(got.Name, want.Name); diff != "" {
			t.Errorf("Name: got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.VersioningEnabled, want.VersioningEnabled); diff != "" {
			t.Errorf("VersioningEnabled: got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.RequesterPays, want.RequesterPays); diff != "" {
			t.Errorf("RequesterPays: got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.DefaultEventBasedHold, want.DefaultEventBasedHold); diff != "" {
			t.Errorf("DefaultEventBasedHold: got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.Encryption, want.Encryption); diff != "" {
			t.Errorf("Encryption: got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.Lifecycle, want.Lifecycle); diff != "" {
			t.Errorf("Lifecycle: got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.Logging, want.Logging); diff != "" {
			t.Errorf("Logging: got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.Website, want.Website); diff != "" {
			t.Errorf("Website: got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.RPO, want.RPO); diff != "" {
			t.Errorf("RPO: got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.StorageClass, want.StorageClass); diff != "" {
			t.Errorf("StorageClass: got(-),want(+):\n%s", diff)
		}
	})
}

func TestGetServiceAccountEmulated(t *testing.T) {
	transportClientTest(skipGRPC("serviceaccount is not implemented"), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		_, err := client.GetServiceAccount(ctx, project)
		if err != nil {
			t.Fatalf("client.GetServiceAccount: %v", err)
		}
	})
}

func TestGetSetTestIamPolicyEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		battrs, err := client.CreateBucket(ctx, project, bucket, &BucketAttrs{
			Name: bucket,
		}, nil)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		got, err := client.GetIamPolicy(ctx, battrs.Name, 0)
		if err != nil {
			t.Fatalf("client.GetIamPolicy: %v", err)
		}
		err = client.SetIamPolicy(ctx, battrs.Name, &iampb.Policy{
			Etag:     got.GetEtag(),
			Bindings: []*iampb.Binding{{Role: "roles/viewer", Members: []string{"allUsers"}}},
		})
		if err != nil {
			t.Fatalf("client.SetIamPolicy: %v", err)
		}
		want := []string{"storage.foo", "storage.bar"}
		perms, err := client.TestIamPermissions(ctx, battrs.Name, want)
		if err != nil {
			t.Fatalf("client.TestIamPermissions: %v", err)
		}
		if diff := cmp.Diff(perms, want); diff != "" {
			t.Errorf("got(-),want(+):\n%s", diff)
		}
	})
}

func TestDeleteObjectEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		// Populate test object that will be deleted.
		_, err := client.CreateBucket(ctx, project, bucket, &BucketAttrs{
			Name: bucket,
		}, nil)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		want := ObjectAttrs{
			Bucket: bucket,
			Name:   fmt.Sprintf("testObject-%d", time.Now().Nanosecond()),
		}
		w := veneerClient.Bucket(bucket).Object(want.Name).NewWriter(ctx)
		if _, err := w.Write(randomBytesToWrite); err != nil {
			t.Fatalf("failed to populate test object: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("closing object: %v", err)
		}
		err = client.DeleteObject(ctx, bucket, want.Name, defaultGen, nil)
		if err != nil {
			t.Fatalf("client.DeleteBucket: %v", err)
		}
	})
}

func TestGetObjectEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		// Populate test object.
		_, err := client.CreateBucket(ctx, project, bucket, &BucketAttrs{
			Name: bucket,
		}, nil)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		want := ObjectAttrs{
			Bucket: bucket,
			Name:   fmt.Sprintf("testObject-%d", time.Now().Nanosecond()),
		}
		w := veneerClient.Bucket(bucket).Object(want.Name).NewWriter(ctx)
		if _, err := w.Write(randomBytesToWrite); err != nil {
			t.Fatalf("failed to populate test object: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("closing object: %v", err)
		}
		got, err := client.GetObject(ctx, &getObjectParams{bucket: bucket, object: want.Name, gen: defaultGen})
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(got.Name, want.Name); diff != "" {
			t.Errorf("got(-),want(+):\n%s", diff)
		}
	})
}

func TestRewriteObjectEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		// Populate test object.
		_, err := client.CreateBucket(ctx, project, bucket, &BucketAttrs{
			Name: bucket,
		}, nil)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		src := ObjectAttrs{
			Bucket: bucket,
			Name:   fmt.Sprintf("testObject-%d", time.Now().Nanosecond()),
		}
		w := veneerClient.Bucket(bucket).Object(src.Name).NewWriter(ctx)
		if _, err := w.Write(randomBytesToWrite); err != nil {
			t.Fatalf("failed to populate test object: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("closing object: %v", err)
		}
		req := &rewriteObjectRequest{
			dstObject: destinationObject{
				bucket: bucket,
				name:   fmt.Sprintf("copy-of-%s", src.Name),
				attrs:  &ObjectAttrs{},
			},
			srcObject: sourceObject{
				bucket: bucket,
				name:   src.Name,
				gen:    defaultGen,
			},
		}
		got, err := client.RewriteObject(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		if !got.done {
			t.Fatal("didn't finish writing!")
		}
		if want := int64(len(randomBytesToWrite)); got.written != want {
			t.Errorf("Bytes written: got %d, want %d", got.written, want)
		}
	})
}

func TestUpdateObjectEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		// Populate test object.
		_, err := client.CreateBucket(ctx, project, bucket, &BucketAttrs{
			Name: bucket,
		}, nil)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		ct := time.Date(2022, 5, 25, 12, 12, 12, 0, time.UTC)
		o := ObjectAttrs{
			Bucket:     bucket,
			Name:       fmt.Sprintf("testObject-%d", time.Now().Nanosecond()),
			CustomTime: ct,
		}
		w := veneerClient.Bucket(bucket).Object(o.Name).NewWriter(ctx)
		if _, err := w.Write(randomBytesToWrite); err != nil {
			t.Fatalf("failed to populate test object: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("closing object: %v", err)
		}
		want := &ObjectAttrsToUpdate{
			EventBasedHold:     false,
			TemporaryHold:      false,
			ContentType:        "text/html",
			ContentLanguage:    "en",
			ContentEncoding:    "gzip",
			ContentDisposition: "",
			CacheControl:       "",
			CustomTime:         ct.Add(10 * time.Hour),
		}

		params := &updateObjectParams{bucket: bucket, object: o.Name, uattrs: want, gen: defaultGen, conds: &Conditions{MetagenerationMatch: 1}}
		got, err := client.UpdateObject(ctx, params)
		if err != nil {
			t.Fatalf("client.UpdateObject: %v", err)
		}
		if diff := cmp.Diff(got.Name, o.Name); diff != "" {
			t.Errorf("Name: got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.EventBasedHold, want.EventBasedHold); diff != "" {
			t.Errorf("EventBasedHold: got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.TemporaryHold, want.TemporaryHold); diff != "" {
			t.Errorf("TemporaryHold: got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.ContentType, want.ContentType); diff != "" {
			t.Errorf("ContentType: got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.ContentLanguage, want.ContentLanguage); diff != "" {
			t.Errorf("ContentLanguage: got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.ContentEncoding, want.ContentEncoding); diff != "" {
			t.Errorf("ContentEncoding: got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.ContentDisposition, want.ContentDisposition); diff != "" {
			t.Errorf("ContentDisposition: got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.CacheControl, want.CacheControl); diff != "" {
			t.Errorf("CacheControl: got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.CustomTime, want.CustomTime); diff != "" {
			t.Errorf("CustomTime: got(-),want(+):\n%s", diff)
		}
	})
}

func TestListObjectsEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		// Populate test data.
		_, err := client.CreateBucket(ctx, project, bucket, &BucketAttrs{
			Name: bucket,
		}, nil)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		prefix := time.Now().Nanosecond()
		want := []*ObjectAttrs{
			{
				Bucket: bucket,
				Name:   fmt.Sprintf("%d-object-%d", prefix, time.Now().Nanosecond()),
			},
			{
				Bucket: bucket,
				Name:   fmt.Sprintf("%d-object-%d", prefix, time.Now().Nanosecond()),
			},
			{
				Bucket: bucket,
				Name:   fmt.Sprintf("object-%d", time.Now().Nanosecond()),
			},
		}
		for _, obj := range want {
			w := veneerClient.Bucket(bucket).Object(obj.Name).NewWriter(ctx)
			if _, err := w.Write(randomBytesToWrite); err != nil {
				t.Fatalf("failed to populate test data: %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("closing object: %v", err)
			}
		}

		// Simple list, no query.
		it := client.ListObjects(ctx, bucket, nil)
		var o *ObjectAttrs
		var got int
		for i := 0; err == nil && i <= len(want); i++ {
			o, err = it.Next()
			if err != nil {
				break
			}
			got++
			if diff := cmp.Diff(o.Name, want[i].Name); diff != "" {
				t.Errorf("got(-),want(+):\n%s", diff)
			}
		}
		if err != iterator.Done {
			t.Fatalf("expected %q but got %q", iterator.Done, err)
		}
		expected := len(want)
		if got != expected {
			t.Errorf("expected to get %d objects, but got %d", expected, got)
		}
	})
}

func TestListObjectsWithPrefixEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		// Populate test data.
		_, err := client.CreateBucket(ctx, project, bucket, &BucketAttrs{
			Name: bucket,
		}, nil)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		prefix := time.Now().Nanosecond()
		want := []*ObjectAttrs{
			{
				Bucket: bucket,
				Name:   fmt.Sprintf("%d-object-%d", prefix, time.Now().Nanosecond()),
			},
			{
				Bucket: bucket,
				Name:   fmt.Sprintf("%d-object-%d", prefix, time.Now().Nanosecond()),
			},
			{
				Bucket: bucket,
				Name:   fmt.Sprintf("object-%d", time.Now().Nanosecond()),
			},
		}
		for _, obj := range want {
			w := veneerClient.Bucket(bucket).Object(obj.Name).NewWriter(ctx)
			if _, err := w.Write(randomBytesToWrite); err != nil {
				t.Fatalf("failed to populate test data: %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("closing object: %v", err)
			}
		}

		// Query with Prefix.
		it := client.ListObjects(ctx, bucket, &Query{Prefix: strconv.Itoa(prefix)})
		var o *ObjectAttrs
		var got int
		want = want[:2]
		for i := 0; i <= len(want); i++ {
			o, err = it.Next()
			if err != nil {
				break
			}
			got++
			if diff := cmp.Diff(o.Name, want[i].Name); diff != "" {
				t.Errorf("got(-),want(+):\n%s", diff)
			}
		}
		if err != iterator.Done {
			t.Fatalf("expected %q but got %q", iterator.Done, err)
		}
		expected := len(want)
		if got != expected {
			t.Errorf("expected to get %d objects, but got %d", expected, got)
		}
	})
}

func TestListBucketsEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		prefix := time.Now().Nanosecond()
		want := []*BucketAttrs{
			{Name: fmt.Sprintf("%d-%s-%d", prefix, bucket, time.Now().Nanosecond())},
			{Name: fmt.Sprintf("%d-%s-%d", prefix, bucket, time.Now().Nanosecond())},
			{Name: fmt.Sprintf("%s-%d", bucket, time.Now().Nanosecond())},
		}
		// Create the buckets that will be listed.
		for _, b := range want {
			_, err := client.CreateBucket(ctx, project, b.Name, b, nil)
			if err != nil {
				t.Fatalf("client.CreateBucket: %v", err)
			}
		}

		it := client.ListBuckets(ctx, project)
		it.Prefix = strconv.Itoa(prefix)
		// Drop the non-prefixed bucket from the expected results.
		want = want[:2]
		var err error
		var b *BucketAttrs
		var got int
		for i := 0; err == nil && i <= len(want); i++ {
			b, err = it.Next()
			if err != nil {
				break
			}
			got++
			if diff := cmp.Diff(b.Name, want[i].Name); diff != "" {
				t.Errorf("got(-),want(+):\n%s", diff)
			}
		}
		if err != iterator.Done {
			t.Fatalf("expected %q but got %q", iterator.Done, err)
		}
		expected := len(want)
		if got != expected {
			t.Errorf("expected to get %d buckets, but got %d", expected, got)
		}
	})
}

func TestListBucketACLsEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		attrs := &BucketAttrs{
			Name:          bucket,
			PredefinedACL: "publicRead",
		}
		// Create the bucket that will be retrieved.
		if _, err := client.CreateBucket(ctx, project, attrs.Name, attrs, nil); err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}

		acls, err := client.ListBucketACLs(ctx, bucket)
		if err != nil {
			t.Fatalf("client.ListBucketACLs: %v", err)
		}
		if want, got := len(acls), 2; want != got {
			t.Errorf("ListBucketACLs: got %v, want %v items", acls, want)
		}
	})
}

func TestUpdateBucketACLEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		attrs := &BucketAttrs{
			Name:          bucket,
			PredefinedACL: "authenticatedRead",
		}
		// Create the bucket that will be retrieved.
		if _, err := client.CreateBucket(ctx, project, attrs.Name, attrs, nil); err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		var listAcls []ACLRule
		var err error
		// Assert bucket has two BucketACL entities, including project owner and predefinedACL.
		if listAcls, err = client.ListBucketACLs(ctx, bucket); err != nil {
			t.Fatalf("client.ListBucketACLs: %v", err)
		}
		if got, want := len(listAcls), 2; got != want {
			t.Errorf("ListBucketACLs: got %v, want %v items", listAcls, want)
		}
		entity := AllUsers
		role := RoleReader
		err = client.UpdateBucketACL(ctx, bucket, entity, role)
		if err != nil {
			t.Fatalf("client.UpdateBucketACL: %v", err)
		}
		// Assert bucket now has three BucketACL entities, including existing ACLs.
		if listAcls, err = client.ListBucketACLs(ctx, bucket); err != nil {
			t.Fatalf("client.ListBucketACLs: %v", err)
		}
		if got, want := len(listAcls), 3; got != want {
			t.Errorf("ListBucketACLs: got %v, want %v items", listAcls, want)
		}
	})
}

func TestDeleteBucketACLEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		attrs := &BucketAttrs{
			Name:          bucket,
			PredefinedACL: "publicRead",
		}
		// Create the bucket that will be retrieved.
		if _, err := client.CreateBucket(ctx, project, attrs.Name, attrs, nil); err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		// Assert bucket has two BucketACL entities, including project owner and predefinedACL.
		acls, err := client.ListBucketACLs(ctx, bucket)
		if err != nil {
			t.Fatalf("client.ListBucketACLs: %v", err)
		}
		if got, want := len(acls), 2; got != want {
			t.Errorf("ListBucketACLs: got %v, want %v items", acls, want)
		}
		// Delete one BucketACL with AllUsers entity.
		if err := client.DeleteBucketACL(ctx, bucket, AllUsers); err != nil {
			t.Fatalf("client.DeleteBucketACL: %v", err)
		}
		// Assert bucket has one BucketACL.
		acls, err = client.ListBucketACLs(ctx, bucket)
		if err != nil {
			t.Fatalf("client.ListBucketACLs: %v", err)
		}
		if got, want := len(acls), 1; got != want {
			t.Errorf("ListBucketACLs: got %v, want %v items", acls, want)
		}
	})
}

func TestDefaultObjectACLCRUDEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		attrs := &BucketAttrs{
			Name:                       bucket,
			PredefinedDefaultObjectACL: "publicRead",
		}
		// Create the bucket that will be retrieved.
		if _, err := client.CreateBucket(ctx, project, attrs.Name, attrs, nil); err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		// Assert bucket has 2 DefaultObjectACL entities, including project owner and PredefinedDefaultObjectACL.
		acls, err := client.ListDefaultObjectACLs(ctx, bucket)
		if err != nil {
			t.Fatalf("client.ListDefaultObjectACLs: %v", err)
		}
		if got, want := len(acls), 2; got != want {
			t.Errorf("ListDefaultObjectACLs: got %v, want %v items", acls, want)
		}
		entity := AllAuthenticatedUsers
		role := RoleOwner
		err = client.UpdateDefaultObjectACL(ctx, bucket, entity, role)
		if err != nil {
			t.Fatalf("UpdateDefaultObjectCL: %v", err)
		}
		// Assert there are now 3 DefaultObjectACL entities, including existing DefaultObjectACLs.
		acls, err = client.ListDefaultObjectACLs(ctx, bucket)
		if err != nil {
			t.Fatalf("client.ListDefaultObjectACLs: %v", err)
		}
		if got, want := len(acls), 3; got != want {
			t.Errorf("ListDefaultObjectACLs: %v got %v, want %v items", len(acls), acls, want)
		}
		// Delete 1 DefaultObjectACL with AllUsers entity.
		if err := client.DeleteDefaultObjectACL(ctx, bucket, AllUsers); err != nil {
			t.Fatalf("client.DeleteDefaultObjectACL: %v", err)
		}
		// Assert bucket has 2 DefaultObjectACL entities.
		acls, err = client.ListDefaultObjectACLs(ctx, bucket)
		if err != nil {
			t.Fatalf("client.ListDefaultObjectACLs: %v", err)
		}
		if got, want := len(acls), 2; got != want {
			t.Errorf("ListDefaultObjectACLs: %v got %v, want %v items", len(acls), acls, want)
		}
	})
}

func TestObjectACLCRUDEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		// Populate test object.
		_, err := client.CreateBucket(ctx, project, bucket, &BucketAttrs{
			Name: bucket,
		}, nil)
		if err != nil {
			t.Fatalf("CreateBucket: %v", err)
		}
		o := ObjectAttrs{
			Bucket: bucket,
			Name:   fmt.Sprintf("testObject-%d", time.Now().Nanosecond()),
		}
		w := veneerClient.Bucket(bucket).Object(o.Name).NewWriter(ctx)
		if _, err := w.Write(randomBytesToWrite); err != nil {
			t.Fatalf("failed to populate test object: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("closing object: %v", err)
		}
		var listAcls []ACLRule
		// Assert there are 4 ObjectACL entities, including object owner and project owners/editors/viewers.
		if listAcls, err = client.ListObjectACLs(ctx, bucket, o.Name); err != nil {
			t.Fatalf("ListObjectACLs: %v", err)
		}
		if got, want := len(listAcls), 4; got != want {
			t.Errorf("ListObjectACLs: got %v, want %v items", listAcls, want)
		}
		entity := AllUsers
		role := RoleReader
		err = client.UpdateObjectACL(ctx, bucket, o.Name, entity, role)
		if err != nil {
			t.Fatalf("UpdateObjectCL: %v", err)
		}
		// Assert there are now 5 ObjectACL entities, including existing ACLs.
		if listAcls, err = client.ListObjectACLs(ctx, bucket, o.Name); err != nil {
			t.Fatalf("ListObjectACLs: %v", err)
		}
		if got, want := len(listAcls), 5; got != want {
			t.Errorf("ListObjectACLs: got %v, want %v items", listAcls, want)
		}
		if err = client.DeleteObjectACL(ctx, bucket, o.Name, AllUsers); err != nil {
			t.Fatalf("client.DeleteObjectACL: %v", err)
		}
		// Assert there are now 4 ObjectACL entities after deletion.
		if listAcls, err = client.ListObjectACLs(ctx, bucket, o.Name); err != nil {
			t.Fatalf("ListObjectACLs: %v", err)
		}
		if got, want := len(listAcls), 4; got != want {
			t.Errorf("ListObjectACLs: got %v, want %v items", listAcls, want)
		}
	})
}

func TestOpenReaderEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		// Populate test data.
		_, err := client.CreateBucket(ctx, project, bucket, &BucketAttrs{
			Name: bucket,
		}, nil)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		prefix := time.Now().Nanosecond()
		want := &ObjectAttrs{
			Bucket: bucket,
			Name:   fmt.Sprintf("%d-object-%d", prefix, time.Now().Nanosecond()),
		}
		w := veneerClient.Bucket(bucket).Object(want.Name).NewWriter(ctx)
		if _, err := w.Write(randomBytesToWrite); err != nil {
			t.Fatalf("failed to populate test data: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("closing object: %v", err)
		}

		params := &newRangeReaderParams{
			bucket: bucket,
			object: want.Name,
			gen:    defaultGen,
			offset: 0,
			length: -1,
		}
		r, err := client.NewRangeReader(ctx, params)
		if err != nil {
			t.Fatalf("opening reading: %v", err)
		}
		wantLen := len(randomBytesToWrite)
		got := make([]byte, wantLen)
		n, err := r.Read(got)
		if n != wantLen {
			t.Fatalf("expected to read %d bytes, but got %d", wantLen, n)
		}
		if diff := cmp.Diff(got, randomBytesToWrite); diff != "" {
			t.Fatalf("Read: got(-),want(+):\n%s", diff)
		}
	})
}

func TestOpenWriterEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		// Populate test data.
		_, err := client.CreateBucket(ctx, project, bucket, &BucketAttrs{
			Name: bucket,
		}, nil)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		prefix := time.Now().Nanosecond()
		want := &ObjectAttrs{
			Bucket:     bucket,
			Name:       fmt.Sprintf("%d-object-%d", prefix, time.Now().Nanosecond()),
			Generation: defaultGen,
		}

		var gotAttrs *ObjectAttrs
		params := &openWriterParams{
			attrs:    want,
			bucket:   bucket,
			ctx:      ctx,
			donec:    make(chan struct{}),
			setError: func(_ error) {}, // no-op
			progress: func(_ int64) {}, // no-op
			setObj:   func(o *ObjectAttrs) { gotAttrs = o },
		}
		pw, err := client.OpenWriter(params)
		if err != nil {
			t.Fatalf("failed to open writer: %v", err)
		}
		if _, err := pw.Write(randomBytesToWrite); err != nil {
			t.Fatalf("failed to populate test data: %v", err)
		}
		if err := pw.Close(); err != nil {
			t.Fatalf("closing object: %v", err)
		}
		select {
		case <-params.donec:
		}
		if gotAttrs == nil {
			t.Fatalf("Writer finished, but resulting object wasn't set")
		}
		if diff := cmp.Diff(gotAttrs.Name, want.Name); diff != "" {
			t.Fatalf("Resulting object name: got(-),want(+):\n%s", diff)
		}

		r, err := veneerClient.Bucket(bucket).Object(want.Name).NewReader(ctx)
		if err != nil {
			t.Fatalf("opening reading: %v", err)
		}
		wantLen := len(randomBytesToWrite)
		got := make([]byte, wantLen)
		n, err := r.Read(got)
		if n != wantLen {
			t.Fatalf("expected to read %d bytes, but got %d", wantLen, n)
		}
		if diff := cmp.Diff(got, randomBytesToWrite); diff != "" {
			t.Fatalf("checking written content: got(-),want(+):\n%s", diff)
		}
	})
}

func TestListNotificationsEmulated(t *testing.T) {
	transportClientTest(skipGRPC("notifications not implemented"), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		// Populate test object.
		_, err := client.CreateBucket(ctx, project, bucket, &BucketAttrs{
			Name: bucket,
		}, nil)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		_, err = client.CreateNotification(ctx, bucket, &Notification{
			TopicProjectID: project,
			TopicID:        "go-storage-notification-test",
			PayloadFormat:  "JSON_API_V1",
		})
		if err != nil {
			t.Fatalf("client.CreateNotification: %v", err)
		}
		n, err := client.ListNotifications(ctx, bucket)
		if err != nil {
			t.Fatalf("client.ListNotifications: %v", err)
		}
		if want, got := 1, len(n); want != got {
			t.Errorf("ListNotifications: got %v, want %v items", n, want)
		}
	})
}

func TestCreateNotificationEmulated(t *testing.T) {
	transportClientTest(skipGRPC("notifications not implemented"), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		// Populate test object.
		_, err := client.CreateBucket(ctx, project, bucket, &BucketAttrs{
			Name: bucket,
		}, nil)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}

		want := &Notification{
			TopicProjectID: project,
			TopicID:        "go-storage-notification-test",
			PayloadFormat:  "JSON_API_V1",
		}
		got, err := client.CreateNotification(ctx, bucket, want)
		if err != nil {
			t.Fatalf("client.CreateNotification: %v", err)
		}
		if diff := cmp.Diff(got.TopicID, want.TopicID); diff != "" {
			t.Errorf("CreateNotification topic: got(-),want(+):\n%s", diff)
		}
	})
}

func TestDeleteNotificationEmulated(t *testing.T) {
	transportClientTest(skipGRPC("notifications not implemented"), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		// Populate test object.
		_, err := client.CreateBucket(ctx, project, bucket, &BucketAttrs{
			Name: bucket,
		}, nil)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		var n *Notification
		n, err = client.CreateNotification(ctx, bucket, &Notification{
			TopicProjectID: project,
			TopicID:        "go-storage-notification-test",
			PayloadFormat:  "JSON_API_V1",
		})
		if err != nil {
			t.Fatalf("client.CreateNotification: %v", err)
		}
		err = client.DeleteNotification(ctx, bucket, n.ID)
		if err != nil {
			t.Fatalf("client.DeleteNotification: %v", err)
		}
	})
}

func initEmulatorClients() func() error {
	noopCloser := func() error { return nil }
	if !isEmulatorEnvironmentSet() {
		return noopCloser
	}
	ctx := context.Background()

	grpcClient, err := newGRPCStorageClient(ctx)
	if err != nil {
		log.Fatalf("Error setting up gRPC client for emulator tests: %v", err)
		return noopCloser
	}
	httpClient, err := newHTTPStorageClient(ctx)
	if err != nil {
		log.Fatalf("Error setting up HTTP client for emulator tests: %v", err)
		return noopCloser
	}
	emulatorClients = map[string]storageClient{
		"http": httpClient,
		"grpc": grpcClient,
	}

	veneerClient, err = NewClient(ctx)
	if err != nil {
		log.Fatalf("Error setting up Veneer client for emulator tests: %v", err)
		return noopCloser
	}

	return func() error {
		gerr := grpcClient.Close()
		herr := httpClient.Close()
		verr := veneerClient.Close()

		if gerr != nil {
			return gerr
		} else if herr != nil {
			return herr
		}
		return verr
	}
}

func TestLockBucketRetentionPolicyEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		b := &BucketAttrs{
			Name: bucket,
			RetentionPolicy: &RetentionPolicy{
				RetentionPeriod: time.Minute,
			},
		}
		// Create the bucket that will be locked.
		_, err := client.CreateBucket(ctx, project, b.Name, b, nil)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		// Lock the bucket's retention policy.
		err = client.LockBucketRetentionPolicy(ctx, b.Name, &BucketConditions{MetagenerationMatch: 1})
		if err != nil {
			t.Fatalf("client.LockBucketRetentionPolicy: %v", err)
		}
		got, err := client.GetBucket(ctx, bucket, nil)
		if err != nil {
			t.Fatalf("client.GetBucket: %v", err)
		}
		if !got.RetentionPolicy.IsLocked {
			t.Error("Expected bucket retention policy to be locked, but was not.")
		}
	})
}

func TestComposeEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		// Populate test data.
		_, err := client.CreateBucket(ctx, project, bucket, &BucketAttrs{
			Name: bucket,
		}, nil)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		prefix := time.Now().Nanosecond()
		srcNames := []string{
			fmt.Sprintf("%d-object1", prefix),
			fmt.Sprintf("%d-object2", prefix),
		}

		for _, n := range srcNames {
			w := veneerClient.Bucket(bucket).Object(n).NewWriter(ctx)
			if _, err := w.Write(randomBytesToWrite); err != nil {
				t.Fatalf("failed to populate test data: %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("closing object: %v", err)
			}
		}

		dstName := fmt.Sprintf("%d-object3", prefix)
		req := composeObjectRequest{
			dstBucket: bucket,
			dstObject: destinationObject{
				name:  dstName,
				attrs: &ObjectAttrs{StorageClass: "COLDLINE"},
			},
			srcs: []sourceObject{
				{name: srcNames[0]},
				{name: srcNames[1]},
			},
		}
		attrs, err := client.ComposeObject(ctx, &req)
		if err != nil {
			t.Fatalf("client.ComposeObject(): %v", err)
		}
		if got := attrs.Name; got != dstName {
			t.Errorf("attrs.Name: got %v, want %v", got, dstName)
		}
		// Check that the destination object size is equal to the sum of its
		// sources.
		if got, want := attrs.Size, 2*len(randomBytesToWrite); got != int64(want) {
			t.Errorf("attrs.Size: got %v, want %v", got, want)
		}
		// Check that destination attrs set via object attrs are preserved.
		if got, want := attrs.StorageClass, "COLDLINE"; got != want {
			t.Errorf("attrs.StorageClass: got %v, want %v", got, want)
		}
	})
}

func TestHMACKeyCRUDEmulated(t *testing.T) {
	transportClientTest(skipGRPC("hmac not implemented"), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		serviceAccountEmail := "test@test-project.iam.gserviceaccount.com"
		want, err := client.CreateHMACKey(ctx, project, serviceAccountEmail)
		if err != nil {
			t.Fatalf("CreateHMACKey: %v", err)
		}
		if want == nil {
			t.Fatal("CreateHMACKey: Unexpectedly got back a nil HMAC key")
		}
		if want.State != Active {
			t.Fatalf("CreateHMACKey: Unexpected state %q, expected %q", want.State, Active)
		}
		got, err := client.GetHMACKey(ctx, project, want.AccessID)
		if err != nil {
			t.Fatalf("GetHMACKey: %v", err)
		}
		if diff := cmp.Diff(got.ID, want.ID); diff != "" {
			t.Errorf("GetHMACKey ID:got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.UpdatedTime, want.UpdatedTime); diff != "" {
			t.Errorf("GetHMACKey UpdatedTime: got(-),want(+):\n%s", diff)
		}
		attr := &HMACKeyAttrsToUpdate{
			State: Inactive,
		}
		got, err = client.UpdateHMACKey(ctx, project, serviceAccountEmail, want.AccessID, attr)
		if err != nil {
			t.Fatalf("UpdateHMACKey: %v", err)
		}
		if got.State != attr.State {
			t.Errorf("UpdateHMACKey State: got %v, want %v", got.State, attr.State)
		}
		showDeletedKeys := false
		it := client.ListHMACKeys(ctx, project, serviceAccountEmail, showDeletedKeys)
		var count int
		var e error
		for ; ; count++ {
			_, e = it.Next()
			if e != nil {
				break
			}
		}
		if e != iterator.Done {
			t.Fatalf("ListHMACKeys: expected %q but got %q", iterator.Done, err)
		}
		if expected := 1; count != expected {
			t.Errorf("ListHMACKeys: expected to get %d hmacKeys, but got %d", expected, count)
		}
		err = client.DeleteHMACKey(ctx, project, want.AccessID)
		if err != nil {
			t.Fatalf("DeleteHMACKey: %v", err)
		}
		got, err = client.GetHMACKey(ctx, project, want.AccessID)
		if err == nil {
			t.Fatalf("GetHMACKey unexcepted error: wanted 404")
		}
	})
}

func TestBucketConditionsEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		cases := []struct {
			name string
			call func(bucket string, metaGen int64) error
		}{
			{
				name: "get",
				call: func(bucket string, metaGen int64) error {
					_, err := client.GetBucket(ctx, bucket, &BucketConditions{MetagenerationMatch: metaGen})
					return err
				},
			},
			{
				name: "update",
				call: func(bucket string, metaGen int64) error {
					_, err := client.UpdateBucket(ctx, bucket, &BucketAttrsToUpdate{StorageClass: "ARCHIVE"}, &BucketConditions{MetagenerationMatch: metaGen})
					return err
				},
			},
			{
				name: "delete",
				call: func(bucket string, metaGen int64) error {
					return client.DeleteBucket(ctx, bucket, &BucketConditions{MetagenerationMatch: metaGen})
				},
			},
			{
				name: "lockRetentionPolicy",
				call: func(bucket string, metaGen int64) error {
					return client.LockBucketRetentionPolicy(ctx, bucket, &BucketConditions{MetagenerationMatch: metaGen})
				},
			},
		}
		for _, c := range cases {
			t.Run(c.name, func(r *testing.T) {
				bucket, metaGen, err := createBucket(ctx, project)
				if err != nil {
					r.Fatalf("creating bucket: %v", err)
				}
				if err := c.call(bucket, metaGen); err != nil {
					r.Errorf("error: %v", err)
				}
			})
		}
	})
}

func TestObjectConditionsEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {

		// Create test bucket
		if _, err := client.CreateBucket(ctx, project, bucket, &BucketAttrs{Name: bucket}, nil); err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}

		cases := []struct {
			name string
			call func() error
		}{
			{
				name: "update generation",
				call: func() error {
					objName, gen, _, err := createObject(ctx, bucket)
					if err != nil {
						return fmt.Errorf("creating object: %w", err)
					}
					uattrs := &ObjectAttrsToUpdate{CustomTime: time.Now()}
					_, err = client.UpdateObject(ctx, &updateObjectParams{bucket: bucket, object: objName, uattrs: uattrs, gen: gen})
					return err
				},
			},
			{
				name: "update ifMetagenerationMatch",
				call: func() error {
					objName, gen, metaGen, err := createObject(ctx, bucket)
					if err != nil {
						return fmt.Errorf("creating object: %w", err)
					}
					uattrs := &ObjectAttrsToUpdate{CustomTime: time.Now()}
					conds := &Conditions{
						GenerationMatch:     gen,
						MetagenerationMatch: metaGen,
					}
					_, err = client.UpdateObject(ctx, &updateObjectParams{bucket: bucket, object: objName, uattrs: uattrs, gen: gen, conds: conds})
					return err
				},
			},
			{
				name: "write ifGenerationMatch",
				call: func() error {
					var err error
					_, err = client.OpenWriter(&openWriterParams{
						ctx:                ctx,
						chunkSize:          256 * 1024,
						chunkRetryDeadline: 0,
						bucket:             bucket,
						attrs:              &ObjectAttrs{},
						conds:              &Conditions{DoesNotExist: true},
						encryptionKey:      nil,
						sendCRC32C:         false,
						donec:              nil,
						setError: func(e error) {
							if e != nil {
								err = e
							}
						},
						progress: nil,
						setObj:   nil,
					})
					return err
				},
			},
			{
				name: "rewrite ifMetagenerationMatch",
				call: func() error {
					objName, gen, metaGen, err := createObject(ctx, bucket)
					if err != nil {
						return fmt.Errorf("creating object: %w", err)
					}
					_, err = client.RewriteObject(ctx, &rewriteObjectRequest{
						srcObject: sourceObject{
							name:   objName,
							bucket: bucket,
							gen:    gen,
							conds: &Conditions{
								GenerationMatch:     gen,
								MetagenerationMatch: metaGen,
							},
						},
						dstObject: destinationObject{
							name:   fmt.Sprintf("%d-object", time.Now().Nanosecond()),
							bucket: bucket,
							conds: &Conditions{
								DoesNotExist: true,
							},
							attrs: &ObjectAttrs{},
						},
					})
					return err
				},
			},
			{
				name: "compose ifGenerationMatch",
				call: func() error {
					obj1, obj1Gen, _, err := createObject(ctx, bucket)
					if err != nil {
						return fmt.Errorf("creating object: %w", err)
					}
					obj2, obj2Gen, _, err := createObject(ctx, bucket)
					if err != nil {
						return fmt.Errorf("creating object: %w", err)
					}
					_, err = client.ComposeObject(ctx, &composeObjectRequest{
						dstBucket: bucket,
						dstObject: destinationObject{
							name:   fmt.Sprintf("%d-object", time.Now().Nanosecond()),
							bucket: bucket,
							conds:  &Conditions{DoesNotExist: true},
							attrs:  &ObjectAttrs{},
						},
						srcs: []sourceObject{
							{
								name:   obj1,
								bucket: bucket,
								gen:    obj1Gen,
								conds: &Conditions{
									GenerationMatch: obj1Gen,
								},
							},
							{
								name:   obj2,
								bucket: bucket,
								conds: &Conditions{
									GenerationMatch: obj2Gen,
								},
							},
						},
					})
					return err
				},
			},
			{
				name: "delete ifGenerationMatch",
				call: func() error {
					objName, gen, _, err := createObject(ctx, bucket)
					if err != nil {
						return fmt.Errorf("creating object: %w", err)
					}
					err = client.DeleteObject(ctx, bucket, objName, gen, &Conditions{GenerationMatch: gen})
					return err
				},
			},
			{
				name: "get ifMetagenerationMatch",
				call: func() error {
					objName, gen, metaGen, err := createObject(ctx, bucket)
					if err != nil {
						return fmt.Errorf("creating object: %w", err)
					}
					_, err = client.GetObject(ctx, &getObjectParams{bucket: bucket, object: objName, gen: gen, conds: &Conditions{GenerationMatch: gen, MetagenerationMatch: metaGen}})
					return err
				},
			},
		}
		for _, c := range cases {
			t.Run(c.name, func(r *testing.T) {
				if err := c.call(); err != nil {
					r.Errorf("error: %v", err)
				}
			})
		}
	})
}

// Test that RetryNever prevents any retries from happening in both transports.
func TestRetryNeverEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		_, err := client.CreateBucket(ctx, project, bucket, &BucketAttrs{}, nil)
		if err != nil {
			t.Fatalf("creating bucket: %v", err)
		}
		instructions := map[string][]string{"storage.buckets.get": {"return-503"}}
		testID := createRetryTest(t, client, instructions)
		ctx = callctx.SetHeaders(ctx, "x-retry-test-id", testID)
		_, err = client.GetBucket(ctx, bucket, nil, withRetryConfig(&retryConfig{policy: RetryNever}))

		var ae *apierror.APIError
		if errors.As(err, &ae) {
			// We expect a 503/UNAVAILABLE error. For anything else including a nil
			// error, the test should fail.
			if ae.GRPCStatus().Code() != codes.Unavailable && ae.HTTPCode() != 503 {
				t.Errorf("GetBucket: got unexpected error %v; want 503", err)
			}
		}
	})
}

// Test that errors are wrapped correctly if retry happens until a timeout.
func TestRetryTimeoutEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		_, err := client.CreateBucket(ctx, project, bucket, &BucketAttrs{}, nil)
		if err != nil {
			t.Fatalf("creating bucket: %v", err)
		}
		instructions := map[string][]string{"storage.buckets.get": {"return-503", "return-503", "return-503", "return-503", "return-503"}}
		testID := createRetryTest(t, client, instructions)
		ctx = callctx.SetHeaders(ctx, "x-retry-test-id", testID)
		ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		_, err = client.GetBucket(ctx, bucket, nil, idempotent(true))

		var ae *apierror.APIError
		if errors.As(err, &ae) {
			// We expect a 503/UNAVAILABLE error. For anything else including a nil
			// error, the test should fail.
			if ae.GRPCStatus().Code() != codes.Unavailable && ae.HTTPCode() != 503 {
				t.Errorf("GetBucket: got unexpected error: %v; want 503", err)
			}
		}
		// Error should be wrapped so it's also equivalent to a context timeout.
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("GetBucket: got unexpected error %v, want to match DeadlineExceeded.", err)
		}
	})
}

// Test that errors are wrapped correctly if retry happens until max attempts.
func TestRetryMaxAttemptsEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		_, err := client.CreateBucket(ctx, project, bucket, &BucketAttrs{}, nil)
		if err != nil {
			t.Fatalf("creating bucket: %v", err)
		}
		instructions := map[string][]string{"storage.buckets.get": {"return-503", "return-503", "return-503", "return-503", "return-503"}}
		testID := createRetryTest(t, client, instructions)
		ctx = callctx.SetHeaders(ctx, "x-retry-test-id", testID)
		config := &retryConfig{maxAttempts: expectedAttempts(3), backoff: &gax.Backoff{Initial: 10 * time.Millisecond}}
		_, err = client.GetBucket(ctx, bucket, nil, idempotent(true), withRetryConfig(config))

		var ae *apierror.APIError
		if errors.As(err, &ae) {
			// We expect a 503/UNAVAILABLE error. For anything else including a nil
			// error, the test should fail.
			if ae.GRPCStatus().Code() != codes.Unavailable && ae.HTTPCode() != 503 {
				t.Errorf("GetBucket: got unexpected error %v; want 503", err)
			}
		}
		// Error should be wrapped so it indicates that MaxAttempts has been reached.
		if got, want := err.Error(), "retry failed after 3 attempts"; !strings.Contains(got, want) {
			t.Errorf("got error: %q, want to contain: %q", got, want)
		}
	})
}

// Test that a timeout returns a DeadlineExceeded error, in spite of DeadlineExceeded being a retryable
// status when it is returned by the server.
func TestTimeoutErrorEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		ctx, cancel := context.WithTimeout(ctx, time.Nanosecond)
		defer cancel()
		time.Sleep(5 * time.Nanosecond)
		config := &retryConfig{backoff: &gax.Backoff{Initial: 10 * time.Millisecond}}
		_, err := client.GetBucket(ctx, bucket, nil, idempotent(true), withRetryConfig(config))

		// Error may come through as a context.DeadlineExceeded (HTTP) or status.DeadlineExceeded (gRPC)
		if !(errors.Is(err, context.DeadlineExceeded) || status.Code(err) == codes.DeadlineExceeded) {
			t.Errorf("GetBucket: got unexpected error %v; want DeadlineExceeded", err)
		}

		// Validate that error was not retried. If it was retried, that will be mentioned
		// in the error string because of wrapping.
		if strings.Contains(err.Error(), "retry") {
			t.Errorf("GetBucket: got error %v, expected non-retried error", err)
		}
	})
}

// Test that server-side DEADLINE_EXCEEDED errors are retried as expected with gRPC.
func TestRetryDeadlineExceedeEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client storageClient) {
		_, err := client.CreateBucket(ctx, project, bucket, &BucketAttrs{}, nil)
		if err != nil {
			t.Fatalf("creating bucket: %v", err)
		}
		instructions := map[string][]string{"storage.buckets.get": {"return-504", "return-504"}}
		testID := createRetryTest(t, client, instructions)
		ctx = callctx.SetHeaders(ctx, "x-retry-test-id", testID)
		config := &retryConfig{maxAttempts: expectedAttempts(4), backoff: &gax.Backoff{Initial: 10 * time.Millisecond}}
		if _, err := client.GetBucket(ctx, bucket, nil, idempotent(true), withRetryConfig(config)); err != nil {
			t.Fatalf("GetBucket: got unexpected error %v, want nil", err)
		}
	})
}

// Test validates the retry for stalled read-request, when client is created with
// WithReadStallTimeout.
func TestRetryReadStallEmulated(t *testing.T) {
	checkEmulatorEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Initialize new storage.Client with ReadStallTimeout option. This cannot be initialized
	// at the transportClient level so we must use NewClient for this test.
	client, err := NewClient(ctx, experimental.WithReadStallTimeout(
		&experimental.ReadStallTimeoutConfig{
			TargetPercentile: 0.99,
			Min:              10 * time.Millisecond,
		}))
	if err != nil {
		t.Fatalf("storage.NewClient: %v", err)
	}

	// Setup bucket and upload object.
	project := "fake-project"
	bucket := fmt.Sprintf("http-bucket-%d", time.Now().Nanosecond())
	if err := client.Bucket(bucket).Create(ctx, project, nil); err != nil {
		t.Fatalf("client.Bucket.Create: %v", err)
	}

	name, _, _, err := createObjectWithContent(ctx, bucket, randomBytes3MiB)
	if err != nil {
		t.Fatalf("createObject: %v", err)
	}

	// Plant stall at start for 10s.
	// The ReadStallTimeout should cause the stalled request to be stopped and
	// retried before hitting the 5s context deadline.
	instructions := map[string][]string{"storage.objects.get": {"stall-for-10s-after-0K"}}
	testID := createRetryTest(t, client.tc, instructions)

	ctx = callctx.SetHeaders(ctx, "x-retry-test-id", testID)
	r, err := client.Bucket(bucket).Object(name).NewReader(ctx)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	defer r.Close()

	buf := &bytes.Buffer{}
	if _, err := io.Copy(buf, r); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), randomBytes3MiB) {
		t.Errorf("content does not match, got len %v, want len %v", buf.Len(), len(randomBytes3MiB))
	}
}

// createRetryTest creates a bucket in the emulator and sets up a test using the
// Retry Test API for the given instructions. This is intended for emulator tests
// of retry behavior that are not covered by conformance tests.
func createRetryTest(t *testing.T, client storageClient, instructions map[string][]string) string {
	t.Helper()

	// Need the HTTP hostname to set up a retry test, as well as knowledge of
	// underlying transport to specify instructions.
	host := os.Getenv("STORAGE_EMULATOR_HOST")
	endpoint, err := url.Parse(host)
	if err != nil {
		t.Fatalf("parsing endpoint: %v", err)
	}
	var transport string
	if _, ok := client.(*httpStorageClient); ok {
		transport = "http"
	} else {
		transport = "grpc"
	}

	et := emulatorTest{T: t, name: t.Name(), resources: resources{}, host: endpoint}
	et.create(instructions, transport)
	t.Cleanup(func() {
		et.delete()
	})
	return et.id
}

// createObject creates an object in the emulator with content randomBytesToWrite and
// returns its name, generation, and metageneration.
func createObject(ctx context.Context, bucket string) (string, int64, int64, error) {
	return createObjectWithContent(ctx, bucket, randomBytesToWrite)
}

// createObject creates an object in the emulator with the provided []byte contents,
// and returns its name, generation, and metageneration.
func createObjectWithContent(ctx context.Context, bucket string, bytes []byte) (string, int64, int64, error) {
	prefix := time.Now().Nanosecond()
	objName := fmt.Sprintf("%d-object", prefix)

	w := veneerClient.Bucket(bucket).Object(objName).NewWriter(ctx)
	if _, err := w.Write(bytes); err != nil {
		return "", 0, 0, fmt.Errorf("failed to populate test data: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", 0, 0, fmt.Errorf("closing object: %w", err)
	}
	attrs, err := veneerClient.Bucket(bucket).Object(objName).Attrs(ctx)
	if err != nil {
		return "", 0, 0, fmt.Errorf("get object: %w", err)
	}
	return objName, attrs.Generation, attrs.Metageneration, nil
}

// createBucket creates a new bucket in the emulator and returns its name and
// metageneration.
func createBucket(ctx context.Context, projectID string) (string, int64, error) {
	prefix := time.Now().Nanosecond()
	bucket := fmt.Sprintf("%d-bucket", prefix)

	if err := veneerClient.Bucket(bucket).Create(ctx, projectID, nil); err != nil {
		return "", 0, fmt.Errorf("Bucket.Create: %w", err)
	}
	attrs, err := veneerClient.Bucket(bucket).Attrs(ctx)
	if err != nil {
		return "", 0, fmt.Errorf("Bucket.Attrs: %w", err)
	}
	return bucket, attrs.MetaGeneration, nil
}

// transportClienttest executes the given function with a sub-test, a project name
// based on the transport, a unique bucket name also based on the transport, and
// the transport-specific client to run the test with. It also checks the environment
// to ensure it is suitable for emulator-based tests, or skips.
func transportClientTest(ctx context.Context, t *testing.T, test func(*testing.T, context.Context, string, string, storageClient)) {
	checkEmulatorEnvironment(t)

	for transport, client := range emulatorClients {
		t.Run(transport, func(t *testing.T) {
			if reason := ctx.Value(skipTransportTestKey(transport)); reason != nil {
				t.Skip("transport", fmt.Sprintf("%q", transport), "explicitly skipped:", reason)
			}
			project := fmt.Sprintf("%s-project", transport)
			bucket := fmt.Sprintf("%s-bucket-%d", transport, time.Now().Nanosecond())
			test(t, ctx, project, bucket, client)
		})
	}
}

// checkEmulatorEnvironment skips the test if the emulator environment variables
// are not set.
func checkEmulatorEnvironment(t *testing.T) {
	if !isEmulatorEnvironmentSet() {
		t.Skip("Emulator tests skipped without emulator environment variables set")
	}
}

// isEmulatorEnvironmentSet checks if the emulator environment variables are set.
func isEmulatorEnvironmentSet() bool {
	return os.Getenv("STORAGE_EMULATOR_HOST_GRPC") != "" && os.Getenv("STORAGE_EMULATOR_HOST") != ""
}
