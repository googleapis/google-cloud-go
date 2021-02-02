// Copyright 2018 Google LLC
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
//
// Package proftest contains test helpers for profiler agent integration tests.
// This package is experimental.

package proftest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"cloud.google.com/go/storage"
	gax "github.com/googleapis/gax-go/v2"
	compute "google.golang.org/api/compute/v1"
	container "google.golang.org/api/container/v1"
	"google.golang.org/api/googleapi"
)

const (
	monitorWriteScope = "https://www.googleapis.com/auth/monitoring.write"
)

var (
	logtimeRE = regexp.MustCompile(`[A-Z][a-z]{2} [A-Z][a-z]{2}  ?\d+ \d\d:\d\d:\d\d [A-Z]{3} \d{4}`)

	// "ms" must be specified before "m" in the regexp, to ensure "ms" is fully
	// matched.
	backoffTimeRE = regexp.MustCompile(`(\d+(\.\d+)?(ms|h|m|s|us))+`)
)

// BaseStartupTmpl is the common part of the startup script that
// can be shared by multiple tests.
var BaseStartupTmpl = template.Must(template.New("startupScript").Parse(`
{{ define "prologue" -}}
#! /bin/bash
(
# Signal any unexpected error.
trap 'echo "{{.ErrorString}}"' ERR

# Shut down the VM in 5 minutes after this script exits
# to stop accounting the VM for billing and cores quota.
trap "sleep 300 && poweroff" EXIT

retry() {
  for i in {1..3}; do
    [ $i == 1 ] || sleep 10  # Backing off after a failed attempt.
    "${@}" && return 0
  done
  return 1
}

# Fail on any error.
set -eo pipefail

# Display commands being run
set -x
{{- end }}

{{ define "epilogue" -}}
# Write output to serial port 2 with timestamp.
) 2>&1 | while read line; do echo "$(date): ${line}"; done >/dev/ttyS1
{{- end }}
`))

// TestRunner has common elements used for testing profiling agents on a range
// of environments.
type TestRunner struct {
	Client *http.Client
}

// GCETestRunner supports testing a profiling agent on GCE.
type GCETestRunner struct {
	TestRunner
	ComputeService *compute.Service
}

// GKETestRunner supports testing a profiling agent on GKE.
type GKETestRunner struct {
	TestRunner
	ContainerService *container.Service
	StorageClient    *storage.Client
	Dockerfile       string
}

// ProfileResponse contains the response produced when querying profile server.
type ProfileResponse struct {
	Profile     ProfileData   `json:"profile"`
	NumProfiles int32         `json:"numProfiles"`
	Deployments []interface{} `json:"deployments"`
}

// ProfileData has data of a single profile.
type ProfileData struct {
	Samples           []int32         `json:"samples"`
	SampleMetrics     interface{}     `json:"sampleMetrics"`
	DefaultMetricType string          `json:"defaultMetricType"`
	TreeNodes         interface{}     `json:"treeNodes"`
	Functions         functionArray   `json:"functions"`
	SourceFiles       sourceFileArray `json:"sourceFiles"`
}

type functionArray struct {
	Name       []string `json:"name"`
	Sourcefile []int32  `json:"sourceFile"`
}

type sourceFileArray struct {
	Name []string `json:"name"`
}

// InstanceConfig is configuration for starting single GCE instance for
// profiling agent test case.
type InstanceConfig struct {
	ProjectID     string
	Zone          string
	Name          string
	StartupScript string
	MachineType   string
	ImageProject  string
	ImageFamily   string
	Scopes        []string
}

// ClusterConfig is configuration for starting single GKE cluster for profiling
// agent test case.
type ClusterConfig struct {
	ProjectID       string
	Zone            string
	ClusterName     string
	PodName         string
	ImageSourceName string
	ImageName       string
	Bucket          string
	Dockerfile      string
}

// queryProfileRequest is the request format for querying the profile server.
type queryProfileRequest struct {
	StartTime        string            `json:"startTime"`
	EndTime          string            `json:"endTime"`
	ProfileType      string            `json:"profileType"`
	Target           string            `json:"target"`
	DeploymentLabels map[string]string `json:"deploymentLabels,omitempty"`
}

// CheckNonEmpty returns nil if the profile has a profiles and deployments
// associated. Otherwise, returns a desciptive error.
func (pr *ProfileResponse) CheckNonEmpty() error {
	if pr.NumProfiles == 0 {
		return fmt.Errorf("profile response contains zero profiles: %v", pr)
	}
	if len(pr.Deployments) == 0 {
		return fmt.Errorf("profile response contains zero deployments: %v", pr)
	}
	return nil
}

