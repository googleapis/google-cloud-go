// Copyright 2017 Google Inc. All Rights Reserved.
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
	"net/http"
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/internal/pretty"
	raw "google.golang.org/api/storage/v1"
)

func TestToRawBucket(t *testing.T) {
	t.Parallel()
	attrs := &BucketAttrs{
		Name:              "name",
		ACL:               []ACLRule{{Entity: "bob@example.com", Role: RoleOwner}},
		DefaultObjectACL:  []ACLRule{{Entity: AllUsers, Role: RoleReader}},
		Location:          "loc",
		StorageClass:      "class",
		VersioningEnabled: false,
		// should be ignored:
		MetaGeneration: 39,
		Created:        time.Now(),
	}
	got := attrs.toRawBucket()
	want := &raw.Bucket{
		Name: "name",
		Acl: []*raw.BucketAccessControl{
			{Entity: "bob@example.com", Role: "OWNER"},
		},
		DefaultObjectAcl: []*raw.ObjectAccessControl{
			{Entity: "allUsers", Role: "READER"},
		},
		Location:     "loc",
		StorageClass: "class",
		Versioning:   nil, // ignore VersioningEnabled if flase
	}
	msg, ok, err := pretty.Diff(want, got)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error(msg)
	}

	attrs.VersioningEnabled = true
	got = attrs.toRawBucket()
	want.Versioning = &raw.BucketVersioning{Enabled: true}
	msg, ok, err = pretty.Diff(want, got)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error(msg)
	}
}

func TestCallBuilders(t *testing.T) {
	rc, err := raw.New(&http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{raw: rc}

	b := c.Bucket("name")
	bm := b.If(BucketConditions{MetagenerationMatch: metagen})

	got, err := b.newGetCall()
	if err != nil {
		t.Fatal(err)
	}
	want := rc.Buckets.Get("name").Projection("full")
	setClientHeader(want.Header())
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
	got, err = bm.newGetCall()
	if err != nil {
		t.Fatal(err)
	}
	want.IfMetagenerationMatch(17)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}

	gotd, err := b.newDeleteCall()
	if err != nil {
		t.Fatal(err)
	}
	wantd := rc.Buckets.Delete("name")
	setClientHeader(wantd.Header())
	if !reflect.DeepEqual(gotd, wantd) {
		t.Errorf("got %#v, want %#v", gotd, wantd)
	}
	gotd, err = bm.newDeleteCall()
	if err != nil {
		t.Fatal(err)
	}
	wantd.IfMetagenerationMatch(17)
	if !reflect.DeepEqual(gotd, wantd) {
		t.Errorf("got %#v, want %#v", gotd, wantd)
	}

	// Error.
	bm = b.If(BucketConditions{MetagenerationMatch: 1, MetagenerationNotMatch: 2})
	if _, err := bm.newGetCall(); err == nil {
		t.Errorf("got nil, want error")
	}
	if _, err := bm.newDeleteCall(); err == nil {
		t.Errorf("got nil, want error")
	}
	if _, err := bm.newPatchCall(&BucketAttrsToUpdate{}); err == nil {
		t.Errorf("got nil, want error")
	}
}
