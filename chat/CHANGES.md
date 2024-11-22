# Changelog

## [0.7.1](https://github.com/googleapis/google-cloud-go/compare/chat/v0.7.0...chat/v0.7.1) (2024-10-23)


### Bug Fixes

* **chat:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **chat:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [0.7.0](https://github.com/googleapis/google-cloud-go/compare/chat/v0.6.0...chat/v0.7.0) (2024-10-09)


### Features

* **chat:** Add doc for import mode external users support ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **chat:** Add doc for permission settings & announcement space support ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))


### Documentation

* **chat:** Discoverable space docs improvement ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **chat:** Memberships API dev docs improvement ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **chat:** Messages API dev docs improvement ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))

## [0.6.0](https://github.com/googleapis/google-cloud-go/compare/chat/v0.5.0...chat/v0.6.0) (2024-09-19)


### Features

* **chat:** If you're a domain administrator or a delegated administrator, you can now include the `useAdminAccess` parameter when you call the Chat API with your administrator privileges with the following methods to manage Chat spaces and memberships... ([#10858](https://github.com/googleapis/google-cloud-go/issues/10858)) ([b45d2ee](https://github.com/googleapis/google-cloud-go/commit/b45d2ee9488b74505c045d009835875e3e2291fe))


### Documentation

* **chat:** A comment for field `filter` in message `.google.chat.v1.ListMembershipsRequest` is updated to support `!=` operator ([b45d2ee](https://github.com/googleapis/google-cloud-go/commit/b45d2ee9488b74505c045d009835875e3e2291fe))

## [0.5.0](https://github.com/googleapis/google-cloud-go/compare/chat/v0.4.0...chat/v0.5.0) (2024-09-12)


### Features

* **chat:** Add CHAT_SPACE link type support for GA launch ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))


### Bug Fixes

* **chat:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/chat/v0.3.1...chat/v0.4.0) (2024-08-20)


### Features

* **chat:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [0.3.1](https://github.com/googleapis/google-cloud-go/compare/chat/v0.3.0...chat/v0.3.1) (2024-08-08)


### Bug Fixes

* **chat:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/chat/v0.2.0...chat/v0.3.0) (2024-07-24)


### Features

* **chat:** Add GetSpaceEvent and ListSpaceEvents APIs ([#10540](https://github.com/googleapis/google-cloud-go/issues/10540)) ([432de64](https://github.com/googleapis/google-cloud-go/commit/432de6473f74860812871ebc3ab930f04c0a65a8))


### Bug Fixes

* **chat:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/chat/v0.1.3...chat/v0.2.0) (2024-07-10)


### Features

* **chat:** Add doc for Discoverable Space support for GA launch ([#10505](https://github.com/googleapis/google-cloud-go/issues/10505)) ([a187451](https://github.com/googleapis/google-cloud-go/commit/a187451a912835703078e5b6a339c514edebe5de))


### Bug Fixes

* **chat:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))


### Documentation

* **chat:** Update resource naming formats ([a187451](https://github.com/googleapis/google-cloud-go/commit/a187451a912835703078e5b6a339c514edebe5de))

## [0.1.3](https://github.com/googleapis/google-cloud-go/compare/chat/v0.1.2...chat/v0.1.3) (2024-07-01)


### Bug Fixes

* **chat:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))


### Documentation

* **chat:** Update doc for `CreateMembership` in service `ChatService` to support group members ([eec7a3b](https://github.com/googleapis/google-cloud-go/commit/eec7a3b5c00fc18076f410ddc4910cdcc61c702c))
* **chat:** Update doc for `SetUpSpace` in service `ChatService` to support group members ([eec7a3b](https://github.com/googleapis/google-cloud-go/commit/eec7a3b5c00fc18076f410ddc4910cdcc61c702c))
* **chat:** Update doc for field `group_member` in message `google.chat.v1.Membership` ([eec7a3b](https://github.com/googleapis/google-cloud-go/commit/eec7a3b5c00fc18076f410ddc4910cdcc61c702c))

## [0.1.2](https://github.com/googleapis/google-cloud-go/compare/chat/v0.1.1...chat/v0.1.2) (2024-06-26)


### Bug Fixes

* **chat:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [0.1.1](https://github.com/googleapis/google-cloud-go/compare/chat/v0.1.0...chat/v0.1.1) (2024-05-16)


### Documentation

* **chat:** Update Chat API comments ([292e812](https://github.com/googleapis/google-cloud-go/commit/292e81231b957ae7ac243b47b8926564cee35920))

## 0.1.0 (2024-05-01)


### Features

* **chat:** Add Chat read state APIs ([1d757c6](https://github.com/googleapis/google-cloud-go/commit/1d757c66478963d6cbbef13fee939632c742759c))
* **chat:** Add UpdateMembership API ([1d757c6](https://github.com/googleapis/google-cloud-go/commit/1d757c66478963d6cbbef13fee939632c742759c))


### Documentation

* **chat:** Chat API documentation update ([1d757c6](https://github.com/googleapis/google-cloud-go/commit/1d757c66478963d6cbbef13fee939632c742759c))

## Changes
