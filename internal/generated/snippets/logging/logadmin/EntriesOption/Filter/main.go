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

// [START logging_generated_logging_logadmin_Filter_timestamp]

package main

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/logging/logadmin"
)

func main() {
	// This example demonstrates how to list the last 24 hours of log entries.
	ctx := context.Background()
	client, err := logadmin.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	oneDayAgo := time.Now().Add(-24 * time.Hour)
	t := oneDayAgo.Format(time.RFC3339) // Logging API wants timestamps in RFC 3339 format.
	it := client.Entries(ctx, logadmin.Filter(fmt.Sprintf(`timestamp > "%s"`, t)))
	_ = it // TODO: iterate using Next or iterator.Pager.
}

// [END logging_generated_logging_logadmin_Filter_timestamp]
