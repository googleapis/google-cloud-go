#!/bin/bash
# Copyright 2023 Google LLC
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
# beta-integration.sh
# Runs CI checks for spanner submodule on other environments excluding production.
#
# Jobs types
#
# Continuous: Runs root tests & tests in spanner submodules if changed by a PR. Triggered by PR merges.
# Nightly: Runs root tests & tests in spanner module. Triggered nightly.
##

export GOOGLE_APPLICATION_CREDENTIALS=$(realpath ${KOKORO_GFILE_DIR}/${GOOGLE_APPLICATION_CREDENTIALS})

export GCLOUD_TESTS_GOLANG_PROJECT_ID=dulcet-port-762
export GCLOUD_TESTS_GOLANG_SPANNER_HOST=staging-wrenchworks.sandbox.googleapis.com:443
export GCLOUD_TESTS_GOLANG_KEY=$GOOGLE_APPLICATION_CREDENTIALS

# Spanner integration tests for backup/restore is flaky https://github.com/googleapis/google-cloud-go/issues/5037
# to fix the flaky test Spanner need to run on us-west1 region (OMG/43748). But other environments do not have this region, so
# using regional-us-central1 instead.
export GCLOUD_TESTS_GOLANG_SPANNER_INSTANCE_CONFIG="regional-us-central1"

# Fail on any error
set -eo pipefail

# Display commands being run
set -x

# cd to project dir on Kokoro instance
cd github/google-cloud-go
git config --global --add safe.directory "$(pwd)/./.git"

go version

export GOCLOUD_HOME=$KOKORO_ARTIFACTS_DIR/google-cloud-go/
export PATH="$GOPATH/bin:$PATH"
export GO111MODULE=on
export GOPROXY=https://proxy.golang.org

# Move code into artifacts dir
mkdir -p $GOCLOUD_HOME
git clone . $GOCLOUD_HOME
cd $GOCLOUD_HOME

# cd to spanner dir on Kokoro instance
cd spanner

try3() { eval "$*" || eval "$*" || eval "$*"; }

# only spanner packages are fetched.
try3 go mod download
# get out of spanner directory
cd ..

# runDirectoryTests runs all tests in the current directory.
# If a PATH argument is specified, it runs `go test [PATH]`.
runDirectoryTests() {
  if [[ $PWD != *"/internal/"* ]] ||
    [[ $PWD != *"/third_party/"* ]] &&
    [[ $KOKORO_JOB_NAME == *"earliest"* ]]; then
    # internal tools only expected to work with latest go version
    return
  fi
  go test -race -v -timeout 45m "${1:-./...}" 2>&1 |
    tee sponge_log.log
  # Takes the kokoro output log (raw stdout) and creates a machine-parseable
  # xUnit XML file.
  cat sponge_log.log |
    go-junit-report -set-exit-code >sponge_log.xml
  # Add the exit codes together so we exit non-zero if any module fails.
  exit_code=$(($exit_code + $?))
}

# testChangedModules runs tests in changed modules only.
testChangedModules() {
  for d in $CHANGED_DIRS; do
    goDirectories="$(find "$d" -name "*.go" -printf "%h\n" | sort -u)"
    if [[ -n "$goDirectories" ]]; then
      for gd in $goDirectories; do
        # run tests only if spanner module is part of $CHANGED_DIRS
        if [[ $gd == *"spanner"* ]]; then
          pushd "$gd" >/dev/null
          runDirectoryTests
          popd >/dev/null
        fi
      done
    fi
  done
}

set +e # Run all tests, don't stop after the first failure.
exit_code=0

case $JOB_TYPE in
integration-cloud-devel)
  GCLOUD_TESTS_GOLANG_SPANNER_HOST=staging-wrenchworks.sandbox.googleapis.com:443
  echo "running against cloud-devel environment: $GCLOUD_TESTS_GOLANG_SPANNER_HOST"
  ;;
integration-cloud-staging)
  GCLOUD_TESTS_GOLANG_SPANNER_HOST=preprod-spanner.sandbox.googleapis.com:443
  echo "running against staging environment: $GCLOUD_TESTS_GOLANG_SPANNER_HOST"
  ;;
esac

if [[ $KOKORO_JOB_NAME == *"continuous"* ]]; then
  # Continuous jobs only run root tests & tests in submodules changed by the PR.
  # We need to find CHANGED_DIRS because PR merge in any other modulo ether than spanner also triggers this sh file. So we verify if spanner is part of CHANGED_DIRS
  SIGNIFICANT_CHANGES=$(git --no-pager diff --name-only $KOKORO_GIT_COMMIT^..$KOKORO_GIT_COMMIT | grep -Ev '(\.md$|^\.github|\.json$|\.yaml$)' || true)
  # CHANGED_DIRS is the list of significant top-level directories that changed,
  # but weren't deleted by the current PR. CHANGED_DIRS will be empty when run on main.
  CHANGED_DIRS=$(echo "$SIGNIFICANT_CHANGES" | tr ' ' '\n' | grep "/" | cut -d/ -f1 | sort -u | tr '\n' ' ' | xargs ls -d 2>/dev/null || true)
  # Run tests for spanner alone if it is part of CHANGED_DIRS.
  runDirectoryTests . # Always run base tests.
  echo "list of changed submodules: $CHANGED_DIRS"
  testChangedModules
elif [[ $KOKORO_JOB_NAME == *"nightly"* ]]; then
  # Expected job name format: ".../nightly/[OPTIONAL_MODULE_NAME]/[OPTIONAL_JOB_NAMES...]"
  ARR=(${KOKORO_JOB_NAME//// }) # Splits job name by "/" where ARR[0] is expected to be "nightly".
  SUBMODULE_NAME=${ARR[5]}      # Gets the token after "nightly/".
  # Runs test only in spanner submodule
  if [[ -n $SUBMODULE_NAME ]] && [[ -d "./$SUBMODULE_NAME" ]] && [[ $SUBMODULE_NAME == *"spanner"* ]]; then
    # Only run tests in the submodule designated in the Kokoro job name.
    # Expected format example: ...google-cloud-go/nightly/spanner.
    runDirectoryTests . # Always run base tests
    echo "Running tests in one submodule: $SUBMODULE_NAME"
    pushd $SUBMODULE_NAME >/dev/null
    runDirectoryTests
    popd >/dev/null
  fi
fi

# Disabling flaky bot reporting on intergration test errors temporarily.
#if [[ $KOKORO_BUILD_ARTIFACTS_SUBDIR = *"continuous"* ]] || [[ $KOKORO_BUILD_ARTIFACTS_SUBDIR = *"nightly"* ]]; then
#  chmod +x $KOKORO_GFILE_DIR/linux_amd64/flakybot
#  $KOKORO_GFILE_DIR/linux_amd64/flakybot -logs_dir=$GOCLOUD_HOME \
#    -repo=googleapis/google-cloud-go \
#    -commit_hash=$KOKORO_GITHUB_COMMIT_URL_google_cloud_go
#fi

exit $exit_code
