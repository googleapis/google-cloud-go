// Copyright 2016, Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// AUTO-GENERATED CODE. DO NOT EDIT.

package vision

import (
	visionpb "google.golang.org/genproto/googleapis/cloud/vision/v1"
)

import (
	"flag"
	"io"
	"log"
	"net"
	"os"
	"reflect"
	"testing"

	"golang.org/x/net/context"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var _ = io.EOF

type mockImageAnnotatorServer struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

func (s *mockImageAnnotatorServer) BatchAnnotateImages(_ context.Context, req *visionpb.BatchAnnotateImagesRequest) (*visionpb.BatchAnnotateImagesResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*visionpb.BatchAnnotateImagesResponse), nil
}

// clientOpt is the option tests should use to connect to the test server.
// It is initialized by TestMain.
var clientOpt option.ClientOption

var (
	mockImageAnnotator mockImageAnnotatorServer
)

func TestMain(m *testing.M) {
	flag.Parse()

	serv := grpc.NewServer()
	visionpb.RegisterImageAnnotatorServer(serv, &mockImageAnnotator)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatal(err)
	}
	go serv.Serve(lis)

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	clientOpt = option.WithGRPCConn(conn)

	os.Exit(m.Run())
}

func TestImageAnnotatorBatchAnnotateImagesError(t *testing.T) {
	errCode := codes.Internal
	mockImageAnnotator.err = grpc.Errorf(errCode, "test error")

	c, err := NewImageAnnotatorClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *visionpb.BatchAnnotateImagesRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.BatchAnnotateImages(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
