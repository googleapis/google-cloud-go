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
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"testing"

	"github.com/golang/oauth2/google"
	"google.golang.org/cloud"

	"crypto/md5"

	"code.google.com/p/go.net/context"
)

var bucket string

var objects = []string{"obj1", "obj2", "obj3", "obj4", "obj5", "obj6"}

const (
	envProjID     = "GCLOUD_TESTS_GOLANG_PROJECT_ID"
	envPrivateKey = "GCLOUD_TESTS_GOLANG_KEY"
	envBucket     = "GCLOUD_TESTS_GOLANG_BUCKET_NAME"
)

// TODO(jbd): Create a new bucket for each test run and delete the bucket.

func TestObjectReadWrite(t *testing.T) {
	ctx := testContext(t)
	contents := randomContents()
	object := objects[0]
	wc := NewWriter(ctx, bucket, object, nil)
	_, err := wc.Write(contents)
	if err != nil {
		t.Error(err)
	}
	err = wc.Close()
	if err != nil {
		t.Error(err)
	}
	_, err = wc.Object()
	rc, err := NewReader(ctx, bucket, object)
	if err != nil {
		t.Error(err)
	}
	slurp, err := ioutil.ReadAll(rc)
	if err != nil {
		t.Error(err)
	}
	if string(slurp) != string(contents) {
		t.Errorf("read file's content is found to be '%v' unexpectedly", slurp)
	}
}

func TestObjectReadNotFound(t *testing.T) {
	ctx := testContext(t)
	_, err := NewReader(ctx, bucket, "obj-not-exists")
	if err != ErrObjectNotExists {
		t.Errorf("expected object not to exist, err found to be %v", err)
	}
}

func TestObjectStat(t *testing.T) {
	ctx := testContext(t)
	o, err := Stat(ctx, bucket, objects[0])
	if err != nil {
		t.Error(err)
	}
	if o.Name != objects[0] {
		t.Errorf("stat returned object info for %v unexpectedly", o.Name)
	}
}

func TestObjectCopy(t *testing.T) {
	ctx := testContext(t)
	o, err := Copy(ctx, bucket, objects[0], &Object{
		Name:        "copy-object",
		ContentType: "text/html",
	})
	if err != nil {
		t.Error(err)
	}
	if o.Name != "copy-object" {
		t.Errorf("copy object's name is %v unexpectedly", o.Name)
	}
}

func TestObjectPublicACL(t *testing.T) {
	ctx := testContext(t)

	contents := randomContents()
	name := objects[1]
	wc := NewWriter(ctx, bucket, name, &Object{
		ACL: []*ACLRule{&ACLRule{"allUsers", RoleReader}},
	})
	_, err := wc.Write(contents)
	if err != nil {
		t.Error(err)
	}
	err = wc.Close()
	if err != nil {
		t.Error(err)
	}
	_, err = wc.Object()
	if err != nil {
		t.Error(err)
	}

	ctx = cloud.NewContext(
		os.Getenv(envProjID), &http.Client{Transport: http.DefaultTransport})
	r, err := NewReader(ctx, bucket, name)
	if err != nil {
		t.Error(err)
	}
	slurp, err := ioutil.ReadAll(r)
	if err != nil {
		t.Error(err)
	}
	if string(slurp) != string(contents) {
		t.Errorf("public file's content is expected to be %s, found %s", contents, slurp)
	}
}

func TestObjectDelete(t *testing.T) {
	ctx := testContext(t)
	err := Delete(ctx, bucket, "copy-object")
	if err != nil {
		t.Error(err)
	}
	_, err = Stat(ctx, bucket, "copy-object")
	if err != ErrObjectNotExists {
		t.Errorf("copy-object is expected to be deleted, stat errored with %v", err)
	}
}

func randomContents() []byte {
	h := md5.New()
	io.WriteString(h, fmt.Sprintf("hello world%d", rand.Intn(100000)))
	return h.Sum(nil)
}

func testContext(t *testing.T) context.Context {
	bucket = os.Getenv(envBucket)
	conf, err := google.NewServiceAccountJSONConfig(
		os.Getenv(envPrivateKey),
		ScopeFullControl)
	if err != nil {
		t.Fatal(err)
	}
	return cloud.NewContext(
		os.Getenv(envProjID), &http.Client{Transport: conf.NewTransport()})
}
