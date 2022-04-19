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
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"sync"

	"cloud.google.com/go/logging/metadata"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
)

// CommonResource sets the monitored resource associated with all log entries
// written from a Logger. If not provided, the resource is automatically
// detected based on the running environment (on GCE, GCR, GCF and GAE Standard only).
// This value can be overridden per-entry by setting an Entry's Resource field.
func CommonResource(r *mrpb.MonitoredResource) LoggerOption { return commonResource{r} }

type commonResource struct{ *mrpb.MonitoredResource }

func (r commonResource) set(l *Logger) { l.commonResource = r.MonitoredResource }

var detectedResource struct {
	pb   *mrpb.MonitoredResource
	once sync.Once
}

func detectResource() *mrpb.MonitoredResource {
	detectedResource.once.Do(func() {
		if metadata.IsMetadataActive() {
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
				detectedResource.pb = detectGCEResource()
			}
		}
	})
	return detectedResource.pb
}

// isAppEngine returns true for both standard and flex
func isAppEngine() bool {
	_, service := os.LookupEnv("GAE_SERVICE")
	_, version := os.LookupEnv("GAE_VERSION")
	_, instance := os.LookupEnv("GAE_INSTANCE")

	return service && version && instance
}

func detectAppEngineResource() *mrpb.MonitoredResource {
	projectID, err := metadata.ProjectID()
	if err != nil {
		return nil
	}
	zone, _ := metadata.InstanceZone()
	return &mrpb.MonitoredResource{
		Type: "gae_app",
		Labels: map[string]string{
			"project_id":  projectID,
			"module_id":   os.Getenv("GAE_SERVICE"),
			"version_id":  os.Getenv("GAE_VERSION"),
			"zone":        zone,
		},
	}
}

func isCloudFunction() bool {
	// Reserved envvars in Gen1 and Gen2 function runtimes.
	_, target := os.LookupEnv("FUNCTION_TARGET")
	_, signature := os.LookupEnv("FUNCTION_SIGNATURE_TYPE")
	// this envvar is present in Cloud Run too
	_, service := os.LookupEnv("K_SERVICE")
	return target && signature && service
}

func detectCloudFunction() *mrpb.MonitoredResource {
	projectID, err := metadata.ProjectID()
	if err != nil {
		return nil
	}
	region, _ := metadata.InstanceRegion()
	// Newer functions runtimes store name in K_SERVICE.
	functionName, exists := os.LookupEnv("K_SERVICE")
	if !exists {
		functionName, _ = os.LookupEnv("FUNCTION_NAME")
	}
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
	_, config := os.LookupEnv("K_CONFIGURATION")
	// this envvar is present in Cloud Functions too
	_, service := os.LookupEnv("K_SERVICE")
	_, revision := os.LookupEnv("K_REVISION")
	return config && service && revision
}

func detectCloudRunResource() *mrpb.MonitoredResource {
	projectID, err := metadata.ProjectID()
	if err != nil {
		return nil
	}
	region, _ := metadata.InstanceRegion()
	return &mrpb.MonitoredResource{
		Type: "cloud_run_revision",
		Labels: map[string]string{
			"project_id":         projectID,
			"location":           region,
			"service_name":       os.Getenv("K_SERVICE"),
			"revision_name":      os.Getenv("K_REVISION"),
			"configuration_name": os.Getenv("K_CONFIGURATION"),
		},
	}
}

func isKubernetesEngine() bool {
	clusterName, err := metadata.InstanceAttributeValue("cluster-name")
	// Note: InstanceAttributeValue can return "", nil
	if err != nil || clusterName == "" {
		return false
	}
	return true
}

func detectKubernetesResource() *mrpb.MonitoredResource {
	var namespaceName string
	
	projectID, err := metadata.ProjectID()
	if err != nil {
		return nil
	}
	zone, _ := metadata.InstanceZone()
	clusterName, _ := metadata.InstanceAttributeValue("cluster-name")
	namespaceBytes, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err == nil {
		namespaceName = string(namespaceBytes)
	} else {
		// TODO: provide deterministic way to identify a pod namespace
		// Use NAMESPACE_NAME environment variable if pod's
		// /spec/automountServiceAccountToken was set to false
		namespaceName = os.Getenv("NAMESPACE_NAME")
	}
	// TODO: provide deterministic way to identify pod name
	// Use POD_NAME environment variable if pod's /spec/hostname
	// was customized at deployment
    podName := os.Getenv("POD_NAME")
    if podName == "" {
		podName = os.Getenv("HOSTNAME");
	}
	return &mrpb.MonitoredResource{
		Type: "k8s_container",
		Labels: map[string]string{
			"cluster_name":   clusterName,
			"location":       zone,
			"project_id":     projectID,
			"pod_name":       podName,
			"namespace_name": namespaceName,
			// TODO: provide deterministic way to identify pod name
			// Use CONTAINER_NAME environment variable to provide
			// container name
			"container_name": os.Getenv("CONTAINER_NAME"),
		},
	}
}

func isComputeEngine() bool {
	preempted, _ := metadata.InstancePreempted()
	platform, _ := metadata.InstanceCPUPlatform()
	gae_app_bucket, _ := metadata.InstanceAttributeValue("gae_app_bucket")

	return preempted != "" && platform != "" && gae_app_bucket == ""
}

func detectGCEResource() *mrpb.MonitoredResource {
	projectID, err := metadata.ProjectID()
	if err != nil {
		return nil
	}
	id, _ := metadata.InstanceID()
	zone, _ := metadata.InstanceZone()
	return &mrpb.MonitoredResource{
		Type: "gce_instance",
		Labels: map[string]string{
			"project_id":  projectID,
			"instance_id": id,
			"zone":        zone,
		},
	}
}

func systemProductName() (string, bool) {
	if runtime.GOOS != "linux" {
		// We don't have any non-Linux clues available, at least yet.
		return "", false
	}
	slurp, err := ioutil.ReadFile("/sys/class/dmi/id/product_name")
	return strings.TrimSpace(string(slurp)), err == nil
}

func monitoredResource(parent string) *mrpb.MonitoredResource {
	parts := strings.SplitN(parent, "/", 2)
	switch len(parts) {
	case 1: return globalResource(parent)
	case 2: return globalResource(parts[1])
	// this behavior is unexpected and should not happened
	// since we cannot panic returning undefined resource
	default: return &mrpb.MonitoredResource{Type: "global"}
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
