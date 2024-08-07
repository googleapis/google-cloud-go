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

	pb "cloud.google.com/go/pubsublite/apiv1/pubsublitepb"
	"google.golang.org/protobuf/types/known/durationpb"
	fmpb "google.golang.org/protobuf/types/known/fieldmaskpb"
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
	// "projects/PROJECT_ID/locations/LOCATION/topics/TOPIC_ID".
	//
	// - PROJECT_ID: The project ID (e.g. "my-project") or the project number
	//   (e.g. "987654321") can be provided.
	// - LOCATION: The Google Cloud region (e.g. "us-central1") or zone
	//   (e.g. "us-central1-a") where the topic is located.
	//   See https://cloud.google.com/pubsub/lite/docs/locations for the list of
	//   regions and zones where Pub/Sub Lite is available.
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
	// value of `RetentionDuration`. Must be >= 30 GiB.
	PerPartitionBytes int64

	// How long a published message is retained. If set to `InfiniteRetention`,
	// messages will be retained as long as the bytes retained for each partition
	// is below `PerPartitionBytes`. Otherwise, must be > 0.
	RetentionDuration time.Duration

	// The path of the reservation to use for this topic's throughput capacity, in
	// the format:
	// "projects/PROJECT_ID/locations/REGION/reservations/RESERVATION_ID".
	ThroughputReservation string
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
		topicpb.RetentionConfig.Period = durationpb.New(tc.RetentionDuration)
	}
	if len(tc.ThroughputReservation) > 0 {
		topicpb.ReservationConfig = &pb.Topic_ReservationConfig{
			ThroughputReservation: tc.ThroughputReservation,
		}
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
		ThroughputReservation:      t.GetReservationConfig().GetThroughputReservation(),
	}
	// An unset retention period proto denotes "infinite retention".
	if retentionCfg.Period != nil {
		if err := retentionCfg.GetPeriod().CheckValid(); err != nil {
			return nil, fmt.Errorf("pubsublite: invalid retention period in topic config: %w", err)
		}
		topic.RetentionDuration = retentionCfg.GetPeriod().AsDuration()
	}
	return topic, nil
}

// TopicConfigToUpdate specifies the properties to update for a topic.
type TopicConfigToUpdate struct {
	// The full path of the topic to update, in the format:
	// "projects/PROJECT_ID/locations/LOCATION/topics/TOPIC_ID". Required.
	Name string

	// If non-zero, will update the number of partitions in the topic.
	// Set value must be >= 1. The number of partitions can only be increased, not
	// decreased.
	PartitionCount int

	// If non-zero, will update the publish throughput capacity per partition.
	// Set value must be >= 4 and <= 16.
	PublishCapacityMiBPerSec int

	// If non-zero, will update the subscribe throughput capacity per partition.
	// Set value must be >= 4 and <= 32.
	SubscribeCapacityMiBPerSec int

	// If non-zero, will update the provisioned storage per partition.
	// Set value must be >= 30 GiB.
	PerPartitionBytes int64

	// If specified, will update how long a published message is retained. To
	// clear a retention duration (i.e. retain messages as long as there is
	// available storage), set this to `InfiniteRetention`.
	RetentionDuration optional.Duration

	// The path of the reservation to use for this topic's throughput capacity, in
	// the format:
	// "projects/PROJECT_ID/locations/REGION/reservations/RESERVATION_ID".
	ThroughputReservation optional.String
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
			updatedTopic.RetentionConfig.Period = durationpb.New(duration)
		}
	}
	if tc.ThroughputReservation != nil {
		fields = append(fields, "reservation_config.throughput_reservation")
		updatedTopic.ReservationConfig = &pb.Topic_ReservationConfig{
			ThroughputReservation: optional.ToString(tc.ThroughputReservation),
		}
	}

	return &pb.UpdateTopicRequest{
		Topic:      updatedTopic,
		UpdateMask: &fmpb.FieldMask{Paths: fields},
	}
}

// ExportDestinationConfig is the configuration for exporting to a destination.
// Implemented by *PubSubDestinationConfig.
type ExportDestinationConfig interface {
	setExportConfig(ec *pb.ExportConfig) string
}

// PubSubDestinationConfig configures messages to be exported to a Pub/Sub
// topic. Implements the ExportDestinationConfig interface.
//
// See https://cloud.google.com/pubsub/lite/docs/export-pubsub for more
// information about how export subscriptions to Pub/Sub are configured.
type PubSubDestinationConfig struct {
	// The path of a Pub/Sub topic, in the format:
	// "projects/PROJECT_ID/topics/TOPIC_ID".
	Topic string
}

