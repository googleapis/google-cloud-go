#!/bin/bash

# This script performs an integration test on the librariangen Docker container.
# It simulates the environment that the Librarian tool would create by:
# 1. Creating a temporary directory structure for inputs and outputs.
# 2. Mounting these directories into the container.
# 3. Running the container to generate files.
# 4. Verifying that the container succeeds and generates the expected files by
#    comparing them against golden repositories.

set -e # Exit immediately if a command exits with a non-zero status.

IMAGE_NAME="gcr.io/cloud-go-infra/librariangen:latest"
LIBRARIANGEN_LOG=librariangen-container.log
echo "Cleaning up from last time: rm -f $LIBRARIANGEN_LOG"
rm -f "$LIBRARIANGEN_LOG"

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

enable_post_processor=true
# Parse command-line arguments
for arg in "$@"
do
    case $arg in
        --enable-post-processor)
        enable_post_processor=true
        shift # Remove --enable-post-processor from processing
        ;;
    esac
done

# Create a temporary directory for the entire test environment.
TEST_DIR=$(mktemp -d -t tmp.XXXXXXXXXX)
echo "Using temporary directory: $TEST_DIR"

# Define the directories replicating the mounts in the Docker container.
LIBRARIAN_DIR="$TEST_DIR/librarian"
OUTPUT_DIR="$TEST_DIR/output"
mkdir -p "$LIBRARIAN_DIR" "$OUTPUT_DIR"

# Check for external googleapis checkout.
if [ ! -d "$LIBRARIANGEN_GOOGLEAPIS_DIR" ]; then
  echo "Error: LIBRARIANGEN_GOOGLEAPIS_DIR is not set or not a directory."
  exit 1
fi
echo "Using googleapis source from $LIBRARIANGEN_GOOGLEAPIS_DIR"

# --- Prepare Inputs ---

# Copy the generate-request.json into the librarian directory.
cp "testdata/librarian/generate-request.json" "$LIBRARIAN_DIR/"

# --- Execute ---

echo "Running librariangen container..."
# The container's /source, /librarian, and /output directories are mapped to
# the host's directories using --mount.
if [ "$enable_post_processor" = true ]; then
    docker run --rm \
      --mount type=bind,source="$LIBRARIANGEN_GOOGLEAPIS_DIR",target=/source,readonly \
      --mount type=bind,source="$LIBRARIAN_DIR",target=/librarian,readonly \
      --mount type=bind,source="$OUTPUT_DIR",target=/output \
      "$IMAGE_NAME" \
      generate \
      --source=/source \
      --librarian=/librarian \
      --output=/output \
      --enable-post-processor >> "$LIBRARIANGEN_LOG" 2>&1
else
    docker run --rm \
      --mount type=bind,source="$LIBRARIANGEN_GOOGLEAPIS_DIR",target=/source,readonly \
      --mount type=bind,source="$LIBRARIAN_DIR",target=/librarian,readonly \
      --mount type=bind,source="$OUTPUT_DIR",target=/output \
      "$IMAGE_NAME" \
      generate \
      --source=/source \
      --librarian=/librarian \
      --output=/output >> "$LIBRARIANGEN_LOG" 2>&1
fi

# --- Verify ---

echo "Verifying output..."
echo "Librariangen logs are available in: $LIBRARIANGEN_LOG"
if [ -z "$(ls -A "$OUTPUT_DIR")" ]; then
  echo "Error: Output directory is empty."
  exit 1
fi

if [ "$enable_post_processor" = true ]; then
    if [ ! -d "$LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR" ]; then
      echo "Error: LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR is not set or not a directory."
      exit 1
    fi
    echo "Using cached google-cloud-go from $LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR"
    GEN_DIR="$LIBRARIANGEN_GOOGLE_CLOUD_GO_DIR"
    APIS=("chronicle/apiv1")
    GEN_API_PATHS=("chronicle/apiv1")
else
    if [ ! -d "$LIBRARIANGEN_GOOGLEAPIS_GEN_DIR" ]; then
      echo "Error: LIBRARIANGEN_GOOGLEAPIS_GEN_DIR is not set or not a directory."
      exit 1
    fi
    echo "Using cached googleapis-gen from $LIBRARIANGEN_GOOGLEAPIS_GEN_DIR"
    GEN_DIR="$LIBRARIANGEN_GOOGLEAPIS_GEN_DIR"
    APIS=("chronicle/apiv1")
    GEN_API_PATHS=("google/cloud/chronicle/v1")
fi

# --- Verification using Git ---
echo "Verifying output by comparing with the goldens repository..."

(
echo "--- Git Reset Summary ---"
pushd "$GEN_DIR" > /dev/null
git reset --hard HEAD
git clean -fd
popd > /dev/null
) >> "$LIBRARIANGEN_LOG" 2>&1

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
      rm -rf "${EXPECTED_API_DIR:?}"/*
      mkdir -p "$EXPECTED_API_DIR"
      cp -a "$OUTPUT_API_DIR"/. "$EXPECTED_API_DIR/"
done
fi

(
echo "--- Git Status Summary ---"
pushd "$GEN_DIR" > /dev/null
original_filemode=$(git config --get core.fileMode)
git config core.fileMode false
if [ "$enable_post_processor" = true ]; then
    module_name=$(echo "${APIS[0]}" | cut -d'/' -f1)
    git add "$module_name"
    git status "$module_name"
else
    git add .
    git status
fi
if [ -n "$original_filemode" ]; then
  git config core.fileMode "$original_filemode"
else
  git config --unset core.fileMode
fi

if [ "$enable_post_processor" = true ]; then
    module_name=$(echo "${APIS[0]}" | cut -d'/' -f1)
    top_level_files=$(find "$OUTPUT_DIR/$module_name" -maxdepth 1 -type f -exec basename {} \;)
    for file in $top_level_files; do
        echo ""
        echo "--- Diff for $module_name/$file ---"
        git -C "$GEN_DIR" -c core.pager=cat diff --staged -p -- "$module_name/$file"
    done
else
    first_modified_file=$(git -C "$GEN_DIR" diff --staged --numstat | awk '$1 != "0" || $2 != "0" {print $3}' | head -n 1)
    if [ -n "$first_modified_file" ]; then
      echo ""
      echo "--- Diff for first modified file: $first_modified_file ---"
      git -C "$GEN_DIR" -c core.pager=cat diff --staged -p -- "$first_modified_file"
    fi
fi
popd > /dev/null
) >> "$LIBRARIANGEN_LOG" 2>&1

echo ""
echo "Verification complete. The status in $LIBRARIANGEN_LOG shows the difference."
echo "To reset your goldens repository:"
echo "  cd $GEN_DIR"
echo "  git reset --hard HEAD && git clean -fd"

echo "Container integration test passed successfully."
echo "Generated files are available for inspection in: $OUTPUT_DIR"
