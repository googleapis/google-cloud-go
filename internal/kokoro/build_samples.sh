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
# limitations under the License..

# Fail on any error
set -eo pipefail

# Display commands being run
set -x

# Where google-cloud-go code is located.
gcwd=$PWD/github/google-cloud-go

cd github/golang-samples

# Returns 0 if the test should be skipped because the current Go
# version is too old for the current module.
goVersionShouldSkip() {
  echo "Downloading modules and checking versions"
  modVersion="$(go list -m -f '{{.GoVersion}}')"
  if [ -z "$modVersion" ]; then
    # Not in a module or minimum Go version not specified, don't skip.
    return 1
  fi

  go list -f "{{context.ReleaseTags}}" | grep -q -v "go$modVersion\b"
}

# For each module, build the code with local google-cloud-go changes.
for i in $(find . -name go.mod); do
  # internal: this does not need to be built to test compatibility
  # run/events_pubsub: module requires go1.13 for cloudevents
  if [[ $i == *"/internal/"* ]]; then
    continue
  fi
  # internal tooling
  if [[ $i == *"/testing/sampletests/"* ]]; then
    continue
  fi

  pushd $(dirname $i)
  if goVersionShouldSkip; then
    popd
    continue
  fi
  # TODO(codyoss): if we spilt out modules someday we should make this programmatic.
  go mod edit -replace cloud.google.com/go=$gcwd
  go mod edit -replace cloud.google.com/go/bigtable=$gcwd/bigtable
  go mod edit -replace cloud.google.com/go/bigquery=$gcwd/bigquery
  go mod edit -replace cloud.google.com/go/datastore=$gcwd/datastore
  go mod edit -replace cloud.google.com/go/firestore=$gcwd/firestore
  go mod edit -replace cloud.google.com/go/logging=$gcwd/logging
  go mod edit -replace cloud.google.com/go/pubsub=$gcwd/pubsub
  go mod edit -replace cloud.google.com/go/spanner=$gcwd/spanner
  go mod edit -replace cloud.google.com/go/storage=$gcwd/storage
  echo "Building module $i"
  go build ./...
  popd
done
