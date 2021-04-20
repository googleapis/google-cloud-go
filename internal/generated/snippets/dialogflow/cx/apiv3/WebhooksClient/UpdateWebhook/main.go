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

// [START dialogflow_generated_dialogflow_cx_apiv3_WebhooksClient_UpdateWebhook]

package main

import (
	"context"

	cx "cloud.google.com/go/dialogflow/cx/apiv3"
	cxpb "google.golang.org/genproto/googleapis/cloud/dialogflow/cx/v3"
)

func main() {
	// import cxpb "google.golang.org/genproto/googleapis/cloud/dialogflow/cx/v3"

	ctx := context.Background()
	c, err := cx.NewWebhooksClient(ctx)
	if err != nil {
		// TODO: Handle error.
	}

	req := &cxpb.UpdateWebhookRequest{
		// TODO: Fill request struct fields.
	}
	resp, err := c.UpdateWebhook(ctx, req)
	if err != nil {
		// TODO: Handle error.
	}
	// TODO: Use resp.
	_ = resp
}

// [END dialogflow_generated_dialogflow_cx_apiv3_WebhooksClient_UpdateWebhook]
