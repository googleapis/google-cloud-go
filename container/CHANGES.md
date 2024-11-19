# Changes

## [1.42.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.41.0...container/v1.42.0) (2024-11-14)


### Features

* **container:** Add desired_enterprise_config,desired_node_pool_auto_config_linux_node_config to ClusterUpdate. ([f072178](https://github.com/googleapis/google-cloud-go/commit/f072178f6fd90537a5782395f4229e4c8b30af7e))
* **container:** Add desired_tier to EnterpriseConfig. ([f072178](https://github.com/googleapis/google-cloud-go/commit/f072178f6fd90537a5782395f4229e4c8b30af7e))
* **container:** Add DesiredEnterpriseConfig proto message ([f072178](https://github.com/googleapis/google-cloud-go/commit/f072178f6fd90537a5782395f4229e4c8b30af7e))
* **container:** Add LinuxNodeConfig in NodePoolAutoConfig ([f072178](https://github.com/googleapis/google-cloud-go/commit/f072178f6fd90537a5782395f4229e4c8b30af7e))
* **container:** Add LocalSsdEncryptionMode in NodeConfig ([#11103](https://github.com/googleapis/google-cloud-go/issues/11103)) ([f072178](https://github.com/googleapis/google-cloud-go/commit/f072178f6fd90537a5782395f4229e4c8b30af7e))
* **container:** Add UpgradeInfoEvent proto message ([f072178](https://github.com/googleapis/google-cloud-go/commit/f072178f6fd90537a5782395f4229e4c8b30af7e))


### Documentation

* **container:** Minor documentation updates ([f072178](https://github.com/googleapis/google-cloud-go/commit/f072178f6fd90537a5782395f4229e4c8b30af7e))

## [1.41.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.40.0...container/v1.41.0) (2024-10-23)


### Features

* **container:** Add an effective_cgroup_mode field in node config ([f0b05e2](https://github.com/googleapis/google-cloud-go/commit/f0b05e260435d5e8889b9a0ca0ab215fcde169ab))
* **container:** Add API to enable/disable secret manager csi component on GKE clusters ([f0b05e2](https://github.com/googleapis/google-cloud-go/commit/f0b05e260435d5e8889b9a0ca0ab215fcde169ab))
* **container:** Add CompliancePosture field for configuration of GKE Compliance Posture product ([f0b05e2](https://github.com/googleapis/google-cloud-go/commit/f0b05e260435d5e8889b9a0ca0ab215fcde169ab))
* **container:** Add CompliancePosture field for configuration of GKE Compliance Posture product ([f0b05e2](https://github.com/googleapis/google-cloud-go/commit/f0b05e260435d5e8889b9a0ca0ab215fcde169ab))
* **container:** Add KCP_SSHD and KCP_CONNECTION to the supported values for the --logging flag for the create and update cluster commands ([f0b05e2](https://github.com/googleapis/google-cloud-go/commit/f0b05e260435d5e8889b9a0ca0ab215fcde169ab))
* **container:** Add RBACBindingConfig to API ([f0b05e2](https://github.com/googleapis/google-cloud-go/commit/f0b05e260435d5e8889b9a0ca0ab215fcde169ab))
* **container:** Added support for Parallelstore CSI Driver ([f0b05e2](https://github.com/googleapis/google-cloud-go/commit/f0b05e260435d5e8889b9a0ca0ab215fcde169ab))
* **container:** Surface upgrade_target_version in GetServerConfig for all supported release channels ([f0b05e2](https://github.com/googleapis/google-cloud-go/commit/f0b05e260435d5e8889b9a0ca0ab215fcde169ab))


### Bug Fixes

* **container:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **container:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))


### Documentation

* **container:** Minor documentation updates ([f0b05e2](https://github.com/googleapis/google-cloud-go/commit/f0b05e260435d5e8889b9a0ca0ab215fcde169ab))

## [1.40.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.39.0...container/v1.40.0) (2024-09-12)


### Features

* **container:** Add ReleaseChannel EXTENDED value ([b3ea577](https://github.com/googleapis/google-cloud-go/commit/b3ea5776b171fc60b4e96035d56d35dbd7505f3b))


### Bug Fixes

* **container:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.39.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.38.1...container/v1.39.0) (2024-08-20)


### Features

* **container:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.38.1](https://github.com/googleapis/google-cloud-go/compare/container/v1.38.0...container/v1.38.1) (2024-08-08)


### Bug Fixes

* **container:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.38.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.37.3...container/v1.38.0) (2024-07-24)


### Features

* **container:** Add DCGM enum in monitoring config ([eb63f0d](https://github.com/googleapis/google-cloud-go/commit/eb63f0d4f42a06581e1425f99c2a03d52d6cb404))
* **container:** Support for Ray Clusters ([1bb4c84](https://github.com/googleapis/google-cloud-go/commit/1bb4c846ec1ff37f394afb1684823ea76c18d16e))


### Bug Fixes

* **container:** Deprecate "EXPERIMENTAL" option for Gateway API (this value has never been supported) ([eb63f0d](https://github.com/googleapis/google-cloud-go/commit/eb63f0d4f42a06581e1425f99c2a03d52d6cb404))
* **container:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))


### Documentation

* **container:** Minor updates to reference documentation ([#10579](https://github.com/googleapis/google-cloud-go/issues/10579)) ([ddbc02b](https://github.com/googleapis/google-cloud-go/commit/ddbc02b8630b7cc9d49d1692e317521db7a8b825))
* **container:** Trivial updates ([1bb4c84](https://github.com/googleapis/google-cloud-go/commit/1bb4c846ec1ff37f394afb1684823ea76c18d16e))

## [1.37.3](https://github.com/googleapis/google-cloud-go/compare/container/v1.37.2...container/v1.37.3) (2024-07-10)


### Bug Fixes

* **container:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.37.2](https://github.com/googleapis/google-cloud-go/compare/container/v1.37.1...container/v1.37.2) (2024-07-01)


### Bug Fixes

* **container:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [1.37.1](https://github.com/googleapis/google-cloud-go/compare/container/v1.37.0...container/v1.37.1) (2024-06-26)


### Bug Fixes

* **container:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.37.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.36.0...container/v1.37.0) (2024-06-12)


### Features

* **container:** Enable REST transport for google/container/v1 ([#10356](https://github.com/googleapis/google-cloud-go/issues/10356)) ([fc9e895](https://github.com/googleapis/google-cloud-go/commit/fc9e895c460d6911edbe0b47d8fc689cf76a4a58))

## [1.36.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.35.1...container/v1.36.0) (2024-06-10)


### Features

* **container:** A new field `accelerators` is added to message `.google.container.v1.UpdateNodePoolRequest` ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A new field `additive_vpc_scope_dns_domain` is added to message `.google.container.v1.DNSConfig` ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A new field `containerd_config` is added to message `.google.container.v1.NodeConfig` ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A new field `containerd_config` is added to message `.google.container.v1.NodeConfigDefaults` ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A new field `containerd_config` is added to message `.google.container.v1.UpdateNodePoolRequest` ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A new field `desired_containerd_config` is added to message `.google.container.v1.ClusterUpdate` ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A new field `desired_node_kubelet_config` is added to message `.google.container.v1.ClusterUpdate` ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A new field `desired_node_pool_auto_config_kubelet_config` is added to message `.google.container.v1.ClusterUpdate` ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A new field `enable_nested_virtualization` is added to message `.google.container.v1.AdvancedMachineFeatures` ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A new field `hugepages` is added to message `.google.container.v1.LinuxNodeConfig` ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A new field `node_kubelet_config` is added to message `.google.container.v1.NodeConfigDefaults` ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A new field `node_kubelet_config` is added to message `.google.container.v1.NodePoolAutoConfig` ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A new field `satisfies_pzi` is added to message `.google.container.v1.Cluster` ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A new field `satisfies_pzs` is added to message `.google.container.v1.Cluster` ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A new message `ContainerdConfig` is added ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A new message `HugepagesConfig` is added ([#10346](https://github.com/googleapis/google-cloud-go/issues/10346)) ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A new method_signature `parent` is added to method `ListOperations` in service `ClusterManager` ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A new value `CADVISOR` is added to enum `Component` ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A new value `ENTERPRISE` is added to enum `Mode` ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A new value `KUBELET` is added to enum `Component` ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A new value `MPS` is added to enum `GPUSharingStrategy` ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))


### Documentation

* **container:** A comment for field `desired_private_cluster_config` in message `.google.container.v1.ClusterUpdate` is changed ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **container:** A comment for field `in_transit_encryption_config` in message `.google.container.v1.NetworkConfig` is changed ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))

## [1.35.1](https://github.com/googleapis/google-cloud-go/compare/container/v1.35.0...container/v1.35.1) (2024-05-01)


### Bug Fixes

* **container:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [1.35.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.34.0...container/v1.35.0) (2024-03-27)


### Features

* **container:** Add several fields to manage state of database encryption update ([4834425](https://github.com/googleapis/google-cloud-go/commit/48344254a5d21ec51ffee275c78a15c9345dc09c))

## [1.34.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.33.1...container/v1.34.0) (2024-03-25)


### Features

* **container:** Add optional secondary_boot_disk_update_strategy field to NodePool API ([cddd528](https://github.com/googleapis/google-cloud-go/commit/cddd528a02edae10dde8ba2529922565ef27c418))

## [1.33.1](https://github.com/googleapis/google-cloud-go/compare/container/v1.33.0...container/v1.33.1) (2024-03-14)


### Bug Fixes

* **container:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.33.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.32.0...container/v1.33.0) (2024-03-07)


### Features

* **container:** Add API to enable/disable secret manager csi component on GKE clusters ([a74cbbe](https://github.com/googleapis/google-cloud-go/commit/a74cbbee6be0c02e0280f115119596da458aa707))
* **container:** Add secondary boot disks field to NodePool API ([a74cbbe](https://github.com/googleapis/google-cloud-go/commit/a74cbbee6be0c02e0280f115119596da458aa707))

## [1.32.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.31.0...container/v1.32.0) (2024-02-26)


### Features

* **container:** Add API to enable Provisioning Request API on existing nodepools ([3814ee3](https://github.com/googleapis/google-cloud-go/commit/3814ee3f27724ad0d02688ad86030b83e0a72fd4))

## [1.31.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.30.1...container/v1.31.0) (2024-02-09)


### Features

* **container:** Added configuration for the StatefulHA addon to the AddonsConfig ([46a5050](https://github.com/googleapis/google-cloud-go/commit/46a50502f033ff0afe2f17b5f1e9812a956e190e))

## [1.30.1](https://github.com/googleapis/google-cloud-go/compare/container/v1.30.0...container/v1.30.1) (2024-01-30)


### Bug Fixes

* **container:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.30.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.29.0...container/v1.30.0) (2024-01-22)


### Features

* **container:** Add fields desired_in_transit_encryption_config and in_transit_encryption_config ([af2f8b4](https://github.com/googleapis/google-cloud-go/commit/af2f8b4f3401c0b12dadb2c504aa0f902aee76de))

## [1.29.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.28.0...container/v1.29.0) (2023-11-27)


### Features

* **container:** Add enable_relay field to advanced_datapath_observability_config ([#9037](https://github.com/googleapis/google-cloud-go/issues/9037)) ([63ffff2](https://github.com/googleapis/google-cloud-go/commit/63ffff2a994d991304ba1ef93cab847fa7cd39e4))
* **container:** Add Provisioning Request API ([2020edf](https://github.com/googleapis/google-cloud-go/commit/2020edff24e3ffe127248cf9a90c67593c303e18))

## [1.28.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.27.1...container/v1.28.0) (2023-11-09)


### Features

* **container:** Added EnterpriseConfig ([b44c4b3](https://github.com/googleapis/google-cloud-go/commit/b44c4b301a91e8d4d107be6056b49a8fbdac9003))

## [1.27.1](https://github.com/googleapis/google-cloud-go/compare/container/v1.27.0...container/v1.27.1) (2023-11-01)


### Bug Fixes

* **container:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.27.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.26.2...container/v1.27.0) (2023-10-31)


### Features

* **container:** Add ResourceManagerTags API to attach tags on the underlying Compute Engine VMs of GKE Nodes which can be used to selectively enforce Cloud Firewall network firewall policies ([4d40180](https://github.com/googleapis/google-cloud-go/commit/4d40180da0557c2a2e9e2cb8b0509b429676bfc0))

## [1.26.2](https://github.com/googleapis/google-cloud-go/compare/container/v1.26.1...container/v1.26.2) (2023-10-26)


### Bug Fixes

* **container:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.26.1](https://github.com/googleapis/google-cloud-go/compare/container/v1.26.0...container/v1.26.1) (2023-10-12)


### Bug Fixes

* **container:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.26.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.25.0...container/v1.26.0) (2023-09-11)


### Features

* **container:** Add support for NodeConfig Update ([#8461](https://github.com/googleapis/google-cloud-go/issues/8461)) ([20725c8](https://github.com/googleapis/google-cloud-go/commit/20725c86c970ad24efa18c056fc3aa71dc3a4f03))

## [1.25.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.24.0...container/v1.25.0) (2023-08-14)


### Features

* **container:** Add APIs for GKE OOTB metrics packages ([fcb41cc](https://github.com/googleapis/google-cloud-go/commit/fcb41cc1d2435452ee78314c1b0362e3f21ae637))

## [1.24.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.23.0...container/v1.24.0) (2023-07-18)


### Features

* **container:** Add Multi-networking API ([#8270](https://github.com/googleapis/google-cloud-go/issues/8270)) ([4a5651c](https://github.com/googleapis/google-cloud-go/commit/4a5651caa472882fe4c7f6be400f782f60f6f258))

## [1.23.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.22.1...container/v1.23.0) (2023-07-10)


### Features

* **container:** Add `KUBE_DNS` option to `DNSConfig.cluster_dns` ([#8183](https://github.com/googleapis/google-cloud-go/issues/8183)) ([a3ec3cf](https://github.com/googleapis/google-cloud-go/commit/a3ec3cf858c7d9154338ac4cd8a9a068dc7a7f4d))

## [1.22.1](https://github.com/googleapis/google-cloud-go/compare/container/v1.22.0...container/v1.22.1) (2023-06-20)


### Bug Fixes

* **container:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.22.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.21.0...container/v1.22.0) (2023-06-13)


### Features

* **container:** Add API for GPU driver installation config ([3abdfa1](https://github.com/googleapis/google-cloud-go/commit/3abdfa14dd56cf773c477f289a7f888e20bbbd9a))

## [1.21.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.20.0...container/v1.21.0) (2023-06-07)


### Features

* **container:** Add a API field to enable FQDN Network Policy on clusters ([79eac77](https://github.com/googleapis/google-cloud-go/commit/79eac771ecf99172157cc4499ba95536778354e6))

## [1.20.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.19.0...container/v1.20.0) (2023-05-31)


### Features

* **container:** Add SoleTenantConfig API ([#8015](https://github.com/googleapis/google-cloud-go/issues/8015)) ([01eff11](https://github.com/googleapis/google-cloud-go/commit/01eff11eedb3edde69cc33db23e26be6a7e42f10))

## [1.19.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.18.1...container/v1.19.0) (2023-05-30)


### Features

* **container:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))


### Documentation

* **container:** Clarified release channel defaulting behavior for create cluster requests when release channel is unspecified ([2b3e7d9](https://github.com/googleapis/google-cloud-go/commit/2b3e7d9af7d2f500e736e3db77487127cb44ca23))

## [1.18.1](https://github.com/googleapis/google-cloud-go/compare/container/v1.18.0...container/v1.18.1) (2023-05-08)


### Bug Fixes

* **container:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.18.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.17.0...container/v1.18.0) (2023-05-03)


### Features

* **container:** Support fleet registration via cluster update ([#7877](https://github.com/googleapis/google-cloud-go/issues/7877)) ([d5d1fe9](https://github.com/googleapis/google-cloud-go/commit/d5d1fe96c9cf3cc3bb0e05fb75297a68bbbd8e41))

## [1.17.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.16.0...container/v1.17.0) (2023-04-25)


### Features

* **container:** Add support for updating additional pod IPv4 ranges for Standard and Autopilot clusters ([87a67b4](https://github.com/googleapis/google-cloud-go/commit/87a67b44b2c7ffc3cea986b255614ea0d21aa6fc))


### Documentation

* **container:** Minor formatting in docstring ([#7814](https://github.com/googleapis/google-cloud-go/issues/7814)) ([4900851](https://github.com/googleapis/google-cloud-go/commit/49008518e168fe6f7891b907d6fc14eecdef758c))
* **container:** Operation.Type is now documented in detail ([#7811](https://github.com/googleapis/google-cloud-go/issues/7811)) ([87eaf38](https://github.com/googleapis/google-cloud-go/commit/87eaf383fab91f5e1dcbaa037bff36d3044d06db))

## [1.16.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.15.0...container/v1.16.0) (2023-04-11)


### Features

* **container:** Add support for disabling pod IP cidr overprovision ([fc90e54](https://github.com/googleapis/google-cloud-go/commit/fc90e54b25bda6b339266e3e5388174339ed6a44))
* **container:** Add support for updating additional pod IPv4 ranges for Standard and Autopilot clusters ([fc90e54](https://github.com/googleapis/google-cloud-go/commit/fc90e54b25bda6b339266e3e5388174339ed6a44))

## [1.15.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.14.0...container/v1.15.0) (2023-04-04)


### Features

* **container:** Add a new fleet registration feature ([3f1ed9c](https://github.com/googleapis/google-cloud-go/commit/3f1ed9c63fb115f47607a3ab478842fe5ba0df11))

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.13.1...container/v1.14.0) (2023-03-15)


### Features

* **container:** Update iam and longrunning deps ([91a1f78](https://github.com/googleapis/google-cloud-go/commit/91a1f784a109da70f63b96414bba8a9b4254cddd))


### Documentation

* **container:** Minor grammar improvements ([69067f8](https://github.com/googleapis/google-cloud-go/commit/69067f8c0075099a84dd9d40e438711881710784))
* **container:** Minor typo fix ([69067f8](https://github.com/googleapis/google-cloud-go/commit/69067f8c0075099a84dd9d40e438711881710784))

## [1.13.1](https://github.com/googleapis/google-cloud-go/compare/container/v1.13.0...container/v1.13.1) (2023-02-14)


### Documentation

* **container:** Add clarification on whether `NodePool.version` is a required field ([2fef56f](https://github.com/googleapis/google-cloud-go/commit/2fef56f75a63dc4ff6e0eea56c7b26d4831c8e27))
* **container:** Clarified wording around the NodePoolUpdateStrategy default behavior docs: add references for available node image types ([3f118f9](https://github.com/googleapis/google-cloud-go/commit/3f118f9a4fb8ccbd96c81e6044ccb05addc78ded))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/container-v1.12.0...container/v1.13.0) (2023-01-26)


### Features

* **container:** Add a FastSocket API ([7231644](https://github.com/googleapis/google-cloud-go/commit/7231644e71f05abc864924a0065b9ea22a489180))
* **container:** Add APIs for GKE Control Plane Logs ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))
* **container:** Add compact placement feature for node pools ([2a0b1ae](https://github.com/googleapis/google-cloud-go/commit/2a0b1aeb1683222e6aa5c876cb945845c00cef79))
* **container:** Add etags for cluster and node pool update operations ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **container:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **container:** Add support for viewing the subnet IPv6 CIDR and services IPv6 CIDR assigned to dual stack clusters ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **container:** Added High Throughput Logging API for Google Kubernetes Engine docs: ReservationAffinity key field docs incorrect docs: missing period in description for min CPU platform ([bc7a5f6](https://github.com/googleapis/google-cloud-go/commit/bc7a5f609994f73e26f72a78f0ff14aa75c1c227))
* **container:** Launch GKE Cost Allocations configuration to the v1 GKE API ([de4e16a](https://github.com/googleapis/google-cloud-go/commit/de4e16a498354ea7271f5b396f7cb2bb430052aa))
* **container:** Rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))
* **container:** Start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))


### Documentation

* **container:** BinaryAuthorization.enabled field is marked as deprecated ([338d9c3](https://github.com/googleapis/google-cloud-go/commit/338d9c38b9c7f1b5e75493a2e3437c50785c561c))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/container-v1.11.0...container/v1.12.0) (2023-01-26)


### Features

* **container:** Add a FastSocket API ([7231644](https://github.com/googleapis/google-cloud-go/commit/7231644e71f05abc864924a0065b9ea22a489180))
* **container:** Add APIs for GKE Control Plane Logs ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))
* **container:** Add compact placement feature for node pools ([2a0b1ae](https://github.com/googleapis/google-cloud-go/commit/2a0b1aeb1683222e6aa5c876cb945845c00cef79))
* **container:** Add etags for cluster and node pool update operations ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **container:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **container:** Add support for viewing the subnet IPv6 CIDR and services IPv6 CIDR assigned to dual stack clusters ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **container:** Added High Throughput Logging API for Google Kubernetes Engine docs: ReservationAffinity key field docs incorrect docs: missing period in description for min CPU platform ([bc7a5f6](https://github.com/googleapis/google-cloud-go/commit/bc7a5f609994f73e26f72a78f0ff14aa75c1c227))
* **container:** Launch GKE Cost Allocations configuration to the v1 GKE API ([de4e16a](https://github.com/googleapis/google-cloud-go/commit/de4e16a498354ea7271f5b396f7cb2bb430052aa))
* **container:** Rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))
* **container:** Start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))


### Documentation

* **container:** BinaryAuthorization.enabled field is marked as deprecated ([338d9c3](https://github.com/googleapis/google-cloud-go/commit/338d9c38b9c7f1b5e75493a2e3437c50785c561c))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.10.0...container/v1.11.0) (2023-01-26)


### Features

* **container:** Add support for viewing the subnet IPv6 CIDR and services IPv6 CIDR assigned to dual stack clusters ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.9.0...container/v1.10.0) (2023-01-04)


### Features

* **container:** Add etags for cluster and node pool update operations ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **container:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.8.0...container/v1.9.0) (2022-12-01)


### Features

* **container:** add a FastSocket API ([7231644](https://github.com/googleapis/google-cloud-go/commit/7231644e71f05abc864924a0065b9ea22a489180))
* **container:** add compact placement feature for node pools ([2a0b1ae](https://github.com/googleapis/google-cloud-go/commit/2a0b1aeb1683222e6aa5c876cb945845c00cef79))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.7.0...container/v1.8.0) (2022-11-09)


### Features

* **container:** add APIs for GKE Control Plane Logs ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.6.0...container/v1.7.0) (2022-11-03)


### Features

* **container:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.5.0...container/v1.6.0) (2022-10-25)


### Features

* **container:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.4.0...container/v1.5.0) (2022-10-14)


### Features

* **container:** launch GKE Cost Allocations configuration to the v1 GKE API ([de4e16a](https://github.com/googleapis/google-cloud-go/commit/de4e16a498354ea7271f5b396f7cb2bb430052aa))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.3.1...container/v1.4.0) (2022-09-19)


### Features

* **container:** added High Throughput Logging API for Google Kubernetes Engine docs: ReservationAffinity key field docs incorrect docs: missing period in description for min CPU platform ([bc7a5f6](https://github.com/googleapis/google-cloud-go/commit/bc7a5f609994f73e26f72a78f0ff14aa75c1c227))

## [1.3.1](https://github.com/googleapis/google-cloud-go/compare/container/v1.3.0...container/v1.3.1) (2022-08-02)


### Documentation

* **container:** BinaryAuthorization.enabled field is marked as deprecated ([338d9c3](https://github.com/googleapis/google-cloud-go/commit/338d9c38b9c7f1b5e75493a2e3437c50785c561c))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.2.0...container/v1.3.0) (2022-07-12)


### Features

* **container:** add support to modify kubelet pod pid limit in node system configuration feat: support spot VM feat: support Tier 1 bandwidth feat: update support for node pool labels, taints and network tags feat: add Binauthz Evaluation mode support to GKE Classic feat: add GKE Identity Service feat: add network tags to autopilot cluster feat: support enabling Confidential Nodes in the node pool feat: support node pool blue-green upgrade feat: add Location Policy API feat: support GPU timesharing feat: add managed prometheus feature ([963efe2](https://github.com/googleapis/google-cloud-go/commit/963efe22cf67bc04fed09b5fa8f9cb20b9edf1a3))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.1.0...container/v1.2.0) (2022-02-23)


### Features

* **container:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.1.0](https://github.com/googleapis/google-cloud-go/compare/container/v1.0.0...container/v1.1.0) (2022-02-14)


### Features

* **container:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## 1.0.0

Stabilize GA surface.

## v0.1.0

This is the first tag to carve out container as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
