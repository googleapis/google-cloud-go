#!/bin/bash

# Copyright 2020 Google Inc. All Rights Reserved.
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

retry() {
  for i in {1..3}; do
  [ $i == 1 ] || sleep 10  # Backing off after a failed attempt.
    "${@}" && return 0
  done
  return 1
}

# Fail on any error.
set -eo pipefail

# Display commands being run.
set -x

cd $(dirname $0)/..

git config --global --add safe.directory /tmpfs/src/github/google-cloud-go

export GOOGLE_APPLICATION_CREDENTIALS="${KOKORO_KEYSTORE_DIR}/72935_cloud-profiler-e2e-service-account-key"
export GCLOUD_TESTS_GOLANG_PROJECT_ID="cloud-profiler-e2e"

# Ensure a newer version of Go is used so it is compatible with newer libraries.
# Here we install v1.18.4 which is the current version as of July 2022.
retry curl -LO https://go.dev/dl/go1.18.4.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && tar -C /usr/local -xzf go1.18.4.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Run test.
go version
retry go mod download
go test -run TestAgentIntegration -run_only_profiler_backoff_test -timeout 1h
