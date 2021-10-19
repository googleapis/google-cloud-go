#!/bin/bash
# Copyright 2020 Google LLC
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
# limitations under the License..

# Fail on any error
set -eo pipefail

# Display commands being run
set -x

export STORAGE_EMULATOR_PORT=9000
export STORAGE_EMULATOR_HOST=http://localhost:$STORAGE_EMULATOR_PORT
#export GCLOUD_TESTS_GOLANG_PROJECT_ID=emulator-test-project
echo "Running the Cloud Storage emulator: $STORAGE_EMULATOR_HOST";

# Download the emulator
export DEFAULT_IMAGE_NAME='gcr.io/cloud-devrel-public-resources/storage-testbench'
export DEFAULT_IMAGE_TAG='latest'

docker pull ${DEFAULT_IMAGE_NAME}:${DEFAULT_IMAGE_TAG}

# Start the emulator
docker run -p $STORAGE_EMULATOR_PORT ${DEFAULT_IMAGE_NAME}:${DEFAULT_IMAGE_TAG} &

EMULATOR_PID=$!

# Stop the emulator & clean the environment variables
function cleanup() {
    echo "Cleanup environment variables"
    kill -2 $EMULATOR_PID
    unset STORAGE_EMULATOR_HOST
    unset STORAGE_EMULATOR_PORT
    unset DEFAULT_IMAGE_NAME
    unset DEFAULT_IMAGE_TAG;
}
trap cleanup EXIT

# the regex ^[^23] skips conformance tests with ids 2 and 3, which are non-idempotent and do not yet pass
# TODO: remove regex once non-idempotent retries are aligned
go test -v -timeout 10m ./ -run=TestRetryConformance/^[^23] -short 2>&1 | tee -a sponge_log.log
