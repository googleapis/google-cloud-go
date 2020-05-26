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

export GITHUB_ACCESS_TOKEN=$(cat "${KOKORO_KEYSTORE_DIR}/73713_yoshi-automation-github-key")
export GITHUB_NAME="Yoshi Automation Bot"
export GITHUB_EMAIL="yoshi-automation@google.com"
export GERRIT_COOKIE_VALUE=$(cat "${KOKORO_KEYSTORE_DIR}/72523_go_gerrit_account_password")

# Init git creds.
echo "https://$GITHUB_USERNAME:$GITHUB_ACCESS_TOKEN@example.com" > ~/.git-credentials

# Fail on any error.
set -eo

# Display commands being run.
set -x

# cd to gapicgen dir on Kokoro instance.
cd git/gocloud/internal/gapicgen

go get \
    golang.org/x/lint/golint \
    golang.org/x/tools/cmd/goimports \
    honnef.co/go/tools/cmd/staticcheck \
    golang.org/x/review/git-codereview

go run cloud.google.com/go/internal/gapicgen/cmd/genmgr \
    --githubAccessToken="$GITHUB_ACCESS_TOKEN" \
    --githubName="$GITHUB_NAME" \
    --githubEmail="$GITHUB_EMAIL" \
    --gerritCookieValue="$GERRIT_COOKIE_VALUE"
