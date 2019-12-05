// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package generator

// microgenConfig represents a single microgen target.
type microgenConfig struct {
	// inputDirectoryPath is the path to the input (.proto, etc) files, relative
	// to googleapisDir.
	inputDirectoryPath string

	// importPath is the path that this library should be imported as.
	importPath string

	// pkg is the name that should be used in the package declaration.
	pkg string

	// gRPCServiceConfigPath is the path to the grpc service config for this
	// target, relative to googleapisDir.
	gRPCServiceConfigPath string

	// apiServiceConfigPath is the path to the gapic service config for this
	// target, relative to googleapisDir.
	apiServiceConfigPath string

	// releaseLevel is the release level of this target. Values incl ga,
	// beta, alpha.
	releaseLevel string
}

var microgenGapicConfigs = []*microgenConfig{
	{
		inputDirectoryPath:    "google/cloud/texttospeech/v1",
		pkg:                   "texttospeech",
		importPath:            "cloud.google.com/go/texttospeech/apiv1",
		gRPCServiceConfigPath: "google/cloud/texttospeech/v1/texttospeech_grpc_service_config.json",
		apiServiceConfigPath:  "google/cloud/texttospeech/v1/texttospeech_v1.yaml",
		releaseLevel:          "alpha",
	},
	{
		inputDirectoryPath:    "google/cloud/asset/v1",
		pkg:                   "asset",
		importPath:            "cloud.google.com/go/asset/apiv1",
		gRPCServiceConfigPath: "google/cloud/asset/v1/cloudasset_grpc_service_config.json",
		apiServiceConfigPath:  "google/cloud/asset/v1/cloudasset_v1.yaml",
		releaseLevel:          "alpha",
	},
	{
		inputDirectoryPath:    "google/cloud/language/v1",
		pkg:                   "language",
		importPath:            "cloud.google.com/go/language/apiv1",
		gRPCServiceConfigPath: "google/cloud/language/v1/language_grpc_service_config.json",
		apiServiceConfigPath:  "google/cloud/language/language_v1.yaml",
		releaseLevel:          "alpha",
	},
	{
		inputDirectoryPath:    "google/cloud/phishingprotection/v1beta1",
		pkg:                   "phishingprotection",
		importPath:            "cloud.google.com/go/phishingprotection/apiv1beta1",
		gRPCServiceConfigPath: "google/cloud/phishingprotection/v1beta1/phishingprotection_grpc_service_config.json",
		apiServiceConfigPath:  "google/cloud/phishingprotection/v1beta1/phishingprotection_v1beta1.yaml",
		releaseLevel:          "beta",
	},
	{
		inputDirectoryPath:    "google/cloud/translate/v3",
		pkg:                   "translate",
		importPath:            "cloud.google.com/go/translate/apiv3",
		gRPCServiceConfigPath: "google/cloud/translate/v3/translate_grpc_service_config.json",
		apiServiceConfigPath:  "google/cloud/translate/v3/translate_v3.yaml",
		releaseLevel:          "ga",
	},
	{
		inputDirectoryPath:    "google/cloud/scheduler/v1",
		pkg:                   "scheduler",
		importPath:            "cloud.google.com/go/scheduler/apiv1",
		gRPCServiceConfigPath: "google/cloud/scheduler/v1/cloudscheduler_grpc_service_config.json",
		apiServiceConfigPath:  "google/cloud/scheduler/v1/cloudscheduler_v1.yaml",
		releaseLevel:          "ga",
	},
	{
		inputDirectoryPath:    "google/cloud/scheduler/v1beta1",
		pkg:                   "scheduler",
		importPath:            "cloud.google.com/go/scheduler/apiv1beta1",
		gRPCServiceConfigPath: "google/cloud/scheduler/v1beta1/cloudscheduler_grpc_service_config.json",
		apiServiceConfigPath:  "google/cloud/scheduler/v1beta1/cloudscheduler_v1beta1.yaml",
		releaseLevel:          "beta",
	},
	{
		inputDirectoryPath:    "google/cloud/speech/v1",
		pkg:                   "speech",
		importPath:            "cloud.google.com/go/speech/apiv1",
		gRPCServiceConfigPath: "google/cloud/speech/v1/speech_grpc_service_config.json",
		apiServiceConfigPath:  "google/cloud/speech/v1/speech_v1.yaml",
		releaseLevel:          "ga",
	},
	{
		inputDirectoryPath:    "google/cloud/speech/v1p1beta1",
		pkg:                   "speech",
		importPath:            "cloud.google.com/go/speech/apiv1p1beta1",
		gRPCServiceConfigPath: "google/cloud/speech/v1p1beta1/speech_grpc_service_config.json",
		apiServiceConfigPath:  "google/cloud/speech/v1p1beta1/speech_v1p1beta1.yaml",
		releaseLevel:          "beta",
	},
	{
		inputDirectoryPath:    "google/cloud/bigquery/datatransfer/v1",
		pkg:                   "datatransfer",
		importPath:            "cloud.google.com/go/bigquery/datatransfer/apiv1",
		gRPCServiceConfigPath: "google/cloud/bigquery/datatransfer/v1/bigquerydatatransfer_grpc_service_config.json",
		apiServiceConfigPath:  "google/cloud/bigquery/datatransfer/v1/bigquerydatatransfer_v1.yaml",
		releaseLevel:          "alpha",
	},
	{
		inputDirectoryPath:    "google/cloud/bigquery/storage/v1beta1",
		pkg:                   "storage",
		importPath:            "cloud.google.com/go/bigquery/storage/apiv1beta1",
		gRPCServiceConfigPath: "google/cloud/bigquery/storage/v1beta1/bigquerystorage_grpc_service_config.json",
		apiServiceConfigPath:  "google/cloud/bigquery/storage/v1beta1/bigquerystorage_v1beta1.yaml",
		releaseLevel:          "beta",
	},
	{
		inputDirectoryPath:    "google/cloud/iot/v1",
		pkg:                   "iot",
		importPath:            "cloud.google.com/go/iot/apiv1",
		gRPCServiceConfigPath: "google/cloud/iot/v1/cloudiot_grpc_service_config.json",
		apiServiceConfigPath:  "google/cloud/iot/v1/cloudiot_v1.yaml",
		releaseLevel:          "ga",
	},
	{
		inputDirectoryPath:    "google/cloud/recommender/v1beta1",
		pkg:                   "recommender",
		importPath:            "cloud.google.com/go/recommender/apiv1beta1",
		gRPCServiceConfigPath: "google/cloud/recommender/v1beta1/recommender_grpc_service_config.json",
		apiServiceConfigPath:  "google/cloud/recommender/v1beta1/recommender_v1beta1.yaml",
		releaseLevel:          "beta",
	},
	{
		inputDirectoryPath:    "google/cloud/tasks/v2",
		pkg:                   "cloudtasks",
		importPath:            "cloud.google.com/go/cloudtasks/apiv2",
		gRPCServiceConfigPath: "google/cloud/tasks/v2/cloudtasks_grpc_service_config.json",
		apiServiceConfigPath:  "google/cloud/tasks/v2/cloudtasks_v2.yaml",
		releaseLevel:          "ga",
	},
	{
		inputDirectoryPath:    "google/cloud/tasks/v2beta2",
		pkg:                   "cloudtasks",
		importPath:            "cloud.google.com/go/cloudtasks/apiv2beta2",
		gRPCServiceConfigPath: "google/cloud/tasks/v2beta2/cloudtasks_grpc_service_config.json",
		apiServiceConfigPath:  "google/cloud/tasks/v2beta2/cloudtasks_v2beta2.yaml",
		releaseLevel:          "beta",
	},
	{
		inputDirectoryPath:    "google/cloud/tasks/v2beta3",
		pkg:                   "cloudtasks",
		importPath:            "cloud.google.com/go/cloudtasks/apiv2beta3",
		gRPCServiceConfigPath: "google/cloud/tasks/v2beta3/cloudtasks_grpc_service_config.json",
		apiServiceConfigPath:  "google/cloud/tasks/v2beta3/cloudtasks_v2beta3.yaml",
		releaseLevel:          "beta",
	},
	{
		inputDirectoryPath:    "google/cloud/videointelligence/v1",
		pkg:                   "videointelligence",
		importPath:            "cloud.google.com/go/videointelligence/apiv1",
		gRPCServiceConfigPath: "google/cloud/videointelligence/v1/videointelligence_grpc_service_config.json",
		apiServiceConfigPath:  "google/cloud/videointelligence/v1/videointelligence_v1.yaml",
		releaseLevel:          "alpha",
	},
	{
		inputDirectoryPath:    "google/cloud/vision/v1",
		pkg:                   "vision",
		importPath:            "cloud.google.com/go/vision/apiv1",
		gRPCServiceConfigPath: "google/cloud/vision/v1/vision_grpc_service_config.json",
		apiServiceConfigPath:  "google/cloud/vision/v1/vision_v1.yaml",
		releaseLevel:          "ga",
	},
	{
		inputDirectoryPath:    "google/cloud/webrisk/v1beta1",
		pkg:                   "webrisk",
		importPath:            "cloud.google.com/go/webrisk/apiv1beta1",
		gRPCServiceConfigPath: "google/cloud/webrisk/v1beta1/webrisk_grpc_service_config.json",
		apiServiceConfigPath:  "google/cloud/webrisk/v1beta1/webrisk_v1beta1.yaml",
		releaseLevel:          "beta",
	},
	{
		inputDirectoryPath:    "google/cloud/secrets/v1beta1",
		pkg:                   "secretmanager",
		importPath:            "cloud.google.com/go/secretmanager/apiv1beta1",
		gRPCServiceConfigPath: "google/cloud/secrets/v1beta1/secretmanager_grpc_service_config.json",
		apiServiceConfigPath:  "google/cloud/secrets/v1beta1/secretmanager_v1beta1.yaml",
		releaseLevel:          "beta",
	},
}

