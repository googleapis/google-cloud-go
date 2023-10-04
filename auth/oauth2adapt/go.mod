module cloud.google.com/go/auth/oauth2adapt

go 1.19

require (
	cloud.google.com/go/auth v0.0.0
	github.com/google/go-cmp v0.5.9
	golang.org/x/oauth2 v0.11.0
)

require (
	github.com/golang/protobuf v1.5.3 // indirect
	golang.org/x/net v0.14.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
)

// TODO(codyoss): remove this once we have a real release.
replace cloud.google.com/go/auth => ../
