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
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"testing"

	"github.com/golang/oauth2/google"
	"google.golang.org/cloud/storage"
)

// TODO(jbd): Remove after Go 1.4.
// Related to https://codereview.appspot.com/107320046
func TestA(t *testing.T) {}

func Example_auth() {
	// Initialize an authorized transport with Google Developers Console
	// JSON key. Read the google package examples to learn more about
	// different authorization flows you can use.
	// https://github.com/golang/oauth2/blob/master/google/example_test.go
	conf, err := google.NewServiceAccountJSONConfig(
		"/path/to/json/keyfile.json", storage.ScopeFullControl)
	if err != nil {
		log.Fatal(err)
	}

	b := storage.New(conf.NewTransport()).BucketClient("your-bucket-name")
	b.List(nil) // ...
}

func Example_listObjects() {
	tr := (http.RoundTripper)(nil) // replace it with an actual authorized transport
	b := storage.New(tr).BucketClient("your-bucket-name")

	for {
		var query *storage.Query
		objects, err := b.List(query)
		if err != nil {
			log.Fatalln(err)
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

func Example_readObjects() {
	tr := (http.RoundTripper)(nil) // replace it with an actual authorized transport
	b := storage.New(tr).BucketClient("your-bucket-name")

	rc, err := b.NewReader("filename1")
	if err != nil {
		log.Fatalln(err)
	}
	defer rc.Close()
	slurp, err := ioutil.ReadAll(rc)
	if err != nil {
		log.Println(err)
	}

	log.Println("file contents:", slurp)
}

func Example_writeObjects() {
	tr := (http.RoundTripper)(nil) // replace it with an actual authorized transport
	b := storage.New(tr).BucketClient("your-bucket-name")

	wc := b.NewWriter("filename1", &storage.Object{
		ContentType: "text/plain",
	})
	defer wc.Close()
	if _, err := wc.Write([]byte("hello world")); err != nil {
		log.Fatalln(err)
	}
	if err := wc.Close(); err != nil {
		log.Fatalln(err)
	}

	o, err := wc.Object()
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("updated object:", o)

	// or copy data from a file
	wc = b.NewWriter("filename1", &storage.Object{
		ContentType: "text/plain",
	})
	defer wc.Close()

	file, err := os.Open("/path/to/some/file")
	if err != nil {
		log.Fatalln(err)
	}
	defer file.Close()

	_, err = io.Copy(wc, file)
	if err != nil {
		log.Fatalln(err)
	}

	o, err = wc.Object()
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("updated object:", o)
}

func Example_touchObjects() {
	tr := (http.RoundTripper)(nil) // replace it with an actual authorized transport
	b := storage.New(tr).BucketClient("your-bucket-name")

	o, err := b.Put("filename", &storage.Object{
		ContentType: "text/plain",
	})
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("touched new file:", o)
}

func Example_copyObjects() {
	tr := (http.RoundTripper)(nil) // replace it with an actual authorized transport
	b := storage.New(tr).BucketClient("your-bucket-name")

	o, err := b.Copy("file1", &storage.Object{
		Name:   "file2",
		Bucket: "yet-another-bucket",
	})
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("copied file:", o)
}
