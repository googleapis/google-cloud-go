// Sample run-helloworld is a minimal Cloud Run service.
package main

import (
        "context"
        // "fmt"
        "log"
        "net/http"

        "cloud.google.com/go/internal/testutil"
        // This is replaced by the local version of cloud logging
        "cloud.google.com/go/logging"
)

func main() {
        log.Print("starting server...")
        http.HandleFunc("/", handler)

        // Determine port for HTTP service.
        port := "8080"
        // Start HTTP server.
        log.Printf("listening on port %s", port)
        if err := http.ListenAndServe(":"+port, nil); err != nil {
                log.Fatal(err)
        }
}

// HTTP Trigger: post/message
// Expect array of tests to run, unique logName (that's how i retrieve logs later)
func handler(w http.ResponseWriter, r *http.Request) {
        ctx := context.Background()
        testProjectID := testutil.ProjID()
        client, err := logging.NewClient(ctx, testProjectID)
        if err != nil {
                log.Fatalf("Failed to create client: %v", err)
        }
        
        const name = "cloud-run-log" // TODO replace from httprequest
        logger := client.Logger(name)
        defer logger.Flush() // Ensure the entry is written

        // TODO Log the logs
        logger.Log(logging.Entry{Payload: "THIS IS A LOG"})

        if err := client.Close(); err != nil {
                log.Fatalf("Failed to close client: %v", err)
        }

        log.Print("Finished handler execution")
}

// TODO(nicolezhu): test CloudEvent Trigger
// TODO(nicolezhu): test gRPC Trigger

// *** Available Test Functions ***
