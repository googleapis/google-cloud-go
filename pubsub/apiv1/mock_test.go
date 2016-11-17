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

package pubsub

import (
	google_protobuf "github.com/golang/protobuf/ptypes/empty"
	iampb "google.golang.org/genproto/googleapis/iam/v1"
	pubsubpb "google.golang.org/genproto/googleapis/pubsub/v1"
)

import (
	"io"

	"golang.org/x/net/context"
)

var _ = io.EOF

type mockPublisher struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

var _ pubsubpb.PublisherServer = &mockPublisher{}

func (s *mockPublisher) CreateTopic(_ context.Context, req *pubsubpb.Topic) (*pubsubpb.Topic, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*pubsubpb.Topic), nil
}

func (s *mockPublisher) Publish(_ context.Context, req *pubsubpb.PublishRequest) (*pubsubpb.PublishResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*pubsubpb.PublishResponse), nil
}

func (s *mockPublisher) GetTopic(_ context.Context, req *pubsubpb.GetTopicRequest) (*pubsubpb.Topic, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*pubsubpb.Topic), nil
}

func (s *mockPublisher) ListTopics(_ context.Context, req *pubsubpb.ListTopicsRequest) (*pubsubpb.ListTopicsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*pubsubpb.ListTopicsResponse), nil
}

func (s *mockPublisher) ListTopicSubscriptions(_ context.Context, req *pubsubpb.ListTopicSubscriptionsRequest) (*pubsubpb.ListTopicSubscriptionsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*pubsubpb.ListTopicSubscriptionsResponse), nil
}

func (s *mockPublisher) DeleteTopic(_ context.Context, req *pubsubpb.DeleteTopicRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

type mockIAMPolicy struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

var _ iampb.IAMPolicyServer = &mockIAMPolicy{}

func (s *mockIAMPolicy) SetIamPolicy(_ context.Context, req *iampb.SetIamPolicyRequest) (*iampb.Policy, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*iampb.Policy), nil
}

func (s *mockIAMPolicy) GetIamPolicy(_ context.Context, req *iampb.GetIamPolicyRequest) (*iampb.Policy, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*iampb.Policy), nil
}

func (s *mockIAMPolicy) TestIamPermissions(_ context.Context, req *iampb.TestIamPermissionsRequest) (*iampb.TestIamPermissionsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*iampb.TestIamPermissionsResponse), nil
}

type mockSubscriber struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

var _ pubsubpb.SubscriberServer = &mockSubscriber{}

func (s *mockSubscriber) CreateSubscription(_ context.Context, req *pubsubpb.Subscription) (*pubsubpb.Subscription, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*pubsubpb.Subscription), nil
}

func (s *mockSubscriber) GetSubscription(_ context.Context, req *pubsubpb.GetSubscriptionRequest) (*pubsubpb.Subscription, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*pubsubpb.Subscription), nil
}

func (s *mockSubscriber) ListSubscriptions(_ context.Context, req *pubsubpb.ListSubscriptionsRequest) (*pubsubpb.ListSubscriptionsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*pubsubpb.ListSubscriptionsResponse), nil
}

func (s *mockSubscriber) DeleteSubscription(_ context.Context, req *pubsubpb.DeleteSubscriptionRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

func (s *mockSubscriber) ModifyAckDeadline(_ context.Context, req *pubsubpb.ModifyAckDeadlineRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

func (s *mockSubscriber) Acknowledge(_ context.Context, req *pubsubpb.AcknowledgeRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

func (s *mockSubscriber) Pull(_ context.Context, req *pubsubpb.PullRequest) (*pubsubpb.PullResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*pubsubpb.PullResponse), nil
}

func (s *mockSubscriber) ModifyPushConfig(_ context.Context, req *pubsubpb.ModifyPushConfigRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}
