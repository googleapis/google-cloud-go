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

// +build integration

package storage

import (
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"

	"google.golang.org/cloud/internal/testutil"
)

var (
	bucket     string
	contents   = make(map[string][]byte)
	objects    = []string{"obj1", "obj2"}
	aclObjects = []string{"acl1", "acl2"}
	copyObj    = "copy-object"
)

const envBucket = "GCLOUD_TESTS_GOLANG_PROJECT_ID"

func TestObjects(t *testing.T) {
	ctx := testutil.Context(ScopeFullControl)
	bucket = os.Getenv(envBucket)

	// Cleanup.
	cleanup(t, "obj")

	// Test Writer.
	for _, obj := range objects {
		t.Logf("Writing %v", obj)
		wc := NewWriter(ctx, bucket, obj, &Object{})
		c := randomContents()
		if _, err := wc.Write(c); err != nil {
			t.Errorf("Write for %v failed with %v", obj, err)
		}
		if err := wc.Close(); err != nil {
			t.Errorf("Close for %v failed with %v", obj, err)
		}
		contents[obj] = c
	}

	// Test Reader.
	for _, obj := range objects {
		t.Logf("Creating a reader to read %v", obj)
		r, err := NewReader(ctx, bucket, obj)
		if err != nil {
			t.Errorf("Can't create a reader for %v, errored with %v", obj, err)
		}
		slurp, err := ioutil.ReadAll(r)
		if err != nil {
			t.Errorf("Can't ReadAll object %v, errored with %v", obj, err)
		}
		actual := string(slurp)
		expected := string(contents[obj])
		if actual != expected {
			t.Errorf("Expected contents for %v is '%v', found '%v'", obj, expected, actual)
		}
	}

	// Test NotFound.
	_, err := NewReader(ctx, bucket, "obj-not-exists")
	if err != ErrObjectNotExists {
		t.Errorf("Object should not exist, err found to be %v", err)
	}

	// Test StatObject.
	o, err := StatObject(ctx, bucket, objects[0])
	if err != nil {
		t.Error(err)
	}
	if o.Name != objects[0] {
		t.Errorf("StatObject returned object info for %v unexpectedly", o.Name)
	}

	// Test object copy.
	copy, err := CopyObject(ctx, bucket, objects[0], &Object{
		Name:        copyObj,
		ContentType: "text/html",
	})
	if err != nil {
		t.Errorf("CopyObject failed with %v", err)
	}
	if copy.Name != copyObj {
		t.Errorf("Copy object's name is %v unexpectedly", copy.Name)
	}
	if copy.Bucket != bucket {
		t.Errorf("Copy object's bucket is %v unexpectedly", copy.Bucket)
	}

	// Test put.
	updated, err := PutObject(ctx, bucket, objects[0], &Object{
		Name:        objects[0],
		ContentType: "text/html",
		ACL:         []ACLRule{{Entity: "domain-google.com", Role: RoleReader}},
	})
	if err != nil {
		t.Errorf("PutObject failed with %v", err)
	}
	if want := "text/html"; updated.ContentType != want {
		t.Errorf("updated.ContentType == %q, want %q", updated.ContentType, want)
	}

	// Test public ACL.
	publicObj := objects[0]
	if err = PutACLRule(ctx, bucket, publicObj, "allUsers", RoleReader); err != nil {
		t.Errorf("PutACLRule failed with %v", err)
	}
	publicCtx := testutil.NoAuthContext()
	r, err := NewReader(publicCtx, bucket, publicObj)
	if err != nil {
		t.Error(err)
	}
	slurp, err := ioutil.ReadAll(r)
	if err != nil {
		t.Errorf("ReadAll failed with %v", err)
	}
	if string(slurp) != string(contents[publicObj]) {
		t.Errorf("Public object's content is expected to be %s, found %s", contents[publicObj], slurp)
	}

	// Test writer error handling.
	wc := NewWriter(publicCtx, bucket, publicObj, nil)
	if _, err := wc.Write([]byte("hello")); err != nil {
		t.Errorf("Write unexpectedly failed with %v", err)
	}
	if err = wc.Close(); err == nil {
		t.Error("Close expected an error, found none")
	}

	// DeleteObject object.
	// The rest of the other object will be deleted during
	// the initial cleanup. This tests exists, so we still can cover
	// deletion if there are no objects on the bucket to clean.
	if err := DeleteObject(ctx, bucket, copyObj); err != nil {
		t.Errorf("Deletion of %v failed with %v", copyObj, err)
	}
	_, err = StatObject(ctx, bucket, copyObj)
	if err != ErrObjectNotExists {
		t.Errorf("Copy is expected to be deleted, stat errored with %v", err)
	}
}

