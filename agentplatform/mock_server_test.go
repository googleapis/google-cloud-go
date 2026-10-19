// Copyright 2026 Google LLC
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

package agentplatform

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type MockResponse struct {
	StatusCode int
	Headers    map[string]string
	// Body is the response body to be served and can be anything,
	// the mock server will marshal it to JSON when it serves the response.
	Body any
}

type MockSpy struct {
	Requests []*http.Request
}

type MockServer struct {
	responses []*MockResponse
	Server    *httptest.Server
	Spy       *MockSpy
	t         *testing.T
	mu        sync.Mutex

	// Counter is used to determine which response to serve.
	counter int
}

// ServeHTTP serves the mock responses to the requests and records the requests in the spy.
func (m *MockServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	m.t.Helper()
	m.mu.Lock()
	c := m.counter
	m.counter++

	// Record the request in the spy.
	m.Spy.Requests = append(m.Spy.Requests, req)

	// Record the request in the spy and return if we've run out of responses.
	if c >= len(m.responses) {
		counter := m.counter
		m.mu.Unlock()
		m.t.Fatalf("expected maximum of %d requests, got %d", len(m.responses), counter)
		return
	}
	resp := m.responses[c]
	m.mu.Unlock()

	// Set Content-Type header
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}

	// Set status code
	w.WriteHeader(int(resp.StatusCode))

	// Write response body
	if responseBody, err := json.Marshal(resp.Body); err != nil {
		m.t.Fatalf("error marshalling response body, err: %v", err)
	} else if _, err := w.Write(responseBody); err != nil {
		m.t.Fatalf("error writing response, err: %v", err)
	}
}

// NewMockServer creates a mock server for the given test.
func NewMockServer(t *testing.T) *MockServer {
	m := &MockServer{
		t:         t,
		responses: []*MockResponse{},
		Spy: &MockSpy{
			Requests: []*http.Request{},
		},
	}
	m.Server = httptest.NewServer(m)
	m.t.Cleanup(func() {
		m.Server.Close()
	})
	return m
}

// AddResponse adds a response to the mock server. Responses are served in the order they are added
// and are consumed on a first-come, first-served basis.
func (m *MockServer) AddResponses(responses ...*MockResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, responses...)
}
