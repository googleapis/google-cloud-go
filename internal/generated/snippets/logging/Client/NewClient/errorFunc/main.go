// Copyright 2021 Google LLC
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

// [START logging_generated_logging_NewClient_errorFunc]

package main

import (
	"context"
	"fmt"
	"os"

	"cloud.google.com/go/logging"
)

func main() {
	ctx := context.Background()
	client, err := logging.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	// Print all errors to stdout, and count them. Multiple calls to the OnError
	// function never happen concurrently, so there is no need for locking nErrs,
	// provided you don't read it until after the logging client is closed.
	var nErrs int
	client.OnError = func(e error) {
		fmt.Fprintf(os.Stdout, "logging: %v", e)
		nErrs++
	}
	// Use client to manage logs, metrics and sinks.
	// Close the client when finished.
	if err := client.Close(); err != nil {
		// TODO: Handle error.
	}
	fmt.Printf("saw %d errors\n", nErrs)
}

// [END logging_generated_logging_NewClient_errorFunc]
