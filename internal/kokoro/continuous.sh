#!/bin/bash
# Copyright 2019 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

export GOOGLE_APPLICATION_CREDENTIALS=$KOKORO_KEYSTORE_DIR/72523_go_integration_service_account
# Removing the GCLOUD_TESTS_GOLANG_PROJECT_ID setting may make some integration
# tests (like profiler's) silently skipped, so make sure you know what you are
# doing when changing / removing the next line.

export GCLOUD_TESTS_GOLANG_PROJECT_ID=dulcet-port-762
export GCLOUD_TESTS_GOLANG_KEY=$GOOGLE_APPLICATION_CREDENTIALS
export GCLOUD_TESTS_GOLANG_FIRESTORE_PROJECT_ID=gcloud-golang-firestore-tests
export GCLOUD_TESTS_GOLANG_FIRESTORE_KEY=$KOKORO_KEYSTORE_DIR/72523_go_firestore_integration_service_account
export GCLOUD_TESTS_API_KEY=`cat $KOKORO_KEYSTORE_DIR/72523_go_gcloud_tests_api_key`
export GCLOUD_TESTS_GOLANG_KEYRING=projects/dulcet-port-762/locations/us/keyRings/go-integration-test
export GCLOUD_TESTS_GOLANG_PROFILER_ZONE="us-west1-b"

# Fail on any error
set -eo pipefail

# Display commands being run
set -x

# cd to project dir on Kokoro instance
cd github/google-cloud-go

go version

export GOCLOUD_HOME=$KOKORO_ARTIFACTS_DIR/google-cloud-go/
export PATH="$GOPATH/bin:$PATH"
export GO111MODULE=on
export GOPROXY=https://proxy.golang.org

# Move code into artifacts dir
mkdir -p $GOCLOUD_HOME
git clone . $GOCLOUD_HOME
cd $GOCLOUD_HOME

try3() { eval "$*" || eval "$*" || eval "$*"; }

# All packages, including +build tools, are fetched.
try3 go mod download
go install github.com/jstemmer/go-junit-report
./internal/kokoro/vet.sh

# Continuous jobs only run root tests & tests in submodules changed by the PR
# Nightly jobs run all tests in all submodules
SIGNIFICANT_CHANGES=$(git --no-pager diff --name-only master..HEAD | grep -Ev '(\.md$|^\.github)' || true)
# CHANGED_DIRS is the list of significant top-level directories that changed,
# but weren't deleted by the current PR.
# CHANGED_DIRS will be empty when run on master.
CHANGED_DIRS=$(echo "$SIGNIFICANT_CHANGES" | tr ' ' '\n' | grep "/" | cut -d/ -f1 | sort -u | tr '\n' ' ' | xargs ls -d 2>/dev/null || true)

# List all modules in changed directories.
# If running on master will collect all modules in the repo, including the root module.
# shellcheck disable=SC2086
GO_CHANGED_MODULES="$(find ${CHANGED_DIRS:-.} -name go.mod)"
# If we didn't find any modules, use the root module.
GO_CHANGED_MODULES=${GO_CHANGED_MODULES:-./go.mod}
# Exclude the root module, if present, from the list of sub-modules.
GO_CHANGED_SUBMODULES=${GO_CHANGED_MODULES#./go.mod}

# Override to determine if all go tests should be run.
# Does not include static analysis checks.
RUN_ALL_TESTS="0"
# If this is a nightly test (not a PR), run all tests.
if [ -z "${KOKORO_GITHUB_PULL_REQUEST_NUMBER:-}" ]; then
  RUN_ALL_TESTS="0"
# If the change touches a repo-spanning file or directory of significance, run all tests.
elif echo "$SIGNIFICANT_CHANGES" | tr ' ' '\n' | grep "^go.mod$" || [[ $CHANGED_DIRS =~ "internal" ]]; then
  RUN_ALL_TESTS="1"
fi

# runTests runs the tests in the current directory. If an argument is specified,
# it is used as the argument to `go test`.
runTests() {
  go test -race -v -timeout 45m "${1:-./...}" 2>&1 \
    | tee sponge_log.log
  # Takes the kokoro output log (raw stdout) and creates a machine-parseable
  # xUnit XML file.
  cat sponge_log.log \
    | go-junit-report -set-exit-code > sponge_log.xml
  # Add the exit codes together so we exit non-zero if any module fails.
  exit_code=$(($exit_code + $?))
}

set +e # Run all tests, don't stop after the first failure.
exit_code=0

if [[ $RUN_ALL_TESTS = "1" ]]; then
  echo "Running all tests"
  # shellcheck disable=SC2044
  for i in $(find . -name go.mod); do
    pushd "$(dirname "$i")" > /dev/null;
      runTests
    popd > /dev/null;
  done
elif [[ -z "${CHANGED_DIRS// }" ]]; then
  echo "Only running root tests"
  runTests .
else
  runTests . # Always run root tests.
  echo "Running tests in modified directories: $CHANGED_DIRS"
  for d in $CHANGED_DIRS; do
    mods=$(find "$d" -name go.mod)
    # If there are no modules, just run the tests directly.
    if [[ -z "$mods" ]]; then
      pushd "$d" > /dev/null;
        runTests
      popd > /dev/null;
    # Otherwise, run the tests in all Go directories. This way, we don't have to
    # check to see if there are tests that aren't in a sub-module.
    else
      goDirectories="$(find "$d" -name "*.go" -printf "%h\n" | sort -u)"
      if [[ -n "$goDirectories" ]]; then
        for gd in $goDirectories; do
          pushd "$gd" > /dev/null;
            runTests .
          popd > /dev/null;
        done
      fi
    fi
  done
fi

if [[ $KOKORO_BUILD_ARTIFACTS_SUBDIR = *"continuous"* ]] || [[ $KOKORO_BUILD_ARTIFACTS_SUBDIR = *"nightly"* ]]; then
  chmod +x $KOKORO_GFILE_DIR/linux_amd64/buildcop
  $KOKORO_GFILE_DIR/linux_amd64/buildcop -logs_dir=$GOCLOUD_HOME \
    -repo=googleapis/google-cloud-go \
    -commit_hash=$KOKORO_GITHUB_COMMIT_URL_google_cloud_go
fi

exit $exit_code
