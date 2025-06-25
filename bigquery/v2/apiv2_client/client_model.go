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

	bigquery "cloud.google.com/go/bigquery/v2/apiv2"
	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"

	gax "github.com/googleapis/gax-go/v2"
)

// GetModel gets the specified model resource by model ID.
func (mc *Client) GetModel(ctx context.Context, req *bigquerypb.GetModelRequest, opts ...gax.CallOption) (*bigquerypb.Model, error) {
	return mc.modelClient.GetModel(ctx, req, opts...)
}

// ListModels lists all models in the specified dataset. Requires the READER dataset
// role. After retrieving the list of models, you can get information about a
// particular model by calling the models.get method.
func (mc *Client) ListModels(ctx context.Context, req *bigquerypb.ListModelsRequest, opts ...gax.CallOption) *bigquery.ModelIterator {
	return mc.modelClient.ListModels(ctx, req, opts...)
}

// PatchModel patch specific fields in the specified model.
func (mc *Client) PatchModel(ctx context.Context, req *bigquerypb.PatchModelRequest, opts ...gax.CallOption) (*bigquerypb.Model, error) {
	return mc.modelClient.PatchModel(ctx, req, opts...)
}

// DeleteModel deletes the model specified by modelId from the dataset.
func (mc *Client) DeleteModel(ctx context.Context, req *bigquerypb.DeleteModelRequest, opts ...gax.CallOption) error {
	return mc.modelClient.DeleteModel(ctx, req, opts...)
}
