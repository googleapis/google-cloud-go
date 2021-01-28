// Copyright 2017 Google LLC
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

// +build linux

package profiler

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"text/template"
	"time"

	"cloud.google.com/go/profiler/proftest"
	"golang.org/x/oauth2/google"
	compute "google.golang.org/api/compute/v1"
)

var runBackoffTest = flag.Bool("run_only_profiler_backoff_test", false, "Enables only the backoff integration test. This integration test requires over 45 mins to run, so it is not run by default.")

const (
	cloudScope        = "https://www.googleapis.com/auth/cloud-platform"
	benchFinishString = "benchmark application(s) complete"
	errorString       = "failed to set up or run the benchmark"
	gceBenchDuration  = 200 * time.Second
	gceTestTimeout    = 25 * time.Minute

	// For any agents to receive backoff, there must be more than 32 agents in
	// the deployment. The initial backoff received will be 33 minutes; each
	// subsequent backoff will be one minute longer. Running 45 benchmarks for
	// 45 minutes will ensure that several agents receive backoff responses and
	// are able to wait for the backoff duration then send another request.
	numBackoffBenchmarks = 45
	backoffBenchDuration = 45 * time.Minute
	backoffTestTimeout   = 60 * time.Minute
)

const startupTemplate = `
{{ define "setup"}}
# Install git
retry apt-get update >/dev/null
retry apt-get -y -q install git >/dev/null

# $GOCACHE is required from Go 1.12. See https://golang.org/doc/go1.11#gocache
# $GOCACHE is explicitly set because $HOME is not set when this code runs
mkdir -p /tmp/gocache
export GOCACHE=/tmp/gocache

# Install gcc, needed to install go master
if [ "{{.GoVersion}}" = "master" ]
then
retry apt-get -y -q install gcc >/dev/null
fi

# Install desired Go version
mkdir -p /tmp/bin
retry curl -sL -o /tmp/bin/gimme https://raw.githubusercontent.com/travis-ci/gimme/master/gimme
chmod +x /tmp/bin/gimme
export PATH=$PATH:/tmp/bin

retry eval "$(gimme {{.GoVersion}})"

# Set $GOPATH
export GOPATH="$HOME/go"

export GOCLOUD_HOME=$GOPATH/src/cloud.google.com/go
mkdir -p $GOCLOUD_HOME

# Install agent
retry git clone https://code.googlesource.com/gocloud $GOCLOUD_HOME >/dev/null
cd $GOCLOUD_HOME
retry git fetch origin {{.Commit}}
git reset --hard {{.Commit}}

cd $GOCLOUD_HOME/profiler/busybench
retry go get >/dev/null
{{- end }}

{{ define "integration" -}}
{{- template "prologue" . }}
{{- template "setup" . }}

# Run benchmark with agent.
go run busybench.go --service="{{.Service}}" --duration={{.DurationSec}} --mutex_profiling="{{.MutexProfiling}}"

echo "{{.FinishString}}"

{{ template "epilogue" . -}}
{{ end }}

{{ define "integration_backoff" -}}
{{- template "prologue" . }}
{{- template "setup" . }}
# Do not display commands being run to simplify logging output.
set +x

# Run benchmarks with agent.
echo "Starting {{.NumBackoffBenchmarks}} benchmarks."
for (( i = 0; i < {{.NumBackoffBenchmarks}}; i++ )); do
	(go run busybench.go --service="{{.Service}}" --duration={{.DurationSec}} \
		--num_busyworkers=1) |& while read line; \
		do echo "benchmark $i: ${line}"; done &
done
echo "Successfully started {{.NumBackoffBenchmarks}} benchmarks."

wait

# Continue displaying commands being run.
set -x

echo "{{.FinishString}}"

{{ template "epilogue" . -}}
{{ end }}
`

type goGCETestCase struct {
	proftest.InstanceConfig
	name          string
	goVersion     string
	benchDuration time.Duration
	timeout       time.Duration

	backoffTest bool

	// mutexProfiling and wantProfileTypes will not be used when the test
	// is a backoff integration test.
	mutexProfiling   bool
	wantProfileTypes []string
}