func (pc *PubSubDestinationConfig) setExportConfig(ec *pb.ExportConfig) string {
	ec.Destination = &pb.ExportConfig_PubsubConfig{
		PubsubConfig: &pb.ExportConfig_PubSubConfig{Topic: pc.Topic},
	}
	return "export_config.pubsub_config"
}

// ExportState specifies the desired state of an export subscription.
type ExportState int

const (
	// UnspecifiedExportState represents an unset export state.
	UnspecifiedExportState ExportState = iota

	// ExportActive specifies that export processing should be enabled.
	ExportActive

	// ExportPaused specifies that export processing should be suspended.
	ExportPaused

	// ExportPermissionDenied specifies that messages cannot be exported due to
	// permission denied errors. Output only.
	ExportPermissionDenied

	// ExportResourceNotFound specifies that messages cannot be exported due to
	// missing resources. Output only.
	ExportResourceNotFound
)

// ExportConfig describes the properties of a Pub/Sub Lite export subscription,
// which configures the service to write messages to a destination.
type ExportConfig struct {
	// The desired state of this export subscription. This should only be set to
	// ExportActive or ExportPaused.
	DesiredState ExportState

	// This is an output only field that reports the current export state. It is
	// ignored if set in any requests.
	CurrentState ExportState

	// The path of an optional Pub/Sub Lite topic to receive messages that cannot
	// be exported to the destination, in the format:
	// "projects/PROJECT_ID/locations/LOCATION/topics/TOPIC_ID".
	// Must be within the same project and location as the subscription.
	DeadLetterTopic string

	// The destination to export messages to.
	Destination ExportDestinationConfig
}

func (ec *ExportConfig) toProto() *pb.ExportConfig {
	if ec == nil {
		return nil
	}
	epb := &pb.ExportConfig{
		DeadLetterTopic: ec.DeadLetterTopic,
		// Note: Assumes enum values match API proto.
		DesiredState: pb.ExportConfig_State(ec.DesiredState),
		CurrentState: pb.ExportConfig_State(ec.CurrentState),
	}
	if ec.Destination != nil {
		ec.Destination.setExportConfig(epb)
	}
	return epb
}

func protoToExportConfig(epb *pb.ExportConfig) *ExportConfig {
	if epb == nil {
		return nil
	}
	ec := &ExportConfig{
		DeadLetterTopic: epb.GetDeadLetterTopic(),
		// Note: Assumes enum values match API proto.
		DesiredState: ExportState(epb.GetDesiredState().Number()),
		CurrentState: ExportState(epb.GetCurrentState().Number()),
	}
	if ps := epb.GetPubsubConfig(); ps != nil {
		ec.Destination = &PubSubDestinationConfig{Topic: ps.Topic}
	}
	return ec
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
	// "projects/PROJECT_ID/locations/LOCATION/subscriptions/SUBSCRIPTION_ID".
	//
	// - PROJECT_ID: The project ID (e.g. "my-project") or the project number
	//   (e.g. "987654321") can be provided.
	// - LOCATION: The Google Cloud region (e.g. "us-central1") or zone
	//   (e.g. "us-central1-a") of the corresponding topic.
	// - SUBSCRIPTION_ID: The ID of the subscription (e.g. "my-subscription"). See
	//   https://cloud.google.com/pubsub/docs/admin#resource_names for information
	//   about valid subscription IDs.
	Name string

	// The path of the topic that this subscription is attached to, in the format:
	// "projects/PROJECT_ID/locations/LOCATION/topics/TOPIC_ID". This cannot be
	// changed after creation.
	Topic string

	// Whether a message should be delivered to subscribers immediately after it
	// has been published or after it has been successfully written to storage.
	DeliveryRequirement DeliveryRequirement

	// If non-nil, configures this subscription to export messages from the
	// associated topic to a destination. The ExportConfig cannot be removed after
	// creation of the subscription, however its properties can be changed.
	ExportConfig *ExportConfig
}

func (sc *SubscriptionConfig) toProto() *pb.Subscription {
	subspb := &pb.Subscription{
		Name:         sc.Name,
		Topic:        sc.Topic,
		ExportConfig: sc.ExportConfig.toProto(),
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
		ExportConfig:        protoToExportConfig(s.GetExportConfig()),
	}
}

// ExportConfigToUpdate specifies the properties to update for an export
// subscription.
type ExportConfigToUpdate struct {
	// If non-zero, updates the desired state. This should only be set to
	// ExportActive or ExportPaused.
	DesiredState ExportState

	// The path of an optional Pub/Sub Lite topic to receive messages that cannot
	// be exported to the destination, in the format:
	// "projects/PROJECT_ID/locations/LOCATION/topics/TOPIC_ID".
	// Must be within the same project and location as the subscription.
	DeadLetterTopic optional.String

	// If non-nil, updates the export destination configuration.
	Destination ExportDestinationConfig
}

