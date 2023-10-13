# Regenerating protos

```
cd spanner/cloud-executor/proto
protoc --go_out=plugins=grpc:. -I=<local path to googleapis> -I=./ cloud_executor.proto
```