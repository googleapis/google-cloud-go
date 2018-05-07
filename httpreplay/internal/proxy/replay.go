// Copyright 2018 Google Inc. All Rights Reserved.
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

package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/google/martian/har"
	"github.com/google/martian/martianlog"
)

// ForReplaying returns a Proxy configured to replay.
func ForReplaying(filename string, port int) (*Proxy, error) {
	p, err := newProxy(filename)
	if err != nil {
		return nil, err
	}
	calls, err := readLog(filename)
	if err != nil {
		return nil, err
	}
	p.mproxy.SetRoundTripper(replayRoundTripper{calls: calls})

	// Debug logging.
	// TODO(jba): factor out from here and ForRecording.
	logger := martianlog.NewLogger()
	logger.SetDecode(true)
	p.mproxy.SetRequestModifier(logger)
	p.mproxy.SetResponseModifier(logger)

	if err := p.start(port); err != nil {
		return nil, err
	}
	return p, nil
}

// A call is an HTTP request and its matching response.
type call struct {
	req     *har.Request
	reqBody *requestBody // parsed request body
	res     *har.Response
}

func readLog(filename string) ([]*call, error) {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var h har.HAR
	if err := json.Unmarshal(bytes, &h); err != nil {
		return nil, err
	}
	ignoreIDs := map[string]bool{} // IDs of requests to ignore
	callsByID := map[string]*call{}
	var calls []*call
	for _, e := range h.Log.Entries {
		if ignoreIDs[e.ID] {
			continue
		}
		c, ok := callsByID[e.ID]
		switch {
		case !ok:
			if e.Request == nil {
				return nil, fmt.Errorf("first entry for ID %s does not have a request", e.ID)
			}
			if e.Request.Method == "CONNECT" {
				// Ignore CONNECT methods.
				ignoreIDs[e.ID] = true
			} else {
				reqBody, err := newRequestBodyFromHAR(e.Request)
				if err != nil {
					return nil, err
				}
				c := &call{e.Request, reqBody, e.Response}
				calls = append(calls, c)
				callsByID[e.ID] = c
			}
		case e.Request != nil:
			if e.Response != nil {
				return nil, errors.New("HAR entry has both request and response")
			}
			c.req = e.Request
		case e.Response != nil:
			c.res = e.Response
		default:
			return nil, errors.New("HAR entry has neither request nor response")
		}
	}
	for _, c := range calls {
		if c.req == nil || c.res == nil {
			return nil, fmt.Errorf("missing request or response: %+v", c)
		}
	}
	return calls, nil
}

type replayRoundTripper struct {
	calls []*call
}

func (r replayRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	reqBody, err := newRequestBodyFromHTTP(req)
	if err != nil {
		return nil, err
	}
	for i, call := range r.calls {
		if call == nil {
			continue
		}
		if requestsMatch(req, reqBody, call.req, call.reqBody) {
			r.calls[i] = nil // nil out this call so we don't reuse it
			res := harResponseToHTTPResponse(call.res)
			res.Request = req
			return res, nil
		}
	}
	return nil, fmt.Errorf("no matching request for %+v", req)
}

// Report whether the incoming request in matches the candidate request cand.
func requestsMatch(in *http.Request, inBody *requestBody, cand *har.Request, candBody *requestBody) bool {
	// TODO(jba): compare headers?
	if in.Method != cand.Method {
		return false
	}
	if in.URL.String() != cand.URL {
		return false
	}
	return inBody.equal(candBody)
}

// Convert a HAR response to a Go http.Response.
// HAR (Http ARchive) is a standard for storing HTTP interactions.
// See http://www.softwareishard.com/blog/har-12-spec.
func harResponseToHTTPResponse(hr *har.Response) *http.Response {
	return &http.Response{
		StatusCode: hr.Status,
		Status:     hr.StatusText,
		Proto:      hr.HTTPVersion,
		// TODO(jba): headers?
		Body:          ioutil.NopCloser(bytes.NewReader(hr.Content.Text)),
		ContentLength: int64(len(hr.Content.Text)),
	}
}

// A requestBody represents the body of a request. If the content type is multipart, the
// body is split into parts.
//
// The replaying proxy needs to understand multipart bodies because the boundaries are
// generated randomly, so we can't just compare the entire bodies for equality.
type requestBody struct {
	mediaType string   // the media type part of the Content-Type header
	parts     [][]byte // the parts of the body, or just a single []byte if not multipart
}

func newRequestBodyFromHTTP(req *http.Request) (*requestBody, error) {
	defer req.Body.Close()
	return newRequestBody(req.Header.Get("Content-Type"), req.Body)
}

func newRequestBodyFromHAR(req *har.Request) (*requestBody, error) {
	if req.PostData == nil {
		return nil, nil
	}
	var cth string
	for _, h := range req.Headers {
		if h.Name == "Content-Type" {
			cth = h.Value
			break
		}
	}
	return newRequestBody(cth, strings.NewReader(req.PostData.Text))
}

// newRequestBody parses the Content-Type header, reads the body, and splits it into
// parts if necessary.
func newRequestBody(contentType string, body io.Reader) (*requestBody, error) {
	if contentType == "" {
		// No content-type header. There should not be a body.
		if _, err := body.Read(make([]byte, 1)); err != io.EOF {
			return nil, errors.New("no Content-Type, but body")
		}
		return nil, nil
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, err
	}
	rb := &requestBody{mediaType: mediaType}
	if strings.HasPrefix(mediaType, "multipart/") {
		mr := multipart.NewReader(body, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			part, err := ioutil.ReadAll(p)
			if err != nil {
				return nil, err
			}
			// TODO(jba): care about part headers?
			rb.parts = append(rb.parts, part)
		}
	} else {
		bytes, err := ioutil.ReadAll(body)
		if err != nil {
			return nil, err
		}
		rb.parts = [][]byte{bytes}
	}
	return rb, nil
}

func (r1 *requestBody) equal(r2 *requestBody) bool {
	if r1 == nil || r2 == nil {
		return r1 == r2
	}
	if r1.mediaType != r2.mediaType {
		return false
	}
	if len(r1.parts) != len(r2.parts) {
		return false
	}
	for i, p1 := range r1.parts {
		if !bytes.Equal(p1, r2.parts[i]) {
			return false
		}
	}
	return true
}
