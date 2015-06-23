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

package storage_test

import (
	"io/ioutil"
	"log"
	"testing"

	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/cloud"
	"google.golang.org/cloud/storage"
)

// TODO(jbd): Remove after Go 1.4.
// Related to https://codereview.appspot.com/107320046
func TestA(t *testing.T) {}

func Example_auth() {
	// Initialize an authorized context with Google Developers Console
	// JSON key. Read the google package examples to learn more about
	// different authorization flows you can use.
	// http://godoc.org/golang.org/x/oauth2/google
	jsonKey, err := ioutil.ReadFile("/path/to/json/keyfile.json")
	if err != nil {
		log.Fatal(err)
	}
	conf, err := google.JWTConfigFromJSON(
		jsonKey,
		storage.ScopeFullControl,
	)
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.TODO()
	client, err := storage.NewClient(ctx, "project-id", cloud.WithTokenSource(conf.TokenSource(ctx)))
	if err != nil {
		log.Fatal(err)
	}

	// Use the client (see other examples)
	_ = client
}

func ExampleListObjects() {
	ctx := context.TODO()
	var client *storage.Client // See Example (Auth)

	var query *storage.Query
	for {
		// If you are using this package on App Engine Managed VMs runtime,
		// you can init a bucket client with your app's default bucket name.
		// See http://godoc.org/google.golang.org/appengine/file#DefaultBucketName.
		objects, err := client.ListObjects(ctx, "bucketname", query)
		if err != nil {
			log.Fatal(err)
		}
		for _, obj := range objects.Results {
			log.Printf("object name: %s, size: %v", obj.Name, obj.Size)
		}
		// if there are more results, objects.Next
		// will be non-nil.
		query = objects.Next
		if query == nil {
			break
		}
	}

	log.Println("paginated through all object items in the bucket you specified.")
}

func ExampleNewReader() {
	ctx := context.TODO()
	var client *storage.Client // See Example (Auth)

	rc, err := client.NewReader(ctx, "bucketname", "filename1")
	if err != nil {
		log.Fatal(err)
	}
	slurp, err := ioutil.ReadAll(rc)
	rc.Close()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("file contents:", slurp)
}

func ExampleNewWriter() {
	ctx := context.TODO()
	var client *storage.Client // See Example (Auth)

	wc := client.NewWriter(ctx, "bucketname", "filename1")
	wc.ContentType = "text/plain"
	wc.ACL = []storage.ACLRule{{storage.AllUsers, storage.RoleReader}}
	if _, err := wc.Write([]byte("hello world")); err != nil {
		log.Fatal(err)
	}
	if err := wc.Close(); err != nil {
		log.Fatal(err)
	}
	log.Println("updated object:", wc.Object())
}

func ExampleCopyObject() {
	ctx := context.TODO()
	var client *storage.Client // See Example (Auth)

	o, err := client.CopyObject(ctx, "bucketname", "file1", "another-bucketname", "file2", nil)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("copied file:", o)
}

func ExampleDeleteObject() {
	ctx := context.TODO()
	var client *storage.Client // See Example (Auth)

	// To delete multiple objects in a bucket, first ListObjects then delete them.

	// If you are using this package on App Engine Managed VMs runtime,
	// you can init a bucket client with your app's default bucket name.
	// See http://godoc.org/google.golang.org/appengine/file#DefaultBucketName.
	const bucket = "bucketname"

	var query *storage.Query // Set up query as desired.
	for {
		objects, err := client.ListObjects(ctx, bucket, query)
		if err != nil {
			log.Fatal(err)
		}
		for _, obj := range objects.Results {
			log.Printf("deleting object name: %q, size: %v", obj.Name, obj.Size)
			if err := client.DeleteObject(ctx, bucket, obj.Name); err != nil {
				log.Fatalf("unable to delete %q: %v", obj.Name, err)
			}
		}
		// if there are more results, objects.Next will be non-nil.
		query = objects.Next
		if query == nil {
			break
		}
	}

	log.Println("deleted all object items in the bucket you specified.")
}
