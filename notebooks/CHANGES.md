# Changes


## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/notebooks/v1.5.0...notebooks/v1.6.0) (2022-12-01)


### Features

* **notebooks:** added UpdateRuntime, UpgradeRuntime, DiagnoseRuntime, DiagnoseInstance to v1 API feat: add Instance.reservation_affinity, nic_type, can_ip_forward to v1beta1 API feat: add IsInstanceUpgradeableResponse.upgrade_image to v1beta1 API feat: added Location and IAM methods fix: deprecate AcceleratorType.NVIDIA_TESLA_K80 ([22ec3e3](https://github.com/googleapis/google-cloud-go/commit/22ec3e3e727f8c0232059a5d31bccd12b7b5034c))


### Documentation

* **notebooks:** fix minor docstring formatting ([7231644](https://github.com/googleapis/google-cloud-go/commit/7231644e71f05abc864924a0065b9ea22a489180))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/notebooks/v1.4.0...notebooks/v1.5.0) (2022-11-03)


### Features

* **notebooks:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/notebooks/v1.3.0...notebooks/v1.4.0) (2022-10-25)


### Features

* **notebooks:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/notebooks/v1.2.0...notebooks/v1.3.0) (2022-09-21)


### Features

* **notebooks:** rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/notebooks/v1.1.0...notebooks/v1.2.0) (2022-09-19)


### Features

* **notebooks:** start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))

## [1.1.0](https://github.com/googleapis/google-cloud-go/compare/notebooks/v1.0.0...notebooks/v1.1.0) (2022-09-15)


### Features

* **notebooks/apiv1beta1:** add REST transport ([f7b0822](https://github.com/googleapis/google-cloud-go/commit/f7b082212b1e46ff2f4126b52d49618785c2e8ca))

## [1.0.0](https://github.com/googleapis/google-cloud-go/compare/notebooks/v0.4.0...notebooks/v1.0.0) (2022-06-29)


### Features

* **notebooks:** release 1.0.0 ([7678be5](https://github.com/googleapis/google-cloud-go/commit/7678be543d9130dcd8fc4147608a10b70faef44e))


### Miscellaneous Chores

* **notebooks:** release 1.0.0 ([1b39bf4](https://github.com/googleapis/google-cloud-go/commit/1b39bf40f7fd25c3a4a60661929ec37f6a814898))

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/notebooks/v0.3.0...notebooks/v0.4.0) (2022-05-09)


### Features

* **notebooks:** start generating apiv1 ([#6004](https://github.com/googleapis/google-cloud-go/issues/6004)) ([1084ab1](https://github.com/googleapis/google-cloud-go/commit/1084ab16ca4dab6022bb06fdf5c380e52044171f)), refs [#5961](https://github.com/googleapis/google-cloud-go/issues/5961)

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/notebooks/v0.2.0...notebooks/v0.3.0) (2022-02-23)


### Features

* **notebooks:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/notebooks/v0.1.0...notebooks/v0.2.0) (2022-02-14)


### Features

* **notebooks:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## v0.1.0

This is the first tag to carve out notebooks as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
