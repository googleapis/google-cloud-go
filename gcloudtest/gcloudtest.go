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
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
)

// Mocker interface is used by higher level testing libraries for
// recording requests and registering another http.RoundTripper for
// providing mocked responses.
type Mocker interface {
	Register(http.RoundTripper) error
	Record(*http.Request) error
	Len() int
	GetRequest(int) (*http.Request, error)
	http.RoundTripper
}

// MockTransport can be used in developers' unit tests as well as in
// higher level testing libraries for specific services. This object
// is thread-safe (or goroutine-safe).
type MockTransport struct {
	handler  http.RoundTripper
	requests []*http.Request
	mutex    sync.RWMutex
}

// creates new MockTransport.
func NewMockTransport() *MockTransport {
	return &MockTransport{}
}

// Register registers another RoundTripper for actually creating
// mocked responses.
func (m *MockTransport) Register(rt http.RoundTripper) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.handler = rt
	return nil
}

// Len just returns the length of the recorded requests. You can use
// the following code for looping through the recorded requests.
//
// mock := gcloudtest.NewMockTransport()
// // your test code here
// for mock.Len() > 0 {
//   req := mock.GetRequest(0) // or use -1 for reverse order
// }
func (m *MockTransport) Len() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.requests)
}

// Record append a request to internal slice.
func (m *MockTransport) Record(r *http.Request) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.requests = append(m.requests, r)
	return nil
}

// GetRequest pulls out the specified request and returns it.
func (m *MockTransport) GetRequest(n int) (*http.Request, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if n >= len(m.requests) {
		return nil, fmt.Errorf("Index out of bounds with n: %d, actual length: %d", n, len(m.requests))
	}
	if n == -1 {
		// treats -1 as the last element
		n = len(m.requests) - 1
	}
	ret := m.requests[n]
	m.requests = append(m.requests[:n], m.requests[n+1:]...)
	return ret, nil
}

// RoundTrip records a copy of the request, and deferes actual work to
// the registered RoundTripper.
func (m *MockTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("ReadAll failed, %v", err)
	}
	defer r.Body.Close()
	record, _ := http.NewRequest(r.Method, r.URL.String(), bytes.NewReader(body))
	req, _ := http.NewRequest(r.Method, r.URL.String(), bytes.NewReader(body))
	m.Record(record)
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	if m.handler == nil {
		return nil, errors.New("No handler registered.")
	}
	return m.handler.RoundTrip(req)
}
