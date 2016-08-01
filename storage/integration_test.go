// Copyright 2014 Google Inc. All Rights Reserved.
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
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/context"

	"google.golang.org/api/googleapi"
	"google.golang.org/cloud"
	"google.golang.org/cloud/internal/testutil"
)

// suffix is a timestamp-based suffix which is added, where possible, to all
// buckets and objects created by tests. This reduces flakiness when the tests
// are run in parallel and allows automatic cleaning up of artifacts left when
// tests fail.
var suffix = fmt.Sprintf("-t%d", time.Now().UnixNano())

var ranIntegrationTest bool

func TestMain(m *testing.M) {
	// Run the tests, then follow by running any cleanup required.
	exit := m.Run()
	if ranIntegrationTest {
		if err := cleanup(); err != nil {
			log.Fatalf("Post-test cleanup failed: %v", err)
		}
	}
	os.Exit(exit)
}

// testConfig returns the Client used to access GCS and the default bucket
// name to use. testConfig skips the current test if credentials are not
// available or when being run in Short mode.
func testConfig(ctx context.Context, t *testing.T) (*Client, string) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	client, bucket := config(ctx)
	if client == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	ranIntegrationTest = true
	return client, bucket
}

// config is like testConfig, but it doesn't need a *testing.T.
func config(ctx context.Context) (*Client, string) {
	ts := testutil.TokenSource(ctx, ScopeFullControl)
	if ts == nil {
		return nil, ""
	}
	p := testutil.ProjID()
	if p == "" {
		log.Fatal("The project ID must be set. See CONTRIBUTING.md for details")
	}
	client, err := NewClient(ctx, cloud.WithTokenSource(ts))
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}
	return client, p
}

func TestBucketMethods(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	ctx := context.Background()
	ts := testutil.TokenSource(ctx, ScopeFullControl)
	if ts == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	projectID := testutil.ProjID()

	newBucket := projectID + suffix
	t.Logf("Testing with Bucket %q", newBucket)

	client, err := NewClient(ctx, cloud.WithTokenSource(ts))
	if err != nil {
		t.Fatalf("Could not create client: %v", err)
	}
	defer client.Close()

	if err := client.Bucket(newBucket).Create(ctx, projectID, nil); err != nil {
		t.Errorf("Bucket(%v).Create(%v, %v) failed: %v", newBucket, projectID, nil, err)
	}
	if err := client.Bucket(newBucket).Delete(ctx); err != nil {
		t.Errorf("Bucket(%v).Delete failed: %v", newBucket, err)
		t.Logf("TODO: Warning this test left a new bucket in the cloud project, it must be deleted manually")
	}
	attrs := BucketAttrs{
		DefaultObjectACL: []ACLRule{{Entity: "domain-google.com", Role: RoleReader}},
	}
	if err := client.Bucket(newBucket).Create(ctx, projectID, &attrs); err != nil {
		t.Errorf("Bucket(%v).Create(%v, %v) failed: %v", newBucket, projectID, attrs, err)
	}
	if err := client.Bucket(newBucket).Delete(ctx); err != nil {
		t.Errorf("Bucket(%v).Delete failed: %v", newBucket, err)
		t.Logf("TODO: Warning this test left a new bucket in the cloud project, it must be deleted manually")
	}
}

func TestIntegration_ConditionalDelete(t *testing.T) {
	ctx := context.Background()
	client, bucket := testConfig(ctx, t)
	defer client.Close()

	o := client.Bucket(bucket).Object("conddel" + suffix)

	wc := o.NewWriter(ctx)
	wc.ContentType = "text/plain"
	if _, err := wc.Write([]byte("foo")); err != nil {
		t.Fatal(err)
	}
	if err := wc.Close(); err != nil {
		t.Fatal(err)
	}

	gen := wc.Attrs().Generation
	metaGen := wc.Attrs().MetaGeneration

	if err := o.WithConditions(Generation(gen - 1)).Delete(ctx); err == nil {
		t.Fatalf("Unexpected successful delete with Generation")
	}
	if err := o.WithConditions(IfMetaGenerationMatch(metaGen + 1)).Delete(ctx); err == nil {
		t.Fatalf("Unexpected successful delete with IfMetaGenerationMatch")
	}
	if err := o.WithConditions(IfMetaGenerationNotMatch(metaGen)).Delete(ctx); err == nil {
		t.Fatalf("Unexpected successful delete with IfMetaGenerationNotMatch")
	}
	if err := o.WithConditions(Generation(gen)).Delete(ctx); err != nil {
		t.Fatalf("final delete failed: %v", err)
	}
}

