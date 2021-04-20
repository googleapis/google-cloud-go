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

// [START logging_generated_logging_ToLogEntry]

package main

import (
	"context"

	"cloud.google.com/go/logging"
	vkit "cloud.google.com/go/logging/apiv2"
	logpb "google.golang.org/genproto/googleapis/logging/v2"
)

func main() {
	e := logging.Entry{
		Payload: "Message",
	}
	le, err := logging.ToLogEntry(e, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	client, err := vkit.NewClient(context.Background())
	if err != nil {
		// TODO: Handle error.
	}
	_, err = client.WriteLogEntries(context.Background(), &logpb.WriteLogEntriesRequest{
		Entries: []*logpb.LogEntry{le},
		LogName: "stdout",
	})
	if err != nil {
		// TODO: Handle error.
	}
}

// [END logging_generated_logging_ToLogEntry]
