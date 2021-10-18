# Changes

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
