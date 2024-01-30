// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logging

import (
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
)

const (
	there               = "anyvalue"
	projectID           = "test-project"
	zoneID              = "test-region-zone"
	regionID            = "test-region"
	serviceName         = "test-service"
	version             = "1.0"
	instanceName        = "test-12345"
	qualifiedZoneName   = "projects/" + projectID + "/zones/" + zoneID
	qualifiedRegionName = "projects/" + projectID + "/regions/" + regionID
	funcSignature       = "test-cf-signature"
	funcTarget          = "test-cf-target"
	crConfig            = "test-cr-config"
	clusterName         = "test-k8s-cluster"
	podName             = "test-k8s-pod-name"
	containerName       = "test-k8s-container-name"
	namespaceName       = "test-k8s-namespace-name"
	instanceID          = "test-instance-12345"
)

// fakeResourceGetter mocks internal.ResourceAtttributesGetter interface to retrieve env vars and metadata
type fakeResourceGetter struct {
	envVars  map[string]string
	metaVars map[string]string
	fsPaths  map[string]string
}

func (g *fakeResourceGetter) EnvVar(name string) string {
	if g.envVars != nil {
		if v, ok := g.envVars[name]; ok {
			return v
		}
	}
	return ""
}

func (g *fakeResourceGetter) Metadata(path string) string {
	if g.metaVars != nil {
		if v, ok := g.metaVars[path]; ok {
			return v
		}
	}
	return ""
}

func (g *fakeResourceGetter) ReadAll(path string) string {
	if g.fsPaths != nil {
		if v, ok := g.fsPaths[path]; ok {
			return v
		}
	}
	return ""
}

// setupDetectResource resets sync.Once on detectResource and enforces mocked resource attribute getter
func setupDetectedResource(envVars, metaVars, fsPaths map[string]string) {
	detectedResource.once = new(sync.Once)
	detectedResource.attrs = &fakeResourceGetter{
		envVars:  envVars,
		metaVars: metaVars,
		fsPaths:  fsPaths,
	}
	detectedResource.pb = nil
}

