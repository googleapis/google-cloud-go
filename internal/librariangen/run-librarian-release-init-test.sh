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

# This script performs an end-to-end integration test for the `release-init`
# command by invoking the `librarian` CLI, which in turn runs the
# `librariangen` container.

set -e # Exit immediately if a command exits with a non-zero status.

IMAGE_NAME="gcr.io/cloud-go-infra/librariangen:latest"

# --- Host Dependency Checks ---
if ! command -v "docker" &> /dev/null; then
  echo "Error: docker not found in PATH. Please install it."
  exit 1
fi
if ! command -v "git" &> /dev/null; then
  echo "Error: git not found in PATH. Please install it."
  exit 1
fi
if ! command -v "go" &> /dev/null; then
  echo "Error: go not found in PATH. Please install it."
  exit 1
fi

# --- Setup ---
if [ ! -d "$LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR" ]; then
  echo "Error: LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR is not set or not a directory."
  exit 1
fi
echo "Using google-cloud-go repo from $LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR"

# --- Prepare Target Repository ---
echo "Preparing target repository..."
pushd "$LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR" > /dev/null
git reset --hard HEAD
git clean -fd
# Create a new commit to simulate a change that needs to be released.
echo "// New feature" >> "secretmanager/apiv1/secretmanager_client.go"
git add "secretmanager/apiv1/secretmanager_client.go"
git commit -m "feat(secretmanager): add new feature"
popd

# --- Execute ---
echo "Running librarian release init..."
# Note: We are testing the `release init` command of the librarian CLI.
go run github.com/googleapis/librarian/cmd/librarian@HEAD release init \
  --image="$IMAGE_NAME" \
  --repo="$LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR" \
  --library=secretmanager

# --- Verify ---
echo "Verifying output..."
pushd "$LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR" > /dev/null

# Check that the expected files were modified.
MODIFIED_FILES=$(git status --porcelain | awk '{print $2}')
EXPECTED_FILES=(
  ".librarian/state.yaml"
  "secretmanager/CHANGES.md"
  "secretmanager/internal/version.go"
  "internal/generated/snippets/secretmanager/snippet_metadata.google.cloud.secretmanager.v1.json"
)

for f in "${EXPECTED_FILES[@]}"; do
  if ! echo "$MODIFIED_FILES" | grep -q "$f"; then
    echo "Error: Expected file '$f' to be modified, but it was not."
    git status
    exit 1
  fi
done

echo "Correct files were modified. Checking content with git diff..."
git diff

# Check that the version was bumped in state.yaml
if ! grep -q "version: 1.16.0" ".librarian/state.yaml"; then
    echo "Error: Version was not bumped to 1.16.0 in state.yaml"
    exit 1
fi

echo "Verification successful."

# --- Cleanup ---
echo "Cleaning up target repository..."
git reset --hard HEAD
git clean -fd
popd

echo "Librarian release-init integration test passed successfully."
