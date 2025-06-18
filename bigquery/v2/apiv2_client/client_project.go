// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package apiv2_client

import (
	"context"

	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	gax "github.com/googleapis/gax-go/v2"
)

// GetServiceAccount rPC to get the service account for a project used for interactions with
// Google Cloud KMS
func (mc *Client) GetServiceAccount(ctx context.Context, req *bigquerypb.GetServiceAccountRequest, opts ...gax.CallOption) (*bigquerypb.GetServiceAccountResponse, error) {
	return mc.projClient.GetServiceAccount(ctx, req, opts...)
}
