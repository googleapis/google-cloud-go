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

// [START servicemanagement_generated_servicemanagement_apiv1_ServiceManagerClient_DisableService]

package main

import (
	"context"

	servicemanagement "cloud.google.com/go/servicemanagement/apiv1"
	servicemanagementpb "google.golang.org/genproto/googleapis/api/servicemanagement/v1"
)

func main() {
	// import servicemanagementpb "google.golang.org/genproto/googleapis/api/servicemanagement/v1"

	ctx := context.Background()
	c, err := servicemanagement.NewServiceManagerClient(ctx)
	if err != nil {
		// TODO: Handle error.
	}

	req := &servicemanagementpb.DisableServiceRequest{
		// TODO: Fill request struct fields.
	}
	op, err := c.DisableService(ctx, req)
	if err != nil {
		// TODO: Handle error.
	}

	resp, err := op.Wait(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	// TODO: Use resp.
	_ = resp
}

// [END servicemanagement_generated_servicemanagement_apiv1_ServiceManagerClient_DisableService]