// HasFunction returns nil if the function is present, or, if the function is
// not present, and error providing more details why the function is not
// present.
func (pr *ProfileResponse) HasFunction(functionName string) error {
	if err := pr.CheckNonEmpty(); err != nil {
		return fmt.Errorf("failed to find function name %s in profile: %v", functionName, err)
	}
	for _, name := range pr.Profile.Functions.Name {
		if strings.Contains(name, functionName) {
			return nil
		}
	}
	return fmt.Errorf("failed to find function name %s in profile", functionName)
}

// HasFunctionInFile returns nil if function is present in the specified file, and an
// error if the function/file combination is not present in the profile.
func (pr *ProfileResponse) HasFunctionInFile(functionName string, filename string) error {
	if err := pr.CheckNonEmpty(); err != nil {
		return fmt.Errorf("failed to find function name %s in file %s in profile: %v", functionName, filename, err)
	}
	for i, name := range pr.Profile.Functions.Name {
		file := pr.Profile.SourceFiles.Name[pr.Profile.Functions.Sourcefile[i]]
		if strings.Contains(name, functionName) && strings.HasSuffix(file, filename) {
			return nil
		}
	}
	return fmt.Errorf("failed to find function name %s in file %s in profile", functionName, filename)
}

// HasSourceFile returns nil if the file (or file where the end of the file path
// matches the filename) is present in the profile. Or, if the filename is not
// present, an error is returned.
func (pr *ProfileResponse) HasSourceFile(filename string) error {
	if err := pr.CheckNonEmpty(); err != nil {
		return fmt.Errorf("failed to find filename %s in profile: %v", filename, err)
	}
	for _, name := range pr.Profile.SourceFiles.Name {
		if strings.HasSuffix(name, filename) {
			return nil
		}
	}
	return fmt.Errorf("failed to find filename %s in profile", filename)
}

// StartInstance starts a GCE Instance with configs specified by inst,
// and which runs the startup script specified in inst. If image project
// is not specified, it defaults to "debian-cloud". If image family is
// not specified, it defaults to "debian-9".
func (tr *GCETestRunner) StartInstance(ctx context.Context, inst *InstanceConfig) error {
	imageProject, imageFamily := inst.ImageProject, inst.ImageFamily
	if imageProject == "" {
		imageProject = "debian-cloud"
	}
	if imageFamily == "" {
		imageFamily = "debian-9"
	}
	img, err := tr.ComputeService.Images.GetFromFamily(imageProject, imageFamily).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to get image from family %q in project %q: %v", imageFamily, imageProject, err)
	}

	op, err := tr.ComputeService.Instances.Insert(inst.ProjectID, inst.Zone, &compute.Instance{
		MachineType: fmt.Sprintf("zones/%s/machineTypes/%s", inst.Zone, inst.MachineType),
		Name:        inst.Name,
		Disks: []*compute.AttachedDisk{{
			AutoDelete: true, // delete the disk when the VM is deleted.
			Boot:       true,
			Type:       "PERSISTENT",
			Mode:       "READ_WRITE",
			InitializeParams: &compute.AttachedDiskInitializeParams{
				SourceImage: img.SelfLink,
				DiskType:    fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/diskTypes/pd-standard", inst.ProjectID, inst.Zone),
			},
		}},
		NetworkInterfaces: []*compute.NetworkInterface{{
			Network: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/global/networks/default", inst.ProjectID),
			AccessConfigs: []*compute.AccessConfig{{
				Name: "External NAT",
			}},
		}},
		Metadata: &compute.Metadata{
			Items: []*compute.MetadataItems{{
				Key:   "startup-script",
				Value: googleapi.String(inst.StartupScript),
			}},
		},
		ServiceAccounts: []*compute.ServiceAccount{{
			Email:  "default",
			Scopes: append(inst.Scopes, monitorWriteScope),
		}},
	}).Do()

	if err != nil {
		return fmt.Errorf("failed to create instance: %v", err)
	}

	// Poll status of the operation to create the instance.
	getOpCall := tr.ComputeService.ZoneOperations.Get(inst.ProjectID, inst.Zone, op.Name)
	for {
		if err := checkOpErrors(op); err != nil {
			return fmt.Errorf("failed to create instance: %v", err)
		}
		if op.Status == "DONE" {
			return nil
		}

		if err := gax.Sleep(ctx, 5*time.Second); err != nil {
			return err
		}

		op, err = getOpCall.Do()
		if err != nil {
			return fmt.Errorf("failed to get operation: %v", err)
		}
	}
}

