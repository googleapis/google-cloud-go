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

# This script generates all GAPIC clients in this repo.
# See instructions at go/yoshi-site.

set -ex

GOCLOUD_DIR="$(dirname "$0")"
source "$GOCLOUD_DIR/gapics.sh"

for api in "${APIS[@]}"; do
  rm -rf artman-genfiles/*
  artman --config "$api" generate go_gapic
  cp -r artman-genfiles/gapi-*/cloud.google.com/go/* $GOCLOUD_DIR
done

pushd $GOCLOUD_DIR
  gofmt -s -d -l -w . && goimports -w .

  # NOTE(pongad): `sed -i` doesn't work on Macs, because -i option needs an argument.
  # `-i ''` doesn't work on GNU, since the empty string is treated as a file name.
  # So we just create the backup and delete it after.
  ver=$(date +%Y%m%d)
  git ls-files -mo | while read modified; do
    dir=${modified%/*.*}
    find . -path "*/$dir/doc.go" -exec sed -i.backup -e "s/^const versionClient.*/const versionClient = \"$ver\"/" '{}' +
  done
popd

for dir in "${HAS_MANUAL[@]}"; do
	find "$GOCLOUD_DIR/$dir" -name '*.go' -exec sed -i.backup -e 's/setGoogleClientInfo/SetGoogleClientInfo/g' '{}' '+'
done

find $GOCLOUD_DIR -name '*.backup' -delete
