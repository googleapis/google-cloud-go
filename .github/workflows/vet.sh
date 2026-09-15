#!/bin/bash
# Copyright 2019 Google LLC
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

# Fail on any error
set -e

# Display commands being run
set -x

# Fail if a dependency was added without the necessary go.mod/go.sum change
# being part of the commit.
go mod tidy
for i in $(find . -name go.mod); do
  pushd $(dirname $i)
  go mod tidy
  popd
done

git diff '*go.mod'  | tee /dev/stderr | (! read)
git diff '*go.sum'  | tee /dev/stderr | (! read)

goimports -l . 2>&1 | grep -vE ".pb.go" | tee /dev/stderr | (! read)


staticcheck ./... 2>&1 | (
    grep -v SA1019 |
    grep -v internal/btree/btree.go |
    grep -v httpreplay/internal/proxy/debug.go |
    grep -v third_party/pkgsite/synopsis.go
) |
  tee /dev/stderr | (! read)

echo "Done vetting!"
