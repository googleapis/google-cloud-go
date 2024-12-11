# Changes

## [1.35.2](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.35.1...securitycenter/v1.35.2) (2024-10-23)


### Bug Fixes

* **securitycenter:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **securitycenter:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([2b8ca4b](https://github.com/googleapis/google-cloud-go/commit/2b8ca4b4127ce3025c7a21cc7247510e07cc5625))

## [1.35.1](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.35.0...securitycenter/v1.35.1) (2024-09-12)


### Bug Fixes

* **securitycenter:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.35.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.34.0...securitycenter/v1.35.0) (2024-08-20)


### Features

* **securitycenter:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.34.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.33.1...securitycenter/v1.34.0) (2024-08-08)


### Features

* **securitycenter:** Enable Dynamic Mute ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **securitycenter:** Enable Dynamic Mute ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **securitycenter:** New values `EXPLOITATION_FOR_PRIVILEGE_ESCALATION` corresponding to T1068 and `INDICATOR_REMOVAL_FILE_DELETION` corresponding to T1070.004 are added to enum `Technique` ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **securitycenter:** New values `EXPLOITATION_FOR_PRIVILEGE_ESCALATION` corresponding to T1068 and `INDICATOR_REMOVAL_FILE_DELETION` corresponding to T1070.004 are added to enum `Technique` ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))


### Bug Fixes

* **securitycenter:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))


### Documentation

* **securitycenter:** T1068 is added for value `EXPLOITATION_FOR_PRIVILEGE_ESCALATION` and T1070.004 is added for value `INDICATOR_REMOVAL_FILE_DELETION` for enum `Technique ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **securitycenter:** T1068 is added for value `EXPLOITATION_FOR_PRIVILEGE_ESCALATION` and T1070.004 is added for value `INDICATOR_REMOVAL_FILE_DELETION` for enum `Technique ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))

## [1.33.1](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.33.0...securitycenter/v1.33.1) (2024-07-24)


### Bug Fixes

* **securitycenter:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [1.33.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.32.0...securitycenter/v1.33.0) (2024-07-10)


### Features

