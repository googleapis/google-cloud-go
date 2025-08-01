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

# This script performs an integration test on the compiled librariangen binary.
# It simulates the environment that the Librarian tool would create by:
# 1. Creating a temporary directory structure for inputs and outputs.
# 2. Copying the testdata protos into the temporary source directory.
# 3. Creating a generate-request.json file.
# 4. Compiling and running the librariangen binary with flags pointing to the
#    temporary directories.
# 5. Verifying that the binary succeeds and generates the expected files.

set -e # Exit immediately if a command exits with a non-zero status.
# set -x # Print commands and their arguments as they are executed.

LIBRARIANGEN_GOTOOLCHAIN=local
LIBRARIANGEN_LOG=librariangen.log
echo "Cleaning up from last time: rm -f $LIBRARIANGEN_LOG"
rm -f "$LIBRARIANGEN_LOG"

# --- Dependency Checks & Version Info ---
(
echo "--- Tool Versions ---"
echo "Go: $(GOWORK=off GOTOOLCHAIN=${LIBRARIANGEN_GOTOOLCHAIN} go version)"
echo "protoc: $(protoc --version 2>&1)"
echo "protoc-gen-go: $(protoc-gen-go --version 2>&1)"
echo "protoc-gen-go-grpc: $(protoc-gen-go-grpc --version 2>&1)"
echo "protoc-gen-go_gapic: v0.53.1"
echo "---------------------"
) >> "$LIBRARIANGEN_LOG" 2>&1

# Ensure that all required protoc dependencies are available in PATH.
if ! command -v "protoc" &> /dev/null; then
  echo "Error: protoc not found in PATH. Please install it."
fi
if ! command -v "protoc-gen-go" &> /dev/null; then
  echo "Error: protoc-gen-go not found in PATH. Please install it."
  echo "  go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.35.2"
fi
if ! command -v "protoc-gen-go-grpc" &> /dev/null; then
  echo "Error: protoc-gen-go-grpc not found in PATH. Please install it."
  echo "  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.3.0"
fi
if ! command -v "protoc-gen-go_gapic" &> /dev/null; then
  echo "protoc-gen-go_gapic not found in PATH. Installing..."
  (GOWORK=off GOTOOLCHAIN=${LIBRARIANGEN_GOTOOLCHAIN} go install github.com/googleapis/gapic-generator-go/cmd/protoc-gen-go_gapic@v0.53.1)
fi


echo "Cleaning up from last time..."
rm -rf "$TEST_DIR"
rm -f "$BINARY_PATH"

# --- Setup ---

enable_post_processor=true

# Create a temporary directory for the entire test environment.
TEST_DIR=$(mktemp -d -t tmp.XXXXXXXXXX)
echo "Using temporary directory: $TEST_DIR"

# Define the directories replicating the mounts in the Docker container.
LIBRARIAN_DIR="$TEST_DIR/librarian"
OUTPUT_DIR="$TEST_DIR/output"
mkdir -p "$LIBRARIAN_DIR" "$OUTPUT_DIR"

# Use an external googleapis checkout.
if [ ! -d "$LIBRARIANGEN_GOOGLEAPIS_DIR" ]; then
  echo "Error: LIBRARIANGEN_GOOGLEAPIS_DIR is not set or not a directory."
  echo "Please set it to the path of your local googleapis clone."
  exit 1
fi
echo "Using googleapis source from $LIBRARIANGEN_GOOGLEAPIS_DIR"
SOURCE_DIR="$LIBRARIANGEN_GOOGLEAPIS_DIR"

# The compiled binary will be placed in the current directory.
BINARY_PATH="./librariangen"
echo "Cleaning up from last time: rm -f $BINARY_PATH"
rm -f "$BINARY_PATH"

# --- Prepare Inputs ---

# 1. Copy the generate-request.json into the librarian directory.
cp "testdata/librarian/generate-request.json" "$LIBRARIAN_DIR/"

