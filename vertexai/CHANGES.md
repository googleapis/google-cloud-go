# Changelog

## [0.13.2](https://github.com/googleapis/google-cloud-go/compare/vertexai/v0.13.1...vertexai/v0.13.2) (2024-11-04)


### Bug Fixes

* **vertexai:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **vertexai:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([2b8ca4b](https://github.com/googleapis/google-cloud-go/commit/2b8ca4b4127ce3025c7a21cc7247510e07cc5625))

## [0.13.1](https://github.com/googleapis/google-cloud-go/compare/vertexai/v0.13.0...vertexai/v0.13.1) (2024-09-05)


### Bug Fixes

* **vertexai:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [0.13.0](https://github.com/googleapis/google-cloud-go/compare/vertexai/v0.12.0...vertexai/v0.13.0) (2024-08-22)


### Features

* **vertexai/genai:** Add WithClientInfo option ([#10535](https://github.com/googleapis/google-cloud-go/issues/10535)) ([265963b](https://github.com/googleapis/google-cloud-go/commit/265963bd5b91c257b3c3d3c1f52cdf2b5f4c9d1a))
* **vertexai:** Update tokenizer documentation and pull new code ([#10718](https://github.com/googleapis/google-cloud-go/issues/10718)) ([0ee1430](https://github.com/googleapis/google-cloud-go/commit/0ee1430154f4d51d84b5d5927b1b477f6beb0fc1))


### Bug Fixes

* **vertexai:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))
* **vertexai:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))
* **vertexai:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))
* **vertexai:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [0.12.0](https://github.com/googleapis/google-cloud-go/compare/vertexai/v0.11.0...vertexai/v0.12.0) (2024-06-12)


### Features

* **vertexai/genai:** Add MergedResponse method to GenerateContentResponseIterator ([#10355](https://github.com/googleapis/google-cloud-go/issues/10355)) ([9d365d1](https://github.com/googleapis/google-cloud-go/commit/9d365d113bd9c89beed640fb3de17747ab580993))

## [0.11.0](https://github.com/googleapis/google-cloud-go/compare/vertexai/v0.10.0...vertexai/v0.11.0) (2024-06-11)


### Features

* **vertexai:** Explicit caching ([#10363](https://github.com/googleapis/google-cloud-go/issues/10363)) ([d9754c7](https://github.com/googleapis/google-cloud-go/commit/d9754c7c07656b2f68cb63f24f1da4ddcc697f8f))


### Bug Fixes

* **vertexai:** Don't add empty Text parts to session history ([#10362](https://github.com/googleapis/google-cloud-go/issues/10362)) ([088b6c3](https://github.com/googleapis/google-cloud-go/commit/088b6c3afd85d75ce3b30af0620529ec04d4ce1c)), refs [#10309](https://github.com/googleapis/google-cloud-go/issues/10309)

## [0.10.0](https://github.com/googleapis/google-cloud-go/compare/vertexai/v0.9.0...vertexai/v0.10.0) (2024-05-20)


### Features

* **vertexai:** Infer location when not passed explicitly ([#10222](https://github.com/googleapis/google-cloud-go/issues/10222)) ([4f1f033](https://github.com/googleapis/google-cloud-go/commit/4f1f0339b30d44b52eddcbadd504c31ab215db2e))
* **vertexai:** Support model garden and tuned models names ([#10197](https://github.com/googleapis/google-cloud-go/issues/10197)) ([d481e0e](https://github.com/googleapis/google-cloud-go/commit/d481e0e746d6c19dc51493b0311f7b8a8029e017)), refs [#9630](https://github.com/googleapis/google-cloud-go/issues/9630)

## [0.9.0](https://github.com/googleapis/google-cloud-go/compare/vertexai/v0.8.0...vertexai/v0.9.0) (2024-05-13)


### Features

* **vertexai:** Add Candidate.FunctionCalls accessor ([#10149](https://github.com/googleapis/google-cloud-go/issues/10149)) ([6c76a67](https://github.com/googleapis/google-cloud-go/commit/6c76a67af1b630e48597a352fface154fcfdacfb))

## [0.8.0](https://github.com/googleapis/google-cloud-go/compare/vertexai/v0.7.1...vertexai/v0.8.0) (2024-05-06)


### Features

* **vertexai/genai:** Add SystemInstruction ([#9736](https://github.com/googleapis/google-cloud-go/issues/9736)) ([84e3236](https://github.com/googleapis/google-cloud-go/commit/84e3236355de8d3d018c49d64d8dffe67caaf49d))
* **vertexai/genai:** Change TopK to int ([#9522](https://github.com/googleapis/google-cloud-go/issues/9522)) ([29d2c7d](https://github.com/googleapis/google-cloud-go/commit/29d2c7d0be85f0055f4992dc01897782b8a51bcb))
* **vertexai/genai:** Constrained decoding ([#9731](https://github.com/googleapis/google-cloud-go/issues/9731)) ([bb84fbd](https://github.com/googleapis/google-cloud-go/commit/bb84fbd185448bdee5e848e761f094b91365e4c2))
* **vertexai/genai:** Update to latest protos ([#9555](https://github.com/googleapis/google-cloud-go/issues/9555)) ([e078458](https://github.com/googleapis/google-cloud-go/commit/e0784583abdd40bdf7f5c0646cda369926202a63))


### Bug Fixes

* **vertexai/genai:** Check for nil content ([#10057](https://github.com/googleapis/google-cloud-go/issues/10057)) ([22e3eae](https://github.com/googleapis/google-cloud-go/commit/22e3eaee413ea314963f6f9f31d09e439be989b3))
* **vertexai:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))
* **vertexai:** Clarify Client.GenerativeModel documentation ([#9533](https://github.com/googleapis/google-cloud-go/issues/9533)) ([511d9b2](https://github.com/googleapis/google-cloud-go/commit/511d9b2d7055a2711b3976c319e98d7aec31121f))
* **vertexai:** Clarify documentation of NewClient ([#9532](https://github.com/googleapis/google-cloud-go/issues/9532)) ([f1bca4c](https://github.com/googleapis/google-cloud-go/commit/f1bca4cde57239cd3c606a1566e83a7d7f5e7953))
* **vertexai:** If GenerateContentResponse.Candidates.Content is nil will panic ([#9687](https://github.com/googleapis/google-cloud-go/issues/9687)) ([966a0c3](https://github.com/googleapis/google-cloud-go/commit/966a0c30407748b039ecff608b85754de1f3820e))
* **vertexai:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))


### Documentation

* **vertexai:** Fix typo in README ([#9690](https://github.com/googleapis/google-cloud-go/issues/9690)) ([bac84bf](https://github.com/googleapis/google-cloud-go/commit/bac84bf20bf2aef21a5bdae93792aaf13ec0349c))

## [0.7.1](https://github.com/googleapis/google-cloud-go/compare/vertexai/v0.7.0...vertexai/v0.7.1) (2024-02-08)


### Bug Fixes

* **vertexai:** Fix dependency on aiplatform ([#9394](https://github.com/googleapis/google-cloud-go/issues/9394)) ([8bd57a1](https://github.com/googleapis/google-cloud-go/commit/8bd57a1abf3d65651f25aba9c582ff273a678dfa))

## [0.7.0](https://github.com/googleapis/google-cloud-go/compare/vertexai/v0.6.0...vertexai/v0.7.0) (2024-02-08)


### Features

* **vertexai:** Add WithREST option to vertexai client ([#9389](https://github.com/googleapis/google-cloud-go/issues/9389)) ([f5d56eb](https://github.com/googleapis/google-cloud-go/commit/f5d56eb03558fce093a5b9947ae041fba4d844b2))

## [0.6.0](https://github.com/googleapis/google-cloud-go/compare/vertexai/v0.5.2...vertexai/v0.6.0) (2023-12-17)


### Features

* **vertexai:** Use pointers for GenerationConfig fields ([#9182](https://github.com/googleapis/google-cloud-go/issues/9182)) ([91990c1](https://github.com/googleapis/google-cloud-go/commit/91990c1746945c7f0548df972acf1498b165beb9))

## [0.5.2](https://github.com/googleapis/google-cloud-go/compare/vertexai/v0.5.1...vertexai/v0.5.2) (2023-12-14)


### Bug Fixes

* **vertexai:** Bump deps to fix build error ([#9168](https://github.com/googleapis/google-cloud-go/issues/9168)) ([d361c59](https://github.com/googleapis/google-cloud-go/commit/d361c59953ec815bc3fbd0fdba04069c68e5cd99)), refs [#9167](https://github.com/googleapis/google-cloud-go/issues/9167)

## [0.5.1](https://github.com/googleapis/google-cloud-go/compare/vertexai/v0.5.0...vertexai/v0.5.1) (2023-12-13)


### Bug Fixes

* **vertexai:** Passthrough user opts ([#9163](https://github.com/googleapis/google-cloud-go/issues/9163)) ([c24e93c](https://github.com/googleapis/google-cloud-go/commit/c24e93c06851d3917d75a9b2362af993071961c0)), refs [#9160](https://github.com/googleapis/google-cloud-go/issues/9160)

## [0.5.0](https://github.com/googleapis/google-cloud-go/compare/vertexai/v0.4.0...vertexai/v0.5.0) (2023-12-13)


### Features

* **vertexai:** Fix README link and add example comments ([#9157](https://github.com/googleapis/google-cloud-go/issues/9157)) ([25f04f2](https://github.com/googleapis/google-cloud-go/commit/25f04f2adf24bebacefd686a378aad986f3a192c))


### Bug Fixes

* **vertexai:** Set internal headers ([#9151](https://github.com/googleapis/google-cloud-go/issues/9151)) ([eb5a007](https://github.com/googleapis/google-cloud-go/commit/eb5a007d1ddaece1438fa02cc465a501bad05d4b))

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/vertexai/v0.3.0...vertexai/v0.4.0) (2023-12-13)


### Features

* **vertexai:** Add UsageMetadata ([#9155](https://github.com/googleapis/google-cloud-go/issues/9155)) ([27498e0](https://github.com/googleapis/google-cloud-go/commit/27498e05155ec8e93eb4e9b261b7aed4556a6bac))
* **vertexai:** Update README with more details and links ([#9146](https://github.com/googleapis/google-cloud-go/issues/9146)) ([6dfbc78](https://github.com/googleapis/google-cloud-go/commit/6dfbc780548f7fe797a8618cb42f6b0ca12638c4))

## 0.1.0 (2023-12-11)


### Features

* **vertexai:** Add CountTokens ([#9109](https://github.com/googleapis/google-cloud-go/issues/9109)) ([1372adf](https://github.com/googleapis/google-cloud-go/commit/1372adfe412d4ebcac4db5989e8a7bc290979c62))
* **vertexai:** Add function support ([#9113](https://github.com/googleapis/google-cloud-go/issues/9113)) ([d15779e](https://github.com/googleapis/google-cloud-go/commit/d15779e00dc577dfe3075915fc56d4120c03c72c))
* **vertexai:** Add string methods ([#9121](https://github.com/googleapis/google-cloud-go/issues/9121)) ([27f31ed](https://github.com/googleapis/google-cloud-go/commit/27f31edf5f4c932a37a80667dc7b9b4d44d246a9))
* **vertexai:** Vertex AI for go ([#9095](https://github.com/googleapis/google-cloud-go/issues/9095)) ([b3b293a](https://github.com/googleapis/google-cloud-go/commit/b3b293aee06690ed734bb19c404eb6c8af893fa1))


### Bug Fixes

* **vertexai:** Nil pointer exception (probably) ([#9106](https://github.com/googleapis/google-cloud-go/issues/9106)) ([1ce1ace](https://github.com/googleapis/google-cloud-go/commit/1ce1ace31af3439b4cabdf92562044a787996ac9))
* **vertexai:** Set up endpoint and fix test ([#9105](https://github.com/googleapis/google-cloud-go/issues/9105)) ([c0d92f9](https://github.com/googleapis/google-cloud-go/commit/c0d92f95115751d36adc3ebbd5f4413e4e0db17a))

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/vertexai/v0.2.0...vertexai/v0.3.0) (2023-12-11)


### Features

* **vertexai:** Add function support ([#9113](https://github.com/googleapis/google-cloud-go/issues/9113)) ([d15779e](https://github.com/googleapis/google-cloud-go/commit/d15779e00dc577dfe3075915fc56d4120c03c72c))
* **vertexai:** Add string methods ([#9121](https://github.com/googleapis/google-cloud-go/issues/9121)) ([27f31ed](https://github.com/googleapis/google-cloud-go/commit/27f31edf5f4c932a37a80667dc7b9b4d44d246a9))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/vertexai/v0.1.1...vertexai/v0.2.0) (2023-12-08)


### Features

* **vertexai:** Add CountTokens ([#9109](https://github.com/googleapis/google-cloud-go/issues/9109)) ([1372adf](https://github.com/googleapis/google-cloud-go/commit/1372adfe412d4ebcac4db5989e8a7bc290979c62))

## [0.1.1](https://github.com/googleapis/google-cloud-go/compare/vertexai/v0.1.0...vertexai/v0.1.1) (2023-12-08)


### Bug Fixes

* **vertexai:** Nil pointer exception (probably) ([#9106](https://github.com/googleapis/google-cloud-go/issues/9106)) ([1ce1ace](https://github.com/googleapis/google-cloud-go/commit/1ce1ace31af3439b4cabdf92562044a787996ac9))
* **vertexai:** Set up endpoint and fix test ([#9105](https://github.com/googleapis/google-cloud-go/issues/9105)) ([c0d92f9](https://github.com/googleapis/google-cloud-go/commit/c0d92f95115751d36adc3ebbd5f4413e4e0db17a))

## 0.1.0 (2023-12-07)


### Features

* **vertexai:** Vertex AI for go ([#9095](https://github.com/googleapis/google-cloud-go/issues/9095)) ([b3b293a](https://github.com/googleapis/google-cloud-go/commit/b3b293aee06690ed734bb19c404eb6c8af893fa1))
