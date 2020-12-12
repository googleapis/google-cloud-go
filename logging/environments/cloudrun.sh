# GCP permissions needed:
# - cloudbuild.googleapis.com
# - run.googleapis.com
# - storage.buckets.create for CloudBuild service account

# TODO error handling for all the things
set -eo pipefail
set -ex

GCR_REGION=us-west1
GCR_SERVICE=logging-gcr-test

scaffold() {
    # Gets folder containing this running script
    SCRIPT_DIR=$(realpath $(dirname "$0"))
    ROOT_DIR=$SCRIPT_DIR/../..

    # Temporarily move Dockerfile into the local library build context
    cd $ROOT_DIR
    cp ./logging/environments/Dockerfile .

    gcloud builds submit --tag gcr.io/${GCLOUD_TESTS_GOLANG_PROJECT_ID}/$GCR_SERVICE
    rm Dockerfile

    gcloud run deploy --image gcr.io/${GCLOUD_TESTS_GOLANG_PROJECT_ID}/$GCR_SERVICE \
        --platform managed \
        --region $GCR_REGION \
        --allow-unauthenticated \
        $GCR_SERVICE
}

# Deletes GCR service and container image
teardown() {
    gcloud run services delete $GCR_SERVICE \
        --platform managed \
        --region $GCR_REGION \
        --quiet || true
    
    gcloud container images delete gcr.io/${GCLOUD_TESTS_GOLANG_PROJECT_ID}/$GCR_SERVICE \
        --force-delete-tags \
        --quiet || true
}

# bash cloudrun.sh $FUNCTION
"$@"