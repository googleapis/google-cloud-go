# Changes



## [1.28.0](https://github.com/googleapis/google-cloud-go/releases/tag/maps%2Fv1.28.0) (2026-02-19)

### Features

* Add `googleMapsLinks` field for links to different actions on Google Maps ([d3eb851](https://github.com/googleapis/google-cloud-go/commit/d3eb851d1b09f28ae2b6c7c63d694eb67b0c11c7))
* Add `googleMapsTypeLabel` field to show type information on Google Maps ([d3eb851](https://github.com/googleapis/google-cloud-go/commit/d3eb851d1b09f28ae2b6c7c63d694eb67b0c11c7))

### Bug Fixes

* An existing field `references` is removed from message `.google.maps.places.v1.ContentBlock` ([d3eb851](https://github.com/googleapis/google-cloud-go/commit/d3eb851d1b09f28ae2b6c7c63d694eb67b0c11c7))
* An existing field `topic` is removed from message `.google.maps.places.v1.ContentBlock` ([d3eb851](https://github.com/googleapis/google-cloud-go/commit/d3eb851d1b09f28ae2b6c7c63d694eb67b0c11c7))

### Documentation

* A comment for field `photos` in message `.google.maps.places.v1.ContextualContent` is changed ([d3eb851](https://github.com/googleapis/google-cloud-go/commit/d3eb851d1b09f28ae2b6c7c63d694eb67b0c11c7))
* A comment for field `reviews` in message `.google.maps.places.v1.ContextualContent` is changed ([d3eb851](https://github.com/googleapis/google-cloud-go/commit/d3eb851d1b09f28ae2b6c7c63d694eb67b0c11c7))

## [1.27.0](https://github.com/googleapis/google-cloud-go/releases/tag/maps%2Fv1.27.0) (2026-02-12)

### Documentation

* Add pre-GA details for Experiments enum ([6f31019](https://github.com/googleapis/google-cloud-go/commit/6f310199e136b133bb4fadaa353e264e809db6d7))
* Add satellite imagery to `rgb_url` description ([6f31019](https://github.com/googleapis/google-cloud-go/commit/6f310199e136b133bb4fadaa353e264e809db6d7))

## [1.26.0](https://github.com/googleapis/google-cloud-go/releases/tag/maps%2Fv1.26.0) (2025-11-06)

### Features

* Add Place.consumerAlert field for suspicious review activity ([84cf55a](https://github.com/googleapis/google-cloud-go/commit/84cf55a9adab99b310134e04b562eddfa4d63c67))
* Add Review.visitDate field to indicate when the review author visited ([84cf55a](https://github.com/googleapis/google-cloud-go/commit/84cf55a9adab99b310134e04b562eddfa4d63c67))

## [1.25.0](https://github.com/googleapis/google-cloud-go/releases/tag/maps%2Fv1.25.0) (2025-10-22)

### Features

* Add moved_place and moved_place_id to Place 
* add OptimizeToursLongRunning method 
* add OptimizeToursUri method 
* add experimental Load Costs feature 
* add experimental U-turn Avoidance feature 

### Documentation

* A comment for enum `TravelMode` is changed 
* A comment for field `duration_distance_matrices` in message `.google.maps.routeoptimization.v1.ShipmentModel` is changed 
* A comment for field `place_id` in message `.google.maps.routeoptimization.v1.Waypoint` is changed 
* A comment for field `precedence_rules` in message `.google.maps.routeoptimization.v1.ShipmentModel` is changed 
* Change comments in some fields in Places API 

## [1.24.0](https://github.com/googleapis/google-cloud-go/releases/tag/maps%2Fv1.24.0) (2025-10-10)

### Features

* Upgrade to Go 1.24 (#12781) ([4f1129c](https://github.com/googleapis/google-cloud-go/commit/4f1129c))

### Bug Fixes

* upgrade gRPC service registration func (#13014) ([b80fb13](https://github.com/googleapis/google-cloud-go/commit/b80fb13fbee3f0e2a152337304e9cf8f7762cb72))

## [1.23.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.22.0...maps/v1.23.0) (2025-08-18)


### Features

* **maps/solar:** Add `experiments` param to endpoints for access to pre-GA features ([f1de706](https://github.com/googleapis/google-cloud-go/commit/f1de7062db662aa6dfbf1e8cd2f0ac5df678e76d))
* **maps/solar:** Add new `ImageryQuality` tier for allowing satellite-based Solar API results ([f1de706](https://github.com/googleapis/google-cloud-go/commit/f1de7062db662aa6dfbf1e8cd2f0ac5df678e76d))


### Documentation

* **maps/solar:** Add more detailed descriptions for each `ImageryQuality` tier ([f1de706](https://github.com/googleapis/google-cloud-go/commit/f1de7062db662aa6dfbf1e8cd2f0ac5df678e76d))

## [1.22.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.21.1...maps/v1.22.0) (2025-07-16)


### Features

* **maps/addressvalidation:** Add has_spell_corrected_components field to the Address Validation verdict in the API response ([f3c3c1a](https://github.com/googleapis/google-cloud-go/commit/f3c3c1ace2e36aa5e5c4c73e39a42cf3fbe2dbcb))


### Documentation

* **maps/addressvalidation:** Update proto field descriptions ([f3c3c1a](https://github.com/googleapis/google-cloud-go/commit/f3c3c1ace2e36aa5e5c4c73e39a42cf3fbe2dbcb))

## [1.21.1](https://github.com/googleapis/google-cloud-go/compare/maps/v1.21.0...maps/v1.21.1) (2025-07-09)


### Documentation

* **maps/places:** Clarify behavior of primary types and weekday text ([98ba6f0](https://github.com/googleapis/google-cloud-go/commit/98ba6f06e69685bca510ca85c12124434f9ba1e8))

## [1.21.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.20.4...maps/v1.21.0) (2025-05-21)


### Features

* **maps/places:** Add address descriptor proto ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))
* **maps/places:** Add new GoogleMapsLinks fields ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))
* **maps/places:** Add new NeighborhoodSummary fields ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))
* **maps/places:** Add new photo fields ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))
* **maps/places:** Add new review fields ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))
* **maps/places:** Add new ReviewSummary fields ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))
* **maps/places:** Update GenerativeSummary fields ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))


### Bug Fixes

* **maps/places:** An existing field `area_summary` is removed from message `.google.maps.places.v1.Place` ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))
* **maps/places:** An existing field `description` is removed from message `.google.maps.places.v1.Place` ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))
* **maps/places:** An existing field `references` is removed from message `.google.maps.places.v1.Place` ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))
* **maps/places:** An existing message `AreaSummary` is removed ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))


### Documentation

* **maps/places:** Change comments for some fields in Places API ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))

## [1.20.4](https://github.com/googleapis/google-cloud-go/compare/maps/v1.20.3...maps/v1.20.4) (2025-04-30)


### Documentation

* **maps/areainsights:** A comment for method `ComputeInsights` in service `AreaInsights` is changed ([a95a0bf](https://github.com/googleapis/google-cloud-go/commit/a95a0bf4172b8a227955a0353fd9c845f4502411))
* **maps/areainsights:** A comment for service `AreaInsights` is changed ([a95a0bf](https://github.com/googleapis/google-cloud-go/commit/a95a0bf4172b8a227955a0353fd9c845f4502411))
* **maps/areainsights:** Clarifications of comments ([a95a0bf](https://github.com/googleapis/google-cloud-go/commit/a95a0bf4172b8a227955a0353fd9c845f4502411))
* **maps/areainsights:** Sample requests rewritten as JSON ([a95a0bf](https://github.com/googleapis/google-cloud-go/commit/a95a0bf4172b8a227955a0353fd9c845f4502411))

## [1.20.3](https://github.com/googleapis/google-cloud-go/compare/maps/v1.20.2...maps/v1.20.3) (2025-04-22)


### Documentation

* **maps/areainsights:** A comment for field `place` in message `.google.maps.areainsights.v1.LocationFilter` is changed ([fe831f9](https://github.com/googleapis/google-cloud-go/commit/fe831f9b125baf2cf5774ad892361df2d655814a))
* **maps/areainsights:** A comment for field `place` in message `.google.maps.areainsights.v1.PlaceInsight` is changed ([fe831f9](https://github.com/googleapis/google-cloud-go/commit/fe831f9b125baf2cf5774ad892361df2d655814a))

## [1.20.2](https://github.com/googleapis/google-cloud-go/compare/maps/v1.20.1...maps/v1.20.2) (2025-04-15)


### Bug Fixes

* **maps:** Update google.golang.org/api to 0.229.0 ([3319672](https://github.com/googleapis/google-cloud-go/commit/3319672f3dba84a7150772ccb5433e02dab7e201))

## [1.20.1](https://github.com/googleapis/google-cloud-go/compare/maps/v1.20.0...maps/v1.20.1) (2025-03-13)


### Bug Fixes

* **maps:** Update golang.org/x/net to 0.37.0 ([1144978](https://github.com/googleapis/google-cloud-go/commit/11449782c7fb4896bf8b8b9cde8e7441c84fb2fd))

## [1.20.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.19.0...maps/v1.20.0) (2025-03-12)


### Features

* **maps/places:** Add postal_address and time_zone to Place ([b323f88](https://github.com/googleapis/google-cloud-go/commit/b323f888e349168b4e30a9edb4630dfb6060ed4d))


### Documentation

* **maps/places:** Change comments in some fields in Places API ([#11798](https://github.com/googleapis/google-cloud-go/issues/11798)) ([b323f88](https://github.com/googleapis/google-cloud-go/commit/b323f888e349168b4e30a9edb4630dfb6060ed4d))

## [1.19.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.18.0...maps/v1.19.0) (2025-02-26)


### Features

* **maps/places:** Add NACS EVCS connector type support ([794ecf7](https://github.com/googleapis/google-cloud-go/commit/794ecf77993a83fcad01912fb066366ba16adc11))

## [1.18.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.17.1...maps/v1.18.0) (2025-01-30)


### Features

* **maps/places:** Add more fuel type enum values ([90edd74](https://github.com/googleapis/google-cloud-go/commit/90edd74d13b9dd737134a75d5b18a064a8ee656a))


### Bug Fixes

* **maps/places:** Add oauth scopes to Places proto ([#11518](https://github.com/googleapis/google-cloud-go/issues/11518)) ([afbe132](https://github.com/googleapis/google-cloud-go/commit/afbe1326ff6d25034eb232129019b2e2687df7d0))


### Documentation

* **maps/places:** A comment for enum value `LPG` in enum `FuelType` is changed to be clearer. ([90edd74](https://github.com/googleapis/google-cloud-go/commit/90edd74d13b9dd737134a75d5b18a064a8ee656a))

## [1.17.1](https://github.com/googleapis/google-cloud-go/compare/maps/v1.17.0...maps/v1.17.1) (2025-01-02)


### Bug Fixes

* **maps:** Update golang.org/x/net to v0.33.0 ([e9b0b69](https://github.com/googleapis/google-cloud-go/commit/e9b0b69644ea5b276cacff0a707e8a5e87efafc9))

## [1.17.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.16.0...maps/v1.17.0) (2024-12-11)


### Features

* **maps/routing:** Add API for experimental flyover and narrow road polyline details ([#11220](https://github.com/googleapis/google-cloud-go/issues/11220)) ([57fdec7](https://github.com/googleapis/google-cloud-go/commit/57fdec7ce3792753c419298b9e526c4889f4101d))


### Documentation

* **maps/fleetengine:** Correct SearchVehiclesRequest.ordered_by description ([76266a6](https://github.com/googleapis/google-cloud-go/commit/76266a619b4a2178c21de4d330f751bffc84994e))
* **maps/routeoptimization:** Update reference doc links ([38385d4](https://github.com/googleapis/google-cloud-go/commit/38385d441ba43e7bf6166ee5507a70e77c0b01f5))

## [1.16.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.15.0...maps/v1.16.0) (2024-12-04)


### Features

* **maps/routing:** Add API for shorter distance reference routes ([8dedb87](https://github.com/googleapis/google-cloud-go/commit/8dedb878c070cc1e92d62bb9b32358425e3ceffb))

## [1.15.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.14.1...maps/v1.15.0) (2024-11-14)


### Features

* **maps/places:** Update attributes in Places API ([f072178](https://github.com/googleapis/google-cloud-go/commit/f072178f6fd90537a5782395f4229e4c8b30af7e))

## [1.14.1](https://github.com/googleapis/google-cloud-go/compare/maps/v1.14.0...maps/v1.14.1) (2024-10-23)


### Bug Fixes

* **maps:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **maps:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.13.0...maps/v1.14.0) (2024-09-25)


### Features

* **maps/routeoptimization:** A new field `route_token` is added to message `.google.maps.routeoptimization.v1.ShipmentRoute.Transition` ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))
* **maps/routeoptimization:** Add support for generating route tokens ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))


### Documentation

* **maps/routeoptimization:** A comment for field `code` in message `.google.maps.routeoptimization.v1.OptimizeToursValidationError` is changed ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))
* **maps/routeoptimization:** A comment for field `populate_transition_polylines` in message `.google.maps.routeoptimization.v1.OptimizeToursRequest` is changed ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))
* **maps/routeoptimization:** A comment for method `BatchOptimizeTours` in service `RouteOptimization` is changed ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.12.1...maps/v1.13.0) (2024-09-19)


### Features

* **maps/places:** Add `routing_parameters` to SearchNearbyRequest and SearchTextRequest ([0b3c268](https://github.com/googleapis/google-cloud-go/commit/0b3c268c564ffe0d87b0efc716f08afaf064b4cc))
* **maps/routeoptimization:** A new field `cost_per_kilometer_below_soft_max` is added to message `.google.maps.routeoptimization.v1.DistanceLimit` ([ba22f7b](https://github.com/googleapis/google-cloud-go/commit/ba22f7b5b8f21a39685017d2d8522456ce528c4c))
* **maps/routeoptimization:** A new field `route_modifiers` is added to message `.google.maps.routeoptimization.v1.Vehicle` ([ba22f7b](https://github.com/googleapis/google-cloud-go/commit/ba22f7b5b8f21a39685017d2d8522456ce528c4c))
* **maps/routeoptimization:** A new message `RouteModifiers` is added ([ba22f7b](https://github.com/googleapis/google-cloud-go/commit/ba22f7b5b8f21a39685017d2d8522456ce528c4c))
* **maps/routeoptimization:** Minor fields and documentation update ([#10861](https://github.com/googleapis/google-cloud-go/issues/10861)) ([ba22f7b](https://github.com/googleapis/google-cloud-go/commit/ba22f7b5b8f21a39685017d2d8522456ce528c4c))
* **maps:** New clients ([#10867](https://github.com/googleapis/google-cloud-go/issues/10867)) ([338ca6e](https://github.com/googleapis/google-cloud-go/commit/338ca6e9a104c4cb9dff57d015ecb5b4dbd01bc5))


### Documentation

* **maps/routeoptimization:** A comment for enum value `CODE_UNSPECIFIED` in enum `Code` is changed ([ba22f7b](https://github.com/googleapis/google-cloud-go/commit/ba22f7b5b8f21a39685017d2d8522456ce528c4c))
* **maps/routeoptimization:** A comment for enum value `DEFAULT_SOLVE` in enum `SolvingMode` is changed ([ba22f7b](https://github.com/googleapis/google-cloud-go/commit/ba22f7b5b8f21a39685017d2d8522456ce528c4c))
* **maps/routeoptimization:** A comment for enum value `RELAX_VISIT_TIMES_AND_SEQUENCE_AFTER_THRESHOLD` in enum `Level` is changed ([ba22f7b](https://github.com/googleapis/google-cloud-go/commit/ba22f7b5b8f21a39685017d2d8522456ce528c4c))
* **maps/routeoptimization:** A comment for field `code` in message `.google.maps.routeoptimization.v1.OptimizeToursValidationError` is changed ([ba22f7b](https://github.com/googleapis/google-cloud-go/commit/ba22f7b5b8f21a39685017d2d8522456ce528c4c))
* **maps/routeoptimization:** A comment for field `reasons` in message `.google.maps.routeoptimization.v1.SkippedShipment` is changed ([ba22f7b](https://github.com/googleapis/google-cloud-go/commit/ba22f7b5b8f21a39685017d2d8522456ce528c4c))
* **maps/routeoptimization:** A comment for field `validation_errors` in message `.google.maps.routeoptimization.v1.OptimizeToursResponse` is changed ([ba22f7b](https://github.com/googleapis/google-cloud-go/commit/ba22f7b5b8f21a39685017d2d8522456ce528c4c))
* **maps/routeoptimization:** A comment for message `OptimizeToursValidationError` is changed ([ba22f7b](https://github.com/googleapis/google-cloud-go/commit/ba22f7b5b8f21a39685017d2d8522456ce528c4c))
* **maps/routeoptimization:** A comment for message `TimeWindow` is changed ([ba22f7b](https://github.com/googleapis/google-cloud-go/commit/ba22f7b5b8f21a39685017d2d8522456ce528c4c))
* **maps/routeoptimization:** A comment for method `BatchOptimizeTours` in service `RouteOptimization` is changed ([ba22f7b](https://github.com/googleapis/google-cloud-go/commit/ba22f7b5b8f21a39685017d2d8522456ce528c4c))

## [1.12.1](https://github.com/googleapis/google-cloud-go/compare/maps/v1.12.0...maps/v1.12.1) (2024-09-12)


### Bug Fixes

* **maps:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))


### Documentation

* **maps/fleetengine/delivery:** Update comment link for ListTasks filter ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.11.7...maps/v1.12.0) (2024-08-20)


### Features

* **maps:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.11.7](https://github.com/googleapis/google-cloud-go/compare/maps/v1.11.6...maps/v1.11.7) (2024-08-08)


### Bug Fixes

* **maps:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.11.6](https://github.com/googleapis/google-cloud-go/compare/maps/v1.11.5...maps/v1.11.6) (2024-08-01)


### Documentation

* **maps/fleetengine/delivery:** Document that delivery_vehicle.type can be set on CreateDeliveryVehicle ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))

## [1.11.5](https://github.com/googleapis/google-cloud-go/compare/maps/v1.11.4...maps/v1.11.5) (2024-07-24)


### Bug Fixes

* **maps:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))


### Documentation

* **maps/fleetengine/delivery:** Clarify behavior of UpdateDeliveryVehicle ([eb63f0d](https://github.com/googleapis/google-cloud-go/commit/eb63f0d4f42a06581e1425f99c2a03d52d6cb404))

## [1.11.4](https://github.com/googleapis/google-cloud-go/compare/maps/v1.11.3...maps/v1.11.4) (2024-07-10)


### Bug Fixes

* **maps:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.11.3](https://github.com/googleapis/google-cloud-go/compare/maps/v1.11.2...maps/v1.11.3) (2024-07-01)


### Bug Fixes

* **maps:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [1.11.2](https://github.com/googleapis/google-cloud-go/compare/maps/v1.11.1...maps/v1.11.2) (2024-06-26)


### Bug Fixes

* **maps:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.11.1](https://github.com/googleapis/google-cloud-go/compare/maps/v1.11.0...maps/v1.11.1) (2024-06-10)


### Bug Fixes

* **maps/places:** Update Go maps/places to unstable ([385b6ee](https://github.com/googleapis/google-cloud-go/commit/385b6ee9060f4dbfad2e12b1ab635edab7ec4466))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.10.0...maps/v1.11.0) (2024-05-29)


### Features

* **maps:** Removed mapsplatformdatasets v1alpha library ([dafecc9](https://github.com/googleapis/google-cloud-go/commit/dafecc9f28a6b028889c8cefb352e50f60563a4e))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.9.0...maps/v1.10.0) (2024-05-22)


### Features

* **maps/places:** Add `generative_summary` and `area_summary` for place summaries ([#10204](https://github.com/googleapis/google-cloud-go/issues/10204)) ([5238dbc](https://github.com/googleapis/google-cloud-go/commit/5238dbc48971a7295127be0f415280248608c6be))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.8.0...maps/v1.9.0) (2024-05-16)


### Features

* **maps:** FleetEngine and Delivery RPC turndown and removal ([e4543f8](https://github.com/googleapis/google-cloud-go/commit/e4543f87bbad42eb37f501a4571128c3a426780b))


### Bug Fixes

* **maps:** An existing message `SearchTasksRequest` is removed ([e4543f8](https://github.com/googleapis/google-cloud-go/commit/e4543f87bbad42eb37f501a4571128c3a426780b))
* **maps:** An existing message `SearchTasksResponse` is removed ([e4543f8](https://github.com/googleapis/google-cloud-go/commit/e4543f87bbad42eb37f501a4571128c3a426780b))
* **maps:** An existing message `UpdateVehicleLocationRequest` is removed ([e4543f8](https://github.com/googleapis/google-cloud-go/commit/e4543f87bbad42eb37f501a4571128c3a426780b))
* **maps:** An existing method `SearchFuzzedVehicles` is removed from service `VehicleService` ([e4543f8](https://github.com/googleapis/google-cloud-go/commit/e4543f87bbad42eb37f501a4571128c3a426780b))
* **maps:** An existing method `SearchTasks` is removed from service `DeliveryService` ([e4543f8](https://github.com/googleapis/google-cloud-go/commit/e4543f87bbad42eb37f501a4571128c3a426780b))
* **maps:** An existing method `UpdateVehicleLocation` is removed from service `VehicleService` ([e4543f8](https://github.com/googleapis/google-cloud-go/commit/e4543f87bbad42eb37f501a4571128c3a426780b))


### Documentation

* **maps/fleetengine/delivery:** Remove comment about deleted SearchTasks method ([292e812](https://github.com/googleapis/google-cloud-go/commit/292e81231b957ae7ac243b47b8926564cee35920))
* **maps/fleetengine:** Mark TerminalPointId as deprecated ([#10130](https://github.com/googleapis/google-cloud-go/issues/10130)) ([292e812](https://github.com/googleapis/google-cloud-go/commit/292e81231b957ae7ac243b47b8926564cee35920))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.7.3...maps/v1.8.0) (2024-05-08)


### Features

* **maps:** New clients ([#10129](https://github.com/googleapis/google-cloud-go/issues/10129)) ([97eb0f5](https://github.com/googleapis/google-cloud-go/commit/97eb0f5c93e8a4528a35910f9b0ab75a113a002c))

## [1.7.3](https://github.com/googleapis/google-cloud-go/compare/maps/v1.7.2...maps/v1.7.3) (2024-05-01)


### Bug Fixes

* **maps:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))


### Documentation

* **maps/fleetengine/delivery:** Correct link in ListTasks documentation ([1d757c6](https://github.com/googleapis/google-cloud-go/commit/1d757c66478963d6cbbef13fee939632c742759c))
* **maps/places:** Slightly improved documentation for EVOptions in SearchTextRequest ([1d757c6](https://github.com/googleapis/google-cloud-go/commit/1d757c66478963d6cbbef13fee939632c742759c))
* **maps/places:** Update comment of Places API ([1d757c6](https://github.com/googleapis/google-cloud-go/commit/1d757c66478963d6cbbef13fee939632c742759c))

## [1.7.2](https://github.com/googleapis/google-cloud-go/compare/maps/v1.7.1...maps/v1.7.2) (2024-04-15)


### Documentation

* **maps/places:** Fix designation of Text Search ([#9728](https://github.com/googleapis/google-cloud-go/issues/9728)) ([ce55ad6](https://github.com/googleapis/google-cloud-go/commit/ce55ad694f21cacfa608e9b9952ee31f8d566e49))
* **maps/places:** Fix typo in PriceLevel enum ([#9669](https://github.com/googleapis/google-cloud-go/issues/9669)) ([264a6dc](https://github.com/googleapis/google-cloud-go/commit/264a6dcddbffaec987dce1dc00f6550c263d2df7))
* **maps/routing:** Various formatting and grammar fixes for proto documentation ([cca3f47](https://github.com/googleapis/google-cloud-go/commit/cca3f47c895e7cac07d7d48ab3c4850b265a710f))

## [1.7.1](https://github.com/googleapis/google-cloud-go/compare/maps/v1.7.0...maps/v1.7.1) (2024-03-14)


### Bug Fixes

* **maps:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.6.4...maps/v1.7.0) (2024-02-21)


### Features

* **maps/addressvalidation:** Add session token support for Autocomplete (New) sessions that end with a call to Address Validation ([0195fe9](https://github.com/googleapis/google-cloud-go/commit/0195fe9292274ff9d86c71079a8e96ed2e5f9331))
* **maps/places:** Add AutoComplete API ([0195fe9](https://github.com/googleapis/google-cloud-go/commit/0195fe9292274ff9d86c71079a8e96ed2e5f9331))


### Documentation

* **maps/fleetengine/delivery:** Updated incorrect reference to `Task.journeySharingInfo` ([#9428](https://github.com/googleapis/google-cloud-go/issues/9428)) ([7e6c208](https://github.com/googleapis/google-cloud-go/commit/7e6c208c5d97d3f6e2f7fd7aca09b8ae98dc0bf2))

## [1.6.4](https://github.com/googleapis/google-cloud-go/compare/maps/v1.6.3...maps/v1.6.4) (2024-01-30)


### Bug Fixes

* **maps:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))


### Documentation

* **maps/fleetengine:** Update comment on Waypoint ([97d62c7](https://github.com/googleapis/google-cloud-go/commit/97d62c7a6a305c47670ea9c147edc444f4bf8620))
* **maps/fleetengine:** Update comment on Waypoint ([#9291](https://github.com/googleapis/google-cloud-go/issues/9291)) ([97d62c7](https://github.com/googleapis/google-cloud-go/commit/97d62c7a6a305c47670ea9c147edc444f4bf8620))

## [1.6.3](https://github.com/googleapis/google-cloud-go/compare/maps/v1.6.2...maps/v1.6.3) (2024-01-11)


### Documentation

* **maps/fleetengine:** Better comments on SearchVehicle fields ([c3f1174](https://github.com/googleapis/google-cloud-go/commit/c3f1174dc29d1c00d514a69590bd83f9b08a60d1))

## [1.6.2](https://github.com/googleapis/google-cloud-go/compare/maps/v1.6.1...maps/v1.6.2) (2023-12-11)


### Documentation

* **maps/places:** Change comments for some fields in Places API ([29effe6](https://github.com/googleapis/google-cloud-go/commit/29effe600e16f24a127a1422ec04263c4f7a600a))

## [1.6.1](https://github.com/googleapis/google-cloud-go/compare/maps/v1.6.0...maps/v1.6.1) (2023-11-01)


### Bug Fixes

* **maps:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.5.1...maps/v1.6.0) (2023-10-31)


### Features

* **maps/fleetengine/delivery:** Add default sensors for RawLocation & SupplementalLocation ([3053c79](https://github.com/googleapis/google-cloud-go/commit/3053c7933a05b1b1c10d7730c29b28688b218552))
* **maps/fleetengine:** Add default sensors for RawLocation & SupplementalLocation ([3053c79](https://github.com/googleapis/google-cloud-go/commit/3053c7933a05b1b1c10d7730c29b28688b218552))
* **maps/places:** New features for Places GA ([ffb0dda](https://github.com/googleapis/google-cloud-go/commit/ffb0ddabf3d9822ba8120cabaf25515fd32e9615))

## [1.5.1](https://github.com/googleapis/google-cloud-go/compare/maps/v1.5.0...maps/v1.5.1) (2023-10-26)


### Bug Fixes

* **maps:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.4.1...maps/v1.5.0) (2023-10-17)


### Features

* **maps:** New clients ([#8739](https://github.com/googleapis/google-cloud-go/issues/8739)) ([5f1d27a](https://github.com/googleapis/google-cloud-go/commit/5f1d27aae41ff75573fdab254da2548052556b1f))

## [1.4.1](https://github.com/googleapis/google-cloud-go/compare/maps/v1.4.0...maps/v1.4.1) (2023-10-12)


### Bug Fixes

* **maps:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.3.0...maps/v1.4.0) (2023-07-24)


### Features

* **maps/places:** Promote to GA ([#8299](https://github.com/googleapis/google-cloud-go/issues/8299)) ([08ec41a](https://github.com/googleapis/google-cloud-go/commit/08ec41aba981874a7b86a9a941b07f9eb2fc6ce1))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.2.1...maps/v1.3.0) (2023-07-10)


### Features

* **maps/routing:** Add HTML Navigation Instructions feature to ComputeRoutes ([a3ec3cf](https://github.com/googleapis/google-cloud-go/commit/a3ec3cf858c7d9154338ac4cd8a9a068dc7a7f4d))

## [1.2.1](https://github.com/googleapis/google-cloud-go/compare/maps/v1.2.0...maps/v1.2.1) (2023-06-20)


### Bug Fixes

* **maps:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.1.0...maps/v1.2.0) (2023-05-30)


### Features

* **maps:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.1.0](https://github.com/googleapis/google-cloud-go/compare/maps/v1.0.1...maps/v1.1.0) (2023-05-16)


### Features

* **maps/places:** Start generating apiv1 ([#7919](https://github.com/googleapis/google-cloud-go/issues/7919)) ([ee10cfd](https://github.com/googleapis/google-cloud-go/commit/ee10cfd2e59a3d228af2dd8c56f5229cf6c577f0))

## [1.0.1](https://github.com/googleapis/google-cloud-go/compare/maps/v1.0.0...maps/v1.0.1) (2023-05-08)


### Bug Fixes

* **maps:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.0.0](https://github.com/googleapis/google-cloud-go/compare/maps/v0.7.0...maps/v1.0.0) (2023-04-04)


### Features

* **maps/addressvalidation:** Promote to GA ([fce42e0](https://github.com/googleapis/google-cloud-go/commit/fce42e0e6764e27760cf6f137b66fed45145ebf8))
* **maps/routing:** Promote to GA ([fce42e0](https://github.com/googleapis/google-cloud-go/commit/fce42e0e6764e27760cf6f137b66fed45145ebf8))
* **maps:** Promote to GA ([#7639](https://github.com/googleapis/google-cloud-go/issues/7639)) ([d0302eb](https://github.com/googleapis/google-cloud-go/commit/d0302ebe0dfc9b4d9274db33b3947e90559b068f))

## [0.7.0](https://github.com/googleapis/google-cloud-go/compare/maps/v0.6.0...maps/v0.7.0) (2023-03-22)


### Features

* **maps/routing:** Added support for specifying waypoints as addresses docs: clarified usage of RouteLegStepTravelAdvisory in comment ([00fff3a](https://github.com/googleapis/google-cloud-go/commit/00fff3a58bed31274ab39af575876dab91d708c9))
* **maps/routing:** Adds support for specifying region_code in the ComputeRoutesRequest feat: adds support for specifying region_code and language_code in the ComputeRouteMatrixRequest ([00fff3a](https://github.com/googleapis/google-cloud-go/commit/00fff3a58bed31274ab39af575876dab91d708c9))


### Documentation

* **maps/routing:** Clarify usage of compute_alternative_routes in proto comment ([00fff3a](https://github.com/googleapis/google-cloud-go/commit/00fff3a58bed31274ab39af575876dab91d708c9))

## [0.6.0](https://github.com/googleapis/google-cloud-go/compare/maps/v0.5.0...maps/v0.6.0) (2023-02-14)


### Features

* **maps/mapsplatformdatasets:** Start generating apiv1alpha ([#7386](https://github.com/googleapis/google-cloud-go/issues/7386)) ([6ec787f](https://github.com/googleapis/google-cloud-go/commit/6ec787fb392cd3c82a3ce608489e4d6e358eccbc))

## [0.5.0](https://github.com/googleapis/google-cloud-go/compare/maps-v0.4.0...maps/v0.5.0) (2023-01-26)


### Features

* **maps/addressvalidation:** Start generating apiv1 ([#7012](https://github.com/googleapis/google-cloud-go/issues/7012)) ([3e88250](https://github.com/googleapis/google-cloud-go/commit/3e882501ea196ff4f122989e5726bfd4c72e5133))
* **maps/routing:** Add ExtraComputations feature to ComputeRoutes and ComputeRouteMatrix ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **maps/routing:** Start generating apiv2 ([#7056](https://github.com/googleapis/google-cloud-go/issues/7056)) ([1b7993d](https://github.com/googleapis/google-cloud-go/commit/1b7993d6931cf33bab07124da4180eeb3faffe7e))
* **maps:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))


### Bug Fixes

* **maps/addressvalidation:** Return to grpc-only transport for C# ([19e9d03](https://github.com/googleapis/google-cloud-go/commit/19e9d033c263e889d32b74c4c853c440ce136d68))

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/maps-v0.3.0...maps/v0.4.0) (2023-01-26)


### Features

* **maps/addressvalidation:** Start generating apiv1 ([#7012](https://github.com/googleapis/google-cloud-go/issues/7012)) ([3e88250](https://github.com/googleapis/google-cloud-go/commit/3e882501ea196ff4f122989e5726bfd4c72e5133))
* **maps/routing:** Add ExtraComputations feature to ComputeRoutes and ComputeRouteMatrix ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **maps/routing:** Start generating apiv2 ([#7056](https://github.com/googleapis/google-cloud-go/issues/7056)) ([1b7993d](https://github.com/googleapis/google-cloud-go/commit/1b7993d6931cf33bab07124da4180eeb3faffe7e))
* **maps:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))


### Bug Fixes

* **maps/addressvalidation:** Return to grpc-only transport for C# ([19e9d03](https://github.com/googleapis/google-cloud-go/commit/19e9d033c263e889d32b74c4c853c440ce136d68))

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/maps/v0.2.0...maps/v0.3.0) (2023-01-26)


### Features

* **maps/routing:** Add ExtraComputations feature to ComputeRoutes and ComputeRouteMatrix ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))


### Bug Fixes

* **maps/addressvalidation:** Return to grpc-only transport for C# ([19e9d03](https://github.com/googleapis/google-cloud-go/commit/19e9d033c263e889d32b74c4c853c440ce136d68))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/maps/v0.1.0...maps/v0.2.0) (2023-01-04)


### Features

* **maps:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## 0.1.0 (2022-11-16)


### Features

* **maps/addressvalidation:** Start generating apiv1 ([#7012](https://github.com/googleapis/google-cloud-go/issues/7012)) ([3e88250](https://github.com/googleapis/google-cloud-go/commit/3e882501ea196ff4f122989e5726bfd4c72e5133))
* **maps/routing:** Start generating apiv2 ([#7056](https://github.com/googleapis/google-cloud-go/issues/7056)) ([1b7993d](https://github.com/googleapis/google-cloud-go/commit/1b7993d6931cf33bab07124da4180eeb3faffe7e))