func (tc *goGCETestCase) initializeStartupScript(template *template.Template, commit string) error {
	params := struct {
		Service              string
		GoVersion            string
		Commit               string
		ErrorString          string
		FinishString         string
		MutexProfiling       bool
		DurationSec          int
		NumBackoffBenchmarks int
	}{
		Service:        tc.InstanceConfig.Name,
		GoVersion:      tc.goVersion,
		Commit:         commit,
		ErrorString:    errorString,
		FinishString:   benchFinishString,
		MutexProfiling: tc.mutexProfiling,
		DurationSec:    int(tc.benchDuration.Seconds()),
	}

	testTemplate := "integration"
	if tc.backoffTest {
		testTemplate = "integration_backoff"
		params.DurationSec = int(backoffBenchDuration.Seconds())
		params.NumBackoffBenchmarks = numBackoffBenchmarks
	}
	var buf bytes.Buffer
	err := template.Lookup(testTemplate).Execute(&buf, params)
	if err != nil {
		return fmt.Errorf("failed to render startup script for %s: %v", tc.name, err)
	}
	tc.StartupScript = buf.String()
	return nil
}

// gitCommit returns the Git commit of the current directory. The source
// checkout in the test VM will run in the same commit. Note that any local
// changes to the profiler agent won't be tested in the integration test. This
// flow only works with code that has been committed and pushed to the public
// repo (either to master or to a branch).
func gitCommit() (string, error) {
	output, err := exec.Command("git", "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.Trim(string(output), "\n"), nil
}

// pstTimeStr returns a string representation of the time in the PST timezone.
func pstTimeStr() (string, error) {
	pst, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return "", fmt.Errorf("failed to initialize PST location: %v", err)
	}
	return strings.Replace(time.Now().In(pst).Format("2006-01-02-15-04-05.000000-0700"), ".", "-", -1), nil
}

