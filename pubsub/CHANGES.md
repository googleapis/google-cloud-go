# Changes

### [1.8.3](https://www.github.com/googleapis/google-cloud-go/compare/pubsub/v1.8.2...v1.8.3) (2020-11-10)


### Bug Fixes

* **pubsub:** retry deadline exceeded errors in Acknowledge ([#3157](https://www.github.com/googleapis/google-cloud-go/issues/3157)) ([ae75b46](https://www.github.com/googleapis/google-cloud-go/commit/ae75b46033d9f14f41c1bde4b9646c93f8e2bbad))

## v1.8.2

- Fixes:
  - fix(pubsub): track errors in published messages opencensus metric (#2970)
  - fix(pubsub): do not propagate context deadline exceeded error (#3055)

## v1.8.1

- Suppress connection is closing on error on subscriber close. (#2951)

## v1.8.0

- Add status code to error injection in pstest. This is a BREAKING CHANGE.

## v1.7.0

- Add reactor options to pstest server. (#2916)

## v1.6.2

- Make message.Modacks thread safe in pstest. (#2755)
- Fix issue with closing publisher and subscriber client errors. (#2867)
- Fix updating subscription filtering/retry policy in pstest. (#2901)

## v1.6.1

- Fix issue where EnableMessageOrdering wasn't being parsed properly to `SubscriptionConfig`.

## v1.6.0

- Fix issue where subscriber streams were limited because it was using a single grpc conn.
  - As a side effect, publisher and subscriber grpc conns are no longer shared.
- Add fake time function in pstest.
- Add support for server side flow control.

## v1.5.0

- Add support for subscription detachment.
- Add support for message filtering in subscriptions.
- Add support for RetryPolicy (server-side feature).
- Fix publish error path when ordering key is disabled.
- Fix panic on Topic.ResumePublish method.

## v1.4.0

- Add support for upcoming ordering keys feature.

## v1.3.1

- Fix bug with removing dead letter policy from a subscription
- Set default value of MaxExtensionPeriod to 0, which is functionally equivalent

## v1.3.0

- Update cloud.google.com/go to v0.54.0

## v1.2.0

- Add support for upcoming dead letter topics feature
- Expose Subscription.ReceiveSettings.MaxExtensionPeriod setting
- Standardize default settings with other client libraries
  - Increase publish delay threshold from 1ms to 10ms
  - Increase subscription MaxExtension from 10m to 60m
- Always send keepalive/heartbeat ping on StreamingPull streams to minimize
  stream reopen requests

## v1.1.0

- Limit default grpc connections to 4.
- Fix issues with OpenCensus metric for pull count not including synchronous pull messages.
- Fix issue with publish bundle size calculations.
- Add ClearMessages method to pstest server.

## v1.0.1

Small fix to a package name.

## v1.0.0

This is the first tag to carve out pubsub as its own module. See:
https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository.
