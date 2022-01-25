# Changes

## [1.19.0](https://www.github.com/googleapis/google-cloud-go/compare/storage/v1.18.2...storage/v1.19.0) (2022-01-25)


### Features

* **storage:** add net.ErrClosed to default retry ([#5384](https://www.github.com/googleapis/google-cloud-go/issues/5384)) ([a4801c7](https://www.github.com/googleapis/google-cloud-go/commit/a4801c70db3f8c920110881aa9a4d617aca39d1b))
* **storage:** add retry config to ACL handle methods ([#5185](https://www.github.com/googleapis/google-cloud-go/issues/5185)) ([be07d8d](https://www.github.com/googleapis/google-cloud-go/commit/be07d8dbd63857d3fb7f8e1cd0a0048c89a02ebd))
* **storage:** add retry config to BucketHandle ([#5170](https://www.github.com/googleapis/google-cloud-go/issues/5170)) ([b2b5476](https://www.github.com/googleapis/google-cloud-go/commit/b2b54767d45a2b00e005a2bce0d7c8b15ad3605e))
* **storage:** add retry config to IAM handle and reader ([#5223](https://www.github.com/googleapis/google-cloud-go/issues/5223)) ([8365456](https://www.github.com/googleapis/google-cloud-go/commit/8365456ee14d7af0328ce75760df489d59bc6b86))
* **storage:** add retry config to iterators ([4910cd4](https://www.github.com/googleapis/google-cloud-go/commit/4910cd4d8a1998d16a78b14393de78c634dba0c3))
* **storage:** add retry config to notifications ([#5221](https://www.github.com/googleapis/google-cloud-go/issues/5221)) ([ba5d6cf](https://www.github.com/googleapis/google-cloud-go/commit/ba5d6cf3a946520749447f1942b20cbc76944c9d))
* **storage:** add retry config to the Client and HmacKey operations ([#5193](https://www.github.com/googleapis/google-cloud-go/issues/5193)) ([ce6e7a0](https://www.github.com/googleapis/google-cloud-go/commit/ce6e7a022252362ecd04547d3cd6ec50eaf3eec3))
* **storage:** add retry configurability ([#5159](https://www.github.com/googleapis/google-cloud-go/issues/5159)) ([a12641a](https://www.github.com/googleapis/google-cloud-go/commit/a12641a43de60ed123f9ac604e9017c83a9654df))
* **storage:** add retry idempotency configs ([#5165](https://www.github.com/googleapis/google-cloud-go/issues/5165)) ([fa5e458](https://www.github.com/googleapis/google-cloud-go/commit/fa5e4587cd6677edaa7f501b1521b27fe6bef0bd))
* **storage:** add rpo (turbo replication) support ([#5003](https://www.github.com/googleapis/google-cloud-go/issues/5003)) ([3bd5995](https://www.github.com/googleapis/google-cloud-go/commit/3bd59958e0c06d2655b67fcb5410668db3c52af0))
* **storage:** allow retry ErrorFunc configs ([#5166](https://www.github.com/googleapis/google-cloud-go/issues/5166)) ([c103ff6](https://www.github.com/googleapis/google-cloud-go/commit/c103ff6cb37aba3f326aff7e04c84dc2050d53f5))
* **storage:** configurable retries for uploads ([#5210](https://www.github.com/googleapis/google-cloud-go/issues/5210)) ([ee4f600](https://www.github.com/googleapis/google-cloud-go/commit/ee4f600929df86d4c65db1abcc4bef2f1d6d23a5))
* **storage:** GenerateSignedPostPolicyV4 can use existing creds to authenticate ([#5105](https://www.github.com/googleapis/google-cloud-go/issues/5105)) ([46489f4](https://www.github.com/googleapis/google-cloud-go/commit/46489f4c8a634068a3e7cf2fd5e5ca11b555c0a8))
* **storage:** post policy can be signed with a fn that takes raw bytes ([#5079](https://www.github.com/googleapis/google-cloud-go/issues/5079)) ([25d1278](https://www.github.com/googleapis/google-cloud-go/commit/25d1278cab539fbfdd8563ed6b297e30d3fe555c))
* **storage:** retry copy funcs on idempotent conds ([#5172](https://www.github.com/googleapis/google-cloud-go/issues/5172)) ([6d09687](https://www.github.com/googleapis/google-cloud-go/commit/6d0968722352895f2d8034e5c079bd406e3ca4b2))
* **storage:** retry on a HTTP 408 response code ([#5314](https://www.github.com/googleapis/google-cloud-go/issues/5314)) ([b5fe903](https://www.github.com/googleapis/google-cloud-go/commit/b5fe903c4390aa8fb62b632d7f87ecf5665b821c))


### Bug Fixes

* **storage:** fix nil check in gRPC Reader ([#5376](https://www.github.com/googleapis/google-cloud-go/issues/5376)) ([5e7d722](https://www.github.com/googleapis/google-cloud-go/commit/5e7d722d18a62b28ba98169b3bdbb49401377264))

### [1.18.2](https://www.github.com/googleapis/google-cloud-go/compare/storage/v1.18.1...storage/v1.18.2) (2021-10-18)


### Bug Fixes

* **storage:** upgrade genproto ([#4993](https://www.github.com/googleapis/google-cloud-go/issues/4993)) ([5ca462d](https://www.github.com/googleapis/google-cloud-go/commit/5ca462d99fe851b7cddfd70108798e2fa959bdfd)), refs [#4991](https://www.github.com/googleapis/google-cloud-go/issues/4991)

### [1.18.1](https://www.github.com/googleapis/google-cloud-go/compare/storage/v1.18.0...storage/v1.18.1) (2021-10-14)


### Bug Fixes

* **storage:** don't assume auth from a client option ([#4982](https://www.github.com/googleapis/google-cloud-go/issues/4982)) ([e17334d](https://www.github.com/googleapis/google-cloud-go/commit/e17334d1fe7645d89d14ae7148313498b984dfbb))

## [1.18.0](https://www.github.com/googleapis/google-cloud-go/compare/storage/v1.17.0...storage/v1.18.0) (2021-10-11)


### Features

* **storage:** returned wrapped error for timeouts ([#4802](https://www.github.com/googleapis/google-cloud-go/issues/4802)) ([0e102a3](https://www.github.com/googleapis/google-cloud-go/commit/0e102a385dc67a06f6b444b3a93e6998428529be)), refs [#4197](https://www.github.com/googleapis/google-cloud-go/issues/4197)
* **storage:** SignedUrl can use existing creds to authenticate ([#4604](https://www.github.com/googleapis/google-cloud-go/issues/4604)) ([b824c89](https://www.github.com/googleapis/google-cloud-go/commit/b824c897e6941270747b612f6d36a8d6ae081315))


### Bug Fixes

* **storage:** update PAP to use inherited instead of unspecified ([#4909](https://www.github.com/googleapis/google-cloud-go/issues/4909)) ([dac26b1](https://www.github.com/googleapis/google-cloud-go/commit/dac26b1af2f2972f12775341173bcc5f982438b8))

## [1.17.0](https://www.github.com/googleapis/google-cloud-go/compare/storage/v1.16.1...storage/v1.17.0) (2021-09-28)


### Features

* **storage:** add projectNumber field to bucketAttrs. ([#4805](https://www.github.com/googleapis/google-cloud-go/issues/4805)) ([07343af](https://www.github.com/googleapis/google-cloud-go/commit/07343afc15085b164cc41d202d13f9d46f5c0d02))


### Bug Fixes

* **storage:** align retry idempotency (part 1) ([#4715](https://www.github.com/googleapis/google-cloud-go/issues/4715)) ([ffa903e](https://www.github.com/googleapis/google-cloud-go/commit/ffa903eeec61aa3869e5220e2f09371127b5c393))

### [1.16.1](https://www.github.com/googleapis/google-cloud-go/compare/storage/v1.16.0...storage/v1.16.1) (2021-08-30)


### Bug Fixes

* **storage/internal:** Update encryption_key fields to "bytes" type. fix: Improve date/times and field name clarity in lifecycle conditions. ([a52baa4](https://www.github.com/googleapis/google-cloud-go/commit/a52baa456ed8513ec492c4b573c191eb61468758))
* **storage:** accept emulator env var without scheme ([#4616](https://www.github.com/googleapis/google-cloud-go/issues/4616)) ([5f8cbb9](https://www.github.com/googleapis/google-cloud-go/commit/5f8cbb98070109e2a34409ac775ed63b94d37efd))
* **storage:** preserve supplied endpoint's scheme ([#4609](https://www.github.com/googleapis/google-cloud-go/issues/4609)) ([ee2756f](https://www.github.com/googleapis/google-cloud-go/commit/ee2756fb0a335d591464a770c9fa4f8fe0ba2e01))
* **storage:** remove unnecessary variable ([#4608](https://www.github.com/googleapis/google-cloud-go/issues/4608)) ([27fc784](https://www.github.com/googleapis/google-cloud-go/commit/27fc78456fb251652bdf5cdb493734a7e1e643e1))
* **storage:** retry LockRetentionPolicy ([#4439](https://www.github.com/googleapis/google-cloud-go/issues/4439)) ([09879ea](https://www.github.com/googleapis/google-cloud-go/commit/09879ea80cb67f9bfd8fc9384b0fda335567cba9)), refs [#4437](https://www.github.com/googleapis/google-cloud-go/issues/4437)
* **storage:** revise Reader to send XML preconditions ([#4479](https://www.github.com/googleapis/google-cloud-go/issues/4479)) ([e36b29a](https://www.github.com/googleapis/google-cloud-go/commit/e36b29a3d43bce5c1c044f7daf6e1db00b0a49e0)), refs [#4470](https://www.github.com/googleapis/google-cloud-go/issues/4470)

## [1.16.0](https://www.github.com/googleapis/google-cloud-go/compare/storage/v1.15.0...storage/v1.16.0) (2021-06-28)


### Features

* **storage:** support PublicAccessPrevention ([#3608](https://www.github.com/googleapis/google-cloud-go/issues/3608)) ([99bc782](https://www.github.com/googleapis/google-cloud-go/commit/99bc782fb50a47602b45278384ef5d5b5da9263b)), refs [#3203](https://www.github.com/googleapis/google-cloud-go/issues/3203)


### Bug Fixes

* **storage:** fix Writer.ChunkSize validation ([#4255](https://www.github.com/googleapis/google-cloud-go/issues/4255)) ([69c2e9d](https://www.github.com/googleapis/google-cloud-go/commit/69c2e9dc6303e1a004d3104a8178532fa738e742)), refs [#4167](https://www.github.com/googleapis/google-cloud-go/issues/4167)
* **storage:** try to reopen for failed Reads ([#4226](https://www.github.com/googleapis/google-cloud-go/issues/4226)) ([564102b](https://www.github.com/googleapis/google-cloud-go/commit/564102b335dbfb558bec8af883e5f898efb5dd10)), refs [#3040](https://www.github.com/googleapis/google-cloud-go/issues/3040)

## [1.15.0](https://www.github.com/googleapis/google-cloud-go/compare/storage/v1.13.0...storage/v1.15.0) (2021-04-21)


### Features

* **transport** Bump dependency on google.golang.org/api to pick up HTTP/2
  config updates (see [googleapis/google-api-go-client#882](https://github.com/googleapis/google-api-go-client/pull/882)).

### Bug Fixes

* **storage:** retry io.ErrUnexpectedEOF ([#3957](https://www.github.com/googleapis/google-cloud-go/issues/3957)) ([f6590cd](https://www.github.com/googleapis/google-cloud-go/commit/f6590cdc26c8479be5df48949fa59f879e0c24fc))


## v1.14.0

- Updates to various dependencies.

## [1.13.0](https://www.github.com/googleapis/google-cloud-go/compare/storage/v1.12.0...v1.13.0) (2021-02-03)


### Features

* **storage:** add missing StorageClass in BucketAttrsToUpdate ([#3038](https://www.github.com/googleapis/google-cloud-go/issues/3038)) ([2fa1b72](https://www.github.com/googleapis/google-cloud-go/commit/2fa1b727f8a7b20aa62fe0990530744f6c109be0))
* **storage:** add projection parameter for BucketHandle.Objects() ([#3549](https://www.github.com/googleapis/google-cloud-go/issues/3549)) ([9b9c3dc](https://www.github.com/googleapis/google-cloud-go/commit/9b9c3dce3ee10af5b6c4d070821bf47a861efd5b))


### Bug Fixes

* **storage:** fix endpoint selection logic ([#3172](https://www.github.com/googleapis/google-cloud-go/issues/3172)) ([99edf0d](https://www.github.com/googleapis/google-cloud-go/commit/99edf0d211a9e617f2586fbc83b6f9630da3c537))

## v1.12.0
- V4 signed URL fixes:
  - Fix encoding of spaces in query parameters.
  - Add fields that were missing from PostPolicyV4 policy conditions.
- Fix Query to correctly list prefixes as well as objects when SetAttrSelection
  is used.

## v1.11.0
- Add support for CustomTime and NoncurrentTime object lifecycle management
  features.

## v1.10.0
- Bump dependency on google.golang.org/api to capture changes to retry logic
  which will make retries on writes more resilient.
- Improve documentation for Writer.ChunkSize.
- Fix a bug in lifecycle to allow callers to clear lifecycle rules on a bucket.

## v1.9.0
- Add retry for transient network errors on most operations (with the exception
  of writes).
- Bump dependency for google.golang.org/api to capture a change in the default
  HTTP transport which will improve performance for reads under heavy load.
- Add CRC32C checksum validation option to Composer.

## v1.8.0
- Add support for V4 signed post policies.

## v1.7.0
- V4 signed URL support:
  - Add support for bucket-bound domains and virtual hosted style URLs.
  - Add support for query parameters in the signature.
  - Fix text encoding to align with standards.
- Add the object name to query parameters for write calls.
- Fix retry behavior when reading files with Content-Encoding gzip.
- Fix response header in reader.
- New code examples:
   - Error handling for `ObjectHandle` preconditions.
   - Existence checks for buckets and objects.

## v1.6.0

- Updated option handling:
  - Don't drop custom scopes (#1756)
  - Don't drop port in provided endpoint (#1737)

## v1.5.0

- Honor WithEndpoint client option for reads as well as writes.
- Add archive storage class to docs.
- Make fixes to storage benchwrapper.

## v1.4.0

- When listing objects in a bucket, allow callers to specify which attributes
  are queried. This allows for performance optimization.

## v1.3.0

- Use `storage.googleapis.com/storage/v1` by default for GCS requests
  instead of `www.googleapis.com/storage/v1`.

## v1.2.1

- Fixed a bug where UniformBucketLevelAccess and BucketPolicyOnly were not
  being sent in all cases.

## v1.2.0

- Add support for UniformBucketLevelAccess. This configures access checks
  to use only bucket-level IAM policies.
  See: https://godoc.org/cloud.google.com/go/storage#UniformBucketLevelAccess.
- Fix userAgent to use correct version.

## v1.1.2

- Fix memory leak in BucketIterator and ObjectIterator.

## v1.1.1

- Send BucketPolicyOnly even when it's disabled.

## v1.1.0

- Performance improvements for ObjectIterator and BucketIterator.
- Fix Bucket.ObjectIterator size calculation checks.
- Added HMACKeyOptions to all the methods which allows for options such as
  UserProject to be set per invocation and optionally be used.

## v1.0.0

This is the first tag to carve out storage as its own module. See:
https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository.
