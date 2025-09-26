package bigtable

import (
	"context"
	"log"
	"os"
)

func ExampleCheckDirectAccessSupported() {
	// Example Usage:
	ctx := context.Background()
	projectID := "my-project"
	instanceID := "my-instance"
	appProfileID := "default"

	// Set the environment variable if not already set
	os.Setenv("CBT_ENABLE_DIRECTPATH", "true")

	isDirectPath, err := CheckDirectAccessSupported(ctx, projectID, instanceID, appProfileID)
	if err != nil {
		log.Fatalf("DirectPath check failed: %v", err)
	}

	if isDirectPath {
		log.Printf("DirectPath connectivity is active for %s/%s", projectID, instanceID)
	} else {
		log.Printf("DirectPath connectivity is NOT active for %s/%s", projectID, instanceID)
	}
}
