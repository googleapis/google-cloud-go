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
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/iterator"
	iampb "google.golang.org/genproto/googleapis/iam/v1"
)

var emulatorClients map[string]storageClient
var veneerClient *Client

func TestCreateBucketEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		want := &BucketAttrs{
			Name: bucket,
		}
		got, err := client.CreateBucket(context.Background(), project, want)
		if err != nil {
			t.Fatal(err)
		}
		want.Location = "US"
		if diff := cmp.Diff(got.Name, want.Name); diff != "" {
			t.Errorf("got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.Location, want.Location); diff != "" {
			t.Errorf("got(-),want(+):\n%s", diff)
		}
	})
}

func TestDeleteBucketEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		b := &BucketAttrs{
			Name: bucket,
		}
		// Create the bucket that will be deleted.
		_, err := client.CreateBucket(context.Background(), project, b)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		// Delete the bucket that was just created.
		err = client.DeleteBucket(context.Background(), b.Name, nil)
		if err != nil {
			t.Fatalf("client.DeleteBucket: %v", err)
		}
	})
}

func TestGetBucketEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		want := &BucketAttrs{
			Name: bucket,
		}
		// Create the bucket that will be retrieved.
		_, err := client.CreateBucket(context.Background(), project, want)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		got, err := client.GetBucket(context.Background(), want.Name, &BucketConditions{MetagenerationMatch: 1})
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(got.Name, want.Name); diff != "" {
			t.Errorf("got(-),want(+):\n%s", diff)
		}
	})
}

func TestUpdateBucketEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		bkt := &BucketAttrs{
			Name: bucket,
		}
		// Create the bucket that will be updated.
		_, err := client.CreateBucket(context.Background(), project, bkt)
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

		got, err := client.UpdateBucket(context.Background(), bucket, ua, &BucketConditions{MetagenerationMatch: 1})
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
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		_, err := client.GetServiceAccount(context.Background(), project)
		if err != nil {
			t.Fatalf("client.GetServiceAccount: %v", err)
		}
	})
}

func TestGetSetTestIamPolicyEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		battrs, err := client.CreateBucket(context.Background(), project, &BucketAttrs{
			Name: bucket,
		})
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		got, err := client.GetIamPolicy(context.Background(), battrs.Name, 0)
		if err != nil {
			t.Fatalf("client.GetIamPolicy: %v", err)
		}
		err = client.SetIamPolicy(context.Background(), battrs.Name, &iampb.Policy{
			Etag:     got.GetEtag(),
			Bindings: []*iampb.Binding{{Role: "roles/viewer", Members: []string{"allUsers"}}},
		})
		if err != nil {
			t.Fatalf("client.SetIamPolicy: %v", err)
		}
		want := []string{"storage.foo", "storage.bar"}
		perms, err := client.TestIamPermissions(context.Background(), battrs.Name, want)
		if err != nil {
			t.Fatalf("client.TestIamPermissions: %v", err)
		}
		if diff := cmp.Diff(perms, want); diff != "" {
			t.Errorf("got(-),want(+):\n%s", diff)
		}
	})
}

func TestDeleteObjectEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		// Populate test object that will be deleted.
		_, err := client.CreateBucket(context.Background(), project, &BucketAttrs{
			Name: bucket,
		})
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		want := ObjectAttrs{
			Bucket: bucket,
			Name:   fmt.Sprintf("testObject-%d", time.Now().Nanosecond()),
		}
		w := veneerClient.Bucket(bucket).Object(want.Name).NewWriter(context.Background())
		if _, err := w.Write(randomBytesToWrite); err != nil {
			t.Fatalf("failed to populate test object: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("closing object: %v", err)
		}
		err = client.DeleteObject(context.Background(), bucket, want.Name, defaultGen, nil)
		if err != nil {
			t.Fatalf("client.DeleteBucket: %v", err)
		}
	})
}

func TestGetObjectEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		// Populate test object.
		_, err := client.CreateBucket(context.Background(), project, &BucketAttrs{
			Name: bucket,
		})
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		want := ObjectAttrs{
			Bucket: bucket,
			Name:   fmt.Sprintf("testObject-%d", time.Now().Nanosecond()),
		}
		w := veneerClient.Bucket(bucket).Object(want.Name).NewWriter(context.Background())
		if _, err := w.Write(randomBytesToWrite); err != nil {
			t.Fatalf("failed to populate test object: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("closing object: %v", err)
		}
		got, err := client.GetObject(context.Background(), bucket, want.Name, defaultGen, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(got.Name, want.Name); diff != "" {
			t.Errorf("got(-),want(+):\n%s", diff)
		}
	})
}

func TestUpdateObjectEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		// Populate test object.
		_, err := client.CreateBucket(context.Background(), project, &BucketAttrs{
			Name: bucket,
		})
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		ct := time.Date(2022, 5, 25, 12, 12, 12, 0, time.UTC)
		o := ObjectAttrs{
			Bucket:     bucket,
			Name:       fmt.Sprintf("testObject-%d", time.Now().Nanosecond()),
			CustomTime: ct,
		}
		w := veneerClient.Bucket(bucket).Object(o.Name).NewWriter(context.Background())
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

		got, err := client.UpdateObject(context.Background(), bucket, o.Name, want, defaultGen, nil, &Conditions{MetagenerationMatch: 1})
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
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		// Populate test data.
		_, err := client.CreateBucket(context.Background(), project, &BucketAttrs{
			Name: bucket,
		})
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
			w := veneerClient.Bucket(bucket).Object(obj.Name).NewWriter(context.Background())
			if _, err := w.Write(randomBytesToWrite); err != nil {
				t.Fatalf("failed to populate test data: %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("closing object: %v", err)
			}
		}

		// Simple list, no query.
		it := client.ListObjects(context.Background(), bucket, nil)
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
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		// Populate test data.
		_, err := client.CreateBucket(context.Background(), project, &BucketAttrs{
			Name: bucket,
		})
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
			w := veneerClient.Bucket(bucket).Object(obj.Name).NewWriter(context.Background())
			if _, err := w.Write(randomBytesToWrite); err != nil {
				t.Fatalf("failed to populate test data: %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("closing object: %v", err)
			}
		}

		// Query with Prefix.
		it := client.ListObjects(context.Background(), bucket, &Query{Prefix: strconv.Itoa(prefix)})
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
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		prefix := time.Now().Nanosecond()
		want := []*BucketAttrs{
			{Name: fmt.Sprintf("%d-%s-%d", prefix, bucket, time.Now().Nanosecond())},
			{Name: fmt.Sprintf("%d-%s-%d", prefix, bucket, time.Now().Nanosecond())},
			{Name: fmt.Sprintf("%s-%d", bucket, time.Now().Nanosecond())},
		}
		// Create the buckets that will be listed.
		for _, b := range want {
			_, err := client.CreateBucket(context.Background(), project, b)
			if err != nil {
				t.Fatalf("client.CreateBucket: %v", err)
			}
		}

		it := client.ListBuckets(context.Background(), project)
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
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		ctx := context.Background()
		attrs := &BucketAttrs{
			Name:          bucket,
			PredefinedACL: "publicRead",
		}
		// Create the bucket that will be retrieved.
		if _, err := client.CreateBucket(ctx, project, attrs); err != nil {
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

func TestListDefaultObjectACLsEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		ctx := context.Background()
		attrs := &BucketAttrs{
			Name:                       bucket,
			PredefinedDefaultObjectACL: "publicRead",
		}
		// Create the bucket that will be retrieved.
		if _, err := client.CreateBucket(ctx, project, attrs); err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}

		acls, err := client.ListDefaultObjectACLs(ctx, bucket)
		if err != nil {
			t.Fatalf("client.ListDefaultObjectACLs: %v", err)
		}
		if want, got := len(acls), 2; want != got {
			t.Errorf("ListDefaultObjectACLs: got %v, want %v items", acls, want)
		}
	})
}

func TestUpdateBucketACLEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		ctx := context.Background()
		attrs := &BucketAttrs{
			Name:          bucket,
			PredefinedACL: "authenticatedRead",
		}
		// Create the bucket that will be retrieved.
		if _, err := client.CreateBucket(ctx, project, attrs); err != nil {
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
		want := &ACLRule{Entity: entity, Role: role}
		got, err := client.UpdateBucketACL(ctx, bucket, entity, role)
		if err != nil {
			t.Fatalf("client.UpdateBucketACL: %v", err)
		}
		if diff := cmp.Diff(got.Entity, want.Entity); diff != "" {
			t.Errorf("got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.Role, want.Role); diff != "" {
			t.Errorf("got(-),want(+):\n%s", diff)
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
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		ctx := context.Background()
		attrs := &BucketAttrs{
			Name:          bucket,
			PredefinedACL: "publicRead",
		}
		// Create the bucket that will be retrieved.
		if _, err := client.CreateBucket(ctx, project, attrs); err != nil {
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

func TestDeleteDefaultObjectACLEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		ctx := context.Background()
		attrs := &BucketAttrs{
			Name:                       bucket,
			PredefinedDefaultObjectACL: "publicRead",
		}
		// Create the bucket that will be retrieved.
		if _, err := client.CreateBucket(ctx, project, attrs); err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		// Assert bucket has two DefaultObjectACL entities, including project owner and PredefinedDefaultObjectACL.
		acls, err := client.ListDefaultObjectACLs(ctx, bucket)
		if err != nil {
			t.Fatalf("client.ListDefaultObjectACLs: %v", err)
		}
		if got, want := len(acls), 2; got != want {
			t.Errorf("ListDefaultObjectACLs: got %v, want %v items", acls, want)
		}
		// Delete one DefaultObjectACL with AllUsers entity.
		if err := client.DeleteDefaultObjectACL(ctx, bucket, AllUsers); err != nil {
			t.Fatalf("client.DeleteDefaultObjectACL: %v", err)
		}
		// Assert bucket has one DefaultObjectACL entity.
		acls, err = client.ListDefaultObjectACLs(ctx, bucket)
		if err != nil {
			t.Fatalf("client.ListDefaultObjectACLs: %v", err)
		}
		if got, want := len(acls), 1; got != want {
			t.Errorf("ListDefaultObjectACLs: %v got %v, want %v items", len(acls), acls, want)
		}
	})
}

func TestOpenReaderEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		// Populate test data.
		_, err := client.CreateBucket(context.Background(), project, &BucketAttrs{
			Name: bucket,
		})
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		prefix := time.Now().Nanosecond()
		want := &ObjectAttrs{
			Bucket: bucket,
			Name:   fmt.Sprintf("%d-object-%d", prefix, time.Now().Nanosecond()),
		}
		w := veneerClient.Bucket(bucket).Object(want.Name).NewWriter(context.Background())
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
		r, err := client.NewRangeReader(context.Background(), params)
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
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		b := &BucketAttrs{
			Name: bucket,
			RetentionPolicy: &RetentionPolicy{
				RetentionPeriod: time.Minute,
			},
		}
		// Create the bucket that will be locked.
		_, err := client.CreateBucket(context.Background(), project, b)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		// Lock the bucket's retention policy.
		err = client.LockBucketRetentionPolicy(context.Background(), b.Name, &BucketConditions{MetagenerationMatch: 1})
		if err != nil {
			t.Fatalf("client.LockBucketRetentionPolicy: %v", err)
		}
		got, err := client.GetBucket(context.Background(), bucket, nil)
		if err != nil {
			t.Fatalf("client.GetBucket: %v", err)
		}
		if !got.RetentionPolicy.IsLocked {
			t.Error("Expected bucket retention policy to be locked, but was not.")
		}
	})
}

// transportClienttest executes the given function with a sub-test, a project name
// based on the transport, a unique bucket name also based on the transport, and
// the transport-specific client to run the test with. It also checks the environment
// to ensure it is suitable for emulator-based tests, or skips.
func transportClientTest(t *testing.T, test func(*testing.T, string, string, storageClient)) {
	checkEmulatorEnvironment(t)

	for transport, client := range emulatorClients {
		t.Run(transport, func(t *testing.T) {
			project := fmt.Sprintf("%s-project", transport)
			bucket := fmt.Sprintf("%s-bucket-%d", transport, time.Now().Nanosecond())
			test(t, project, bucket, client)
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
