// Copyright 2014 Google LLC
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
	"compress/gzip"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"hash/crc32"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/httpreplay"
	"cloud.google.com/go/iam"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	itesting "google.golang.org/api/iterator/testing"
	"google.golang.org/api/option"
	iampb "google.golang.org/genproto/googleapis/iam/v1"
)

const (
	testPrefix     = "go-integration-test"
	replayFilename = "storage.replay"
	// TODO(jba): move to testutil, factor out from firestore/integration_test.go.
	envFirestoreProjID     = "GCLOUD_TESTS_GOLANG_FIRESTORE_PROJECT_ID"
	envFirestorePrivateKey = "GCLOUD_TESTS_GOLANG_FIRESTORE_KEY"
)

var (
	record = flag.Bool("record", false, "record RPCs")

	uidSpace   *uid.Space
	bucketName string
	// Use our own random number generator to isolate the sequence of random numbers from
	// other packages. This makes it possible to use HTTP replay and draw the same sequence
	// of numbers as during recording.
	rng           *rand.Rand
	newTestClient func(ctx context.Context, opts ...option.ClientOption) (*Client, error)
	replaying     bool
	testTime      time.Time
)

func TestMain(m *testing.M) {
	cleanup := initIntegrationTest()
	exit := m.Run()
	if err := cleanup(); err != nil {
		// Don't fail the test if cleanup fails.
		log.Printf("Post-test cleanup failed: %v", err)
	}
	os.Exit(exit)
}

// If integration tests will be run, create a unique bucket for them.
// Also, set newTestClient to handle record/replay.
// Return a cleanup function.
func initIntegrationTest() func() error {
	flag.Parse() // needed for testing.Short()
	switch {
	case testing.Short() && *record:
		log.Fatal("cannot combine -short and -record")
		return nil

	case testing.Short() && httpreplay.Supported() && testutil.CanReplay(replayFilename) && testutil.ProjID() != "":
		// go test -short with a replay file will replay the integration tests, if
		// the appropriate environment variables have been set.
		replaying = true
		httpreplay.DebugHeaders()
		replayer, err := httpreplay.NewReplayer(replayFilename)
		if err != nil {
			log.Fatal(err)
		}
		var t time.Time
		if err := json.Unmarshal(replayer.Initial(), &t); err != nil {
			log.Fatal(err)
		}
		initUIDsAndRand(t)
		newTestClient = func(ctx context.Context, _ ...option.ClientOption) (*Client, error) {
			hc, err := replayer.Client(ctx) // no creds needed
			if err != nil {
				return nil, err
			}
			return NewClient(ctx, option.WithHTTPClient(hc))
		}
		log.Printf("replaying from %s", replayFilename)
		return func() error { return replayer.Close() }

	case testing.Short():
		// go test -short without a replay file skips the integration tests.
		if testutil.CanReplay(replayFilename) && testutil.ProjID() != "" {
			log.Print("replay not supported for Go versions before 1.8")
		}
		newTestClient = nil
		return func() error { return nil }

	default: // Run integration tests against a real backend.
		now := time.Now().UTC()
		initUIDsAndRand(now)
		var cleanup func() error
		if *record && httpreplay.Supported() {
			// Remember the time for replay.
			nowBytes, err := json.Marshal(now)
			if err != nil {
				log.Fatal(err)
			}
			recorder, err := httpreplay.NewRecorder(replayFilename, nowBytes)
			if err != nil {
				log.Fatalf("could not record: %v", err)
			}
			newTestClient = func(ctx context.Context, opts ...option.ClientOption) (*Client, error) {
				hc, err := recorder.Client(ctx, opts...)
				if err != nil {
					return nil, err
				}
				return NewClient(ctx, option.WithHTTPClient(hc))
			}
			cleanup = func() error {
				err1 := cleanupBuckets()
				err2 := recorder.Close()
				if err1 != nil {
					return err1
				}
				return err2
			}
			log.Printf("recording to %s", replayFilename)
		} else {
			if *record {
				log.Print("record not supported for Go versions before 1.8")
			}
			newTestClient = NewClient
			cleanup = cleanupBuckets
		}
		ctx := context.Background()
		client := config(ctx)
		if client == nil {
			return func() error { return nil }
		}
		defer client.Close()
		if err := client.Bucket(bucketName).Create(ctx, testutil.ProjID(), nil); err != nil {
			log.Fatalf("creating bucket %q: %v", bucketName, err)
		}
		return cleanup
	}
}

func initUIDsAndRand(t time.Time) {
	uidSpace = uid.NewSpace(testPrefix, &uid.Options{Time: t})
	bucketName = uidSpace.New()
	// Use our own random source, to avoid other parts of the program taking
	// random numbers from the global source and putting record and replay
	// out of sync.
	rng = testutil.NewRand(t)
	testTime = t
}

// testConfig returns the Client used to access GCS. testConfig skips
// the current test if credentials are not available or when being run
// in Short mode.
func testConfig(ctx context.Context, t *testing.T) *Client {
	if testing.Short() && !replaying {
		t.Skip("Integration tests skipped in short mode")
	}
	client := config(ctx)
	if client == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	return client
}

// config is like testConfig, but it doesn't need a *testing.T.
func config(ctx context.Context) *Client {
	ts := testutil.TokenSource(ctx, ScopeFullControl)
	if ts == nil {
		return nil
	}
	client, err := newTestClient(ctx, option.WithTokenSource(ts))
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}
	return client
}

func TestIntegration_BucketMethods(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	projectID := testutil.ProjID()
	newBucketName := uidSpace.New()
	b := client.Bucket(newBucketName)
	// Test Create and Delete.
	h.mustCreate(b, projectID, nil)
	attrs := h.mustBucketAttrs(b)
	if got, want := attrs.MetaGeneration, int64(1); got != want {
		t.Errorf("got metagen %d, want %d", got, want)
	}
	if got, want := attrs.StorageClass, "STANDARD"; got != want {
		t.Errorf("got storage class %q, want %q", got, want)
	}
	if attrs.VersioningEnabled {
		t.Error("got versioning enabled, wanted it disabled")
	}
	if attrs.LocationType == "" {
		t.Error("got an empty LocationType")
	}
	h.mustDeleteBucket(b)

	// Test Create and Delete with attributes.
	labels := map[string]string{
		"l1":    "v1",
		"empty": "",
	}
	attrs = &BucketAttrs{
		StorageClass:      "NEARLINE",
		VersioningEnabled: true,
		Labels:            labels,
		Lifecycle: Lifecycle{
			Rules: []LifecycleRule{{
				Action: LifecycleAction{
					Type:         SetStorageClassAction,
					StorageClass: "NEARLINE",
				},
				Condition: LifecycleCondition{
					AgeInDays:             10,
					Liveness:              Archived,
					CreatedBefore:         time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC),
					MatchesStorageClasses: []string{"STANDARD"},
					NumNewerVersions:      3,
				},
			}, {
				Action: LifecycleAction{
					Type:         SetStorageClassAction,
					StorageClass: "ARCHIVE",
				},
				Condition: LifecycleCondition{
					CustomTimeBefore:      time.Date(2020, 1, 2, 3, 0, 0, 0, time.UTC),
					DaysSinceCustomTime:   20,
					Liveness:              Live,
					MatchesStorageClasses: []string{"STANDARD"},
				},
			}, {
				Action: LifecycleAction{
					Type: DeleteAction,
				},
				Condition: LifecycleCondition{
					DaysSinceNoncurrentTime: 30,
					Liveness:                Live,
					NoncurrentTimeBefore:    time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC),
					MatchesStorageClasses:   []string{"NEARLINE"},
					NumNewerVersions:        10,
				},
			}},
		},
	}
	h.mustCreate(b, projectID, attrs)
	attrs = h.mustBucketAttrs(b)
	if got, want := attrs.MetaGeneration, int64(1); got != want {
		t.Errorf("got metagen %d, want %d", got, want)
	}
	if got, want := attrs.StorageClass, "NEARLINE"; got != want {
		t.Errorf("got storage class %q, want %q", got, want)
	}
	if !attrs.VersioningEnabled {
		t.Error("got versioning disabled, wanted it enabled")
	}
	if got, want := attrs.Labels, labels; !testutil.Equal(got, want) {
		t.Errorf("labels: got %v, want %v", got, want)
	}
	if attrs.LocationType == "" {
		t.Error("got an empty LocationType")
	}
	h.mustDeleteBucket(b)
}

func TestIntegration_BucketUpdate(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	b := client.Bucket(uidSpace.New())
	h.mustCreate(b, testutil.ProjID(), nil)
	defer h.mustDeleteBucket(b)

	attrs := h.mustBucketAttrs(b)
	if attrs.VersioningEnabled {
		t.Fatal("bucket should not have versioning by default")
	}
	if len(attrs.Labels) > 0 {
		t.Fatal("bucket should not have labels initially")
	}

	// Using empty BucketAttrsToUpdate should be a no-nop.
	attrs = h.mustUpdateBucket(b, BucketAttrsToUpdate{})
	if attrs.VersioningEnabled {
		t.Fatal("should not have versioning")
	}
	if len(attrs.Labels) > 0 {
		t.Fatal("should not have labels")
	}

	// Turn on versioning, add some labels.
	ua := BucketAttrsToUpdate{VersioningEnabled: true}
	ua.SetLabel("l1", "v1")
	ua.SetLabel("empty", "")
	attrs = h.mustUpdateBucket(b, ua)
	if !attrs.VersioningEnabled {
		t.Fatal("should have versioning now")
	}
	wantLabels := map[string]string{
		"l1":    "v1",
		"empty": "",
	}
	if !testutil.Equal(attrs.Labels, wantLabels) {
		t.Fatalf("got %v, want %v", attrs.Labels, wantLabels)
	}

	// Turn off versioning again; add and remove some more labels.
	ua = BucketAttrsToUpdate{VersioningEnabled: false}
	ua.SetLabel("l1", "v2")   // update
	ua.SetLabel("new", "new") // create
	ua.DeleteLabel("empty")   // delete
	ua.DeleteLabel("absent")  // delete non-existent
	attrs = h.mustUpdateBucket(b, ua)
	if attrs.VersioningEnabled {
		t.Fatal("should have versioning off")
	}
	wantLabels = map[string]string{
		"l1":  "v2",
		"new": "new",
	}
	if !testutil.Equal(attrs.Labels, wantLabels) {
		t.Fatalf("got %v, want %v", attrs.Labels, wantLabels)
	}

	// Configure a lifecycle
	wantLifecycle := Lifecycle{
		Rules: []LifecycleRule{
			{
				Action:    LifecycleAction{Type: "Delete"},
				Condition: LifecycleCondition{AgeInDays: 30},
			},
		},
	}
	ua = BucketAttrsToUpdate{Lifecycle: &wantLifecycle}
	attrs = h.mustUpdateBucket(b, ua)
	if !testutil.Equal(attrs.Lifecycle, wantLifecycle) {
		t.Fatalf("got %v, want %v", attrs.Lifecycle, wantLifecycle)
	}
	// Check that StorageClass has "STANDARD" value for unset field by default
	// before passing new value.
	wantStorageClass := "STANDARD"
	if !testutil.Equal(attrs.StorageClass, wantStorageClass) {
		t.Fatalf("got %v, want %v", attrs.StorageClass, wantStorageClass)
	}
	wantStorageClass = "NEARLINE"
	ua = BucketAttrsToUpdate{StorageClass: wantStorageClass}
	attrs = h.mustUpdateBucket(b, ua)
	if !testutil.Equal(attrs.StorageClass, wantStorageClass) {
		t.Fatalf("got %v, want %v", attrs.StorageClass, wantStorageClass)
	}
}