func TestAgentIntegration(t *testing.T) {
	// Testing against master requires building go code and may take up to 10 minutes.
	// Allow this test to run in parallel with other top level tests to avoid timeouts.
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping profiler integration tests in short mode")
	}

	projectID := os.Getenv("GCLOUD_TESTS_GOLANG_PROJECT_ID")
	if projectID == "" {
		t.Skip("skipping profiler integration test when GCLOUD_TESTS_GOLANG_PROJECT_ID variable is not set")
	}

	// all us-west1 zones
	zones := []string{"us-west1-a", "us-west1-b", "us-west1-c"}

	commit, err := gitCommit()
	if err != nil {
		t.Fatalf("failed to gather the Git revision of the current source: %v", err)
	}
	t.Logf("using Git commit %q for the profiler integration test", commit)

	runID, err := pstTimeStr()
	if err != nil {
		t.Fatalf("failed to get current time to generate a run ID: %v", err)
	}

	ctx := context.Background()

	client, err := google.DefaultClient(ctx, cloudScope)
	if err != nil {
		t.Fatalf("failed to get default client: %v", err)
	}

	computeService, err := compute.New(client)
	if err != nil {
		t.Fatalf("failed to initialize compute service: %v", err)
	}

	template, err := proftest.BaseStartupTmpl.Parse(startupTemplate)
	if err != nil {
		t.Fatalf("failed to parse startup script template: %v", err)
	}

	tr := proftest.TestRunner{
		Client: client,
	}
	gceTr := proftest.GCETestRunner{
		TestRunner:     tr,
		ComputeService: computeService,
	}

	// Determine go version used by current test run
	goVersion := strings.TrimPrefix(runtime.Version(), "go")
	goVersionName := strings.Replace(goVersion, ".", "", -1)

	testcases := []goGCETestCase{
		{
			InstanceConfig: proftest.InstanceConfig{
				ProjectID:   projectID,
				Name:        fmt.Sprintf("profiler-test-gomaster-%s", runID),
				MachineType: "n1-standard-1",
			},
			name:             "profiler-test-gomaster",
			wantProfileTypes: []string{"CPU", "HEAP", "THREADS", "CONTENTION", "HEAP_ALLOC"},
			goVersion:        "master",
			mutexProfiling:   true,
			timeout:          gceTestTimeout,
			benchDuration:    gceBenchDuration,
		},
		{
			InstanceConfig: proftest.InstanceConfig{
				ProjectID:   projectID,
				Name:        fmt.Sprintf("profiler-test-go%s-%s", goVersionName, runID),
				MachineType: "n1-standard-1",
			},
			name:             fmt.Sprintf("profiler-test-go%s", goVersionName),
			wantProfileTypes: []string{"CPU", "HEAP", "THREADS", "CONTENTION", "HEAP_ALLOC"},
			goVersion:        goVersion,
			mutexProfiling:   true,
			timeout:          gceTestTimeout,
			benchDuration:    gceBenchDuration,
		},
	}

	if *runBackoffTest {
		testcases = []goGCETestCase{
			{
				InstanceConfig: proftest.InstanceConfig{
					ProjectID: projectID,
					Name:      fmt.Sprintf("profiler-backoff-test-go%s-%s", goVersionName, runID),

					// Running many copies of the benchmark requires more
					// memory than is available on an n1-standard-1. Use a
					// machine type with more memory for backoff test.
					MachineType: "n1-highmem-2",
				},
				name:          fmt.Sprintf("profiler-backoff-test-go%s", goVersionName),
				goVersion:     goVersion,
				backoffTest:   true,
				timeout:       backoffTestTimeout,
				benchDuration: backoffBenchDuration,
			},
		}
	}
	// The number of tests run in parallel is the current value of GOMAXPROCS.
	runtime.GOMAXPROCS(len(testcases))
	for _, tc := range testcases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.initializeStartupScript(template, commit); err != nil {
				t.Fatalf("failed to initialize startup script")
			}

			for i := range zones {
				tc.InstanceConfig.Zone = zones[i]
				if err := gceTr.StartInstance(ctx, &tc.InstanceConfig); err != nil {
					if strings.Contains(err.Error(), "failed to create instance") && i < (len(zones)-1) {
						// try other zones if instance failed to create
						continue
					}
					t.Fatal(err)
				}
				break
			}
			defer func() {
				if err := gceTr.DeleteInstance(ctx, &tc.InstanceConfig); err != nil {
					t.Fatal(err)
				}
			}()

			timeoutCtx, cancel := context.WithTimeout(ctx, tc.timeout)
			defer cancel()
			output, err := gceTr.PollAndLogSerialPort(timeoutCtx, &tc.InstanceConfig, benchFinishString, errorString, t.Logf)
			if err != nil {
				t.Fatalf("PollAndLogSerialPort() got error: %v", err)
			}

			if tc.backoffTest {
				if err := proftest.CheckSerialOutputForBackoffs(output, numBackoffBenchmarks, "action throttled, backoff", "creating a new profile via profiler service", "benchmark"); err != nil {
					t.Errorf("failed to check serial output for backoffs: %v", err)
				}
			}

			timeNow := time.Now()
			endTime := timeNow.Format(time.RFC3339)
			startTime := timeNow.Add(-1 * time.Hour).Format(time.RFC3339)
			for _, pType := range tc.wantProfileTypes {
				pr, err := tr.QueryProfilesWithZone(tc.ProjectID, tc.InstanceConfig.Name, startTime, endTime, pType, tc.Zone)
				if err != nil {
					t.Errorf("QueryProfilesWithZone(%s, %s, %s, %s, %s, %s) got error: %v", tc.ProjectID, tc.InstanceConfig.Name, startTime, endTime, pType, tc.Zone, err)
					continue
				}
				if err := pr.HasFunction("busywork"); err != nil {
					t.Errorf("HasFunction(%s, %s, %s, %s, %s) got error: %v", tc.ProjectID, tc.InstanceConfig.Name, startTime, endTime, pType, err)
				}
			}
		})
	}
}
