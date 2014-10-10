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

// Package datastore is a Google Cloud Datastore client.
//
// More information about Google Cloud Datastore is available on
// https://cloud.google.com/datastore/docs
package datastore

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"reflect"

	"code.google.com/p/goprotobuf/proto"

	pb "google.golang.org/cloud/internal/datastore"
)

const (
	// ScopeDataStore grants permissions to view and/or manage datastore entities
	ScopeDatastore = "https://www.googleapis.com/auth/datastore"
	ScopeUserEmail = "https://www.googleapis.com/auth/userinfo.email"
)

var typeOfCommitRequest = reflect.TypeOf(pb.CommitRequest{})

// Client is a Google Cloud Datastore client.
type Client struct {
	projectId string
	c         *http.Client
	// BasePath is the root path to where all API requests will go
	// This can be changed to, for example, mocking or local test servers
	// Such as: https://cloud.google.com/datastore/docs/tools/devserver
	// The default base path is https://www.googleapis.com/datastore/v1beta2/datasets/
	BasePath  string
	namespace string

	transaction         []byte
	transactionFinished bool
}

// New creates a new Datastore client to manage datastore entities
// under the provided project. The provided RoundTripper should be
// authorized and authenticated to make calls to Google Cloud Datastore API.
// See the package examples for how to create an authorized http.RoundTripper.
func New(projID string, tr http.RoundTripper) *Client {
	return NewNS(projID, "", tr)
}

// NewNS creates a new Datastore client to manage datastore entities
// under the provided project and namespace. The provided RoundTripper should be
// authorized and authenticated to make calls to Google Cloud Datastore API.
// See the package examples for how to create an authorized http.RoundTripper.
func NewNS(projID string, namespace string, tr http.RoundTripper) *Client {
	return NewWithClientNS(projID, namespace, &http.Client{Transport: tr})
}

// NewWithClient creates a new Datastore client to datastore entities
// under the provided project. The client's transport should be
// authorized and authenticated to make calls to Google Cloud Datastore API.
// See the package examples for how to create an authorized http.RoundTripper.
func NewWithClient(projID string, c *http.Client) *Client {
	return NewWithClientNS(projID, "", c)
}

// NewWithClientNS creates a new Datastore client to datastore entities
// under the provided project and namespace. The client's transport should be
// authorized and authenticated to make calls to Google Cloud Datastore API.
// See the package examples for how to create an authorized http.RoundTripper.
func NewWithClientNS(projID string, namespace string, c *http.Client) *Client {
	// TODO(jbd): Add user-agent.
	return &Client{projectId: projID, c: c, namespace: namespace, BasePath: "https://www.googleapis.com/datastore/v1beta2/datasets/"}
}

// A namespace is how datastore allows for multitenancy, entities in
// any one namespace are entirely distinct and isolated from other namespaces
// See: https://cloud.google.com/appengine/docs/go/multitenancy/multitenancy
func (client *Client) Namespace() string {
	return client.namespace
}

func (client *Client) call(method string, req proto.Message, resp proto.Message) (err error) {
	if client.transaction != nil {
		if client.transactionFinished {
			return errors.New("datastore: transaction context has expired")
		}
	}

	payload, err := proto.Marshal(req)
	if err != nil {
		return
	}

	r, err := client.c.Post(client.BasePath+client.projectId+"/"+method, "application/x-protobuf", bytes.NewBuffer(payload))
	if err != nil {
		return
	}

	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if r.StatusCode != http.StatusOK {
		if err != nil {
			return err
		}
		return errors.New("datastore: error during call: " + string(body))
	}
	if err != nil {
		return err
	}
	if err = proto.Unmarshal(body, resp); err != nil {
		return
	}
	return
}
