// Copyright 2021 Google LLC
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

package logging

import (
	"runtime"
	"strings"
	"sync"

	"cloud.google.com/go/logging/internal"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
)

// CommonResource sets the monitored resource associated with all log entries
// written from a Logger. If not provided, the resource is automatically
// detected based on the running environment (on GCE, GCR, GCF and GAE Standard only).
// This value can be overridden per-entry by setting an Entry's Resource field.
func CommonResource(r *mrpb.MonitoredResource) LoggerOption { return commonResource{r} }

type commonResource struct{ *mrpb.MonitoredResource }

func (r commonResource) set(l *Logger) { l.commonResource = r.MonitoredResource }

var detectedResource = struct {
	pb    *mrpb.MonitoredResource
	attrs internal.ResourceAtttributesGetter
	once  *sync.Once
}{
	attrs: internal.ResourceAttributes(),
	once:  new(sync.Once),
}

func metadataProjectID() (string, bool) {
	return detectedResource.attrs.Metadata("project/project-id")
}

func metadataZone() (string, bool) {
	zone, ok := detectedResource.attrs.Metadata("instance/zone")
	if ok {
		return zone[strings.LastIndex(zone, "/")+1:], true
	}
	return "", false
}

func metadataRegion() (string, bool) {
	region, ok := detectedResource.attrs.Metadata("instance/region")
	if ok {
		return region[strings.LastIndex(region, "/")+1:], true
	}
	return "", false
}

// isAppEngine returns true for both standard and flex
func isAppEngine() bool {
	_, serviceOK := detectedResource.attrs.EnvVar("GAE_SERVICE")
	_, versionOK := detectedResource.attrs.EnvVar("GAE_VERSION")
	_, instanceOK := detectedResource.attrs.EnvVar("GAE_INSTANCE")
	return serviceOK && versionOK && instanceOK
}

func detectAppEngineResource() *mrpb.MonitoredResource {
	projectID, ok := metadataProjectID()
	if !ok {
		return nil
	}
	if projectID == "" {
		projectID, _ = detectedResource.attrs.EnvVar("GOOGLE_CLOUD_PROJECT")
	}
	zone, _ := metadataZone()
	service, _ := detectedResource.attrs.EnvVar("GAE_SERVICE")
	version, _ := detectedResource.attrs.EnvVar("GAE_VERSION")

	return &mrpb.MonitoredResource{
		Type: "gae_app",
		Labels: map[string]string{
			"project_id": projectID,
			"module_id":  service,
			"version_id": version,
			"zone":       zone,
		},
	}
}

func isCloudFunction() bool {
	_, targetOK := detectedResource.attrs.EnvVar("FUNCTION_TARGET")
	_, signatureOK := detectedResource.attrs.EnvVar("FUNCTION_SIGNATURE_TYPE")
	// note that this envvar is also present in Cloud Run environments
	_, serviceOK := detectedResource.attrs.EnvVar("K_SERVICE")
	return targetOK && signatureOK && serviceOK
}

func detectCloudFunction() *mrpb.MonitoredResource {
	projectID, ok := metadataProjectID()
	if !ok {
		return nil
	}
	region, _ := metadataRegion()
	functionName, _ := detectedResource.attrs.EnvVar("K_SERVICE")
	return &mrpb.MonitoredResource{
		Type: "cloud_function",
		Labels: map[string]string{
			"project_id":    projectID,
			"region":        region,
			"function_name": functionName,
		},
	}
}

func isCloudRun() bool {
	_, configOK := detectedResource.attrs.EnvVar("K_CONFIGURATION")
	// note that this envvar is also present in Cloud Function environments
	_, serviceOK := detectedResource.attrs.EnvVar("K_SERVICE")
	_, revisionOK := detectedResource.attrs.EnvVar("K_REVISION")
	return configOK && serviceOK && revisionOK
}

func detectCloudRunResource() *mrpb.MonitoredResource {
	projectID, ok := metadataProjectID()
	if !ok {
		return nil
	}
	region, _ := metadataRegion()
	config, _ := detectedResource.attrs.EnvVar("K_CONFIGURATION")
	service, _ := detectedResource.attrs.EnvVar("K_SERVICE")
	revision, _ := detectedResource.attrs.EnvVar("K_REVISION")
	return &mrpb.MonitoredResource{
		Type: "cloud_run_revision",
		Labels: map[string]string{
			"project_id":         projectID,
			"location":           region,
			"service_name":       service,
			"revision_name":      revision,
			"configuration_name": config,
		},
	}
}

