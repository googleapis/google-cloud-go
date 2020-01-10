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

package testutil

import (
	"context"

	"github.com/golang/protobuf/proto"
	instancepb "google.golang.org/genproto/googleapis/spanner/admin/instance/v1"
)

// InMemInstanceAdminServer contains the InstanceAdminServer interface plus a couple
// of specific methods for setting mocked results.
type InMemInstanceAdminServer interface {
	instancepb.InstanceAdminServer
	Stop()
	Resps() []proto.Message
	SetResps([]proto.Message)
	Reqs() []proto.Message
	SetReqs([]proto.Message)
	SetErr(error)
}

// inMemInstanceAdminServer implements InMemInstanceAdminServer interface. Note that
// there is no mutex protecting the data structures, so it is not safe for
// concurrent use.
type inMemInstanceAdminServer struct {
	instancepb.InstanceAdminServer
	reqs []proto.Message
	// If set, all calls return this error
	err error
	// responses to return if err == nil
	resps []proto.Message
}

// NewInMemInstanceAdminServer creates a new in-mem test server.
func NewInMemInstanceAdminServer() InMemInstanceAdminServer {
	res := &inMemInstanceAdminServer{}
	return res
}

// GetInstance returns the metadata of a spanner instance.
func (s *inMemInstanceAdminServer) GetInstance(ctx context.Context, req *instancepb.GetInstanceRequest) (*instancepb.Instance, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		defer func() { s.err = nil }()
		return nil, s.err
	}
	return s.resps[0].(*instancepb.Instance), nil
}

func (s *inMemInstanceAdminServer) Stop() {
	// do nothing
}

func (s *inMemInstanceAdminServer) Resps() []proto.Message {
	return s.resps
}

func (s *inMemInstanceAdminServer) SetResps(resps []proto.Message) {
	s.resps = resps
}

func (s *inMemInstanceAdminServer) Reqs() []proto.Message {
	return s.reqs
}

func (s *inMemInstanceAdminServer) SetReqs(reqs []proto.Message) {
	s.reqs = reqs
}

func (s *inMemInstanceAdminServer) SetErr(err error) {
	s.err = err
}
