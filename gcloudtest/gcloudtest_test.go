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
	"testing"
)

func TestConcurrentAccess(t *testing.T) {
	bodyBase := "BODY"
	method := "POST"
	URL := "http://example.com/whatever"
	num := 100

	rec := &recorder{}
	done := make(chan struct{}, num)
	defer close(done)

	recorded := map[string]bool{}
	// testing 100 requests with concurrent access
	for i := 0; i < num; i++ {
		go func(n int) {
			bodyString := fmt.Sprintf("%s-%d", bodyBase, n)
			recorded[bodyString] = false
			body := bytes.NewBufferString(bodyString)
			req, err := http.NewRequest(method, URL, body)
			if err != nil {
				t.Errorf("NewRequest failed, %v", err)
			}
			_, err = rec.RoundTrip(req)
			if err != nil {
				t.Errorf("RoundTrip failed, %v", err)
			}
			done <- struct{}{}
		}(i)
	}
	// wait for all done
	for i := 0; i < num; i++ {
		<-done
	}
	// number of records should be precise
	reqs := rec.getRequests()
	if len(reqs) != num {
		t.Errorf("m.Len should be %d, but %d.", num, len(reqs))
	}
	for _, req := range reqs {
		b, err := ioutil.ReadAll(req.Body)
		if err != nil {
			t.Errorf("ReadAll failed, %v", err)
		}
		recorded[string(b)] = true
	}
	for k, v := range recorded {
		if !v {
			t.Errorf("A request with a body '%s' should be recorded", k)
		}
	}
}
