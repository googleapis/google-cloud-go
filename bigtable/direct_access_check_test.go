/*
Copyright 2025 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
