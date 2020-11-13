# Changes

## v1.6.0
- Add support partial results in InstanceAdminClient.Instances. In the case of
  partial availability, available instances will be returned along with an
  ErrPartiallyUnavailable error.
- Add support for label filters.
- Fix max valid timestamp in the emulator to allow reversed timestamp support.

## v1.5.0
- Add support for managed backups.

## v1.4.0
- Add support for instance state and labels to the admin API.
- Add metadata header to all data requests.
- Fix bug in timestamp to time conversion.

## v1.3.0

- Clients now use transport/grpc.DialPool rather than Dial.
  - Connection pooling now does not use the deprecated (and soon to be removed) gRPC load balancer API.

## v1.2.0

- Update cbt usage string.

- Fix typo in cbt tool.

- Ignore empty lines in cbtrc.

- Emulator now rejects microseconds precision.

## v1.1.0

- Add support to cbt tool to drop all rows from a table.

- Adds a method to update an instance with clusters.

- Adds StorageType to ClusterInfo.

- Add support for the `-auth-token` flag to cbt tool.

- Adds support for Table-level IAM, including some bug fixes.

## v1.0.0

This is the first tag to carve out bigtable as its own module. See:
https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository.
