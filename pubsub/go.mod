module cloud.google.com/go/pubsub

go 1.20

require (
	cloud.google.com/go v0.115.0
	cloud.google.com/go/iam v1.1.8
	cloud.google.com/go/kms v1.17.1
	github.com/google/go-cmp v0.6.0
	github.com/googleapis/gax-go/v2 v2.12.4
	go.einride.tech/aip v0.67.1
	go.opencensus.io v0.24.0
	go.opentelemetry.io/otel v1.24.0
<<<<<<< HEAD
	go.opentelemetry.io/otel/sdk v1.24.0
	go.opentelemetry.io/otel/trace v1.24.0
	golang.org/x/oauth2 v0.21.0
	golang.org/x/sync v0.7.0
=======
	go.opentelemetry.io/otel/sdk v1.22.0
	go.opentelemetry.io/otel/trace v1.24.0
	golang.org/x/exp v0.0.0-20240314144324-c7f7c6466f7f
	golang.org/x/oauth2 v0.18.0
	golang.org/x/sync v0.6.0
>>>>>>> c3d5c9bbbb404b9c3c72d7dfa9efa9886b2c391c
	golang.org/x/time v0.5.0
	google.golang.org/api v0.184.0
	google.golang.org/genproto v0.0.0-20240604185151-ef581f913117
	google.golang.org/genproto/googleapis/api v0.0.0-20240610135401-a8a62080eff3
	google.golang.org/grpc v1.64.0
	google.golang.org/protobuf v1.34.2
)

require (
	cloud.google.com/go/auth v0.5.1 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.2 // indirect
	cloud.google.com/go/compute/metadata v0.3.0 // indirect
	cloud.google.com/go/longrunning v0.5.7 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/s2a-go v0.1.7 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.2 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.49.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.49.0 // indirect
	go.opentelemetry.io/otel/metric v1.24.0 // indirect
<<<<<<< HEAD
	golang.org/x/crypto v0.24.0 // indirect
	golang.org/x/net v0.26.0 // indirect
	golang.org/x/sys v0.21.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240610135401-a8a62080eff3 // indirect
=======
	golang.org/x/crypto v0.21.0 // indirect
	golang.org/x/net v0.22.0 // indirect
	golang.org/x/sys v0.18.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240311132316-a219d84964c2 // indirect
>>>>>>> c3d5c9bbbb404b9c3c72d7dfa9efa9886b2c391c
)

replace cloud.google.com/go => ../
