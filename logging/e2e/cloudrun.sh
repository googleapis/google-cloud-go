#!/bin/bash -ex

# Copyright 2020 Google Inc. All Rights Reserved.
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
    gcloud builds submit --tag gcr.io/${GCLOUD_TESTS_GOLANG_PROJECT_ID}/$TOPIC_ID
    rm Dockerfile

    # Build test image to Cloud Run
    gcloud run deploy --image gcr.io/${GCLOUD_TESTS_GOLANG_PROJECT_ID}/$TOPIC_ID \
        --platform managed \
        --region $GCR_REGION \
        --allow-unauthenticated \
        --set-env-vars TOPIC_ID=$TOPIC_ID \
        $TOPIC_ID

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
    gcloud run services add-iam-policy-binding $TOPIC_ID \
        --member=serviceAccount:$SERVICEACCOUNT_NAME@$PROJECT_ID.iam.gserviceaccount.com \
        --role=roles/run.invoker \
        --region $GCR_REGION \
        --platform managed

    # Create pubsub subscription which pushes messages to the service account
    GCR_URL=$(gcloud run services describe $TOPIC_ID --platform managed --region us-west1 --format 'value(status.url)')
    missing=$(gcloud pubsub subscriptions describe $TOPIC_ID 2>&1) || true
    if [[ "$missing" == *"NOT_FOUND"* ]]; then
        gcloud pubsub subscriptions create $TOPIC_ID \
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
    gcloud container images delete gcr.io/${GCLOUD_TESTS_GOLANG_PROJECT_ID}/$TOPIC_ID \
        --force-delete-tags \
        --quiet || true
    
    gcloud pubsub subscriptions delete $TOPIC_ID \
        --quiet || true

    gcloud run services delete $TOPIC_ID \
        --platform managed \
        --region $GCR_REGION \
        --quiet || true
}

# bash cloudrun.sh $FUNCTION
"$@"