# --- Execute ---

# 3. Compile the librariangen binary.
echo "Compiling librariangen..."
GOWORK=off GOTOOLCHAIN=${LIBRARIANGEN_GOTOOLCHAIN} go build -o "$BINARY_PATH" .

# 4. Run the librariangen generate command.
echo "Running librariangen..."
if [ "$enable_post_processor" = true ]; then
    PATH=$(GOWORK=off GOTOOLCHAIN=${LIBRARIANGEN_GOTOOLCHAIN} go env GOPATH)/bin:$HOME/go/bin:$PATH ./librariangen generate \
      --source="$SOURCE_DIR" \
      --librarian="$LIBRARIAN_DIR" \
      --output="$OUTPUT_DIR" >> "$LIBRARIANGEN_LOG" 2>&1
else
    PATH=$(GOWORK=off GOTOOLCHAIN=${LIBRARIANGEN_GOTOOLCHAIN} go env GOPATH)/bin:$HOME/go/bin:$PATH ./librariangen generate \
      --source="$SOURCE_DIR" \
      --librarian="$LIBRARIAN_DIR" \
      --output="$OUTPUT_DIR" \
      --disable-post-processor >> "$LIBRARIANGEN_LOG" 2>&1
fi


# Run gofmt just like the Bazel rule:
# https://github.com/googleapis/gapic-generator-go/blob/main/rules_go_gapic/go_gapic.bzl#L34
# TODO: move this to librariangen
GOWORK=off GOTOOLCHAIN=${LIBRARIANGEN_GOTOOLCHAIN} gofmt -w -l $OUTPUT_DIR > /dev/null

# --- Verify ---

# 5. Check that the command succeeded and generated files.
echo "Verifying output..."
echo "Librariangen logs are available in: $LIBRARIANGEN_LOG"
if [ -z "$(ls -A "$OUTPUT_DIR")" ]; then
  echo "Error: Output directory is empty."
  exit 1
fi

if [ "$enable_post_processor" = true ]; then
    # Use a cached version of google-cloud-go if available.
    if [ ! -d "$LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR" ]; then
      echo "Error: LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR is not set or not a directory."
      echo "Please set it to the path of your local google-cloud-go clone."
      exit 1
    fi
    echo "Using cached google-cloud-go from $LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR"
    GEN_DIR="$LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR"
    # Define the API paths to verify.
    APIS=(
      "chronicle/apiv1"
    )
    # These are the corresponding paths in the google-cloud-go repository.
    GEN_API_PATHS=(
      "chronicle/apiv1"
    )
else
    # Use a cached version of googleapis-gen if available.
    if [ ! -d "$LIBRARIANGEN_GOOGLEAPIS_GEN_DIR" ]; then
      echo "Error: LIBRARIANGEN_GOOGLEAPIS_GEN_DIR is not set or not a directory."
      echo "Please set it to the path of your local googleapis-gen clone."
      exit 1
    fi
    echo "Using cached googleapis-gen from $LIBRARIANGEN_GOOGLEAPIS_GEN_DIR"
    GEN_DIR="$LIBRARIANGEN_GOOGLEAPIS_GEN_DIR"
    # Define the API paths to verify.
    APIS=(
      "chronicle/apiv1"
    )
    # These are the corresponding paths in the googleapis-gen repository.
    GEN_API_PATHS=(
      "google/cloud/chronicle/v1"
    )
fi

# --- Verification using Git ---
echo "Verifying output by comparing with the goldens repository..."
echo "The script will modify files in your local goldens clone."

# Before files are copied, run git reset and git clean to clean up prior run.
(
echo "--- Git Reset Summary ---"
pushd "$GEN_DIR" > /dev/null
git reset --hard HEAD
git clean -fd
popd > /dev/null
) >> "$LIBRARIANGEN_LOG" 2>&1

