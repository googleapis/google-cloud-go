#!/bin/bash
# Copyright 2019 Google LLC
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

# This file just exists to list gapics and manuals.

APIS=(
google/api/expr/artman_cel.yaml
google/iam/artman_iam_admin.yaml
google/cloud/asset/artman_cloudasset_v1beta1.yaml
google/cloud/asset/artman_cloudasset_v1p2beta1.yaml
google/cloud/asset/artman_cloudasset_v1.yaml
google/iam/credentials/artman_iamcredentials_v1.yaml
google/cloud/automl/artman_automl_v1beta1.yaml
google/cloud/bigquery/datatransfer/artman_bigquerydatatransfer.yaml
google/cloud/bigquery/storage/artman_bigquerystorage_v1beta1.yaml
google/cloud/dataproc/artman_dataproc_v1.yaml
google/cloud/dataproc/artman_dataproc_v1beta2.yaml
google/cloud/dialogflow/v2/artman_dialogflow_v2.yaml
google/cloud/iot/artman_cloudiot.yaml
google/cloud/irm/artman_irm_v1alpha2.yaml
google/cloud/kms/artman_cloudkms.yaml
google/cloud/language/artman_language_v1.yaml
google/cloud/language/artman_language_v1beta2.yaml
google/cloud/oslogin/artman_oslogin_v1.yaml
google/cloud/oslogin/artman_oslogin_v1beta.yaml
google/cloud/phishingprotection/artman_phishingprotection_v1beta1.yaml
google/cloud/recaptchaenterprise/artman_recaptchaenterprise_v1beta1.yaml
google/cloud/recommender/artman_recommender_v1beta1.yaml
google/cloud/redis/artman_redis_v1beta1.yaml
google/cloud/redis/artman_redis_v1.yaml
google/cloud/scheduler/artman_cloudscheduler_v1beta1.yaml
google/cloud/scheduler/artman_cloudscheduler_v1.yaml
google/cloud/securitycenter/artman_securitycenter_v1beta1.yaml
google/cloud/securitycenter/artman_securitycenter_v1.yaml
google/cloud/speech/artman_speech_v1.yaml
google/cloud/speech/artman_speech_v1p1beta1.yaml
google/cloud/talent/artman_talent_v4beta1.yaml
google/cloud/tasks/artman_cloudtasks_v2beta2.yaml
google/cloud/tasks/artman_cloudtasks_v2beta3.yaml
google/cloud/tasks/artman_cloudtasks_v2.yaml
google/cloud/texttospeech/artman_texttospeech_v1.yaml
google/cloud/videointelligence/artman_videointelligence_v1.yaml
google/cloud/videointelligence/artman_videointelligence_v1beta1.yaml
google/cloud/videointelligence/artman_videointelligence_v1beta2.yaml
google/cloud/vision/artman_vision_v1.yaml
google/cloud/vision/artman_vision_v1p1beta1.yaml
google/cloud/webrisk/artman_webrisk_v1beta1.yaml
google/devtools/artman_clouddebugger.yaml
google/devtools/clouderrorreporting/artman_errorreporting.yaml
google/devtools/cloudtrace/artman_cloudtrace_v1.yaml
google/devtools/cloudtrace/artman_cloudtrace_v2.yaml

# The containeranalysis team wants manual changes in the auto-generated gapic.
# So, let's remove it from the autogen list until we're ready to spend energy
# generating and manually updating it.
# google/devtools/containeranalysis/artman_containeranalysis_v1.yaml

google/devtools/containeranalysis/artman_containeranalysis_v1beta1.yaml
google/firestore/artman_firestore.yaml
google/firestore/admin/artman_firestore_v1.yaml

# See containeranalysis note above.
# grafeas/artman_grafeas_v1.yaml

google/logging/artman_logging.yaml
google/longrunning/artman_longrunning.yaml
google/monitoring/artman_monitoring.yaml
google/privacy/dlp/artman_dlp_v2.yaml
google/pubsub/artman_pubsub.yaml
google/spanner/admin/database/artman_spanner_admin_database.yaml
google/spanner/admin/instance/artman_spanner_admin_instance.yaml
google/spanner/artman_spanner.yaml
)

HAS_MANUAL=(
errorreporting/apiv1beta1
firestore/apiv1beta1
firestore/apiv1
logging/apiv2
longrunning/autogen
pubsub/apiv1
spanner/apiv1
trace/apiv1
)