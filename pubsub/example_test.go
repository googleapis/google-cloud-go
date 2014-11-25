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

package pubsub_test

import (
	"log"
	"net/http"
	"testing"

	"github.com/golang/oauth2"
	"github.com/golang/oauth2/google"
	"google.golang.org/cloud"
	"google.golang.org/cloud/pubsub"
)

// TODO(jbd): Remove after Go 1.4.
// Related to https://codereview.appspot.com/107320046
func TestA(t *testing.T) {}

func Example_auth() {
	// Initialize an authorized transport with Google Developers Console
	// JSON key. Read the google package examples to learn more about
	// different authorization flows you can use.
	// http://godoc.org/github.com/golang/oauth2/google
	opts, err := oauth2.New(
		google.ServiceAccountJSONKey("/path/to/json/keyfile.json"),
		oauth2.Scope(
			pubsub.ScopeCloudPlatform,
			pubsub.ScopePubSub,
		),
	)
	if err != nil {
		log.Fatal(err)
	}

	c := &http.Client{Transport: opts.NewTransport()}
	ctx := cloud.NewContext("project-id", c)
	_ = ctx // See the other samples to learn how to use the context.
}

func Example_publishAndSubscribe() {
	tr := (http.RoundTripper)(nil) // initiate a RoundTripper. See the auth example.
	ctx := cloud.NewContext("project-id", &http.Client{Transport: tr})

	// Publish hello world on topic1.
	go func() {
		for {
			err := pubsub.Publish(ctx, "topic1", []byte("hello"), nil)
			if err != nil {
				log.Println(err)
			}
		}
	}()

	// sub1 is a subscription that is subscribed to topic1.
	// E.g. c.CreateSub("sub1", "topic1", time.Duration(0), "")
	for {
		m, err := pubsub.PullWait(ctx, "sub1")
		if err != nil {
			log.Println(err)
		} else {
			log.Println("new message arrived:", m)
			if err := pubsub.Ack(ctx, "sub1", m.AckID); err != nil {
				log.Println("error while acknowledging the message:", err)
			}
		}
	}
}