// Relative to gocloud dir.
var gapicsWithManual = []string{
	"errorreporting/apiv1beta1",
	"firestore/apiv1beta1",
	"firestore/apiv1",
	"logging/apiv2",
	"longrunning/autogen",
	"pubsub/apiv1",
	"spanner/apiv1",
	"trace/apiv1",
}

// Relative to googleapis dir.
var artmanGapicConfigPaths = []string{
	"google/api/expr/artman_cel.yaml",
	"google/cloud/asset/artman_cloudasset_v1beta1.yaml",
	"google/cloud/asset/artman_cloudasset_v1p2beta1.yaml",
	"google/iam/credentials/artman_iamcredentials_v1.yaml",
	"google/cloud/automl/artman_automl_v1.yaml",
	"google/cloud/automl/artman_automl_v1beta1.yaml",
	"google/cloud/dataproc/artman_dataproc_v1.yaml",
	"google/cloud/dataproc/artman_dataproc_v1beta2.yaml",
	"google/cloud/dialogflow/v2/artman_dialogflow_v2.yaml",
	"google/cloud/irm/artman_irm_v1alpha2.yaml",
	"google/cloud/kms/artman_cloudkms.yaml",
	"google/cloud/language/artman_language_v1beta2.yaml",
	"google/cloud/oslogin/artman_oslogin_v1.yaml",
	"google/cloud/oslogin/artman_oslogin_v1beta.yaml",
	"google/cloud/recaptchaenterprise/artman_recaptchaenterprise_v1beta1.yaml",
	"google/cloud/redis/artman_redis_v1beta1.yaml",
	"google/cloud/redis/artman_redis_v1.yaml",
	"google/cloud/securitycenter/artman_securitycenter_v1beta1.yaml",
	"google/cloud/securitycenter/artman_securitycenter_v1.yaml",
	"google/cloud/talent/artman_talent_v4beta1.yaml",
	"google/cloud/videointelligence/artman_videointelligence_v1beta2.yaml",
	"google/cloud/vision/artman_vision_v1p1beta1.yaml",
	"google/devtools/artman_clouddebugger.yaml",
	"google/devtools/cloudbuild/artman_cloudbuild.yaml",
	"google/devtools/clouderrorreporting/artman_errorreporting.yaml",
	"google/devtools/cloudtrace/artman_cloudtrace_v1.yaml",
	"google/devtools/cloudtrace/artman_cloudtrace_v2.yaml",
	"google/devtools/containeranalysis/artman_containeranalysis_v1beta1.yaml",
	"google/firestore/artman_firestore.yaml",
	"google/firestore/admin/artman_firestore_v1.yaml",
	"google/logging/artman_logging.yaml",
	"google/longrunning/artman_longrunning.yaml",
	"google/monitoring/artman_monitoring.yaml",
	"google/privacy/dlp/artman_dlp_v2.yaml",
	"google/pubsub/artman_pubsub.yaml",
	"google/spanner/admin/database/artman_spanner_admin_database.yaml",
	"google/spanner/admin/instance/artman_spanner_admin_instance.yaml",
	"google/spanner/artman_spanner.yaml",
}
