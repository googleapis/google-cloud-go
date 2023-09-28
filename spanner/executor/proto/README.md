protoc --go_out=plugins=grpc:. -I=/usr/local/google/home/sriharshach/github/Executor-Framework/googleapis/google/api/ -I=/usr/local/google/home/sriharshach/github/Executor-Framework/google-cloud-go/spanner/executor/proto/ *.proto 

protoc --go_out=plugins=grpc:. -I=/usr/local/google/home/sriharshach/github/Executor-Framework/googleapis/google/api/ /usr/local/google/home/sriharshach/github/Executor-Framework/google-cloud-go/spanner/executor/proto/cloud_executor.proto 


protoc --go_out=plugins=grpc:. -I=/usr/local/google/home/sriharshach/github/Executor-Framework/googleapis/google/api/ /usr/local/google/home/sriharshach/github/Executor-Framework/google-cloud-go/spanner/executor/proto/cloud_executor.proto

protoc --go_out=. --go_opt=paths=./ \
--go-grpc_out=. --go-grpc_opt=paths=./ \
-I=/usr/local/google/home/sriharshach/github/Executor-Framework/googleapis/google/api/ -I=/usr/local/google/home/sriharshach/github/Executor-Framework/google-cloud-go/spanner/executor/proto1/ *.proto