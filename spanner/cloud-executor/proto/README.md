# Regenerating protos

Cloud Spanner Executor Framework - To generate code manually for cloud_executor.proto file using protoc, run the command below.
```
cd spanner/cloud-executor/proto
protoc --go_out=plugins=grpc:. -I=<local path to googleapis> -I=./ cloud_executor.proto
```