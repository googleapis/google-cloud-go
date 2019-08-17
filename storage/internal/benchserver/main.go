// Copyright 2019 Google LLC
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

// Package main pretends to be the storage backend for the sake of benchmarking.
package main

import (
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"google.golang.org/api/googleapi"
	raw "google.golang.org/api/storage/v1"
)

var (
	port  = flag.String("port", "", "specify a port to run on")
	files = map[string]int64{
		"oneMB":     10000000,
		"tenMB":     100000000,
		"hundredMB": 1000000000,
		"oneGB":     10000000000,
	}
)

func main() {
	// Enable line numbers for easier debugging.
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	flag.Parse()
	if *port == "" {
		log.Fatalf("usage: %s --port=8080", os.Args[0])
	}

	// Read.
	for objectName, size := range files {
		objectName := objectName
		size := size

		// Go: Download object.
		http.HandleFunc("/some-bucket-name/"+objectName, func(resp http.ResponseWriter, req *http.Request) {
			contents := io.LimitReader(rand.Reader, size)
			if _, err := io.Copy(resp, contents); err != nil {
				log.Println(err)
				return
			}
		})

		// Node: Download object.
		http.HandleFunc("/b/some-bucket-name/o/"+objectName, func(resp http.ResponseWriter, req *http.Request) {
			fmt.Println("emulator read <<NODE>> called")
			contents := io.LimitReader(rand.Reader, size)
			if _, err := io.Copy(resp, contents); err != nil {
				log.Println(err)
				return
			}
		})

		// Java: Download metadata (pointing to download link for object).
		http.HandleFunc("/storage/v1/b/some-bucket-name/o/"+objectName, func(resp http.ResponseWriter, req *http.Request) {
			fmt.Println("emulator metadata read <<JAVA>> called")

			resp.Header().Set("Content-Type", "application/json")
			if _, err := fmt.Fprintf(resp, `{
	"name": "%s-download",
	"bucket": "some-bucket-name-download",
	"generation": "1"
}`, objectName); err != nil {
				log.Println(err)
				return
			}
		})

		// Java: Download object.
		http.HandleFunc(fmt.Sprintf("/download/storage/v1/b/some-bucket-name-download/o/%s-download", objectName), func(resp http.ResponseWriter, req *http.Request) {
			fmt.Println("emulator read <<JAVA>> called")
			contents := io.LimitReader(rand.Reader, size)
			if _, err := io.Copy(resp, contents); err != nil {
				log.Println(err)
				return
			}
		})
	}

	// Write.
	http.HandleFunc("/b/some-bucket-name/o", func(resp http.ResponseWriter, req *http.Request) {
		fmt.Println("emulator write called")
		resp.Header().Set("Content-Type", "application/json")
		ret := &raw.Object{
			ServerResponse: googleapi.ServerResponse{
				Header:         resp.Header(),
				HTTPStatusCode: http.StatusCreated,
			},
		}
		if err := json.NewEncoder(resp).Encode(ret); err != nil {
			log.Fatal(err)
		}
	})
	log.Printf("listening on localhost:%s\n", *port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", *port), nil))
}
