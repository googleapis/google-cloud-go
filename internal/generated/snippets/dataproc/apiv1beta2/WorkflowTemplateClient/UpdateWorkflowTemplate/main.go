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

// [START dataproc_generated_dataproc_apiv1beta2_WorkflowTemplateClient_UpdateWorkflowTemplate]

package main

import (
	"context"

	dataproc "cloud.google.com/go/dataproc/apiv1beta2"
	dataprocpb "google.golang.org/genproto/googleapis/cloud/dataproc/v1beta2"
)

func main() {
	// import dataprocpb "google.golang.org/genproto/googleapis/cloud/dataproc/v1beta2"

	ctx := context.Background()
	c, err := dataproc.NewWorkflowTemplateClient(ctx)
	if err != nil {
		// TODO: Handle error.
	}

	req := &dataprocpb.UpdateWorkflowTemplateRequest{
		// TODO: Fill request struct fields.
	}
	resp, err := c.UpdateWorkflowTemplate(ctx, req)
	if err != nil {
		// TODO: Handle error.
	}
	// TODO: Use resp.
	_ = resp
}

// [END dataproc_generated_dataproc_apiv1beta2_WorkflowTemplateClient_UpdateWorkflowTemplate]
