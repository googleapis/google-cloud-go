# Changes

## [0.13.0](https://github.com/googleapis/google-cloud-go/releases/tag/dataform%2Fv0.13.0) (2026-02-19)

### Features

* Update GCP Client Libraries in v1beta1 to support Folders, TeamFolders, and other relevant APIs The v1beta1 API now includes support for Folders and TeamFolders, allowing users to organize repositories and files hierarchically and manage access controls. New Features: - Added TeamFolder resource and methods: CreateTeamFolder, GetTeamFolder, UpdateTeamFolder, DeleteTeamFolder QueryTeamFolderContents to list folder contents. SearchTeamFolders to search for TeamFolders. - Added Folder resource and methods: CreateFolder, GetFolder, UpdateFolder, DeleteFolder QueryFolderContents to list folder contents. Added MoveFolder to move Folders between TeamFolders, other Folders, or the user root folder. - Added MoveRepository to move Repositories between TeamFolders, Folders, or the user root folder. - Added QueryUserRootContents to list contents of a user&#39;s root folder. Repository resource now includes containing_folder and team_folder_name fields to indicate its location within the folder hierarchy. - IAM methods (GetIamPolicy, SetIamPolicy, TestIamPermissions) now support Folder and TeamFolder resources for access control management ([cc0ef5a](https://github.com/googleapis/google-cloud-go/commit/cc0ef5a91ba73f591f63beffb1a9026cf9a3fb8c))

### Documentation

* A comment for field `force` in message `.google.cloud.dataform.v1beta1.DeleteRepositoryRequest` is changed PiperOrigin-RevId: 868182714 ([cc0ef5a](https://github.com/googleapis/google-cloud-go/commit/cc0ef5a91ba73f591f63beffb1a9026cf9a3fb8c))

## [0.12.1](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.12.0...dataform/v0.12.1) (2025-09-16)


### Bug Fixes

* **dataform:** Upgrade gRPC service registration func ([617bb68](https://github.com/googleapis/google-cloud-go/commit/617bb68f41d785126666b9cea1be9fd2d6271515))

## [0.12.0](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.11.2...dataform/v0.12.0) (2025-05-21)


### Features

* **dataform:** Support adding a workflow action to execute a Data Preparation node ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))


### Documentation

* **dataform:** Updated the formatting in some comments in multiple services ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))

## [0.11.2](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.11.1...dataform/v0.11.2) (2025-04-15)


### Bug Fixes

