#!/usr/bin/env bash
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -ev

echo "==== Install protoc ===="
curl -fsSL --retry 5 --retry-delay 15 -o /tmp/protoc.zip https://github.com/protocolbuffers/protobuf/releases/download/v25.7/protoc-25.7-linux-x86_64.zip
sha256sum -c <(echo 877408bab02767938d1e5555f11c39dfe05e96f2a9571bc59dd2639f33d9954e /tmp/protoc.zip)
sudo unzip -o /tmp/protoc.zip -d /usr/local
protoc --version

echo "==== Regenerate all the code ===="
version=$(sed -n 's/^version: *//p' librarian.yaml)
go run github.com/googleapis/librarian/cmd/librarian@${version} generate --all

# If there is any difference between the generated code and the
# committed code that is an error. All the inputs should be pinned,
# including the generator version and the googleapis SHA.
git diff --exit-code
