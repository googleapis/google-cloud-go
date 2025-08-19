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

# This script performs a hermetic integration test on the librariangen Docker
# container for the `release-init` command.

set -e # Exit immediately if a command exits with a non-zero status.

IMAGE_NAME="gcr.io/cloud-go-infra/librariangen:latest"
LIBRARIANGEN_LOG=librariangen-container-release-init.log
echo "Cleaning up from last time: rm -f $LIBRARIANGEN_LOG"
rm -f "$LIBRARIANGEN_LOG"

# --- Host Dependency Checks ---
if ! command -v "docker" &> /dev/null; then
  echo "Error: docker not found in PATH. Please install it."
  exit 1
fi

# --- Setup ---
TEST_DIR=$(mktemp -d -t tmp.XXXXXXXXXX)
echo "Using temporary directory: $TEST_DIR"

LIBRARIAN_DIR="$TEST_DIR/librarian"
REPO_DIR="$TEST_DIR/repo"
OUTPUT_DIR="$TEST_DIR/output"
mkdir -p "$LIBRARIAN_DIR" "$REPO_DIR" "$OUTPUT_DIR"

# --- Prepare Inputs ---
# The testdata directories contain the state of the world *before* the release.
cp -r "testdata/release-init/.librarian/." "$LIBRARIAN_DIR/"
cp -r "testdata/release-init/repo/." "$REPO_DIR/"

# --- Execute ---
echo "Running librariangen container for release-init..."
docker run --rm \
  --env GOOGLE_SDK_GO_LOGGING_LEVEL=debug \
  --mount type=bind,source="$(pwd)/$LIBRARIAN_DIR",target=/librarian,readonly \
  --mount type=bind,source="$(pwd)/$REPO_DIR",target=/repo,readonly \
  --mount type=bind,source="$(pwd)/$OUTPUT_DIR",target=/output \
  "$IMAGE_NAME" \
  release-init \
  --librarian=/librarian \
  --repo=/repo \
  --output=/output >> "$LIBRARIANGEN_LOG" 2>&1

# --- Verify ---
echo "Verifying output..."
echo "Librariangen logs are available in: $LIBRARIANGEN_LOG"

if diff -r "$OUTPUT_DIR" "testdata/release-init/golden"; then
  echo "'release-init' container integration test passed successfully."
else
  echo "Error: Output does not match golden files. See diff above."
  exit 1
fi

echo "Generated files are available for inspection in: $OUTPUT_DIR"
