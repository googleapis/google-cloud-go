#!/usr/bin/env bash
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

echo "==> 1. Ensuring protoc-gen-go-vtproto is installed..."
export PATH="$(go env GOPATH)/bin:${PATH}"
if ! command -v protoc-gen-go-vtproto &> /dev/null; then
    go install github.com/planetscale/vtprotobuf/cmd/protoc-gen-go-vtproto@v0.6.0
fi

echo "==> 2. Vendoring dependencies..."
rm -rf vendor
go mod tidy
go mod vendor

echo "==> 3. Generating vtprotobuf methods for Cloud Spanner..."
TMP_PROTO_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_PROTO_DIR}"' EXIT

git clone --depth 1 https://github.com/googleapis/googleapis.git "${TMP_PROTO_DIR}/googleapis"

protoc \
  -I"${TMP_PROTO_DIR}/googleapis" \
  --go-vtproto_out=vendor \
  --go-vtproto_opt=features=marshal+unmarshal+size+pool \
  "${TMP_PROTO_DIR}/googleapis/google/spanner/v1/"*.proto

echo "==> Successfully generated vtprotobuf code into vendor/cloud.google.com/go/spanner/apiv1/spannerpb!"
echo "==> Ready! You can run tests with: go test ./..."
