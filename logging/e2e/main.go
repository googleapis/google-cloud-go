// Sample run-helloworld is a minimal Cloud Run service.
package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	// "time"

	// This is replaced by the local version of cloud logging
	"cloud.google.com/go/logging"
	// "cloud.google.com/go/pubsub"

)

// PubSubMessage is the payload of a Pub/Sub event.
// TODO replace this with client lib?
type PubSubMessage struct {
	Message struct {
			Data []byte `json:"data,omitempty"`
			ID   string `json:"id"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

// HandlePubsub processes a Pub/Sub push message through http.
// You cannot use Pub/Sub pull subscriptions because Cloud Run only allocates CPU during the processing of a request.
func HandlePubSub(w http.ResponseWriter, r *http.Request) {
	projectID := "log-bench" //TODO ping api for this info
	ctx := context.Background()

	var m PubSubMessage
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
			log.Printf("ioutil.ReadAll: %v", err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
	}
	log.Printf("Body: \n: %v", body)
	log.Printf("Message: \n: %v", m)
	if err := json.Unmarshal(body, &m); err != nil {
			log.Printf("json.Unmarshal: %v", err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
	}

	name := string(m.Message.Data)
	if name == "" {
			name = "World"
	}
	log.Printf("Hello %s!", name)

	// Logging setup
	logClient, err := logging.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer logClient.Close()
	logger := logClient.Logger("my-log")
	logger.Log(logging.Entry{Payload: name})
}

func main() {
	// TODO: enable only for Cloud Run, update this depending on fnc being run 
	cloudRun := true
	if (cloudRun) {
		http.HandleFunc("/", HandlePubSub)
		// Determine port for HTTP service.
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

func testLogEntry(logger *logging.Logger) {
	logger.Log(logging.Entry{Payload: "THIS IS A LOG"})
}
