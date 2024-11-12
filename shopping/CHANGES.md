# Changelog

## [0.12.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.11.1...shopping/v0.12.0) (2024-11-06)


### Features

* **shopping/css:** A new enum `SubscriptionPeriod` is added ([706ecb2](https://github.com/googleapis/google-cloud-go/commit/706ecb2c813da3109035b986a642ca891a33847f))
* **shopping/css:** A new field `headline_offer_installment` is added to message `.google.shopping.css.v1.Attributes` ([706ecb2](https://github.com/googleapis/google-cloud-go/commit/706ecb2c813da3109035b986a642ca891a33847f))
* **shopping/css:** A new field `headline_offer_subscription_cost` is added to message `.google.shopping.css.v1.Attributes` ([706ecb2](https://github.com/googleapis/google-cloud-go/commit/706ecb2c813da3109035b986a642ca891a33847f))
* **shopping/css:** A new message `HeadlineOfferInstallment` is added ([706ecb2](https://github.com/googleapis/google-cloud-go/commit/706ecb2c813da3109035b986a642ca891a33847f))
* **shopping/css:** A new message `HeadlineOfferSubscriptionCost` is added ([706ecb2](https://github.com/googleapis/google-cloud-go/commit/706ecb2c813da3109035b986a642ca891a33847f))

## [0.11.1](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.11.0...shopping/v0.11.1) (2024-10-23)


### Bug Fixes

* **shopping:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **shopping:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([2b8ca4b](https://github.com/googleapis/google-cloud-go/commit/2b8ca4b4127ce3025c7a21cc7247510e07cc5625))

## [0.11.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.10.0...shopping/v0.11.0) (2024-10-09)


### Features

* **shopping/merchant/accounts:** A new field `account_aggregation` is added to message `.google.shopping.merchant.accounts.v1beta.CreateAndConfigureAccountRequest` ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **shopping/merchant/accounts:** A new field `korean_business_registration_number` is added to message `.google.shopping.merchant.accounts.v1beta.BusinessInfo` ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **shopping/merchant/accounts:** A new message `AccountAggregation` is added ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **shopping/merchant/accounts:** A new message `AutofeedSettings` is added ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **shopping/merchant/accounts:** A new message `GetAutofeedSettingsRequest` is added ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **shopping/merchant/accounts:** A new message `UpdateAutofeedSettingsRequest` is added ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **shopping/merchant/accounts:** A new resource_definition `merchantapi.googleapis.com/AutofeedSettings` is added ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **shopping/merchant/accounts:** A new service `AutofeedSettingsService` is added ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **shopping/merchant/accounts:** Add 'force' parameter for accounts.delete method ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **shopping/merchant/datasources:** Adding some more information about supplemental data sources ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))


### Bug Fixes

* **shopping/merchant/accounts:** An existing field `account_aggregation` is removed from message `.google.shopping.merchant.accounts.v1beta.CreateAndConfigureAccountRequest` ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **shopping/merchant/accounts:** Changed field behavior for an existing field `kind` in message `.google.shopping.merchant.accounts.v1beta.RetrieveLatestTermsOfServiceRequest` ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **shopping/merchant/accounts:** Changed field behavior for an existing field `region_code` in message `.google.shopping.merchant.accounts.v1beta.RetrieveLatestTermsOfServiceRequest` ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **shopping/merchant/accounts:** Changed field behavior for an existing field `service` in message `.google.shopping.merchant.accounts.v1beta.CreateAndConfigureAccountRequest` ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **shopping/merchant/accounts:** The type of an existing field `time_zone` is changed from `message` to `string` in message `.google.shopping.merchant.accounts.v1beta.ListAccountIssuesRequest` ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))


### Documentation

* **shopping/merchant/accounts:** Updated descriptions for the DeleteAccount and ListAccounts RPCs ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))

## [0.10.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.9.2...shopping/v0.10.0) (2024-09-25)


### Features

* **shopping/merchant/datasources:** Add FileUploads service ([#10887](https://github.com/googleapis/google-cloud-go/issues/10887)) ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))


### Documentation

* **shopping/merchant/datasources:** A comment for enum value `PRODUCTS` in enum `Channel` is changed ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))

## [0.9.2](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.9.1...shopping/v0.9.2) (2024-09-12)


### Bug Fixes

* **shopping:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [0.9.1](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.9.0...shopping/v0.9.1) (2024-08-23)


### Documentation

* **shopping/css:** Update `Certification` field descriptions ([946a5fc](https://github.com/googleapis/google-cloud-go/commit/946a5fcfeb85e22b1d8e995cda6b18b745459656))

## [0.9.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.8.7...shopping/v0.9.0) (2024-08-20)


### Features

* **shopping:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [0.8.7](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.8.6...shopping/v0.8.7) (2024-08-08)


### Bug Fixes

* **shopping:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [0.8.6](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.8.5...shopping/v0.8.6) (2024-07-24)


### Bug Fixes

* **shopping:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [0.8.5](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.8.4...shopping/v0.8.5) (2024-07-10)


### Bug Fixes

* **shopping:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [0.8.4](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.8.3...shopping/v0.8.4) (2024-07-01)


### Bug Fixes

* **shopping:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [0.8.3](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.8.2...shopping/v0.8.3) (2024-06-26)


### Bug Fixes

* **shopping:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [0.8.2](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.8.1...shopping/v0.8.2) (2024-06-18)


### Documentation

* **shopping/css:** Remove "in Google Shopping" from documentation comments ([abac5c6](https://github.com/googleapis/google-cloud-go/commit/abac5c6eec859477c6d390b116ea8954213ba585))

## [0.8.1](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.8.0...shopping/v0.8.1) (2024-06-10)


### Documentation

* **shopping/merchant/accounts:** Format comments in ListUsersRequest ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))

## [0.8.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.7.0...shopping/v0.8.0) (2024-06-05)


### Features

* **shopping:** New client(s) ([#10313](https://github.com/googleapis/google-cloud-go/issues/10313)) ([b439b80](https://github.com/googleapis/google-cloud-go/commit/b439b80a7488ff6b3bce775b63f7923951ee5e1a))


### Documentation

* **shopping/merchant/accounts:** Mark `BusinessInfo.phone` as output only ([#10319](https://github.com/googleapis/google-cloud-go/issues/10319)) ([d5150d3](https://github.com/googleapis/google-cloud-go/commit/d5150d34eabac0218cbd16a9bbdaaaf019cf237d))

## [0.7.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.6.0...shopping/v0.7.0) (2024-05-22)


### Features

* **shopping/merchant/reports:** A new enum `Effectiveness` is added ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/reports:** A new field `effectiveness` is added to message `.google.shopping.merchant.reports.v1beta.PriceInsightsProductView` ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/reports:** Add `effectiveness` field to `price_insights_product_view` table in Reports sub-API ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/reports:** Add `non_product_performance_view` table to Reports sub-API ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))


### Documentation

* **shopping/merchant/conversions:** A comment for message `MerchantCenterDestination` is changed ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/conversions:** Change in wording ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/inventories:** A comment for field `availability` in message `.google.shopping.merchant.inventories.v1beta.LocalInventory` is changed ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/inventories:** A comment for field `availability` in message `.google.shopping.merchant.inventories.v1beta.RegionalInventory` is changed ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/inventories:** A comment for field `custom_attributes` in message `.google.shopping.merchant.inventories.v1beta.LocalInventory` is changed ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/inventories:** A comment for field `custom_attributes` in message `.google.shopping.merchant.inventories.v1beta.RegionalInventory` is changed ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/inventories:** A comment for field `pickup_method` in message `.google.shopping.merchant.inventories.v1beta.LocalInventory` is changed ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/inventories:** A comment for field `pickup_sla` in message `.google.shopping.merchant.inventories.v1beta.LocalInventory` is changed ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/inventories:** A comment for field `store_code` in message `.google.shopping.merchant.inventories.v1beta.LocalInventory` is changed ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/inventories:** A comment for message `LocalInventory` is changed ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/inventories:** A comment for message `RegionalInventory` is changed ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/inventories:** Change in wording ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/lfp:** A comment for enum `StoreMatchingState` is changed ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/lfp:** A comment for field `availability` in message `.google.shopping.merchant.lfp.v1beta.LfpInventory` is changed ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/lfp:** A comment for field `matching_state` in message `.google.shopping.merchant.lfp.v1beta.LfpStore` is changed ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/lfp:** A comment for field `pickup_method` in message `.google.shopping.merchant.lfp.v1beta.LfpInventory` is changed ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/lfp:** A comment for field `pickup_sla` in message `.google.shopping.merchant.lfp.v1beta.LfpInventory` is changed ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/lfp:** A comment for message `LfpStore` is changed ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/lfp:** Change in wording ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/reports:** A comment for enum `AggregatedReportingContextStatus` is changed ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/reports:** A comment for field `brand_inventory_status` in message `.google.shopping.merchant.reports.v1beta.BestSellersProductClusterView` is changed ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/reports:** A comment for field `inventory_status` in message `.google.shopping.merchant.reports.v1beta.BestSellersProductClusterView` is changed ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))
* **shopping/merchant/reports:** A comment for field `shipping_label` in message `.google.shopping.merchant.reports.v1beta.ProductView` is changed ([a07781a](https://github.com/googleapis/google-cloud-go/commit/a07781a7a28a9895f776742b3bdf1be963ce95e9))

## [0.6.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.5.0...shopping/v0.6.0) (2024-05-01)


### Features

* **shopping:** Add `Weight` to common types for Shopping APIs to be used for accounts bundle ([1d757c6](https://github.com/googleapis/google-cloud-go/commit/1d757c66478963d6cbbef13fee939632c742759c))
* **shopping:** New shopping.merchant.conversions client ([#10076](https://github.com/googleapis/google-cloud-go/issues/10076)) ([59457a3](https://github.com/googleapis/google-cloud-go/commit/59457a33731b2d9f79a4ade9e563643d5487a3c6))


### Bug Fixes

* **shopping:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [0.5.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.4.0...shopping/v0.5.0) (2024-04-15)


### Features

* **shopping/merchant/inventories:** Fix inventories sub-API publication by adding correct child_type in the API proto ([#9750](https://github.com/googleapis/google-cloud-go/issues/9750)) ([6a7cd4f](https://github.com/googleapis/google-cloud-go/commit/6a7cd4f70373fe7c60dcba12636a3d92617e7b66))
* **shopping/merchant/reports:** Add click potential to Reports sub-API publication ([#9738](https://github.com/googleapis/google-cloud-go/issues/9738)) ([4d0547f](https://github.com/googleapis/google-cloud-go/commit/4d0547fc59d73cb013d35c9b52f8683a0d57af67))
* **shopping:** New client(s) ([#9741](https://github.com/googleapis/google-cloud-go/issues/9741)) ([1b2aebd](https://github.com/googleapis/google-cloud-go/commit/1b2aebd50e78de7e39bf2ab1ea12ea02aab58717))
* **shopping:** New clients ([#9746](https://github.com/googleapis/google-cloud-go/issues/9746)) ([cee2900](https://github.com/googleapis/google-cloud-go/commit/cee290011a43e4037ce2de24014fc60dc9a9c141))

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.3.2...shopping/v0.4.0) (2024-03-27)


### Features

* **shopping:** Add DEMAND_GEN_ADS and DEMAND_GEN_ADS_DISCOVER_SURFACE in ReportingContextEnum ([#9648](https://github.com/googleapis/google-cloud-go/issues/9648)) ([4834425](https://github.com/googleapis/google-cloud-go/commit/48344254a5d21ec51ffee275c78a15c9345dc09c))

## [0.3.2](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.3.1...shopping/v0.3.2) (2024-03-14)


### Bug Fixes

* **shopping:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [0.3.1](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.3.0...shopping/v0.3.1) (2024-01-30)


### Bug Fixes

* **shopping:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.2.2...shopping/v0.3.0) (2023-12-13)


### Features

* **shopping:** New clients ([#9141](https://github.com/googleapis/google-cloud-go/issues/9141)) ([4f49b79](https://github.com/googleapis/google-cloud-go/commit/4f49b796ed219869920668698726bee445bf5ff4))

## [0.2.2](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.2.1...shopping/v0.2.2) (2023-11-01)


### Bug Fixes

* **shopping:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [0.2.1](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.2.0...shopping/v0.2.1) (2023-10-26)


### Bug Fixes

* **shopping:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.1.1...shopping/v0.2.0) (2023-10-17)


### Features

* **shopping:** Channel enum is added ([56ce871](https://github.com/googleapis/google-cloud-go/commit/56ce87195320634b07ae0b012efcc5f2b3813fb0))
* **shopping:** ReportingContext enum is added ([56ce871](https://github.com/googleapis/google-cloud-go/commit/56ce87195320634b07ae0b012efcc5f2b3813fb0))

## [0.1.1](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.1.0...shopping/v0.1.1) (2023-10-12)


### Bug Fixes

* **shopping:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## 0.1.0 (2023-10-12)


### Features

* **shopping:** New clients ([#8699](https://github.com/googleapis/google-cloud-go/issues/8699)) ([0e43b40](https://github.com/googleapis/google-cloud-go/commit/0e43b40184bacac8d355ea2cfd00ebe58bd9e30b))

## Changes
