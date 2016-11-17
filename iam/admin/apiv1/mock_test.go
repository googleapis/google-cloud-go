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
	"io"

	"golang.org/x/net/context"
)

var _ = io.EOF

type mockIAM struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

var _ adminpb.IAMServer = &mockIAM{}

func (s *mockIAM) ListServiceAccounts(_ context.Context, req *adminpb.ListServiceAccountsRequest) (*adminpb.ListServiceAccountsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*adminpb.ListServiceAccountsResponse), nil
}

func (s *mockIAM) GetServiceAccount(_ context.Context, req *adminpb.GetServiceAccountRequest) (*adminpb.ServiceAccount, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*adminpb.ServiceAccount), nil
}

func (s *mockIAM) CreateServiceAccount(_ context.Context, req *adminpb.CreateServiceAccountRequest) (*adminpb.ServiceAccount, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*adminpb.ServiceAccount), nil
}

func (s *mockIAM) UpdateServiceAccount(_ context.Context, req *adminpb.ServiceAccount) (*adminpb.ServiceAccount, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*adminpb.ServiceAccount), nil
}

func (s *mockIAM) DeleteServiceAccount(_ context.Context, req *adminpb.DeleteServiceAccountRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

func (s *mockIAM) ListServiceAccountKeys(_ context.Context, req *adminpb.ListServiceAccountKeysRequest) (*adminpb.ListServiceAccountKeysResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*adminpb.ListServiceAccountKeysResponse), nil
}

func (s *mockIAM) GetServiceAccountKey(_ context.Context, req *adminpb.GetServiceAccountKeyRequest) (*adminpb.ServiceAccountKey, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*adminpb.ServiceAccountKey), nil
}

func (s *mockIAM) CreateServiceAccountKey(_ context.Context, req *adminpb.CreateServiceAccountKeyRequest) (*adminpb.ServiceAccountKey, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*adminpb.ServiceAccountKey), nil
}

func (s *mockIAM) DeleteServiceAccountKey(_ context.Context, req *adminpb.DeleteServiceAccountKeyRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

func (s *mockIAM) SignBlob(_ context.Context, req *adminpb.SignBlobRequest) (*adminpb.SignBlobResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*adminpb.SignBlobResponse), nil
}

func (s *mockIAM) GetIamPolicy(_ context.Context, req *iampb.GetIamPolicyRequest) (*iampb.Policy, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*iampb.Policy), nil
}

func (s *mockIAM) SetIamPolicy(_ context.Context, req *iampb.SetIamPolicyRequest) (*iampb.Policy, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*iampb.Policy), nil
}

func (s *mockIAM) TestIamPermissions(_ context.Context, req *iampb.TestIamPermissionsRequest) (*iampb.TestIamPermissionsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*iampb.TestIamPermissionsResponse), nil
}

func (s *mockIAM) QueryGrantableRoles(_ context.Context, req *adminpb.QueryGrantableRolesRequest) (*adminpb.QueryGrantableRolesResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*adminpb.QueryGrantableRolesResponse), nil
}
