# Regenerating protos

source: [cloud_executor.proto](https://github.com/googleapis/cndb-client-testing-protos/blob/main/google/spanner/executor/v1/cloud_executor.proto)
```
cd spanner/executor/apiv1
protoc --go_out=plugins=grpc:. -I={local_path_to_googleapis_directory} -I=./ cloud_executor.proto

example: 
cd spanner/executor/apiv1
protoc --go_out=plugins=grpc:. -I=/github/googleapis/ -I=./ cloud_executor.proto
```