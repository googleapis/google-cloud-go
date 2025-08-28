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

# This script performs an integration test on the compiled librariangen binary
# for the `release-init` command.

set -e # Exit immediately if a command exits with a non-zero status.

LIBRARIANGEN_LOG=librariangen-release-init.log
echo "Cleaning up from last time: rm -f $LIBRARIANGEN_LOG"
rm -f "$LIBRARIANGEN_LOG"

# --- Host Dependency Checks ---
if ! command -v "go" &> /dev/null;
then
  echo "Error: go not found in PATH. Please install it."
  exit 1
fi
if ! command -v "git" &> /dev/null;
then
  echo "Error: git not found in PATH. Please install it."
  exit 1
fi

# --- Setup ---
if [ ! -d "$LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR" ]; then
  echo "Error: LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR is not set or not a directory."
  exit 1
fi
echo "Using google-cloud-go repo from $LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR"
GOLDEN_REPO_DIR="$LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR"

# --- Build ---
echo "Building librariangen binary..."
go build .
echo "Build complete."

# --- Test Execution ---
echo ""
echo "--------------------------------------"
echo "Running 'release-init' integration test..."
echo "--------------------------------------"
TEST_DIR=$(mktemp -d -t tmp.XXXXXXXXXX)
echo "Using temporary directory: $TEST_DIR"
OUTPUT_DIR="$TEST_DIR/output"
LIBRARIAN_INPUT_DIR="$TEST_DIR/librarian-input"
mkdir -p "$OUTPUT_DIR" "$LIBRARIAN_INPUT_DIR"

# Prepare a temporary librarian directory with our test fixtures.
cp -r "testdata/release-init/.librarian/." "$LIBRARIAN_INPUT_DIR/"
cp -r "testdata/release-init/librarian/." "$LIBRARIAN_INPUT_DIR/"

# Reset the golden repo to a clean state before running the test.
(
  echo "--- Git Reset Summary (Pre-run) ---"
  pushd "$GOLDEN_REPO_DIR" > /dev/null
  git reset --hard HEAD
  git clean -fd
  popd > /dev/null
) >> "$LIBRARIANGEN_LOG" 2>&1

# Execute
echo "Running librariangen release-init..."
GOOGLE_SDK_GO_LOGGING_LEVEL=debug ./librariangen release-init \
  --librarian="$LIBRARIAN_INPUT_DIR" \
  --repo="$GOLDEN_REPO_DIR" \
  --output="$OUTPUT_DIR" >> "$LIBRARIANGEN_LOG" 2>&1


# --- Verify ---
echo "Verifying output against golden repository..."

# Copy the output into the golden repo.
cp -r "$OUTPUT_DIR/." "$GOLDEN_REPO_DIR/"

# Check the git status and diff to verify the changes.
(
  echo "--- Git Status Summary ---"
  pushd "$GOLDEN_REPO_DIR" > /dev/null
  git add .
  git status
  echo "--- Git Diff Summary ---"
  git diff --staged
  popd > /dev/null
) >> "$LIBRARIANGEN_LOG" 2>&1


# A simple check to ensure some files were changed. A more robust check
# would involve comparing against a known-good diff.
if [ -z "$(git -C "$GOLDEN_REPO_DIR" status --porcelain)" ]; then
    echo "Error: No files were changed in the golden repository."
    exit 1
fi

echo "'release-init' integration test passed successfully."
echo "Logs are available in: $LIBRARIANGEN_LOG"
echo "To inspect changes, see the git status of: $GOLDEN_REPO_DIR"