// checkOpErrors returns nil if the operation does not have any errors and an
// error summarizing all errors encountered if the operation has errored.
func checkOpErrors(op *compute.Operation) error {
	if op.Error == nil || len(op.Error.Errors) == 0 {
		return nil
	}

	var errs []string
	for _, e := range op.Error.Errors {
		if e.Message != "" {
			errs = append(errs, e.Message)
		} else {
			errs = append(errs, e.Code)
		}
	}
	return errors.New(strings.Join(errs, ","))
}

// DeleteInstance deletes an instance with project id, name, and zone matched
// by inst.
func (tr *GCETestRunner) DeleteInstance(ctx context.Context, inst *InstanceConfig) error {
	if _, err := tr.ComputeService.Instances.Delete(inst.ProjectID, inst.Zone, inst.Name).Context(ctx).Do(); err != nil {
		return fmt.Errorf("Instances.Delete(%s) got error: %v", inst.Name, err)
	}
	return nil
}

// CheckSerialOutputForBackoffs parses serial port output, and validates that
// server-specified backoffs were respected by the agent.
func CheckSerialOutputForBackoffs(output string, numBenchmarks int, serverBackoffSubstring, createProfileSubstring, benchmarkNumPrefix string) error {
	benchmarkNumRE, err := regexp.Compile(fmt.Sprintf("%s (\\d+):", benchmarkNumPrefix))
	if err != nil {
		return fmt.Errorf("could not compile regexp to determine benchmark number: %v", err)
	}

	// Each CreateProfile request after a backoff should occur within
	// createTimeBuffer of the time at which the request is expected.
	createTimeBuffer := time.Minute
	nextCreateTime := make([]time.Time, numBenchmarks)
	benchmarkLogFounds := make([]bool, numBenchmarks)

	var serverBackoff, createAfterBackoff bool
	var logTime time.Time
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}

		// Find the benchmark number associated with line. Ignore cases when
		// the benchmark number cannot be determined; the log line might just
		// not be associated with a benchmark.
		benchNum, err := parseBenchmarkNumber(line, numBenchmarks, benchmarkNumRE)
		if err != nil {
			continue
		}
		benchmarkLogFounds[benchNum] = true

		// Find the time at which log statement was logged.
		logTime, err = parseLogTime(line)
		if err != nil {
			return fmt.Errorf("failed to parse timestamp for log statement: %v", err)
		}

		switch {
		// For a log indicating that the agent has received a server-specified
		// backoff, record when the next CreateProfile request should happen
		// if this backoff is respected.
		case strings.Contains(line, serverBackoffSubstring):
			serverBackoff = true
			backoffDur, err := parseBackoffDuration(line)
			if err != nil {
				return fmt.Errorf("failed to parse backoff duration for line %q: %v", line, err)
			}
			nextCreateTime[benchNum] = logTime.Add(backoffDur)

		// For a log indicating a CreateProfile request has been sent, confirm
		// that if the agent has previously received a server-specified backoff,
		// then this CreateProfile request is sent when the agent should sent
		// its next CreateProfile request.
		case strings.Contains(line, createProfileSubstring):
			if time.Time.IsZero(nextCreateTime[benchNum]) {
				continue
			}
			if logTime.Before(nextCreateTime[benchNum].Add(-1*createTimeBuffer)) || logTime.After(nextCreateTime[benchNum].Add(createTimeBuffer)) {
				return fmt.Errorf("got next CreateProfile request for benchmark %d at %v, want next CreateProfile request within %v of %v", benchNum, logTime, createTimeBuffer, nextCreateTime[benchNum])
			}
			nextCreateTime[benchNum] = time.Time{}
			createAfterBackoff = true
		}
	}

	// Return an error if there is not at least one server-specified backoff
	// in the logs. We can't validated that server-specified backoffs are
	// respected if no server-specified backoffs appearred.
	if !serverBackoff {
		return errors.New("no server-specified backoff encountered")
	}

	// Return an error if there is not at least one CreateProfile request
	// from an agent which had previously received a server-specified
	// backoff. We cannot validate that agents send another CreateProfile
	// request after the backoff duration has elapsed unless some agent
	// has sent a CreateProfile request after having received a backoff.
	if !createAfterBackoff {
		return errors.New("did not encounter a CreateProfile request which occurred after a server-specified backoff")
	}

	// Confirm that no agent which was backed off at the time at which
	// benchmark applications exited should have sent a CreateProfile request
	// prior to the time at which benchmark applications exited.
	for i, nct := range nextCreateTime {
		// logTime will contain the timestamp for the last benchmark log
		// statment, so is a proxy for when benchmarks have exitted.
		// After benchmarks exit, any expected CreateProfile requests should
		// either have been made (and so not be in nextCreateTime) or be
		// expected to occur after the benchmark exits. Since not all
		// benchmarks will exit at the same time and CreateProfile requests may
		// not occur exactly when expected, fail only if CreateProfile requests
		// occur more than 1 minute + createTimeBuffer before logTime.
		if !time.Time.IsZero(nct) && nct.Add(createTimeBuffer+time.Minute).Before(logTime) {
			return fmt.Errorf("got no CreateProfile request for benchmark %d before %v, want CreateProfileRequest at %v", i, logTime, nct)
		}
	}
	return nil
}

