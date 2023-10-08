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

# Fail on any error
set -eo pipefail

# Display commands being run
set -x

# Only run on Go 1.20.2+
min_minor_ver=20
min_patch_ver=2

v=`go version | { read _ _ v _; echo ${v#go}; }`
comps=(${v//./ })
minor_ver=${comps[1]}
patch_ver=${comps[2]}

if [ "$minor_ver" -lt "$min_minor_ver" ]; then
    echo minor version $minor_ver, skipping
    exit 0
fi

if [ "$patch_ver" -lt "$min_patch_ver" ]; then
    echo patch version $patch_ver, skipping
    exit 0
fi

export BIGTABLE_TEST_PROXY_PORT=9999
export BIGTABLE_CLIENT_TESTS_HOME=$KOKORO_ARTIFACTS_DIR/cloud-bigtable-clients-test/

# Build and start the proxy in a separate process
nohup go run internal/testproxy/proxy.go --port $BIGTABLE_TEST_PROXY_PORT &
proxyPID=$!

# Stop the testproxy & cleanup environment variables
function cleanup() {
    echo "Cleanup testproxy"
    kill $proxyPID
    unset BIGTABLE_TEST_PROXY_PORT;
    unset BIGTABLE_CLIENT_TESTS_HOME;
}
trap cleanup EXIT

# Checkout Bigtable conformance tests 
mkdir -p $BIGTABLE_CLIENT_TESTS_HOME
git clone https://github.com/googleapis/cloud-bigtable-clients-test.git $BIGTABLE_CLIENT_TESTS_HOME

pushd $BIGTABLE_CLIENT_TESTS_HOME > /dev/null;
    cd tests

    # Run the conformance tests
    echo "Running the Bigtable conformance tests" | tee -a sponge_log.log
    go test -v -proxy_addr=:$BIGTABLE_TEST_PROXY_PORT | tee -a sponge_log.log

    # Run the conformance tests skipping known failures
    echo "Running the Bigtable conformance tests skipping known failures" | tee -a sponge_log.log
    eval "go test -v -proxy_addr=:$BIGTABLE_TEST_PROXY_PORT -skip `cat $GOCLOUD_HOME/bigtable/testproxy/conformance_known_failures.txt` | tee -a sponge_log.log"
    RETURN_CODE=$?
popd > /dev/null;

echo "exiting with ${RETURN_CODE}"
exit ${RETURN_CODE}