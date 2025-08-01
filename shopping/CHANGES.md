# Changelog


## [0.24.1](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.24.0...shopping/v0.24.1) (2025-07-31)


### Bug Fixes

* **shopping/merchant/reviews:** An existing field `attributes` is renamed to `merchant_review_attributes` in message `.google.shopping.merchant.reviews.v1beta.MerchantReview` ([83f894e](https://github.com/googleapis/google-cloud-go/commit/83f894e372ae66b96d8d9d4379fa0ea18547fe72))
* **shopping/merchant/reviews:** An existing field `attributes` is renamed to `product_review_attributes` in message `.google.shopping.merchant.reviews.v1beta.ProductReview` ([83f894e](https://github.com/googleapis/google-cloud-go/commit/83f894e372ae66b96d8d9d4379fa0ea18547fe72))

## [0.24.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.23.0...shopping/v0.24.0) (2025-07-16)


### Features

* **shopping/merchant/products:** A new field `gtins` is added to message `.google.shopping.merchant.products.v1beta.Attributes` ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A new field `maximum_retail_price` is added to message `.google.shopping.merchant.products.v1beta.Attributes` ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/reviews:** A new field `is_incentivized_review` is added to message `.google.shopping.merchant.reviews.v1beta.ProductReviewAttributes` ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/reviews:** A new field `is_verified_purchase` is added to message `.google.shopping.merchant.reviews.v1beta.ProductReviewAttributes` ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))


### Documentation

* **shopping/merchant/products:** A comment for field `ads_grouping` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `auto_pricing_min_price` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `availability` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `brand` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `color` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `condition` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `custom_attributes` in message `.google.shopping.merchant.products.v1beta.ProductInput` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `custom_label_0` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `custom_label_1` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `custom_label_2` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `custom_label_3` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `custom_label_4` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `data_source` in message `.google.shopping.merchant.products.v1beta.DeleteProductInputRequest` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `data_source` in message `.google.shopping.merchant.products.v1beta.InsertProductInputRequest` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `data_source` in message `.google.shopping.merchant.products.v1beta.UpdateProductInputRequest` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `disclosure_date` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `display_ads_similar_ids` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `display_ads_value` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `excluded_destinations` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `feed_label` in message `.google.shopping.merchant.products.v1beta.Product` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `feed_label` in message `.google.shopping.merchant.products.v1beta.ProductInput` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `gender` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `gtin` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `included_destinations` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `is_bundle` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `link_template` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `material` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `mobile_link_template` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `multipack` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `name` in message `.google.shopping.merchant.products.v1beta.DeleteProductInputRequest` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `name` in message `.google.shopping.merchant.products.v1beta.ProductInput` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `page_size` in message `.google.shopping.merchant.products.v1beta.ListProductsRequest` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `parent` in message `.google.shopping.merchant.products.v1beta.InsertProductInputRequest` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `pattern` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `pickup_method` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `pickup_sla` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `product_highlights` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `product_types` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `product` in message `.google.shopping.merchant.products.v1beta.ProductInput` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `program_label` in message `.google.shopping.merchant.products.v1beta.LoyaltyProgram` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `resolution` in message `.google.shopping.merchant.products.v1beta.ProductStatus` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `sale_price_effective_date` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `shopping_ads_excluded_countries` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `size_system` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `size_types` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `size` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `tax_category` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for field `version_number` in message `.google.shopping.merchant.products.v1beta.ProductInput` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for message `ProductInput` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/products:** A comment for method `InsertProductInput` in service `ProductInputsService` is changed ([#12543](https://github.com/googleapis/google-cloud-go/issues/12543)) ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/reviews:** A comment for field `content` in message `.google.shopping.merchant.reviews.v1beta.ProductReviewAttributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/reviews:** A comment for field `custom_attributes` in message `.google.shopping.merchant.reviews.v1beta.MerchantReview` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **shopping/merchant/reviews:** A comment for field `review_language` in message `.google.shopping.merchant.reviews.v1beta.MerchantReviewAttributes` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))

## [0.23.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.22.0...shopping/v0.23.0) (2025-07-09)


### Features

* **shopping/merchant/accounts:** Add CheckoutSettings service ([#12505](https://github.com/googleapis/google-cloud-go/issues/12505)) ([208745b](https://github.com/googleapis/google-cloud-go/commit/208745bbc1f4fc9122ec71d6cf42f512ae570d13))

## [0.22.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.21.0...shopping/v0.22.0) (2025-06-04)


### Features

* **shopping/merchant/issueresolution:** Add AggregateProductStatuses ([#12397](https://github.com/googleapis/google-cloud-go/issues/12397)) ([75c4346](https://github.com/googleapis/google-cloud-go/commit/75c434671407bbbdce1e1d16424057cbf980cccd))

## [0.21.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.20.0...shopping/v0.21.0) (2025-05-21)


### Features

* **shopping/merchant/accounts:** A new method_signature `parent,online_return_policy` is added to method `CreateOnlineReturnPolicy` in service `OnlineReturnPolicyService` ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* **shopping/merchant/accounts:** Add OmnichannelSetingsService, LfpProvidersService and GbpAccountsService ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* **shopping/merchant/accounts:** Updated comments for returns sub-API publication ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))


### Documentation

* **shopping/merchant/accounts:** A comment for field `accept_defective_only` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* **shopping/merchant/accounts:** A comment for field `accept_exchange` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* **shopping/merchant/accounts:** A comment for field `item_conditions` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is changed ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))
* **shopping/merchant/accounts:** A comment for field `online_return_policy` in message `.google.shopping.merchant.accounts.v1beta.CreateOnlineReturnPolicyRequest` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* **shopping/merchant/accounts:** A comment for field `online_return_policy` in message `.google.shopping.merchant.accounts.v1beta.UpdateOnlineReturnPolicyRequest` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* **shopping/merchant/accounts:** A comment for field `parent` in message `.google.shopping.merchant.accounts.v1beta.CreateOnlineReturnPolicyRequest` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* **shopping/merchant/accounts:** A comment for field `parent` in message `.google.shopping.merchant.accounts.v1beta.ListOnlineReturnPoliciesRequest` is changed ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))
* **shopping/merchant/accounts:** A comment for field `policy` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is changed ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))
* **shopping/merchant/accounts:** A comment for field `process_refund_days` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* **shopping/merchant/accounts:** A comment for field `restocking_fee` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is changed ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))
* **shopping/merchant/accounts:** A comment for field `return_label_source` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* **shopping/merchant/accounts:** A comment for field `return_methods` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is changed ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))
* **shopping/merchant/accounts:** A comment for field `return_shipping_fee` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is changed ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))
* **shopping/merchant/accounts:** A comment for field `update_mask` in message `.google.shopping.merchant.accounts.v1beta.UpdateOnlineReturnPolicyRequest` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* **shopping/merchant/accounts:** A comment for message `UpdateOnlineReturnPolicyRequest` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* **shopping/merchant/accounts:** A comment for method `DeleteOnlineReturnPolicy` in service `OnlineReturnPolicyService` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* **shopping/merchant/accounts:** A comment for method `GetOnlineReturnPolicy` in service `OnlineReturnPolicyService` is changed ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))
* **shopping/merchant/accounts:** A comment for method `ListOnlineReturnPolicies` in service `OnlineReturnPolicyService` is changed ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))
* **shopping/merchant/accounts:** A comment for service `OnlineReturnPolicyService` is changed ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))

