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
	// "time"

	"cloud.google.com/go/compute/metadata"
	// This is replaced by the local version of cloud logging
	"cloud.google.com/go/logging"
	// "cloud.google.com/go/pubsub"

)

// PubSubMessage is the payload of a Pub/Sub event.
// TODO replace this with client lib?
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
	// projectID := "log-bench" // testmode
	ctx := context.Background()

	var m pubSubMessage
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("ioutil.ReadAll: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	// log.Printf("Body: \n: %v", body)
	// log.Printf("Message: \n: %v", m)
	if err := json.Unmarshal(body, &m); err != nil {
		log.Printf("json.Unmarshal: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Logging setup
	// TODO generalize this snippet to be used by other calls
	logClient, err := logging.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer logClient.Close()

	label := make(map[string]string)
	label["testId"] = os.Getenv("SUB_ID")
	label["testEnv"] = "Cloud Run"
	label["testName"] = "testStdLog" // Overridable default
	logger := logClient.Logger("my-log", logging.CommonLabels(label))

	msg := string(m.Message.Data)
	// msg = "testStdLog, testBasicLog" //testmode
	if strings.Contains(msg, "testStdLog"){
		testStdLog(logger)
	}
	if strings.Contains(msg, "testBasicLog"){
		testBasicLog(logger)
	}
}

func main() {
	// TODO: enable only for Cloud Run, update this depending on fnc being run 
	cloudRun := true
	if (cloudRun) {
		http.HandleFunc("/", handlePubSub)
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

// Common tests
func testStdLog(logger *logging.Logger) {
	stdLogger := logger.StandardLogger(logging.Info)
	stdLogger.Println("hi there")
}

func testBasicLog(logger *logging.Logger) {
	logger.Log(logging.Entry{
		Payload: "hello world",
		Labels: map[string]string{"testName":"testBasicLog"},
	})
}