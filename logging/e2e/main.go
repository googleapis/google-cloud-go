// Copyright 2020 Google LLC
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

// Sample run-helloworld is a minimal Cloud Run service.
package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"cloud.google.com/go/compute/metadata"
	// This is replaced by the local version of cloud logging
	"cloud.google.com/go/logging"
)

// PubSubMessage is the payload of a Pub/Sub event.
type pubSubMessage struct {
	Message struct {
			Data []byte `json:"data,omitempty"`
			ID   string `json:"id"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

// HandlePubsub processes a Pub/Sub push message through http.
// We cannot use Pub/Sub pull subscriptions because Cloud Run only allocates CPU during the processing of a request.
func handlePubSub(w http.ResponseWriter, r *http.Request) {
	projectID, err := metadata.ProjectID()
	if err != nil {
		log.Fatalf("metadata.ProjectID: %v", err)
	}
	ctx := context.Background()

	var m pubSubMessage
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("ioutil.ReadAll: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(body, &m); err != nil {
		log.Printf("json.Unmarshal: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Logging setup
	logClient, err := logging.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer logClient.Close()

	// Overridable default label
	label := map[string]string{"testName": "testStdLog"} 
	logger := logClient.Logger(os.Getenv("TOPIC_ID"), logging.CommonLabels(label))

	msg := string(m.Message.Data)
	if strings.Contains(msg, "testStdLog"){
		testStdLog(logger)
	}
	if strings.Contains(msg, "testBasicLog"){
		testBasicLog(logger)
	}
}

func main() {
	environment := os.Getenv("ENVIRONMENT")
	if (environment == "cloudrun") {
		http.HandleFunc("/", handlePubSub)
		port := os.Getenv("PORT")
		if port == "" {
				port = "8080"
				log.Printf("Defaulting to port %s", port)
		}
		// Start HTTP server.
		log.Printf("Listening on port %s", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
				log.Fatal(err)
		}
	}
}

// Logs used in tests
func testStdLog(logger *logging.Logger) {
	stdLogger := logger.StandardLogger(logging.Info)
	stdLogger.Println("hello world")
}

func testBasicLog(logger *logging.Logger) {
	logger.Log(logging.Entry{
		Payload: "hello world",
		Labels: map[string]string{"testName":"testBasicLog"},
	})
}