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

// [START logging_generated_logging_HTTPRequest]

package main

import (
	"context"
	"net/http"

	"cloud.google.com/go/logging"
)

func main() {
	ctx := context.Background()
	client, err := logging.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	lg := client.Logger("my-log")
	httpEntry := logging.Entry{
		Payload: "optional message",
		HTTPRequest: &logging.HTTPRequest{
			// TODO: pass in request
			Request: &http.Request{},
			// TODO: set the status code
			Status: http.StatusOK,
		},
	}
	lg.Log(httpEntry)
}

// [END logging_generated_logging_HTTPRequest]
