# Changelog

## [0.15.0](https://github.com/googleapis/google-cloud-go/compare/chat/v0.14.0...chat/v0.15.0) (2025-07-31)


### Features

* **chat:** Addition of app auth support for chat api ([#12611](https://github.com/googleapis/google-cloud-go/issues/12611)) ([c574e28](https://github.com/googleapis/google-cloud-go/commit/c574e287f49cc1c3b069b35d95b98da2bc9b948f))


### Documentation

* **chat:** Update reference documentation for createSpace,updateSpace,deleteSpace,createMembership,updateMembership,deleteMembership and the newly added field -customer- in space.proto ([c574e28](https://github.com/googleapis/google-cloud-go/commit/c574e287f49cc1c3b069b35d95b98da2bc9b948f))

## [0.14.0](https://github.com/googleapis/google-cloud-go/compare/chat/v0.13.1...chat/v0.14.0) (2025-07-23)


### Features

* **chat:** Exposing 1p integration message content (drive, calendar, huddle, meet chips) ([#12589](https://github.com/googleapis/google-cloud-go/issues/12589)) ([ac4970b](https://github.com/googleapis/google-cloud-go/commit/ac4970b5a6318dbfcdca7da5ee256852ca49ea23))


### Documentation

* **chat:** Update reference documentation for annotations. Introduce new richlink metadata types ([ac4970b](https://github.com/googleapis/google-cloud-go/commit/ac4970b5a6318dbfcdca7da5ee256852ca49ea23))

## [0.13.1](https://github.com/googleapis/google-cloud-go/compare/chat/v0.13.0...chat/v0.13.1) (2025-06-04)


### Bug Fixes

* **chat:** Fix: upgrade gRPC service registration func ([6a871e0](https://github.com/googleapis/google-cloud-go/commit/6a871e0f6924980da4fec78405bfe0736522afa8))

## [0.13.0](https://github.com/googleapis/google-cloud-go/compare/chat/v0.12.2...chat/v0.13.0) (2025-05-06)


### Features

* **chat:** A new method `customEmojis.create` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **chat:** A new method `customEmojis.delete` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **chat:** A new method `customEmojis.get` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **chat:** A new method `customEmojis.list` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))


### Documentation

* **chat:** A comment for field `filter` in message `.google.chat.v1.ListReactionsRequest` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **chat:** A comment for message `CustomEmoji` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))

## [0.12.2](https://github.com/googleapis/google-cloud-go/compare/chat/v0.12.1...chat/v0.12.2) (2025-04-15)


### Bug Fixes

* **chat:** Update google.golang.org/api to 0.229.0 ([3319672](https://github.com/googleapis/google-cloud-go/commit/3319672f3dba84a7150772ccb5433e02dab7e201))

## [0.12.1](https://github.com/googleapis/google-cloud-go/compare/chat/v0.12.0...chat/v0.12.1) (2025-03-13)


### Bug Fixes

* **chat:** Update golang.org/x/net to 0.37.0 ([1144978](https://github.com/googleapis/google-cloud-go/commit/11449782c7fb4896bf8b8b9cde8e7441c84fb2fd))

## [0.12.0](https://github.com/googleapis/google-cloud-go/compare/chat/v0.11.0...chat/v0.12.0) (2025-03-12)


### Features

* **chat:** Addition of space notification setting Chat API ([dd0d1d7](https://github.com/googleapis/google-cloud-go/commit/dd0d1d7b41884c9fc9b5fe808139cccd29e1e486))

## [0.11.0](https://github.com/googleapis/google-cloud-go/compare/chat/v0.10.1...chat/v0.11.0) (2025-02-20)


### Features

* **chat:** Add DeletionType.SPACE_MEMBER. This is returned when a message sent by an app is deleted by a human in a space ([#11600](https://github.com/googleapis/google-cloud-go/issues/11600)) ([c08d347](https://github.com/googleapis/google-cloud-go/commit/c08d34776d398a79f6962a26e8e2c75bc4958e2b))

## [0.10.1](https://github.com/googleapis/google-cloud-go/compare/chat/v0.10.0...chat/v0.10.1) (2025-02-12)


### Documentation

* **chat:** Update Google chat app command documentation ([#11581](https://github.com/googleapis/google-cloud-go/issues/11581)) ([93b6495](https://github.com/googleapis/google-cloud-go/commit/93b649580863dc8121c69263749064660a83e095))

## [0.10.0](https://github.com/googleapis/google-cloud-go/compare/chat/v0.9.1...chat/v0.10.0) (2025-01-30)


### Features

* **chat:** A new field `custom_emoji_metadata` is added to message `.google.chat.v1.Annotation` ([de5ca9d](https://github.com/googleapis/google-cloud-go/commit/de5ca9d636e15ca22c6487c690aeaf815630d129))
* **chat:** A new message `CustomEmojiMetadata` is added ([de5ca9d](https://github.com/googleapis/google-cloud-go/commit/de5ca9d636e15ca22c6487c690aeaf815630d129))
* **chat:** A new value `CUSTOM_EMOJI` is added to enum `AnnotationType` ([de5ca9d](https://github.com/googleapis/google-cloud-go/commit/de5ca9d636e15ca22c6487c690aeaf815630d129))


### Documentation

* **chat:** A comment for field `custom_emoji` in message `.google.chat.v1.Emoji` is changed ([de5ca9d](https://github.com/googleapis/google-cloud-go/commit/de5ca9d636e15ca22c6487c690aeaf815630d129))
* **chat:** A comment for method `CreateReaction` in service `ChatService` is changed ([de5ca9d](https://github.com/googleapis/google-cloud-go/commit/de5ca9d636e15ca22c6487c690aeaf815630d129))
* **chat:** A comment for method `DeleteReaction` in service `ChatService` is changed ([de5ca9d](https://github.com/googleapis/google-cloud-go/commit/de5ca9d636e15ca22c6487c690aeaf815630d129))

## [0.9.1](https://github.com/googleapis/google-cloud-go/compare/chat/v0.9.0...chat/v0.9.1) (2025-01-02)


### Bug Fixes

* **chat:** Update golang.org/x/net to v0.33.0 ([e9b0b69](https://github.com/googleapis/google-cloud-go/commit/e9b0b69644ea5b276cacff0a707e8a5e87efafc9))

## [0.9.0](https://github.com/googleapis/google-cloud-go/compare/chat/v0.8.0...chat/v0.9.0) (2024-12-11)


### Features

* **chat:** Add missing field annotations in space.proto, message.proto, reaction.proto, space_event.proto, membership.proto, attachment.proto ([#11246](https://github.com/googleapis/google-cloud-go/issues/11246)) ([ca8d4c3](https://github.com/googleapis/google-cloud-go/commit/ca8d4c36476b2834fca7500368a3f09bea12bd08))

## [0.8.0](https://github.com/googleapis/google-cloud-go/compare/chat/v0.7.1...chat/v0.8.0) (2024-12-04)


### Features

* **chat:** Chat Apps can now retrieve the import mode expire time information to know when to complete the import mode properly ([191a664](https://github.com/googleapis/google-cloud-go/commit/191a6643252221a2d6947d85aea7f31bae17cec6))


### Documentation

* **chat:** Update reference documentation to include import_mode_expire_time field ([191a664](https://github.com/googleapis/google-cloud-go/commit/191a6643252221a2d6947d85aea7f31bae17cec6))

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
