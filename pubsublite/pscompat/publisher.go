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

package pscompat

import (
	"context"
	"sync"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsublite/internal/wire"
	"google.golang.org/api/option"

	ipubsub "cloud.google.com/go/internal/pubsub"
	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

var (
	// ErrOverflow is set for a PublishResult when publish buffers overflow. This
	// can occur when backends are unavailable or the actual publish throughput
	// of clients exceeds the allocated publish throughput for the Pub/Sub Lite
	// topic. Use errors.Is for comparing errors.
	ErrOverflow = wire.ErrOverflow

	// ErrOversizedMessage is set for a PublishResult when a published message
	// exceeds MaxPublishRequestBytes. Publishing this message will never succeed.
	// Use errors.Is for comparing errors.
	ErrOversizedMessage = wire.ErrOversizedMessage

	// ErrPublisherStopped is set for a PublishResult when a message cannot be
	// published because the publisher client has stopped or is in the process of
	// stopping. It may be stopping due to a fatal error. PublisherClient.Error()
	// returns the error that caused the publisher client to terminate (if any).
	// Use errors.Is for comparing errors.
	ErrPublisherStopped = wire.ErrServiceStopped

	// ErrBackendUnavailable indicates that the backend service has been
	// unavailable for a period of time. The timeout can be configured using
	// PublishSettings.Timeout or ReceiveSettings.Timeout. Use errors.Is for
	// comparing errors.
	ErrBackendUnavailable = wire.ErrBackendUnavailable
)

// PublisherClient is a Pub/Sub Lite client to publish messages to a given
// topic. A PublisherClient is safe to use from multiple goroutines.
//
// See https://cloud.google.com/pubsub/lite/docs/publishing for more information
// about publishing.
type PublisherClient struct {
	settings PublishSettings
	wirePub  wire.Publisher

	// Fields below must be guarded with mutex.
	mu  sync.Mutex
	err error
}

// NewPublisherClient creates a new Pub/Sub Lite publisher client to publish
// messages to a given topic, using DefaultPublishSettings. A valid topic path
// has the format: "projects/PROJECT_ID/locations/LOCATION/topics/TOPIC_ID".
func NewPublisherClient(ctx context.Context, topic string, opts ...option.ClientOption) (*PublisherClient, error) {
	return NewPublisherClientWithSettings(ctx, topic, DefaultPublishSettings, opts...)
}

// NewPublisherClientWithSettings creates a new Pub/Sub Lite publisher client to
// publish messages to a given topic, using the specified PublishSettings. A
// valid topic path has the format:
// "projects/PROJECT_ID/locations/LOCATION/topics/TOPIC_ID".
func NewPublisherClientWithSettings(ctx context.Context, topic string, settings PublishSettings, opts ...option.ClientOption) (*PublisherClient, error) {
	topicPath, err := wire.ParseTopicPath(topic)
	if err != nil {
		return nil, err
	}
	region, err := wire.LocationToRegion(topicPath.Location)
	if err != nil {
		return nil, err
	}

	wirePub, err := wire.NewPublisher(ctx, settings.toWireSettings(), region, topic, opts...)
	if err != nil {
		return nil, err
	}
	wirePub.Start()
	if err := wirePub.WaitStarted(); err != nil {
		return nil, err
	}
	return &PublisherClient{settings: settings, wirePub: wirePub}, nil
}

// Publish publishes `msg` to the topic asynchronously. Messages are batched and
// sent according to the client's PublishSettings. Publish never blocks.
//
// Publish returns a non-nil PublishResult which will be ready when the
// message has been sent (or has failed to be sent) to the server. Retryable
// errors are automatically handled. If a PublishResult returns an error, this
// indicates that the publisher client encountered a fatal error and can no
// longer be used. Fatal errors should be manually inspected and the cause
// resolved. A new publisher client instance must be created to republish failed
// messages.
//
// Once Stop() has been called or the publisher client has failed permanently
// due to an error, future calls to Publish will immediately return a
// PublishResult with error ErrPublisherStopped.
//
// Error() returns the error that caused the publisher client to terminate and
// may contain more context than the error returned by PublishResult.
func (p *PublisherClient) Publish(ctx context.Context, msg *pubsub.Message) *pubsub.PublishResult {
	result := ipubsub.NewPublishResult()
	msgpb := new(pb.PubSubMessage)
	if err := p.transformMessage(msg, msgpb); err != nil {
		ipubsub.SetPublishResult(result, "", err)
		p.setError(err)
		p.wirePub.Stop()
		return result
	}

	p.wirePub.Publish(msgpb, func(metadata *wire.MessageMetadata, err error) {
		if metadata != nil {
			ipubsub.SetPublishResult(result, metadata.String(), err)
		} else {
			ipubsub.SetPublishResult(result, "", err)
		}
	})
	return result
}

// Stop sends all remaining published messages and closes publish streams.
// Returns once all outstanding messages have been sent or have failed to be
// sent. Stop should be called when the client is no longer required.
func (p *PublisherClient) Stop() {
	p.wirePub.Stop()
	p.wirePub.WaitStopped()
}

// Error returns the error that caused the publisher client to terminate. The
// error returned here may contain more context than PublishResult errors. The
// return value may be nil if Stop() was called.
func (p *PublisherClient) Error() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.err != nil {
		return p.err
	}
	return p.wirePub.Error()
}

func (p *PublisherClient) setError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Don't clobber original error.
	if p.err == nil {
		p.err = err
	}
}

func (p *PublisherClient) transformMessage(from *pubsub.Message, to *pb.PubSubMessage) error {
	if p.settings.MessageTransformer != nil {
		return p.settings.MessageTransformer(from, to)
	}

	keyExtractor := p.settings.KeyExtractor
	if keyExtractor == nil {
		keyExtractor = extractOrderingKey
	}
	return transformPublishedMessage(from, to, keyExtractor)
}