func isKubernetesEngine() bool {
	clusterName, ok := detectedResource.attrs.Metadata("instance/attributes/cluster-name")
	if !ok || clusterName == "" {
		return false
	}
	return true
}

func detectKubernetesResource() *mrpb.MonitoredResource {
	projectID, ok := metadataProjectID()
	if !ok {
		return nil
	}
	zone, _ := metadataZone()
	clusterName, _ := detectedResource.attrs.Metadata("instance/attributes/cluster-name")
	namespaceName, err := detectedResource.attrs.ReadAll("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		// if automountServiceAccountToken is disabled allow to customize
		// the namespace via environment
		namespaceName, _ = detectedResource.attrs.EnvVar("NAMESPACE_NAME")
	}
	// note: if deployment customizes hostname, HOSTNAME envvar will have invalid content
	podName, _ := detectedResource.attrs.EnvVar("HOSTNAME")
	// there is no way to derive container name from within container; use custom envvar if available
	containerName, _ := detectedResource.attrs.EnvVar("CONTAINER_NAME")
	return &mrpb.MonitoredResource{
		Type: "k8s_container",
		Labels: map[string]string{
			"cluster_name":   clusterName,
			"location":       zone,
			"project_id":     projectID,
			"pod_name":       podName,
			"namespace_name": namespaceName,
			"container_name": containerName,
		},
	}
}

func isComputeEngine() bool {
	_, preemptedOK := detectedResource.attrs.Metadata("instance/preempted")
	_, platformOK := detectedResource.attrs.Metadata("instance/cpu-platform")
	_, appBucketOK := detectedResource.attrs.Metadata("instance/attributes/gae_app_bucket")
	return preemptedOK && platformOK && !appBucketOK
}

func detectComputeEngineResource() *mrpb.MonitoredResource {
	projectID, ok := metadataProjectID()
	if !ok {
		return nil
	}
	id, _ := detectedResource.attrs.Metadata("instance/id")
	zone, _ := metadataZone()
	return &mrpb.MonitoredResource{
		Type: "gce_instance",
		Labels: map[string]string{
			"project_id":  projectID,
			"instance_id": id,
			"zone":        zone,
		},
	}
}

func detectResource() *mrpb.MonitoredResource {
	detectedResource.once.Do(func() {
		if isMetadataActive() {
			name, _ := systemProductName()
			switch {
			case name == "Google App Engine", isAppEngine():
				detectedResource.pb = detectAppEngineResource()
			case name == "Google Cloud Functions", isCloudFunction():
				detectedResource.pb = detectCloudFunction()
			case name == "Google Cloud Run", isCloudRun():
				detectedResource.pb = detectCloudRunResource()
			// cannot use name validation for GKE and GCE because
			// both of them set product name to "Google Compute Engine"
			case isKubernetesEngine():
				detectedResource.pb = detectKubernetesResource()
			case isComputeEngine():
				detectedResource.pb = detectComputeEngineResource()
			}
		}
	})
	return detectedResource.pb
}

// isMetadataActive queries valid response on "/computeMetadata/v1/" URL
func isMetadataActive() bool {
	_, ok := detectedResource.attrs.Metadata("")
	return ok
}

// systemProductName reads resource type on the Linux-based environments such as
// Cloud Functions, Cloud Run, GKE, GCE, GAE, etc.
func systemProductName() (string, bool) {
	if runtime.GOOS != "linux" {
		// We don't have any non-Linux clues available, at least yet.
		return "", false
	}
	slurp, err := detectedResource.attrs.ReadAll("/sys/class/dmi/id/product_name")
	return strings.TrimSpace(slurp), err == nil
}

var resourceInfo = map[string]struct{ rtype, label string }{
	"organizations":   {"organization", "organization_id"},
	"folders":         {"folder", "folder_id"},
	"projects":        {"project", "project_id"},
	"billingAccounts": {"billing_account", "account_id"},
}

func monitoredResource(parent string) *mrpb.MonitoredResource {
	parts := strings.SplitN(parent, "/", 2)
	if len(parts) != 2 {
		return globalResource(parent)
	}
	info, ok := resourceInfo[parts[0]]
	if !ok {
		return globalResource(parts[1])
	}
	return &mrpb.MonitoredResource{
		Type:   info.rtype,
		Labels: map[string]string{info.label: parts[1]},
	}
}

func globalResource(projectID string) *mrpb.MonitoredResource {
	return &mrpb.MonitoredResource{
		Type: "global",
		Labels: map[string]string{
			"project_id": projectID,
		},
	}
}
