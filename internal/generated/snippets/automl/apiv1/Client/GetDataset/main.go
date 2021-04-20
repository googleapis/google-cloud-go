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

// [START automl_generated_automl_apiv1_Client_GetDataset]

package main

import (
	"context"

	automl "cloud.google.com/go/automl/apiv1"
	automlpb "google.golang.org/genproto/googleapis/cloud/automl/v1"
)

func main() {
	// import automlpb "google.golang.org/genproto/googleapis/cloud/automl/v1"

	ctx := context.Background()
	c, err := automl.NewClient(ctx)
	if err != nil {
		// TODO: Handle error.
	}

	req := &automlpb.GetDatasetRequest{
		// TODO: Fill request struct fields.
	}
	resp, err := c.GetDataset(ctx, req)
	if err != nil {
		// TODO: Handle error.
	}
	// TODO: Use resp.
	_ = resp
}

// [END automl_generated_automl_apiv1_Client_GetDataset]
