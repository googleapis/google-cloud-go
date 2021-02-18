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

package pubsublite

import (
	"fmt"
	"time"

	"cloud.google.com/go/internal/optional"
	"github.com/golang/protobuf/ptypes"

	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
	fmpb "google.golang.org/genproto/protobuf/field_mask"
)

// InfiniteRetention is a sentinel used in topic configs to denote an infinite
// retention duration (i.e. retain messages as long as there is available
// storage).
const InfiniteRetention = time.Duration(-1)

// TopicConfig describes the properties of a Pub/Sub Lite topic.
// See https://cloud.google.com/pubsub/lite/docs/topics for more information
// about how topics are configured.
type TopicConfig struct {
	// The full path of the topic, in the format:
	// "projects/PROJECT_ID/locations/ZONE/topics/TOPIC_ID".
	//
	// - PROJECT_ID: The project ID (e.g. "my-project") or the project number
	//   (e.g. "987654321") can be provided.
	// - ZONE: The Google Cloud zone (e.g. "us-central1-a") where the topic is
	//   located. See https://cloud.google.com/pubsub/lite/docs/locations for the
	//   list of zones where Pub/Sub Lite is available.
	// - TOPIC_ID: The ID of the topic (e.g. "my-topic"). See
	//   https://cloud.google.com/pubsub/docs/admin#resource_names for information
	//   about valid topic IDs.
	Name string

	// The number of partitions in the topic. Must be at least 1. Can be increased
	// after creation, but not decreased.
	PartitionCount int

	// Publish throughput capacity per partition in MiB/s.
	// Must be >= 4 and <= 16.
	PublishCapacityMiBPerSec int

	// Subscribe throughput capacity per partition in MiB/s.
	// Must be >= 4 and <= 32.
	SubscribeCapacityMiBPerSec int

	// The provisioned storage, in bytes, per partition. If the number of bytes
	// stored in any of the topic's partitions grows beyond this value, older
	// messages will be dropped to make room for newer ones, regardless of the
	// value of `RetentionDuration`. Must be > 0.
	PerPartitionBytes int64

	// How long a published message is retained. If set to `InfiniteRetention`,
	// messages will be retained as long as the bytes retained for each partition
	// is below `PerPartitionBytes`. Otherwise, must be > 0.
	RetentionDuration time.Duration
}

func (tc *TopicConfig) toProto() *pb.Topic {
	topicpb := &pb.Topic{
		Name: tc.Name,
		PartitionConfig: &pb.Topic_PartitionConfig{
			Count: int64(tc.PartitionCount),
			Dimension: &pb.Topic_PartitionConfig_Capacity_{
				Capacity: &pb.Topic_PartitionConfig_Capacity{
					PublishMibPerSec:   int32(tc.PublishCapacityMiBPerSec),
					SubscribeMibPerSec: int32(tc.SubscribeCapacityMiBPerSec),
				},
			},
		},
		RetentionConfig: &pb.Topic_RetentionConfig{
			PerPartitionBytes: tc.PerPartitionBytes,
		},
	}
	if tc.RetentionDuration >= 0 {
		topicpb.RetentionConfig.Period = ptypes.DurationProto(tc.RetentionDuration)
	}
	return topicpb
}

func protoToTopicConfig(t *pb.Topic) (*TopicConfig, error) {
	partitionCfg := t.GetPartitionConfig()
	retentionCfg := t.GetRetentionConfig()
	topic := &TopicConfig{
		Name:                       t.GetName(),
		PartitionCount:             int(partitionCfg.GetCount()),
		PublishCapacityMiBPerSec:   int(partitionCfg.GetCapacity().GetPublishMibPerSec()),
		SubscribeCapacityMiBPerSec: int(partitionCfg.GetCapacity().GetSubscribeMibPerSec()),
		PerPartitionBytes:          retentionCfg.GetPerPartitionBytes(),
		RetentionDuration:          InfiniteRetention,
	}
	// An unset retention period proto denotes "infinite retention".
	if retentionCfg.Period != nil {
		period, err := ptypes.Duration(retentionCfg.Period)
		if err != nil {
			return nil, fmt.Errorf("pubsublite: invalid retention period in topic config: %v", err)
		}
		topic.RetentionDuration = period
	}
	return topic, nil
}

// TopicConfigToUpdate specifies the properties to update for a topic.
type TopicConfigToUpdate struct {
	// The full path of the topic to update, in the format:
	// "projects/PROJECT_ID/locations/ZONE/topics/TOPIC_ID". Required.
	Name string

	// If non-zero, will update the number of partitions in the topic. The number
	// of partitions can only be increased, not decreased.
	PartitionCount int

	// If non-zero, will update the publish throughput capacity per partition.
	PublishCapacityMiBPerSec int

	// If non-zero, will update the subscribe throughput capacity per partition.
	SubscribeCapacityMiBPerSec int

	// If non-zero, will update the provisioned storage per partition.
	PerPartitionBytes int64

	// If specified, will update how long a published message is retained. To
	// clear a retention duration (i.e. retain messages as long as there is
	// available storage), set this to `InfiniteRetention`.
	RetentionDuration optional.Duration
}

