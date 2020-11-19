# Changes

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
