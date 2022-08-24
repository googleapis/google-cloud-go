// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

type pkg struct {
	importPath string
	migrated   bool
}

var m map[string]pkg = map[string]pkg{
	"google.golang.org/genproto/googleapis/analytics/admin/v1alpha": {
		importPath: "cloud.google.com/go/analytics/admin/apiv1alpha/adminpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/api/servicecontrol/v1": {
		importPath: "cloud.google.com/go/servicecontrol/apiv1/servicecontrolpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/api/servicemanagement/v1": {
		importPath: "cloud.google.com/go/servicemanagement/apiv1/servicemanagementpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/api/serviceusage/v1": {
		importPath: "cloud.google.com/go/serviceusage/apiv1/serviceusagepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/appengine/v1": {
		importPath: "cloud.google.com/go/appengine/apiv1/appenginepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/area120/tables/v1alpha1": {
		importPath: "cloud.google.com/go/area120/tables/apiv1alpha1/tablespb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/accessapproval/v1": {
		importPath: "cloud.google.com/go/accessapproval/apiv1/accessapprovalpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/aiplatform/v1": {
		importPath: "cloud.google.com/go/aiplatform/apiv1/aiplatformpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/aiplatform/v1beta1": {
		importPath: "cloud.google.com/go/aiplatform/apiv1beta1/aiplatformpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/apigateway/v1": {
		importPath: "cloud.google.com/go/apigateway/apiv1/apigatewaypb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/apigeeconnect/v1": {
		importPath: "cloud.google.com/go/apigeeconnect/apiv1/apigeeconnectpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/asset/v1": {
		importPath: "cloud.google.com/go/asset/apiv1/assetpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/asset/v1p2beta1": {
		importPath: "cloud.google.com/go/asset/apiv1p2beta1/assetpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/asset/v1p5beta1": {
		importPath: "cloud.google.com/go/asset/apiv1p5beta1/assetpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/assuredworkloads/v1": {
		importPath: "cloud.google.com/go/assuredworkloads/apiv1/assuredworkloadspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/assuredworkloads/v1beta1": {
		importPath: "cloud.google.com/go/assuredworkloads/apiv1beta1/assuredworkloadspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/automl/v1": {
		importPath: "cloud.google.com/go/automl/apiv1/automlpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/automl/v1beta1": {
		importPath: "cloud.google.com/go/automl/apiv1beta1/automlpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/baremetalsolution/v2": {
		importPath: "cloud.google.com/go/baremetalsolution/apiv2/baremetalsolutionpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/batch/v1": {
		importPath: "cloud.google.com/go/batch/apiv1/batchpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/beyondcorp/appconnections/v1": {
		importPath: "cloud.google.com/go/beyondcorp/appconnections/apiv1/appconnectionspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/beyondcorp/appconnectors/v1": {
		importPath: "cloud.google.com/go/beyondcorp/appconnectors/apiv1/appconnectorspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/beyondcorp/appgateways/v1": {
		importPath: "cloud.google.com/go/beyondcorp/appgateways/apiv1/appgatewayspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/beyondcorp/clientconnectorservices/v1": {
		importPath: "cloud.google.com/go/beyondcorp/clientconnectorservices/apiv1/clientconnectorservicespb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/beyondcorp/clientgateways/v1": {
		importPath: "cloud.google.com/go/beyondcorp/clientgateways/apiv1/clientgatewayspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/connection/v1": {
		importPath: "cloud.google.com/go/bigquery/connection/apiv1/connectionpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/connection/v1beta1": {
		importPath: "cloud.google.com/go/bigquery/connection/apiv1beta1/connectionpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/dataexchange/v1beta1": {
		importPath: "cloud.google.com/go/bigquery/dataexchange/apiv1beta1/dataexchangepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/datatransfer/v1": {
		importPath: "cloud.google.com/go/bigquery/datatransfer/apiv1/datatransferpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/migration/v2": {
		importPath: "cloud.google.com/go/bigquery/migration/apiv2/migrationpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/migration/v2alpha": {
		importPath: "cloud.google.com/go/bigquery/migration/apiv2alpha/migrationpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/reservation/v1": {
		importPath: "cloud.google.com/go/bigquery/reservation/apiv1/reservationpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/reservation/v1beta1": {
		importPath: "cloud.google.com/go/bigquery/reservation/apiv1beta1/reservationpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1": {
		importPath: "cloud.google.com/go/bigquery/storage/apiv1/storagepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta1": {
		importPath: "cloud.google.com/go/bigquery/storage/apiv1beta1/storagepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta2": {
		importPath: "cloud.google.com/go/bigquery/storage/apiv1beta2/storagepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/billing/budgets/v1": {
		importPath: "cloud.google.com/go/billing/budgets/apiv1/budgetspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/billing/budgets/v1beta1": {
		importPath: "cloud.google.com/go/billing/budgets/apiv1beta1/budgetspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/billing/v1": {
		importPath: "cloud.google.com/go/billing/apiv1/billingpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/binaryauthorization/v1": {
		importPath: "cloud.google.com/go/binaryauthorization/apiv1/binaryauthorizationpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/binaryauthorization/v1beta1": {
		importPath: "cloud.google.com/go/binaryauthorization/apiv1beta1/binaryauthorizationpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/certificatemanager/v1": {
		importPath: "cloud.google.com/go/certificatemanager/apiv1/certificatemanagerpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/channel/v1": {
		importPath: "cloud.google.com/go/channel/apiv1/channelpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/clouddms/v1": {
		importPath: "cloud.google.com/go/clouddms/apiv1/clouddmspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/compute/v1": {
		importPath: "cloud.google.com/go/compute/apiv1/computepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/contactcenterinsights/v1": {
		importPath: "cloud.google.com/go/contactcenterinsights/apiv1/contactcenterinsightspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/datacatalog/v1": {
		importPath: "cloud.google.com/go/datacatalog/apiv1/datacatalogpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/datacatalog/v1beta1": {
		importPath: "cloud.google.com/go/datacatalog/apiv1beta1/datacatalogpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/dataform/v1alpha2": {
		importPath: "cloud.google.com/go/dataform/apiv1alpha2/dataformpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/datafusion/v1": {
		importPath: "cloud.google.com/go/datafusion/apiv1/datafusionpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/datalabeling/v1beta1": {
		importPath: "cloud.google.com/go/datalabeling/apiv1beta1/datalabelingpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/dataplex/v1": {
		importPath: "cloud.google.com/go/dataplex/apiv1/dataplexpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/dataproc/v1": {
		importPath: "cloud.google.com/go/dataproc/apiv1/dataprocpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/dataqna/v1alpha": {
		importPath: "cloud.google.com/go/dataqna/apiv1alpha/dataqnapb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/datastream/v1": {
		importPath: "cloud.google.com/go/datastream/apiv1/datastreampb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/datastream/v1alpha1": {
		importPath: "cloud.google.com/go/datastream/apiv1alpha1/datastreampb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/deploy/v1": {
		importPath: "cloud.google.com/go/deploy/apiv1/deploypb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/dialogflow/cx/v3": {
		importPath: "cloud.google.com/go/dialogflow/cx/apiv3/cxpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/dialogflow/cx/v3beta1": {
		importPath: "cloud.google.com/go/dialogflow/cx/apiv3beta1/cxpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/dialogflow/v2": {
		importPath: "cloud.google.com/go/dialogflow/apiv2/dialogflowpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/documentai/v1": {
		importPath: "cloud.google.com/go/documentai/apiv1/documentaipb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/documentai/v1beta3": {
		importPath: "cloud.google.com/go/documentai/apiv1beta3/documentaipb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/domains/v1beta1": {
		importPath: "cloud.google.com/go/domains/apiv1beta1/domainspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/essentialcontacts/v1": {
		importPath: "cloud.google.com/go/essentialcontacts/apiv1/essentialcontactspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/eventarc/publishing/v1": {
		importPath: "cloud.google.com/go/eventarc/publishing/apiv1/publishingpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/eventarc/v1": {
		importPath: "cloud.google.com/go/eventarc/apiv1/eventarcpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/filestore/v1": {
		importPath: "cloud.google.com/go/filestore/apiv1/filestorepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/functions/v1": {
		importPath: "cloud.google.com/go/functions/apiv1/functionspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/functions/v2": {
		importPath: "cloud.google.com/go/functions/apiv2/functionspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/functions/v2beta": {
		importPath: "cloud.google.com/go/functions/apiv2beta/functionspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/gaming/v1": {
		importPath: "cloud.google.com/go/gaming/apiv1/gamingpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/gaming/v1beta": {
		importPath: "cloud.google.com/go/gaming/apiv1beta/gamingpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/gkebackup/v1": {
		importPath: "cloud.google.com/go/gkebackup/apiv1/gkebackuppb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/gkeconnect/gateway/v1beta1": {
		importPath: "cloud.google.com/go/gkeconnect/gateway/apiv1beta1/gatewaypb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/gkehub/v1beta1": {
		importPath: "cloud.google.com/go/gkehub/apiv1beta1/gkehubpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/gkemulticloud/v1": {
		importPath: "cloud.google.com/go/gkemulticloud/apiv1/gkemulticloudpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/gsuiteaddons/v1": {
		importPath: "cloud.google.com/go/gsuiteaddons/apiv1/gsuiteaddonspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/iap/v1": {
		importPath: "cloud.google.com/go/iap/apiv1/iappb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/ids/v1": {
		importPath: "cloud.google.com/go/ids/apiv1/idspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/iot/v1": {
		importPath: "cloud.google.com/go/iot/apiv1/iotpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/kms/v1": {
		importPath: "cloud.google.com/go/kms/apiv1/kmspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/language/v1": {
		importPath: "cloud.google.com/go/language/apiv1/languagepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/language/v1beta2": {
		importPath: "cloud.google.com/go/language/apiv1beta2/languagepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/lifesciences/v2beta": {
		importPath: "cloud.google.com/go/lifesciences/apiv2beta/lifesciencespb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/managedidentities/v1": {
		importPath: "cloud.google.com/go/managedidentities/apiv1/managedidentitiespb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/mediatranslation/v1beta1": {
		importPath: "cloud.google.com/go/mediatranslation/apiv1beta1/mediatranslationpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/memcache/v1": {
		importPath: "cloud.google.com/go/memcache/apiv1/memcachepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/memcache/v1beta2": {
		importPath: "cloud.google.com/go/memcache/apiv1beta2/memcachepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/metastore/v1": {
		importPath: "cloud.google.com/go/metastore/apiv1/metastorepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/metastore/v1alpha": {
		importPath: "cloud.google.com/go/metastore/apiv1alpha/metastorepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/metastore/v1beta": {
		importPath: "cloud.google.com/go/metastore/apiv1beta/metastorepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/networkconnectivity/v1": {
		importPath: "cloud.google.com/go/networkconnectivity/apiv1/networkconnectivitypb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/networkconnectivity/v1alpha1": {
		importPath: "cloud.google.com/go/networkconnectivity/apiv1alpha1/networkconnectivitypb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/networkmanagement/v1": {
		importPath: "cloud.google.com/go/networkmanagement/apiv1/networkmanagementpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/networksecurity/v1beta1": {
		importPath: "cloud.google.com/go/networksecurity/apiv1beta1/networksecuritypb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/notebooks/v1": {
		importPath: "cloud.google.com/go/notebooks/apiv1/notebookspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/notebooks/v1beta1": {
		importPath: "cloud.google.com/go/notebooks/apiv1beta1/notebookspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/optimization/v1": {
		importPath: "cloud.google.com/go/optimization/apiv1/optimizationpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/orchestration/airflow/service/v1": {
		importPath: "cloud.google.com/go/orchestration/airflow/service/apiv1/servicepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/orgpolicy/v2": {
		importPath: "cloud.google.com/go/orgpolicy/apiv2/orgpolicypb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/osconfig/agentendpoint/v1": {
		importPath: "cloud.google.com/go/osconfig/agentendpoint/apiv1/agentendpointpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/osconfig/agentendpoint/v1beta": {
		importPath: "cloud.google.com/go/osconfig/agentendpoint/apiv1beta/agentendpointpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/osconfig/v1": {
		importPath: "cloud.google.com/go/osconfig/apiv1/osconfigpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/osconfig/v1alpha": {
		importPath: "cloud.google.com/go/osconfig/apiv1alpha/osconfigpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/osconfig/v1beta": {
		importPath: "cloud.google.com/go/osconfig/apiv1beta/osconfigpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/oslogin/v1": {
		importPath: "cloud.google.com/go/oslogin/apiv1/osloginpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/oslogin/v1beta": {
		importPath: "cloud.google.com/go/oslogin/apiv1beta/osloginpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/phishingprotection/v1beta1": {
		importPath: "cloud.google.com/go/phishingprotection/apiv1beta1/phishingprotectionpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/policytroubleshooter/v1": {
		importPath: "cloud.google.com/go/policytroubleshooter/apiv1/policytroubleshooterpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/privatecatalog/v1beta1": {
		importPath: "cloud.google.com/go/privatecatalog/apiv1beta1/privatecatalogpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/pubsublite/v1": {
		importPath: "cloud.google.com/go/pubsublite/apiv1/pubsublitepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/recaptchaenterprise/v1": {
		importPath: "cloud.google.com/go/recaptchaenterprise/v2/apiv1/v2pb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/recaptchaenterprise/v1beta1": {
		importPath: "cloud.google.com/go/recaptchaenterprise/apiv1beta1/recaptchaenterprisepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/recommendationengine/v1beta1": {
		importPath: "cloud.google.com/go/recommendationengine/apiv1beta1/recommendationenginepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/recommender/v1": {
		importPath: "cloud.google.com/go/recommender/apiv1/recommenderpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/recommender/v1beta1": {
		importPath: "cloud.google.com/go/recommender/apiv1beta1/recommenderpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/redis/v1": {
		importPath: "cloud.google.com/go/redis/apiv1/redispb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/redis/v1beta1": {
		importPath: "cloud.google.com/go/redis/apiv1beta1/redispb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/resourcemanager/v2": {
		importPath: "cloud.google.com/go/resourcemanager/apiv2/resourcemanagerpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/resourcemanager/v3": {
		importPath: "cloud.google.com/go/resourcemanager/apiv3/resourcemanagerpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/resourcesettings/v1": {
		importPath: "cloud.google.com/go/resourcesettings/apiv1/resourcesettingspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/retail/v2": {
		importPath: "cloud.google.com/go/retail/apiv2/retailpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/retail/v2alpha": {
		importPath: "cloud.google.com/go/retail/apiv2alpha/retailpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/retail/v2beta": {
		importPath: "cloud.google.com/go/retail/apiv2beta/retailpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/run/v2": {
		importPath: "cloud.google.com/go/run/apiv2/runpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/scheduler/v1": {
		importPath: "cloud.google.com/go/scheduler/apiv1/schedulerpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/scheduler/v1beta1": {
		importPath: "cloud.google.com/go/scheduler/apiv1beta1/schedulerpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/secretmanager/v1": {
		importPath: "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/secrets/v1beta1": {
		importPath: "cloud.google.com/go/secretmanager/apiv1beta1/secretmanagerpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/security/privateca/v1": {
		importPath: "cloud.google.com/go/security/privateca/apiv1/privatecapb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/security/privateca/v1beta1": {
		importPath: "cloud.google.com/go/security/privateca/apiv1beta1/privatecapb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/securitycenter/settings/v1beta1": {
		importPath: "cloud.google.com/go/securitycenter/settings/apiv1beta1/settingspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/securitycenter/v1": {
		importPath: "cloud.google.com/go/securitycenter/apiv1/securitycenterpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/securitycenter/v1beta1": {
		importPath: "cloud.google.com/go/securitycenter/apiv1beta1/securitycenterpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/securitycenter/v1p1beta1": {
		importPath: "cloud.google.com/go/securitycenter/apiv1p1beta1/securitycenterpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/servicedirectory/v1": {
		importPath: "cloud.google.com/go/servicedirectory/apiv1/servicedirectorypb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/servicedirectory/v1beta1": {
		importPath: "cloud.google.com/go/servicedirectory/apiv1beta1/servicedirectorypb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/shell/v1": {
		importPath: "cloud.google.com/go/shell/apiv1/shellpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/speech/v1": {
		importPath: "cloud.google.com/go/speech/apiv1/speechpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/speech/v1p1beta1": {
		importPath: "cloud.google.com/go/speech/apiv1p1beta1/speechpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/talent/v4": {
		importPath: "cloud.google.com/go/talent/apiv4/talentpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/talent/v4beta1": {
		importPath: "cloud.google.com/go/talent/apiv4beta1/talentpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/tasks/v2": {
		importPath: "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/tasks/v2beta2": {
		importPath: "cloud.google.com/go/cloudtasks/apiv2beta2/cloudtaskspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/tasks/v2beta3": {
		importPath: "cloud.google.com/go/cloudtasks/apiv2beta3/cloudtaskspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/texttospeech/v1": {
		importPath: "cloud.google.com/go/texttospeech/apiv1/texttospeechpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/tpu/v1": {
		importPath: "cloud.google.com/go/tpu/apiv1/tpupb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/translate/v3": {
		importPath: "cloud.google.com/go/translate/apiv3/translatepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/video/livestream/v1": {
		importPath: "cloud.google.com/go/video/livestream/apiv1/livestreampb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/video/stitcher/v1": {
		importPath: "cloud.google.com/go/video/stitcher/apiv1/stitcherpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/video/transcoder/v1": {
		importPath: "cloud.google.com/go/video/transcoder/apiv1/transcoderpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/videointelligence/v1": {
		importPath: "cloud.google.com/go/videointelligence/apiv1/videointelligencepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/videointelligence/v1beta2": {
		importPath: "cloud.google.com/go/videointelligence/apiv1beta2/videointelligencepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/videointelligence/v1p3beta1": {
		importPath: "cloud.google.com/go/videointelligence/apiv1p3beta1/videointelligencepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/vision/v1": {
		importPath: "cloud.google.com/go/vision/v2/apiv1/v2pb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/vision/v1p1beta1": {
		importPath: "cloud.google.com/go/vision/apiv1p1beta1/visionpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/vmmigration/v1": {
		importPath: "cloud.google.com/go/vmmigration/apiv1/vmmigrationpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/vpcaccess/v1": {
		importPath: "cloud.google.com/go/vpcaccess/apiv1/vpcaccesspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/webrisk/v1": {
		importPath: "cloud.google.com/go/webrisk/apiv1/webriskpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/webrisk/v1beta1": {
		importPath: "cloud.google.com/go/webrisk/apiv1beta1/webriskpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/websecurityscanner/v1": {
		importPath: "cloud.google.com/go/websecurityscanner/apiv1/websecurityscannerpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/workflows/executions/v1": {
		importPath: "cloud.google.com/go/workflows/executions/apiv1/executionspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/workflows/executions/v1beta": {
		importPath: "cloud.google.com/go/workflows/executions/apiv1beta/executionspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/workflows/v1": {
		importPath: "cloud.google.com/go/workflows/apiv1/workflowspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/cloud/workflows/v1beta": {
		importPath: "cloud.google.com/go/workflows/apiv1beta/workflowspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/container/v1": {
		importPath: "cloud.google.com/go/container/apiv1/containerpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/dataflow/v1beta3": {
		importPath: "cloud.google.com/go/dataflow/apiv1beta3/dataflowpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/datastore/admin/v1": {
		importPath: "cloud.google.com/go/datastore/admin/apiv1/adminpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/devtools/artifactregistry/v1": {
		importPath: "cloud.google.com/go/artifactregistry/apiv1/artifactregistrypb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/devtools/artifactregistry/v1beta2": {
		importPath: "cloud.google.com/go/artifactregistry/apiv1beta2/artifactregistrypb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/devtools/cloudbuild/v1": {
		importPath: "cloud.google.com/go/cloudbuild/apiv1/v2/apiv1pb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/devtools/clouddebugger/v2": {
		importPath: "cloud.google.com/go/debugger/apiv2/debuggerpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/devtools/clouderrorreporting/v1beta1": {
		importPath: "cloud.google.com/go/errorreporting/apiv1beta1/errorreportingpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/devtools/cloudtrace/v1": {
		importPath: "cloud.google.com/go/trace/apiv1/tracepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/devtools/cloudtrace/v2": {
		importPath: "cloud.google.com/go/trace/apiv2/tracepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/devtools/containeranalysis/v1beta1": {
		importPath: "cloud.google.com/go/containeranalysis/apiv1beta1/containeranalysispb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/devtools/containeranalysis/v1beta1/grafeas": {
		importPath: "cloud.google.com/go/containeranalysis/apiv1beta1/containeranalysispb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/firestore/admin/v1": {
		importPath: "cloud.google.com/go/firestore/apiv1/admin/apiv1pb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/firestore/v1": {
		importPath: "cloud.google.com/go/firestore/apiv1/firestorepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/iam/credentials/v1": {
		importPath: "cloud.google.com/go/iam/credentials/apiv1/credentialspb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/identity/accesscontextmanager/v1": {
		importPath: "cloud.google.com/go/accesscontextmanager/apiv1/accesscontextmanagerpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/logging/v2": {
		importPath: "cloud.google.com/go/logging/apiv2/loggingpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/longrunning": {
		importPath: "cloud.google.com/go/longrunning/autogen/longrunningpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/monitoring/dashboard/v1": {
		importPath: "cloud.google.com/go/monitoring/dashboard/apiv1/dashboardpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/monitoring/metricsscope/v1": {
		importPath: "cloud.google.com/go/monitoring/metricsscope/apiv1/metricsscopepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/monitoring/v3": {
		importPath: "cloud.google.com/go/monitoring/apiv3/v2/apiv3pb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/privacy/dlp/v2": {
		importPath: "cloud.google.com/go/dlp/apiv2/dlppb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/pubsub/v1": {
		importPath: "cloud.google.com/go/pubsub/apiv1/pubsubpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/spanner/admin/database/v1": {
		importPath: "cloud.google.com/go/spanner/admin/database/apiv1/databasepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/spanner/admin/instance/v1": {
		importPath: "cloud.google.com/go/spanner/admin/instance/apiv1/instancepb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/spanner/v1": {
		importPath: "cloud.google.com/go/spanner/apiv1/spannerpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/storage/v2": {
		importPath: "cloud.google.com/go/storage/internal/apiv2/internalpb",
		migrated:   false,
	},
	"google.golang.org/genproto/googleapis/storagetransfer/v1": {
		importPath: "cloud.google.com/go/storagetransfer/apiv1/storagetransferpb",
		migrated:   false,
	},
}
