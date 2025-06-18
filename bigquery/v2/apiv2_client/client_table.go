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

// GetTable gets the specified table resource by table ID.
// This method does not return the data in the table, it only returns the
// table resource, which describes the structure of this table.
func (mc *Client) GetTable(ctx context.Context, req *bigquerypb.GetTableRequest, opts ...gax.CallOption) (*bigquerypb.Table, error) {
	return mc.tblClient.GetTable(ctx, req, opts...)
}

// InsertTable creates a new, empty table in the dataset.
func (mc *Client) InsertTable(ctx context.Context, req *bigquerypb.InsertTableRequest, opts ...gax.CallOption) (*bigquerypb.Table, error) {
	return mc.tblClient.InsertTable(ctx, req, opts...)
}

// PatchTable updates information in an existing table. The update method replaces the
// entire table resource, whereas the patch method only replaces fields that
// are provided in the submitted table resource.
// This method supports RFC5789 patch semantics.
func (mc *Client) PatchTable(ctx context.Context, req *bigquerypb.UpdateOrPatchTableRequest, opts ...gax.CallOption) (*bigquerypb.Table, error) {
	return mc.tblClient.PatchTable(ctx, req, opts...)
}

// UpdateTable updates information in an existing table. The update method replaces the
// entire Table resource, whereas the patch method only replaces fields that
// are provided in the submitted Table resource.
func (mc *Client) UpdateTable(ctx context.Context, req *bigquerypb.UpdateOrPatchTableRequest, opts ...gax.CallOption) (*bigquerypb.Table, error) {
	return mc.tblClient.UpdateTable(ctx, req, opts...)
}

// DeleteTable deletes the table specified by tableId from the dataset.
// If the table contains data, all the data will be deleted.
func (mc *Client) DeleteTable(ctx context.Context, req *bigquerypb.DeleteTableRequest, opts ...gax.CallOption) error {
	return mc.tblClient.DeleteTable(ctx, req, opts...)
}

// ListTables lists all tables in the specified dataset. Requires the READER dataset
// role.
func (mc *Client) ListTables(ctx context.Context, req *bigquerypb.ListTablesRequest, opts ...gax.CallOption) *bigquery.ListFormatTableIterator {
	return mc.tblClient.ListTables(ctx, req, opts...)
}