func (ec *ExportConfigToUpdate) toUpdateRequest() (*pb.ExportConfig, []string) {
	if ec == nil {
		return nil, nil
	}
	var fields []string
	updatedExport := &pb.ExportConfig{
		// Note: Assumes enum values match API proto.
		DesiredState: pb.ExportConfig_State(ec.DesiredState),
	}
	if ec.DesiredState > 0 {
		fields = append(fields, "export_config.desired_state")
	}
	if ec.Destination != nil {
		destinationField := ec.Destination.setExportConfig(updatedExport)
		fields = append(fields, destinationField)
	}
	if ec.DeadLetterTopic != nil {
		updatedExport.DeadLetterTopic = optional.ToString(ec.DeadLetterTopic)
		fields = append(fields, "export_config.dead_letter_topic")
	}
	return updatedExport, fields
}

// SubscriptionConfigToUpdate specifies the properties to update for a
// subscription.
type SubscriptionConfigToUpdate struct {
	// The full path of the subscription to update, in the format:
	// "projects/PROJECT_ID/locations/LOCATION/subscriptions/SUBSCRIPTION_ID".
	// Required.
	Name string

	// If non-zero, updates the message delivery requirement.
	DeliveryRequirement DeliveryRequirement

	// If non-nil, updates export config properties.
	ExportConfig *ExportConfigToUpdate
}

func (sc *SubscriptionConfigToUpdate) toUpdateRequest() *pb.UpdateSubscriptionRequest {
	exportConfig, fields := sc.ExportConfig.toUpdateRequest()
	updatedSubs := &pb.Subscription{
		Name: sc.Name,
		DeliveryConfig: &pb.Subscription_DeliveryConfig{
			// Note: Assumes DeliveryRequirement enum values match API proto.
			DeliveryRequirement: pb.Subscription_DeliveryConfig_DeliveryRequirement(sc.DeliveryRequirement),
		},
		ExportConfig: exportConfig,
	}
	if sc.DeliveryRequirement > 0 {
		fields = append(fields, "delivery_config.delivery_requirement")
	}

	return &pb.UpdateSubscriptionRequest{
		Subscription: updatedSubs,
		UpdateMask:   &fmpb.FieldMask{Paths: fields},
	}
}

// ReservationConfig describes the properties of a Pub/Sub Lite reservation.
type ReservationConfig struct {
	// The full path of the reservation, in the format:
	// "projects/PROJECT_ID/locations/REGION/reservations/RESERVATION_ID".
	//
	// - PROJECT_ID: The project ID (e.g. "my-project") or the project number
	//   (e.g. "987654321") can be provided.
	// - REGION: The Google Cloud region (e.g. "us-central1") for the reservation.
	//   See https://cloud.google.com/pubsub/lite/docs/locations for the list of
	//   regions where Pub/Sub Lite is available.
	// - RESERVATION_ID: The ID of the reservation (e.g. "my-reservation"). See
	//   https://cloud.google.com/pubsub/docs/admin#resource_names for information
	//   about valid reservation IDs.
	Name string

	// The reserved throughput capacity. Every unit of throughput capacity is
	// equivalent to 1 MiB/s of published messages or 2 MiB/s of subscribed
	// messages.
	//
	// Any topics which are declared as using capacity from a reservation will
	// consume resources from this reservation instead of being charged
	// individually.
	ThroughputCapacity int
}

func (rc *ReservationConfig) toProto() *pb.Reservation {
	return &pb.Reservation{
		Name:               rc.Name,
		ThroughputCapacity: int64(rc.ThroughputCapacity),
	}
}

func protoToReservationConfig(r *pb.Reservation) *ReservationConfig {
	return &ReservationConfig{
		Name:               r.GetName(),
		ThroughputCapacity: int(r.GetThroughputCapacity()),
	}
}

// ReservationConfigToUpdate specifies the properties to update for a
// reservation.
type ReservationConfigToUpdate struct {
	// The full path of the reservation to update, in the format:
	// "projects/PROJECT_ID/locations/REGION/reservations/RESERVATION_ID".
	// Required.
	Name string

	// If non-zero, updates the throughput capacity.
	ThroughputCapacity int
}

func (rc *ReservationConfigToUpdate) toUpdateRequest() *pb.UpdateReservationRequest {
	var fields []string
	if rc.ThroughputCapacity > 0 {
		fields = append(fields, "throughput_capacity")
	}

	return &pb.UpdateReservationRequest{
		Reservation: &pb.Reservation{
			Name:               rc.Name,
			ThroughputCapacity: int64(rc.ThroughputCapacity),
		},
		UpdateMask: &fmpb.FieldMask{Paths: fields},
	}
}
