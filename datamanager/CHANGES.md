# Changes

## [0.3.0](https://github.com/googleapis/google-cloud-go/releases/tag/datamanager%2Fv0.3.0) (2026-03-12)

### Features

* deprecate INVALID_COUNTRY_CODE and add MEMBERSHIP_DURATION_TOO_LONG to the ErrorReason enum ([177550d](https://github.com/googleapis/google-cloud-go/commit/177550d454fe98dcd1cd6645bf9b4c51eef7a419))

### Bug Fixes

* feat: update advertiser_identifier_count in PairIdInfo to be optional ([177550d](https://github.com/googleapis/google-cloud-go/commit/177550d454fe98dcd1cd6645bf9b4c51eef7a419))
* update match_rate_percentage in PairIdInfo to be required ([177550d](https://github.com/googleapis/google-cloud-go/commit/177550d454fe98dcd1cd6645bf9b4c51eef7a419))
* update publisher_name in PairIdInfo to be required ([177550d](https://github.com/googleapis/google-cloud-go/commit/177550d454fe98dcd1cd6645bf9b4c51eef7a419))

### Documentation

* update filter field documentation to clarify case requirements and improve examples ([177550d](https://github.com/googleapis/google-cloud-go/commit/177550d454fe98dcd1cd6645bf9b4c51eef7a419))

## [0.2.0](https://github.com/googleapis/google-cloud-go/releases/tag/datamanager%2Fv0.2.0) (2026-02-26)

### Features

* add `AgeRange` and `Gender` enums to support demographic breakdown in marketing insights ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* add `GOOGLE_AD_MANAGER_AUDIENCE_LINK` to the `AccountType` enum ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* add `IngestPpidDataStatus` to `IngestAudienceMembersStatus` to report the status of PPID data ingestion ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* add `IngestUserIdDataStatus` to `IngestAudienceMembersStatus` to report the status of user ID data ingestion ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* add `MarketingDataInsightsService` for retrieving marketing data insights for a given user list ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* add `PartnerLinkService` for creating and managing links between advertiser and data partner accounts ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* add `PartnerLink` resource ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* add `PpidData` to `AudienceMember` to support Publisher Provided ID (PPID) in audience member ingestion ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* add `RemovePpidDataStatus` to `RemoveAudienceMembersStatus` to report the status of PPID data removal ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* add `RemoveUserIdDataStatus` to `RemoveAudienceMembersStatus` to report the status of user ID data removal ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* add `UserIdData` to `AudienceMember` to support User ID in audience member ingestion ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* add `UserListDirectLicenseService` for creating and managing direct user list licenses ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* add `UserListDirectLicense` resource ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* add `UserListGlobalLicenseCustomerInfo` resource ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* add `UserListGlobalLicenseService` for creating and managing global user list licenses ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* add `UserListGlobalLicense` resource ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* add `UserListService` for creating and managing user lists ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* add `UserList` resource ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* add new `ErrorReason` values for licensing, user list operations, and permission checks ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))

### Bug Fixes

* changed `conversion_value` field to be optional in message `Event` ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))

### Documentation

* a comment for enum `ErrorReason` is changed to clarify that it is subject to future additions ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* a comment for field `pair_data` in message `AudienceMember` is changed to clarify it is only available to data partners ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* a comment for message `PairData` is changed to clarify it is only available to data partners ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))
* add comments to resources and methods to clarify which are available only to data partners ([b21a3b8](https://github.com/googleapis/google-cloud-go/commit/b21a3b8409f1af4f077be833949c1b6cc3e4c319))

## [0.1.0](https://github.com/googleapis/google-cloud-go/releases/tag/datamanager%2Fv0.1.0) (2026-02-12)

### Features

* add new clients (#13817) ([edc2b93](https://github.com/googleapis/google-cloud-go/commit/edc2b93546e4814cb6587f4d86bfb21b156be5e2))

