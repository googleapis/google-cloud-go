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

// GetRoutine gets the specified routine resource by routine ID.
func (mc *Client) GetRoutine(ctx context.Context, req *bigquerypb.GetRoutineRequest, opts ...gax.CallOption) (*bigquerypb.Routine, error) {
	return mc.routClient.GetRoutine(ctx, req, opts...)
}

// InsertRoutine creates a new routine in the dataset.
func (mc *Client) InsertRoutine(ctx context.Context, req *bigquerypb.InsertRoutineRequest, opts ...gax.CallOption) (*bigquerypb.Routine, error) {
	return mc.routClient.InsertRoutine(ctx, req, opts...)
}

// UpdateRoutine updates information in an existing routine. The update method replaces the
// entire Routine resource.
func (mc *Client) UpdateRoutine(ctx context.Context, req *bigquerypb.UpdateRoutineRequest, opts ...gax.CallOption) (*bigquerypb.Routine, error) {
	return mc.routClient.UpdateRoutine(ctx, req, opts...)
}

// PatchRoutine patches information in an existing routine. The patch method does a partial
// update to an existing Routine resource.
func (mc *Client) PatchRoutine(ctx context.Context, req *bigquerypb.PatchRoutineRequest, opts ...gax.CallOption) (*bigquerypb.Routine, error) {
	return mc.routClient.PatchRoutine(ctx, req, opts...)
}

// DeleteRoutine deletes the routine specified by routineId from the dataset.
func (mc *Client) DeleteRoutine(ctx context.Context, req *bigquerypb.DeleteRoutineRequest, opts ...gax.CallOption) error {
	return mc.routClient.DeleteRoutine(ctx, req, opts...)
}

// ListRoutines lists all routines in the specified dataset. Requires the READER dataset
// role.
func (mc *Client) ListRoutines(ctx context.Context, req *bigquerypb.ListRoutinesRequest, opts ...gax.CallOption) *bigquery.RoutineIterator {
	return mc.routClient.ListRoutines(ctx, req, opts...)
}
