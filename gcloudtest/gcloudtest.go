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

// package gcloudtest is a core part of the gcloud-golang testing tool.
package gcloudtest

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
)

// Recorder interface adds GetRequests to http.RoundTripper
// interface. It should intercepts the requests and record
// them. GetRequests should return the recorded requests.
type Recorder interface {
	http.RoundTripper
	GetRequests() []*http.Request
}

// SimpleRecorder is a Recorder that always return a simple HTTP 200
// response.
type SimpleRecorder struct {
	requests []*http.Request
	mutex    sync.RWMutex
}

// Record append a request to internal slice.
func (rec *SimpleRecorder) record(r *http.Request) error {
	rec.mutex.Lock()
	defer rec.mutex.Unlock()
	rec.requests = append(rec.requests, r)
	return nil
}

// Thread-safe getter.
func (rec *SimpleRecorder) GetRequests() []*http.Request {
	rec.mutex.RLock()
	defer rec.mutex.RUnlock()
	return rec.requests
}

// RoundTrip records a copy of the request and returns a response with
// status code 200.
func (rec *SimpleRecorder) RoundTrip(r *http.Request) (*http.Response, error) {
	var body []byte
	var err error
	if r.Body != nil {
		body, err = ioutil.ReadAll(r.Body)
		if err != nil {
			return nil, fmt.Errorf("ReadAll failed, %v", err)
		}
	}
	r.Body.Close()
	req, _ := http.NewRequest(r.Method, r.URL.String(), bytes.NewReader(body))
	rec.record(req)
	rec.mutex.RLock()
	defer rec.mutex.RUnlock()
	return &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Body:       ioutil.NopCloser(bytes.NewBufferString("")),
	}, nil
}
