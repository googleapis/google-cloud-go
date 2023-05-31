#!/bin/bash

# Fail on any error.
set -eo pipefail

# Install jq
apt install jq zip -y

# Start the releasetool reporter
requirementsFile=$(realpath $(dirname "$0"))/requirements.txt
python3 -m pip install --require-hashes -r $requirementsFile
python3 -m releasetool publish-reporter-script > /tmp/publisher-script; source /tmp/publisher-script

cd github/google-cloud-go/

# Create a directory for storing all the artifacts 
mkdir pkg

# Test prechecks
if [ -z "${AUTORELEASE_PR}" ]
then
  echo "Need to provide URL to release PR via AUTORELEASE_PR environment variable"
  exit 1
fi

# Extract the PR number from the AUTORELEASE_PR variable
get_pr_number () {
  echo "$(basename "$1")"
}

# Get the PR number 
pr_number=$(get_pr_number $AUTORELEASE_PR)

# Returns the list of modules released in the PR. 
release_modules () {
  echo $(curl -s https://api.github.com/repos/googleapis/google-cloud-go/pulls/$1/files | \
  jq -r 'map(select(.filename == ".release-please-manifest-individual.json" or .filename == ".release-please-manifest-submodules.json") | .patch)[0]' | \
  awk -v RS='\n' '/^[+]/{print substr($2,2, length($2)-3)}')
}

# Create a zip file for each released module and store it in the pkg/
# directory to be used as an artifact
release_modules $pr_number | while read -r module
do
  zip -rq "pkg/$module.zip" "$module" >/dev/null
done

# Store the commit hash in a txt as an artifact.
echo -e $KOKORO_GITHUB_COMMIT >> pkg/commit.txt

# Test!
sample_pr_1="https://github.com/googleapis/google-cloud-go/pull/7687"
sample_pr_2="https://github.com/googleapis/google-cloud-go/pull/7701"

pr_number_1=$(get_pr_number $sample_pr_1)
pr_number_2=$(get_pr_number $sample_pr_2)

if [ "$pr_number_1" != "7687" ] ; then
  echo "Error: Incorrect value from get_pr_number."
  exit
fi

if [ "$(release_modules $pr_number_1)" != "aiplatform appengine compute contactcenterinsights container iap retail security workstations" ] ; then
  echo "Error: Incorrect value from release_modules."
  exit
fi

if [ "$(release_modules $pr_number_2)" != "bigquery" ] ; then
  echo "Error: Incorrect value from release_modules."
  exit
fi
