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
	"strings"
	"testing"
)

type MyMocker struct {
	Mocker
}

type EchoRoundTripper struct {
	statusCode int
	status     string
}

func (rt *EchoRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("ReadAll failed, %v", err)
	}
	return &http.Response{
		Status:     rt.status,
		StatusCode: rt.statusCode,
		Body:       ioutil.NopCloser(bytes.NewBuffer(body)),
	}, nil
}

func TestInterface(t *testing.T) {
	// It should fail if MockTransport doesn't implement Mocker
	mm := &MyMocker{NewMockTransport()}
	t.Logf("mm: %v", mm)
}

func TestRegister(t *testing.T) {
	m := NewMockTransport()
	rt := http.DefaultTransport
	m.Register(rt)
	if m.handler != rt {
		t.Errorf("m.handler should be %v, but actually %v", rt, m.handler)
	}
}

func TestConcurrentAccess(t *testing.T) {
	num := 100
	bodyBase := "BODY"
	method := "GET"
	URL := "https://www.google.com/"

	m := NewMockTransport()
	m.Register(&EchoRoundTripper{status: "200 OK", statusCode: 200})
	done := make(chan struct{}, num)
	defer close(done)
	// testing 100 requests with concurrent access
	for i := 0; i < num; i++ {
		go func(n int) {
			bodyString := fmt.Sprintf("%s-%d", bodyBase, n)
			body := bytes.NewBufferString(bodyString)
			req, err := http.NewRequest(method, URL, body)
			if err != nil {
				t.Errorf("NewRequest failed, %v", err)
			}
			res, err := m.RoundTrip(req)
			if err != nil {
				t.Errorf("RoundTrip failed, %v", err)
			}
			b, err := ioutil.ReadAll(res.Body)
			if err != nil {
				t.Errorf("Read failed, %v", err)
			}
			if string(b) != bodyString {
				t.Errorf("Response body should be %s, but %s", bodyString, string(b))
			}
			done <- struct{}{}
		}(i)
	}
	// wait for all done
	for i := 0; i < num; i++ {
		<-done
	}
	// Len should be precise
	if m.Len() != num {
		t.Errorf("m.Len should be %d, but %d.", num, m.Len())
	}
	// Test concurrent GetRequest
	for i := 0; i < num; i++ {
		go func() {
			req, err := m.GetRequest(-1)
			if err != nil {
				t.Error("GetRequest failed, but it shouldn't")
			}
			b, err := ioutil.ReadAll(req.Body)
			if err != nil {
				t.Errorf("ReadAll failed, %v", err)
			}
			if req.Method != method {
				t.Errorf("Method should be %s, but %s", method, req.Method)
			}
			if req.URL.String() != URL {
				t.Errorf("URL should be %s, but %s", URL, req.URL.String())
			}
			if !strings.HasPrefix(string(b), bodyBase) {
				t.Errorf("body should have %s as a prefix, but %s", bodyBase, string(b))
			}
			done <- struct{}{}
		}()
	}
	// wait for all done
	for i := 0; i < num; i++ {
		<-done
	}
	if m.Len() != 0 {
		t.Errorf("m.Len should be 0, but %d.", m.Len())
	}
}
