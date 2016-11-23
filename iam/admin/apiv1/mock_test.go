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

package admin

import (
	google_protobuf "github.com/golang/protobuf/ptypes/empty"
	adminpb "google.golang.org/genproto/googleapis/iam/admin/v1"
	iampb "google.golang.org/genproto/googleapis/iam/v1"
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

type mockIamServer struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

func (s *mockIamServer) ListServiceAccounts(_ context.Context, req *adminpb.ListServiceAccountsRequest) (*adminpb.ListServiceAccountsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*adminpb.ListServiceAccountsResponse), nil
}

func (s *mockIamServer) GetServiceAccount(_ context.Context, req *adminpb.GetServiceAccountRequest) (*adminpb.ServiceAccount, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*adminpb.ServiceAccount), nil
}

func (s *mockIamServer) CreateServiceAccount(_ context.Context, req *adminpb.CreateServiceAccountRequest) (*adminpb.ServiceAccount, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*adminpb.ServiceAccount), nil
}

func (s *mockIamServer) UpdateServiceAccount(_ context.Context, req *adminpb.ServiceAccount) (*adminpb.ServiceAccount, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*adminpb.ServiceAccount), nil
}

func (s *mockIamServer) DeleteServiceAccount(_ context.Context, req *adminpb.DeleteServiceAccountRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

func (s *mockIamServer) ListServiceAccountKeys(_ context.Context, req *adminpb.ListServiceAccountKeysRequest) (*adminpb.ListServiceAccountKeysResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*adminpb.ListServiceAccountKeysResponse), nil
}

func (s *mockIamServer) GetServiceAccountKey(_ context.Context, req *adminpb.GetServiceAccountKeyRequest) (*adminpb.ServiceAccountKey, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*adminpb.ServiceAccountKey), nil
}

func (s *mockIamServer) CreateServiceAccountKey(_ context.Context, req *adminpb.CreateServiceAccountKeyRequest) (*adminpb.ServiceAccountKey, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*adminpb.ServiceAccountKey), nil
}

func (s *mockIamServer) DeleteServiceAccountKey(_ context.Context, req *adminpb.DeleteServiceAccountKeyRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

func (s *mockIamServer) SignBlob(_ context.Context, req *adminpb.SignBlobRequest) (*adminpb.SignBlobResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*adminpb.SignBlobResponse), nil
}

func (s *mockIamServer) GetIamPolicy(_ context.Context, req *iampb.GetIamPolicyRequest) (*iampb.Policy, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*iampb.Policy), nil
}

func (s *mockIamServer) SetIamPolicy(_ context.Context, req *iampb.SetIamPolicyRequest) (*iampb.Policy, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*iampb.Policy), nil
}

func (s *mockIamServer) TestIamPermissions(_ context.Context, req *iampb.TestIamPermissionsRequest) (*iampb.TestIamPermissionsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*iampb.TestIamPermissionsResponse), nil
}

func (s *mockIamServer) QueryGrantableRoles(_ context.Context, req *adminpb.QueryGrantableRolesRequest) (*adminpb.QueryGrantableRolesResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*adminpb.QueryGrantableRolesResponse), nil
}

// clientOpt is the option tests should use to connect to the test server.
// It is initialized by TestMain.
var clientOpt option.ClientOption

var (
	mockIam mockIamServer
)

func TestMain(m *testing.M) {
	flag.Parse()

	serv := grpc.NewServer()
	adminpb.RegisterIAMServer(serv, &mockIam)

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

func TestIamListServiceAccountsError(t *testing.T) {
	errCode := codes.Internal
	mockIam.err = grpc.Errorf(errCode, "test error")

	c, err := NewIamClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *adminpb.ListServiceAccountsRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.ListServiceAccounts(context.Background(), req).Next()

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestIamGetServiceAccountError(t *testing.T) {
	errCode := codes.Internal
	mockIam.err = grpc.Errorf(errCode, "test error")

	c, err := NewIamClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *adminpb.GetServiceAccountRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.GetServiceAccount(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestIamCreateServiceAccountError(t *testing.T) {
	errCode := codes.Internal
	mockIam.err = grpc.Errorf(errCode, "test error")

	c, err := NewIamClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *adminpb.CreateServiceAccountRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.CreateServiceAccount(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestIamUpdateServiceAccountError(t *testing.T) {
	errCode := codes.Internal
	mockIam.err = grpc.Errorf(errCode, "test error")

	c, err := NewIamClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *adminpb.ServiceAccount

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.UpdateServiceAccount(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestIamDeleteServiceAccountError(t *testing.T) {
	errCode := codes.Internal
	mockIam.err = grpc.Errorf(errCode, "test error")

	c, err := NewIamClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *adminpb.DeleteServiceAccountRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	err = c.DeleteServiceAccount(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestIamListServiceAccountKeysError(t *testing.T) {
	errCode := codes.Internal
	mockIam.err = grpc.Errorf(errCode, "test error")

	c, err := NewIamClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *adminpb.ListServiceAccountKeysRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.ListServiceAccountKeys(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestIamGetServiceAccountKeyError(t *testing.T) {
	errCode := codes.Internal
	mockIam.err = grpc.Errorf(errCode, "test error")

	c, err := NewIamClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *adminpb.GetServiceAccountKeyRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.GetServiceAccountKey(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestIamCreateServiceAccountKeyError(t *testing.T) {
	errCode := codes.Internal
	mockIam.err = grpc.Errorf(errCode, "test error")

	c, err := NewIamClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *adminpb.CreateServiceAccountKeyRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.CreateServiceAccountKey(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestIamDeleteServiceAccountKeyError(t *testing.T) {
	errCode := codes.Internal
	mockIam.err = grpc.Errorf(errCode, "test error")

	c, err := NewIamClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *adminpb.DeleteServiceAccountKeyRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	err = c.DeleteServiceAccountKey(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestIamSignBlobError(t *testing.T) {
	errCode := codes.Internal
	mockIam.err = grpc.Errorf(errCode, "test error")

	c, err := NewIamClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *adminpb.SignBlobRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.SignBlob(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestIamGetIamPolicyError(t *testing.T) {
	errCode := codes.Internal
	mockIam.err = grpc.Errorf(errCode, "test error")

	c, err := NewIamClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *iampb.GetIamPolicyRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.getIamPolicy(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestIamSetIamPolicyError(t *testing.T) {
	errCode := codes.Internal
	mockIam.err = grpc.Errorf(errCode, "test error")

	c, err := NewIamClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *iampb.SetIamPolicyRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.setIamPolicy(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestIamTestIamPermissionsError(t *testing.T) {
	errCode := codes.Internal
	mockIam.err = grpc.Errorf(errCode, "test error")

	c, err := NewIamClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *iampb.TestIamPermissionsRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.TestIamPermissions(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestIamQueryGrantableRolesError(t *testing.T) {
	errCode := codes.Internal
	mockIam.err = grpc.Errorf(errCode, "test error")

	c, err := NewIamClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *adminpb.QueryGrantableRolesRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.QueryGrantableRoles(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
