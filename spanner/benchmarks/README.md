# Spanner Benchmarking 

This module supports running performance benchmarking in Go against *Production* and *Cloud devel* environments.

It supports stale reads and queries.

## Commands

``go run spanner/benchmarks/benchmarks.go <option1> <value1> <option2> <value2> ...``

Please look at the [Configurations](#configurations) section for allowed options and values.

## Environmental variables

| Environment Variable                          | Description                                        | Possible values                            |
|-----------------------------------------------|----------------------------------------------------|--------------------------------------------|
| SPANNER_CLIENT_BENCHMARK_GOOGLE_CLOUD_PROJECT | To configure project id of the spanner instance    | any valid project ID                       |
| SPANNER_CLIENT_BENCHMARK_SPANNER_INSTANCE     | To configure instance id of the spanner instance   | any valid instance ID in the same project  |
| SPANNER_CLIENT_BENCHMARK_SPANNER_DATABASE     | To configure database name in the spanner instance | any valid database ID in the same instance |
| SPANNER_CLIENT_BENCHMARK_CLOUD_ENVIRONMENT    | To configure spanner environment                   | PRODUCTION, DEVEL                          |

## Configurations

| Config                  | Description                                                                                                                                                                                          | Short Option | Long Option            | Default |
|-------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|--------------|------------------------|---------|
| Warm up time            | Total warm up time before running </br> actual benchmarking                                                                                                                                          | -wu          | -warmUpTime            | 7 mins  |
| Execution time          | Total execution time of benchmarking                                                                                                                                                                 | -et          | -executionTime         | 30 mins |
| Wait between requests   | Total wait time between two requests. </br> After executing a request  script will wait </br> for sometime before starting the next request. </br> Usually it will wait 2X time of configured value. | -wbr         | -waitBetweenRequests   | 5 ms    |
| Staleness               | Total Staleness for Reads and Queries                                                                                                                                                                | -st          | -staleness             | 15 secs |
| Transaction Type        | Transaction type of benchmarking, read or query.. etc                                                                                                                                                | NA           | -transactionType       | read    |
| Traces Enabled          | To decide if traces should be enabled or not                                                                                                                                                         | -te          | -tracesEnabled         | false   |
| Disable Native Metrics  | To decide if built-in metrics should be disabled                                                                                                                                                     | -dnm         | -disableNativeMetrics  | false   |
| Trace Sampling Fraction | To configuring trace sampling fraction. 0 - No sampling, >= 1 - Always sample                                                                                                                        | -tsf         | -traceSamplingFraction | 0.5     |

## Other configurations

To enable some application specific configurations, you can reference the following table.

| Description                | Environment variable                           |
|----------------------------|------------------------------------------------|
| Enabling multiplex session | GOOGLE_CLOUD_SPANNER_MULTIPLEXED_SESSIONS=true |
| Enabling directpath        | GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS=true       |

