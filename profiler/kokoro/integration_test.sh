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

# Remove expired certificate; otherwise `go mod download` may fail.
# See https://letsencrypt.org/docs/dst-root-ca-x3-expiration-september-2021/
# for more context.
sudo apt-get install -y ca-certificates
sudo rm -f /usr/share/ca-certificates/mozilla/DST_Root_CA_X3.crt
sudo update-ca-certificates

cd $(dirname $0)/..

export GOOGLE_APPLICATION_CREDENTIALS="${KOKORO_KEYSTORE_DIR}/72935_cloud-profiler-e2e-service-account-key"
export GCLOUD_TESTS_GOLANG_PROJECT_ID="cloud-profiler-e2e"

# Run test.
retry go mod download
go test -run TestAgentIntegration -run_only_profiler_backoff_test -timeout 1h
