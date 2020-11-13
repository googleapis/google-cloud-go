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

python3 -m pip install --upgrade pip
# Workaround for six 1.15 incompatibility issue.
python3 -m pip install --use-feature=2020-resolver "gcp-docuploader<2019.0.0"

cd github/google-cloud-go/internal/godocfx
go install
cd -

export GOOGLE_APPLICATION_CREDENTIALS=$KOKORO_KEYSTORE_DIR/72523_go_integration_service_account
# Keep GCLOUD_TESTS_GOLANG_PROJECT_ID in sync with continuous.sh.
export GCLOUD_TESTS_GOLANG_PROJECT_ID=dulcet-port-762

if [[ -n "$FORCE_GENERATE_ALL" ]]; then
  for m in $(find . -name go.mod -execdir go list -m \; | grep -v internal); do
    godocfx -out obj/api/$m@latest $m
  done
else
  godocfx -project $GCLOUD_TESTS_GOLANG_PROJECT_ID -new-modules cloud.google.com/go
fi

for f in $(find obj/api -name docs.metadata); do
  d=$(dirname $f)
  cd $d
  python3 -m docuploader upload \
    --staging-bucket docs-staging-v2 \
    --destination-prefix docfx \
    --credentials "$KOKORO_KEYSTORE_DIR/73713_docuploader_service_account" \
    .
  cd -
done
