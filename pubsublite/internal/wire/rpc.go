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

package wire

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	vkit "cloud.google.com/go/pubsublite/apiv1"
	gax "github.com/googleapis/gax-go/v2"
)

// streamRetryer implements the retry policy for establishing gRPC stream
// connections.
type streamRetryer struct {
	bo       gax.Backoff
	deadline time.Time
}

func newStreamRetryer(timeout time.Duration) *streamRetryer {
	return &streamRetryer{
		bo: gax.Backoff{
			Initial:    10 * time.Millisecond,
			Max:        10 * time.Second,
			Multiplier: 2,
		},
		deadline: time.Now().Add(timeout),
	}
}

func (r *streamRetryer) RetrySend(err error) (time.Duration, bool) {
	if time.Now().After(r.deadline) {
		return 0, false
	}
	if isRetryableSendError(err) {
		return r.bo.Pause(), true
	}
	return 0, false
}

func (r *streamRetryer) RetryRecv(err error) (time.Duration, bool) {
	if time.Now().After(r.deadline) {
		return 0, false
	}
	if isRetryableRecvError(err) {
		return r.bo.Pause(), true
	}
	return 0, false
}

func isRetryableSendCode(code codes.Code) bool {
	switch code {
	// Client-side errors that occur during grpc.ClientStream.SendMsg() have a
	// smaller set of retryable codes.
	case codes.DeadlineExceeded, codes.Unavailable:
		return true
	default:
		return false
	}
}

func isRetryableRecvCode(code codes.Code) bool {
	switch code {
	// Consistent with https://github.com/googleapis/java-pubsublite/blob/master/google-cloud-pubsublite/src/main/java/com/google/cloud/pubsublite/ErrorCodes.java
	case codes.Aborted, codes.DeadlineExceeded, codes.Internal, codes.ResourceExhausted, codes.Unavailable, codes.Unknown:
		return true
	default:
		return false
	}
}

func isRetryableSendError(err error) bool {
	return isRetryableStreamError(err, isRetryableSendCode)
}

func isRetryableRecvError(err error) bool {
	return isRetryableStreamError(err, isRetryableRecvCode)
}

func isRetryableStreamError(err error, isEligible func(codes.Code) bool) bool {
	s, ok := status.FromError(err)
	if !ok {
		// Includes io.EOF, normal stream close.
		// Consistent with https://github.com/googleapis/google-cloud-go/blob/master/pubsub/service.go
		return true
	}
	return isEligible(s.Code())
}

// retryableReadOnlyCallOption returns a call option that retries with backoff
// for ResourceExhausted in addition to other default retryable codes for
// Pub/Sub. Suitable for read-only operations which are subject to only QPS
// quota limits.
func retryableReadOnlyCallOption() gax.CallOption {
	return gax.WithRetry(func() gax.Retryer {
		return gax.OnCodes([]codes.Code{
			codes.Aborted,
			codes.DeadlineExceeded,
			codes.Internal,
			codes.ResourceExhausted,
			codes.Unavailable,
			codes.Unknown,
		}, gax.Backoff{
			Initial:    100 * time.Millisecond,
			Max:        60 * time.Second,
			Multiplier: 1.3,
		})
	})
}

const pubsubLiteDefaultEndpoint = "-pubsublite.googleapis.com:443"

func defaultClientOptions(region string) []option.ClientOption {
	return []option.ClientOption{
		internaloption.WithDefaultEndpoint(region + pubsubLiteDefaultEndpoint),
	}
}

// NewAdminClient creates a new gapic AdminClient for a region.
func NewAdminClient(ctx context.Context, region string, opts ...option.ClientOption) (*vkit.AdminClient, error) {
	options := append(defaultClientOptions(region), opts...)
	return vkit.NewAdminClient(ctx, options...)
}

func newPublisherClient(ctx context.Context, region string, opts ...option.ClientOption) (*vkit.PublisherClient, error) {
	options := append(defaultClientOptions(region), opts...)
	return vkit.NewPublisherClient(ctx, options...)
}

func newSubscriberClient(ctx context.Context, region string, opts ...option.ClientOption) (*vkit.SubscriberClient, error) {
	options := append(defaultClientOptions(region), opts...)
	return vkit.NewSubscriberClient(ctx, options...)
}

func newCursorClient(ctx context.Context, region string, opts ...option.ClientOption) (*vkit.CursorClient, error) {
	options := append(defaultClientOptions(region), opts...)
	return vkit.NewCursorClient(ctx, options...)
}

func newPartitionAssignmentClient(ctx context.Context, region string, opts ...option.ClientOption) (*vkit.PartitionAssignmentClient, error) {
	options := append(defaultClientOptions(region), opts...)
	return vkit.NewPartitionAssignmentClient(ctx, options...)
}

const (
	routingMetadataHeader    = "x-goog-request-params"
	clientInfoMetadataHeader = "x-goog-pubsub-context"

	languageKey     = "language"
	languageValue   = "GOLANG"
	frameworkKey    = "framework"
	majorVersionKey = "major_version"
	minorVersionKey = "minor_version"
)

func stringValue(str string) *structpb.Value {
	return &structpb.Value{
		Kind: &structpb.Value_StringValue{StringValue: str},
	}
}

// pubsubMetadata stores key/value pairs that should be added to gRPC metadata.
type pubsubMetadata map[string]string

func newPubsubMetadata() pubsubMetadata {
	return make(map[string]string)
}

func (pm pubsubMetadata) AddTopicRoutingMetadata(topic topicPartition) {
	pm[routingMetadataHeader] = fmt.Sprintf("partition=%d&topic=%s", topic.Partition, url.QueryEscape(topic.Path))
}

func (pm pubsubMetadata) AddSubscriptionRoutingMetadata(subscription subscriptionPartition) {
	pm[routingMetadataHeader] = fmt.Sprintf("partition=%d&subscription=%s", subscription.Partition, url.QueryEscape(subscription.Path))
}

func (pm pubsubMetadata) AddClientInfo(framework FrameworkType) {
	pm.doAddClientInfo(framework, libraryVersion)
}

func (pm pubsubMetadata) doAddClientInfo(framework FrameworkType, getVersion func() (version, bool)) {
	s := &structpb.Struct{
		Fields: make(map[string]*structpb.Value),
	}
	s.Fields[languageKey] = stringValue(languageValue)
	if len(framework) > 0 {
		s.Fields[frameworkKey] = stringValue(string(framework))
	}
	if version, ok := getVersion(); ok {
		s.Fields[majorVersionKey] = stringValue(version.Major)
		s.Fields[minorVersionKey] = stringValue(version.Minor)
	}
	if bytes, err := proto.Marshal(s); err == nil {
		pm[clientInfoMetadataHeader] = base64.StdEncoding.EncodeToString(bytes)
	}
}

func (pm pubsubMetadata) AddToContext(ctx context.Context) context.Context {
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	for key, val := range pm {
		md[key] = append(md[key], val)
	}
	return metadata.NewOutgoingContext(ctx, md)
}
