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

// Package cloud contains Google Cloud Platform APIs related types
// and common functions.
package cloud // import "google.golang.org/cloud"

import (
	"net/http"

	"google.golang.org/cloud/internal"

	container "code.google.com/p/google-api-go-client/container/v1beta1"
	pubsub "code.google.com/p/google-api-go-client/pubsub/v1beta1"
	storage "code.google.com/p/google-api-go-client/storage/v1"
	"golang.org/x/net/context"
)

// NewContext returns a new context that uses the provided http.Client.
// Provided http.Client is responsible to authorize and authenticate
// the requests made to the Google Cloud APIs.
// It mutates the client's original Transport to append the cloud
// package's user-agent to the outgoing requests.
// You can obtain the project ID from the Google Developers Console,
// https://console.developers.google.com.
func NewContext(projID string, c *http.Client) context.Context {
	return WithContext(context.Background(), projID, c)
}

// WithContext returns a new context in a similar way NewContext does,
// but initiates the new context with the specified parent.
func WithContext(parent context.Context, projID string, c *http.Client) context.Context {
	if _, ok := c.Transport.(*internal.Transport); !ok {
		c.Transport = &internal.Transport{Base: c.Transport}
	}
	vals := make(map[string]interface{})
	vals["project_id"] = projID
	vals["http_client"] = c
	// TODO(jbd): Lazily initiate the service objects.
	// There is no datastore service as we use the proto directly
	// without passing through google-api-go-client.
	vals["pubsub_service"], _ = pubsub.New(c)
	vals["storage_service"], _ = storage.New(c)
	vals["container_service"], _ = container.New(c)
	return context.WithValue(parent, internal.ContextKey("base"), vals)
}
