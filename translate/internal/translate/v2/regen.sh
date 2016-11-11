#!/bin/bash -e


(cd $GOPATH/src/google.golang.org/api; make generator)

$GOPATH/bin/google-api-go-generator \
	-api_json_file translate-nov2016-api.json \
	-api_pkg_base cloud.google.com/go/translate/internal \
	-output translate-nov2016-gen.go
