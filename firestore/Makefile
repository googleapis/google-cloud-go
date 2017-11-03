# This makefile builds the cross-language tests for Firestore.
# It outputs each test as a separate .textproto file into the
# testdata directory of TESTS_REPO.

# Assume protoc is on the path. The proto compiler must be one that
# supports proto3 syntax.
PROTOC = protoc

# Assume the Go plugin has been downloaded and installed.
PROTOC_GO_PLUGIN_DIR = $(GOPATH)/bin

# Dependent repos.
PROTOBUF_REPO = $(HOME)/git-repos/protobuf
GOOGLEAPIS_REPO = $(HOME)/git-repos/googleapis

# TODO(jba): change the location of TESTS_REPO.
TESTS_REPO = $(HOME)/git-repos/jba/firestore-client-tests

.PHONY: generate-tests sync-protos gen-protos generator

generate-tests: sync-protos gen-protos generator
	$(GOPATH)/bin/generate-firestore-tests -o $(TESTS_REPO)/testdata

sync-protos:
	cd $(PROTOBUF_REPO); git pull
	cd $(GOOGLEAPIS_REPO); git pull

gen-protos: sync-protos
	mkdir -p genproto
	PATH=$(PATH):$(PROTOC_GO_PLUGIN_DIR) \
		$(PROTOC) --go_out=plugins=grpc:genproto \
		-I $(TESTS_REPO)/proto -I $(PROTOBUF_REPO)/src -I $(GOOGLEAPIS_REPO) \
		$(TESTS_REPO)/proto/*.proto

generator:
	go install ./cmd/generate-firestore-tests