# Process each API by replacing the expected files with the generated ones
if [ "$enable_post_processor" = true ]; then
    module_name=$(echo "${APIS[0]}" | cut -d'/' -f1)
    OUTPUT_MODULE_DIR="$OUTPUT_DIR/$module_name"
    EXPECTED_MODULE_DIR="$GEN_DIR/$module_name"
    rm -rf "${EXPECTED_MODULE_DIR:?}"/*
    mkdir -p "$EXPECTED_MODULE_DIR"
    cp -a "$OUTPUT_MODULE_DIR"/. "$EXPECTED_MODULE_DIR/"
else
    for i in "${!APIS[@]}"; do
      api="${APIS[$i]}"
      gen_api_path="${GEN_API_PATHS[$i]}"

      OUTPUT_API_DIR="$OUTPUT_DIR/$api"
      EXPECTED_API_DIR="$GEN_DIR/$gen_api_path/cloud.google.com/go/$api"

      # 1. Remove everything from the expected directory
      rm -rf "${EXPECTED_API_DIR:?}"/*

      # 2. Copy over all files from the output directory
      # Ensure the directory exists after cleaning
      mkdir -p "$EXPECTED_API_DIR"
      cp -a "$OUTPUT_API_DIR"/. "$EXPECTED_API_DIR/"
    done
fi

# After all files are copied, run git add and git status to show changes.
# This entire section is redirected to the log file for later inspection.
(
echo "--- Git Status Summary ---"
pushd "$GEN_DIR" > /dev/null
# Get original git config setting.
original_filemode=$(git config --get core.fileMode)
# Temporarily ignore file mode changes for a cleaner status report.
git config core.fileMode false
# Stage all changes. New files, modifications, and deletions will be staged.
if [ "$enable_post_processor" = true ]; then
    module_name=$(echo "${APIS[0]}" | cut -d'/' -f1)
    git add "$module_name"
    # Print the human-readable status. This will now ignore permission changes.
    git status "$module_name"
else
    git add .
    # Print the human-readable status. This will now ignore permission changes.
    git status
fi
# Restore the original file mode setting for subsequent manual inspection.
if [ -n "$original_filemode" ]; then
  git config core.fileMode "$original_filemode"
else
  git config --unset core.fileMode
fi

# --- Diff of First Modified File ---
if [ "$enable_post_processor" = true ]; then
    module_name=$(echo "${APIS[0]}" | cut -d'/' -f1)
    # Find all top-level files in the output module directory.
    top_level_files=$(find "$OUTPUT_DIR/$module_name" -maxdepth 1 -type f -exec basename {} \;)
    for file in $top_level_files; do
        echo ""
        echo "--- Diff for $module_name/$file ---"
        git -C "$GEN_DIR" -c core.pager=cat diff --staged -p -- "$module_name/$file"
    done
else
    # Use `git diff --numstat` to find the first file with actual content changes,
    # ignoring the noise from permission-only differences.
    first_modified_file=$(git -C "$GEN_DIR" diff --staged --numstat | awk '$1 != "0" || $2 != "0" {print $3}' | head -n 1)

    if [ -n "$first_modified_file" ]; then
      echo ""
      echo "--- Diff for first modified file: $first_modified_file ---"
      # Run git diff --staged to see the staged changes for that file in patch format.
      # We use `-c core.pager=cat` to prevent git from opening an interactive pager.
      git -C "$GEN_DIR" -c core.pager=cat diff --staged -p -- "$first_modified_file"
    fi
fi

popd > /dev/null
) >> "$LIBRARIANGEN_LOG" 2>&1

echo ""
echo "Verification complete. The status in $LIBRARIANGEN_LOG shows the difference between the"
echo "expected generated output (goldens) and the current modified state of your goldens repository (librariangen)."
echo ""
echo -e "To reset your goldens repository:"
echo "  cd $GEN_DIR"
echo "  git reset --hard HEAD && git clean -fd"

echo "Binary integration test passed successfully."
echo "Generated files are available for inspection in: $OUTPUT_DIR"
