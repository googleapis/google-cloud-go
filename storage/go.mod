module cloud.google.com/go/storage

go 1.19

retract [v1.25.0, v1.27.0] // due to https://github.com/googleapis/google-cloud-go/issues/6857

require (
	cloud.google.com/go v0.110.2
	cloud.google.com/go/compute/metadata v0.2.3
	cloud.google.com/go/iam v1.1.0
	github.com/golang/protobuf v1.5.3
	github.com/google/go-cmp v0.5.9
	github.com/google/uuid v1.3.0
	github.com/googleapis/gax-go/v2 v2.11.0
	golang.org/x/oauth2 v0.8.0
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2
	google.golang.org/api v0.126.0
	google.golang.org/genproto v0.0.0-20230530153820-e85fd2cbaebc
	google.golang.org/genproto/googleapis/api v0.0.0-20230530153820-e85fd2cbaebc
	google.golang.org/grpc v1.55.0
	google.golang.org/protobuf v1.30.0
)

require (
	cloud.google.com/go/compute v1.19.3 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/martian/v3 v3.3.2 // indirect
	github.com/google/s2a-go v0.1.4 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.2.3 // indirect
	go.opencensus.io v0.24.0 // indirect
	golang.org/x/crypto v0.9.0 // indirect
	golang.org/x/net v0.10.0 // indirect
	golang.org/x/sys v0.8.0 // indirect
	golang.org/x/text v0.9.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230530153820-e85fd2cbaebc // indirect
)
