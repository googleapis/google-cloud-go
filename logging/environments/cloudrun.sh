# GCP permissions needed:
# - cloudbuild.googleapis.com
# - run.googleapis.com
# - storage.buckets.create for CloudBuild service account

# Fail on any error.
set -eo pipefail

# Display commands being run.
set -ex

# Gets folder containing this running script
SCRIPT_DIR=$(realpath $(dirname "$0"))
ROOT_DIR=$SCRIPT_DIR/../..

# Temporarily move Dockerfile into the local library build context
cd $ROOT_DIR
cp ./logging/environments/Dockerfile .

gcloud builds submit --tag gcr.io/${GCLOUD_TESTS_GOLANG_PROJECT_ID}/logging-gcr-test
gcloud run deploy --image gcr.io/${GCLOUD_TESTS_GOLANG_PROJECT_ID}/logging-gcr-test \
    --platform managed \
    --region us-west1 \
    --allow-unauthenticated \
    logging-gcr-test

# TODO: cleanup steps above, should restore state completely
cd $ROOT_DIR
rm Dockerfile
