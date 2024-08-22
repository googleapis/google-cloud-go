#!/bin/bash

# Copyright 2024 Google LLC
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

# Imports github.com/eliben/go-sentencepiece for local vendoring in our module,
# with the author's permission.

# Fail on any error
set -eo pipefail

# Display commands being run
set -x

# Create a temporary directory
TEMP_DIR=$(mktemp -d)

# Clone the repository with --depth 1 to get only the latest files
git clone --depth 1 https://github.com/eliben/go-sentencepiece.git "$TEMP_DIR/go-sentencepiece"

rm -rf sentencepiece
mkdir -p sentencepiece
rsync -av \
    --exclude='.git' \
    --exclude='.github' \
    --exclude='go.mod' \
    --exclude='go.sum' \
    --exclude='wasm' \
    --exclude='doc' \
    --exclude='test' \
    --exclude='*_test.go' \
    "$TEMP_DIR/go-sentencepiece/" sentencepiece

# Replace import paths.
find "sentencepiece" -type f -name '*.go' \
    -exec sed -i 's|github.com/eliben/go-sentencepiece|cloud.google.com/go/vertexai/internal/sentencepiece|g' {} +

# Prepend the LICENSE_HEADER to each .go file
GO_FILES=$(find sentencepiece -type f -name '*.go')
LICENSE_HEADER=$(realpath "LICENSE_HEADER")

for gofile in $GO_FILES; do
    cat "$LICENSE_HEADER" "$gofile" > "$gofile.tmp" && mv "$gofile.tmp" "$gofile"
done
