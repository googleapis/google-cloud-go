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

package aliasfix

// MigrationStatus represents how far along the alias migration is for a given
// package.
type MigrationStatus int

const (
	// StatusNotMigrated means no progress has been made to migrate the package.
	StatusNotMigrated MigrationStatus = iota
	// StatusInProgress means new types have been generated but there are not
	// yet aliases to these types nor have packages been re-written in terms of
	// these new types.
	StatusInProgress
	// StatusMigrated mean new types are being directly referenced in the GAPIC
	// layer and genproto aliases are in place.
	StatusMigrated
)

// Pkg store information related to the google-cloud-go package and whether it
// has been migrated.
type Pkg struct {
	// ImportPath in the new import path for types.
	ImportPath string
	// Status is current migration status of the associated ImportPath's types.
	Status MigrationStatus
}

// GenprotoPkgMigration maps genproto to google-cloud-go packages and tracks
// their migration status.
var GenprotoPkgMigration map[string]Pkg = map[string]Pkg{
	"google.golang.org/genproto/googleapis/analytics/admin/v1alpha": {
		ImportPath: "cloud.google.com/go/analytics/admin/apiv1alpha/adminpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/api/servicecontrol/v1": {
		ImportPath: "cloud.google.com/go/servicecontrol/apiv1/servicecontrolpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/api/servicemanagement/v1": {
		ImportPath: "cloud.google.com/go/servicemanagement/apiv1/servicemanagementpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/api/serviceusage/v1": {
		ImportPath: "cloud.google.com/go/serviceusage/apiv1/serviceusagepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/appengine/v1": {
		ImportPath: "cloud.google.com/go/appengine/apiv1/appenginepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/area120/tables/v1alpha1": {
		ImportPath: "cloud.google.com/go/area120/tables/apiv1alpha1/tablespb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/accessapproval/v1": {
		ImportPath: "cloud.google.com/go/accessapproval/apiv1/accessapprovalpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/aiplatform/v1": {
		ImportPath: "cloud.google.com/go/aiplatform/apiv1/aiplatformpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/aiplatform/v1beta1": {
		ImportPath: "cloud.google.com/go/aiplatform/apiv1beta1/aiplatformpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/apigateway/v1": {
		ImportPath: "cloud.google.com/go/apigateway/apiv1/apigatewaypb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/apigeeconnect/v1": {
		ImportPath: "cloud.google.com/go/apigeeconnect/apiv1/apigeeconnectpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/asset/v1": {
		ImportPath: "cloud.google.com/go/asset/apiv1/assetpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/asset/v1p2beta1": {
		ImportPath: "cloud.google.com/go/asset/apiv1p2beta1/assetpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/asset/v1p5beta1": {
		ImportPath: "cloud.google.com/go/asset/apiv1p5beta1/assetpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/assuredworkloads/v1": {
		ImportPath: "cloud.google.com/go/assuredworkloads/apiv1/assuredworkloadspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/assuredworkloads/v1beta1": {
		ImportPath: "cloud.google.com/go/assuredworkloads/apiv1beta1/assuredworkloadspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/automl/v1": {
		ImportPath: "cloud.google.com/go/automl/apiv1/automlpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/automl/v1beta1": {
		ImportPath: "cloud.google.com/go/automl/apiv1beta1/automlpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/baremetalsolution/v2": {
		ImportPath: "cloud.google.com/go/baremetalsolution/apiv2/baremetalsolutionpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/batch/v1": {
		ImportPath: "cloud.google.com/go/batch/apiv1/batchpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/beyondcorp/appconnections/v1": {
		ImportPath: "cloud.google.com/go/beyondcorp/appconnections/apiv1/appconnectionspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/beyondcorp/appconnectors/v1": {
		ImportPath: "cloud.google.com/go/beyondcorp/appconnectors/apiv1/appconnectorspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/beyondcorp/appgateways/v1": {
		ImportPath: "cloud.google.com/go/beyondcorp/appgateways/apiv1/appgatewayspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/beyondcorp/clientconnectorservices/v1": {
		ImportPath: "cloud.google.com/go/beyondcorp/clientconnectorservices/apiv1/clientconnectorservicespb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/beyondcorp/clientgateways/v1": {
		ImportPath: "cloud.google.com/go/beyondcorp/clientgateways/apiv1/clientgatewayspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/connection/v1": {
		ImportPath: "cloud.google.com/go/bigquery/connection/apiv1/connectionpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/connection/v1beta1": {
		ImportPath: "cloud.google.com/go/bigquery/connection/apiv1beta1/connectionpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/dataexchange/v1beta1": {
		ImportPath: "cloud.google.com/go/bigquery/dataexchange/apiv1beta1/dataexchangepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/datatransfer/v1": {
		ImportPath: "cloud.google.com/go/bigquery/datatransfer/apiv1/datatransferpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/migration/v2": {
		ImportPath: "cloud.google.com/go/bigquery/migration/apiv2/migrationpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/migration/v2alpha": {
		ImportPath: "cloud.google.com/go/bigquery/migration/apiv2alpha/migrationpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/reservation/v1": {
		ImportPath: "cloud.google.com/go/bigquery/reservation/apiv1/reservationpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1": {
		ImportPath: "cloud.google.com/go/bigquery/storage/apiv1/storagepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta1": {
		ImportPath: "cloud.google.com/go/bigquery/storage/apiv1beta1/storagepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta2": {
		ImportPath: "cloud.google.com/go/bigquery/storage/apiv1beta2/storagepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/billing/budgets/v1": {
		ImportPath: "cloud.google.com/go/billing/budgets/apiv1/budgetspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/billing/budgets/v1beta1": {
		ImportPath: "cloud.google.com/go/billing/budgets/apiv1beta1/budgetspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/billing/v1": {
		ImportPath: "cloud.google.com/go/billing/apiv1/billingpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/binaryauthorization/v1": {
		ImportPath: "cloud.google.com/go/binaryauthorization/apiv1/binaryauthorizationpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/binaryauthorization/v1beta1": {
		ImportPath: "cloud.google.com/go/binaryauthorization/apiv1beta1/binaryauthorizationpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/certificatemanager/v1": {
		ImportPath: "cloud.google.com/go/certificatemanager/apiv1/certificatemanagerpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/channel/v1": {
		ImportPath: "cloud.google.com/go/channel/apiv1/channelpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/clouddms/v1": {
		ImportPath: "cloud.google.com/go/clouddms/apiv1/clouddmspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/compute/v1": {
		ImportPath: "cloud.google.com/go/compute/apiv1/computepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/contactcenterinsights/v1": {
		ImportPath: "cloud.google.com/go/contactcenterinsights/apiv1/contactcenterinsightspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/datacatalog/v1": {
		ImportPath: "cloud.google.com/go/datacatalog/apiv1/datacatalogpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/datacatalog/v1beta1": {
		ImportPath: "cloud.google.com/go/datacatalog/apiv1beta1/datacatalogpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/dataform/v1alpha2": {
		ImportPath: "cloud.google.com/go/dataform/apiv1alpha2/dataformpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/datafusion/v1": {
		ImportPath: "cloud.google.com/go/datafusion/apiv1/datafusionpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/datalabeling/v1beta1": {
		ImportPath: "cloud.google.com/go/datalabeling/apiv1beta1/datalabelingpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/dataplex/v1": {
		ImportPath: "cloud.google.com/go/dataplex/apiv1/dataplexpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/dataproc/v1": {
		ImportPath: "cloud.google.com/go/dataproc/apiv1/dataprocpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/dataqna/v1alpha": {
		ImportPath: "cloud.google.com/go/dataqna/apiv1alpha/dataqnapb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/datastream/v1": {
		ImportPath: "cloud.google.com/go/datastream/apiv1/datastreampb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/datastream/v1alpha1": {
		ImportPath: "cloud.google.com/go/datastream/apiv1alpha1/datastreampb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/deploy/v1": {
		ImportPath: "cloud.google.com/go/deploy/apiv1/deploypb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/dialogflow/cx/v3": {
		ImportPath: "cloud.google.com/go/dialogflow/cx/apiv3/cxpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/dialogflow/cx/v3beta1": {
		ImportPath: "cloud.google.com/go/dialogflow/cx/apiv3beta1/cxpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/dialogflow/v2": {
		ImportPath: "cloud.google.com/go/dialogflow/apiv2/dialogflowpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/documentai/v1": {
		ImportPath: "cloud.google.com/go/documentai/apiv1/documentaipb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/documentai/v1beta3": {
		ImportPath: "cloud.google.com/go/documentai/apiv1beta3/documentaipb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/domains/v1beta1": {
		ImportPath: "cloud.google.com/go/domains/apiv1beta1/domainspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/essentialcontacts/v1": {
		ImportPath: "cloud.google.com/go/essentialcontacts/apiv1/essentialcontactspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/eventarc/publishing/v1": {
		ImportPath: "cloud.google.com/go/eventarc/publishing/apiv1/publishingpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/eventarc/v1": {
		ImportPath: "cloud.google.com/go/eventarc/apiv1/eventarcpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/filestore/v1": {
		ImportPath: "cloud.google.com/go/filestore/apiv1/filestorepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/functions/v1": {
		ImportPath: "cloud.google.com/go/functions/apiv1/functionspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/functions/v2": {
		ImportPath: "cloud.google.com/go/functions/apiv2/functionspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/functions/v2beta": {
		ImportPath: "cloud.google.com/go/functions/apiv2beta/functionspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/gaming/v1": {
		ImportPath: "cloud.google.com/go/gaming/apiv1/gamingpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/gaming/v1beta": {
		ImportPath: "cloud.google.com/go/gaming/apiv1beta/gamingpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/gkebackup/v1": {
		ImportPath: "cloud.google.com/go/gkebackup/apiv1/gkebackuppb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/gkeconnect/gateway/v1beta1": {
		ImportPath: "cloud.google.com/go/gkeconnect/gateway/apiv1beta1/gatewaypb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/gkehub/v1beta1": {
		ImportPath: "cloud.google.com/go/gkehub/apiv1beta1/gkehubpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/gkemulticloud/v1": {
		ImportPath: "cloud.google.com/go/gkemulticloud/apiv1/gkemulticloudpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/gsuiteaddons/v1": {
		ImportPath: "cloud.google.com/go/gsuiteaddons/apiv1/gsuiteaddonspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/iap/v1": {
		ImportPath: "cloud.google.com/go/iap/apiv1/iappb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/ids/v1": {
		ImportPath: "cloud.google.com/go/ids/apiv1/idspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/iot/v1": {
		ImportPath: "cloud.google.com/go/iot/apiv1/iotpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/kms/v1": {
		ImportPath: "cloud.google.com/go/kms/apiv1/kmspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/language/v1": {
		ImportPath: "cloud.google.com/go/language/apiv1/languagepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/language/v1beta2": {
		ImportPath: "cloud.google.com/go/language/apiv1beta2/languagepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/lifesciences/v2beta": {
		ImportPath: "cloud.google.com/go/lifesciences/apiv2beta/lifesciencespb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/managedidentities/v1": {
		ImportPath: "cloud.google.com/go/managedidentities/apiv1/managedidentitiespb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/mediatranslation/v1beta1": {
		ImportPath: "cloud.google.com/go/mediatranslation/apiv1beta1/mediatranslationpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/memcache/v1": {
		ImportPath: "cloud.google.com/go/memcache/apiv1/memcachepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/memcache/v1beta2": {
		ImportPath: "cloud.google.com/go/memcache/apiv1beta2/memcachepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/metastore/v1": {
		ImportPath: "cloud.google.com/go/metastore/apiv1/metastorepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/metastore/v1alpha": {
		ImportPath: "cloud.google.com/go/metastore/apiv1alpha/metastorepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/metastore/v1beta": {
		ImportPath: "cloud.google.com/go/metastore/apiv1beta/metastorepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/networkconnectivity/v1": {
		ImportPath: "cloud.google.com/go/networkconnectivity/apiv1/networkconnectivitypb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/networkconnectivity/v1alpha1": {
		ImportPath: "cloud.google.com/go/networkconnectivity/apiv1alpha1/networkconnectivitypb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/networkmanagement/v1": {
		ImportPath: "cloud.google.com/go/networkmanagement/apiv1/networkmanagementpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/networksecurity/v1beta1": {
		ImportPath: "cloud.google.com/go/networksecurity/apiv1beta1/networksecuritypb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/notebooks/v1": {
		ImportPath: "cloud.google.com/go/notebooks/apiv1/notebookspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/notebooks/v1beta1": {
		ImportPath: "cloud.google.com/go/notebooks/apiv1beta1/notebookspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/optimization/v1": {
		ImportPath: "cloud.google.com/go/optimization/apiv1/optimizationpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/orchestration/airflow/service/v1": {
		ImportPath: "cloud.google.com/go/orchestration/airflow/service/apiv1/servicepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/orgpolicy/v2": {
		ImportPath: "cloud.google.com/go/orgpolicy/apiv2/orgpolicypb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/osconfig/agentendpoint/v1": {
		ImportPath: "cloud.google.com/go/osconfig/agentendpoint/apiv1/agentendpointpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/osconfig/agentendpoint/v1beta": {
		ImportPath: "cloud.google.com/go/osconfig/agentendpoint/apiv1beta/agentendpointpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/osconfig/v1": {
		ImportPath: "cloud.google.com/go/osconfig/apiv1/osconfigpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/osconfig/v1alpha": {
		ImportPath: "cloud.google.com/go/osconfig/apiv1alpha/osconfigpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/osconfig/v1beta": {
		ImportPath: "cloud.google.com/go/osconfig/apiv1beta/osconfigpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/oslogin/v1": {
		ImportPath: "cloud.google.com/go/oslogin/apiv1/osloginpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/oslogin/v1beta": {
		ImportPath: "cloud.google.com/go/oslogin/apiv1beta/osloginpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/phishingprotection/v1beta1": {
		ImportPath: "cloud.google.com/go/phishingprotection/apiv1beta1/phishingprotectionpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/policytroubleshooter/v1": {
		ImportPath: "cloud.google.com/go/policytroubleshooter/apiv1/policytroubleshooterpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/privatecatalog/v1beta1": {
		ImportPath: "cloud.google.com/go/privatecatalog/apiv1beta1/privatecatalogpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/pubsublite/v1": {
		ImportPath: "cloud.google.com/go/pubsublite/apiv1/pubsublitepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/recaptchaenterprise/v1": {
		ImportPath: "cloud.google.com/go/recaptchaenterprise/v2/apiv1/recaptchaenterprisepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/recaptchaenterprise/v1beta1": {
		ImportPath: "cloud.google.com/go/recaptchaenterprise/v2/apiv1beta1/recaptchaenterprisepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/recommendationengine/v1beta1": {
		ImportPath: "cloud.google.com/go/recommendationengine/apiv1beta1/recommendationenginepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/recommender/v1": {
		ImportPath: "cloud.google.com/go/recommender/apiv1/recommenderpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/recommender/v1beta1": {
		ImportPath: "cloud.google.com/go/recommender/apiv1beta1/recommenderpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/redis/v1": {
		ImportPath: "cloud.google.com/go/redis/apiv1/redispb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/redis/v1beta1": {
		ImportPath: "cloud.google.com/go/redis/apiv1beta1/redispb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/resourcemanager/v2": {
		ImportPath: "cloud.google.com/go/resourcemanager/apiv2/resourcemanagerpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/resourcemanager/v3": {
		ImportPath: "cloud.google.com/go/resourcemanager/apiv3/resourcemanagerpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/resourcesettings/v1": {
		ImportPath: "cloud.google.com/go/resourcesettings/apiv1/resourcesettingspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/retail/v2": {
		ImportPath: "cloud.google.com/go/retail/apiv2/retailpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/retail/v2alpha": {
		ImportPath: "cloud.google.com/go/retail/apiv2alpha/retailpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/retail/v2beta": {
		ImportPath: "cloud.google.com/go/retail/apiv2beta/retailpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/run/v2": {
		ImportPath: "cloud.google.com/go/run/apiv2/runpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/scheduler/v1": {
		ImportPath: "cloud.google.com/go/scheduler/apiv1/schedulerpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/scheduler/v1beta1": {
		ImportPath: "cloud.google.com/go/scheduler/apiv1beta1/schedulerpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/secretmanager/v1": {
		ImportPath: "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/security/privateca/v1": {
		ImportPath: "cloud.google.com/go/security/privateca/apiv1/privatecapb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/securitycenter/settings/v1beta1": {
		ImportPath: "cloud.google.com/go/securitycenter/settings/apiv1beta1/settingspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/securitycenter/v1": {
		ImportPath: "cloud.google.com/go/securitycenter/apiv1/securitycenterpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/securitycenter/v1beta1": {
		ImportPath: "cloud.google.com/go/securitycenter/apiv1beta1/securitycenterpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/securitycenter/v1p1beta1": {
		ImportPath: "cloud.google.com/go/securitycenter/apiv1p1beta1/securitycenterpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/servicedirectory/v1": {
		ImportPath: "cloud.google.com/go/servicedirectory/apiv1/servicedirectorypb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/servicedirectory/v1beta1": {
		ImportPath: "cloud.google.com/go/servicedirectory/apiv1beta1/servicedirectorypb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/shell/v1": {
		ImportPath: "cloud.google.com/go/shell/apiv1/shellpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/speech/v1": {
		ImportPath: "cloud.google.com/go/speech/apiv1/speechpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/speech/v1p1beta1": {
		ImportPath: "cloud.google.com/go/speech/apiv1p1beta1/speechpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/talent/v4": {
		ImportPath: "cloud.google.com/go/talent/apiv4/talentpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/talent/v4beta1": {
		ImportPath: "cloud.google.com/go/talent/apiv4beta1/talentpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/tasks/v2": {
		ImportPath: "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/tasks/v2beta2": {
		ImportPath: "cloud.google.com/go/cloudtasks/apiv2beta2/cloudtaskspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/tasks/v2beta3": {
		ImportPath: "cloud.google.com/go/cloudtasks/apiv2beta3/cloudtaskspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/texttospeech/v1": {
		ImportPath: "cloud.google.com/go/texttospeech/apiv1/texttospeechpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/tpu/v1": {
		ImportPath: "cloud.google.com/go/tpu/apiv1/tpupb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/translate/v3": {
		ImportPath: "cloud.google.com/go/translate/apiv3/translatepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/video/livestream/v1": {
		ImportPath: "cloud.google.com/go/video/livestream/apiv1/livestreampb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/video/stitcher/v1": {
		ImportPath: "cloud.google.com/go/video/stitcher/apiv1/stitcherpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/video/transcoder/v1": {
		ImportPath: "cloud.google.com/go/video/transcoder/apiv1/transcoderpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/videointelligence/v1": {
		ImportPath: "cloud.google.com/go/videointelligence/apiv1/videointelligencepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/videointelligence/v1beta2": {
		ImportPath: "cloud.google.com/go/videointelligence/apiv1beta2/videointelligencepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/videointelligence/v1p3beta1": {
		ImportPath: "cloud.google.com/go/videointelligence/apiv1p3beta1/videointelligencepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/vision/v1": {
		ImportPath: "cloud.google.com/go/vision/v2/apiv1/visionpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/vision/v1p1beta1": {
		ImportPath: "cloud.google.com/go/vision/v2/apiv1p1beta1/visionpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/vmmigration/v1": {
		ImportPath: "cloud.google.com/go/vmmigration/apiv1/vmmigrationpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/vpcaccess/v1": {
		ImportPath: "cloud.google.com/go/vpcaccess/apiv1/vpcaccesspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/webrisk/v1": {
		ImportPath: "cloud.google.com/go/webrisk/apiv1/webriskpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/webrisk/v1beta1": {
		ImportPath: "cloud.google.com/go/webrisk/apiv1beta1/webriskpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/websecurityscanner/v1": {
		ImportPath: "cloud.google.com/go/websecurityscanner/apiv1/websecurityscannerpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/workflows/executions/v1": {
		ImportPath: "cloud.google.com/go/workflows/executions/apiv1/executionspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/workflows/executions/v1beta": {
		ImportPath: "cloud.google.com/go/workflows/executions/apiv1beta/executionspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/workflows/v1": {
		ImportPath: "cloud.google.com/go/workflows/apiv1/workflowspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/workflows/v1beta": {
		ImportPath: "cloud.google.com/go/workflows/apiv1beta/workflowspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/container/v1": {
		ImportPath: "cloud.google.com/go/container/apiv1/containerpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/dataflow/v1beta3": {
		ImportPath: "cloud.google.com/go/dataflow/apiv1beta3/dataflowpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/datastore/admin/v1": {
		ImportPath: "cloud.google.com/go/datastore/admin/apiv1/adminpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/devtools/artifactregistry/v1": {
		ImportPath: "cloud.google.com/go/artifactregistry/apiv1/artifactregistrypb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/devtools/artifactregistry/v1beta2": {
		ImportPath: "cloud.google.com/go/artifactregistry/apiv1beta2/artifactregistrypb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/devtools/cloudbuild/v1": {
		ImportPath: "cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/devtools/clouddebugger/v2": {
		ImportPath: "cloud.google.com/go/debugger/apiv2/debuggerpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/devtools/clouderrorreporting/v1beta1": {
		ImportPath: "cloud.google.com/go/errorreporting/apiv1beta1/errorreportingpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/devtools/cloudtrace/v1": {
		ImportPath: "cloud.google.com/go/trace/apiv1/tracepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/devtools/cloudtrace/v2": {
		ImportPath: "cloud.google.com/go/trace/apiv2/tracepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/devtools/containeranalysis/v1beta1": {
		ImportPath: "cloud.google.com/go/containeranalysis/apiv1beta1/containeranalysispb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/devtools/containeranalysis/v1beta1/grafeas": {
		ImportPath: "cloud.google.com/go/containeranalysis/apiv1beta1/grafeas/grafeaspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/firestore/admin/v1": {
		ImportPath: "cloud.google.com/go/firestore/apiv1/admin/adminpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/firestore/v1": {
		ImportPath: "cloud.google.com/go/firestore/apiv1/firestorepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/iam/v1": {
		ImportPath: "cloud.google.com/go/iam/apiv1/iampb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/iam/admin/v1": {
		ImportPath: "cloud.google.com/go/iam/admin/apiv1/adminpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/iam/credentials/v1": {
		ImportPath: "cloud.google.com/go/iam/credentials/apiv1/credentialspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/identity/accesscontextmanager/v1": {
		ImportPath: "cloud.google.com/go/accesscontextmanager/apiv1/accesscontextmanagerpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/logging/v2": {
		ImportPath: "cloud.google.com/go/logging/apiv2/loggingpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/longrunning": {
		ImportPath: "cloud.google.com/go/longrunning/autogen/longrunningpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/monitoring/dashboard/v1": {
		ImportPath: "cloud.google.com/go/monitoring/dashboard/apiv1/dashboardpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/monitoring/metricsscope/v1": {
		ImportPath: "cloud.google.com/go/monitoring/metricsscope/apiv1/metricsscopepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/monitoring/v3": {
		ImportPath: "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/privacy/dlp/v2": {
		ImportPath: "cloud.google.com/go/dlp/apiv2/dlppb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/pubsub/v1": {
		ImportPath: "cloud.google.com/go/pubsub/apiv1/pubsubpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/spanner/admin/database/v1": {
		ImportPath: "cloud.google.com/go/spanner/admin/database/apiv1/databasepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/spanner/admin/instance/v1": {
		ImportPath: "cloud.google.com/go/spanner/admin/instance/apiv1/instancepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/spanner/v1": {
		ImportPath: "cloud.google.com/go/spanner/apiv1/spannerpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/storagetransfer/v1": {
		ImportPath: "cloud.google.com/go/storagetransfer/apiv1/storagetransferpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/security/publicca/v1beta1": {
		ImportPath: "cloud.google.com/go/security/publicca/apiv1beta1/publiccapb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/edgecontainer/v1": {
		ImportPath: "cloud.google.com/go/edgecontainer/apiv1/edgecontainerpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/datapolicies/v1beta1": {
		ImportPath: "cloud.google.com/go/bigquery/datapolicies/apiv1beta1/datapoliciespb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/analyticshub/v1": {
		ImportPath: "cloud.google.com/go/bigquery/analyticshub/apiv1/analyticshubpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/speech/v2": {
		ImportPath: "cloud.google.com/go/speech/apiv2/speechpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/dataform/v1beta1": {
		ImportPath: "cloud.google.com/go/dataform/apiv1beta1/dataformpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/maps/addressvalidation/v1": {
		ImportPath: "cloud.google.com/go/maps/addressvalidation/apiv1/addressvalidationpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/maps/routing/v2": {
		ImportPath: "cloud.google.com/go/maps/routing/apiv2/routingpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/vmwareengine/v1": {
		ImportPath: "cloud.google.com/go/vmwareengine/apiv1/vmwareenginepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/bigquery/datapolicies/v1": {
		ImportPath: "cloud.google.com/go/bigquery/datapolicies/apiv1/datapoliciespb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/datacatalog/lineage/v1": {
		ImportPath: "cloud.google.com/go/datacatalog/lineage/apiv1/lineagepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/discoveryengine/v1beta": {
		ImportPath: "cloud.google.com/go/discoveryengine/apiv1beta/discoveryenginepb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/api/apikeys/v2": {
		ImportPath: "cloud.google.com/go/apikeys/apiv2/apikeyspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/apigeeregistry/v1": {
		ImportPath: "cloud.google.com/go/apigeeregistry/apiv1/apigeeregistrypb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/dialogflow/v2beta1": {
		ImportPath: "cloud.google.com/go/dialogflow/apiv2beta1/dialogflowpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/iam/v2": {
		ImportPath: "cloud.google.com/go/iam/apiv2/iampb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/advisorynotifications/v1": {
		ImportPath: "cloud.google.com/go/advisorynotifications/apiv1/advisorynotificationspb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/alloydb/v1": {
		ImportPath: "cloud.google.com/go/alloydb/apiv1/alloydbpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/alloydb/v1beta": {
		ImportPath: "cloud.google.com/go/alloydb/apiv1beta/alloydbpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/alloydb/v1alpha": {
		ImportPath: "cloud.google.com/go/alloydb/apiv1alpha/alloydbpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/kms/inventory/v1": {
		ImportPath: "cloud.google.com/go/kms/inventory/apiv1/inventorypb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/devtools/cloudbuild/v2": {
		ImportPath: "cloud.google.com/go/cloudbuild/apiv2/cloudbuildpb",
		Status:     StatusMigrated,
	},
	"google.golang.org/genproto/googleapis/cloud/workstations/v1beta": {
		ImportPath: "cloud.google.com/go/workstations/apiv1beta/workstationspb",
		Status:     StatusMigrated,
	},
}
