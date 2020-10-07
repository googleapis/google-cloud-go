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

if [ -z "$MODULE" ] ; then
    echo "Must set the MODULE environment variables"
    exit 1
fi

python3 -m pip install --upgrade pip
# Workaround for six 1.15 incompatibility issue.
python3 -m pip install --use-feature=2020-resolver "gcp-docuploader<2019.0.0"

cd github/google-cloud-go/internal/godocfx
go install
cd -

if [ -z "$VERSION" ] ; then
    VERSION="latest"
fi

cd $(mktemp -d)

# Create a module and get the module@version being asked for.
go mod init cloud.google.com/lets/build/some/docs
go get "$MODULE@$VERSION"

# Generate the YAML and a docs.metadata file.
godocfx "$MODULE/..."

cd obj/api || exit 4

python3 -m docuploader upload \
  --staging-bucket docs-staging-v2 \
  --destination-prefix docfx \
  --credentials "$KOKORO_KEYSTORE_DIR/73713_docuploader_service_account" \
  .
