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

// [START osconfig_generated_osconfig_agentendpoint_apiv1beta_Client_StartNextTask]

package main

import (
	"context"

	agentendpoint "cloud.google.com/go/osconfig/agentendpoint/apiv1beta"
	agentendpointpb "google.golang.org/genproto/googleapis/cloud/osconfig/agentendpoint/v1beta"
)

func main() {
	// import agentendpointpb "google.golang.org/genproto/googleapis/cloud/osconfig/agentendpoint/v1beta"

	ctx := context.Background()
	c, err := agentendpoint.NewClient(ctx)
	if err != nil {
		// TODO: Handle error.
	}

	req := &agentendpointpb.StartNextTaskRequest{
		// TODO: Fill request struct fields.
	}
	resp, err := c.StartNextTask(ctx, req)
	if err != nil {
		// TODO: Handle error.
	}
	// TODO: Use resp.
	_ = resp
}

// [END osconfig_generated_osconfig_agentendpoint_apiv1beta_Client_StartNextTask]
