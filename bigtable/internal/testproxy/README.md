# Cloud Bigtable Go client library test proxy

The Bigtable test proxy is intended for use with the [Test Framework
for Cloud Bigtable Client Libraries](https://github.com/googleapis/cloud-bigtable-clients-test).

See the section on
[Test Execution](https://github.com/googleapis/cloud-bigtable-clients-test#test-execution)
for the Test Frameworks to see usage steps.

## Running the test proxy

1. Open a command terminal at the root of the Bigtable client library.

1. Run the test proxy (this)


        go run internal/testproxy/proxy.go


You can also specify a port to use for the test proxy by using the `--port`
flag:

```
go run internal/testproxy/proxy.go --port 5000
```

## Running the unit tests

1. Run the following at the root of the test proxy folder:

        go test -v .
