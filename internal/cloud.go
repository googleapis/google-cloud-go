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

// Package internal provides support for the cloud packages.
//
// Users should not import this package directly.
package internal

import (
	"fmt"
	"net/http"

	"golang.org/x/net/context"
)

// Key represents a context key. It shouldn't be used by the
// third party packages to avoid collisions.
type ContextKey string

const userAgent = "gcloud-golang/0.1"

// Transport is an http.RoundTripper that appends
// Google Cloud client's user-agent to the original
// request's user-agent header.
type Transport struct {
	// Base represents the actual http.RoundTripper
	// the requests will be delegated to.
	Base http.RoundTripper
}

// RoundTrip appends a user-agent to the existing user-agent
// header and delegates the request to the base http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = cloneRequest(req)
	ua := req.Header.Get("User-Agent")
	if ua == "" {
		ua = userAgent
	} else {
		ua = fmt.Sprintf("%s;%s", ua, userAgent)
	}
	req.Header.Set("User-Agent", ua)
	return t.Base.RoundTrip(req)
}

// cloneRequest returns a clone of the provided *http.Request.
// The clone is a shallow copy of the struct and its Header map.
func cloneRequest(r *http.Request) *http.Request {
	// shallow copy of the struct
	r2 := new(http.Request)
	*r2 = *r
	// deep copy of the Header
	r2.Header = make(http.Header)
	for k, s := range r.Header {
		r2.Header[k] = s
	}
	return r2
}

// ProjID gets the active project id for a context
func ProjID(ctx context.Context) string {
	return ctx.Value(ContextKey("base")).(map[string]interface{})["project_id"].(string)
}

// Namespace gets the active namespace for a context
// defaults to "" if no namespace was specified
func Namespace(ctx context.Context) string {
	v := ctx.Value(ContextKey("namespace"))
	if v == nil {
		return ""
	} else {
		return v.(string)
	}
}

func HttpClient(ctx context.Context) *http.Client {
	return ctx.Value(ContextKey("base")).(map[string]interface{})["http_client"].(*http.Client)
}