// parseBenchmarkNumber returns the benchmark number associated with a line or
// an error if the line is missing or has an invalid benchmark number.
// Each instance of a benchmark run as part of a backoff integration test is
// associated with a number to allow logs for each instance of each benchmark
// to be considered independently.
func parseBenchmarkNumber(line string, numBenchmarks int, benchmarkNumRE *regexp.Regexp) (int, error) {
	m := benchmarkNumRE.FindStringSubmatch(line)
	if m == nil || len(m) < 2 {
		return 0, fmt.Errorf("line %q does not have benchmark number", line)
	}
	benchNum, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("line %q has invalid benchmark number %q: %v", line, benchNum, err)
	}
	if benchNum < 0 || benchNum >= numBenchmarks {
		return 0, fmt.Errorf("line %q had invalid benchmark number %d: benchmark number must be between 0 and %d", line, benchNum, numBenchmarks-1)
	}
	return benchNum, nil
}

// parseLogTime returns the timestamp associated with a logged line, or an
// error if the log's timestamp cannot be determined.
func parseLogTime(line string) (time.Time, error) {
	logTimeStr := logtimeRE.FindString(line)
	if logTimeStr == "" {
		return time.Time{}, fmt.Errorf("logged line %q does not include a timestamp", line)
	}
	return time.Parse("Mon Jan 2 15:04:05 UTC 2006", logTimeStr)
}

// parseBackoffDuration returns the backoff duration associated with a logged
// line, or an error if the line does not contain a valid backoff duration.
func parseBackoffDuration(line string) (time.Duration, error) {
	backoffTimeStr := backoffTimeRE.FindString(line)
	if backoffTimeStr == "" {
		return 0, fmt.Errorf("log for server-specified backoff %q does not include a backoff time", line)
	}
	backoff, err := time.ParseDuration(backoffTimeStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse backoff duration %q", backoffTimeStr)
	}
	return backoff, nil
}

// PollAndLogSerialPort polls serial port 2 of the given GCE instance and
// returns when the finishString appears in the serial output of the instance,
// or when the context times out. It logs the serial output of the instance
// using the specified log function and returns the serial output in the first
// return value.
func (tr *GCETestRunner) PollAndLogSerialPort(ctx context.Context, inst *InstanceConfig, finishString, errorString string, logf func(string, ...interface{})) (string, error) {
	var output string
	defer func() {
		// Avoid escaping and double newlines in the rendered output (b/175999077).
		// TODO: Use strings.ReplaceAll once support for Go 1.11 is dropped.
		output = strings.Replace(output, "\r\n", "\n", -1)
		output = strings.Replace(output, "\033", "\\033", -1)
		logf("Serial port output for %s:\n%s", inst.Name, output)
	}()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(20 * time.Second):
			resp, err := tr.ComputeService.Instances.GetSerialPortOutput(inst.ProjectID, inst.Zone, inst.Name).Port(2).Context(ctx).Do()
			if err != nil {
				// Transient failure.
				logf("Transient error getting serial port output from instance %s (will retry): %v", inst.Name, err)
				continue
			}
			if resp.Contents == "" {
				logf("Ignoring empty serial port output from instance %s (will retry)", inst.Name)
				continue
			}
			if output = resp.Contents; strings.Contains(output, finishString) {
				return output, nil
			}
			if strings.Contains(output, errorString) {
				return output, fmt.Errorf("failed to execute the prober benchmark script")
			}
		}
	}
}