* **dataform:** Update google.golang.org/api to 0.229.0 ([3319672](https://github.com/googleapis/google-cloud-go/commit/3319672f3dba84a7150772ccb5433e02dab7e201))

## [0.11.1](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.11.0...dataform/v0.11.1) (2025-03-13)


### Bug Fixes

* **dataform:** Update golang.org/x/net to 0.37.0 ([1144978](https://github.com/googleapis/google-cloud-go/commit/11449782c7fb4896bf8b8b9cde8e7441c84fb2fd))

## [0.11.0](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.10.3...dataform/v0.11.0) (2025-03-06)


### Features

* **dataform:** Added new field `internal_metadata` to all resources to export all the metadata information that is used internally to serve the resource ([3f23a91](https://github.com/googleapis/google-cloud-go/commit/3f23a9176f29a0a69b9d57b16f44b72eb3096d0c))
* **dataform:** Moving existing field `bigquery_action` to oneof in message `.google.cloud.dataform.v1beta1.WorkflowInvocationAction` to allow adding more actions types such as `notebook_action` ([3f23a91](https://github.com/googleapis/google-cloud-go/commit/3f23a9176f29a0a69b9d57b16f44b72eb3096d0c))
* **dataform:** Returning `commit_sha` in the response of method `CommitRepositoryChanges` ([3f23a91](https://github.com/googleapis/google-cloud-go/commit/3f23a9176f29a0a69b9d57b16f44b72eb3096d0c))


### Bug Fixes

* **dataform:** An existing field `bigquery_action` is moved in to oneof in message `.google.cloud.dataform.v1beta1.WorkflowInvocationAction` ([3f23a91](https://github.com/googleapis/google-cloud-go/commit/3f23a9176f29a0a69b9d57b16f44b72eb3096d0c))
* **dataform:** Remove v1alpha2 client ([#11761](https://github.com/googleapis/google-cloud-go/issues/11761)) ([c85bdd9](https://github.com/googleapis/google-cloud-go/commit/c85bdd9162c75afe426fb8b034179f0e48d00eb3)), refs [#11760](https://github.com/googleapis/google-cloud-go/issues/11760)
* **dataform:** Response type of method `CancelWorkflowInvocation` is changed from `.google.protobuf.Empty` to `.google.cloud.dataform.v1beta1.CancelWorkflowInvocationResponse` in service `Dataform` ([3f23a91](https://github.com/googleapis/google-cloud-go/commit/3f23a9176f29a0a69b9d57b16f44b72eb3096d0c))
* **dataform:** Response type of method `CommitRepositoryChanges` is changed from `.google.protobuf.Empty` to `.google.cloud.dataform.v1beta1.CommitRepositoryChangesResponse` in service `Dataform` ([3f23a91](https://github.com/googleapis/google-cloud-go/commit/3f23a9176f29a0a69b9d57b16f44b72eb3096d0c))
* **dataform:** Response type of method `CommitWorkspaceChanges` is changed from `.google.protobuf.Empty` to `.google.cloud.dataform.v1beta1.CommitWorkspaceChangesResponse` in service `Dataform` ([3f23a91](https://github.com/googleapis/google-cloud-go/commit/3f23a9176f29a0a69b9d57b16f44b72eb3096d0c))
* **dataform:** Response type of method `PullGitCommits` is changed from `.google.protobuf.Empty` to `.google.cloud.dataform.v1beta1.PullGitCommitsResponse` in service `Dataform` ([3f23a91](https://github.com/googleapis/google-cloud-go/commit/3f23a9176f29a0a69b9d57b16f44b72eb3096d0c))
* **dataform:** Response type of method `PushGitCommits` is changed from `.google.protobuf.Empty` to `.google.cloud.dataform.v1beta1.PushGitCommitsResponse` in service `Dataform` ([3f23a91](https://github.com/googleapis/google-cloud-go/commit/3f23a9176f29a0a69b9d57b16f44b72eb3096d0c))
* **dataform:** Response type of method `RemoveDirectory` is changed from `.google.protobuf.Empty` to `.google.cloud.dataform.v1beta1.RemoveDirectoryResponse` in service `Dataform` ([3f23a91](https://github.com/googleapis/google-cloud-go/commit/3f23a9176f29a0a69b9d57b16f44b72eb3096d0c))
* **dataform:** Response type of method `RemoveFileRequest` is changed from `.google.protobuf.Empty` to `.google.cloud.dataform.v1beta1.RemoveFileResponse` in service `Dataform` ([3f23a91](https://github.com/googleapis/google-cloud-go/commit/3f23a9176f29a0a69b9d57b16f44b72eb3096d0c))
* **dataform:** Response type of method `ResetWorkspaceChanges` is changed from `.google.protobuf.Empty` to `.google.cloud.dataform.v1beta1.ResetWorkspaceChangesResponse` in service `Dataform` ([3f23a91](https://github.com/googleapis/google-cloud-go/commit/3f23a9176f29a0a69b9d57b16f44b72eb3096d0c))


### Documentation

* **dataform:** Adds known limitations on several methods such as `UpdateRepository`, `UpdateReleaseConfig` and `UpdateWorkflowConfig` ([3f23a91](https://github.com/googleapis/google-cloud-go/commit/3f23a9176f29a0a69b9d57b16f44b72eb3096d0c))
* **dataform:** Explained the effect of field `page_token` on the pagination in several messages ([3f23a91](https://github.com/googleapis/google-cloud-go/commit/3f23a9176f29a0a69b9d57b16f44b72eb3096d0c))
* **dataform:** Several comments reformatted ([3f23a91](https://github.com/googleapis/google-cloud-go/commit/3f23a9176f29a0a69b9d57b16f44b72eb3096d0c))

## [0.10.3](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.10.2...dataform/v0.10.3) (2025-01-02)


### Bug Fixes

* **dataform:** Update golang.org/x/net to v0.33.0 ([e9b0b69](https://github.com/googleapis/google-cloud-go/commit/e9b0b69644ea5b276cacff0a707e8a5e87efafc9))

## [0.10.2](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.10.1...dataform/v0.10.2) (2024-10-23)


### Bug Fixes

* **dataform:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **dataform:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [0.10.1](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.10.0...dataform/v0.10.1) (2024-09-12)


### Bug Fixes

* **dataform:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [0.10.0](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.9.9...dataform/v0.10.0) (2024-08-20)


### Features

* **dataform:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [0.9.9](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.9.8...dataform/v0.9.9) (2024-08-08)


### Bug Fixes

* **dataform:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [0.9.8](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.9.7...dataform/v0.9.8) (2024-07-24)


### Bug Fixes

* **dataform:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [0.9.7](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.9.6...dataform/v0.9.7) (2024-07-10)


### Bug Fixes

* **dataform:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [0.9.6](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.9.5...dataform/v0.9.6) (2024-07-01)


### Bug Fixes

* **dataform:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [0.9.5](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.9.4...dataform/v0.9.5) (2024-06-26)


### Bug Fixes

* **dataform:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [0.9.4](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.9.3...dataform/v0.9.4) (2024-05-01)


### Bug Fixes

* **dataform:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [0.9.3](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.9.2...dataform/v0.9.3) (2024-03-14)


### Bug Fixes

* **dataform:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [0.9.2](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.9.1...dataform/v0.9.2) (2024-01-30)


### Bug Fixes

* **dataform:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [0.9.1](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.9.0...dataform/v0.9.1) (2023-11-01)


### Bug Fixes

* **dataform:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [0.9.0](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.8.3...dataform/v0.9.0) (2023-10-31)


### Features

* **dataform:** Support for ReleaseConfigs ([ffb0dda](https://github.com/googleapis/google-cloud-go/commit/ffb0ddabf3d9822ba8120cabaf25515fd32e9615))

## [0.8.3](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.8.2...dataform/v0.8.3) (2023-10-26)


### Bug Fixes

* **dataform:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [0.8.2](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.8.1...dataform/v0.8.2) (2023-10-12)


### Bug Fixes

* **dataform:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [0.8.1](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.8.0...dataform/v0.8.1) (2023-06-20)


### Bug Fixes

* **dataform:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [0.8.0](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.7.1...dataform/v0.8.0) (2023-05-30)


### Features

* **dataform:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [0.7.1](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.7.0...dataform/v0.7.1) (2023-05-08)


### Bug Fixes

* **dataform:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [0.7.0](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.6.0...dataform/v0.7.0) (2023-03-15)


### Features

* **dataform:** Update iam and longrunning deps ([91a1f78](https://github.com/googleapis/google-cloud-go/commit/91a1f784a109da70f63b96414bba8a9b4254cddd))

## [0.6.0](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.5.0...dataform/v0.6.0) (2023-01-04)


### Features

* **dataform:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [0.5.0](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.4.0...dataform/v0.5.0) (2022-10-25)


### Features

* **dataform:** Start generating apiv1beta1 ([#6893](https://github.com/googleapis/google-cloud-go/issues/6893)) ([fedaf1e](https://github.com/googleapis/google-cloud-go/commit/fedaf1e355e4014501d5bb7ae61cf84b72d30581))

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.3.0...dataform/v0.4.0) (2022-09-21)


### Features

* **dataform:** rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.2.0...dataform/v0.3.0) (2022-09-19)


### Features

* **dataform:** start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/dataform/v0.1.0...dataform/v0.2.0) (2022-08-02)


### Features

* **dataform:** Release API version v1beta1 (no changes to v1alpha2) ([1d6fbcc](https://github.com/googleapis/google-cloud-go/commit/1d6fbcc6406e2063201ef5a98de560bf32f7fb73))

## 0.1.0 (2022-07-12)


### Features

* **dataform:** remove unused filter field from alpha2 version of API before release ([8a1ad06](https://github.com/googleapis/google-cloud-go/commit/8a1ad06572a65afa91a0a77a85b849e766876671))
* **dataform:** start generating apiv1alpha2 ([#6299](https://github.com/googleapis/google-cloud-go/issues/6299)) ([1c434c6](https://github.com/googleapis/google-cloud-go/commit/1c434c6657b9bd8529760681c95aef9373c66120))
