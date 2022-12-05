# Changes

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/kms/v1.6.0...kms/v1.7.0) (2022-12-01)


### Features

* **kms:** add SHA-2 import methods ([7231644](https://github.com/googleapis/google-cloud-go/commit/7231644e71f05abc864924a0065b9ea22a489180))
* **kms:** add support for additional HMAC algorithms ([2a0b1ae](https://github.com/googleapis/google-cloud-go/commit/2a0b1aeb1683222e6aa5c876cb945845c00cef79))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/kms/v1.5.0...kms/v1.6.0) (2022-11-03)


### Features

* **kms:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/kms/v1.4.0...kms/v1.5.0) (2022-10-25)


### Features

* **kms:** enable generation of Locations mixin ([caf4afa](https://github.com/googleapis/google-cloud-go/commit/caf4afa139ad7b38b6df3e3b17b8357c81e1fd6c))
* **kms:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/kms/v1.3.0...kms/v1.4.0) (2022-02-23)


### Features

* **kms:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/kms/v1.2.0...kms/v1.3.0) (2022-02-14)


### Features

* **kms:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [1.2.0](https://www.github.com/googleapis/google-cloud-go/compare/kms/v1.1.0...kms/v1.2.0) (2022-02-04)


### Features

* **kms:** add a new EkmService API ([7f48e6b](https://www.github.com/googleapis/google-cloud-go/commit/7f48e6b68e59812208ea87b7861fad60169dc63a))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/kms/v1.0.0...kms/v1.1.0) (2021-10-18)


### Features

* **kms:** add OAEP+SHA1 to the list of supported algorithms ([8c5c6cf](https://www.github.com/googleapis/google-cloud-go/commit/8c5c6cf9df046b67998a8608d05595bd9e34feb0))
* **kms:** add RPC retry information for MacSign, MacVerify, and GenerateRandomBytes Committer: [@bdhess](https://www.github.com/bdhess) ([1a0720f](https://www.github.com/googleapis/google-cloud-go/commit/1a0720f2f33bb14617f5c6a524946a93209e1266))
* **kms:** add support for Raw PKCS[#1](https://www.github.com/googleapis/google-cloud-go/issues/1) signing keys ([58bea89](https://www.github.com/googleapis/google-cloud-go/commit/58bea89a3d177d5c431ff19310794e3296253353))

## 1.0.0

Stabilize GA surface.

## [0.2.0](https://www.github.com/googleapis/google-cloud-go/compare/kms/v0.1.0...kms/v0.2.0) (2021-08-30)


### Features

* **kms:** add support for Key Reimport ([bf4378b](https://www.github.com/googleapis/google-cloud-go/commit/bf4378b5b859f7b835946891dbfebfee31c4b123))

## v0.1.0

This is the first tag to carve out kms as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
