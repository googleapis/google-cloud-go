#!/bin/bash
# Copyright 2021 Google LLC
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

export STORAGE_EMULATOR_HOST="http://localhost:9000"

DEFAULT_IMAGE_NAME='gcr.io/cloud-devrel-public-resources/storage-testbench'
DEFAULT_IMAGE_TAG='latest'
DOCKER_IMAGE=${DEFAULT_IMAGE_NAME}:${DEFAULT_IMAGE_TAG}
CONTAINER_NAME=storage_testbench

# Get the docker image for the testbench
docker pull $DOCKER_IMAGE

# Start the testbench
# Note: --net=host makes the container bind directly to the Docker host’s network, 
# with no network isolation. If we were to use port-mapping instead, reset connection errors 
# would be captured differently and cause unexpected test behaviour.
# The host networking driver works only on Linux hosts.
# See more about using host networking: https://docs.docker.com/network/host/
docker run --name $CONTAINER_NAME --rm --net=host $DOCKER_IMAGE &
echo "Running the Cloud Storage testbench: $STORAGE_EMULATOR_HOST"

# Check that the server is running - retry several times to allow for start-up time
response=$(curl -w "%{http_code}\n" $STORAGE_EMULATOR_HOST --retry-connrefused --retry 5 -o /dev/null) 

if [[ $response != 200 ]]
then
    echo "Testbench server did not start correctly"
    exit 1
fi

# Stop the testbench & cleanup environment variables
function cleanup() {
    echo "Cleanup testbench"
    docker stop $CONTAINER_NAME
    unset STORAGE_EMULATOR_HOST;
}
trap cleanup EXIT

# TODO: move to passing once fixed
FAILING=(   "buckets.setIamPolicy"
            "objects.insert"
        )
# TODO: remove regex once all tests are passing
# Unfortunately, there is no simple way to skip specific tests (see https://github.com/golang/go/issues/41583)
# Therefore, we have to simply run all the specific tests we know pass
PASSING=(   "buckets.list"
            "buckets.insert"
            "buckets.get"
            "buckets.delete"
            "buckets.update"
            "buckets.patch"
            "buckets.getIamPolicy"
            "buckets.testIamPermissions"
            "buckets.lockRetentionPolicy"
            "objects.copy"
            "objects.get"
            "objects.list"
            "objects.delete"
            "objects.update"
            "objects.patch"
            "objects.compose"
            "objects.rewrite"
            "serviceaccount.get"
            "hmacKey.get"
            "hmacKey.list"
            "hmacKey.create"
            "hmacKey.delete"
            "hmacKey.update"
            "notifications.list"
            "notifications.create"
            "notifications.get"
            "notifications.delete"
            "object_acl.insert"
            "object_acl.get"
            "object_acl.list"
            "object_acl.patch"
            "object_acl.update"
            "object_acl.delete"
            "default_object_acl.insert"
            "default_object_acl.get"
            "default_object_acl.list"
            "default_object_acl.patch"
            "default_object_acl.update"
            "default_object_acl.delete"
            "bucket_acl.insert"
            "bucket_acl.get"
            "bucket_acl.list"
            "bucket_acl.patch"
            "bucket_acl.update"
            "bucket_acl.delete"
        )
TEMP=${PASSING[@]} 
PASSING_REGEX=${TEMP// /|}

# Run tests
go test -v -timeout 10m ./ -run="TestRetryConformance/($PASSING_REGEX)-" -short 2>&1 | tee -a sponge_log.log