func TestIntegration_BucketPolicyOnly(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}
	bkt := client.Bucket(bucketName)

	// Insert an object with custom ACL.
	o := bkt.Object("bucketPolicyOnly")
	defer func() {
		if err := o.Delete(ctx); err != nil {
			log.Printf("failed to delete test object: %v", err)
		}
	}()
	wc := o.NewWriter(ctx)
	wc.ContentType = "text/plain"
	h.mustWrite(wc, []byte("test"))
	a := o.ACL()
	aclEntity := ACLEntity("user-test@example.com")
	err := a.Set(ctx, aclEntity, RoleReader)
	if err != nil {
		t.Fatalf("set ACL failed: %v", err)
	}

	// Enable BucketPolicyOnly.
	ua := BucketAttrsToUpdate{BucketPolicyOnly: &BucketPolicyOnly{Enabled: true}}
	attrs := h.mustUpdateBucket(bkt, ua)
	if got, want := attrs.BucketPolicyOnly.Enabled, true; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := attrs.BucketPolicyOnly.LockedTime; got.IsZero() {
		t.Fatal("got a zero time value, want a populated value")
	}

	// Confirm BucketAccessControl returns error.
	err = retry(ctx, func() error {
		_, err = bkt.ACL().List(ctx)
		return nil
	}, func() error {
		if err == nil {
			return fmt.Errorf("ACL.List: expected bucket ACL list to fail")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Confirm ObjectAccessControl returns error.
	err = retry(ctx, func() error {
		_, err = o.ACL().List(ctx)
		return nil
	}, func() error {
		if err == nil {
			return fmt.Errorf("ACL.List: expected object ACL list to fail")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Disable BucketPolicyOnly.
	ua = BucketAttrsToUpdate{BucketPolicyOnly: &BucketPolicyOnly{Enabled: false}}
	attrs = h.mustUpdateBucket(bkt, ua)
	if got, want := attrs.BucketPolicyOnly.Enabled, false; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}

	// Check that the object ACLs are the same.
	var acls []ACLRule
	err = retry(ctx, func() error {
		acls, err = o.ACL().List(ctx)
		if err != nil {
			return fmt.Errorf("ACL.List: object ACL list failed: %v", err)
		}
		return nil
	}, func() error {
		if !containsACL(acls, aclEntity, RoleReader) {
			return fmt.Errorf("containsACL: expected ACLs %v to include custom ACL entity %v", acls, aclEntity)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestIntegration_UniformBucketLevelAccess(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}
	bkt := client.Bucket(uidSpace.New())
	h.mustCreate(bkt, testutil.ProjID(), nil)
	defer h.mustDeleteBucket(bkt)

	// Insert an object with custom ACL.
	o := bkt.Object("uniformBucketLevelAccess")
	defer func() {
		if err := o.Delete(ctx); err != nil {
			log.Printf("failed to delete test object: %v", err)
		}
	}()
	wc := o.NewWriter(ctx)
	wc.ContentType = "text/plain"
	h.mustWrite(wc, []byte("test"))
	a := o.ACL()
	aclEntity := ACLEntity("user-test@example.com")
	err := a.Set(ctx, aclEntity, RoleReader)
	if err != nil {
		t.Fatalf("set ACL failed: %v", err)
	}

	// Enable UniformBucketLevelAccess.
	ua := BucketAttrsToUpdate{UniformBucketLevelAccess: &UniformBucketLevelAccess{Enabled: true}}
	attrs := h.mustUpdateBucket(bkt, ua)
	if got, want := attrs.UniformBucketLevelAccess.Enabled, true; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := attrs.UniformBucketLevelAccess.LockedTime; got.IsZero() {
		t.Fatal("got a zero time value, want a populated value")
	}

	// Confirm BucketAccessControl returns error.
	err = retry(ctx, func() error {
		_, err = bkt.ACL().List(ctx)
		return nil
	}, func() error {
		if err == nil {
			return fmt.Errorf("ACL.List: expected bucket ACL list to fail")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Confirm ObjectAccessControl returns error.
	err = retry(ctx, func() error {
		_, err = o.ACL().List(ctx)
		return nil
	}, func() error {
		if err == nil {
			return fmt.Errorf("ACL.List: expected object ACL list to fail")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Disable UniformBucketLevelAccess.
	ua = BucketAttrsToUpdate{UniformBucketLevelAccess: &UniformBucketLevelAccess{Enabled: false}}
	attrs = h.mustUpdateBucket(bkt, ua)
	if got, want := attrs.UniformBucketLevelAccess.Enabled, false; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}

	// Check that the object ACLs are the same.
	var acls []ACLRule
	err = retry(ctx, func() error {
		acls, err = o.ACL().List(ctx)
		if err != nil {
			return fmt.Errorf("ACL.List: object ACL list failed: %v", err)
		}
		return nil
	}, func() error {
		if !containsACL(acls, aclEntity, RoleReader) {
			return fmt.Errorf("containsACL: expected ACLs %v to include custom ACL entity %v", acls, aclEntity)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestIntegration_PublicAccessPrevention(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	// Create a bucket with PublicAccessPrevention enforced.
	bkt := client.Bucket(uidSpace.New())
	h.mustCreate(bkt, testutil.ProjID(), &BucketAttrs{PublicAccessPrevention: PublicAccessPreventionEnforced})
	defer h.mustDeleteBucket(bkt)

	// Making bucket public should fail.
	policy, err := bkt.IAM().V3().Policy(ctx)
	if err != nil {
		t.Fatalf("fetching bucket IAM policy: %v", err)
	}
	policy.Bindings = append(policy.Bindings, &iampb.Binding{
		Role:    "roles/storage.objectViewer",
		Members: []string{iam.AllUsers},
	})
	if err := bkt.IAM().V3().SetPolicy(ctx, policy); err == nil {
		t.Error("SetPolicy: expected adding AllUsers policy to bucket should fail")
	}

	// Making object public via ACL should fail.
	o := bkt.Object("publicAccessPrevention")
	defer func() {
		if err := o.Delete(ctx); err != nil {
			log.Printf("failed to delete test object: %v", err)
		}
	}()
	wc := o.NewWriter(ctx)
	wc.ContentType = "text/plain"
	h.mustWrite(wc, []byte("test"))
	a := o.ACL()
	if err := a.Set(ctx, AllUsers, RoleReader); err == nil {
		t.Error("ACL.Set: expected adding AllUsers ACL to object should fail")
	}

	// Update PAP setting to unspecified should work and not affect UBLA setting.
	attrs, err := bkt.Update(ctx, BucketAttrsToUpdate{PublicAccessPrevention: PublicAccessPreventionUnspecified})
	if err != nil {
		t.Fatalf("updating PublicAccessPrevention failed: %v", err)
	}
	if attrs.PublicAccessPrevention != PublicAccessPreventionUnspecified {
		t.Errorf("updating PublicAccessPrevention: got %s, want %s", attrs.PublicAccessPrevention, PublicAccessPreventionUnspecified)
	}
	if attrs.UniformBucketLevelAccess.Enabled || attrs.BucketPolicyOnly.Enabled {
		t.Error("updating PublicAccessPrevention changed UBLA setting")
	}

	// Now, making object public or making bucket public should succeed.
	a = o.ACL()
	if err := a.Set(ctx, AllUsers, RoleReader); err != nil {
		t.Errorf("ACL.Set: making object public failed: %v", err)
	}
	policy, err = bkt.IAM().V3().Policy(ctx)
	if err != nil {
		t.Fatalf("fetching bucket IAM policy: %v", err)
	}
	policy.Bindings = append(policy.Bindings, &iampb.Binding{
		Role:    "roles/storage.objectViewer",
		Members: []string{iam.AllUsers},
	})
	if err := bkt.IAM().V3().SetPolicy(ctx, policy); err != nil {
		t.Errorf("SetPolicy: making bucket public failed: %v", err)
	}

	// Updating UBLA should not affect PAP setting.
	attrs, err = bkt.Update(ctx, BucketAttrsToUpdate{UniformBucketLevelAccess: &UniformBucketLevelAccess{Enabled: true}})
	if err != nil {
		t.Fatalf("updating UBLA failed: %v", err)
	}
	if !attrs.UniformBucketLevelAccess.Enabled {
		t.Error("updating UBLA: got UBLA not enabled, want enabled")
	}
	if attrs.PublicAccessPrevention != PublicAccessPreventionUnspecified {
		t.Errorf("updating UBLA: got %s, want %s", attrs.PublicAccessPrevention, PublicAccessPreventionUnspecified)
	}
}

func TestIntegration_ConditionalDelete(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	o := client.Bucket(bucketName).Object("conddel")

	wc := o.NewWriter(ctx)
	wc.ContentType = "text/plain"
	h.mustWrite(wc, []byte("foo"))

	gen := wc.Attrs().Generation
	metaGen := wc.Attrs().Metageneration

	if err := o.Generation(gen - 1).Delete(ctx); err == nil {
		t.Fatalf("Unexpected successful delete with Generation")
	}
	if err := o.If(Conditions{MetagenerationMatch: metaGen + 1}).Delete(ctx); err == nil {
		t.Fatalf("Unexpected successful delete with IfMetaGenerationMatch")
	}
	if err := o.If(Conditions{MetagenerationNotMatch: metaGen}).Delete(ctx); err == nil {
		t.Fatalf("Unexpected successful delete with IfMetaGenerationNotMatch")
	}
	if err := o.Generation(gen).Delete(ctx); err != nil {
		t.Fatalf("final delete failed: %v", err)
	}
}

func TestIntegration_ObjectsRangeReader(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	bkt := client.Bucket(bucketName)

	objName := uidSpace.New()
	obj := bkt.Object(objName)
	contents := []byte("Hello, world this is a range request")

	if err := retry(ctx, func() error {
		w := obj.NewWriter(ctx)
		if _, err := w.Write(contents); err != nil {
			return fmt.Errorf("Failed to write contents: %v", err)
		}
		if err := w.Close(); err != nil {
			return fmt.Errorf("Failed to close writer: %v", err)
		}
		return nil
	}, nil); err != nil {
		t.Fatal(err)
	}

	last5s := []struct {
		name   string
		start  int64
		length int64
	}{
		{name: "negative offset", start: -5, length: -1},
		{name: "offset with specified length", start: int64(len(contents)) - 5, length: 5},
		{name: "offset and read till end", start: int64(len(contents)) - 5, length: -1},
	}

	for _, last5 := range last5s {
		t.Run(last5.name, func(t *testing.T) {
			r, err := obj.NewRangeReader(ctx, last5.start, last5.length)
			if err != nil {
				t.Fatalf("Failed to make range read: %v", err)
			}
			defer r.Close()

			if got, want := r.Attrs.StartOffset, int64(len(contents))-5; got != want {
				t.Fatalf("StartOffset mismatch, got %d want %d", got, want)
			}

			nr, _ := io.Copy(ioutil.Discard, r)
			if got, want := nr, int64(5); got != want {
				t.Fatalf("Body length mismatch, got %d want %d", got, want)
			}
		})
	}
}

func TestIntegration_Objects(t *testing.T) {
	// TODO(jba): Use subtests (Go 1.7).
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	// Reset testTime, 'cause object last modification time should be within 5 min
	// from test (test iteration if -count passed) start time.
	testTime = time.Now().UTC()
	newBucketName := uidSpace.New()
	h := testHelper{t}
	bkt := client.Bucket(newBucketName)

	h.mustCreate(bkt, testutil.ProjID(), nil)
	defer func() {
		if err := killBucket(ctx, client, newBucketName); err != nil {
			log.Printf("deleting %q: %v", newBucketName, err)
		}
	}()
	const defaultType = "text/plain"

	// Populate object names and make a map for their contents.
	objects := []string{
		"obj1",
		"obj2",
		"obj/with/slashes",
	}
	contents := make(map[string][]byte)

	// Test Writer.
	for _, obj := range objects {
		c := randomContents()
		if err := writeObject(ctx, bkt.Object(obj), defaultType, c); err != nil {
			t.Errorf("Write for %v failed with %v", obj, err)
		}
		contents[obj] = c
	}

	testObjectIterator(t, bkt, objects)
	testObjectsIterateSelectedAttrs(t, bkt, objects)
	testObjectsIterateAllSelectedAttrs(t, bkt, objects)
	testObjectIteratorWithOffset(t, bkt, objects)
	testObjectsIterateWithProjection(t, bkt)
	t.Run("testObjectsIterateSelectedAttrsDelimiter", func(t *testing.T) {
		query := &Query{Prefix: "", Delimiter: "/"}
		if err := query.SetAttrSelection([]string{"Name"}); err != nil {
			t.Fatalf("selecting query attrs: %v", err)
		}

		var gotNames []string
		var gotPrefixes []string
		it := bkt.Objects(context.Background(), query)
		for {
			attrs, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				t.Fatalf("iterator.Next: %v", err)
			}
			if attrs.Name != "" {
				gotNames = append(gotNames, attrs.Name)
			} else if attrs.Prefix != "" {
				gotPrefixes = append(gotPrefixes, attrs.Prefix)
			}

			if attrs.Bucket != "" {
				t.Errorf("Bucket field not selected, want empty, got = %v", attrs.Bucket)
			}
		}

		sortedNames := []string{"obj1", "obj2"}
		if !cmp.Equal(sortedNames, gotNames) {
			t.Errorf("names = %v, want %v", gotNames, sortedNames)
		}
		sortedPrefixes := []string{"obj/"}
		if !cmp.Equal(sortedPrefixes, gotPrefixes) {
			t.Errorf("prefixes = %v, want %v", gotPrefixes, sortedPrefixes)
		}
	})

	// Test Reader.
	for _, obj := range objects {
		rc, err := bkt.Object(obj).NewReader(ctx)
		if err != nil {
			t.Errorf("Can't create a reader for %v, errored with %v", obj, err)
			continue
		}
		if !rc.checkCRC {
			t.Errorf("%v: not checking CRC", obj)
		}
		slurp, err := ioutil.ReadAll(rc)
		if err != nil {
			t.Errorf("Can't ReadAll object %v, errored with %v", obj, err)
		}
		if got, want := slurp, contents[obj]; !bytes.Equal(got, want) {
			t.Errorf("Contents (%q) = %q; want %q", obj, got, want)
		}
		if got, want := rc.Size(), len(contents[obj]); got != int64(want) {
			t.Errorf("Size (%q) = %d; want %d", obj, got, want)
		}
		if got, want := rc.ContentType(), "text/plain"; got != want {
			t.Errorf("ContentType (%q) = %q; want %q", obj, got, want)
		}
		if got, want := rc.CacheControl(), "public, max-age=60"; got != want {
			t.Errorf("CacheControl (%q) = %q; want %q", obj, got, want)
		}
		// We just wrote these objects, so they should have a recent last-modified time.
		lm, err := rc.LastModified()
		// Accept a time within +/- of the test time, to account for natural
		// variation and the fact that testTime is set at the start of the test run.
		expectedVariance := 5 * time.Minute
		if err != nil {
			t.Errorf("LastModified (%q): got error %v", obj, err)
		} else if lm.Before(testTime.Add(-expectedVariance)) || lm.After(testTime.Add(expectedVariance)) {
			t.Errorf("LastModified (%q): got %s, which not the %v from now (%v)", obj, lm, expectedVariance, testTime)
		}
		rc.Close()

		// Check early close.
		buf := make([]byte, 1)
		rc, err = bkt.Object(obj).NewReader(ctx)
		if err != nil {
			t.Fatalf("%v: %v", obj, err)
		}
		_, err = rc.Read(buf)
		if err != nil {
			t.Fatalf("%v: %v", obj, err)
		}
		if got, want := buf, contents[obj][:1]; !bytes.Equal(got, want) {
			t.Errorf("Contents[0] (%q) = %q; want %q", obj, got, want)
		}
		if err := rc.Close(); err != nil {
			t.Errorf("%v Close: %v", obj, err)
		}
	}

	obj := objects[0]
	objlen := int64(len(contents[obj]))
	// Test Range Reader.
	for i, r := range []struct {
		offset, length, want int64
	}{
		{0, objlen, objlen},
		{0, objlen / 2, objlen / 2},
		{objlen / 2, objlen, objlen / 2},
		{0, 0, 0},
		{objlen / 2, 0, 0},
		{objlen / 2, -1, objlen / 2},
		{0, objlen * 2, objlen},
		{-2, -1, 2},
		{-objlen, -1, objlen},
		{-(objlen / 2), -1, objlen / 2},
	} {
		rc, err := bkt.Object(obj).NewRangeReader(ctx, r.offset, r.length)
		if err != nil {
			t.Errorf("%+v: Can't create a range reader for %v, errored with %v", i, obj, err)
			continue
		}
		if rc.Size() != objlen {
			t.Errorf("%+v: Reader has a content-size of %d, want %d", i, rc.Size(), objlen)
		}
		if rc.Remain() != r.want {
			t.Errorf("%+v: Reader's available bytes reported as %d, want %d", i, rc.Remain(), r.want)
		}
		slurp, err := ioutil.ReadAll(rc)
		if err != nil {
			t.Errorf("%+v: can't ReadAll object %v, errored with %v", r, obj, err)
			continue
		}
		if len(slurp) != int(r.want) {
			t.Errorf("%+v: RangeReader (%d, %d): Read %d bytes, wanted %d bytes", i, r.offset, r.length, len(slurp), r.want)
			continue
		}

		switch {
		case r.offset < 0: // The case of reading the last N bytes.
			start := objlen + r.offset
			if got, want := slurp, contents[obj][start:]; !bytes.Equal(got, want) {
				t.Errorf("RangeReader (%d, %d) = %q; want %q", r.offset, r.length, got, want)
			}

		default:
			if got, want := slurp, contents[obj][r.offset:r.offset+r.want]; !bytes.Equal(got, want) {
				t.Errorf("RangeReader (%d, %d) = %q; want %q", r.offset, r.length, got, want)
			}
		}
		rc.Close()
	}

	objName := objects[0]

	// Test NewReader googleapi.Error.
	// Since a 429 or 5xx is hard to cause, we trigger a 416.
	realLen := len(contents[objName])
	_, err := bkt.Object(objName).NewRangeReader(ctx, int64(realLen*2), 10)
	if err, ok := err.(*googleapi.Error); !ok {
		t.Error("NewRangeReader did not return a googleapi.Error")
	} else {
		if err.Code != 416 {
			t.Errorf("Code = %d; want %d", err.Code, 416)
		}
		if len(err.Header) == 0 {
			t.Error("Missing googleapi.Error.Header")
		}
		if len(err.Body) == 0 {
			t.Error("Missing googleapi.Error.Body")
		}
	}

	// Test StatObject.
	o := h.mustObjectAttrs(bkt.Object(objName))
	if got, want := o.Name, objName; got != want {
		t.Errorf("Name (%v) = %q; want %q", objName, got, want)
	}
	if got, want := o.ContentType, defaultType; got != want {
		t.Errorf("ContentType (%v) = %q; want %q", objName, got, want)
	}
	created := o.Created
	// Check that the object is newer than its containing bucket.
	bAttrs := h.mustBucketAttrs(bkt)
	if o.Created.Before(bAttrs.Created) {
		t.Errorf("Object %v is older than its containing bucket, %v", o, bAttrs)
	}

	// Test object copy.
	copyName := "copy-" + objName
	copyObj, err := bkt.Object(copyName).CopierFrom(bkt.Object(objName)).Run(ctx)
	if err != nil {
		t.Errorf("Copier.Run failed with %v", err)
	} else if !namesEqual(copyObj, newBucketName, copyName) {
		t.Errorf("Copy object bucket, name: got %q.%q, want %q.%q",
			copyObj.Bucket, copyObj.Name, newBucketName, copyName)
	}

	// Copying with attributes.
	const contentEncoding = "identity"
	copier := bkt.Object(copyName).CopierFrom(bkt.Object(objName))
	copier.ContentEncoding = contentEncoding
	copyObj, err = copier.Run(ctx)
	if err != nil {
		t.Errorf("Copier.Run failed with %v", err)
	} else {
		if !namesEqual(copyObj, newBucketName, copyName) {
			t.Errorf("Copy object bucket, name: got %q.%q, want %q.%q",
				copyObj.Bucket, copyObj.Name, newBucketName, copyName)
		}
		if copyObj.ContentEncoding != contentEncoding {
			t.Errorf("Copy ContentEncoding: got %q, want %q", copyObj.ContentEncoding, contentEncoding)
		}
	}

	// Test UpdateAttrs.
	metadata := map[string]string{"key": "value"}
	updated := h.mustUpdateObject(bkt.Object(objName), ObjectAttrsToUpdate{
		ContentType:     "text/html",
		ContentLanguage: "en",
		Metadata:        metadata,
		ACL:             []ACLRule{{Entity: "domain-google.com", Role: RoleReader}},
	})
	if got, want := updated.ContentType, "text/html"; got != want {
		t.Errorf("updated.ContentType == %q; want %q", got, want)
	}
	if got, want := updated.ContentLanguage, "en"; got != want {
		t.Errorf("updated.ContentLanguage == %q; want %q", updated.ContentLanguage, want)
	}
	if got, want := updated.Metadata, metadata; !testutil.Equal(got, want) {
		t.Errorf("updated.Metadata == %+v; want %+v", updated.Metadata, want)
	}
	if got, want := updated.Created, created; got != want {
		t.Errorf("updated.Created == %q; want %q", got, want)
	}
	if !updated.Created.Before(updated.Updated) {
		t.Errorf("updated.Updated should be newer than update.Created")
	}

	// Delete ContentType and ContentLanguage.
	updated = h.mustUpdateObject(bkt.Object(objName), ObjectAttrsToUpdate{
		ContentType:     "",
		ContentLanguage: "",
		Metadata:        map[string]string{},
	})
	if got, want := updated.ContentType, ""; got != want {
		t.Errorf("updated.ContentType == %q; want %q", got, want)
	}
	if got, want := updated.ContentLanguage, ""; got != want {
		t.Errorf("updated.ContentLanguage == %q; want %q", updated.ContentLanguage, want)
	}
	if updated.Metadata != nil {
		t.Errorf("updated.Metadata == %+v; want nil", updated.Metadata)
	}
	if got, want := updated.Created, created; got != want {
		t.Errorf("updated.Created == %q; want %q", got, want)
	}
	if !updated.Created.Before(updated.Updated) {
		t.Errorf("updated.Updated should be newer than update.Created")
	}

	// Test checksums.
	checksumCases := []struct {
		name     string
		contents [][]byte
		size     int64
		md5      string
		crc32c   uint32
	}{
		{
			name:     "checksum-object",
			contents: [][]byte{[]byte("hello"), []byte("world")},
			size:     10,
			md5:      "fc5e038d38a57032085441e7fe7010b0",
			crc32c:   1456190592,
		},
		{
			name:     "zero-object",
			contents: [][]byte{},
			size:     0,
			md5:      "d41d8cd98f00b204e9800998ecf8427e",
			crc32c:   0,
		},
	}
	for _, c := range checksumCases {
		wc := bkt.Object(c.name).NewWriter(ctx)
		for _, data := range c.contents {
			if _, err := wc.Write(data); err != nil {
				t.Errorf("Write(%q) failed with %q", data, err)
			}
		}
		if err = wc.Close(); err != nil {
			t.Errorf("%q: close failed with %q", c.name, err)
		}
		obj := wc.Attrs()
		if got, want := obj.Size, c.size; got != want {
			t.Errorf("Object (%q) Size = %v; want %v", c.name, got, want)
		}
		if got, want := fmt.Sprintf("%x", obj.MD5), c.md5; got != want {
			t.Errorf("Object (%q) MD5 = %q; want %q", c.name, got, want)
		}
		if got, want := obj.CRC32C, c.crc32c; got != want {
			t.Errorf("Object (%q) CRC32C = %v; want %v", c.name, got, want)
		}
	}

	// Test public ACL.
	publicObj := objects[0]
	if err = bkt.Object(publicObj).ACL().Set(ctx, AllUsers, RoleReader); err != nil {
		t.Errorf("PutACLEntry failed with %v", err)
	}
	publicClient, err := newTestClient(ctx, option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}

	slurp := h.mustRead(publicClient.Bucket(newBucketName).Object(publicObj))
	if !bytes.Equal(slurp, contents[publicObj]) {
		t.Errorf("Public object's content: got %q, want %q", slurp, contents[publicObj])
	}

	// Test writer error handling.
	wc := publicClient.Bucket(newBucketName).Object(publicObj).NewWriter(ctx)
	if _, err := wc.Write([]byte("hello")); err != nil {
		t.Errorf("Write unexpectedly failed with %v", err)
	}
	if err = wc.Close(); err == nil {
		t.Error("Close expected an error, found none")
	}

	// Test deleting the copy object.
	h.mustDeleteObject(bkt.Object(copyName))
	// Deleting it a second time should return ErrObjectNotExist.
	if err := bkt.Object(copyName).Delete(ctx); err != ErrObjectNotExist {
		t.Errorf("second deletion of %v = %v; want ErrObjectNotExist", copyName, err)
	}
	_, err = bkt.Object(copyName).Attrs(ctx)
	if err != ErrObjectNotExist {
		t.Errorf("Copy is expected to be deleted, stat errored with %v", err)
	}

	// Test object composition.
	var compSrcs []*ObjectHandle
	var wantContents []byte
	for _, obj := range objects {
		compSrcs = append(compSrcs, bkt.Object(obj))
		wantContents = append(wantContents, contents[obj]...)
	}
	checkCompose := func(obj *ObjectHandle, wantContentType string) {
		rc := h.mustNewReader(obj)
		slurp, err = ioutil.ReadAll(rc)
		if err != nil {
			t.Fatalf("ioutil.ReadAll: %v", err)
		}
		defer rc.Close()
		if !bytes.Equal(slurp, wantContents) {
			t.Errorf("Composed object contents\ngot:  %q\nwant: %q", slurp, wantContents)
		}
		if got := rc.ContentType(); got != wantContentType {
			t.Errorf("Composed object content-type = %q, want %q", got, wantContentType)
		}
	}

	// Compose should work even if the user sets no destination attributes.
	compDst := bkt.Object("composed1")
	c := compDst.ComposerFrom(compSrcs...)
	if _, err := c.Run(ctx); err != nil {
		t.Fatalf("ComposeFrom error: %v", err)
	}
	checkCompose(compDst, "application/octet-stream")

	// It should also work if we do.
	compDst = bkt.Object("composed2")
	c = compDst.ComposerFrom(compSrcs...)
	c.ContentType = "text/json"
	if _, err := c.Run(ctx); err != nil {
		t.Fatalf("ComposeFrom error: %v", err)
	}
	checkCompose(compDst, "text/json")
}

func TestIntegration_Encoding(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	bkt := client.Bucket(bucketName)

	// Test content encoding
	const zeroCount = 20 << 1 // TODO: should be 20 << 20
	obj := bkt.Object("gzip-test")
	w := obj.NewWriter(ctx)
	w.ContentEncoding = "gzip"
	gw := gzip.NewWriter(w)
	if _, err := io.Copy(gw, io.LimitReader(zeros{}, zeroCount)); err != nil {
		t.Fatalf("io.Copy, upload: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Errorf("gzip.Close(): %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("w.Close(): %v", err)
	}
	r, err := obj.NewReader(ctx)
	if err != nil {
		t.Fatalf("NewReader(gzip-test): %v", err)
	}
	n, err := io.Copy(ioutil.Discard, r)
	if err != nil {
		t.Errorf("io.Copy, download: %v", err)
	}
	if n != zeroCount {
		t.Errorf("downloaded bad data: got %d bytes, want %d", n, zeroCount)
	}

	// Test NotFound.
	_, err = bkt.Object("obj-not-exists").NewReader(ctx)
	if err != ErrObjectNotExist {
		t.Errorf("Object should not exist, err found to be %v", err)
	}
}

func testObjectIterator(t *testing.T, bkt *BucketHandle, objects []string) {
	ctx := context.Background()
	h := testHelper{t}
	// Collect the list of items we expect: ObjectAttrs in lexical order by name.
	names := make([]string, len(objects))
	copy(names, objects)
	sort.Strings(names)
	var attrs []*ObjectAttrs
	for _, name := range names {
		attrs = append(attrs, h.mustObjectAttrs(bkt.Object(name)))
	}
	msg, ok := itesting.TestIterator(attrs,
		func() interface{} { return bkt.Objects(ctx, &Query{Prefix: "obj"}) },
		func(it interface{}) (interface{}, error) { return it.(*ObjectIterator).Next() })
	if !ok {
		t.Errorf("ObjectIterator.Next: %s", msg)
	}
	// TODO(jba): test query.Delimiter != ""
}

func testObjectIteratorWithOffset(t *testing.T, bkt *BucketHandle, objects []string) {
	ctx := context.Background()
	h := testHelper{t}
	// Collect the list of items we expect: ObjectAttrs in lexical order by name.
	names := make([]string, len(objects))
	copy(names, objects)
	sort.Strings(names)
	var attrs []*ObjectAttrs
	for _, name := range names {
		attrs = append(attrs, h.mustObjectAttrs(bkt.Object(name)))
	}
	m := make(map[string][]*ObjectAttrs)
	for i, name := range names {
		// StartOffset takes the value of object names, the result must be for:
		// ― obj/with/slashes: obj/with/slashes, obj1, obj2
		// ― obj1: obj1, obj2
		// ― obj2: obj2.
		m[name] = attrs[i:]
		msg, ok := itesting.TestIterator(m[name],
			func() interface{} { return bkt.Objects(ctx, &Query{StartOffset: name}) },
			func(it interface{}) (interface{}, error) { return it.(*ObjectIterator).Next() })
		if !ok {
			t.Errorf("ObjectIterator.Next: %s", msg)
		}
		// EndOffset takes the value of object names, the result must be for:
		// ― obj/with/slashes: ""
		// ― obj1: obj/with/slashes
		// ― obj2: obj/with/slashes, obj1.
		m[name] = attrs[:i]
		msg, ok = itesting.TestIterator(m[name],
			func() interface{} { return bkt.Objects(ctx, &Query{EndOffset: name}) },
			func(it interface{}) (interface{}, error) { return it.(*ObjectIterator).Next() })
		if !ok {
			t.Errorf("ObjectIterator.Next: %s", msg)
		}
	}
}

func testObjectsIterateSelectedAttrs(t *testing.T, bkt *BucketHandle, objects []string) {
	// Create a query that will only select the "Name" attr of objects, and
	// invoke object listing.
	query := &Query{Prefix: ""}
	query.SetAttrSelection([]string{"Name"})

	var gotNames []string
	it := bkt.Objects(context.Background(), query)
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatalf("iterator.Next: %v", err)
		}
		gotNames = append(gotNames, attrs.Name)

		if len(attrs.Bucket) > 0 {
			t.Errorf("Bucket field not selected, want empty, got = %v", attrs.Bucket)
		}
	}

	sortedNames := make([]string, len(objects))
	copy(sortedNames, objects)
	sort.Strings(sortedNames)
	sort.Strings(gotNames)

	if !cmp.Equal(sortedNames, gotNames) {
		t.Errorf("names = %v, want %v", gotNames, sortedNames)
	}
}

func testObjectsIterateAllSelectedAttrs(t *testing.T, bkt *BucketHandle, objects []string) {
	// Tests that all selected attributes work - query succeeds (without actually
	// verifying the returned results).
	query := &Query{
		Prefix:      "",
		StartOffset: "obj/with/slashes",
		EndOffset:   "obj2",
	}
	var selectedAttrs []string
	for k := range attrToFieldMap {
		selectedAttrs = append(selectedAttrs, k)
	}
	query.SetAttrSelection(selectedAttrs)

	count := 0
	it := bkt.Objects(context.Background(), query)
	for {
		_, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatalf("iterator.Next: %v", err)
		}
		count++
	}

	if count != len(objects)-1 {
		t.Errorf("count = %v, want %v", count, len(objects)-1)
	}
}

func testObjectsIterateWithProjection(t *testing.T, bkt *BucketHandle) {
	projections := map[Projection]bool{
		ProjectionDefault: true,
		ProjectionFull:    true,
		ProjectionNoACL:   false,
	}

	for projection, expectACL := range projections {
		query := &Query{Projection: projection}
		it := bkt.Objects(context.Background(), query)
		attrs, err := it.Next()
		if err == iterator.Done {
			t.Fatalf("iterator: no objects")
		}
		if err != nil {
			t.Fatalf("iterator.Next: %v", err)
		}

		if expectACL {
			if attrs.Owner == "" {
				t.Errorf("projection %q: Owner is empty, want nonempty Owner", projection)
			}
			if len(attrs.ACL) == 0 {
				t.Errorf("projection %q: ACL is empty, want at least one ACL rule", projection)
			}
		} else {
			if attrs.Owner != "" {
				t.Errorf("projection %q: got Owner = %q, want empty Owner", projection, attrs.Owner)
			}
			if len(attrs.ACL) != 0 {
				t.Errorf("projection %q: got %d ACL rules, want empty ACL", projection, len(attrs.ACL))
			}
		}
	}
}

func TestIntegration_SignedURL(t *testing.T) {
	if testing.Short() { // do not test during replay
		t.Skip("Integration tests skipped in short mode")
	}
	// To test SignedURL, we need a real user email and private key. Extract them
	// from the JSON key file.
	jwtConf, err := testutil.JWTConfig()
	if err != nil {
		t.Fatal(err)
	}
	if jwtConf == nil {
		t.Skip("JSON key file is not present")
	}

	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()

	bkt := client.Bucket(bucketName)
	obj := "signedURL"
	contents := []byte("This is a test of SignedURL.\n")
	md5 := "Jyxvgwm9n2MsrGTMPbMeYA==" // base64-encoded MD5 of contents
	if err := writeObject(ctx, bkt.Object(obj), "text/plain", contents); err != nil {
		t.Fatalf("writing: %v", err)
	}
	for _, test := range []struct {
		desc    string
		opts    SignedURLOptions
		headers map[string][]string
		fail    bool
	}{
		{
			desc: "basic v2",
		},
		{
			desc: "basic v4",
			opts: SignedURLOptions{Scheme: SigningSchemeV4},
		},
		{
			desc:    "MD5 sent and matches",
			opts:    SignedURLOptions{MD5: md5},
			headers: map[string][]string{"Content-MD5": {md5}},
		},
		{
			desc: "MD5 not sent",
			opts: SignedURLOptions{MD5: md5},
			fail: true,
		},
		{
			desc:    "Content-Type sent and matches",
			opts:    SignedURLOptions{ContentType: "text/plain"},
			headers: map[string][]string{"Content-Type": {"text/plain"}},
		},
		{
			desc:    "Content-Type sent but does not match",
			opts:    SignedURLOptions{ContentType: "text/plain"},
			headers: map[string][]string{"Content-Type": {"application/json"}},
			fail:    true,
		},
		{
			desc: "Canonical headers sent and match",
			opts: SignedURLOptions{Headers: []string{
				" X-Goog-Foo: Bar baz ",
				"X-Goog-Novalue", // ignored: no value
				"X-Google-Foo",   // ignored: wrong prefix
			}},
			headers: map[string][]string{"X-Goog-foo": {"Bar baz  "}},
		},
		{
			desc:    "Canonical headers sent but don't match",
			opts:    SignedURLOptions{Headers: []string{" X-Goog-Foo: Bar baz"}},
			headers: map[string][]string{"X-Goog-Foo": {"bar baz"}},
			fail:    true,
		},
	} {
		opts := test.opts
		opts.GoogleAccessID = jwtConf.Email
		opts.PrivateKey = jwtConf.PrivateKey
		opts.Method = "GET"
		opts.Expires = time.Now().Add(time.Hour)
		u, err := SignedURL(bucketName, obj, &opts)
		if err != nil {
			t.Errorf("%s: SignedURL: %v", test.desc, err)
			continue
		}
		got, err := getURL(u, test.headers)
		if err != nil && !test.fail {
			t.Errorf("%s: getURL %q: %v", test.desc, u, err)
		} else if err == nil && !bytes.Equal(got, contents) {
			t.Errorf("%s: got %q, want %q", test.desc, got, contents)
		}
	}
}

func TestIntegration_SignedURL_WithEncryptionKeys(t *testing.T) {
	if testing.Short() { // do not test during replay
		t.Skip("Integration tests skipped in short mode")
	}
	// To test SignedURL, we need a real user email and private key. Extract
	// them from the JSON key file.
	jwtConf, err := testutil.JWTConfig()
	if err != nil {
		t.Fatal(err)
	}
	if jwtConf == nil {
		t.Skip("JSON key file is not present")
	}

	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()

	// TODO(deklerk): document how these were generated and their significance
	encryptionKey := "AAryxNglNkXQY0Wa+h9+7BLSFMhCzPo22MtXUWjOBbI="
	encryptionKeySha256 := "QlCdVONb17U1aCTAjrFvMbnxW/Oul8VAvnG1875WJ3k="
	headers := map[string][]string{
		"x-goog-encryption-algorithm":  {"AES256"},
		"x-goog-encryption-key":        {encryptionKey},
		"x-goog-encryption-key-sha256": {encryptionKeySha256},
	}
	contents := []byte(`{"message":"encryption with csek works"}`)
	tests := []struct {
		desc string
		opts *SignedURLOptions
	}{
		{
			desc: "v4 URL with customer supplied encryption keys for PUT",
			opts: &SignedURLOptions{
				Method: "PUT",
				Headers: []string{
					"x-goog-encryption-algorithm:AES256",
					"x-goog-encryption-key:AAryxNglNkXQY0Wa+h9+7BLSFMhCzPo22MtXUWjOBbI=",
					"x-goog-encryption-key-sha256:QlCdVONb17U1aCTAjrFvMbnxW/Oul8VAvnG1875WJ3k=",
				},
				Scheme: SigningSchemeV4,
			},
		},
		{
			desc: "v4 URL with customer supplied encryption keys for GET",
			opts: &SignedURLOptions{
				Method: "GET",
				Headers: []string{
					"x-goog-encryption-algorithm:AES256",
					fmt.Sprintf("x-goog-encryption-key:%s", encryptionKey),
					fmt.Sprintf("x-goog-encryption-key-sha256:%s", encryptionKeySha256),
				},
				Scheme: SigningSchemeV4,
			},
		},
	}
	defer func() {
		// Delete encrypted object.
		bkt := client.Bucket(bucketName)
		err := bkt.Object("csek.json").Delete(ctx)
		if err != nil {
			log.Printf("failed to deleted encrypted file: %v", err)
		}
	}()

	for _, test := range tests {
		opts := test.opts
		opts.GoogleAccessID = jwtConf.Email
		opts.PrivateKey = jwtConf.PrivateKey
		opts.Expires = time.Now().Add(time.Hour)

		u, err := SignedURL(bucketName, "csek.json", test.opts)
		if err != nil {
			t.Fatalf("%s: %v", test.desc, err)
		}

		if test.opts.Method == "PUT" {
			if _, err := putURL(u, headers, bytes.NewReader(contents)); err != nil {
				t.Fatalf("%s: %v", test.desc, err)
			}
		}

		if test.opts.Method == "GET" {
			got, err := getURL(u, headers)
			if err != nil {
				t.Fatalf("%s: %v", test.desc, err)
			}
			if !bytes.Equal(got, contents) {
				t.Fatalf("%s: got %q, want %q", test.desc, got, contents)
			}
		}
	}
}

func TestIntegration_SignedURL_EmptyStringObjectName(t *testing.T) {
	if testing.Short() { // do not test during replay
		t.Skip("Integration tests skipped in short mode")
	}

	// To test SignedURL, we need a real user email and private key. Extract them
	// from the JSON key file.
	jwtConf, err := testutil.JWTConfig()
	if err != nil {
		t.Fatal(err)
	}
	if jwtConf == nil {
		t.Skip("JSON key file is not present")
	}

	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()

	opts := &SignedURLOptions{
		Scheme:         SigningSchemeV4,
		Method:         "GET",
		GoogleAccessID: jwtConf.Email,
		PrivateKey:     jwtConf.PrivateKey,
		Expires:        time.Now().Add(time.Hour),
	}

	u, err := SignedURL(bucketName, "", opts)
	if err != nil {
		t.Fatal(err)
	}

	// Should be some ListBucketResult response.
	_, err = getURL(u, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestIntegration_ACL(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()

	bkt := client.Bucket(bucketName)

	entity := ACLEntity("domain-google.com")
	rule := ACLRule{Entity: entity, Role: RoleReader, Domain: "google.com"}
	if err := bkt.DefaultObjectACL().Set(ctx, entity, RoleReader); err != nil {
		t.Errorf("Can't put default ACL rule for the bucket, errored with %v", err)
	}

	acl, err := bkt.DefaultObjectACL().List(ctx)
	if err != nil {
		t.Errorf("DefaultObjectACL.List for bucket %q: %v", bucketName, err)
	} else if !hasRule(acl, rule) {
		t.Errorf("default ACL missing %#v", rule)
	}
	aclObjects := []string{"acl1", "acl2"}
	name := aclObjects[0]
	o := bkt.Object(name)
	err = retry(ctx, func() error {
		for _, obj := range aclObjects {
			c := randomContents()
			if err := writeObject(ctx, bkt.Object(obj), "", c); err != nil {
				t.Errorf("Write for %v failed with %v", obj, err)
			}
		}
		acl, err = o.ACL().List(ctx)
		if err != nil {
			return fmt.Errorf("ACL.List: can't retrieve ACL of %v", name)
		}
		return nil
	}, func() error {
		if !hasRule(acl, rule) {
			return fmt.Errorf("hasRule: object ACL missing %+v", rule)
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}
	if err := o.ACL().Delete(ctx, entity); err != nil {
		t.Errorf("object ACL: could not delete entity %s", entity)
	}
	// Delete the default ACL rule. We can't move this code earlier in the
	// test, because the test depends on the fact that the object ACL inherits
	// it.
	if err := bkt.DefaultObjectACL().Delete(ctx, entity); err != nil {
		t.Errorf("default ACL: could not delete entity %s", entity)
	}

	entity2 := ACLEntity("user-jbd@google.com")
	rule2 := ACLRule{Entity: entity2, Role: RoleReader, Email: "jbd@google.com"}
	if err := bkt.ACL().Set(ctx, entity2, RoleReader); err != nil {
		t.Errorf("Error while putting bucket ACL rule: %v", err)
	}
	var bACL []ACLRule
	err = retry(ctx, func() error {
		bACL, err = bkt.ACL().List(ctx)
		if err != nil {
			return fmt.Errorf("ACL.List: error while getting the ACL of the bucket: %v", err)
		}
		return nil
	}, func() error {
		if !hasRule(bACL, rule2) {
			return fmt.Errorf("hasRule: bucket ACL missing %+v", rule2)
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}
	if err := bkt.ACL().Delete(ctx, entity2); err != nil {
		t.Errorf("Error while deleting bucket ACL rule: %v", err)
	}

}

func TestIntegration_ValidObjectNames(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()

	bkt := client.Bucket(bucketName)

	validNames := []string{
		"gopher",
		"Гоферови",
		"a",
		strings.Repeat("a", 1024),
	}
	for _, name := range validNames {
		if err := writeObject(ctx, bkt.Object(name), "", []byte("data")); err != nil {
			t.Errorf("Object %q write failed: %v. Want success", name, err)
			continue
		}
		defer bkt.Object(name).Delete(ctx)
	}

	invalidNames := []string{
		"",                        // Too short.
		strings.Repeat("a", 1025), // Too long.
		"new\nlines",
		"bad\xffunicode",
	}
	for _, name := range invalidNames {
		// Invalid object names will either cause failure during Write or Close.
		if err := writeObject(ctx, bkt.Object(name), "", []byte("data")); err != nil {
			continue
		}
		defer bkt.Object(name).Delete(ctx)
		t.Errorf("%q should have failed. Didn't", name)
	}
}

func TestIntegration_WriterContentType(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()

	obj := client.Bucket(bucketName).Object("content")
	testCases := []struct {
		content           string
		setType, wantType string
	}{
		{
			content:  "It was the best of times, it was the worst of times.",
			wantType: "text/plain; charset=utf-8",
		},
		{
			content:  "<html><head><title>My first page</title></head></html>",
			wantType: "text/html; charset=utf-8",
		},
		{
			content:  "<html><head><title>My first page</title></head></html>",
			setType:  "text/html",
			wantType: "text/html",
		},
		{
			content:  "<html><head><title>My first page</title></head></html>",
			setType:  "image/jpeg",
			wantType: "image/jpeg",
		},
	}
	for i, tt := range testCases {
		if err := writeObject(ctx, obj, tt.setType, []byte(tt.content)); err != nil {
			t.Errorf("writing #%d: %v", i, err)
		}
		attrs, err := obj.Attrs(ctx)
		if err != nil {
			t.Errorf("obj.Attrs: %v", err)
			continue
		}
		if got := attrs.ContentType; got != tt.wantType {
			t.Errorf("Content-Type = %q; want %q\nContent: %q\nSet Content-Type: %q", got, tt.wantType, tt.content, tt.setType)
		}
	}
}

func TestIntegration_ZeroSizedObject(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	obj := client.Bucket(bucketName).Object("zero")

	// Check writing it works as expected.
	w := obj.NewWriter(ctx)
	if err := w.Close(); err != nil {
		t.Fatalf("Writer.Close: %v", err)
	}
	defer obj.Delete(ctx)

	// Check we can read it too.
	body := h.mustRead(obj)
	if len(body) != 0 {
		t.Errorf("Body is %v, want empty []byte{}", body)
	}
}

func TestIntegration_Encryption(t *testing.T) {
	// This function tests customer-supplied encryption keys for all operations
	// involving objects. Bucket and ACL operations aren't tested because they
	// aren't affected by customer encryption. Neither is deletion.
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	obj := client.Bucket(bucketName).Object("customer-encryption")
	key := []byte("my-secret-AES-256-encryption-key")
	keyHash := sha256.Sum256(key)
	keyHashB64 := base64.StdEncoding.EncodeToString(keyHash[:])
	key2 := []byte("My-Secret-AES-256-Encryption-Key")
	contents := "top secret."

	checkMetadataCall := func(msg string, f func(o *ObjectHandle) (*ObjectAttrs, error)) {
		// Performing a metadata operation without the key should succeed.
		attrs, err := f(obj)
		if err != nil {
			t.Fatalf("%s: %v", msg, err)
		}
		// The key hash should match...
		if got, want := attrs.CustomerKeySHA256, keyHashB64; got != want {
			t.Errorf("%s: key hash: got %q, want %q", msg, got, want)
		}
		// ...but CRC and MD5 should not be present.
		if attrs.CRC32C != 0 {
			t.Errorf("%s: CRC: got %v, want 0", msg, attrs.CRC32C)
		}
		if len(attrs.MD5) > 0 {
			t.Errorf("%s: MD5: got %v, want len == 0", msg, attrs.MD5)
		}

		// Performing a metadata operation with the key should succeed.
		attrs, err = f(obj.Key(key))
		if err != nil {
			t.Fatalf("%s: %v", msg, err)
		}
		// Check the key and content hashes.
		if got, want := attrs.CustomerKeySHA256, keyHashB64; got != want {
			t.Errorf("%s: key hash: got %q, want %q", msg, got, want)
		}
		if attrs.CRC32C == 0 {
			t.Errorf("%s: CRC: got 0, want non-zero", msg)
		}
		if len(attrs.MD5) == 0 {
			t.Errorf("%s: MD5: got len == 0, want len > 0", msg)
		}
	}

	checkRead := func(msg string, o *ObjectHandle, k []byte, wantContents string) {
		// Reading the object without the key should fail.
		if _, err := readObject(ctx, o); err == nil {
			t.Errorf("%s: reading without key: want error, got nil", msg)
		}
		// Reading the object with the key should succeed.
		got := h.mustRead(o.Key(k))
		gotContents := string(got)
		// And the contents should match what we wrote.
		if gotContents != wantContents {
			t.Errorf("%s: contents: got %q, want %q", msg, gotContents, wantContents)
		}
	}

	checkReadUnencrypted := func(msg string, obj *ObjectHandle, wantContents string) {
		got := h.mustRead(obj)
		gotContents := string(got)
		if gotContents != wantContents {
			t.Errorf("%s: got %q, want %q", msg, gotContents, wantContents)
		}
	}

	// Write to obj using our own encryption key, which is a valid 32-byte
	// AES-256 key.
	h.mustWrite(obj.Key(key).NewWriter(ctx), []byte(contents))

	checkMetadataCall("Attrs", func(o *ObjectHandle) (*ObjectAttrs, error) {
		return o.Attrs(ctx)
	})

	checkMetadataCall("Update", func(o *ObjectHandle) (*ObjectAttrs, error) {
		return o.Update(ctx, ObjectAttrsToUpdate{ContentLanguage: "en"})
	})

	checkRead("first object", obj, key, contents)

	obj2 := client.Bucket(bucketName).Object("customer-encryption-2")
	// Copying an object without the key should fail.
	if _, err := obj2.CopierFrom(obj).Run(ctx); err == nil {
		t.Fatal("want error, got nil")
	}
	// Copying an object with the key should succeed.
	if _, err := obj2.CopierFrom(obj.Key(key)).Run(ctx); err != nil {
		t.Fatal(err)
	}
	// The destination object is not encrypted; we can read it without a key.
	checkReadUnencrypted("copy dest", obj2, contents)

	// Providing a key on the destination but not the source should fail,
	// since the source is encrypted.
	if _, err := obj2.Key(key2).CopierFrom(obj).Run(ctx); err == nil {
		t.Fatal("want error, got nil")
	}

	// But copying with keys for both source and destination should succeed.
	if _, err := obj2.Key(key2).CopierFrom(obj.Key(key)).Run(ctx); err != nil {
		t.Fatal(err)
	}
	// And the destination should be encrypted, meaning we can only read it
	// with a key.
	checkRead("copy destination", obj2, key2, contents)

	// Change obj2's key to prepare for compose, where all objects must have
	// the same key. Also illustrates key rotation: copy an object to itself
	// with a different key.
	if _, err := obj2.Key(key).CopierFrom(obj2.Key(key2)).Run(ctx); err != nil {
		t.Fatal(err)
	}
	obj3 := client.Bucket(bucketName).Object("customer-encryption-3")
	// Composing without keys should fail.
	if _, err := obj3.ComposerFrom(obj, obj2).Run(ctx); err == nil {
		t.Fatal("want error, got nil")
	}
	// Keys on the source objects result in an error.
	if _, err := obj3.ComposerFrom(obj.Key(key), obj2).Run(ctx); err == nil {
		t.Fatal("want error, got nil")
	}
	// A key on the destination object both decrypts the source objects
	// and encrypts the destination.
	if _, err := obj3.Key(key).ComposerFrom(obj, obj2).Run(ctx); err != nil {
		t.Fatalf("got %v, want nil", err)
	}
	// Check that the destination in encrypted.
	checkRead("compose destination", obj3, key, contents+contents)

	// You can't compose one or more unencrypted source objects into an
	// encrypted destination object.
	_, err := obj2.CopierFrom(obj2.Key(key)).Run(ctx) // unencrypt obj2
	if err != nil {
		t.Fatal(err)
	}
	if _, err := obj3.Key(key).ComposerFrom(obj2).Run(ctx); err == nil {
		t.Fatal("got nil, want error")
	}
}

func TestIntegration_NonexistentBucket(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()

	bkt := client.Bucket(uidSpace.New())
	if _, err := bkt.Attrs(ctx); err != ErrBucketNotExist {
		t.Errorf("Attrs: got %v, want ErrBucketNotExist", err)
	}
	it := bkt.Objects(ctx, nil)
	if _, err := it.Next(); err != ErrBucketNotExist {
		t.Errorf("Objects: got %v, want ErrBucketNotExist", err)
	}
}

func TestIntegration_PerObjectStorageClass(t *testing.T) {
	const (
		defaultStorageClass = "STANDARD"
		newStorageClass     = "NEARLINE"
	)
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	bkt := client.Bucket(bucketName)

	// The bucket should have the default storage class.
	battrs := h.mustBucketAttrs(bkt)
	if battrs.StorageClass != defaultStorageClass {
		t.Fatalf("bucket storage class: got %q, want %q",
			battrs.StorageClass, defaultStorageClass)
	}
	// Write an object; it should start with the bucket's storage class.
	obj := bkt.Object("posc")
	h.mustWrite(obj.NewWriter(ctx), []byte("foo"))
	oattrs, err := obj.Attrs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if oattrs.StorageClass != defaultStorageClass {
		t.Fatalf("object storage class: got %q, want %q",
			oattrs.StorageClass, defaultStorageClass)
	}
	// Now use Copy to change the storage class.
	copier := obj.CopierFrom(obj)
	copier.StorageClass = newStorageClass
	oattrs2, err := copier.Run(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if oattrs2.StorageClass != newStorageClass {
		t.Fatalf("new object storage class: got %q, want %q",
			oattrs2.StorageClass, newStorageClass)
	}

	// We can also write a new object using a non-default storage class.
	obj2 := bkt.Object("posc2")
	w := obj2.NewWriter(ctx)
	w.StorageClass = newStorageClass
	h.mustWrite(w, []byte("xxx"))
	if w.Attrs().StorageClass != newStorageClass {
		t.Fatalf("new object storage class: got %q, want %q",
			w.Attrs().StorageClass, newStorageClass)
	}
}

func TestIntegration_BucketInCopyAttrs(t *testing.T) {
	// Confirm that if bucket is included in the object attributes of a rewrite
	// call, but object name and content-type aren't, then we get an error. See
	// the comment in Copier.Run.
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	bkt := client.Bucket(bucketName)
	obj := bkt.Object("bucketInCopyAttrs")
	h.mustWrite(obj.NewWriter(ctx), []byte("foo"))
	copier := obj.CopierFrom(obj)
	rawObject := copier.ObjectAttrs.toRawObject(bucketName)
	_, err := copier.callRewrite(ctx, rawObject)
	if err == nil {
		t.Errorf("got nil, want error")
	}
}

func TestIntegration_NoUnicodeNormalization(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	bkt := client.Bucket(bucketName)
	h := testHelper{t}

	for _, tst := range []struct {
		nameQuoted, content string
	}{
		{`"Caf\u00e9"`, "Normalization Form C"},
		{`"Cafe\u0301"`, "Normalization Form D"},
	} {
		name, err := strconv.Unquote(tst.nameQuoted)
		w := bkt.Object(name).NewWriter(ctx)
		h.mustWrite(w, []byte(tst.content))
		if err != nil {
			t.Fatalf("invalid name: %s: %v", tst.nameQuoted, err)
		}
		if got := string(h.mustRead(bkt.Object(name))); got != tst.content {
			t.Errorf("content of %s is %q, want %q", tst.nameQuoted, got, tst.content)
		}
	}
}

func TestIntegration_HashesOnUpload(t *testing.T) {
	// Check that the user can provide hashes on upload, and that these are checked.
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	obj := client.Bucket(bucketName).Object("hashesOnUpload-1")
	data := []byte("I can't wait to be verified")

	write := func(w *Writer) error {
		if _, err := w.Write(data); err != nil {
			_ = w.Close()
			return err
		}
		return w.Close()
	}

	crc32c := crc32.Checksum(data, crc32cTable)
	// The correct CRC should succeed.
	w := obj.NewWriter(ctx)
	w.CRC32C = crc32c
	w.SendCRC32C = true
	if err := write(w); err != nil {
		t.Fatal(err)
	}

	// If we change the CRC, validation should fail.
	w = obj.NewWriter(ctx)
	w.CRC32C = crc32c + 1
	w.SendCRC32C = true
	if err := write(w); err == nil {
		t.Fatal("write with bad CRC32c: want error, got nil")
	}

	// If we have the wrong CRC but forget to send it, we succeed.
	w = obj.NewWriter(ctx)
	w.CRC32C = crc32c + 1
	if err := write(w); err != nil {
		t.Fatal(err)
	}

	// MD5
	md5 := md5.Sum(data)
	// The correct MD5 should succeed.
	w = obj.NewWriter(ctx)
	w.MD5 = md5[:]
	if err := write(w); err != nil {
		t.Fatal(err)
	}

	// If we change the MD5, validation should fail.
	w = obj.NewWriter(ctx)
	w.MD5 = append([]byte(nil), md5[:]...)
	w.MD5[0]++
	if err := write(w); err == nil {
		t.Fatal("write with bad MD5: want error, got nil")
	}
}

func TestIntegration_BucketIAM(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}
	bkt := client.Bucket(uidSpace.New())
	h.mustCreate(bkt, testutil.ProjID(), nil)
	defer h.mustDeleteBucket(bkt)
	// This bucket is unique to this test run. So we don't have
	// to worry about other runs interfering with our IAM policy
	// changes.

	member := "projectViewer:" + testutil.ProjID()
	role := iam.RoleName("roles/storage.objectViewer")
	// Get the bucket's IAM policy.
	policy, err := bkt.IAM().Policy(ctx)
	if err != nil {
		t.Fatalf("Getting policy: %v", err)
	}
	// The member should not have the role.
	if policy.HasRole(member, role) {
		t.Errorf("member %q has role %q", member, role)
	}
	// Change the policy.
	policy.Add(member, role)
	if err := bkt.IAM().SetPolicy(ctx, policy); err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	// Confirm that the binding was added.
	policy, err = bkt.IAM().Policy(ctx)
	if err != nil {
		t.Fatalf("Getting policy: %v", err)
	}
	if !policy.HasRole(member, role) {
		t.Errorf("member %q does not have role %q", member, role)
	}

	// Check TestPermissions.
	// This client should have all these permissions (and more).
	perms := []string{"storage.buckets.get", "storage.buckets.delete"}
	got, err := bkt.IAM().TestPermissions(ctx, perms)
	if err != nil {
		t.Fatalf("TestPermissions: %v", err)
	}
	sort.Strings(perms)
	sort.Strings(got)
	if !testutil.Equal(got, perms) {
		t.Errorf("got %v, want %v", got, perms)
	}
}

func TestIntegration_RequesterPays(t *testing.T) {
	// This test needs a second project and user (token source) to test
	// all possibilities. Since we need these things for Firestore already,
	// we use them here.
	//
	// There are up to three entities involved in a requester-pays call:
	//
	// 1. The user making the request. Here, we use
	//    a. The account used to create the token source used for all our
	//       integration tests (see testutil.TokenSource).
	//    b. The account used for the Firestore tests.
	// 2. The project that owns the requester-pays bucket. Here, that
	//    is the test project ID (see testutil.ProjID).
	// 3. The project provided as the userProject parameter of the request;
	//    the project to be billed. This test uses:
	//    a. The project that owns the requester-pays bucket (same as (2))
	//    b. Another project (the Firestore project).
	//
	// The following must hold for this test to work:
	// - (1a) must have resourcemanager.projects.createBillingAssignment permission
	//       (Owner role) on (2) (the project, not the bucket).
	// - (1b) must NOT have that permission on (2).
	// - (1b) must have serviceusage.services.use permission (Editor role) on (3b).
	// - (1b) must NOT have that permission on (3a).
	// - (1a) must NOT have that permission on (3b).

	const wantErrorCode = 400

	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	bucketName2 := uidSpace.New()
	b1 := client.Bucket(bucketName2)
	projID := testutil.ProjID()
	// Use Firestore project as a project that does not contain the bucket.
	otherProjID := os.Getenv(envFirestoreProjID)
	if otherProjID == "" {
		t.Fatalf("need a second project (env var %s)", envFirestoreProjID)
	}
	ts := testutil.TokenSourceEnv(ctx, envFirestorePrivateKey, ScopeFullControl)
	if ts == nil {
		t.Fatalf("need a second account (env var %s)", envFirestorePrivateKey)
	}
	otherClient, err := newTestClient(ctx, option.WithTokenSource(ts))
	if err != nil {
		t.Fatal(err)
	}
	defer otherClient.Close()
	b2 := otherClient.Bucket(bucketName2)
	user, err := keyFileEmail(os.Getenv("GCLOUD_TESTS_GOLANG_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	otherUser, err := keyFileEmail(os.Getenv(envFirestorePrivateKey))
	if err != nil {
		t.Fatal(err)
	}

	// Create a requester-pays bucket. The bucket is contained in the project projID.
	h.mustCreate(b1, projID, &BucketAttrs{RequesterPays: true})
	if err := b1.ACL().Set(ctx, ACLEntity("user-"+otherUser), RoleOwner); err != nil {
		t.Fatal(err)
	}

	// Extract the error code from err if it's a googleapi.Error.
	errCode := func(err error) int {
		if err == nil {
			return 0
		}
		if err, ok := err.(*googleapi.Error); ok {
			return err.Code
		}
		return -1
	}

	// Call f under various conditions.
	// Here b1 and b2 refer to the same bucket, but b1 is bound to client,
	// while b2 is bound to otherClient. The clients differ in their credentials,
	// i.e. the identity of the user making the RPC: b1's user is an Owner on the
	// bucket's containing project, b2's is not.
	call := func(msg string, f func(*BucketHandle) error) {
		// user: an Owner on the containing project
		// userProject: absent
		// result: success, by the rule permitting access by owners of the containing bucket.
		if err := f(b1); err != nil {
			t.Errorf("%s: %v, want nil\n"+
				"confirm that %s is an Owner on %s",
				msg, err, user, projID)
		}
		// user: an Owner on the containing project
		// userProject: containing project
		// result: success, by the same rule as above; userProject is unnecessary but allowed.
		if err := f(b1.UserProject(projID)); err != nil {
			t.Errorf("%s: got %v, want nil", msg, err)
		}
		// user: not an Owner on the containing project
		// userProject: absent
		// result: failure, by the standard requester-pays rule
		err := f(b2)
		if got, want := errCode(err), wantErrorCode; got != want {
			t.Errorf("%s: got error %v with code %d, want code %d\n"+
				"confirm that %s is NOT an Owner on %s",
				msg, err, got, want, otherUser, projID)
		}
		// user: not an Owner on the containing project
		// userProject: not the containing one, but user has Editor role on it
		// result: success, by the standard requester-pays rule
		if err := f(b2.UserProject(otherProjID)); err != nil {
			t.Errorf("%s: got %v, want nil\n"+
				"confirm that %s is an Editor on %s and that that project has billing enabled",
				msg, err, otherUser, otherProjID)
		}
		// user: not an Owner on the containing project
		// userProject: the containing one, on which the user does NOT have Editor permission.
		// result: failure
		err = f(b2.UserProject("veener-jba"))
		if got, want := errCode(err), 403; got != want {
			t.Errorf("%s: got error %v, want code %d\n"+
				"confirm that %s is NOT an Editor on %s",
				msg, err, want, otherUser, "veener-jba")
		}
	}

	// Getting its attributes requires a user project.
	var attrs *BucketAttrs
	call("Bucket attrs", func(b *BucketHandle) error {
		a, err := b.Attrs(ctx)
		if a != nil {
			attrs = a
		}
		return err
	})
	if attrs != nil {
		if got, want := attrs.RequesterPays, true; got != want {
			t.Fatalf("attr.RequesterPays = %t, want %t", got, want)
		}
	}
	// Object operations.
	obj1 := "acl-go-test" + uidSpace.New()
	call("write object", func(b *BucketHandle) error {
		return writeObject(ctx, b.Object(obj1), "text/plain", []byte("hello"))
	})
	call("read object", func(b *BucketHandle) error {
		_, err := readObject(ctx, b.Object(obj1))
		return err
	})
	call("object attrs", func(b *BucketHandle) error {
		_, err := b.Object(obj1).Attrs(ctx)
		return err
	})
	call("update object", func(b *BucketHandle) error {
		_, err := b.Object(obj1).Update(ctx, ObjectAttrsToUpdate{ContentLanguage: "en"})
		return err
	})

	// ACL operations.
	// Create another object for these to avoid object rate limits.
	obj2 := "acl-go-test" + uidSpace.New()
	call("write object", func(b *BucketHandle) error {
		return writeObject(ctx, b.Object(obj2), "text/plain", []byte("hello"))
	})
	entity := ACLEntity("domain-google.com")
	call("bucket acl set", func(b *BucketHandle) error {
		return b.ACL().Set(ctx, entity, RoleReader)
	})
	call("bucket acl list", func(b *BucketHandle) error {
		_, err := b.ACL().List(ctx)
		return err
	})
	call("bucket acl delete", func(b *BucketHandle) error {
		err := b.ACL().Delete(ctx, entity)
		if errCode(err) == 404 {
			// Since we call the function multiple times, it will
			// fail with NotFound for all but the first.
			return nil
		}
		return err
	})
	call("default object acl set", func(b *BucketHandle) error {
		return b.DefaultObjectACL().Set(ctx, entity, RoleReader)
	})
	call("default object acl list", func(b *BucketHandle) error {
		_, err := b.DefaultObjectACL().List(ctx)
		return err
	})
	call("default object acl delete", func(b *BucketHandle) error {
		err := b.DefaultObjectACL().Delete(ctx, entity)
		if errCode(err) == 404 {
			return nil
		}
		return err
	})
	call("object acl set", func(b *BucketHandle) error {
		return b.Object(obj2).ACL().Set(ctx, entity, RoleReader)
	})
	call("object acl list", func(b *BucketHandle) error {
		_, err := b.Object(obj2).ACL().List(ctx)
		return err
	})
	call("object acl delete", func(b *BucketHandle) error {
		err := b.Object(obj2).ACL().Delete(ctx, entity)
		if errCode(err) == 404 {
			return nil
		}
		return err
	})

	// Copy and compose.
	call("copy", func(b *BucketHandle) error {
		_, err := b.Object("copy").CopierFrom(b.Object(obj1)).Run(ctx)
		return err
	})
	call("compose", func(b *BucketHandle) error {
		_, err := b.Object("compose").ComposerFrom(b.Object(obj1), b.Object("copy")).Run(ctx)
		return err
	})
	call("delete object", func(b *BucketHandle) error {
		// Make sure the object exists, so we don't get confused by ErrObjectNotExist.
		// The storage service may perform validation in any order (perhaps in parallel),
		// so if we delete an object that doesn't exist and for which we lack permission,
		// we could see either of those two errors. (See Google-internal bug 78341001.)
		h.mustWrite(b1.Object(obj1).NewWriter(ctx), []byte("hello")) // note: b1, not b.
		return b.Object(obj1).Delete(ctx)
	})
	b1.Object(obj1).Delete(ctx) // Clean up created objects.
	b1.Object(obj2).Delete(ctx)
	for _, obj := range []string{"copy", "compose"} {
		if err := b1.UserProject(projID).Object(obj).Delete(ctx); err != nil {
			t.Fatalf("could not delete %q: %v", obj, err)
		}
	}

	h.mustDeleteBucket(b1)
}

func TestIntegration_Notifications(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	bkt := client.Bucket(bucketName)

	checkNotifications := func(msg string, want map[string]*Notification) {
		got, err := bkt.Notifications(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if diff := testutil.Diff(got, want); diff != "" {
			t.Errorf("%s: got=-, want=+:\n%s", msg, diff)
		}
	}
	checkNotifications("initial", map[string]*Notification{})

	nArg := &Notification{
		TopicProjectID: testutil.ProjID(),
		TopicID:        "go-storage-notification-test",
		PayloadFormat:  NoPayload,
	}
	n, err := bkt.AddNotification(ctx, nArg)
	if err != nil {
		t.Fatal(err)
	}
	nArg.ID = n.ID
	if !testutil.Equal(n, nArg) {
		t.Errorf("got %+v, want %+v", n, nArg)
	}
	checkNotifications("after add", map[string]*Notification{n.ID: n})

	if err := bkt.DeleteNotification(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	checkNotifications("after delete", map[string]*Notification{})
}

func TestIntegration_PublicBucket(t *testing.T) {
	// Confirm that an unauthenticated client can access a public bucket.
	// See https://cloud.google.com/storage/docs/public-datasets/landsat
	if testing.Short() && !replaying {
		t.Skip("Integration tests skipped in short mode")
	}

	const landsatBucket = "gcp-public-data-landsat"
	const landsatPrefix = "LC08/01/001/002/LC08_L1GT_001002_20160817_20170322_01_T2/"
	const landsatObject = landsatPrefix + "LC08_L1GT_001002_20160817_20170322_01_T2_ANG.txt"

	// Create an unauthenticated client.
	ctx := context.Background()
	client, err := newTestClient(ctx, option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	h := testHelper{t}
	bkt := client.Bucket(landsatBucket)
	obj := bkt.Object(landsatObject)

	// Read a public object.
	bytes := h.mustRead(obj)
	if got, want := len(bytes), 117255; got != want {
		t.Errorf("len(bytes) = %d, want %d", got, want)
	}

	// List objects in a public bucket.
	iter := bkt.Objects(ctx, &Query{Prefix: landsatPrefix})
	gotCount := 0
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		gotCount++
	}
	if wantCount := 14; gotCount != wantCount {
		t.Errorf("object count: got %d, want %d", gotCount, wantCount)
	}

	errCode := func(err error) int {
		err2, ok := err.(*googleapi.Error)
		if !ok {
			return -1
		}
		return err2.Code
	}

	// Reading from or writing to a non-public bucket fails.
	c := testConfig(ctx, t)
	defer c.Close()
	nonPublicObj := client.Bucket(bucketName).Object("noauth")
	// Oddly, reading returns 403 but writing returns 401.
	_, err = readObject(ctx, nonPublicObj)
	if got, want := errCode(err), 403; got != want {
		t.Errorf("got code %d; want %d\nerror: %v", got, want, err)
	}
	err = writeObject(ctx, nonPublicObj, "text/plain", []byte("b"))
	if got, want := errCode(err), 401; got != want {
		t.Errorf("got code %d; want %d\nerror: %v", got, want, err)
	}
}

func TestIntegration_ReadCRC(t *testing.T) {
	// Test that the checksum is handled correctly when reading files.
	// For gzipped files, see https://github.com/GoogleCloudPlatform/google-cloud-dotnet/issues/1641.
	if testing.Short() && !replaying {
		t.Skip("Integration tests skipped in short mode")
	}

	const (
		// This is an uncompressed file.
		// See https://cloud.google.com/storage/docs/public-datasets/landsat
		uncompressedBucket = "gcp-public-data-landsat"
		uncompressedObject = "LC08/01/001/002/LC08_L1GT_001002_20160817_20170322_01_T2/LC08_L1GT_001002_20160817_20170322_01_T2_ANG.txt"

		gzippedObject = "gzipped-text.txt"
	)
	ctx := context.Background()
	client, err := newTestClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	h := testHelper{t}
	defer client.Close()

	// Create gzipped object.
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Name = gzippedObject
	if _, err := zw.Write([]byte("gzipped object data")); err != nil {
		t.Fatalf("creating gzip: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("closing gzip writer: %v", err)
	}
	w := client.Bucket(bucketName).Object(gzippedObject).NewWriter(ctx)
	w.ContentEncoding = "gzip"
	w.ContentType = "text/plain"
	h.mustWrite(w, buf.Bytes())

	for _, test := range []struct {
		desc           string
		obj            *ObjectHandle
		offset, length int64
		readCompressed bool // don't decompress a gzipped file

		wantErr   bool
		wantCheck bool // Should Reader try to check the CRC?
	}{
		{
			desc:           "uncompressed, entire file",
			obj:            client.Bucket(uncompressedBucket).Object(uncompressedObject),
			offset:         0,
			length:         -1,
			readCompressed: false,
			wantCheck:      true,
		},
		{
			desc:           "uncompressed, entire file, don't decompress",
			obj:            client.Bucket(uncompressedBucket).Object(uncompressedObject),
			offset:         0,
			length:         -1,
			readCompressed: true,
			wantCheck:      true,
		},
		{
			desc:           "uncompressed, suffix",
			obj:            client.Bucket(uncompressedBucket).Object(uncompressedObject),
			offset:         1,
			length:         -1,
			readCompressed: false,
			wantCheck:      false,
		},
		{
			desc:           "uncompressed, prefix",
			obj:            client.Bucket(uncompressedBucket).Object(uncompressedObject),
			offset:         0,
			length:         18,
			readCompressed: false,
			wantCheck:      false,
		},
		{
			// When a gzipped file is unzipped on read, we can't verify the checksum
			// because it was computed against the zipped contents. We can detect
			// this case using http.Response.Uncompressed.
			desc:           "compressed, entire file, unzipped",
			obj:            client.Bucket(bucketName).Object(gzippedObject),
			offset:         0,
			length:         -1,
			readCompressed: false,
			wantCheck:      false,
		},
		{
			// When we read a gzipped file uncompressed, it's like reading a regular file:
			// the served content and the CRC match.
			desc:           "compressed, entire file, read compressed",
			obj:            client.Bucket(bucketName).Object(gzippedObject),
			offset:         0,
			length:         -1,
			readCompressed: true,
			wantCheck:      true,
		},
		{
			desc:           "compressed, partial, server unzips",
			obj:            client.Bucket(bucketName).Object(gzippedObject),
			offset:         1,
			length:         8,
			readCompressed: false,
			wantErr:        true, // GCS can't serve part of a gzipped object
			wantCheck:      false,
		},
		{
			desc:           "compressed, partial, read compressed",
			obj:            client.Bucket(bucketName).Object(gzippedObject),
			offset:         1,
			length:         8,
			readCompressed: true,
			wantCheck:      false,
		},
	} {
		obj := test.obj.ReadCompressed(test.readCompressed)
		r, err := obj.NewRangeReader(ctx, test.offset, test.length)
		if err != nil {
			if test.wantErr {
				continue
			}
			t.Fatalf("%s: %v", test.desc, err)
		}
		if got, want := r.checkCRC, test.wantCheck; got != want {
			t.Errorf("%s, checkCRC: got %t, want %t", test.desc, got, want)
		}
		_, err = ioutil.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Fatalf("%s: %v", test.desc, err)
		}
	}
}

func TestIntegration_CancelWrite(t *testing.T) {
	// Verify that canceling the writer's context immediately stops uploading an object.
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	bkt := client.Bucket(bucketName)

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	obj := bkt.Object("cancel-write")
	w := obj.NewWriter(cctx)
	w.ChunkSize = googleapi.MinUploadChunkSize
	buf := make([]byte, w.ChunkSize)
	// Write the first chunk. This is read in its entirety before sending the request
	// (see google.golang.org/api/gensupport.PrepareUpload), so we expect it to return
	// without error.
	_, err := w.Write(buf)
	if err != nil {
		t.Fatal(err)
	}
	// Now cancel the context.
	cancel()
	// The next Write should return context.Canceled.
	_, err = w.Write(buf)
	// TODO: Once we drop support for Go versions < 1.13, use errors.Is() to
	// check for context cancellation instead.
	if err != context.Canceled && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("got %v, wanted context.Canceled", err)
	}
	// The Close should too.
	err = w.Close()
	if err != context.Canceled && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("got %v, wanted context.Canceled", err)
	}
}

func TestIntegration_UpdateCORS(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	initialSettings := []CORS{
		{
			MaxAge:          time.Hour,
			Methods:         []string{"POST"},
			Origins:         []string{"some-origin.com"},
			ResponseHeaders: []string{"foo-bar"},
		},
	}

	for _, test := range []struct {
		input []CORS
		want  []CORS
	}{
		{
			input: []CORS{
				{
					MaxAge:          time.Hour,
					Methods:         []string{"GET"},
					Origins:         []string{"*"},
					ResponseHeaders: []string{"some-header"},
				},
			},
			want: []CORS{
				{
					MaxAge:          time.Hour,
					Methods:         []string{"GET"},
					Origins:         []string{"*"},
					ResponseHeaders: []string{"some-header"},
				},
			},
		},
		{
			input: []CORS{},
			want:  nil,
		},
		{
			input: nil,
			want: []CORS{
				{
					MaxAge:          time.Hour,
					Methods:         []string{"POST"},
					Origins:         []string{"some-origin.com"},
					ResponseHeaders: []string{"foo-bar"},
				},
			},
		},
	} {
		bkt := client.Bucket(uidSpace.New())
		h.mustCreate(bkt, testutil.ProjID(), &BucketAttrs{CORS: initialSettings})
		defer h.mustDeleteBucket(bkt)
		h.mustUpdateBucket(bkt, BucketAttrsToUpdate{CORS: test.input})
		attrs := h.mustBucketAttrs(bkt)
		if diff := testutil.Diff(attrs.CORS, test.want); diff != "" {
			t.Errorf("input: %v\ngot=-, want=+:\n%s", test.input, diff)
		}
	}
}

func TestIntegration_UpdateDefaultEventBasedHold(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	bkt := client.Bucket(uidSpace.New())
	h.mustCreate(bkt, testutil.ProjID(), &BucketAttrs{})
	defer h.mustDeleteBucket(bkt)
	attrs := h.mustBucketAttrs(bkt)
	if attrs.DefaultEventBasedHold != false {
		t.Errorf("got=%v, want=%v", attrs.DefaultEventBasedHold, false)
	}

	h.mustUpdateBucket(bkt, BucketAttrsToUpdate{DefaultEventBasedHold: true})
	attrs = h.mustBucketAttrs(bkt)
	if attrs.DefaultEventBasedHold != true {
		t.Errorf("got=%v, want=%v", attrs.DefaultEventBasedHold, true)
	}

	// Omitting it should leave the value unchanged.
	h.mustUpdateBucket(bkt, BucketAttrsToUpdate{RequesterPays: true})
	attrs = h.mustBucketAttrs(bkt)
	if attrs.DefaultEventBasedHold != true {
		t.Errorf("got=%v, want=%v", attrs.DefaultEventBasedHold, true)
	}
}

func TestIntegration_UpdateEventBasedHold(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	bkt := client.Bucket(uidSpace.New())
	h.mustCreate(bkt, testutil.ProjID(), &BucketAttrs{})
	obj := bkt.Object("some-obj")
	h.mustWrite(obj.NewWriter(ctx), randomContents())

	defer func() {
		h.mustUpdateObject(obj, ObjectAttrsToUpdate{EventBasedHold: false})
		h.mustDeleteObject(obj)
		h.mustDeleteBucket(bkt)
	}()

	attrs := h.mustObjectAttrs(obj)
	if attrs.EventBasedHold != false {
		t.Fatalf("got=%v, want=%v", attrs.EventBasedHold, false)
	}

	h.mustUpdateObject(obj, ObjectAttrsToUpdate{EventBasedHold: true})
	attrs = h.mustObjectAttrs(obj)
	if attrs.EventBasedHold != true {
		t.Fatalf("got=%v, want=%v", attrs.EventBasedHold, true)
	}

	// Omitting it should leave the value unchanged.
	h.mustUpdateObject(obj, ObjectAttrsToUpdate{ContentType: "foo"})
	attrs = h.mustObjectAttrs(obj)
	if attrs.EventBasedHold != true {
		t.Fatalf("got=%v, want=%v", attrs.EventBasedHold, true)
	}
}

func TestIntegration_UpdateTemporaryHold(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	bkt := client.Bucket(uidSpace.New())
	h.mustCreate(bkt, testutil.ProjID(), &BucketAttrs{})
	obj := bkt.Object("some-obj")
	h.mustWrite(obj.NewWriter(ctx), randomContents())

	defer func() {
		h.mustUpdateObject(obj, ObjectAttrsToUpdate{TemporaryHold: false})
		h.mustDeleteObject(obj)
		h.mustDeleteBucket(bkt)
	}()

	attrs := h.mustObjectAttrs(obj)
	if attrs.TemporaryHold != false {
		t.Fatalf("got=%v, want=%v", attrs.TemporaryHold, false)
	}

	h.mustUpdateObject(obj, ObjectAttrsToUpdate{TemporaryHold: true})
	attrs = h.mustObjectAttrs(obj)
	if attrs.TemporaryHold != true {
		t.Fatalf("got=%v, want=%v", attrs.TemporaryHold, true)
	}

	// Omitting it should leave the value unchanged.
	h.mustUpdateObject(obj, ObjectAttrsToUpdate{ContentType: "foo"})
	attrs = h.mustObjectAttrs(obj)
	if attrs.TemporaryHold != true {
		t.Fatalf("got=%v, want=%v", attrs.TemporaryHold, true)
	}
}

func TestIntegration_UpdateRetentionExpirationTime(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	bkt := client.Bucket(uidSpace.New())
	h.mustCreate(bkt, testutil.ProjID(), &BucketAttrs{RetentionPolicy: &RetentionPolicy{RetentionPeriod: time.Hour}})
	obj := bkt.Object("some-obj")
	h.mustWrite(obj.NewWriter(ctx), randomContents())

	defer func() {
		h.mustUpdateBucket(bkt, BucketAttrsToUpdate{RetentionPolicy: &RetentionPolicy{RetentionPeriod: 0}})

		// RetentionPeriod of less than a day is explicitly called out
		// as best effort and not guaranteed, so let's log problems deleting
		// objects instead of failing.
		if err := obj.Delete(context.Background()); err != nil {
			t.Logf("%s: object delete: %v", loc(), err)
		}
		if err := bkt.Delete(context.Background()); err != nil {
			t.Logf("%s: bucket delete: %v", loc(), err)
		}
	}()

	attrs := h.mustObjectAttrs(obj)
	if attrs.RetentionExpirationTime == (time.Time{}) {
		t.Fatalf("got=%v, wanted a non-zero value", attrs.RetentionExpirationTime)
	}
}

func TestIntegration_CustomTime(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	// Create object with CustomTime.
	bkt := client.Bucket(bucketName)
	obj := bkt.Object("custom-time-obj")
	w := obj.NewWriter(ctx)
	ct := time.Date(2020, 8, 25, 12, 12, 12, 0, time.UTC)
	w.ObjectAttrs.CustomTime = ct
	h.mustWrite(w, randomContents())

	// Validate that CustomTime has been set
	checkCustomTime := func(want time.Time) error {
		attrs, err := obj.Attrs(ctx)
		if err != nil {
			return fmt.Errorf("failed to get object attrs: %v", err)
		}
		if got := attrs.CustomTime; got != want {
			return fmt.Errorf("CustomTime not set correctly: got %+v, want %+v", got, ct)
		}
		return nil
	}

	if err := checkCustomTime(ct); err != nil {
		t.Fatalf("checking CustomTime: %v", err)
	}

	// Update CustomTime to the future should succeed.
	laterTime := ct.Add(10 * time.Hour)
	if _, err := obj.Update(ctx, ObjectAttrsToUpdate{CustomTime: laterTime}); err != nil {
		t.Fatalf("updating CustomTime: %v", err)
	}

	// Update CustomTime to the past should give error.
	earlierTime := ct.Add(5 * time.Hour)
	if _, err := obj.Update(ctx, ObjectAttrsToUpdate{CustomTime: earlierTime}); err == nil {
		t.Fatalf("backdating CustomTime: expected error, got none")
	}

	// Zero value for CustomTime should be ignored.
	if _, err := obj.Update(ctx, ObjectAttrsToUpdate{}); err != nil {
		t.Fatalf("empty update: %v", err)
	}
	if err := checkCustomTime(laterTime); err != nil {
		t.Fatalf("after sending zero value: %v", err)
	}
}

func TestIntegration_UpdateRetentionPolicy(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	initial := &RetentionPolicy{RetentionPeriod: time.Minute}

	for _, test := range []struct {
		input *RetentionPolicy
		want  *RetentionPolicy
	}{
		{ // Update
			input: &RetentionPolicy{RetentionPeriod: time.Hour},
			want:  &RetentionPolicy{RetentionPeriod: time.Hour},
		},
		{ // Update even with timestamp (EffectiveTime should be ignored)
			input: &RetentionPolicy{RetentionPeriod: time.Hour, EffectiveTime: time.Now()},
			want:  &RetentionPolicy{RetentionPeriod: time.Hour},
		},
		{ // Remove
			input: &RetentionPolicy{},
			want:  nil,
		},
		{ // Remove even with timestamp (EffectiveTime should be ignored)
			input: &RetentionPolicy{EffectiveTime: time.Now()},
			want:  nil,
		},
		{ // Ignore
			input: nil,
			want:  initial,
		},
	} {
		bkt := client.Bucket(uidSpace.New())
		h.mustCreate(bkt, testutil.ProjID(), &BucketAttrs{RetentionPolicy: initial})
		defer h.mustDeleteBucket(bkt)
		h.mustUpdateBucket(bkt, BucketAttrsToUpdate{RetentionPolicy: test.input})
		attrs := h.mustBucketAttrs(bkt)
		if attrs.RetentionPolicy != nil && attrs.RetentionPolicy.EffectiveTime.Unix() == 0 {
			// Should be set by the server and parsed by the client
			t.Fatal("EffectiveTime should be set, but it was not")
		}
		if diff := testutil.Diff(attrs.RetentionPolicy, test.want, cmpopts.IgnoreTypes(time.Time{})); diff != "" {
			t.Errorf("input: %v\ngot=-, want=+:\n%s", test.input, diff)
		}
	}
}

func TestIntegration_DeleteObjectInBucketWithRetentionPolicy(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	bkt := client.Bucket(uidSpace.New())
	h.mustCreate(bkt, testutil.ProjID(), &BucketAttrs{RetentionPolicy: &RetentionPolicy{RetentionPeriod: 25 * time.Hour}})

	oh := bkt.Object("some-object")
	if err := writeObject(ctx, oh, "text/plain", []byte("hello world")); err != nil {
		t.Fatal(err)
	}

	if err := oh.Delete(ctx); err == nil {
		t.Fatal("expected to err deleting an object in a bucket with retention period, but got nil")
	}

	// Remove the retention period
	h.mustUpdateBucket(bkt, BucketAttrsToUpdate{RetentionPolicy: &RetentionPolicy{}})
	// Deleting with retry, as bucket metadata changes
	// can take some time to propagate.
	err := retry(ctx, func() error {
		return oh.Delete(ctx)
	}, nil)
	if err != nil {
		h.t.Fatalf("%s: object delete: %v", loc(), err)
	}
	h.mustDeleteBucket(bkt)
}

func TestIntegration_LockBucket(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	bkt := client.Bucket(uidSpace.New())
	h.mustCreate(bkt, testutil.ProjID(), &BucketAttrs{RetentionPolicy: &RetentionPolicy{RetentionPeriod: time.Hour * 25}})
	attrs := h.mustBucketAttrs(bkt)
	if attrs.RetentionPolicy.IsLocked {
		t.Fatal("Expected bucket to begin unlocked, but it was not")
	}
	err := bkt.If(BucketConditions{MetagenerationMatch: attrs.MetaGeneration}).LockRetentionPolicy(ctx)
	if err != nil {
		t.Fatal("could not lock", err)
	}

	attrs = h.mustBucketAttrs(bkt)
	if !attrs.RetentionPolicy.IsLocked {
		t.Fatal("Expected bucket to be locked, but it was not")
	}

	_, err = bkt.Update(ctx, BucketAttrsToUpdate{RetentionPolicy: &RetentionPolicy{RetentionPeriod: time.Hour}})
	if err == nil {
		t.Fatal("Expected error updating locked bucket, got nil")
	}
}

func TestIntegration_LockBucket_MetagenerationRequired(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	bkt := client.Bucket(uidSpace.New())
	h.mustCreate(bkt, testutil.ProjID(), &BucketAttrs{
		RetentionPolicy: &RetentionPolicy{RetentionPeriod: time.Hour * 25},
	})
	err := bkt.LockRetentionPolicy(ctx)
	if err == nil {
		t.Fatal("expected error locking bucket without metageneration condition, got nil")
	}
}

func TestIntegration_KMS(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	keyRingName := os.Getenv("GCLOUD_TESTS_GOLANG_KEYRING")
	if keyRingName == "" {
		t.Fatal("GCLOUD_TESTS_GOLANG_KEYRING must be set. See CONTRIBUTING.md for details")
	}
	keyName1 := keyRingName + "/cryptoKeys/key1"
	keyName2 := keyRingName + "/cryptoKeys/key2"
	contents := []byte("my secret")

	write := func(obj *ObjectHandle, setKey bool) {
		w := obj.NewWriter(ctx)
		if setKey {
			w.KMSKeyName = keyName1
		}
		h.mustWrite(w, contents)
	}

	checkRead := func(obj *ObjectHandle) {
		got := h.mustRead(obj)
		if !bytes.Equal(got, contents) {
			t.Errorf("got %v, want %v", got, contents)
		}
		attrs := h.mustObjectAttrs(obj)
		if len(attrs.KMSKeyName) < len(keyName1) || attrs.KMSKeyName[:len(keyName1)] != keyName1 {
			t.Errorf("got %q, want %q", attrs.KMSKeyName, keyName1)
		}
	}

	// Write an object with a key, then read it to verify its contents and the presence of the key name.
	bkt := client.Bucket(bucketName)
	obj := bkt.Object("kms")
	write(obj, true)
	checkRead(obj)
	h.mustDeleteObject(obj)

	// Encrypt an object with a CSEK, then copy it using a CMEK.
	src := bkt.Object("csek").Key(testEncryptionKey)
	if err := writeObject(ctx, src, "text/plain", contents); err != nil {
		t.Fatal(err)
	}
	dest := bkt.Object("cmek")
	c := dest.CopierFrom(src)
	c.DestinationKMSKeyName = keyName1
	if _, err := c.Run(ctx); err != nil {
		t.Fatal(err)
	}
	checkRead(dest)
	src.Delete(ctx)
	dest.Delete(ctx)

	// Create a bucket with a default key, then write and read an object.
	bkt = client.Bucket(uidSpace.New())
	h.mustCreate(bkt, testutil.ProjID(), &BucketAttrs{
		Location:   "US",
		Encryption: &BucketEncryption{DefaultKMSKeyName: keyName1},
	})
	defer h.mustDeleteBucket(bkt)

	attrs := h.mustBucketAttrs(bkt)
	if got, want := attrs.Encryption.DefaultKMSKeyName, keyName1; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	obj = bkt.Object("kms")
	write(obj, false)
	checkRead(obj)
	h.mustDeleteObject(obj)

	// Update the bucket's default key to a different name.
	// (This key doesn't have to exist.)
	attrs = h.mustUpdateBucket(bkt, BucketAttrsToUpdate{Encryption: &BucketEncryption{DefaultKMSKeyName: keyName2}})
	if got, want := attrs.Encryption.DefaultKMSKeyName, keyName2; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	attrs = h.mustBucketAttrs(bkt)
	if got, want := attrs.Encryption.DefaultKMSKeyName, keyName2; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	// Remove the default KMS key.
	attrs = h.mustUpdateBucket(bkt, BucketAttrsToUpdate{Encryption: &BucketEncryption{DefaultKMSKeyName: ""}})
	if attrs.Encryption != nil {
		t.Fatalf("got %#v, want nil", attrs.Encryption)
	}
}

func TestIntegration_PredefinedACLs(t *testing.T) {
	check := func(msg string, rs []ACLRule, i int, wantEntity ACLEntity, wantRole ACLRole) {
		if i >= len(rs) {
			t.Errorf("%s: no rule at index %d", msg, i)
			return
		}
		got := rs[i]
		if got.Entity != wantEntity || got.Role != wantRole {
			t.Errorf("%s[%d]: got %+v, want Entity %s and Role %s",
				msg, i, got, wantEntity, wantRole)
		}
	}
	checkPrefix := func(msg string, rs []ACLRule, i int, wantPrefix string, wantRole ACLRole) {
		if i >= len(rs) {
			t.Errorf("%s: no rule at index %d", msg, i)
			return
		}
		got := rs[i]
		if !strings.HasPrefix(string(got.Entity), wantPrefix) || got.Role != wantRole {
			t.Errorf("%s[%d]: got %+v, want Entity %s... and Role %s",
				msg, i, got, wantPrefix, wantRole)
		}
	}

	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	h := testHelper{t}

	bkt := client.Bucket(uidSpace.New())
	h.mustCreate(bkt, testutil.ProjID(), &BucketAttrs{
		PredefinedACL:              "authenticatedRead",
		PredefinedDefaultObjectACL: "publicRead",
	})
	defer h.mustDeleteBucket(bkt)
	attrs := h.mustBucketAttrs(bkt)
	checkPrefix("Bucket.ACL", attrs.ACL, 0, "project-owners", RoleOwner)
	check("Bucket.ACL", attrs.ACL, 1, AllAuthenticatedUsers, RoleReader)
	check("DefaultObjectACL", attrs.DefaultObjectACL, 0, AllUsers, RoleReader)

	// Bucket update
	attrs = h.mustUpdateBucket(bkt, BucketAttrsToUpdate{
		PredefinedACL:              "private",
		PredefinedDefaultObjectACL: "authenticatedRead",
	})
	checkPrefix("Bucket.ACL update", attrs.ACL, 0, "project-owners", RoleOwner)
	check("DefaultObjectACL update", attrs.DefaultObjectACL, 0, AllAuthenticatedUsers, RoleReader)

	// Object creation
	obj := bkt.Object("private")
	w := obj.NewWriter(ctx)
	w.PredefinedACL = "authenticatedRead"
	h.mustWrite(w, []byte("hello"))
	defer h.mustDeleteObject(obj)
	checkPrefix("Object.ACL", w.Attrs().ACL, 0, "user", RoleOwner)
	check("Object.ACL", w.Attrs().ACL, 1, AllAuthenticatedUsers, RoleReader)

	// Object update
	oattrs := h.mustUpdateObject(obj, ObjectAttrsToUpdate{PredefinedACL: "private"})
	checkPrefix("Object.ACL update", oattrs.ACL, 0, "user", RoleOwner)
	if got := len(oattrs.ACL); got != 1 {
		t.Errorf("got %d ACLs, want 1", got)
	}

	// Copy
	dst := bkt.Object("dst")
	copier := dst.CopierFrom(obj)
	copier.PredefinedACL = "publicRead"
	oattrs, err := copier.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer h.mustDeleteObject(dst)
	// The copied object still retains the "private" ACL of the source object.
	checkPrefix("Copy dest", oattrs.ACL, 0, "user", RoleOwner)
	check("Copy dest", oattrs.ACL, 1, AllUsers, RoleReader)

	// Compose
	comp := bkt.Object("comp")
	composer := comp.ComposerFrom(obj, dst)
	composer.PredefinedACL = "authenticatedRead"
	oattrs, err = composer.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer h.mustDeleteObject(comp)
	// The composed object still retains the "private" ACL.
	checkPrefix("Copy dest", oattrs.ACL, 0, "user", RoleOwner)
	check("Copy dest", oattrs.ACL, 1, AllAuthenticatedUsers, RoleReader)
}

func TestIntegration_ServiceAccount(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()

	s, err := client.ServiceAccount(ctx, testutil.ProjID())
	if err != nil {
		t.Fatal(err)
	}
	want := "@gs-project-accounts.iam.gserviceaccount.com"
	if !strings.Contains(s, want) {
		t.Fatalf("got %v, want to contain %v", s, want)
	}
}

func TestIntegration_ReaderAttrs(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()
	bkt := client.Bucket(bucketName)

	const defaultType = "text/plain"
	obj := "some-object"
	c := randomContents()
	if err := writeObject(ctx, bkt.Object(obj), defaultType, c); err != nil {
		t.Errorf("Write for %v failed with %v", obj, err)
	}
	oh := bkt.Object(obj)

	rc, err := oh.NewReader(ctx)
	if err != nil {
		t.Fatal(err)
	}

	attrs, err := oh.Attrs(ctx)
	if err != nil {
		t.Fatal(err)
	}

	got := rc.Attrs
	want := ReaderObjectAttrs{
		Size:            attrs.Size,
		ContentType:     attrs.ContentType,
		ContentEncoding: attrs.ContentEncoding,
		CacheControl:    attrs.CacheControl,
		LastModified:    got.LastModified, // ignored, tested separately
		Generation:      attrs.Generation,
		Metageneration:  attrs.Metageneration,
	}
	if got != want {
		t.Fatalf("got %v, wanted %v", got, want)
	}

	if got.LastModified.IsZero() {
		t.Fatal("LastModified is 0, should be >0")
	}
}

// Test that context cancellation correctly stops a download before completion.
func TestIntegration_ReaderCancel(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()

	bkt := client.Bucket(bucketName)

	// Upload a 1MB object.
	obj := bkt.Object("reader-cancel-obj")
	w := obj.NewWriter(ctx)
	c := randomContents()
	for i := 0; i < 62500; i++ {
		if _, err := w.Write(c); err != nil {
			t.Fatalf("writer.Write: %v", err)
		}

	}
	w.Close()

	// Create a reader (which makes a GET request to GCS and opens the body to
	// read the object) and then cancel the context before reading.
	readerCtx, cancel := context.WithCancel(ctx)
	r, err := obj.NewReader(readerCtx)
	if err != nil {
		t.Fatalf("obj.NewReader: %v", err)
	}
	defer r.Close()

	cancel()

	// Read the object 1KB a time. We cannot guarantee that Reads will return a
	// context canceled error immediately, but they should always do so before we
	// reach EOF.
	var readErr error
	for i := 0; i < 1000; i++ {
		buf := make([]byte, 1000)
		_, readErr = r.Read(buf)
		if readErr != nil {
			if readErr == context.Canceled {
				return
			}
			break
		}
	}
	t.Fatalf("Reader.Read: got %v, want context.Canceled", readErr)
}

// Ensures that a file stored with a:
// * Content-Encoding of "gzip"
// * Content-Type of "text/plain"
// will be properly served back.
// See:
//  * https://cloud.google.com/storage/docs/transcoding#transcoding_and_gzip
//  * https://github.com/googleapis/google-cloud-go/issues/1800
func TestIntegration_NewReaderWithContentEncodingGzip(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()

	h := testHelper{t}

	projectID := testutil.ProjID()
	bkt := client.Bucket(uidSpace.New())
	h.mustCreate(bkt, projectID, nil)
	defer h.mustDeleteBucket(bkt)
	obj := bkt.Object("decompressive-transcoding")
	original := bytes.Repeat([]byte("a"), 4<<10)

	// Wrap the file upload in a retry.
	// TODO: Investigate removing retry after resolving
	// https://github.com/googleapis/google-api-go-client/issues/392.
	err := retry(ctx, func() error {
		// Firstly upload the gzip compressed file.
		w := obj.NewWriter(ctx)
		// Compress and upload the content.
		gzw := gzip.NewWriter(w)
		if _, err := gzw.Write(original); err != nil {
			return fmt.Errorf("Failed to compress content: %v", err)
		}
		if err := gzw.Close(); err != nil {
			return fmt.Errorf("Failed to compress content: %v", err)
		}
		if err := w.Close(); err != nil {
			return fmt.Errorf("Failed to finish uploading the file: %v", err)
		}
		return nil
	},
		nil)

	defer h.mustDeleteObject(obj)

	// Now update the Content-Encoding and Content-Type to enable
	// decompressive transcoding.
	updatedAttrs, err := obj.Update(ctx, ObjectAttrsToUpdate{
		ContentEncoding: "gzip",
		ContentType:     "text/plain",
	})
	if err != nil {
		t.Fatalf("Attribute update failure: %v", err)
	}
	if g, w := updatedAttrs.ContentEncoding, "gzip"; g != w {
		t.Fatalf("ContentEncoding mismtach:\nGot:  %q\nWant: %q", g, w)
	}
	if g, w := updatedAttrs.ContentType, "text/plain"; g != w {
		t.Fatalf("ContentType mismtach:\nGot:  %q\nWant: %q", g, w)
	}

	rWhole, err := obj.NewReader(ctx)
	if err != nil {
		t.Fatalf("Failed to create wholesome reader: %v", err)
	}
	blobWhole, err := ioutil.ReadAll(rWhole)
	rWhole.Close()
	if err != nil {
		t.Fatalf("Failed to read the whole body: %v", err)
	}
	if g, w := blobWhole, original; !bytes.Equal(g, w) {
		t.Fatalf("Body mismatch\nGot:\n%s\n\nWant:\n%s", g, w)
	}

	// Now try a range read, which should return the whole body anyways since
	// for decompressive transcoding, range requests ARE IGNORED by Cloud Storage.
	r2kBTo3kB, err := obj.NewRangeReader(ctx, 2<<10, 3<<10)
	if err != nil {
		t.Fatalf("Failed to create range reader: %v", err)
	}
	blob2kBTo3kB, err := ioutil.ReadAll(r2kBTo3kB)
	r2kBTo3kB.Close()
	if err != nil {
		t.Fatalf("Failed to read with the 2kB to 3kB range request: %v", err)
	}
	// The ENTIRE body MUST be served back regardless of the requested range.
	if g, w := blob2kBTo3kB, original; !bytes.Equal(g, w) {
		t.Fatalf("Body mismatch\nGot:\n%s\n\nWant:\n%s", g, w)
	}
}

func TestIntegration_HMACKey(t *testing.T) {
	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()

	projectID := testutil.ProjID()

	// Use the service account email from the user's credentials. Requires that the
	// credentials are set via a JSON credentials file.
	// Note that a service account may only have up to 5 active HMAC keys at once; if
	// we see flakes because of this, we should consider switching to using a project
	// pool.
	credentials := testutil.CredentialsEnv(ctx, "GCLOUD_TESTS_GOLANG_KEY")
	if credentials == nil {
		t.Fatal("credentials could not be determined, is GCLOUD_TESTS_GOLANG_KEY set correctly?")
	}
	if credentials.JSON == nil {
		t.Fatal("could not read the JSON key file, is GCLOUD_TESTS_GOLANG_KEY set correctly?")
	}
	conf, err := google.JWTConfigFromJSON(credentials.JSON)
	if err != nil {
		t.Fatal(err)
	}
	serviceAccountEmail := conf.Email

	hmacKey, err := client.CreateHMACKey(ctx, projectID, serviceAccountEmail)
	if err != nil {
		t.Fatalf("Failed to create HMACKey: %v", err)
	}
	if hmacKey == nil {
		t.Fatal("Unexpectedly got back a nil HMAC key")
	}

	if hmacKey.State != Active {
		t.Fatalf("Unexpected state %q, expected %q", hmacKey.State, Active)
	}

	hkh := client.HMACKeyHandle(projectID, hmacKey.AccessID)
	// 1. Ensure that we CANNOT delete an ACTIVE key.
	if err := hkh.Delete(ctx); err == nil {
		t.Fatal("Unexpectedly deleted key whose state is ACTIVE: No error from Delete.")
	}

	invalidStates := []HMACState{"", Deleted, "active", "inactive", "foo_bar"}
	for _, invalidState := range invalidStates {
		t.Run("invalid-"+string(invalidState), func(t *testing.T) {
			_, err := hkh.Update(ctx, HMACKeyAttrsToUpdate{
				State: invalidState,
			})
			if err == nil {
				t.Fatal("Unexpectedly succeeded")
			}
			invalidStateMsg := fmt.Sprintf(`storage: invalid state %q for update, must be either "ACTIVE" or "INACTIVE"`, invalidState)
			if err.Error() != invalidStateMsg {
				t.Fatalf("Mismatched error: got:  %q\nwant: %q", err, invalidStateMsg)
			}
		})
	}

	// 2.1. Setting the State to Inactive should succeed.
	hu, err := hkh.Update(ctx, HMACKeyAttrsToUpdate{
		State: Inactive,
	})
	if err != nil {
		t.Fatalf("Unexpected Update failure: %v", err)
	}
	if got, want := hu.State, Inactive; got != want {
		t.Fatalf("Unexpected updated state %q, expected %q", got, want)
	}

	// 2.2. Setting the State back to Active should succeed.
	hu, err = hkh.Update(ctx, HMACKeyAttrsToUpdate{
		State: Active,
	})
	if err != nil {
		t.Fatalf("Unexpected Update failure: %v", err)
	}
	if got, want := hu.State, Active; got != want {
		t.Fatalf("Unexpected updated state %q, expected %q", got, want)
	}

	// 3. Verify that keys are listed as expected.
	iter := client.ListHMACKeys(ctx, projectID)
	count := 0
	for ; ; count++ {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatalf("Failed to ListHMACKeys: %v", err)
		}
	}
	if count == 0 {
		t.Fatal("Failed to list any HMACKeys")
	}

	// 4. Finally set it to back to Inactive and
	// then retry the deletion which should now succeed.
	_, _ = hkh.Update(ctx, HMACKeyAttrsToUpdate{
		State: Inactive,
	})
	if err := hkh.Delete(ctx); err != nil {
		t.Fatalf("Unexpected deletion failure: %v", err)
	}

	_, err = hkh.Get(ctx)
	if err != nil && !strings.Contains(err.Error(), "404") {
		// If the deleted key has already been garbage collected, a 404 is expected.
		// Other errors should cause a failure and are not expected.
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestIntegration_PostPolicyV4(t *testing.T) {
	jwtConf, err := testutil.JWTConfig()
	if err != nil {
		t.Fatal(err)
	}
	if jwtConf == nil {
		t.Skip("JSON key file is not present")
	}

	ctx := context.Background()
	client := testConfig(ctx, t)
	defer client.Close()

	projectID := testutil.ProjID()
	newBucketName := uidSpace.New()
	b := client.Bucket(newBucketName)
	h := testHelper{t}
	h.mustCreate(b, projectID, nil)
	defer h.mustDeleteBucket(b)

	statusCodeToRespond := 200
	opts := &PostPolicyV4Options{
		GoogleAccessID: jwtConf.Email,
		PrivateKey:     jwtConf.PrivateKey,

		Expires: time.Now().Add(30 * time.Minute),

		Fields: &PolicyV4Fields{
			StatusCodeOnSuccess: statusCodeToRespond,
			ContentType:         "text/plain",
			ACL:                 "public-read",
		},

		// The conditions that the uploaded file will be expected to conform to.
		Conditions: []PostPolicyV4Condition{
			// Make the file a maximum of 10mB.
			ConditionContentLengthRange(0, 10<<20),
			ConditionStartsWith("$acl", "public"),
		},
	}

	objectName := "my-object.txt"
	pv4, err := GenerateSignedPostPolicyV4(newBucketName, objectName, opts)
	if err != nil {
		t.Fatal(err)
	}

	formBuf := new(bytes.Buffer)
	mw := multipart.NewWriter(formBuf)
	for fieldName, value := range pv4.Fields {
		if err := mw.WriteField(fieldName, value); err != nil {
			t.Fatalf("Failed to write form field: %q: %v", fieldName, err)
		}
	}

	// Now let's perform the upload.
	fileBody := bytes.Repeat([]byte("a"), 25)
	mf, err := mw.CreateFormFile("file", "myfile.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mf.Write(fileBody); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	// Compose the HTTP request.
	req, err := http.NewRequest("POST", pv4.URL, formBuf)
	if err != nil {
		t.Fatalf("Failed to compose HTTP request: %v", err)
	}
	// Ensure the Content-Type is derived from the writer.
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if g, w := res.StatusCode, statusCodeToRespond; g != w {
		blob, _ := httputil.DumpResponse(res, true)
		t.Fatalf("Status code in response mismatch: got %d want %d\nBody: %s", g, w, blob)
	}
	io.Copy(ioutil.Discard, res.Body)

	// Verify that the file was properly uploaded, by
	// reading back its attributes and content!
	obj := b.Object(objectName)
	defer h.mustDeleteObject(obj)

	attrs, err := obj.Attrs(ctx)
	if err != nil {
		t.Fatalf("Failed to retrieve attributes: %v", err)
	}
	if g, w := attrs.Size, int64(len(fileBody)); g != w {
		t.Errorf("ContentLength mismatch: got %d want %d", g, w)
	}
	if g, w := attrs.MD5, md5.Sum(fileBody); !bytes.Equal(g, w[:]) {
		t.Errorf("MD5Checksum mismatch\nGot:  %x\nWant: %x", g, w)
	}

	// Compare the uploaded body with the expected.
	rd, err := obj.NewReader(ctx)
	if err != nil {
		t.Fatalf("Failed to create a reader: %v", err)
	}
	gotBody, err := ioutil.ReadAll(rd)
	if err != nil {
		t.Fatalf("Failed to read the body: %v", err)
	}
	if diff := testutil.Diff(string(gotBody), string(fileBody)); diff != "" {
		t.Fatalf("Body mismatch: got - want +\n%s", diff)
	}
}

// Verify that custom scopes passed in by the user are applied correctly.
func TestIntegration_Scopes(t *testing.T) {
	// A default client should be able to write objects since it has scope of
	// FullControl
	ctx := context.Background()
	clientFullControl := testConfig(ctx, t)
	defer clientFullControl.Close()

	bkt := clientFullControl.Bucket(bucketName)
	obj := "FakeObj1"
	contents := []byte("This object should be written successfully\n")
	if err := writeObject(ctx, bkt.Object(obj), "text/plain", contents); err != nil {
		t.Fatalf("writing: %v", err)
	}

	// A client with ReadOnly scope should not be able to write successfully.
	clientReadOnly, err := NewClient(ctx, option.WithScopes(ScopeReadOnly))
	defer clientReadOnly.Close()
	if err != nil {
		t.Fatalf("error creating client: %v", err)
	}

	bkt = clientReadOnly.Bucket(bucketName)
	obj = "FakeObj2"
	contents = []byte("This object should not be written.\n")

	if err := writeObject(ctx, bkt.Object(obj), "text/plain", contents); err == nil {
		t.Fatal("client with ScopeReadOnly was able to write an object unexpectedly.")
	}

}

type testHelper struct {
	t *testing.T
}

func (h testHelper) mustCreate(b *BucketHandle, projID string, attrs *BucketAttrs) {
	if err := b.Create(context.Background(), projID, attrs); err != nil {
		h.t.Fatalf("%s: bucket create: %v", loc(), err)
	}
}

func (h testHelper) mustDeleteBucket(b *BucketHandle) {
	if err := b.Delete(context.Background()); err != nil {
		h.t.Fatalf("%s: bucket delete: %v", loc(), err)
	}
}

func (h testHelper) mustBucketAttrs(b *BucketHandle) *BucketAttrs {
	attrs, err := b.Attrs(context.Background())
	if err != nil {
		h.t.Fatalf("%s: bucket attrs: %v", loc(), err)
	}
	return attrs
}

func (h testHelper) mustUpdateBucket(b *BucketHandle, ua BucketAttrsToUpdate) *BucketAttrs {
	attrs, err := b.Update(context.Background(), ua)
	if err != nil {
		h.t.Fatalf("%s: update: %v", loc(), err)
	}
	return attrs
}

func (h testHelper) mustObjectAttrs(o *ObjectHandle) *ObjectAttrs {
	attrs, err := o.Attrs(context.Background())
	if err != nil {
		h.t.Fatalf("%s: object attrs: %v", loc(), err)
	}
	return attrs
}

func (h testHelper) mustDeleteObject(o *ObjectHandle) {
	if err := o.Delete(context.Background()); err != nil {
		h.t.Fatalf("%s: object delete: %v", loc(), err)
	}
}

func (h testHelper) mustUpdateObject(o *ObjectHandle, ua ObjectAttrsToUpdate) *ObjectAttrs {
	attrs, err := o.Update(context.Background(), ua)
	if err != nil {
		h.t.Fatalf("%s: update: %v", loc(), err)
	}
	return attrs
}

func (h testHelper) mustWrite(w *Writer, data []byte) {
	if _, err := w.Write(data); err != nil {
		w.Close()
		h.t.Fatalf("%s: write: %v", loc(), err)
	}
	if err := w.Close(); err != nil {
		h.t.Fatalf("%s: close write: %v", loc(), err)
	}
}

func (h testHelper) mustRead(obj *ObjectHandle) []byte {
	data, err := readObject(context.Background(), obj)
	if err != nil {
		h.t.Fatalf("%s: read: %v", loc(), err)
	}
	return data
}

func (h testHelper) mustNewReader(obj *ObjectHandle) *Reader {
	r, err := obj.NewReader(context.Background())
	if err != nil {
		h.t.Fatalf("%s: new reader: %v", loc(), err)
	}
	return r
}

func writeObject(ctx context.Context, obj *ObjectHandle, contentType string, contents []byte) error {
	w := obj.NewWriter(ctx)
	w.ContentType = contentType
	w.CacheControl = "public, max-age=60"
	if contents != nil {
		if _, err := w.Write(contents); err != nil {
			_ = w.Close()
			return err
		}
	}
	return w.Close()
}

// loc returns a string describing the file and line of its caller's call site. In
// other words, if a test function calls a helper, and the helper calls loc, then the
// string will refer to the line on which the test function called the helper.
// TODO(jba): use t.Helper once we drop go 1.6.
func loc() string {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		return "???"
	}
	return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}

func readObject(ctx context.Context, obj *ObjectHandle) ([]byte, error) {
	r, err := obj.NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return ioutil.ReadAll(r)
}

// cleanupBuckets deletes the bucket used for testing, as well as old
// testing buckets that weren't cleaned previously.
func cleanupBuckets() error {
	if testing.Short() {
		return nil // Don't clean up in short mode.
	}
	ctx := context.Background()
	client := config(ctx)
	if client == nil {
		return nil // Don't cleanup if we're not configured correctly.
	}
	defer client.Close()
	if err := killBucket(ctx, client, bucketName); err != nil {
		return err
	}

	// Delete buckets whose name begins with our test prefix, and which were
	// created a while ago. (Unfortunately GCS doesn't provide last-modified
	// time, which would be a better way to check for staleness.)
	const expireAge = 24 * time.Hour
	projectID := testutil.ProjID()
	it := client.Buckets(ctx, projectID)
	it.Prefix = testPrefix
	for {
		bktAttrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		if time.Since(bktAttrs.Created) > expireAge {
			log.Printf("deleting bucket %q, which is more than %s old", bktAttrs.Name, expireAge)
			if err := killBucket(ctx, client, bktAttrs.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

// killBucket deletes a bucket and all its objects.
func killBucket(ctx context.Context, client *Client, bucketName string) error {
	bkt := client.Bucket(bucketName)
	// Bucket must be empty to delete.
	it := bkt.Objects(ctx, nil)
	for {
		objAttrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		// Objects with a hold must have the hold released.
		if objAttrs.EventBasedHold || objAttrs.TemporaryHold {
			obj := bkt.Object(objAttrs.Name)
			if _, err := obj.Update(ctx, ObjectAttrsToUpdate{EventBasedHold: false, TemporaryHold: false}); err != nil {
				return fmt.Errorf("removing hold from %q: %v", bucketName+"/"+objAttrs.Name, err)
			}
		}
		if err := bkt.Object(objAttrs.Name).Delete(ctx); err != nil {
			return fmt.Errorf("deleting %q: %v", bucketName+"/"+objAttrs.Name, err)
		}
	}
	// GCS is eventually consistent, so this delete may fail because the
	// replica still sees an object in the bucket. We log the error and expect
	// a later test run to delete the bucket.
	if err := bkt.Delete(ctx); err != nil {
		log.Printf("deleting %q: %v", bucketName, err)
	}
	return nil
}

func randomContents() []byte {
	h := md5.New()
	io.WriteString(h, fmt.Sprintf("hello world%d", rng.Intn(100000)))
	return h.Sum(nil)
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) { return len(p), nil }

// Make a GET request to a URL using an unauthenticated client, and return its contents.
func getURL(url string, headers map[string][]string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header = headers
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	bytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("code=%d, body=%s", res.StatusCode, string(bytes))
	}
	return bytes, nil
}

// Make a PUT request to a URL using an unauthenticated client, and return its contents.
func putURL(url string, headers map[string][]string, payload io.Reader) ([]byte, error) {
	req, err := http.NewRequest("PUT", url, payload)
	if err != nil {
		return nil, err
	}
	req.Header = headers
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	bytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("code=%d, body=%s", res.StatusCode, string(bytes))
	}
	return bytes, nil
}

func namesEqual(obj *ObjectAttrs, bucketName, objectName string) bool {
	return obj.Bucket == bucketName && obj.Name == objectName
}

func keyFileEmail(filename string) (string, error) {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", err
	}
	var v struct {
		ClientEmail string `json:"client_email"`
	}
	if err := json.Unmarshal(bytes, &v); err != nil {
		return "", err
	}
	return v.ClientEmail, nil
}

func containsACL(acls []ACLRule, e ACLEntity, r ACLRole) bool {
	for _, a := range acls {
		if a.Entity == e && a.Role == r {
			return true
		}
	}
	return false
}

func hasRule(acl []ACLRule, rule ACLRule) bool {
	for _, r := range acl {
		if cmp.Equal(r, rule) {
			return true
		}
	}
	return false
}

// retry retries a function call as well as an (optional) correctness check for up
// to 11 seconds. Both call and check must run without error in order to succeed.
// If the timeout is hit, the most recent error from call or check will be returned.
// This function should be used to wrap calls that might cause integration test
// flakes due to delays in propagation (for example, metadata updates).
func retry(ctx context.Context, call func() error, check func() error) error {
	timeout := time.After(11 * time.Second)
	var err error
	for {
		select {
		case <-timeout:
			return err
		default:
		}
		err = call()
		if err == nil {
			if check == nil || check() == nil {
				return nil
			}
			err = check()
		}
		time.Sleep(200 * time.Millisecond)
	}
}
