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
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"cloud.google.com/go/bigquery/internal"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/gax-go/v2"
	"github.com/googleapis/gax-go/v2/callctx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	bq "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/option"
)

func TestSetDatasetTraceMetadata(t *testing.T) {
	// Enable tracing feature for the test
	os.Setenv("GOOGLE_SDK_GO_EXPERIMENTAL_TRACING", "true")
	defer os.Unsetenv("GOOGLE_SDK_GO_EXPERIMENTAL_TRACING")
	gax.TestOnlyResetIsFeatureEnabled()
	defer gax.TestOnlyResetIsFeatureEnabled()

	ctx := context.Background()
	projectID := "test-project"
	datasetID := "test-dataset"
	resourceName := "//bigquery.googleapis.com/projects/test-project/datasets/test-dataset"
	urlTemplate := "/bigquery/v2/projects/{projectId}/datasets/{datasetId}"

	ctx = setDatasetTraceMetadata(ctx, projectID, datasetID)

	res, ok := callctx.TelemetryFromContext(ctx, "resource_name")
	if !ok || res != resourceName {
		t.Errorf("expected resource_name %q, got %q", resourceName, res)
	}

	urlTmpl, ok := callctx.TelemetryFromContext(ctx, "url_template")
	if !ok || urlTmpl != urlTemplate {
		t.Errorf("expected url_template %q, got %q", urlTemplate, urlTmpl)
	}
}