## [0.20.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.19.1...shopping/v0.20.0) (2025-04-30)


### Features

* **shopping:** New clients ([#12056](https://github.com/googleapis/google-cloud-go/issues/12056)) ([3ab1dd8](https://github.com/googleapis/google-cloud-go/commit/3ab1dd8441c2f805f08cb32c1d6d98a76b63130f))
* **shopping:** New clients ([#12068](https://github.com/googleapis/google-cloud-go/issues/12068)) ([25736fe](https://github.com/googleapis/google-cloud-go/commit/25736fe96434648091388e73c8ed58fa94e9deb8))


### Documentation

* **shopping/merchant/lfp:** Add clarification to GetLfpMerchantState documentation ([19c60f9](https://github.com/googleapis/google-cloud-go/commit/19c60f9ac0489ad408b4a8672c5bf091022eda15))

## [0.19.1](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.19.0...shopping/v0.19.1) (2025-04-15)


### Bug Fixes

* **shopping:** Update google.golang.org/api to 0.229.0 ([3319672](https://github.com/googleapis/google-cloud-go/commit/3319672f3dba84a7150772ccb5433e02dab7e201))

## [0.19.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.18.1...shopping/v0.19.0) (2025-04-15)


### Features

* **shopping/css:** Introduce QuotaService for CSS API ([#11962](https://github.com/googleapis/google-cloud-go/issues/11962)) ([48e00d1](https://github.com/googleapis/google-cloud-go/commit/48e00d15ffbec160d21493d07495ac74614167d9))
* **shopping/merchant/lfp:** Add GetLfpMerchantState method ([#11973](https://github.com/googleapis/google-cloud-go/issues/11973)) ([8a2171a](https://github.com/googleapis/google-cloud-go/commit/8a2171a42cca078228fe27bd287a8ba6cad30e70))
* **shopping/merchant/products:** A new field `automated_discounts` is added to message `google.shopping.merchant.products.v1beta.Product` ([48e00d1](https://github.com/googleapis/google-cloud-go/commit/48e00d15ffbec160d21493d07495ac74614167d9))


### Documentation

* **shopping/css:** A comment for field `name` in message `.google.shopping.css.v1.CssProductInput` is changed ([48e00d1](https://github.com/googleapis/google-cloud-go/commit/48e00d15ffbec160d21493d07495ac74614167d9))
* **shopping/css:** A comment for field `name` in message `.google.shopping.css.v1.DeleteCssProductInputRequest` is changed ([48e00d1](https://github.com/googleapis/google-cloud-go/commit/48e00d15ffbec160d21493d07495ac74614167d9))
* **shopping/merchant/products:** Modified several comments ([48e00d1](https://github.com/googleapis/google-cloud-go/commit/48e00d15ffbec160d21493d07495ac74614167d9))

## [0.18.1](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.18.0...shopping/v0.18.1) (2025-03-25)


### Documentation

* **shopping/css:** Added a clarifying note to the description of the parent field in the Account resource ([427f448](https://github.com/googleapis/google-cloud-go/commit/427f448d9a1a32a2a55a695e9e3a915fcc71ae19))

## [0.18.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.17.1...shopping/v0.18.0) (2025-03-19)


### Features

* **shopping/merchant/accounts:** Add AutomaticImprovements service ([05674f7](https://github.com/googleapis/google-cloud-go/commit/05674f71f13269a5ab193388e5478e55fef6622d))
* **shopping/merchant/datasources:** Add a new destinations field ([05674f7](https://github.com/googleapis/google-cloud-go/commit/05674f71f13269a5ab193388e5478e55fef6622d))
* **shopping/merchant/products:** Add an update method ([671eed9](https://github.com/googleapis/google-cloud-go/commit/671eed979bfdbf199c4c3787d4f18bca1d5883f4))


### Documentation

* **shopping/merchant/datasources:** A comment for field `channel` in message `.google.shopping.merchant.datasources.v1beta.PrimaryProductDataSource` is changed ([05674f7](https://github.com/googleapis/google-cloud-go/commit/05674f71f13269a5ab193388e5478e55fef6622d))
* **shopping/merchant/datasources:** A comment for field `promotion_data_source` in message `.google.shopping.merchant.datasources.v1beta.DataSource` is changed ([05674f7](https://github.com/googleapis/google-cloud-go/commit/05674f71f13269a5ab193388e5478e55fef6622d))
* **shopping/merchant/products:** A comment for field `channel` in message `.google.shopping.merchant.products.v1beta.ProductInput` is changed ([671eed9](https://github.com/googleapis/google-cloud-go/commit/671eed979bfdbf199c4c3787d4f18bca1d5883f4))
* **shopping/merchant/products:** A comment for field `data_source` in message `.google.shopping.merchant.products.v1beta.InsertProductInputRequest` is changed ([671eed9](https://github.com/googleapis/google-cloud-go/commit/671eed979bfdbf199c4c3787d4f18bca1d5883f4))
* **shopping/merchant/products:** A comment for message `ProductInput` is changed ([671eed9](https://github.com/googleapis/google-cloud-go/commit/671eed979bfdbf199c4c3787d4f18bca1d5883f4))

## [0.17.1](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.17.0...shopping/v0.17.1) (2025-03-13)


### Bug Fixes

* **shopping:** Update golang.org/x/net to 0.37.0 ([1144978](https://github.com/googleapis/google-cloud-go/commit/11449782c7fb4896bf8b8b9cde8e7441c84fb2fd))

## [0.17.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.16.0...shopping/v0.17.0) (2025-03-12)


### Features

* **shopping/merchant/accounts:** A new field `seasonal_overrides` is added to message .google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy ([dd0d1d7](https://github.com/googleapis/google-cloud-go/commit/dd0d1d7b41884c9fc9b5fe808139cccd29e1e486))
* **shopping/merchant/accounts:** A new message `SeasonalOverride` is added ([dd0d1d7](https://github.com/googleapis/google-cloud-go/commit/dd0d1d7b41884c9fc9b5fe808139cccd29e1e486))


### Bug Fixes

* **shopping/merchant/accounts:** An existing optional field `countries` is converted to required field in message .google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy ([dd0d1d7](https://github.com/googleapis/google-cloud-go/commit/dd0d1d7b41884c9fc9b5fe808139cccd29e1e486))
* **shopping/merchant/accounts:** An existing optional field `label` is converted to required field in message .google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy ([dd0d1d7](https://github.com/googleapis/google-cloud-go/commit/dd0d1d7b41884c9fc9b5fe808139cccd29e1e486))
* **shopping/merchant/accounts:** An existing optional field `return_policy_uri` is converted to required field in message .google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy ([dd0d1d7](https://github.com/googleapis/google-cloud-go/commit/dd0d1d7b41884c9fc9b5fe808139cccd29e1e486))
* **shopping/merchant/accounts:** An existing optional field `type` is converted to required field in message .google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy ([dd0d1d7](https://github.com/googleapis/google-cloud-go/commit/dd0d1d7b41884c9fc9b5fe808139cccd29e1e486))


### Documentation

* **shopping/merchant/accounts:** The documentation for field `countries` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is improved ([dd0d1d7](https://github.com/googleapis/google-cloud-go/commit/dd0d1d7b41884c9fc9b5fe808139cccd29e1e486))
* **shopping/merchant/accounts:** The documentation for field `label` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is improved ([dd0d1d7](https://github.com/googleapis/google-cloud-go/commit/dd0d1d7b41884c9fc9b5fe808139cccd29e1e486))
* **shopping/merchant/accounts:** The documentation for field `parent` in message `.google.shopping.merchant.accounts.v1beta.ListOnlineReturnPoliciesRequest` is improved ([dd0d1d7](https://github.com/googleapis/google-cloud-go/commit/dd0d1d7b41884c9fc9b5fe808139cccd29e1e486))
* **shopping/merchant/accounts:** The documentation for field `return_policy_uri` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is improved ([dd0d1d7](https://github.com/googleapis/google-cloud-go/commit/dd0d1d7b41884c9fc9b5fe808139cccd29e1e486))
* **shopping/merchant/accounts:** The documentation for field `type` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is improved ([dd0d1d7](https://github.com/googleapis/google-cloud-go/commit/dd0d1d7b41884c9fc9b5fe808139cccd29e1e486))
* **shopping/merchant/accounts:** The documentation for method `GetOnlineReturnPolicy` in service `OnlineReturnPolicyService` is improved ([dd0d1d7](https://github.com/googleapis/google-cloud-go/commit/dd0d1d7b41884c9fc9b5fe808139cccd29e1e486))
* **shopping/merchant/accounts:** The documentation for method `ListOnlineReturnPolicies` in service `OnlineReturnPolicyService` is improved ([dd0d1d7](https://github.com/googleapis/google-cloud-go/commit/dd0d1d7b41884c9fc9b5fe808139cccd29e1e486))

## [0.16.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.15.0...shopping/v0.16.0) (2025-01-08)


### Features

* **shopping/merchant/datasources:** A new message `MerchantReviewDataSource` is added to specify the datasource of the merchant review ([2e4feb9](https://github.com/googleapis/google-cloud-go/commit/2e4feb938ce9ab023c8aa6bd1dbdf36fe589213a))
* **shopping/merchant/datasources:** A new message `ProductReviewDataSource` is added to specify the datasource of the product review ([2e4feb9](https://github.com/googleapis/google-cloud-go/commit/2e4feb938ce9ab023c8aa6bd1dbdf36fe589213a))
* **shopping/merchant/datasources:** New field `merchant_review_data_source` added in message `.google.shopping.merchant.datasources.v1beta.DataSource` to specify the datasource of the merchant review ([2e4feb9](https://github.com/googleapis/google-cloud-go/commit/2e4feb938ce9ab023c8aa6bd1dbdf36fe589213a))
* **shopping/merchant/datasources:** New field `product_review_data_source` added in message `.google.shopping.merchant.datasources.v1beta.DataSource` to specify the datasource of the product review ([#11385](https://github.com/googleapis/google-cloud-go/issues/11385)) ([2e4feb9](https://github.com/googleapis/google-cloud-go/commit/2e4feb938ce9ab023c8aa6bd1dbdf36fe589213a))


### Documentation

* **shopping/merchant/datasources:** A comment for enum value `FETCH` in enum `FileInputType` is changed ([2e4feb9](https://github.com/googleapis/google-cloud-go/commit/2e4feb938ce9ab023c8aa6bd1dbdf36fe589213a))
* **shopping/merchant/datasources:** A comment for enum value `GOOGLE_SHEETS` in enum `FileInputType` is changed ([2e4feb9](https://github.com/googleapis/google-cloud-go/commit/2e4feb938ce9ab023c8aa6bd1dbdf36fe589213a))
* **shopping/merchant/datasources:** A comment for field `feed_label` in message `.google.shopping.merchant.datasources.v1beta.SupplementalProductDataSource` is changed ([2e4feb9](https://github.com/googleapis/google-cloud-go/commit/2e4feb938ce9ab023c8aa6bd1dbdf36fe589213a))
* **shopping/merchant/datasources:** A comment for field `password` in message `.google.shopping.merchant.datasources.v1beta.FileInput` is changed ([2e4feb9](https://github.com/googleapis/google-cloud-go/commit/2e4feb938ce9ab023c8aa6bd1dbdf36fe589213a))
* **shopping/merchant/datasources:** A comment for field `take_from_data_sources` in message `.google.shopping.merchant.datasources.v1beta.PrimaryProductDataSource` is changed ([2e4feb9](https://github.com/googleapis/google-cloud-go/commit/2e4feb938ce9ab023c8aa6bd1dbdf36fe589213a))
* **shopping/merchant/datasources:** A comment for field `username` in message `.google.shopping.merchant.datasources.v1beta.FileInput` is changed ([2e4feb9](https://github.com/googleapis/google-cloud-go/commit/2e4feb938ce9ab023c8aa6bd1dbdf36fe589213a))
* **shopping/merchant/datasources:** A comment for message `SupplementalProductDataSource` is changed ([2e4feb9](https://github.com/googleapis/google-cloud-go/commit/2e4feb938ce9ab023c8aa6bd1dbdf36fe589213a))

## [0.15.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.14.0...shopping/v0.15.0) (2025-01-02)


### Features

* **shopping/css:** UpdateCssProduct is added to CssProductInput proto ([8ebcc6d](https://github.com/googleapis/google-cloud-go/commit/8ebcc6d276fc881c3914b5a7af3265a04e718e45))


### Bug Fixes

* **shopping:** Update golang.org/x/net to v0.33.0 ([e9b0b69](https://github.com/googleapis/google-cloud-go/commit/e9b0b69644ea5b276cacff0a707e8a5e87efafc9))


### Documentation

* **shopping/css:** A comment for field `applicable_countries` in message `.google.shopping.css.v1.CssProductStatus` is changed ([8ebcc6d](https://github.com/googleapis/google-cloud-go/commit/8ebcc6d276fc881c3914b5a7af3265a04e718e45))
* **shopping/css:** A comment for field `approved_countries` in message `.google.shopping.css.v1.CssProductStatus` is changed ([8ebcc6d](https://github.com/googleapis/google-cloud-go/commit/8ebcc6d276fc881c3914b5a7af3265a04e718e45))
* **shopping/css:** A comment for field `disapproved_countries` in message `.google.shopping.css.v1.CssProductStatus` is changed ([8ebcc6d](https://github.com/googleapis/google-cloud-go/commit/8ebcc6d276fc881c3914b5a7af3265a04e718e45))
* **shopping/css:** A comment for field `feed_id` in message`.google.shopping.css.v1.InsertCssProductInputRequest` is changed ([8ebcc6d](https://github.com/googleapis/google-cloud-go/commit/8ebcc6d276fc881c3914b5a7af3265a04e718e45))
* **shopping/css:** A comment for field `headline_offer_price` in message `.google.shopping.css.v1.Attributes` is changed ([8ebcc6d](https://github.com/googleapis/google-cloud-go/commit/8ebcc6d276fc881c3914b5a7af3265a04e718e45))
* **shopping/css:** A comment for field `headline_offer_shipping_price` in message `.google.shopping.css.v1.Attributes` is changed ([8ebcc6d](https://github.com/googleapis/google-cloud-go/commit/8ebcc6d276fc881c3914b5a7af3265a04e718e45))
* **shopping/css:** A comment for field `high_price` in message `.google.shopping.css.v1.Attributes` is changed ([8ebcc6d](https://github.com/googleapis/google-cloud-go/commit/8ebcc6d276fc881c3914b5a7af3265a04e718e45))
* **shopping/css:** A comment for field `low_price` in message `.google.shopping.css.v1.Attributes` is changed ([8ebcc6d](https://github.com/googleapis/google-cloud-go/commit/8ebcc6d276fc881c3914b5a7af3265a04e718e45))
* **shopping/css:** A comment for field `number_of_offers` in message `.google.shopping.css.v1.Attributes` is changed ([8ebcc6d](https://github.com/googleapis/google-cloud-go/commit/8ebcc6d276fc881c3914b5a7af3265a04e718e45))
* **shopping/css:** A comment for field `page_size` in message `.google.shopping.css.v1.ListChildAccountsRequest` is changed ([8ebcc6d](https://github.com/googleapis/google-cloud-go/commit/8ebcc6d276fc881c3914b5a7af3265a04e718e45))
* **shopping/css:** A comment for field `pending_countries` in message `.google.shopping.css.v1.CssProductStatus` is changed ([8ebcc6d](https://github.com/googleapis/google-cloud-go/commit/8ebcc6d276fc881c3914b5a7af3265a04e718e45))
* **shopping/css:** A comment for field `servability` in message `.google.shopping.css.v1.CssProductStatus` is changed ([8ebcc6d](https://github.com/googleapis/google-cloud-go/commit/8ebcc6d276fc881c3914b5a7af3265a04e718e45))
* **shopping/css:** A comment for message `CssProduct` is changed ([8ebcc6d](https://github.com/googleapis/google-cloud-go/commit/8ebcc6d276fc881c3914b5a7af3265a04e718e45))

## [0.14.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.13.0...shopping/v0.14.0) (2024-12-18)


### Features

* **shopping:** New clients ([#11311](https://github.com/googleapis/google-cloud-go/issues/11311)) ([720f7e9](https://github.com/googleapis/google-cloud-go/commit/720f7e9c58b364b74b982af9ed53bf0905ee73a8))

## [0.13.0](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.12.1...shopping/v0.13.0) (2024-12-11)


### Features

* **shopping/merchant/products:** A new field `member_price_effective_date` is added to message `.google.shopping.merchant.products.v1beta.LoyaltyProgram` ([279350e](https://github.com/googleapis/google-cloud-go/commit/279350e1703dd7f251603408bf0381d47b87ba60))
* **shopping/merchant/products:** A new field `shipping_label` is added to message `.google.shopping.merchant.products.v1beta.LoyaltyProgram` ([279350e](https://github.com/googleapis/google-cloud-go/commit/279350e1703dd7f251603408bf0381d47b87ba60))


### Bug Fixes

* **shopping/merchant/products:** An existing field `gtin` is moved out of oneof in message `.google.shopping.merchant.products.v1beta.Attributes` ([279350e](https://github.com/googleapis/google-cloud-go/commit/279350e1703dd7f251603408bf0381d47b87ba60))
* **shopping/merchant/products:** Changed repeated flag of an existing field `gtin` in message `.google.shopping.merchant.products.v1beta.Attributes` ([#11247](https://github.com/googleapis/google-cloud-go/issues/11247)) ([279350e](https://github.com/googleapis/google-cloud-go/commit/279350e1703dd7f251603408bf0381d47b87ba60))


### Documentation

* **shopping/merchant/products:** A comment for field `gtin` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([279350e](https://github.com/googleapis/google-cloud-go/commit/279350e1703dd7f251603408bf0381d47b87ba60))
* **shopping/merchant/products:** A comment for field `max_handling_time` in message `.google.shopping.merchant.products.v1beta.Shipping` is changed ([279350e](https://github.com/googleapis/google-cloud-go/commit/279350e1703dd7f251603408bf0381d47b87ba60))
* **shopping/merchant/products:** A comment for field `max_transit_time` in message `.google.shopping.merchant.products.v1beta.Shipping` is changed ([279350e](https://github.com/googleapis/google-cloud-go/commit/279350e1703dd7f251603408bf0381d47b87ba60))
* **shopping/merchant/products:** A comment for field `min_handling_time` in message `.google.shopping.merchant.products.v1beta.Shipping` is changed ([279350e](https://github.com/googleapis/google-cloud-go/commit/279350e1703dd7f251603408bf0381d47b87ba60))
* **shopping/merchant/products:** A comment for field `min_transit_time` in message `.google.shopping.merchant.products.v1beta.Shipping` is changed ([279350e](https://github.com/googleapis/google-cloud-go/commit/279350e1703dd7f251603408bf0381d47b87ba60))
* **shopping/merchant/products:** A comment for field `name` in message `.google.shopping.merchant.products.v1beta.DeleteProductInputRequest` is changed ([279350e](https://github.com/googleapis/google-cloud-go/commit/279350e1703dd7f251603408bf0381d47b87ba60))
* **shopping/merchant/products:** A comment for field `name` in message `.google.shopping.merchant.products.v1beta.GetProductRequest` is changed ([279350e](https://github.com/googleapis/google-cloud-go/commit/279350e1703dd7f251603408bf0381d47b87ba60))
* **shopping/merchant/products:** A comment for field `name` in message `.google.shopping.merchant.products.v1beta.Product` is changed ([279350e](https://github.com/googleapis/google-cloud-go/commit/279350e1703dd7f251603408bf0381d47b87ba60))
* **shopping/merchant/products:** A comment for field `name` in message `.google.shopping.merchant.products.v1beta.ProductInput` is changed ([279350e](https://github.com/googleapis/google-cloud-go/commit/279350e1703dd7f251603408bf0381d47b87ba60))
* **shopping/merchant/products:** A comment for field `page_size` in message `.google.shopping.merchant.products.v1beta.ListProductsRequest` is changed ([279350e](https://github.com/googleapis/google-cloud-go/commit/279350e1703dd7f251603408bf0381d47b87ba60))
* **shopping/merchant/products:** A comment for field `tax_category` in message `.google.shopping.merchant.products.v1beta.Attributes` is changed ([279350e](https://github.com/googleapis/google-cloud-go/commit/279350e1703dd7f251603408bf0381d47b87ba60))
* **shopping/merchant/products:** A comment for message `Product` is changed ([279350e](https://github.com/googleapis/google-cloud-go/commit/279350e1703dd7f251603408bf0381d47b87ba60))
* **shopping/merchant/products:** A comment for message `ProductInput` is changed ([279350e](https://github.com/googleapis/google-cloud-go/commit/279350e1703dd7f251603408bf0381d47b87ba60))

## [0.12.1](https://github.com/googleapis/google-cloud-go/compare/shopping/v0.12.0...shopping/v0.12.1) (2024-12-04)


### Documentation

* **shopping/css:** Fix comment on list account labels ([d3de944](https://github.com/googleapis/google-cloud-go/commit/d3de9448192d4caf8506964cdc494d33f6b82070))

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
