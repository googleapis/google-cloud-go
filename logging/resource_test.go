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
	there               = "non-empty-value"
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
}

func (g *fakeResourceGetter) EnvVar(name string) (string, bool) {
	v, ok := g.envVars[name]
	return v, ok
}

func (g *fakeResourceGetter) Metadata(path string) (string, bool) {
	v, ok := g.metaVars[path]
	return v, ok
}

// setupDetectResource resets sync.Once on detectResource and enforces mocked resource attribute getter
func setupDetectedResource(envVars, metaVars map[string]string) {
	detectedResource.once = new(sync.Once)
	detectedResource.attrs = &fakeResourceGetter{
		envVars:  envVars,
		metaVars: metaVars,
	}
	detectedResource.pb = nil
}

func TestResourceDetection(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		metaVars map[string]string
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
			name:     "detect Cloud Run resource",
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
			name:     "detect GKE resource",
			envVars:  map[string]string{"HOSTNAME": podName, "CONTAINER_NAME": containerName, "NAMESPACE_NAME": namespaceName},
			metaVars: map[string]string{"": there, "project/project-id": projectID, "instance/zone": qualifiedZoneName, "instance/attributes/cluster-name": clusterName},
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupDetectedResource(tc.envVars, tc.metaVars)
			got := detectResource()
			if !cmp.Equal(got, tc.want, cmpopts.IgnoreUnexported(mrpb.MonitoredResource{})) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