func TestObjects(t *testing.T) {
	ctx := context.Background()
	client, bucket := testConfig(ctx, t)
	defer client.Close()

	bkt := client.Bucket(bucket)

	const defaultType = "text/plain"

	// Clear out old objects that might have been left in the bucket.
	if err := clearObjects(ctx, bkt); err != nil {
		t.Fatalf("clearing objects: %v", err)
	}

	// Populate object names and make a map for their contents.
	objects := []string{
		"obj1" + suffix,
		"obj2" + suffix,
		"obj/with/slashes" + suffix,
	}
	contents := make(map[string][]byte)

	// Test Writer.
	for _, obj := range objects {
		t.Logf("Writing %q", obj)
		wc := bkt.Object(obj).NewWriter(ctx)
		wc.ContentType = defaultType
		c := randomContents()
		if _, err := wc.Write(c); err != nil {
			t.Errorf("Write for %v failed with %v", obj, err)
		}
		if err := wc.Close(); err != nil {
			t.Errorf("Close for %v failed with %v", obj, err)
		}
		contents[obj] = c
	}

	testBucketList(t, bkt, objects)
	testObjectIterator(t, bkt, objects)

	// Test Reader.
	for _, obj := range objects {
		t.Logf("Creating a reader to read %v", obj)
		rc, err := bkt.Object(obj).NewReader(ctx)
		if err != nil {
			t.Errorf("Can't create a reader for %v, errored with %v", obj, err)
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
		rc.Close()

		// Test SignedURL
		opts := &SignedURLOptions{
			GoogleAccessID: "xxx@clientid",
			PrivateKey:     dummyKey("rsa"),
			Method:         "GET",
			MD5:            []byte("202cb962ac59075b964b07152d234b70"),
			Expires:        time.Date(2020, time.October, 2, 10, 0, 0, 0, time.UTC),
			ContentType:    "application/json",
			Headers:        []string{"x-header1", "x-header2"},
		}
		u, err := SignedURL(bucket, obj, opts)
		if err != nil {
			t.Fatalf("SignedURL(%q, %q) errored with %v", bucket, obj, err)
		}
		res, err := client.hc.Get(u)
		if err != nil {
			t.Fatalf("Can't get URL %q: %v", u, err)
		}
		slurp, err = ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("Can't ReadAll signed object %v, errored with %v", obj, err)
		}
		if got, want := slurp, contents[obj]; !bytes.Equal(got, want) {
			t.Errorf("Contents (%v) = %q; want %q", obj, got, want)
		}
		res.Body.Close()
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
	} {
		t.Logf("%d: bkt.Object(%v).NewRangeReader(ctx, %d, %d)", i, obj, r.offset, r.length)
		rc, err := bkt.Object(obj).NewRangeReader(ctx, r.offset, r.length)
		if err != nil {
			t.Errorf("%d: Can't create a range reader for %v, errored with %v", i, obj, err)
			continue
		}
		if rc.Size() != objlen {
			t.Errorf("%d: Reader has a content-size of %d, want %d", i, rc.Size(), objlen)
		}
		if rc.Remain() != r.want {
			t.Errorf("%d: Reader's available bytes reported as %d, want %d", i, rc.Remain(), r.want)
		}
		slurp, err := ioutil.ReadAll(rc)
		if err != nil {
			t.Errorf("%d:Can't ReadAll object %v, errored with %v", i, obj, err)
			continue
		}
		if len(slurp) != int(r.want) {
			t.Errorf("%d:RangeReader (%d, %d): Read %d bytes, wanted %d bytes", i, r.offset, r.length, len(slurp), r.want)
			continue
		}
		if got, want := slurp, contents[obj][r.offset:r.offset+r.want]; !bytes.Equal(got, want) {
			t.Errorf("RangeReader (%d, %d) = %q; want %q", r.offset, r.length, got, want)
		}
		rc.Close()
	}

	// Test content encoding
	const zeroCount = 20 << 20
	w := bkt.Object("gzip-test" + suffix).NewWriter(ctx)
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
	r, err := bkt.Object("gzip-test" + suffix).NewReader(ctx)
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

	objName := objects[0]

	// Test NewReader googleapi.Error.
	// Since a 429 or 5xx is hard to cause, we trigger a 416.
	realLen := len(contents[objName])
	_, err = bkt.Object(objName).NewRangeReader(ctx, int64(realLen*2), 10)
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
	o, err := bkt.Object(objName).Attrs(ctx)
	if err != nil {
		t.Error(err)
	}
	if got, want := o.Name, objName; got != want {
		t.Errorf("Name (%v) = %q; want %q", objName, got, want)
	}
	if got, want := o.ContentType, defaultType; got != want {
		t.Errorf("ContentType (%v) = %q; want %q", objName, got, want)
	}
	created := o.Created
	// Check that the object is newer than its containing bucket.
	bAttrs, err := bkt.Attrs(ctx)
	if err != nil {
		t.Error(err)
	}
	if o.Created.Before(bAttrs.Created) {
		t.Errorf("Object %v is older than its containing bucket, %v", o, bAttrs)
	}

	// Test object copy.
	copyName := "copy-" + objName
	copyObj, err := bkt.Object(objName).CopyTo(ctx, bkt.Object(copyName), nil)
	if err != nil {
		t.Errorf("CopyTo failed with %v", err)
	}
	if copyObj.Name != copyName {
		t.Errorf("Copy object's name = %q; want %q", copyObj.Name, copyName)
	}
	if copyObj.Bucket != bucket {
		t.Errorf("Copy object's bucket = %q; want %q", copyObj.Bucket, bucket)
	}

	// Test UpdateAttrs.
	updated, err := bkt.Object(objName).Update(ctx, ObjectAttrs{
		ContentType: "text/html",
		ACL:         []ACLRule{{Entity: "domain-google.com", Role: RoleReader}},
	})
	if err != nil {
		t.Errorf("UpdateAttrs failed with %v", err)
	}
	if want := "text/html"; updated.ContentType != want {
		t.Errorf("updated.ContentType == %q; want %q", updated.ContentType, want)
	}
	if want := created; updated.Created != want {
		t.Errorf("updated.Created == %q; want %q", updated.Created, want)
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
	publicClient, err := NewClient(ctx, cloud.WithBaseHTTP(http.DefaultClient))
	if err != nil {
		t.Fatal(err)
	}
	rc, err := publicClient.Bucket(bucket).Object(publicObj).NewReader(ctx)
	if err != nil {
		t.Error(err)
	}
	slurp, err := ioutil.ReadAll(rc)
	if err != nil {
		t.Errorf("ReadAll failed with %v", err)
	}
	if !bytes.Equal(slurp, contents[publicObj]) {
		t.Errorf("Public object's content: got %q, want %q", slurp, contents[publicObj])
	}
	rc.Close()

	// Test writer error handling.
	wc := publicClient.Bucket(bucket).Object(publicObj).NewWriter(ctx)
	if _, err := wc.Write([]byte("hello")); err != nil {
		t.Errorf("Write unexpectedly failed with %v", err)
	}
	if err = wc.Close(); err == nil {
		t.Error("Close expected an error, found none")
	}

	// Test deleting the copy object.
	if err := bkt.Object(copyName).Delete(ctx); err != nil {
		t.Errorf("Deletion of %v failed with %v", copyName, err)
	}
	// Deleting it a second time should return ErrObjectNotExist.
	if err := bkt.Object(copyName).Delete(ctx); err != ErrObjectNotExist {
		t.Errorf("second deletion of %v = %v; want ErrObjectNotExist", copyName, err)
	}
	_, err = bkt.Object(copyName).Attrs(ctx)
	if err != ErrObjectNotExist {
		t.Errorf("Copy is expected to be deleted, stat errored with %v", err)
	}
}

func clearObjects(ctx context.Context, bkt *BucketHandle) error {
	it := bkt.Objects(ctx, &Query{Prefix: "obj"})
	for {
		attrs, err := it.Next()
		switch err {
		case nil:
			if err := bkt.Object(attrs.Name).Delete(ctx); err != nil {
				return fmt.Errorf("deleting %q: %v", attrs.Name, err)
			}
		case Done:
			return nil
		default:
			return err
		}
	}
}

func testBucketList(t *testing.T, bkt *BucketHandle, objects []string) {
	ctx := context.Background()
	q := &Query{Prefix: "obj"}
	missing := map[string]bool{}
	for _, o := range objects {
		missing[o] = true
	}
	for {
		objs, err := bkt.List(ctx, q)
		if err != nil {
			t.Errorf("List: unexpected error: %v", err)
			break
		}
		for _, oa := range objs.Results {
			delete(missing, oa.Name)
		}
		if objs.Next == nil {
			break
		}
		q = objs.Next
	}
	if len(missing) > 0 {
		t.Errorf("bucket.List: missing %v", missing)
	}
}

func testObjectIterator(t *testing.T, bkt *BucketHandle, objects []string) {
	ctx := context.Background()
	// Collect the list of items we expect: ObjectAttrs in lexical order by name.
	names := make([]string, len(objects))
	copy(names, objects)
	sort.Strings(names)
	var attrs []*ObjectAttrs
	for _, name := range names {
		attr, err := bkt.Object(name).Attrs(ctx)
		if err != nil {
			t.Errorf("Object(%q).Attrs: %v", name, err)
			return
		}
		attrs = append(attrs, attr)
	}

	it := bkt.Objects(ctx, &Query{Prefix: "obj"})
	msg, ok := testutil.TestIteratorNext(attrs, Done, func() (interface{}, error) { return it.Next() })
	if !ok {
		t.Errorf("ObjectIterator.Next: %s", msg)
	}

	msg, ok = testutil.TestIteratorNextPageExact(attrs, Done, DefaultPageSize,
		func() testutil.PagingIterator { return bkt.Objects(ctx, &Query{Prefix: "obj"}) },
		func(i testutil.PagingIterator) (interface{}, error) {
			as, _, err := i.(*ObjectIterator).NextPage()
			return as, err
		})
	if !ok {
		t.Errorf("ObjectIterator.NextPage: %s", msg)
	}

	// TODO(jba): test query.Delimiter != ""
}

func TestACL(t *testing.T) {
	ctx := context.Background()
	client, bucket := testConfig(ctx, t)
	defer client.Close()

	bkt := client.Bucket(bucket)

	entity := ACLEntity("domain-google.com")
	rule := ACLRule{Entity: entity, Role: RoleReader}
	if err := bkt.DefaultObjectACL().Set(ctx, entity, RoleReader); err != nil {
		t.Errorf("Can't put default ACL rule for the bucket, errored with %v", err)
	}
	acl, err := bkt.DefaultObjectACL().List(ctx)
	if err != nil {
		t.Errorf("DefaultObjectACL.List for bucket %q: %v", bucket, err)
	} else if !hasRule(acl, rule) {
		t.Errorf("default ACL missing %#v", rule)
	}
	aclObjects := []string{"acl1" + suffix, "acl2" + suffix}
	for _, obj := range aclObjects {
		t.Logf("Writing %v", obj)
		wc := bkt.Object(obj).NewWriter(ctx)
		c := randomContents()
		if _, err := wc.Write(c); err != nil {
			t.Errorf("Write for %v failed with %v", obj, err)
		}
		if err := wc.Close(); err != nil {
			t.Errorf("Close for %v failed with %v", obj, err)
		}
	}
	name := aclObjects[0]
	o := bkt.Object(name)
	acl, err = o.ACL().List(ctx)
	if err != nil {
		t.Errorf("Can't retrieve ACL of %v", name)
	} else if !hasRule(acl, rule) {
		t.Errorf("object ACL missing %+v", rule)
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
	rule2 := ACLRule{Entity: entity2, Role: RoleReader}
	if err := bkt.ACL().Set(ctx, entity2, RoleReader); err != nil {
		t.Errorf("Error while putting bucket ACL rule: %v", err)
	}
	bACL, err := bkt.ACL().List(ctx)
	if err != nil {
		t.Errorf("Error while getting the ACL of the bucket: %v", err)
	} else if !hasRule(bACL, rule2) {
		t.Errorf("bucket ACL missing %+v", rule2)
	}
	if err := bkt.ACL().Delete(ctx, entity2); err != nil {
		t.Errorf("Error while deleting bucket ACL rule: %v", err)
	}

}

func hasRule(acl []ACLRule, rule ACLRule) bool {
	for _, r := range acl {
		if r == rule {
			return true
		}
	}
	return false
}

func TestValidObjectNames(t *testing.T) {
	ctx := context.Background()
	client, bucket := testConfig(ctx, t)
	defer client.Close()

	bkt := client.Bucket(bucket)

	// NOTE(djd): This test can't append suffix to each name, since we're checking the validity
	// of these exact names. This test will still pass if the objects are not deleted between
	// test runs, but we attempt deletion to keep the bucket clean.
	validNames := []string{
		"gopher",
		"Гоферови",
		"a",
		strings.Repeat("a", 1024),
	}
	for _, name := range validNames {
		w := bkt.Object(name).NewWriter(ctx)
		if _, err := w.Write([]byte("data")); err != nil {
			t.Errorf("Object %q write failed: %v. Want success", name, err)
			continue
		}
		if err := w.Close(); err != nil {
			t.Errorf("Object %q close failed: %v. Want success", name, err)
			continue
		}
		defer bkt.Object(name).Delete(ctx)
	}

	invalidNames := []string{
		"", // Too short.
		strings.Repeat("a", 1025), // Too long.
		"new\nlines",
		"bad\xffunicode",
	}
	for _, name := range invalidNames {
		w := bkt.Object(name).NewWriter(ctx)
		// Invalid object names will either cause failure during Write or Close.
		if _, err := w.Write([]byte("data")); err != nil {
			continue
		}
		if err := w.Close(); err != nil {
			continue
		}
		defer bkt.Object(name).Delete(ctx)
		t.Errorf("%q should have failed. Didn't", name)
	}
}

func TestWriterContentType(t *testing.T) {
	ctx := context.Background()
	client, bucket := testConfig(ctx, t)
	defer client.Close()

	obj := client.Bucket(bucket).Object("content" + suffix)
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
	for _, tt := range testCases {
		w := obj.NewWriter(ctx)
		w.ContentType = tt.setType
		if _, err := w.Write([]byte(tt.content)); err != nil {
			t.Errorf("w.Write: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Errorf("w.Close: %v", err)
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

func TestZeroSizedObject(t *testing.T) {
	ctx := context.Background()
	client, bucket := testConfig(ctx, t)
	defer client.Close()

	obj := client.Bucket(bucket).Object("zero" + suffix)

	// Check writing it works as expected.
	w := obj.NewWriter(ctx)
	if err := w.Close(); err != nil {
		t.Fatalf("Writer.Close: %v", err)
	}
	defer obj.Delete(ctx)

	// Check we can read it too.
	r, err := obj.NewReader(ctx)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	defer r.Close()
	body, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatalf("ioutil.ReadAll: %v", err)
	}
	if len(body) != 0 {
		t.Errorf("Body is %v, want empty []byte{}", body)
	}
}

// cleanup deletes any objects in the default bucket which were created
// during this test run (those with the designated suffix), and any
// objects whose suffix indicates they were created over an hour ago.
func cleanup() error {
	if testing.Short() {
		return nil // Don't clean up in short mode.
	}
	ctx := context.Background()
	client, bucket := config(ctx)
	if client == nil {
		return nil // Don't cleanup if we're not configured correctly.
	}
	defer client.Close()

	suffixRE := regexp.MustCompile(`-t(\d+)$`)
	deadline := time.Now().Add(-1 * time.Hour)

	var q *Query
	for {
		o, err := client.Bucket(bucket).List(ctx, q)
		if err != nil {
			return fmt.Errorf("cleanup list failed: %v", err)
		}

		for _, obj := range o.Results {
			// Delete the object if it matches the suffix exactly,
			// or has a suffix marked before the deadline.
			del := strings.HasSuffix(obj.Name, suffix)
			if m := suffixRE.FindStringSubmatch(obj.Name); m != nil {
				if ns, err := strconv.ParseInt(m[1], 10, 64); err == nil && time.Unix(0, ns).Before(deadline) {
					del = true
				}
			}
			if !del {
				continue
			}
			log.Printf("Cleanup deletion of %q", obj.Name)
			if err := client.Bucket(bucket).Object(obj.Name).Delete(ctx); err != nil {
				// Print the error out, but keep going.
				log.Printf("Cleanup deletion of %q failed: %v", obj.Name, err)
			}
		}
		if o.Next == nil {
			break
		}
		q = o.Next
	}

	// TODO(djd): Similarly list and clean up buckets.
	return nil
}

func randomContents() []byte {
	h := md5.New()
	io.WriteString(h, fmt.Sprintf("hello world%d", rand.Intn(100000)))
	return h.Sum(nil)
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) { return len(p), nil }
