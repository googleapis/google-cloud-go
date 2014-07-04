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

package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
)

type client struct {
	transport http.RoundTripper
}

func (c *client) Do(method string, u *url.URL, body, response interface{}) (err error) {
	client := http.Client{Transport: c.transport}
	req, err := http.NewRequest(method, u.String(), nil)
	if err != nil {
		return err
	}
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		req.Body = ioutil.NopCloser(bytes.NewReader(data))
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// return as error
		return errors.New("storage: error during call")
	}
	if response == nil {
		return
	}
	var data []byte
	if data, err = ioutil.ReadAll(resp.Body); err != nil {
		return
	}
	if err = json.Unmarshal(data, response); err != nil {
		return
	}
	return
}
