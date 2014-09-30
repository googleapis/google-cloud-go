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
	"testing"

	"github.com/golang/oauth2/google"
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
	conf, err := google.NewServiceAccountJSONConfig(
		"/path/to/json/keyfile.json",
		pubsub.ScopeCloudPlatform,
		pubsub.ScopePubSub)
	if err != nil {
		log.Fatal(err)
	}

	c := pubsub.New("project-id", conf.NewTransport())
	_ = c // Use the client
}

func Example_publishAndSubscribe() {
	c := (*pubsub.Client)(nil) // initiate a pubsub client. See the auth example.

	// Publish hello world on topic1.
	go func() {
		for {
			topic := c.TopicClient("topic1")
			err := topic.Publish([]byte("hello"), nil)
			if err != nil {
				log.Println(err)
			}
		}
	}()

	sub := c.SubClient("sub1")
	// sub1 is a subscription that is subscribed to topic1.
	// E.g. sub.Create("topic1", time.Duration(0), "")
	for {
		m, err := sub.PullWait()
		if err != nil {
			log.Println(err)
		} else {
			log.Println("new message arrived:", m)
			if err := sub.Ack(m.AckID); err != nil {
				log.Println("error while acknowledging the message:", err)
			}
		}
	}
}
