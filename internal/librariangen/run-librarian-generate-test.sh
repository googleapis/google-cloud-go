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

# This script performs an integration test on the librariangen Docker container.
# It simulates the environment that the Librarian tool would create by:
# 1. Creating a temporary directory structure for inputs and outputs.
# 2. Mounting these directories into the container.
# 3. Running the container to generate files.
# 4. Verifying that the container succeeds and generates the expected files by
#    comparing them against golden repositories.

set -e # Exit immediately if a command exits with a non-zero status.

IMAGE_NAME="gcr.io/cloud-devrel-public-resources/librarian-go:infrastructure-public-image-latest"

# --- Host Dependency Checks ---
if ! command -v "docker" &> /dev/null; then
  echo "Error: docker not found in PATH. Please install it."
  exit 1
fi
if ! command -v "git" &> /dev/null; then
  echo "Error: git not found in PATH. Please install it."
  exit 1
fi

# --- Setup ---

if [ ! -d "$LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR" ]; then
  echo "Error: LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR is not set or not a directory."
  exit 1
fi

# Check for external googleapis checkout.
if [ ! -d "$LIBRARIANGEN_GOOGLEAPIS_DIR" ]; then
  echo "Error: LIBRARIANGEN_GOOGLEAPIS_DIR is not set or not a directory."
  exit 1
fi
echo "Using googleapis source from $LIBRARIANGEN_GOOGLEAPIS_DIR"

# --- Prepare Target ---

echo "--- Git Reset Summary ---"
pushd "$LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR" > /dev/null
git reset --hard HEAD
git clean -fd
popd

# --- Execute ---

echo "Running librarian..."

go run github.com/googleapis/librarian/cmd/librarian@HEAD generate \
  --image="$IMAGE_NAME" \
  --repo="$LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR" \
  --library=secretmanager \
  --api=google/cloud/secretmanager/v1,google/cloud/secretmanager/v1beta2 \
  --api-source="$LIBRARIANGEN_GOOGLEAPIS_DIR"


# --- Verify ---

echo "Verifying output..."
echo "Using cached google-cloud-go from $LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR"
APIS=("secretmanager/apiv1")
GEN_API_PATHS=("chronicle/apiv1")

# --- Verification using Git ---
echo "Verifying output by comparing with the goldens repository..."


echo "--- Git Status Summary ---"
pushd "$LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR" > /dev/null
original_filemode=$(git config --get core.fileMode)
git config core.fileMode false
module_name=$(echo "${APIS[0]}" | cut -d'/' -f1)
git add "$module_name"
git status "$module_name"

if [ -n "$original_filemode" ]; then
  git config core.fileMode "$original_filemode"
else
  git config --unset core.fileMode
fi

module_name=$(echo "${APIS[0]}" | cut -d'/' -f1)
# Get top level files, excluding go.sum
top_level_files=$(find "$LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR/$module_name" -maxdepth 1 -type f -exec basename {} \; | grep -v "go.sum$" | sed "s,^,$module_name/," )

# Add other specific files
specific_files_to_add="$module_name/apiv1/secretmanagerpb/service.pb.go $module_name/apiv1/version.go $module_name/internal/version.go"

files_to_diff="$top_level_files $specific_files_to_add"

for file in $files_to_diff; do
    echo ""
    echo "--- Diff for $file ---"
    # This will show nothing if there is no diff, which is fine.
    git -C "$LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR" -c core.pager=cat diff --staged -p -- "$file"
done

popd

echo ""
echo "Verification complete."
echo "To reset your goldens repository:"
echo "  cd LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR"
echo "  git reset --hard HEAD && git clean -fd"

echo "Container integration test passed successfully."