func TestACL(t *testing.T) {
	ctx := testutil.Context(ScopeFullControl)
	cleanup(t, "acl")
	entity := "domain-google.com"
	if err := PutDefaultACLRule(ctx, bucket, entity, RoleReader); err != nil {
		t.Errorf("Can't put default ACL rule for the bucket, errored with %v", err)
	}
	for _, obj := range aclObjects {
		t.Logf("Writing %v", obj)
		wc := NewWriter(ctx, bucket, obj, &Object{})
		c := randomContents()
		if _, err := wc.Write(c); err != nil {
			t.Errorf("Write for %v failed with %v", obj, err)
		}
		if err := wc.Close(); err != nil {
			t.Errorf("Close for %v failed with %v", obj, err)
		}
	}
	name := aclObjects[0]
	acl, err := ACL(ctx, bucket, name)
	if err != nil {
		t.Errorf("Can't retrieve ACL of %v", name)
	}
	aclFound := false
	for _, rule := range acl {
		if rule.Entity == entity && rule.Role == RoleReader {
			aclFound = true
		}
	}
	if !aclFound {
		t.Error("Expected to find an ACL rule for google.com domain users, but not found")
	}
	if err := DeleteACLRule(ctx, bucket, name, entity); err != nil {
		t.Errorf("Can't delete the ACL rule for the entity: %v", entity)
	}

	if err := PutBucketACLRule(ctx, bucket, "user-jbd@google.com", RoleReader); err != nil {
		t.Errorf("Error while putting bucket ACL rule: %v", err)
	}
	bACL, err := BucketACL(ctx, bucket)
	if err != nil {
		t.Errorf("Error while getting the ACL of the bucket: %v", err)
	}
	bACLFound := false
	for _, rule := range bACL {
		if rule.Entity == "user-jbd@google.com" && rule.Role == RoleReader {
			bACLFound = true
		}
	}
	if !bACLFound {
		t.Error("Expected to find an ACL rule for jbd@google.com user, but not found")
	}
	if err := DeleteBucketACLRule(ctx, bucket, "user-jbd@google.com"); err != nil {
		t.Errorf("Error while deleting bucket ACL rule: %v", err)
	}
}

func cleanup(t *testing.T, prefix string) {
	ctx := testutil.Context(ScopeFullControl)
	var q *Query = &Query{
		Prefix: prefix,
	}
	for {
		o, err := ListObjects(ctx, bucket, q)
		if err != nil {
			t.Fatalf("Cleanup List for bucket %v failed with error: %v", bucket, err)
		}
		for _, obj := range o.Results {
			t.Logf("Cleanup deletion of %v", obj.Name)
			if err = DeleteObject(ctx, bucket, obj.Name); err != nil {
				t.Fatalf("Cleanup Delete for object %v failed with %v", obj.Name, err)
			}
		}
		if o.Next == nil {
			break
		}
		q = o.Next
	}
}

func randomContents() []byte {
	h := md5.New()
	io.WriteString(h, fmt.Sprintf("hello world%d", rand.Intn(100000)))
	return h.Sum(nil)
}
