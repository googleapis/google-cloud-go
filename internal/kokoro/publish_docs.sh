#!/bin/bash
# Copyright 2020 Google LLC
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

# Fail on any error.
set -eo pipefail

# Display commands being run.
set -x

cd github/google-cloud-go/internal/godocfx
go install -buildvcs=false
cd -

export RELEASE_PROJECT_ID=google-cloud-go-releases

if [[ -n "$FORCE_GENERATE_ALL" ]]; then
  for m in $(find . -name go.mod -execdir go list -m \; | grep -v internal); do
    godocfx -out obj/api/$m@latest $m
  done
elif [[ -n "$MODULE" ]]; then
  godocfx "$MODULE"
else
  godocfx -project $RELEASE_PROJECT_ID -new-modules cloud.google.com/go google.golang.org/appengine
fi

for f in $(find obj/api -name docs.metadata); do
  # Extract the module name and version from the docs.metadata file.
  module=$(cat $f | grep name | sed 's/.*"\(.*\)"/\1/')
  version=$(cat $f | grep version | sed 's/.*"\(.*\)"/\1/')
  name="docfx-go-$module-$version.tar.gz"
  tar_dir=$(dirname $name)
  mkdir -p $tar_dir
  tar \
    --create \
    --directory=$(dirname $f) \
    --file=$name \
    --gzip \
    .
  gsutil cp $name gs://docs-staging-v2/$tar_dir
done
