#!/bin/bash
# Copyright 2025 Google LLC
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

# This script builds the librariangen Docker image and then runs a series of
# checks inside a container to verify that all dependencies are correctly
# installed and available in the PATH.

set -e # Exit immediately if a command exits with a non-zero status.

IMAGE_NAME="gcr.io/cloud-go-infra/librariangen:latest"

echo "--- Building Docker Image ---"
docker build -t "$IMAGE_NAME" .
echo "--- Docker Image Built Successfully ---"

echo ""
echo "--- Verifying Dependencies Inside Container ---"

# Create a temporary script to act as the test entrypoint.
# This script will check for the existence and version of all required tools.
cat > entrypoint-test.sh << EOF
#!/bin/bash
set -e

echo "--- Verifying librariangen binary ---"
if ! command -v librariangen &> /dev/null; then
    echo "Error: librariangen not found in PATH."
    exit 1
fi
echo "librariangen found."
echo "version: \$(librariangen --version)"

echo ""
echo "--- Verifying Go ---"
if ! command -v go &> /dev/null; then
    echo "Error: go not found in PATH."
    exit 1
fi
go version

echo ""
echo "--- Verifying protoc ---"
if ! command -v protoc &> /dev/null; then
    echo "Error: protoc not found in PATH."
    exit 1
fi
protoc --version

echo ""
echo "--- Verifying Go Plugins ---"
if ! command -v protoc-gen-go &> /dev/null; then
    echo "Error: protoc-gen-go not found in PATH."
    exit 1
fi
protoc-gen-go --version

if ! command -v protoc-gen-go-grpc &> /dev/null; then
    echo "Error: protoc-gen-go-grpc not found in PATH."
    exit 1
fi
protoc-gen-go-grpc --version

if ! command -v protoc-gen-go_gapic &> /dev/null; then
    echo "Error: protoc-gen-go_gapic not found in PATH."
    exit 1
fi
# The gapic generator does not have a --version flag, so we check for its presence.
echo "protoc-gen-go_gapic found."

echo ""
echo "--- Verifying Post-processor Tools ---"
if ! command -v goimports &> /dev/null; then
    echo "Error: goimports not found in PATH."
    exit 1
fi
echo "goimports found."

if ! command -v staticcheck &> /dev/null; then
    echo "Error: staticcheck not found in PATH."
    exit 1
fi
staticcheck --version

echo ""
echo "--- All Dependencies Verified Successfully ---"
EOF

chmod +x entrypoint-test.sh

# Run the container with the test script as the entrypoint.
# We mount the test script into the container and execute it.
docker run --rm --entrypoint /bin/bash -v "$(pwd)/entrypoint-test.sh:/entrypoint-test.sh" "$IMAGE_NAME" /entrypoint-test.sh

# Clean up the temporary test script.
rm entrypoint-test.sh

echo ""
echo "--- Docker Image Verification Complete ---"
