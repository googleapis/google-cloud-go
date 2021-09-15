#!/bin/bash
# Copyright 2021 Google LLC
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

# A suite of tests offered by https://github.com/googleapis/env-tests-logging
# That allows deploying and testing features in live GCP services.
# Currently only configured to test logging & error reporting

set -eo pipefail

# Test prechecks
if [[ -z "${ENVIRONMENT:-}" ]]; then
  echo "ENVIRONMENT not set. Exiting"
  exit 1
fi

if [[ -z "${PROJECT_ROOT:-}"  ]]; then
    PROJECT_ROOT="github/google-cloud-go"
fi

# Add the test module as a submodule to the repo.
cd "${KOKORO_ARTIFACTS_DIR}/github/google-cloud-go/internal/"
git submodule add https://github.com/googleapis/env-tests-logging
cd "env-tests-logging/"

# Disable buffering, so that the logs stream through.
export PYTHONUNBUFFERED=1

# Debug: show build environment
env | grep KOKORO

# Set up service account credentials
export GOOGLE_APPLICATION_CREDENTIALS=$KOKORO_KEYSTORE_DIR/72523_go_integration_service_account
gcloud auth activate-service-account --key-file=$GOOGLE_APPLICATION_CREDENTIALS

set -x
export PROJECT_ID="dulcet-port-762"
gcloud config set project $PROJECT_ID
gcloud config set compute/zone us-central1-b

# Authenticate docker
gcloud auth configure-docker -q

# Nox tests require Python 3.7, instead of 3.8
pyenv global 3.7.10
python3 -m pip uninstall --yes --quiet nox-automation
python3 -m pip install --upgrade --quiet nox
python3 -m nox --version

# create a unique id for this run
UUID=$(python  -c 'import uuid; print(uuid.uuid1())' | head -c 7)
export ENVCTL_ID=ci-$UUID
echo $ENVCTL_ID

# If App Engine, install app-engine-go component
if [[ $ENVIRONMENT == *"appengine"* ]]; then
  apt-get install google-cloud-sdk-app-engine-go -y | cat
fi

# If Kubernetes, install kubectl component
if [[ $ENVIRONMENT == *"kubernetes"* ]]; then
  curl -LO "https://dl.k8s.io/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
  chmod +x kubectl
  mkdir -p ~/.local/bin
  mv ./kubectl ~/.local/bin
  export PATH=$PATH:~/.local/bin/
fi

# Run the environment test for the specified GCP service
set +e
python3.7 -m nox --session "tests(language='go', platform='$ENVIRONMENT')"
TEST_STATUS_CODE=$?

# destroy resources
echo "cleaning up..."
./envctl/envctl go $ENVIRONMENT destroy

# exit with proper status code
exit $TEST_STATUS_CODE
