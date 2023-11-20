module cloud.google.com/go/storage

go 1.19

retract [v1.25.0, v1.27.0] // due to https://github.com/googleapis/google-cloud-go/issues/6857

require (
	cloud.google.com/go v0.110.8
	cloud.google.com/go/compute/metadata v0.2.3
	cloud.google.com/go/iam v1.1.3
	github.com/golang/protobuf v1.5.3
	github.com/google/go-cmp v0.6.0
	github.com/google/uuid v1.4.0
	github.com/googleapis/gax-go/v2 v2.12.0
	golang.org/x/oauth2 v0.13.0
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2
	google.golang.org/api v0.150.0
	google.golang.org/genproto v0.0.0-20231016165738-49dd2c1f3d0b
	google.golang.org/genproto/googleapis/api v0.0.0-20231016165738-49dd2c1f3d0b
	google.golang.org/grpc v1.59.0
	google.golang.org/protobuf v1.31.0
)

require (
	cloud.google.com/go/compute v1.23.1 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/martian/v3 v3.3.2 // indirect
	github.com/google/s2a-go v0.1.7 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.2 // indirect
	go.opencensus.io v0.24.0 // indirect
	golang.org/x/crypto v0.14.0 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/sync v0.5.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231030173426-d783a09b4405 // indirect
)
