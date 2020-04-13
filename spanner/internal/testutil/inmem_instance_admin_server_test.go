// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package testutil_test

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"testing"

	instance "cloud.google.com/go/spanner/admin/instance/apiv1"
	"cloud.google.com/go/spanner/internal/testutil"
	"github.com/golang/protobuf/proto"
	"google.golang.org/api/option"
	instancepb "google.golang.org/genproto/googleapis/spanner/admin/instance/v1"
	"google.golang.org/grpc"
)

var instanceClientOpt option.ClientOption

var (
	mockInstanceAdmin = testutil.NewInMemInstanceAdminServer()
)

func setupInstanceAdminServer() {
	flag.Parse()

	serv := grpc.NewServer()
	instancepb.RegisterInstanceAdminServer(serv, mockInstanceAdmin)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatal(err)
	}
	go serv.Serve(lis)

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	instanceClientOpt = option.WithGRPCConn(conn)
}

func TestInstanceAdminGetInstance(t *testing.T) {
	setupInstanceAdminServer()
	var expectedResponse = &instancepb.Instance{
		Name:        "name2-1052831874",
		Config:      "config-1354792126",
		DisplayName: "displayName1615086568",
		NodeCount:   1539922066,
	}

	mockInstanceAdmin.SetErr(nil)
	mockInstanceAdmin.SetReqs(nil)

	mockInstanceAdmin.SetResps(append(mockInstanceAdmin.Resps()[:0], expectedResponse))

	var formattedName string = fmt.Sprintf("projects/%s/instances/%s", "[PROJECT]", "[INSTANCE]")
	var request = &instancepb.GetInstanceRequest{
		Name: formattedName,
	}

	c, err := instance.NewInstanceAdminClient(context.Background(), instanceClientOpt)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := c.GetInstance(context.Background(), request)

	if err != nil {
		t.Fatal(err)
	}

	if want, got := request, mockInstanceAdmin.Reqs()[0]; !proto.Equal(want, got) {
		t.Errorf("wrong request %q, want %q", got, want)
	}

	if want, got := expectedResponse, resp; !proto.Equal(want, got) {
		t.Errorf("wrong response %q, want %q)", got, want)
	}
}