func TestResourceDetection(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		metaVars map[string]string
		fsPaths  map[string]string
		want     *mrpb.MonitoredResource
	}{
		{
			name:     "detect GAE resource",
			envVars:  map[string]string{"GAE_SERVICE": serviceName, "GAE_VERSION": version, "GAE_INSTANCE": instanceName},
			metaVars: map[string]string{"": there, "project/project-id": projectID, "instance/zone": qualifiedZoneName, "instance/attributes/gae_app_bucket": there},
			want: &mrpb.MonitoredResource{
				Type: "gae_app",
				Labels: map[string]string{
					"project_id": projectID,
					"module_id":  serviceName,
					"version_id": version,
					"zone":       zoneID,
				},
			},
		},
		{
			name:     "detect Cloud Function resource",
			envVars:  map[string]string{"FUNCTION_TARGET": funcTarget, "FUNCTION_SIGNATURE_TYPE": funcSignature, "K_SERVICE": serviceName},
			metaVars: map[string]string{"": there, "project/project-id": projectID, "instance/region": qualifiedRegionName},
			want: &mrpb.MonitoredResource{
				Type: "cloud_function",
				Labels: map[string]string{
					"project_id":    projectID,
					"region":        regionID,
					"function_name": serviceName,
				},
			},
		},
		{
			name:     "detect Cloud Run service resource",
			envVars:  map[string]string{"K_CONFIGURATION": crConfig, "K_SERVICE": serviceName, "K_REVISION": version},
			metaVars: map[string]string{"": there, "project/project-id": projectID, "instance/region": qualifiedRegionName},
			want: &mrpb.MonitoredResource{
				Type: "cloud_run_revision",
				Labels: map[string]string{
					"project_id":         projectID,
					"location":           regionID,
					"service_name":       serviceName,
					"revision_name":      version,
					"configuration_name": crConfig,
				},
			},
		},
		{
			name:     "detect Cloud Run job resource",
			envVars:  map[string]string{"CLOUD_RUN_JOB": serviceName, "CLOUD_RUN_EXECUTION": crConfig, "CLOUD_RUN_TASK_INDEX": version, "CLOUD_RUN_TASK_ATTEMPT": instanceID},
			metaVars: map[string]string{"": there, "project/project-id": projectID, "instance/region": qualifiedRegionName},
			want: &mrpb.MonitoredResource{
				Type: "cloud_run_job",
				Labels: map[string]string{
					"project_id": projectID,
					"location":   regionID,
					"job_name":   serviceName,
				},
			},
		},
		{
			name:     "detect GKE resource for a zonal cluster",
			envVars:  map[string]string{"HOSTNAME": podName},
			metaVars: map[string]string{"": there, "project/project-id": projectID, "instance/attributes/cluster-location": zoneID, "instance/attributes/cluster-name": clusterName},
			fsPaths:  map[string]string{"/var/run/secrets/kubernetes.io/serviceaccount/namespace": namespaceName},
			want: &mrpb.MonitoredResource{
				Type: "k8s_container",
				Labels: map[string]string{
					"cluster_name":   clusterName,
					"location":       zoneID,
					"project_id":     projectID,
					"pod_name":       podName,
					"namespace_name": namespaceName,
					"container_name": "",
				},
			},
		},
		{
			name:     "detect GKE resource for a regional cluster",
			envVars:  map[string]string{"HOSTNAME": podName},
			metaVars: map[string]string{"": there, "project/project-id": projectID, "instance/attributes/cluster-location": regionID, "instance/attributes/cluster-name": clusterName},
			fsPaths:  map[string]string{"/var/run/secrets/kubernetes.io/serviceaccount/namespace": namespaceName},
			want: &mrpb.MonitoredResource{
				Type: "k8s_container",
				Labels: map[string]string{
					"cluster_name":   clusterName,
					"location":       regionID,
					"project_id":     projectID,
					"pod_name":       podName,
					"namespace_name": namespaceName,
					"container_name": "",
				},
			},
		},
		{
			name:     "detect GKE resource with custom container and namespace config",
			envVars:  map[string]string{"HOSTNAME": podName, "CONTAINER_NAME": containerName, "NAMESPACE_NAME": namespaceName},
			metaVars: map[string]string{"": there, "project/project-id": projectID, "instance/attributes/cluster-location": zoneID, "instance/attributes/cluster-name": clusterName},
			want: &mrpb.MonitoredResource{
				Type: "k8s_container",
				Labels: map[string]string{
					"cluster_name":   clusterName,
					"location":       zoneID,
					"project_id":     projectID,
					"pod_name":       podName,
					"namespace_name": namespaceName,
					"container_name": containerName,
				},
			},
		},
		{
			name:     "detect Compute Engine resource",
			envVars:  map[string]string{},
			metaVars: map[string]string{"": there, "project/project-id": projectID, "instance/id": instanceID, "instance/zone": qualifiedZoneName, "instance/preempted": there, "instance/cpu-platform": there},
			want: &mrpb.MonitoredResource{
				Type: "gce_instance",
				Labels: map[string]string{
					"project_id":  projectID,
					"instance_id": instanceID,
					"zone":        zoneID,
				},
			},
		},
		{
			name:     "detect GAE resource by product name",
			envVars:  map[string]string{},
			metaVars: map[string]string{"": there, "project/project-id": projectID, "instance/zone": qualifiedZoneName, "instance/attributes/gae_app_bucket": there},
			fsPaths:  map[string]string{"/sys/class/dmi/id/product_name": "Google App Engine"},
			want: &mrpb.MonitoredResource{
				Type: "gae_app",
				Labels: map[string]string{
					"project_id": projectID,
					"module_id":  "",
					"version_id": "",
					"zone":       zoneID,
				},
			},
		},
		{
			name:     "detect Cloud Function resource by product name",
			envVars:  map[string]string{},
			metaVars: map[string]string{"": there, "project/project-id": projectID, "instance/region": qualifiedRegionName},
			fsPaths:  map[string]string{"/sys/class/dmi/id/product_name": "Google Cloud Functions"},
			want: &mrpb.MonitoredResource{
				Type: "cloud_function",
				Labels: map[string]string{
					"project_id":    projectID,
					"region":        regionID,
					"function_name": "",
				},
			},
		},
		{
			name:     "unknown resource detection",
			envVars:  map[string]string{},
			metaVars: map[string]string{"": there, "project/project-id": projectID},
			want:     nil,
		},
		{
			name:     "resource without metadata detection",
			envVars:  map[string]string{},
			metaVars: map[string]string{},
			want:     nil,
		},
	}

	// cleanup
	oldAttrs := detectedResource.attrs
	defer func() {
		detectedResource.attrs = oldAttrs
		detectedResource.once = new(sync.Once)
	}()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupDetectedResource(tc.envVars, tc.metaVars, tc.fsPaths)
			got := detectResource()
			if diff := cmp.Diff(got, tc.want, cmpopts.IgnoreUnexported(mrpb.MonitoredResource{})); diff != "" {
				t.Errorf("got(-),want(+):\n%s", diff)
			}
		})
	}
}

var benchmarkResultHolder *mrpb.MonitoredResource

func BenchmarkDetectResource(b *testing.B) {
	var result *mrpb.MonitoredResource

	for n := 0; n < b.N; n++ {
		result = detectResource()
	}

	benchmarkResultHolder = result
}
