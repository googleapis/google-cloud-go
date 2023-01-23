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

package modconfig

// A ModuleConfig represents a module that requires minimum required files
// TODO: more info in this doc about minimum required files
type ModuleConfig struct {
	// ClientShortName is the path fragment between google/cloud and
	// the version number in the path for the API client in the
	// googleapis/googleapis repo
	ClientShortName string
	ImportPath      string
	ClientIsModule  bool
}

var ModuleConfigs = []*ModuleConfig{
	{
		ImportPath: "cloud.google.com/go/accessapproval",
	},
	{
		ImportPath: "cloud.google.com/go/accesscontextmanager",
	},
	{
		ImportPath: "cloud.google.com/go/aiplatform",
	},
	{
		ImportPath: "cloud.google.com/go/analytics",
	},
	{
		ImportPath: "cloud.google.com/go/apigateway",
	},
	{
		ImportPath: "cloud.google.com/go/apigeeconnect",
	},
	{
		ImportPath: "cloud.google.com/go/apigeeregistry",
	},
	{
		ImportPath: "cloud.google.com/go/apikeys",
	},
	{
		ImportPath: "cloud.google.com/go/appengine",
	},
	{
		ImportPath: "cloud.google.com/go/area120",
	},
	{
		ImportPath: "cloud.google.com/go/artifactregistry",
	},
	{
		ImportPath: "cloud.google.com/go/asset",
	},
	{
		ImportPath: "cloud.google.com/go/assuredworkloads",
	},
	{
		ImportPath: "cloud.google.com/go/automl",
	},
	{
		ImportPath: "cloud.google.com/go/baremetalsolution",
	},
	{
		ImportPath: "cloud.google.com/go/batch",
	},
	{
		ImportPath: "cloud.google.com/go/beyondcorp",
	},
	{
		ImportPath: "cloud.google.com/go/bigquery",
	},
	{
		ImportPath: "cloud.google.com/go/bigtable",
	},
	{
		ImportPath: "cloud.google.com/go/billing",
	},
	{
		ImportPath: "cloud.google.com/go/binaryauthorization",
	},
	{
		ImportPath: "cloud.google.com/go/certificatemanager",
	},
	{
		ImportPath: "cloud.google.com/go/channel",
	},
	{
		ImportPath: "cloud.google.com/go/cloudbuild",
	},
	{
		ImportPath: "cloud.google.com/go/clouddms",
	},
	{
		ImportPath: "cloud.google.com/go/cloudtasks",
	},
	{
		ImportPath: "cloud.google.com/go/compute",
	},
	{
		ImportPath: "cloud.google.com/go/contactcenterinsights",
	},
	{
		ImportPath: "cloud.google.com/go/container",
	},
	{
		ImportPath: "cloud.google.com/go/containeranalysis",
	},
	{
		ImportPath: "cloud.google.com/go/datacatalog",
	},
	{
		ImportPath: "cloud.google.com/go/dataflow",
	},
	{
		ImportPath: "cloud.google.com/go/dataform",
	},
	{
		ImportPath: "cloud.google.com/go/datafusion",
	},
	{
		ImportPath: "cloud.google.com/go/datalabeling",
	},
	{
		ImportPath: "cloud.google.com/go/dataplex",
	},
	{
		ImportPath: "cloud.google.com/go/dataproc",
	},
	{
		ImportPath: "cloud.google.com/go/dataqna",
	},
	{
		ImportPath: "cloud.google.com/go/datastore",
	},
	{
		ImportPath: "cloud.google.com/go/datastream",
	},
	{
		ImportPath: "cloud.google.com/go/deploy",
	},
	{
		ImportPath: "cloud.google.com/go/dialogflow",
	},
	{
		ImportPath: "cloud.google.com/go/dlp",
	},
	{
		ImportPath: "cloud.google.com/go/documentai",
	},
	{
		ImportPath: "cloud.google.com/go/domains",
	},
	{
		ImportPath: "cloud.google.com/go/edgecontainer",
	},
	{
		ImportPath: "cloud.google.com/go/errorreporting",
	},
	{
		ImportPath: "cloud.google.com/go/essentialcontacts",
	},
	{
		ImportPath: "cloud.google.com/go/eventarc",
	},
	{
		ImportPath: "cloud.google.com/go/filestore",
	},
	{
		ImportPath: "cloud.google.com/go/firestore",
	},
	{
		ImportPath: "cloud.google.com/go/functions",
	},
	{
		ImportPath: "cloud.google.com/go/gaming",
	},
	{
		ImportPath: "cloud.google.com/go/gkebackup",
	},
	{
		ImportPath: "cloud.google.com/go/gkeconnect",
	},
	{
		ImportPath: "cloud.google.com/go/gkehub",
	},
	{
		ImportPath: "cloud.google.com/go/gkemulticloud",
	},
	{
		ImportPath: "cloud.google.com/go/grafeas",
	},
	{
		ImportPath: "cloud.google.com/go/gsuiteaddons",
	},
	{
		ImportPath: "cloud.google.com/go/iam",
	},
	{
		ImportPath: "cloud.google.com/go/iap",
	},
	{
		ImportPath: "cloud.google.com/go/ids",
	},
	{
		ImportPath: "cloud.google.com/go/iot",
	},
	{
		ImportPath: "cloud.google.com/go/kms",
	},
	{
		ImportPath: "cloud.google.com/go/language",
	},
	{
		ImportPath: "cloud.google.com/go/lifesciences",
	},
	{
		ImportPath: "cloud.google.com/go/logging",
	},
	{
		ImportPath: "cloud.google.com/go/longrunning",
	},
	{
		ImportPath: "cloud.google.com/go/managedidentities",
	},
	{
		ImportPath: "cloud.google.com/go/maps",
	},
	{
		ImportPath: "cloud.google.com/go/mediatranslation",
	},
	{
		ImportPath: "cloud.google.com/go/memcache",
	},
	{
		ImportPath: "cloud.google.com/go/metastore",
	},
	{
		ImportPath: "cloud.google.com/go/monitoring",
	},
	{
		ImportPath: "cloud.google.com/go/networkconnectivity",
	},
	{
		ImportPath: "cloud.google.com/go/networkmanagement",
	},
	{
		ImportPath: "cloud.google.com/go/networksecurity",
	},
	{
		ImportPath: "cloud.google.com/go/notebooks",
	},
	{
		ImportPath: "cloud.google.com/go/optimization",
	},
	{
		ImportPath: "cloud.google.com/go/orchestration",
	},
	{
		ImportPath: "cloud.google.com/go/orgpolicy",
	},
	{
		ImportPath: "cloud.google.com/go/osconfig",
	},
	{
		ImportPath: "cloud.google.com/go/oslogin",
	},
	{
		ImportPath: "cloud.google.com/go/phishingprotection",
	},
	{
		ImportPath: "cloud.google.com/go/policytroubleshooter",
	},
	{
		ImportPath: "cloud.google.com/go/privatecatalog",
	},
	{
		ImportPath: "cloud.google.com/go/profiler",
	},
	{
		ImportPath: "cloud.google.com/go/pubsub",
	},
	{
		ImportPath: "cloud.google.com/go/pubsublite",
	},
	{
		ImportPath: "cloud.google.com/go/recaptchaenterprise",
	},
	{
		ImportPath: "cloud.google.com/go/recaptchaenterprise/v2",
	},
	{
		ImportPath: "cloud.google.com/go/recommendationengine",
	},
	{
		ImportPath: "cloud.google.com/go/recommender",
	},
	{
		ImportPath: "cloud.google.com/go/redis",
	},
	{
		ImportPath: "cloud.google.com/go/resourcemanager",
	},
	{
		ImportPath: "cloud.google.com/go/resourcesettings",
	},
	{
		ImportPath: "cloud.google.com/go/retail",
	},
	{
		ImportPath: "cloud.google.com/go/run",
	},
	{
		ImportPath: "cloud.google.com/go/scheduler",
	},
	{
		ImportPath: "cloud.google.com/go/secretmanager",
	},
	{
		ImportPath: "cloud.google.com/go/security",
	},
	{
		ImportPath: "cloud.google.com/go/securitycenter",
	},
	{
		ImportPath: "cloud.google.com/go/servicecontrol",
	},
	{
		ImportPath: "cloud.google.com/go/servicedirectory",
	},
	{
		ImportPath: "cloud.google.com/go/servicemanagement",
	},
	{
		ImportPath: "cloud.google.com/go/serviceusage",
	},
	{
		ImportPath: "cloud.google.com/go/shell",
	},
	{
		ImportPath: "cloud.google.com/go/spanner",
	},
	{
		ImportPath: "cloud.google.com/go/speech",
	},
	{
		ImportPath: "cloud.google.com/go/storagetransfer",
	},
	{
		ImportPath: "cloud.google.com/go/talent",
	},
	{
		ImportPath: "cloud.google.com/go/texttospeech",
	},
	{
		ImportPath: "cloud.google.com/go/tpu",
	},
	{
		ImportPath: "cloud.google.com/go/trace",
	},
	{
		ImportPath: "cloud.google.com/go/translate",
	},
	{
		ImportPath: "cloud.google.com/go/video",
	},
	{
		ImportPath: "cloud.google.com/go/videointelligence",
	},
	{
		ImportPath: "cloud.google.com/go/vision",
	},
	{
		ImportPath: "cloud.google.com/go/vision/v2",
	},
	{
		ImportPath: "cloud.google.com/go/vmmigration",
	},
	{
		ImportPath: "cloud.google.com/go/vmwareengine",
	},
	{
		ImportPath: "cloud.google.com/go/vpcaccess",
	},
	{
		ImportPath: "cloud.google.com/go/webrisk",
	},
	{
		ImportPath: "cloud.google.com/go/websecurityscanner",
	},
	{
		ImportPath: "cloud.google.com/go/workflows",
	},
}
