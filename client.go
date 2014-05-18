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

package gcloud

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"

	"code.google.com/p/goprotobuf/proto"
)

type Client struct {
	// An authorized transport.
	Transport http.RoundTripper
}

func (c *Client) Call(url string, req proto.Message, resp proto.Message) (err error) {
	client := http.Client{Transport: c.Transport}
	payload, err := proto.Marshal(req)
	if err != nil {
		return
	}
	r, err := client.Post(url, "application/x-protobuf", bytes.NewBuffer(payload))
	if err != nil {
		return
	}
	if r.StatusCode != http.StatusOK {
		// TODO(jbd): Handle if there is no body
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return errors.New("gcloud: error during call")
		}
		return errors.New("gcloud: error during call: " + string(body))
	}
	body, _ := ioutil.ReadAll(r.Body)
	if err = proto.Unmarshal(body, resp); err != nil {
		return
	}
	return
}
