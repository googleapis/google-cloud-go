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

// GetDataset returns the dataset specified by datasetID.
func (mc *Client) GetDataset(ctx context.Context, req *bigquerypb.GetDatasetRequest, opts ...gax.CallOption) (*bigquerypb.Dataset, error) {
	return mc.dsClient.GetDataset(ctx, req, opts...)
}

// InsertDataset creates a new empty dataset.
func (mc *Client) InsertDataset(ctx context.Context, req *bigquerypb.InsertDatasetRequest, opts ...gax.CallOption) (*bigquerypb.Dataset, error) {
	return mc.dsClient.InsertDataset(ctx, req, opts...)
}

// PatchDataset updates information in an existing dataset. The update method replaces the
// entire dataset resource, whereas the patch method only replaces fields that
// are provided in the submitted dataset resource.
// This method supports RFC5789 patch semantics.
func (mc *Client) PatchDataset(ctx context.Context, req *bigquerypb.UpdateOrPatchDatasetRequest, opts ...gax.CallOption) (*bigquerypb.Dataset, error) {
	return mc.dsClient.PatchDataset(ctx, req, opts...)
}

// UpdateDataset updates information in an existing dataset. The update method replaces the
// entire dataset resource, whereas the patch method only replaces fields that
// are provided in the submitted dataset resource.
func (mc *Client) UpdateDataset(ctx context.Context, req *bigquerypb.UpdateOrPatchDatasetRequest, opts ...gax.CallOption) (*bigquerypb.Dataset, error) {
	return mc.dsClient.UpdateDataset(ctx, req, opts...)
}

// DeleteDataset deletes the dataset specified by the datasetId value. Before you can delete
// a dataset, you must delete all its tables, either manually or by specifying
// deleteContents. Immediately after deletion, you can create another dataset
// with the same name.
func (mc *Client) DeleteDataset(ctx context.Context, req *bigquerypb.DeleteDatasetRequest, opts ...gax.CallOption) error {
	return mc.dsClient.DeleteDataset(ctx, req, opts...)
}

// ListDatasets lists all datasets in the specified project to which the user has been
// granted the READER dataset role.
func (mc *Client) ListDatasets(ctx context.Context, req *bigquerypb.ListDatasetsRequest, opts ...gax.CallOption) *bigquery.ListFormatDatasetIterator {
	return mc.dsClient.ListDatasets(ctx, req, opts...)
}

// UndeleteDataset undeletes a dataset which is within time travel window based on datasetId.
// If a time is specified, the dataset version deleted at that time is
// undeleted, else the last live version is undeleted.
func (mc *Client) UndeleteDataset(ctx context.Context, req *bigquerypb.UndeleteDatasetRequest, opts ...gax.CallOption) (*bigquerypb.Dataset, error) {
	return mc.dsClient.UndeleteDataset(ctx, req, opts...)
}
