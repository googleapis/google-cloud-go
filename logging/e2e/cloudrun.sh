#!/bin/bash -ex
# TODO, remove the ex
# GCP permissions needed:
# APIs
# - cloudbuild.googleapis.com
# - run.googleapis.com
# ROLES
# - enable pub/sub to create authentication tokens   
# - storage.buckets.create for CloudBuild service account

set -ex
set -eo pipefail

GCR_REGION=us-west1
PROJECT_ID=$(gcloud config get-value project 2> /dev/null)
TOPIC_ID=$2
TEST_ID=$3

scaffold() {
    # Gets folder containing this running script
    SCRIPT_DIR=$(realpath $(dirname "$0"))  # e2e/.
    ROOT_DIR=$SCRIPT_DIR/../..              # google-cloud-go/.

    # Handle if e2e_test didn't create topic
    missing=$(gcloud pubsub topics describe $TOPIC_ID 2>&1) || true
    if [[ "$missing" == *"NOT_FOUND"* ]]; then
        gcloud pubsub topics create $TOPIC_ID
    else
        echo TOPIC FOUND
    fi

    # Temporarily move Dockerfile into the local library build context
    cd $ROOT_DIR
    cp ./logging/e2e/Dockerfile .

    # Build latest test app image
    gcloud builds submit --tag gcr.io/${GCLOUD_TESTS_GOLANG_PROJECT_ID}/$TEST_ID
    rm Dockerfile

    # Build test image to Cloud Run
    gcloud run deploy --image gcr.io/${GCLOUD_TESTS_GOLANG_PROJECT_ID}/$TEST_ID \
        --platform managed \
        --region $GCR_REGION \
        --allow-unauthenticated \
        --set-env-vars TEST_ID=$TEST_ID \
        $TEST_ID

    # Allow Pub/Sub to create authentication tokens in your project:
    PROJECT_NUMBER=$(gcloud projects describe log-bench --format 'value(projectNumber)')
    gcloud projects add-iam-policy-binding $PROJECT_ID \
     --member=serviceAccount:service-$PROJECT_NUMBER@gcp-sa-pubsub.iam.gserviceaccount.com \
     --role=roles/iam.serviceAccountTokenCreator

    # Create a service account (to enable pubsub subscription pushes) if it doesn't exist
    # Service accounts are not deleted by the test run
    SERVICEACCOUNT_NAME=logging-cloudrun-pubsubinvoker
    missing=$(gcloud iam service-accounts describe $SERVICEACCOUNT_NAME@$PROJECT_ID.iam.gserviceaccount.com 2>&1) || true
    if [[ "$missing" == *"NOT_FOUND"* ]]; then
        gcloud iam service-accounts create $SERVICEACCOUNT_NAME \
        --display-name "Logging CloudRun PubSub Invoker"
    fi
    
    # Give service account permission to invoke pubsub service
    gcloud run services add-iam-policy-binding $TEST_ID \
        --member=serviceAccount:$SERVICEACCOUNT_NAME@$PROJECT_ID.iam.gserviceaccount.com \
        --role=roles/run.invoker \
        --region $GCR_REGION \
        --platform managed

    # Create pubsub subscription which pushes messages to the service account
    GCR_URL=$(gcloud run services describe $TEST_ID --platform managed --region us-west1 --format 'value(status.url)')
    missing=$(gcloud pubsub subscriptions describe $TEST_ID 2>&1) || true
    if [[ "$missing" == *"NOT_FOUND"* ]]; then
        gcloud pubsub subscriptions create $TEST_ID \
            --topic $TOPIC_ID \
            --topic-project $PROJECT_ID \
            --push-endpoint $GCR_URL \
            --push-auth-service-account $SERVICEACCOUNT_NAME@$PROJECT_ID.iam.gserviceaccount.com
    else
        echo subscription found
    fi
}

# Deletes GCR service and container image
teardown() {
    gcloud container images delete gcr.io/${GCLOUD_TESTS_GOLANG_PROJECT_ID}/$TEST_ID \
        --force-delete-tags \
        --quiet || true
    
    gcloud pubsub subscriptions delete $TEST_ID \
        --quiet || true

    gcloud run services delete $TEST_ID \
        --platform managed \
        --region $GCR_REGION \
        --quiet || true
}

# bash cloudrun.sh $FUNCTION
"$@"