// QueryProfiles retrieves profiles of a specific type, from a specific time
// range, associated with a particular service and project.
func (tr *TestRunner) QueryProfiles(projectID, service, startTime, endTime, profileType string) (ProfileResponse, error) {
	return tr.QueryProfilesWithZone(projectID, service, startTime, endTime, profileType, "")
}

// QueryProfilesWithZone retrieves profiles of a specific type, from a specific
// time range, in a specified zone, associated with a particular service
// and project.
func (tr *TestRunner) QueryProfilesWithZone(projectID, service, startTime, endTime, profileType, zone string) (ProfileResponse, error) {
	queryURL := fmt.Sprintf("https://cloudprofiler.googleapis.com/v2/projects/%s/profiles:query", projectID)
	deploymentLabels := map[string]string{}
	if zone != "" {
		deploymentLabels["zone"] = zone
	}

	qpr := queryProfileRequest{
		StartTime:        startTime,
		EndTime:          endTime,
		ProfileType:      profileType,
		Target:           service,
		DeploymentLabels: deploymentLabels,
	}

	queryJSON, err := json.Marshal(qpr)
	if err != nil {
		return ProfileResponse{}, fmt.Errorf("failed to marshall request to JSON: %v", err)
	}

	req, err := http.NewRequest("POST", queryURL, bytes.NewReader(queryJSON))
	if err != nil {
		return ProfileResponse{}, fmt.Errorf("failed to create an API request: %v", err)
	}
	req.Header = map[string][]string{
		"X-Goog-User-Project": {projectID},
	}

	resp, err := tr.Client.Do(req)
	if err != nil {
		return ProfileResponse{}, fmt.Errorf("failed to query API: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return ProfileResponse{}, fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode != 200 {
		return ProfileResponse{}, fmt.Errorf("failed to query API: status: %s, response body: %s", resp.Status, string(body))
	}

	var pr ProfileResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		return ProfileResponse{}, err
	}

	return pr, nil
}

type imageResponse struct {
	Manifest map[string]interface{} `json:"manifest"`
	Name     string                 `json:"name"`
	Tags     []string               `json:"tags"`
}

// deleteDockerImage deletes a docker image from Google Container Registry.
func (tr *GKETestRunner) deleteDockerImage(ctx context.Context, ImageName string) []error {
	queryImageURL := fmt.Sprintf("https://gcr.io/v2/%s/tags/list", ImageName)
	resp, err := tr.Client.Get(queryImageURL)
	if err != nil {
		return []error{fmt.Errorf("failed to list tags: %v", err)}
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []error{err}
	}
	var ir imageResponse
	if err := json.Unmarshal(body, &ir); err != nil {
		return []error{err}
	}

	const deleteImageURLFmt = "https://gcr.io/v2/%s/manifests/%s"
	var errs []error
	for _, tag := range ir.Tags {
		if err := deleteDockerImageResource(tr.Client, fmt.Sprintf(deleteImageURLFmt, ImageName, tag)); err != nil {
			errs = append(errs, fmt.Errorf("failed to delete tag %s: %v", tag, err))
		}
	}

	for manifest := range ir.Manifest {
		if err := deleteDockerImageResource(tr.Client, fmt.Sprintf(deleteImageURLFmt, ImageName, manifest)); err != nil {
			errs = append(errs, fmt.Errorf("failed to delete manifest %s: %v", manifest, err))
		}
	}
	return errs
}

func deleteDockerImageResource(client *http.Client, url string) error {
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to get request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete resource: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("failed to delete resource: status code = %d", resp.StatusCode)
	}
	return nil
}

// DeleteClusterAndImage deletes cluster and images used to create cluster.
func (tr *GKETestRunner) DeleteClusterAndImage(ctx context.Context, cfg *ClusterConfig) []error {
	var errs []error
	if err := tr.StorageClient.Bucket(cfg.Bucket).Object(cfg.ImageSourceName).Delete(ctx); err != nil {
		errs = append(errs, fmt.Errorf("failed to delete storage client: %v", err))
	}
	for _, err := range tr.deleteDockerImage(ctx, cfg.ImageName) {
		errs = append(errs, fmt.Errorf("failed to delete docker image: %v", err))
	}
	if _, err := tr.ContainerService.Projects.Zones.Clusters.Delete(cfg.ProjectID, cfg.Zone, cfg.ClusterName).Context(ctx).Do(); err != nil {
		errs = append(errs, fmt.Errorf("failed to delete cluster %s: %v", cfg.ClusterName, err))
	}

	return errs
}