* **securitycenter:** Added attack path API methods ([3b15f9d](https://github.com/googleapis/google-cloud-go/commit/3b15f9db9e0ee3bff3d8d5aafc82cdc2a31d60fc))
* **securitycenter:** Added cloud provider field to list findings response ([#10506](https://github.com/googleapis/google-cloud-go/issues/10506)) ([3b15f9d](https://github.com/googleapis/google-cloud-go/commit/3b15f9db9e0ee3bff3d8d5aafc82cdc2a31d60fc))
* **securitycenter:** Added etd custom module protos and API methods ([3b15f9d](https://github.com/googleapis/google-cloud-go/commit/3b15f9db9e0ee3bff3d8d5aafc82cdc2a31d60fc))
* **securitycenter:** Added ResourceValueConfig protos and API methods ([3b15f9d](https://github.com/googleapis/google-cloud-go/commit/3b15f9db9e0ee3bff3d8d5aafc82cdc2a31d60fc))
* **securitycenter:** Added toxic combination field to finding ([3b15f9d](https://github.com/googleapis/google-cloud-go/commit/3b15f9db9e0ee3bff3d8d5aafc82cdc2a31d60fc))


### Bug Fixes

* **securitycenter:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))


### Documentation

* **securitycenter:** Update examples in comments to use backticks ([3b15f9d](https://github.com/googleapis/google-cloud-go/commit/3b15f9db9e0ee3bff3d8d5aafc82cdc2a31d60fc))
* **securitycenter:** Update toxic combinations comments ([3b15f9d](https://github.com/googleapis/google-cloud-go/commit/3b15f9db9e0ee3bff3d8d5aafc82cdc2a31d60fc))

## [1.32.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.31.0...securitycenter/v1.32.0) (2024-07-01)


### Features

* **securitycenter:** Added cloud provider field to list findings response ([eec7a3b](https://github.com/googleapis/google-cloud-go/commit/eec7a3b5c00fc18076f410ddc4910cdcc61c702c))
* **securitycenter:** Added http configuration rule to ResourceValueConfig and ValuedResource API methods ([eec7a3b](https://github.com/googleapis/google-cloud-go/commit/eec7a3b5c00fc18076f410ddc4910cdcc61c702c))
* **securitycenter:** Added toxic combination field to finding ([eec7a3b](https://github.com/googleapis/google-cloud-go/commit/eec7a3b5c00fc18076f410ddc4910cdcc61c702c))


### Bug Fixes

* **securitycenter:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))


### Documentation

* **securitycenter:** Updated comments for ResourceValueConfig ([eec7a3b](https://github.com/googleapis/google-cloud-go/commit/eec7a3b5c00fc18076f410ddc4910cdcc61c702c))

## [1.31.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.30.0...securitycenter/v1.31.0) (2024-06-26)


### Features

* **securitycenter:** Add toxic_combination and group_memberships fields to finding ([7ca4fa3](https://github.com/googleapis/google-cloud-go/commit/7ca4fa38519b24acde1675724edcde7b99fb32ee))
* **securitycenter:** Add toxic_combination and group_memberships fields to finding ([7ca4fa3](https://github.com/googleapis/google-cloud-go/commit/7ca4fa38519b24acde1675724edcde7b99fb32ee))


### Bug Fixes

* **securitycenter:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.30.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.29.0...securitycenter/v1.30.0) (2024-05-01)


### Features

* **securitycenter:** Add cloud_armor field to finding's list of attributes ([1d757c6](https://github.com/googleapis/google-cloud-go/commit/1d757c66478963d6cbbef13fee939632c742759c))


### Bug Fixes

* **securitycenter:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [1.29.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.28.0...securitycenter/v1.29.0) (2024-04-15)


### Features

* **securitycenter:** Add Notebook field to finding's list of attributes ([fe85be0](https://github.com/googleapis/google-cloud-go/commit/fe85be03d1e6ba69182ff1045a3faed15aa00128))


### Documentation

* **securitycenter:** Fixed backtick and double quotes mismatch in security_marks.proto ([dbcdfd7](https://github.com/googleapis/google-cloud-go/commit/dbcdfd7843be16573b1683466852242a84891456))

## [1.28.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.27.0...securitycenter/v1.28.0) (2024-03-12)


### Features

* **securitycenter:** Add security_posture, external_system.case_uri, external_system.case_priority, external_system.case_sla, external_system.case_create_time, external_system.case_close_time, and external_system.ticket_info to finding's list of attributes ([ccfe599](https://github.com/googleapis/google-cloud-go/commit/ccfe59970fac372e07202d26c520e36e0b3b9598))
* **securitycenter:** New client(s) ([#9562](https://github.com/googleapis/google-cloud-go/issues/9562)) ([9d6b29d](https://github.com/googleapis/google-cloud-go/commit/9d6b29d136cad2dde290b4ca1383c9382eb83b34))

## [1.27.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.26.0...securitycenter/v1.27.0) (2024-03-04)


### Features

* **securitycenter:** Add container.create_time, vulnerability.offending_package, vulnerability.fixed_package, vulnerability.security_bulletin, vulnerability.cve.impact, vulnerability.cve.exploitation_activity, vulnerability.cve.observed_in_the_wild, v... ([#9473](https://github.com/googleapis/google-cloud-go/issues/9473)) ([d130d86](https://github.com/googleapis/google-cloud-go/commit/d130d861f55d137a2803340c2e11da3589669cb8))

## [1.26.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.25.0...securitycenter/v1.26.0) (2024-02-26)


### Features

* **securitycenter:** Add Backup DR field to finding's list of attributes ([3814ee3](https://github.com/googleapis/google-cloud-go/commit/3814ee3f27724ad0d02688ad86030b83e0a72fd4))

## [1.25.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.24.4...securitycenter/v1.25.0) (2024-02-21)


### Features

* **securitycenter:** Add application field to finding's list of attributes ([a86aa8e](https://github.com/googleapis/google-cloud-go/commit/a86aa8e962b77d152ee6cdd433ad94967150ef21))

## [1.24.4](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.24.3...securitycenter/v1.24.4) (2024-01-30)


### Bug Fixes

* **securitycenter:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.24.3](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.24.2...securitycenter/v1.24.3) (2023-11-27)


### Documentation

* **securitycenter:** Modify documentation of SimulateSecurityHealthAnalyticsCustomModuleRequest ([63ffff2](https://github.com/googleapis/google-cloud-go/commit/63ffff2a994d991304ba1ef93cab847fa7cd39e4))

## [1.24.2](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.24.1...securitycenter/v1.24.2) (2023-11-01)


### Bug Fixes

* **securitycenter:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.24.1](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.24.0...securitycenter/v1.24.1) (2023-10-26)


### Bug Fixes

* **securitycenter:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.24.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.23.1...securitycenter/v1.24.0) (2023-10-19)


### Features

* **securitycenter:** Add SimulateSecurityHealthAnalyticsCustomModule API for testing SHA custom module ([#8743](https://github.com/googleapis/google-cloud-go/issues/8743)) ([f3e2b05](https://github.com/googleapis/google-cloud-go/commit/f3e2b05129582f599fa9f53598f0cd7abe177493))

## [1.23.1](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.23.0...securitycenter/v1.23.1) (2023-10-12)


### Bug Fixes

* **securitycenter:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.23.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.22.1...securitycenter/v1.23.0) (2023-06-27)


### Features

* **securitycenter:** Mark the Asset APIs as deprecated in client libraries ([94ea341](https://github.com/googleapis/google-cloud-go/commit/94ea3410e233db6040a7cb0a931948f1e3bb4c9a))

## [1.22.1](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.22.0...securitycenter/v1.22.1) (2023-06-20)


### Bug Fixes

* **securitycenter:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.22.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.21.0...securitycenter/v1.22.0) (2023-06-13)


### Features

* **securitycenter:** Add user agent and DLP parent type fields to finding's list of attributes ([3abdfa1](https://github.com/googleapis/google-cloud-go/commit/3abdfa14dd56cf773c477f289a7f888e20bbbd9a))

## [1.21.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.20.1...securitycenter/v1.21.0) (2023-05-30)


### Features

* **securitycenter:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.20.1](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.20.0...securitycenter/v1.20.1) (2023-05-08)


### Bug Fixes

* **securitycenter:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.20.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.19.0...securitycenter/v1.20.0) (2023-04-25)


### Features

* **securitycenter:** Add cloud_dlp_inspection and cloud_dlp_data_profile fields to finding's list of attributes ([#7808](https://github.com/googleapis/google-cloud-go/issues/7808)) ([2c9b4cf](https://github.com/googleapis/google-cloud-go/commit/2c9b4cf95c5af845537e204cbcf3034f423ea10c))

## [1.19.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.18.1...securitycenter/v1.19.0) (2023-03-15)


### Features

* **securitycenter:** Update iam and longrunning deps ([91a1f78](https://github.com/googleapis/google-cloud-go/commit/91a1f784a109da70f63b96414bba8a9b4254cddd))

## [1.18.1](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.18.0...securitycenter/v1.18.1) (2023-01-18)


### Documentation

* **securitycenter:** Update documentation for Security Command Center *.assets.list "parent" parameter ([8b3b76d](https://github.com/googleapis/google-cloud-go/commit/8b3b76d4c896e3f3338ccd357a5b2b7a6155c773))

## [1.18.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.17.0...securitycenter/v1.18.0) (2023-01-04)


### Features

* **securitycenter:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [1.17.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.16.0...securitycenter/v1.17.0) (2022-11-16)


### Features

* **securitycenter:** Add files field to finding's list of attributes ([ac0c5c2](https://github.com/googleapis/google-cloud-go/commit/ac0c5c21221e8d055e6b8b1c473600c58e306b00))

## [1.16.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.15.0...securitycenter/v1.16.0) (2022-11-03)


### Features

* **securitycenter:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.15.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.14.0...securitycenter/v1.15.0) (2022-10-25)


### Features

* **securitycenter:** Adding project/folder level parents to notification configs in SCC ([caf4afa](https://github.com/googleapis/google-cloud-go/commit/caf4afa139ad7b38b6df3e3b17b8357c81e1fd6c))
* **securitycenter:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.13.0...securitycenter/v1.14.0) (2022-09-21)


### Features

* **securitycenter:** rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.12.0...securitycenter/v1.13.0) (2022-09-19)


### Features

* **securitycenter:** start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.11.0...securitycenter/v1.12.0) (2022-09-15)


### Features

* **securitycenter/apiv1beta1:** add REST transport ([f7b0822](https://github.com/googleapis/google-cloud-go/commit/f7b082212b1e46ff2f4126b52d49618785c2e8ca))
* **securitycenter/apiv1p1beta1:** add REST transport ([f7b0822](https://github.com/googleapis/google-cloud-go/commit/f7b082212b1e46ff2f4126b52d49618785c2e8ca))
* **securitycenter/settings/apiv1beta1:** add REST transport ([f7b0822](https://github.com/googleapis/google-cloud-go/commit/f7b082212b1e46ff2f4126b52d49618785c2e8ca))
* **securitycenter:** Added parent display name i.e. source display name for a finding as one of the finding attributes ([a679a5a](https://github.com/googleapis/google-cloud-go/commit/a679a5a9b1ea60cb155eb6c8be4afcc43d3b121f))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.10.0...securitycenter/v1.11.0) (2022-09-06)


### Features

* **securitycenter:** Adding database access information, such as queries field to a finding. A database may be a sub-resource of an instance (as in the case of CloudSQL instances or Cloud Spanner instances), or the database instance itself ([3bc37e2](https://github.com/googleapis/google-cloud-go/commit/3bc37e28626df5f7ec37b00c0c2f0bfb91c30495))
* **securitycenter:** serviceAccountKeyName, serviceAccountDelegationInfo, and principalSubject attributes added to the existing access attribute. These new attributes provide additional context about the principals that are associated with the finding ([3bc37e2](https://github.com/googleapis/google-cloud-go/commit/3bc37e28626df5f7ec37b00c0c2f0bfb91c30495))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.9.0...securitycenter/v1.10.0) (2022-07-26)


### Features

* **securitycenter:** Added container field to findings attributes feat: Added kubernetes field to findings attribute. This field is populated only when the container is a kubernetes cluster explicitly ([1ffeb95](https://github.com/googleapis/google-cloud-go/commit/1ffeb9557bf1f18cc131aff40ec7e0e15a9f4ead))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.8.0...securitycenter/v1.9.0) (2022-07-12)


### Features

* **securitycenter:** Added contacts field to findings attributes, specifying Essential Contacts defined at org, folder or project level within a GCP org feat: Added process signature fields to the indicator attribute that helps surface multiple types of signature defined IOCs ([8a1ad06](https://github.com/googleapis/google-cloud-go/commit/8a1ad06572a65afa91a0a77a85b849e766876671))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.7.0...securitycenter/v1.8.0) (2022-06-01)


### Features

* **securitycenter:** Add compliances, processes and exfiltration fields to findings attributes. They contain compliance information about a security standard indicating unmet recommendations, represents operating system processes, and data exfiltration attempt of one or more source(s) to one or more target(s).  Source(s) represent the source of data that is exfiltrated, and Target(s) represents the destination the data was copied to ([9266276](https://github.com/googleapis/google-cloud-go/commit/92662768493738a4492eae3ea4ac6db250056bf1))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.6.0...securitycenter/v1.7.0) (2022-04-20)


### Features

* **securitycenter:** Add connection and description field to finding's list of attributes ([689cad9](https://github.com/googleapis/google-cloud-go/commit/689cad94fdcf54cebd22aecfcdad4d8b44f58df9))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.5.0...securitycenter/v1.6.0) (2022-04-14)


### Features

* **securitycenter:** Add iam_binding field to findings attributes. It represents particular IAM bindings, which captures a member's role addition, removal, or state ([bb5da6b](https://github.com/googleapis/google-cloud-go/commit/bb5da6b3c34079a01d18b766b67f626cff18d849))
* **securitycenter:** Add next_steps field to finding's list of attributes ([19a9ef2](https://github.com/googleapis/google-cloud-go/commit/19a9ef2d9b8d77d3bc3e4c11c7f1f3e47700edd4))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.4.0...securitycenter/v1.5.0) (2022-03-14)


### Features

* **securitycenter:** Add BigQuery export APIs that help you enable writing new/updated findings from  Security Command Center to a BigQuery table in near-real time. You can then integrate the data into existing workflows and create custom analyses. You can enable this feature at the organization, folder, and project levels to export findings based on your requirements ([35d591a](https://github.com/googleapis/google-cloud-go/commit/35d591adf1f98e5707ffe7a7bf5c48a5cc4ae8d4))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.3.0...securitycenter/v1.4.0) (2022-02-23)


### Features

* **securitycenter:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.2.0...securitycenter/v1.3.0) (2022-02-14)


### Features

* **securitycenter:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [1.2.0](https://www.github.com/googleapis/google-cloud-go/compare/securitycenter/v1.1.0...securitycenter/v1.2.0) (2022-01-04)


### Features

* **securitycenter:** Added a new API method UpdateExternalSystem, which enables updating a finding w/ external system metadata. External systems are a child resource under finding, and are housed on the finding itself, and can also be filtered on in Notifications, the ListFindings and GroupFindings API ([c8271d4](https://www.github.com/googleapis/google-cloud-go/commit/c8271d4b217a6e6924d9f87eac9468c4b5767ba7))
* **securitycenter:** Added mute related APIs, proto messages and fields ([3e7185c](https://www.github.com/googleapis/google-cloud-go/commit/3e7185c241d97ee342f132ae04bc93bb79a8e897))
* **securitycenter:** Added resource type and display_name field to the FindingResult, and supported them in the filter for ListFindings and GroupFindings. Also added display_name to the resource which is surfaced in NotificationMessage ([1f5aa78](https://www.github.com/googleapis/google-cloud-go/commit/1f5aa78a4d6633871651c89a6d9c48e3409fecc5))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/securitycenter/v1.0.0...securitycenter/v1.1.0) (2021-10-11)


### Features

* **securitycenter:** Added vulnerability field to the finding feat: Added type field to the resource which is surfaced in NotificationMessage ([090cc3a](https://www.github.com/googleapis/google-cloud-go/commit/090cc3ae0f8747a14cc904fc6d429e2f5379bb03))

## 1.0.0

Stabilize GA surface.

## v0.1.0

This is the first tag to carve out securitycenter as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
