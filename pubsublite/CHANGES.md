# Changes

## [0.5.0](https://www.github.com/googleapis/google-cloud-go/compare/v0.4.0...v0.5.0) (2021-01-07)


### Features

* **pubsublite:** add client library metadata to headers ([#3458](https://www.github.com/googleapis/google-cloud-go/issues/3458)) ([8226811](https://www.github.com/googleapis/google-cloud-go/commit/822681105bc13f1e1f0784c4557faf849c1110b4))
* **pubsublite:** publisher client ([#3303](https://www.github.com/googleapis/google-cloud-go/issues/3303)) ([1648ea0](https://www.github.com/googleapis/google-cloud-go/commit/1648ea06bbb08c3452f79551a9d45147379f13e4))
* **pubsublite:** settings and message transforms for Cloud Pub/Sub shim ([#3281](https://www.github.com/googleapis/google-cloud-go/issues/3281)) ([74923c2](https://www.github.com/googleapis/google-cloud-go/commit/74923c27efd7936b3e18cd8ccb72882a40c7ff42))
* **pubsublite:** subscriber client ([#3442](https://www.github.com/googleapis/google-cloud-go/issues/3442)) ([221bfba](https://www.github.com/googleapis/google-cloud-go/commit/221bfbae54107486ab9060b950081faa27489d1c))


### Bug Fixes

* **pubsublite:** return an error if no topic or subscription fields were updated ([#3502](https://www.github.com/googleapis/google-cloud-go/issues/3502)) ([a875969](https://www.github.com/googleapis/google-cloud-go/commit/a87596942d39fbfe47427c007e4029bd9be2ca0e))

## [0.4.0](https://www.github.com/googleapis/google-cloud-go/compare/pubsublite/v0.3.0...v0.4.0) (2020-12-09)


### Features

pubsublite/internal/wire implementation:

* **pubsublite:** assigning subscriber ([#3238](https://www.github.com/googleapis/google-cloud-go/issues/3238)) ([d1c03da](https://www.github.com/googleapis/google-cloud-go/commit/d1c03dae383f5a175e4237d5f46dc1bdc2cd33f0))
* **pubsublite:** committer ([#3198](https://www.github.com/googleapis/google-cloud-go/issues/3198)) ([ecc706b](https://www.github.com/googleapis/google-cloud-go/commit/ecc706b03079c6521a31e1066b00677aaf51e7dd))
* **pubsublite:** receive settings ([#3195](https://www.github.com/googleapis/google-cloud-go/issues/3195)) ([bd837fc](https://www.github.com/googleapis/google-cloud-go/commit/bd837fc9aad4181b8aa574e41341000755875eca))
* **pubsublite:** routing publisher ([#3277](https://www.github.com/googleapis/google-cloud-go/issues/3277)) ([88e5466](https://www.github.com/googleapis/google-cloud-go/commit/88e546600c7d4f7570530aa72355f51f44187890))
* **pubsublite:** single and multi partition subscribers ([#3221](https://www.github.com/googleapis/google-cloud-go/issues/3221)) ([299b803](https://www.github.com/googleapis/google-cloud-go/commit/299b803aaee9a0dc0b2ec8c81fac66341045b8b2))
* **pubsublite:** single partition publisher ([#3225](https://www.github.com/googleapis/google-cloud-go/issues/3225)) ([4982eeb](https://www.github.com/googleapis/google-cloud-go/commit/4982eeb32ebe85de211ae09d13fdaf6140d9e115))


### Bug Fixes

* **pubsublite:** fixed return value of AdminClient.TopicSubscriptions ([#3220](https://www.github.com/googleapis/google-cloud-go/issues/3220)) ([f37f118](https://www.github.com/googleapis/google-cloud-go/commit/f37f118c87d4d0a77a554515a430ae06e5852294))

## [0.3.0](https://www.github.com/googleapis/google-cloud-go/compare/pubsublite/v0.2.0...v0.3.0) (2020-11-10)


### Features

* **pubsublite:** Added Pub/Sub Lite clients and routing headers ([#3105](https://www.github.com/googleapis/google-cloud-go/issues/3105)) ([98668fa](https://www.github.com/googleapis/google-cloud-go/commit/98668fa5457d26ed34debee708614f027020e5bc))
* **pubsublite:** Flow controller and offset tracker for the subscriber ([#3132](https://www.github.com/googleapis/google-cloud-go/issues/3132)) ([5899bdd](https://www.github.com/googleapis/google-cloud-go/commit/5899bdd7d6d5eac96e42e1baa1bd5e905e767a17))
* **pubsublite:** Mock server and utils for unit tests ([#3092](https://www.github.com/googleapis/google-cloud-go/issues/3092)) ([586592e](https://www.github.com/googleapis/google-cloud-go/commit/586592ef5875667e65e19e3662fe532b26293172))
* **pubsublite:** Move internal implementation details to internal/wire subpackage ([#3123](https://www.github.com/googleapis/google-cloud-go/issues/3123)) ([ed3fd1a](https://www.github.com/googleapis/google-cloud-go/commit/ed3fd1aed7dbc9396aecc70622ccfd302bbb4265))
* **pubsublite:** Periodic background task ([#3152](https://www.github.com/googleapis/google-cloud-go/issues/3152)) ([58c12cc](https://www.github.com/googleapis/google-cloud-go/commit/58c12ccba01cfe3b320e2e83d7ca1145f1e310d7))
* **pubsublite:** Test utils for streams ([#3153](https://www.github.com/googleapis/google-cloud-go/issues/3153)) ([5bb2b02](https://www.github.com/googleapis/google-cloud-go/commit/5bb2b0218d355bc558b03f24db1a0786a3489cac))
* **pubsublite:** Trackers for acks and commit cursor ([#3137](https://www.github.com/googleapis/google-cloud-go/issues/3137)) ([26599a0](https://www.github.com/googleapis/google-cloud-go/commit/26599a0995d9b108bbaaceca775457ffc331dcb2))

## v0.2.0

- Features
  - feat(pubsublite): Types for resource paths and topic/subscription configs (#3026)
  - feat(pubsublite): Pub/Sub Lite admin client (#3036)

## v0.1.0

This is the first tag to carve out pubsublite as its own module. See:
https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository.
