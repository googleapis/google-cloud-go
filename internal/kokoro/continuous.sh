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

##
# continuous.sh
# Runs CI checks for entire repository.
#
# Jobs types
#
# Continuous: Runs root tests & tests in submodules changed by a PR. Triggered by PR merges.
# Nightly: Runs root tests & tests in all modules. Triggered nightly.
# Nightly/$MODULE: Runs tests in a specified module. Triggered nightly.
##

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

# runTests runs all tests in the current directory.
# If a PATH argument is specified, it runs `go test [PATH]`.
runDirectoryTests() {
  go test -race -v -timeout 45m "${1:-./...}" 2>&1 \
    | tee sponge_log.log
  # Takes the kokoro output log (raw stdout) and creates a machine-parseable
  # xUnit XML file.
  cat sponge_log.log \
    | go-junit-report -set-exit-code > sponge_log.xml
  # Add the exit codes together so we exit non-zero if any module fails.
  exit_code=$(($exit_code + $?))
}

# testAllModules runs all modules tests.
testAllModules() {
  echo "Testing all modules"
  for i in $(find . -name go.mod); do
    pushd "$(dirname "$i")" > /dev/null;
      runDirectoryTests
    popd > /dev/null;
  done
}

# testChangedModules runs tests in changed modules only.
testChangedModules() {
  for d in $CHANGED_DIRS; do
    goDirectories="$(find "$d" -name "*.go" -printf "%h\n" | sort -u)"
    if [[ -n "$goDirectories" ]]; then
      for gd in $goDirectories; do
        pushd "$gd" > /dev/null;
          runDirectoryTests .
        popd > /dev/null;
      done
    fi
  done
}

set +e # Run all tests, don't stop after the first failure.
exit_code=0

if [[ $KOKORO_JOB_NAME == *"continuous"* ]]; then
  # Continuous jobs only run root tests & tests in submodules changed by the PR.
  SIGNIFICANT_CHANGES=$(git --no-pager diff --name-only $KOKORO_GIT_COMMIT^..$KOKORO_GIT_COMMIT | grep -Ev '(\.md$|^\.github)' || true)
  # CHANGED_DIRS is the list of significant top-level directories that changed,
  # but weren't deleted by the current PR. CHANGED_DIRS will be empty when run on master.
  CHANGED_DIRS=$(echo "$SIGNIFICANT_CHANGES" | tr ' ' '\n' | grep "/" | cut -d/ -f1 | sort -u | tr '\n' ' ' | xargs ls -d 2>/dev/null || true)
  # If PR changes affect all submodules, then run all tests.
  if [[ -z $SIGNIFICANT_CHANGES ]] || echo "$SIGNIFICANT_CHANGES" | tr ' ' '\n' | grep "^go.mod$" || [[ $CHANGED_DIRS =~ "internal" ]]; then
    testAllModules
  else
    runDirectoryTests . # Always run base tests.
    echo "Running tests only in changed submodules: $CHANGED_DIRS"
    testChangedModules
  fi
elif [[ $KOKORO_JOB_NAME == *"nightly"* ]]; then
  # Expected job name format: ".../nightly/[OPTIONAL_MODULE_NAME]/[OPTIONAL_JOB_NAMES...]"
  ARR=(${KOKORO_JOB_NAME//// }) # Splits job name by "/" where ARR[0] is expected to be "nightly".
  SUBMODULE_NAME=${ARR[5]} # Gets the token after "nightly/".
  if [[ -n $SUBMODULE_NAME ]] && [[ -d "./$SUBMODULE_NAME" ]]; then
    # Only run tests in the submodule designated in the Kokoro job name.
    # Expected format example: ...google-cloud-go/nightly/logging.
    runDirectoryTests . # Always run base tests
    echo "Running tests in one submodule: $SUBMODULE_NAME"
    pushd $SUBMODULE_NAME > /dev/null;
      runDirectoryTests
    popd > /dev/null
  else
    # Run all tests if it is a regular nightly job.
    testAllModules
  fi
else
  testAllModules
fi

if [[ $KOKORO_BUILD_ARTIFACTS_SUBDIR = *"continuous"* ]] || [[ $KOKORO_BUILD_ARTIFACTS_SUBDIR = *"nightly"* ]]; then
  chmod +x $KOKORO_GFILE_DIR/linux_amd64/flakybot
  $KOKORO_GFILE_DIR/linux_amd64/flakybot -logs_dir=$GOCLOUD_HOME \
    -repo=googleapis/google-cloud-go \
    -commit_hash=$KOKORO_GITHUB_COMMIT_URL_google_cloud_go
fi

exit $exit_code
