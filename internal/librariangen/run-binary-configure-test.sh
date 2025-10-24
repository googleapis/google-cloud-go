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

# This script performs an integration test on the compiled librariangen binary's
# 'configure' command. It simulates the environment that the Librarian tool
# would create by:
# 1. Creating a temporary directory structure for inputs and outputs.
# 2. Copying testdata fixtures (request, config, state) into the temp dir.
# 3. Compiling and running the librariangen binary with flags pointing to the
#    temporary directories and real checkouts of googleapis and google-cloud-go.
# 4. Verifying that the binary succeeds and generates the expected files for a
#    new module.

set -e # Exit immediately if a command exits with a non-zero status.

# --- Cleanup ---
# The TEST_DIR and BINARY_PATH variables are defined later in the script.
# A trap is used to ensure that cleanup happens regardless of script success or failure.
cleanup() {
  echo "Cleaning up..."
  rm -rf "$TEST_DIR"
  rm -f "$BINARY_PATH"
}
trap cleanup EXIT INT TERM

LIBRARIANGEN_GOTOOLCHAIN=local
LIBRARIANGEN_LOG=librariangen.log
echo "Cleaning up from last time: rm -f $LIBRARIANGEN_LOG"
rm -f "$LIBRARIANGEN_LOG"

# --- Dependency Checks & Version Info ---
(
echo "--- Tool Versions ---"
echo "Go: $(GOWORK=off GOTOOLCHAIN=${LIBRARIANGEN_GOTOOLCHAIN} go version)"
echo "---------------------"
) >> "$LIBRARIANGEN_LOG" 2>&1

# --- Setup ---

# Create a temporary directory for the entire test environment.
TEST_DIR=$(mktemp -d -t tmp.XXXXXXXXXX)
echo "Using temporary directory: $TEST_DIR"

# Define the directories replicating the mounts in the Docker container.
LIBRARIAN_DIR="$TEST_DIR/librarian"
INPUT_DIR="$TEST_DIR/input"
OUTPUT_DIR="$TEST_DIR/output"
mkdir -p "$LIBRARIAN_DIR" "$INPUT_DIR" "$OUTPUT_DIR"

# Use an external googleapis checkout for the --source flag.
if [ ! -d "$LIBRARIANGEN_GOOGLEAPIS_DIR" ]; then
  echo "Error: LIBRARIANGEN_GOOGLEAPIS_DIR is not set or not a directory."
  echo "Please set it to the path of your local googleapis clone."
  exit 1
fi
echo "Using googleapis source from $LIBRARIANGEN_GOOGLEAPIS_DIR"
SOURCE_DIR="$LIBRARIANGEN_GOOGLEAPIS_DIR"

# Use an external google-cloud-go checkout for the --repo flag.
if [ ! -d "$LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR" ]; then
  echo "Error: LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR is not set or not a directory."
  echo "Please set it to the path of your local google-cloud-go clone."
  exit 1
fi
echo "Using google-cloud-go repo from $LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR"
REPO_DIR="$LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR"


# The compiled binary will be placed in the current directory.
BINARY_PATH="./librariangen"

# --- Prepare Inputs ---

# 1. Copy the configure-request.json and other .librarian files into the librarian directory.
cp "testdata/configure/librarian/configure-request.json" "$LIBRARIAN_DIR/"
cp "testdata/configure/.librarian/config.yaml" "$LIBRARIAN_DIR/"
cp "testdata/configure/.librarian/state.yaml" "$LIBRARIAN_DIR/"


# --- Execute ---

# 2. Compile the librariangen binary.
echo "Compiling librariangen..."
GOWORK=off GOTOOLCHAIN=${LIBRARIANGEN_GOTOOLCHAIN} go build -o "$BINARY_PATH" .

# 3. Run the librariangen configure command.
echo "Running librariangen configure..."
GOOGLE_SDK_GO_LOGGING_LEVEL=debug ./librariangen configure \
  --source="$SOURCE_DIR" \
  --repo="$REPO_DIR" \
  --librarian="$LIBRARIAN_DIR" \
  --input="$INPUT_DIR" \
  --output="$OUTPUT_DIR" >> "$LIBRARIANGEN_LOG" 2>&1

# --- Verify ---

# 4. Check that the command succeeded and generated files.
echo "Verifying output..."
echo "Librariangen logs are available in: $LIBRARIANGEN_LOG"
if [ -z "$(ls -A "$OUTPUT_DIR")" ]; then
  echo "Error: Output directory is empty."
  exit 1
fi

# 5. Verify that the expected files were created.
FILES_TO_CHECK=(
  "$LIBRARIAN_DIR/configure-response.json"
  "$OUTPUT_DIR/capacityplanner/README.md"
  "$OUTPUT_DIR/capacityplanner/CHANGES.md"
  "$OUTPUT_DIR/capacityplanner/internal/version.go"
  "$OUTPUT_DIR/capacityplanner/apiv1beta/version.go"
  "$OUTPUT_DIR/internal/generated/snippets/go.mod"
)

for file in "${FILES_TO_CHECK[@]}"; do
  if [ ! -f "$file" ]; then
    echo "Error: Expected file not found: $file"
    exit 1
  fi
done

echo "All expected files were found."
echo "Binary integration test passed successfully."
echo "Generated files are available for inspection in: $OUTPUT_DIR"

# --- Verification using Git ---
echo "Verifying output by comparing with the goldens repository..."
echo "The script will modify files in your local goldens clone."

echo "Using cached google-cloud-go from $LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR"
GEN_DIR="$LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR"

# Before files are copied, run git reset and git clean to clean up prior run.
(
echo "--- Git Reset Summary ---"
pushd "$GEN_DIR" > /dev/null
git reset --hard HEAD
git clean -fd
popd > /dev/null
) >> "$LIBRARIANGEN_LOG" 2>&1

# Process the new module by replacing the expected files with the generated ones
module_name="capacityplanner"
OUTPUT_MODULE_DIR="$OUTPUT_DIR/$module_name"
EXPECTED_MODULE_DIR="$GEN_DIR/$module_name"
rm -rf "${EXPECTED_MODULE_DIR:?}"
cp -a "$OUTPUT_MODULE_DIR" "$GEN_DIR/"

# Also copy the modified go.mod and go.sum for snippets
cp "$OUTPUT_DIR/internal/generated/snippets/go.mod" "$GEN_DIR/internal/generated/snippets/"
if [ -f "$OUTPUT_DIR/internal/generated/snippets/go.sum" ]; then
    cp "$OUTPUT_DIR/internal/generated/snippets/go.sum" "$GEN_DIR/internal/generated/snippets/"
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
git add .
# Print the human-readable status. This will now ignore permission changes.
git status
# Restore the original file mode setting for subsequent manual inspection.
if [ -n "$original_filemode" ]; then
  git config core.fileMode "$original_filemode"
else
  git config --unset core.fileMode
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
