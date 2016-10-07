// Copyright 2016 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// AUTO-GENERATED CODE. DO NOT EDIT.

package pubsub

import (
	"fmt"
	"math"
	"runtime"
	"time"

	gax "github.com/googleapis/gax-go"
	"golang.org/x/net/context"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/api/transport"
	iampb "google.golang.org/genproto/googleapis/iam/v1"
	pubsubpb "google.golang.org/genproto/googleapis/pubsub/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

var (
	publisherProjectPathTemplate = gax.MustCompilePathTemplate("projects/{project}")
	publisherTopicPathTemplate   = gax.MustCompilePathTemplate("projects/{project}/topics/{topic}")
)

// PublisherCallOptions contains the retry settings for each method of PublisherClient.
type PublisherCallOptions struct {
	CreateTopic            []gax.CallOption
	Publish                []gax.CallOption
	GetTopic               []gax.CallOption
	ListTopics             []gax.CallOption
	ListTopicSubscriptions []gax.CallOption
	DeleteTopic            []gax.CallOption
	SetIamPolicy           []gax.CallOption
	GetIamPolicy           []gax.CallOption
	TestIamPermissions     []gax.CallOption
}

func defaultPublisherClientOptions() []option.ClientOption {
	return []option.ClientOption{
		option.WithEndpoint("pubsub.googleapis.com:443"),
		option.WithScopes(
			"https://www.googleapis.com/auth/cloud-platform",
			"https://www.googleapis.com/auth/pubsub",
		),
	}
}

func defaultPublisherCallOptions() *PublisherCallOptions {
	retry := map[[2]string][]gax.CallOption{
		{"default", "idempotent"}: {
			gax.WithRetry(func() gax.Retryer {
				return gax.OnCodes([]codes.Code{
					codes.DeadlineExceeded,
					codes.Unavailable,
				}, gax.Backoff{
					Initial:    100 * time.Millisecond,
					Max:        60000 * time.Millisecond,
					Multiplier: 1.3,
				})
			}),
		},
		{"messaging", "one_plus_delivery"}: {
			gax.WithRetry(func() gax.Retryer {
				return gax.OnCodes([]codes.Code{
					codes.DeadlineExceeded,
					codes.Unavailable,
				}, gax.Backoff{
					Initial:    100 * time.Millisecond,
					Max:        60000 * time.Millisecond,
					Multiplier: 1.3,
				})
			}),
		},
	}
	return &PublisherCallOptions{
		CreateTopic:            retry[[2]string{"default", "idempotent"}],
		Publish:                retry[[2]string{"messaging", "one_plus_delivery"}],
		GetTopic:               retry[[2]string{"default", "idempotent"}],
		ListTopics:             retry[[2]string{"default", "idempotent"}],
		ListTopicSubscriptions: retry[[2]string{"default", "idempotent"}],
		DeleteTopic:            retry[[2]string{"default", "idempotent"}],
		SetIamPolicy:           retry[[2]string{"default", "non_idempotent"}],
		GetIamPolicy:           retry[[2]string{"default", "idempotent"}],
		TestIamPermissions:     retry[[2]string{"default", "non_idempotent"}],
	}
}

// PublisherClient is a client for interacting with Google Cloud Pub/Sub API.
type PublisherClient struct {
	// The connection to the service.
	conn *grpc.ClientConn

	// The gRPC API client.
	iamPolicyClient iampb.IAMPolicyClient
	publisherClient pubsubpb.PublisherClient

	// The call options for this service.
	CallOptions *PublisherCallOptions

	// The metadata to be sent with each request.
	metadata map[string][]string
}

// NewPublisherClient creates a new publisher client.
//
// The service that an application uses to manipulate topics, and to send
// messages to a topic.
func NewPublisherClient(ctx context.Context, opts ...option.ClientOption) (*PublisherClient, error) {
	conn, err := transport.DialGRPC(ctx, append(defaultPublisherClientOptions(), opts...)...)
	if err != nil {
		return nil, err
	}
	c := &PublisherClient{
		conn:        conn,
		CallOptions: defaultPublisherCallOptions(),

		iamPolicyClient: iampb.NewIAMPolicyClient(conn),
		publisherClient: pubsubpb.NewPublisherClient(conn),
	}
	c.SetGoogleClientInfo("gax", gax.Version)
	return c, nil
}

// Connection returns the client's connection to the API service.
func (c *PublisherClient) Connection() *grpc.ClientConn {
	return c.conn
}

// Close closes the connection to the API service. The user should invoke this when
// the client is no longer required.
func (c *PublisherClient) Close() error {
	return c.conn.Close()
}

// SetGoogleClientInfo sets the name and version of the application in
// the `x-goog-api-client` header passed on each request. Intended for
// use by Google-written clients.
func (c *PublisherClient) SetGoogleClientInfo(name, version string) {
	c.metadata = map[string][]string{
		"x-goog-api-client": {fmt.Sprintf("%s/%s %s gax/%s go/%s", name, version, gapicNameVersion, gax.Version, runtime.Version())},
	}
}

// PublisherProjectPath returns the path for the project resource.
func PublisherProjectPath(project string) string {
	path, err := publisherProjectPathTemplate.Render(map[string]string{
		"project": project,
	})
	if err != nil {
		panic(err)
	}
	return path
}

// PublisherTopicPath returns the path for the topic resource.
func PublisherTopicPath(project, topic string) string {
	path, err := publisherTopicPathTemplate.Render(map[string]string{
		"project": project,
		"topic":   topic,
	})
	if err != nil {
		panic(err)
	}
	return path
}

// CreateTopic creates the given topic with the given name.
func (c *PublisherClient) CreateTopic(ctx context.Context, req *pubsubpb.Topic) (*pubsubpb.Topic, error) {
	md, _ := metadata.FromContext(ctx)
	ctx = metadata.NewContext(ctx, metadata.Join(md, c.metadata))
	var resp *pubsubpb.Topic
	err := gax.Invoke(ctx, func(ctx context.Context) error {
		var err error
		resp, err = c.publisherClient.CreateTopic(ctx, req)
		return err
	}, c.CallOptions.CreateTopic...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Publish adds one or more messages to the topic. Returns `NOT_FOUND` if the topic
// does not exist. The message payload must not be empty; it must contain
//  either a non-empty data field, or at least one attribute.
func (c *PublisherClient) Publish(ctx context.Context, req *pubsubpb.PublishRequest) (*pubsubpb.PublishResponse, error) {
	md, _ := metadata.FromContext(ctx)
	ctx = metadata.NewContext(ctx, metadata.Join(md, c.metadata))
	var resp *pubsubpb.PublishResponse
	err := gax.Invoke(ctx, func(ctx context.Context) error {
		var err error
		resp, err = c.publisherClient.Publish(ctx, req)
		return err
	}, c.CallOptions.Publish...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// GetTopic gets the configuration of a topic.
func (c *PublisherClient) GetTopic(ctx context.Context, req *pubsubpb.GetTopicRequest) (*pubsubpb.Topic, error) {
	md, _ := metadata.FromContext(ctx)
	ctx = metadata.NewContext(ctx, metadata.Join(md, c.metadata))
	var resp *pubsubpb.Topic
	err := gax.Invoke(ctx, func(ctx context.Context) error {
		var err error
		resp, err = c.publisherClient.GetTopic(ctx, req)
		return err
	}, c.CallOptions.GetTopic...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// ListTopics lists matching topics.
func (c *PublisherClient) ListTopics(ctx context.Context, req *pubsubpb.ListTopicsRequest) *TopicIterator {
	md, _ := metadata.FromContext(ctx)
	ctx = metadata.NewContext(ctx, metadata.Join(md, c.metadata))
	it := &TopicIterator{}

	fetch := func(pageSize int, pageToken string) (string, error) {
		var resp *pubsubpb.ListTopicsResponse
		req.PageToken = pageToken
		if pageSize > math.MaxInt32 {
			req.PageSize = math.MaxInt32
		} else {
			req.PageSize = int32(pageSize)
		}
		err := gax.Invoke(ctx, func(ctx context.Context) error {
			var err error
			resp, err = c.publisherClient.ListTopics(ctx, req)
			return err
		}, c.CallOptions.ListTopics...)
		if err != nil {
			return "", err
		}
		it.items = append(it.items, resp.Topics...)
		return resp.NextPageToken, nil
	}
	bufLen := func() int { return len(it.items) }
	takeBuf := func() interface{} {
		b := it.items
		it.items = nil
		return b
	}

	it.pageInfo, it.nextFunc = iterator.NewPageInfo(fetch, bufLen, takeBuf)
	return it
}

// ListTopicSubscriptions lists the name of the subscriptions for this topic.
func (c *PublisherClient) ListTopicSubscriptions(ctx context.Context, req *pubsubpb.ListTopicSubscriptionsRequest) *StringIterator {
	md, _ := metadata.FromContext(ctx)
	ctx = metadata.NewContext(ctx, metadata.Join(md, c.metadata))
	it := &StringIterator{}

	fetch := func(pageSize int, pageToken string) (string, error) {
		var resp *pubsubpb.ListTopicSubscriptionsResponse
		req.PageToken = pageToken
		if pageSize > math.MaxInt32 {
			req.PageSize = math.MaxInt32
		} else {
			req.PageSize = int32(pageSize)
		}
		err := gax.Invoke(ctx, func(ctx context.Context) error {
			var err error
			resp, err = c.publisherClient.ListTopicSubscriptions(ctx, req)
			return err
		}, c.CallOptions.ListTopicSubscriptions...)
		if err != nil {
			return "", err
		}
		it.items = append(it.items, resp.Subscriptions...)
		return resp.NextPageToken, nil
	}
	bufLen := func() int { return len(it.items) }
	takeBuf := func() interface{} {
		b := it.items
		it.items = nil
		return b
	}

	it.pageInfo, it.nextFunc = iterator.NewPageInfo(fetch, bufLen, takeBuf)
	return it
}

// DeleteTopic deletes the topic with the given name. Returns `NOT_FOUND` if the topic
// does not exist. After a topic is deleted, a new topic may be created with
// the same name; this is an entirely new topic with none of the old
// configuration or subscriptions. Existing subscriptions to this topic are
// not deleted, but their `topic` field is set to `_deleted-topic_`.
func (c *PublisherClient) DeleteTopic(ctx context.Context, req *pubsubpb.DeleteTopicRequest) error {
	md, _ := metadata.FromContext(ctx)
	ctx = metadata.NewContext(ctx, metadata.Join(md, c.metadata))
	err := gax.Invoke(ctx, func(ctx context.Context) error {
		var err error
		_, err = c.publisherClient.DeleteTopic(ctx, req)
		return err
	}, c.CallOptions.DeleteTopic...)
	return err
}

// SetIamPolicy sets the access control policy on the specified resource. Replaces any
// existing policy.
func (c *PublisherClient) SetIamPolicy(ctx context.Context, req *iampb.SetIamPolicyRequest) (*iampb.Policy, error) {
	md, _ := metadata.FromContext(ctx)
	ctx = metadata.NewContext(ctx, metadata.Join(md, c.metadata))
	var resp *iampb.Policy
	err := gax.Invoke(ctx, func(ctx context.Context) error {
		var err error
		resp, err = c.iamPolicyClient.SetIamPolicy(ctx, req)
		return err
	}, c.CallOptions.SetIamPolicy...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// GetIamPolicy gets the access control policy for a resource.
// Returns an empty policy if the resource exists and does not have a policy
// set.
func (c *PublisherClient) GetIamPolicy(ctx context.Context, req *iampb.GetIamPolicyRequest) (*iampb.Policy, error) {
	md, _ := metadata.FromContext(ctx)
	ctx = metadata.NewContext(ctx, metadata.Join(md, c.metadata))
	var resp *iampb.Policy
	err := gax.Invoke(ctx, func(ctx context.Context) error {
		var err error
		resp, err = c.iamPolicyClient.GetIamPolicy(ctx, req)
		return err
	}, c.CallOptions.GetIamPolicy...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// TestIamPermissions returns permissions that a caller has on the specified resource.
func (c *PublisherClient) TestIamPermissions(ctx context.Context, req *iampb.TestIamPermissionsRequest) (*iampb.TestIamPermissionsResponse, error) {
	md, _ := metadata.FromContext(ctx)
	ctx = metadata.NewContext(ctx, metadata.Join(md, c.metadata))
	var resp *iampb.TestIamPermissionsResponse
	err := gax.Invoke(ctx, func(ctx context.Context) error {
		var err error
		resp, err = c.iamPolicyClient.TestIamPermissions(ctx, req)
		return err
	}, c.CallOptions.TestIamPermissions...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// StringIterator manages a stream of string.
type StringIterator struct {
	items    []string
	pageInfo *iterator.PageInfo
	nextFunc func() error
}

// PageInfo supports pagination. See the google.golang.org/api/iterator package for details.
func (it *StringIterator) PageInfo() *iterator.PageInfo {
	return it.pageInfo
}

// Next returns the next result. Its second return value is iterator.Done if there are no more
// results. Once Next returns Done, all subsequent calls will return Done.
func (it *StringIterator) Next() (string, error) {
	if err := it.nextFunc(); err != nil {
		return "", err
	}
	item := it.items[0]
	it.items = it.items[1:]
	return item, nil
}

// TopicIterator manages a stream of *pubsubpb.Topic.
type TopicIterator struct {
	items    []*pubsubpb.Topic
	pageInfo *iterator.PageInfo
	nextFunc func() error
}

// PageInfo supports pagination. See the google.golang.org/api/iterator package for details.
func (it *TopicIterator) PageInfo() *iterator.PageInfo {
	return it.pageInfo
}

// Next returns the next result. Its second return value is iterator.Done if there are no more
// results. Once Next returns Done, all subsequent calls will return Done.
func (it *TopicIterator) Next() (*pubsubpb.Topic, error) {
	if err := it.nextFunc(); err != nil {
		return nil, err
	}
	item := it.items[0]
	it.items = it.items[1:]
	return item, nil
}
