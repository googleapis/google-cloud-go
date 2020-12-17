#!/bin/bash -ex

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
# TODO make this UUID
GCR_SERVICE=logging-gcr-test
PROJECT_ID=log-bench
TOPIC_ID=$2
SUB_ID=$3

scaffold() {    
    # Gets folder containing this running script
    SCRIPT_DIR=$(realpath $(dirname "$0"))  # e2e/.
    ROOT_DIR=$SCRIPT_DIR/../..              # google-cloud-go/.

    # Handle existing pubsub topic
    # TODO move it out of this bash script, since topicID is created on test run start
    missing=$(gcloud pubsub topics describe $TOPIC_ID 2>&1) || true
    if [[ "$missing" == *"NOT_FOUND"* ]]; then
        gcloud pubsub topics create $1
    else
        echo TOPIC FOUND
    fi

    # Temporarily move Dockerfile into the local library build context
    cd $ROOT_DIR
    cp ./logging/e2e/Dockerfile .

    # gcloud builds submit --tag gcr.io/${GCLOUD_TESTS_GOLANG_PROJECT_ID}/$GCR_SERVICE
    # rm Dockerfile

    # gcloud run deploy --image gcr.io/${GCLOUD_TESTS_GOLANG_PROJECT_ID}/$GCR_SERVICE \
    #     --platform managed \
    #     --region $GCR_REGION \
    #     --allow-unauthenticated \
    #     --set-env-vars SUB_ID=$SUB_ID \
    #     $GCR_SERVICE
    
    PROJECT_NUMBER=$(gcloud projects describe log-bench --format 'value(projectNumber)')
    # Allow Pub/Sub to create authentication tokens in your project:
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
    gcloud run services add-iam-policy-binding $GCR_SERVICE \
        --member=serviceAccount:$SERVICEACCOUNT_NAME@$PROJECT_ID.iam.gserviceaccount.com \
        --role=roles/run.invoker \
        --region $GCR_REGION \
        --platform managed

    GCR_URL=$(gcloud run services describe logging-gcr-test --platform managed --region us-west1 --format 'value(status.url)')
    # Handle pubsub subscription creation which pushes to the SA
    missing=$(gcloud pubsub subscriptions describe $SUB_ID 2>&1) || true
    if [[ "$missing" == *"NOT_FOUND"* ]]; then
        gcloud pubsub subscriptions create $SUB_ID \
            --topic $TOPIC_ID \
            --topic-project $PROJECT_ID \
            --push-endpoint $GCR_URL \
            --push-auth-service-account $SERVICEACCOUNT_NAME@$PROJECT_ID.iam.gserviceaccount.com
    else
        echo subscription found
    fi

    pwd
    # TODO pop back to prev
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
    
    # TODO destroy subscriptions on the topic
    gcloud pubsub subscriptions delete $SUB_ID \
        --quiet || true
    pwd
}

# bash cloudrun.sh $FUNCTION
"$@"