func TestTracingTelemetryAttributes(t *testing.T) {
	os.Setenv("GOOGLE_SDK_GO_EXPERIMENTAL_TRACING", "true")
	defer os.Unsetenv("GOOGLE_SDK_GO_EXPERIMENTAL_TRACING")
	gax.TestOnlyResetIsFeatureEnabled()
	defer gax.TestOnlyResetIsFeatureEnabled()

	tests := []struct {
		name             string
		callFunc         func(ctx context.Context, client *Client)
		mockResponse     string
		mockStatusCodes  []int
		wantResourceName string
		wantURLTemplate  string
		wantAttempts     int
		wantMethod       string
	}{
		{
			name: "Dataset_Metadata",
			callFunc: func(ctx context.Context, client *Client) {
				_, _ = client.Dataset("test-dataset").Metadata(ctx)
			},
			mockResponse:     `{"id": "test-dataset", "datasetReference": {"projectId": "test-project", "datasetId": "test-dataset"}}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/datasets/test-dataset",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets/{datasetId}",
			wantAttempts:     1,
			wantMethod:       "GET",
		},
		{
			name: "Dataset_Create",
			callFunc: func(ctx context.Context, client *Client) {
				_ = client.Dataset("test-dataset").Create(ctx, &DatasetMetadata{})
			},
			mockResponse:     `{"id": "test-dataset", "datasetReference": {"projectId": "test-project", "datasetId": "test-dataset"}}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets",
			wantAttempts:     1,
			wantMethod:       "POST",
		},
		{
			name: "Dataset_Update",
			callFunc: func(ctx context.Context, client *Client) {
				_, _ = client.Dataset("test-dataset").Update(ctx, DatasetMetadataToUpdate{}, "")
			},
			mockResponse:     `{"id": "test-dataset", "datasetReference": {"projectId": "test-project", "datasetId": "test-dataset"}}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/datasets/test-dataset",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets/{datasetId}",
			wantAttempts:     1,
			wantMethod:       "PATCH",
		},
		{
			name: "Dataset_Delete",
			callFunc: func(ctx context.Context, client *Client) {
				_ = client.Dataset("test-dataset").Delete(ctx)
			},
			mockResponse:     `{}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/datasets/test-dataset",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets/{datasetId}",
			wantAttempts:     1,
			wantMethod:       "DELETE",
		},
		{
			name: "Client_Datasets",
			callFunc: func(ctx context.Context, client *Client) {
				it := client.Datasets(ctx)
				_, _ = it.Next()
			},
			mockResponse:     `{"datasets": [{"datasetReference": {"projectId": "test-project", "datasetId": "test-dataset"}}]}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets",
			wantAttempts:     1,
			wantMethod:       "GET",
		},
		{
			name: "Dataset_Models",
			callFunc: func(ctx context.Context, client *Client) {
				it := client.Dataset("test-dataset").Models(ctx)
				_, _ = it.Next()
			},
			mockResponse:     `{"models": [{"modelReference": {"projectId": "test-project", "datasetId": "test-dataset", "modelId": "test-model"}}]}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/datasets/test-dataset",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets/{datasetId}/models",
			wantAttempts:     1,
			wantMethod:       "GET",
		},
		{
			name: "Model_Metadata",
			callFunc: func(ctx context.Context, client *Client) {
				_, _ = client.Dataset("test-dataset").Model("test-model").Metadata(ctx)
			},
			mockResponse:     `{"modelReference": {"projectId": "test-project", "datasetId": "test-dataset", "modelId": "test-model"}}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/datasets/test-dataset/models/test-model",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets/{datasetId}/models/{modelId}",
			wantAttempts:     1,
			wantMethod:       "GET",
		},
		{
			name: "Model_Update",
			callFunc: func(ctx context.Context, client *Client) {
				_, _ = client.Dataset("test-dataset").Model("test-model").Update(ctx, ModelMetadataToUpdate{}, "")
			},
			mockResponse:     `{"modelReference": {"projectId": "test-project", "datasetId": "test-dataset", "modelId": "test-model"}}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/datasets/test-dataset/models/test-model",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets/{datasetId}/models/{modelId}",
			wantAttempts:     1,
			wantMethod:       "PATCH",
		},
		{
			name: "Model_Delete",
			callFunc: func(ctx context.Context, client *Client) {
				_ = client.Dataset("test-dataset").Model("test-model").Delete(ctx)
			},
			mockResponse:     `{}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/datasets/test-dataset/models/test-model",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets/{datasetId}/models/{modelId}",
			wantAttempts:     1,
			wantMethod:       "DELETE",
		},
		{
			name: "Table_Create",
			callFunc: func(ctx context.Context, client *Client) {
				_ = client.Dataset("test-dataset").Table("test-table").Create(ctx, &TableMetadata{})
			},
			mockResponse:     `{"tableReference": {"projectId": "test-project", "datasetId": "test-dataset", "tableId": "test-table"}}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/datasets/test-dataset",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets/{datasetId}/tables",
			wantAttempts:     1,
			wantMethod:       "POST",
		},
		{
			name: "Table_Metadata",
			callFunc: func(ctx context.Context, client *Client) {
				_, _ = client.Dataset("test-dataset").Table("test-table").Metadata(ctx)
			},
			mockResponse:     `{"tableReference": {"projectId": "test-project", "datasetId": "test-dataset", "tableId": "test-table"}}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/datasets/test-dataset/tables/test-table",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets/{datasetId}/tables/{tableId}",
			wantAttempts:     1,
			wantMethod:       "GET",
		},
		{
			name: "Table_Update",
			callFunc: func(ctx context.Context, client *Client) {
				_, _ = client.Dataset("test-dataset").Table("test-table").Update(ctx, TableMetadataToUpdate{}, "")
			},
			mockResponse:     `{"tableReference": {"projectId": "test-project", "datasetId": "test-dataset", "tableId": "test-table"}}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/datasets/test-dataset/tables/test-table",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets/{datasetId}/tables/{tableId}",
			wantAttempts:     1,
			wantMethod:       "PATCH",
		},
		{
			name: "Table_Delete",
			callFunc: func(ctx context.Context, client *Client) {
				_ = client.Dataset("test-dataset").Table("test-table").Delete(ctx)
			},
			mockResponse:     `{}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/datasets/test-dataset/tables/test-table",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets/{datasetId}/tables/{tableId}",
			wantAttempts:     1,
			wantMethod:       "DELETE",
		},
		{
			name: "Dataset_Tables",
			callFunc: func(ctx context.Context, client *Client) {
				it := client.Dataset("test-dataset").Tables(ctx)
				_, _ = it.Next()
			},
			mockResponse:     `{"tables": [{"tableReference": {"projectId": "test-project", "datasetId": "test-dataset", "tableId": "test-table"}}]}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/datasets/test-dataset",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets/{datasetId}/tables",
			wantAttempts:     1,
			wantMethod:       "GET",
		},
		{
			name: "Routine_Create",
			callFunc: func(ctx context.Context, client *Client) {
				_ = client.Dataset("test-dataset").Routine("test-routine").Create(ctx, &RoutineMetadata{})
			},
			mockResponse:     `{"routineReference": {"projectId": "test-project", "datasetId": "test-dataset", "routineId": "test-routine"}}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/datasets/test-dataset",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets/{datasetId}/routines",
			wantAttempts:     1,
			wantMethod:       "POST",
		},
		{
			name: "Routine_Metadata",
			callFunc: func(ctx context.Context, client *Client) {
				_, _ = client.Dataset("test-dataset").Routine("test-routine").Metadata(ctx)
			},
			mockResponse:     `{"routineReference": {"projectId": "test-project", "datasetId": "test-dataset", "routineId": "test-routine"}}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/datasets/test-dataset/routines/test-routine",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets/{datasetId}/routines/{routineId}",
			wantAttempts:     1,
			wantMethod:       "GET",
		},
		{
			name: "Routine_Update",
			callFunc: func(ctx context.Context, client *Client) {
				_, _ = client.Dataset("test-dataset").Routine("test-routine").Update(ctx, &RoutineMetadataToUpdate{}, "")
			},
			mockResponse:     `{"routineReference": {"projectId": "test-project", "datasetId": "test-dataset", "routineId": "test-routine"}}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/datasets/test-dataset/routines/test-routine",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets/{datasetId}/routines/{routineId}",
			wantAttempts:     1,
			wantMethod:       "PUT",
		},
		{
			name: "Routine_Delete",
			callFunc: func(ctx context.Context, client *Client) {
				_ = client.Dataset("test-dataset").Routine("test-routine").Delete(ctx)
			},
			mockResponse:     `{}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/datasets/test-dataset/routines/test-routine",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets/{datasetId}/routines/{routineId}",
			wantAttempts:     1,
			wantMethod:       "DELETE",
		},
		{
			name: "Dataset_Routines",
			callFunc: func(ctx context.Context, client *Client) {
				it := client.Dataset("test-dataset").Routines(ctx)
				_, _ = it.Next()
			},
			mockResponse:     `{"routines": [{"routineReference": {"projectId": "test-project", "datasetId": "test-dataset", "routineId": "test-routine"}}]}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/datasets/test-dataset",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets/{datasetId}/routines",
			wantAttempts:     1,
			wantMethod:       "GET",
		},
		{
			name: "Job_Cancel",
			callFunc: func(ctx context.Context, client *Client) {
				job := &Job{projectID: "test-project", jobID: "test-job", c: client}
				_ = job.Cancel(ctx)
			},
			mockResponse:     `{"jobReference": {"projectId": "test-project", "jobId": "test-job"}}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/jobs/test-job",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/jobs/{jobId}/cancel",
			wantAttempts:     1,
			wantMethod:       "POST",
		},
		{
			name: "Job_Delete",
			callFunc: func(ctx context.Context, client *Client) {
				job := &Job{projectID: "test-project", jobID: "test-job", c: client}
				_ = job.Delete(ctx)
			},
			mockResponse:     `{}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/jobs/test-job",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/jobs/{jobId}",
			wantAttempts:     1,
			wantMethod:       "DELETE",
		},
		{
			name: "Client_Query",
			callFunc: func(ctx context.Context, client *Client) {
				q := client.Query("SELECT 1")
				_, _ = q.Read(ctx)
			},
			mockResponse:     `{"jobReference": {"projectId": "test-project", "jobId": "test-job"}, "jobComplete": true}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/queries",
			wantAttempts:     1,
			wantMethod:       "POST",
		},
		{
			name: "Client_Jobs",
			callFunc: func(ctx context.Context, client *Client) {
				it := client.Jobs(ctx)
				_, _ = it.Next()
			},
			mockResponse:     `{"jobs": [{"jobReference": {"projectId": "test-project", "jobId": "test-job"}}]}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/jobs",
			wantAttempts:     1,
			wantMethod:       "GET",
		},
		{
			name: "Job_GetQueryResults",
			callFunc: func(ctx context.Context, client *Client) {
				job := &Job{projectID: "test-project", jobID: "test-job", c: client, config: &bq.JobConfiguration{Query: &bq.JobConfigurationQuery{}}}
				_, _ = job.Read(ctx)
			},
			mockResponse:     `{"jobReference": {"projectId": "test-project", "jobId": "test-job"}, "jobComplete": true}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/jobs/test-job",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/jobs/{jobId}/getQueryResults",
			wantAttempts:     1,
			wantMethod:       "GET",
		},
		{
			name: "Inserter_Put",
			callFunc: func(ctx context.Context, client *Client) {
				ins := client.Dataset("test-dataset").Table("test-table").Inserter()
				_ = ins.Put(ctx, []ValueSaver{
					&ValuesSaver{Schema: Schema{{Name: "name", Type: StringFieldType}}, Row: []Value{"test"}},
				})
			},
			mockResponse:     `{"insertErrors": []}`,
			mockStatusCodes:  []int{http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/datasets/test-dataset/tables/test-table",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets/{datasetId}/tables/{tableId}/insertAll",
			wantAttempts:     1,
			wantMethod:       "POST",
		}, {
			name: "Retry_Dataset_Metadata",
			callFunc: func(ctx context.Context, client *Client) {
				_, _ = client.Dataset("test-dataset").Metadata(ctx)
			},
			mockResponse:     `{"id": "test-dataset", "datasetReference": {"projectId": "test-project", "datasetId": "test-dataset"}}`,
			mockStatusCodes:  []int{http.StatusServiceUnavailable, http.StatusOK},
			wantResourceName: "//bigquery.googleapis.com/projects/test-project/datasets/test-dataset",
			wantURLTemplate:  "/bigquery/v2/projects/{projectId}/datasets/{datasetId}",
			wantAttempts:     2,
			wantMethod:       "GET",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := tracetest.NewInMemoryExporter()
			tp := sdktrace.NewTracerProvider(
				sdktrace.WithSyncer(exp),
			)
			otel.SetTracerProvider(tp)

			attempts := 0
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				status := http.StatusOK
				if attempts < len(tt.mockStatusCodes) {
					status = tt.mockStatusCodes[attempts]
				}
				attempts++
				w.WriteHeader(status)
				if status == http.StatusOK {
					w.Write([]byte(tt.mockResponse))
				}
			}))
			defer ts.Close()

			ctx := context.Background()
			client, err := NewClient(ctx, "test-project", option.WithEndpoint(ts.URL), option.WithoutAuthentication())
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}
			defer client.Close()

			tt.callFunc(ctx, client)

			spans := exp.GetSpans()
			if len(spans) == 0 {
				t.Fatalf("expected spans to be recorded, got 0")
			}

			if attempts != tt.wantAttempts {
				t.Errorf("expected %d attempts, got %d", tt.wantAttempts, attempts)
			}

			networkSpans := 0
			for _, span := range spans {
				// The otelAttributeTransport renames the span to "{METHOD} {url.template}" but might have a duplicated method name
				if strings.Contains(span.Name, tt.wantURLTemplate) {
					networkSpans++

					expectedAttributes := map[attribute.Key]string{
						"gcp.resource.destination.id": tt.wantResourceName,
						"url.template":                tt.wantURLTemplate,
						"gcp.client.artifact":         "cloud.google.com/go/bigquery",
						"gcp.client.language":         "go",
						"gcp.client.repo":             "googleapis/google-cloud-go",
						"gcp.client.service":          "bigquery.googleapis.com",
						"gcp.client.version":          internal.Version,
						"http.request.method":         tt.wantMethod,
						"http.response.status_code":   "",
						"network.protocol.version":    "1.1",
						"rpc.system.name":             "http",
						"url.domain":                  "bigquery.googleapis.com",
					}

					actualAttributes := make(map[attribute.Key]string, len(span.Attributes))
					for _, attr := range span.Attributes {
						actualAttributes[attr.Key] = attr.Value.AsString()
					}

					if tt.name == "Retry_Dataset_Metadata" && actualAttributes["error.type"] == "503" {
						expectedAttributes["error.type"] = "503"
						expectedAttributes["status.message"] = "503 Service Unavailable"
					}

					// Verify dynamic fields and then delete them so cmp.Diff doesn't fail
					if val, ok := actualAttributes["url.full"]; ok {
						if !strings.HasPrefix(val, ts.URL) {
							t.Errorf("url.full mismatch: got %v, want prefix %v", val, ts.URL)
						}
						delete(actualAttributes, "url.full")
					} else {
						t.Errorf("missing url.full attribute")
					}

					if val, ok := actualAttributes["server.address"]; ok {
						if !strings.Contains(ts.URL, val) {
							t.Errorf("server.address mismatch: got %v, want it in %v", val, ts.URL)
						}
						delete(actualAttributes, "server.address")
					} else {
						t.Errorf("missing server.address attribute")
					}

					if _, ok := actualAttributes["server.port"]; ok {
						delete(actualAttributes, "server.port")
					}

					if diff := cmp.Diff(expectedAttributes, actualAttributes); diff != "" {
						t.Errorf("attributes mismatch (-want +got):\n%s", diff)
					}
				}
			}

			if networkSpans != tt.wantAttempts {
				var names []string
				for _, s := range spans {
					names = append(names, s.Name)
				}
				t.Errorf("expected %d network spans, got %d. Found span names: %v", tt.wantAttempts, networkSpans, names)
			}
		})
	}
}