func (tc *TopicConfigToUpdate) toUpdateRequest() *pb.UpdateTopicRequest {
	updatedTopic := &pb.Topic{
		Name: tc.Name,
		PartitionConfig: &pb.Topic_PartitionConfig{
			Count: int64(tc.PartitionCount),
			Dimension: &pb.Topic_PartitionConfig_Capacity_{
				Capacity: &pb.Topic_PartitionConfig_Capacity{
					PublishMibPerSec:   int32(tc.PublishCapacityMiBPerSec),
					SubscribeMibPerSec: int32(tc.SubscribeCapacityMiBPerSec),
				},
			},
		},
		RetentionConfig: &pb.Topic_RetentionConfig{
			PerPartitionBytes: tc.PerPartitionBytes,
		},
	}

	var fields []string
	if tc.PartitionCount > 0 {
		fields = append(fields, "partition_config.count")
	}
	if tc.PublishCapacityMiBPerSec > 0 {
		fields = append(fields, "partition_config.capacity.publish_mib_per_sec")
	}
	if tc.SubscribeCapacityMiBPerSec > 0 {
		fields = append(fields, "partition_config.capacity.subscribe_mib_per_sec")
	}
	if tc.PerPartitionBytes > 0 {
		fields = append(fields, "retention_config.per_partition_bytes")
	}
	if tc.RetentionDuration != nil {
		fields = append(fields, "retention_config.period")
		duration := optional.ToDuration(tc.RetentionDuration)
		// An unset retention period proto denotes "infinite retention".
		if duration >= 0 {
			updatedTopic.RetentionConfig.Period = ptypes.DurationProto(duration)
		}
	}

	return &pb.UpdateTopicRequest{
		Topic:      updatedTopic,
		UpdateMask: &fmpb.FieldMask{Paths: fields},
	}
}

// DeliveryRequirement specifies when a subscription should send messages to
// subscribers relative to persistence in storage.
type DeliveryRequirement int

const (
	// UnspecifiedDeliveryRequirement represents an unset delivery requirement.
	UnspecifiedDeliveryRequirement DeliveryRequirement = iota

	// DeliverImmediately means the server will not wait for a published message
	// to be successfully written to storage before delivering it to subscribers.
	DeliverImmediately

	// DeliverAfterStored means the server will not deliver a published message to
	// subscribers until the message has been successfully written to storage.
	// This will result in higher end-to-end latency, but consistent delivery.
	DeliverAfterStored
)

// SubscriptionConfig describes the properties of a Pub/Sub Lite subscription,
// which is attached to a Pub/Sub Lite topic.
// See https://cloud.google.com/pubsub/lite/docs/subscriptions for more
// information about how subscriptions are configured.
type SubscriptionConfig struct {
	// The full path of the subscription, in the format:
	// "projects/PROJECT_ID/locations/ZONE/subscriptions/SUBSCRIPTION_ID".
	//
	// - PROJECT_ID: The project ID (e.g. "my-project") or the project number
	//   (e.g. "987654321") can be provided.
	// - ZONE: The Google Cloud zone (e.g. "us-central1-a") of the corresponding
	//   topic.
	// - SUBSCRIPTION_ID: The ID of the subscription (e.g. "my-subscription"). See
	//   https://cloud.google.com/pubsub/docs/admin#resource_names for information
	//   about valid subscription IDs.
	Name string

	// The path of the topic that this subscription is attached to, in the format:
	// "projects/PROJECT_ID/locations/ZONE/topics/TOPIC_ID". This cannot be
	// changed after creation.
	Topic string

	// Whether a message should be delivered to subscribers immediately after it
	// has been published or after it has been successfully written to storage.
	DeliveryRequirement DeliveryRequirement
}

func (sc *SubscriptionConfig) toProto() *pb.Subscription {
	subspb := &pb.Subscription{
		Name:  sc.Name,
		Topic: sc.Topic,
	}
	if sc.DeliveryRequirement > 0 {
		subspb.DeliveryConfig = &pb.Subscription_DeliveryConfig{
			// Note: Assumes DeliveryRequirement enum values match API proto.
			DeliveryRequirement: pb.Subscription_DeliveryConfig_DeliveryRequirement(sc.DeliveryRequirement),
		}
	}
	return subspb
}

func protoToSubscriptionConfig(s *pb.Subscription) *SubscriptionConfig {
	return &SubscriptionConfig{
		Name:  s.GetName(),
		Topic: s.GetTopic(),
		// Note: Assumes DeliveryRequirement enum values match API proto.
		DeliveryRequirement: DeliveryRequirement(s.GetDeliveryConfig().GetDeliveryRequirement().Number()),
	}
}

// SubscriptionConfigToUpdate specifies the properties to update for a
// subscription.
type SubscriptionConfigToUpdate struct {
	// The full path of the subscription to update, in the format:
	// "projects/PROJECT_ID/locations/ZONE/subscriptions/SUBSCRIPTION_ID".
	// Required.
	Name string

	// If non-zero, updates the message delivery requirement.
	DeliveryRequirement DeliveryRequirement
}

func (sc *SubscriptionConfigToUpdate) toUpdateRequest() *pb.UpdateSubscriptionRequest {
	updatedSubs := &pb.Subscription{
		Name: sc.Name,
		DeliveryConfig: &pb.Subscription_DeliveryConfig{
			// Note: Assumes DeliveryRequirement enum values match API proto.
			DeliveryRequirement: pb.Subscription_DeliveryConfig_DeliveryRequirement(sc.DeliveryRequirement),
		},
	}

	var fields []string
	if sc.DeliveryRequirement > 0 {
		fields = append(fields, "delivery_config.delivery_requirement")
	}

	return &pb.UpdateSubscriptionRequest{
		Subscription: updatedSubs,
		UpdateMask:   &fmpb.FieldMask{Paths: fields},
	}
}
