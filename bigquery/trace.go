// Copyright 2026 Google LLC
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

package bigquery

import (
	"context"
	"fmt"
	"strings"

	"github.com/googleapis/gax-go/v2"
	"github.com/googleapis/gax-go/v2/callctx"
)

// setTraceMetadata sets the trace metadata into the context. It assumes the caller has already checked if tracing is enabled.
func setTraceMetadata(ctx context.Context, resourceName, urlTemplate string) context.Context {
	return callctx.WithTelemetryContext(ctx, "resource_name", resourceName, "url_template", urlTemplate)
}

// datasetResourceName constructs the standard resource name for a dataset.
// E.g., "//bigquery.googleapis.com/projects/{project}/datasets/{dataset}"
func datasetResourceName(projectID, datasetID string) string {
	return fmt.Sprintf("//bigquery.googleapis.com/projects/%s/datasets/%s", projectID, datasetID)
}

// modelResourceName constructs the standard resource name for a model.
// E.g., "//bigquery.googleapis.com/projects/{project}/datasets/{dataset}/models/{model}"
func modelResourceName(projectID, datasetID, modelID string) string {
	return fmt.Sprintf("//bigquery.googleapis.com/projects/%s/datasets/%s/models/%s", projectID, datasetID, modelID)
}

func fullyQualifiedDatasetResourceName(projectID, datasetID string) string {
	if strings.HasPrefix(datasetID, "projects/") {
		// Handle fully qualified names
		return fmt.Sprintf("//bigquery.googleapis.com/%s", datasetID)
	}
	return datasetResourceName(projectID, datasetID)
}

func setProjectItemTraceMetadata(ctx context.Context, projectID, childType string) context.Context {
	if !gax.IsFeatureEnabled("TRACING") {
		return ctx
	}
	return setTraceMetadata(ctx,
		fmt.Sprintf("//bigquery.googleapis.com/projects/%s", projectID),
		fmt.Sprintf("/bigquery/v2/projects/{projectId}/%s", childType))
}

func setDatasetTraceMetadata(ctx context.Context, projectID, datasetID string) context.Context {
	if !gax.IsFeatureEnabled("TRACING") {
		return ctx
	}
	return setTraceMetadata(ctx,
		fullyQualifiedDatasetResourceName(projectID, datasetID),
		"/bigquery/v2/projects/{projectId}/datasets/{datasetId}")
}

func setDatasetItemTraceMetadata(ctx context.Context, projectID, datasetID, childType string) context.Context {
	if !gax.IsFeatureEnabled("TRACING") {
		return ctx
	}
	return setTraceMetadata(ctx,
		fullyQualifiedDatasetResourceName(projectID, datasetID),
		fmt.Sprintf("/bigquery/v2/projects/{projectId}/datasets/{datasetId}/%s", childType))
}

func setModelTraceMetadata(ctx context.Context, projectID, datasetID, modelID string) context.Context {
	if !gax.IsFeatureEnabled("TRACING") {
		return ctx
	}
	return setTraceMetadata(ctx,
		modelResourceName(projectID, datasetID, modelID),
		"/bigquery/v2/projects/{projectId}/datasets/{datasetId}/models/{modelId}")
}

// tableResourceName constructs the standard resource name for a table.
// E.g., "//bigquery.googleapis.com/projects/{project}/datasets/{dataset}/tables/{table}"
func tableResourceName(projectID, datasetID, tableID string) string {
	return fmt.Sprintf("//bigquery.googleapis.com/projects/%s/datasets/%s/tables/%s", projectID, datasetID, tableID)
}

// routineResourceName constructs the standard resource name for a routine.
// E.g., "//bigquery.googleapis.com/projects/{project}/datasets/{dataset}/routines/{routine}"
func routineResourceName(projectID, datasetID, routineID string) string {
	return fmt.Sprintf("//bigquery.googleapis.com/projects/%s/datasets/%s/routines/%s", projectID, datasetID, routineID)
}

func setTableTraceMetadata(ctx context.Context, projectID, datasetID, tableID string) context.Context {
	if !gax.IsFeatureEnabled("TRACING") {
		return ctx
	}
	return setTraceMetadata(ctx,
		tableResourceName(projectID, datasetID, tableID),
		"/bigquery/v2/projects/{projectId}/datasets/{datasetId}/tables/{tableId}")
}

func setRoutineTraceMetadata(ctx context.Context, projectID, datasetID, routineID string) context.Context {
	if !gax.IsFeatureEnabled("TRACING") {
		return ctx
	}
	return setTraceMetadata(ctx,
		routineResourceName(projectID, datasetID, routineID),
		"/bigquery/v2/projects/{projectId}/datasets/{datasetId}/routines/{routineId}")
}

// jobResourceName constructs the standard resource name for a job.
// E.g., "//bigquery.googleapis.com/projects/{project}/jobs/{jobId}"
func jobResourceName(projectID, jobID string) string {
	return fmt.Sprintf("//bigquery.googleapis.com/projects/%s/jobs/%s", projectID, jobID)
}

func setJobTraceMetadata(ctx context.Context, projectID, jobID string) context.Context {
	if !gax.IsFeatureEnabled("TRACING") {
		return ctx
	}
	return setTraceMetadata(ctx,
		jobResourceName(projectID, jobID),
		"/bigquery/v2/projects/{projectId}/jobs/{jobId}")
}

func setTableItemTraceMetadata(ctx context.Context, projectID, datasetID, tableID, childType string) context.Context {
	if !gax.IsFeatureEnabled("TRACING") {
		return ctx
	}
	return setTraceMetadata(ctx,
		tableResourceName(projectID, datasetID, tableID),
		fmt.Sprintf("/bigquery/v2/projects/{projectId}/datasets/{datasetId}/tables/{tableId}/%s", childType))
}

func setJobItemTraceMetadata(ctx context.Context, projectID, jobID, childType string) context.Context {
	if !gax.IsFeatureEnabled("TRACING") {
		return ctx
	}
	return setTraceMetadata(ctx,
		jobResourceName(projectID, jobID),
		fmt.Sprintf("/bigquery/v2/projects/{projectId}/jobs/{jobId}/%s", childType))
}
