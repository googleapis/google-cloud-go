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

// ListRowAccessPolicies lists all row access policies on the specified table.
func (mc *Client) ListRowAccessPolicies(ctx context.Context, req *bigquerypb.ListRowAccessPoliciesRequest, opts ...gax.CallOption) *bigquery.RowAccessPolicyIterator {
	return mc.rapClient.ListRowAccessPolicies(ctx, req, opts...)
}

// GetRowAccessPolicy gets the specified row access policy by policy ID.
func (mc *Client) GetRowAccessPolicy(ctx context.Context, req *bigquerypb.GetRowAccessPolicyRequest, opts ...gax.CallOption) (*bigquerypb.RowAccessPolicy, error) {
	return mc.rapClient.GetRowAccessPolicy(ctx, req, opts...)
}

// CreateRowAccessPolicy creates a row access policy.
func (mc *Client) CreateRowAccessPolicy(ctx context.Context, req *bigquerypb.CreateRowAccessPolicyRequest, opts ...gax.CallOption) (*bigquerypb.RowAccessPolicy, error) {
	return mc.rapClient.CreateRowAccessPolicy(ctx, req, opts...)
}

// UpdateRowAccessPolicy updates a row access policy.
func (mc *Client) UpdateRowAccessPolicy(ctx context.Context, req *bigquerypb.UpdateRowAccessPolicyRequest, opts ...gax.CallOption) (*bigquerypb.RowAccessPolicy, error) {
	return mc.rapClient.UpdateRowAccessPolicy(ctx, req, opts...)
}

// DeleteRowAccessPolicy deletes a row access policy.
func (mc *Client) DeleteRowAccessPolicy(ctx context.Context, req *bigquerypb.DeleteRowAccessPolicyRequest, opts ...gax.CallOption) error {
	return mc.rapClient.DeleteRowAccessPolicy(ctx, req, opts...)
}

// BatchDeleteRowAccessPolicies deletes provided row access policies.
func (mc *Client) BatchDeleteRowAccessPolicies(ctx context.Context, req *bigquerypb.BatchDeleteRowAccessPoliciesRequest, opts ...gax.CallOption) error {
	return mc.rapClient.BatchDeleteRowAccessPolicies(ctx, req, opts...)
}
