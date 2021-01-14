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
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"google.golang.org/protobuf/proto"

	dpb "github.com/golang/protobuf/ptypes/duration"
	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
	fmpb "google.golang.org/genproto/protobuf/field_mask"
)

func TestTopicConfigToProtoConversion(t *testing.T) {
	for _, tc := range []struct {
		desc       string
		topicpb    *pb.Topic
		wantConfig *TopicConfig
	}{
		{
			desc: "retention duration set",
			topicpb: &pb.Topic{
				Name: "projects/my-proj/locations/us-central1-c/topics/my-topic",
				PartitionConfig: &pb.Topic_PartitionConfig{
					Count: 2,
					Dimension: &pb.Topic_PartitionConfig_Capacity_{
						Capacity: &pb.Topic_PartitionConfig_Capacity{
							PublishMibPerSec:   6,
							SubscribeMibPerSec: 16,
						},
					},
				},
				RetentionConfig: &pb.Topic_RetentionConfig{
					PerPartitionBytes: 1073741824,
					Period: &dpb.Duration{
						Seconds: 86400,
						Nanos:   600,
					},
				},
			},
			wantConfig: &TopicConfig{
				Name:                       "projects/my-proj/locations/us-central1-c/topics/my-topic",
				PartitionCount:             2,
				PublishCapacityMiBPerSec:   6,
				SubscribeCapacityMiBPerSec: 16,
				PerPartitionBytes:          1073741824,
				RetentionDuration:          time.Duration(86400*1e9 + 600),
			},
		},
		{
			desc: "retention duration unset",
			topicpb: &pb.Topic{
				Name: "projects/my-proj/locations/europe-west1-b/topics/my-topic",
				PartitionConfig: &pb.Topic_PartitionConfig{
					Count: 3,
					Dimension: &pb.Topic_PartitionConfig_Capacity_{
						Capacity: &pb.Topic_PartitionConfig_Capacity{
							PublishMibPerSec:   4,
							SubscribeMibPerSec: 8,
						},
					},
				},
				RetentionConfig: &pb.Topic_RetentionConfig{
					PerPartitionBytes: 4294967296,
				},
			},
			wantConfig: &TopicConfig{
				Name:                       "projects/my-proj/locations/europe-west1-b/topics/my-topic",
				PartitionCount:             3,
				PublishCapacityMiBPerSec:   4,
				SubscribeCapacityMiBPerSec: 8,
				PerPartitionBytes:          4294967296,
				RetentionDuration:          InfiniteRetention,
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			gotConfig, gotErr := protoToTopicConfig(tc.topicpb)
			if !testutil.Equal(gotConfig, tc.wantConfig) || gotErr != nil {
				t.Errorf("protoToTopicConfig(%v)\ngot (%v, %v)\nwant (%v, nil)", tc.topicpb, gotConfig, gotErr, tc.wantConfig)
			}

			// Check that the config converts back to an identical proto.
			if gotProto := tc.wantConfig.toProto(); !proto.Equal(gotProto, tc.topicpb) {
				t.Errorf("TopicConfig: %v toProto():\ngot: %v\nwant: %v", tc.wantConfig, gotProto, tc.topicpb)
			}
		})
	}
}

func TestTopicUpdateRequest(t *testing.T) {
	for _, tc := range []struct {
		desc   string
		config *TopicConfigToUpdate
		want   *pb.UpdateTopicRequest
	}{
		{
			desc: "all fields set",
			config: &TopicConfigToUpdate{
				Name:                       "projects/my-proj/locations/us-central1-c/topics/my-topic",
				PublishCapacityMiBPerSec:   4,
				SubscribeCapacityMiBPerSec: 12,
				PerPartitionBytes:          500000,
				RetentionDuration:          time.Duration(0),
			},
			want: &pb.UpdateTopicRequest{
				Topic: &pb.Topic{
					Name: "projects/my-proj/locations/us-central1-c/topics/my-topic",
					PartitionConfig: &pb.Topic_PartitionConfig{
						Dimension: &pb.Topic_PartitionConfig_Capacity_{
							Capacity: &pb.Topic_PartitionConfig_Capacity{
								PublishMibPerSec:   4,
								SubscribeMibPerSec: 12,
							},
						},
					},
					RetentionConfig: &pb.Topic_RetentionConfig{
						PerPartitionBytes: 500000,
						Period:            &dpb.Duration{},
					},
				},
				UpdateMask: &fmpb.FieldMask{
					Paths: []string{
						"partition_config.capacity.publish_mib_per_sec",
						"partition_config.capacity.subscribe_mib_per_sec",
						"retention_config.per_partition_bytes",
						"retention_config.period",
					},
				},
			},
		},
		{
			desc: "clear retention duration",
			config: &TopicConfigToUpdate{
				Name:              "projects/my-proj/locations/us-central1-c/topics/my-topic",
				RetentionDuration: InfiniteRetention,
			},
			want: &pb.UpdateTopicRequest{
				Topic: &pb.Topic{
					Name: "projects/my-proj/locations/us-central1-c/topics/my-topic",
					PartitionConfig: &pb.Topic_PartitionConfig{
						Dimension: &pb.Topic_PartitionConfig_Capacity_{
							Capacity: &pb.Topic_PartitionConfig_Capacity{},
						},
					},
					RetentionConfig: &pb.Topic_RetentionConfig{},
				},
				UpdateMask: &fmpb.FieldMask{
					Paths: []string{
						"retention_config.period",
					},
				},
			},
		},
		{
			desc: "no fields set",
			config: &TopicConfigToUpdate{
				Name: "projects/my-proj/locations/us-central1-c/topics/my-topic",
			},
			want: &pb.UpdateTopicRequest{
				Topic: &pb.Topic{
					Name: "projects/my-proj/locations/us-central1-c/topics/my-topic",
					PartitionConfig: &pb.Topic_PartitionConfig{
						Dimension: &pb.Topic_PartitionConfig_Capacity_{
							Capacity: &pb.Topic_PartitionConfig_Capacity{},
						},
					},
					RetentionConfig: &pb.Topic_RetentionConfig{},
				},
				UpdateMask: &fmpb.FieldMask{},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			if got := tc.config.toUpdateRequest(); !proto.Equal(got, tc.want) {
				t.Errorf("TopicConfigToUpdate: %v toUpdateRequest():\ngot: %v\nwant: %v", tc.config, got, tc.want)
			}
		})
	}
}

func TestSubscriptionConfigToProtoConversion(t *testing.T) {
	for _, tc := range []struct {
		desc       string
		subspb     *pb.Subscription
		wantConfig *SubscriptionConfig
	}{
		{
			desc: "with delivery config",
			subspb: &pb.Subscription{
				Name:  "projects/my-proj/locations/us-central1-c/subscriptions/my-subs",
				Topic: "projects/my-proj/locations/us-central1-c/topics/my-topic",
				DeliveryConfig: &pb.Subscription_DeliveryConfig{
					DeliveryRequirement: pb.Subscription_DeliveryConfig_DELIVER_AFTER_STORED,
				},
			},
			wantConfig: &SubscriptionConfig{
				Name:                "projects/my-proj/locations/us-central1-c/subscriptions/my-subs",
				Topic:               "projects/my-proj/locations/us-central1-c/topics/my-topic",
				DeliveryRequirement: DeliverAfterStored,
			},
		},
		{
			desc: "missing delivery config",
			subspb: &pb.Subscription{
				Name:  "projects/my-proj/locations/us-central1-c/subscriptions/my-subs",
				Topic: "projects/my-proj/locations/us-central1-c/topics/my-topic",
			},
			wantConfig: &SubscriptionConfig{
				Name:                "projects/my-proj/locations/us-central1-c/subscriptions/my-subs",
				Topic:               "projects/my-proj/locations/us-central1-c/topics/my-topic",
				DeliveryRequirement: UnspecifiedDeliveryRequirement,
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			gotConfig := protoToSubscriptionConfig(tc.subspb)
			if !testutil.Equal(gotConfig, tc.wantConfig) {
				t.Errorf("protoToSubscriptionConfig(%v)\ngot: %v\nwant: %v", tc.subspb, gotConfig, tc.wantConfig)
			}

			// Check that the config converts back to an identical proto.
			if gotProto := tc.wantConfig.toProto(); !proto.Equal(gotProto, tc.subspb) {
				t.Errorf("SubscriptionConfig: %v toProto():\ngot: %v\nwant: %v", tc.wantConfig, gotProto, tc.subspb)
			}
		})
	}
}

func TestSubscriptionUpdateRequest(t *testing.T) {
	for _, tc := range []struct {
		desc   string
		config *SubscriptionConfigToUpdate
		want   *pb.UpdateSubscriptionRequest
	}{
		{
			desc: "all fields set",
			config: &SubscriptionConfigToUpdate{
				Name:                "projects/my-proj/locations/us-central1-c/subscriptions/my-subs",
				DeliveryRequirement: DeliverImmediately,
			},
			want: &pb.UpdateSubscriptionRequest{
				Subscription: &pb.Subscription{
					Name: "projects/my-proj/locations/us-central1-c/subscriptions/my-subs",
					DeliveryConfig: &pb.Subscription_DeliveryConfig{
						DeliveryRequirement: pb.Subscription_DeliveryConfig_DELIVER_IMMEDIATELY,
					},
				},
				UpdateMask: &fmpb.FieldMask{
					Paths: []string{
						"delivery_config.delivery_requirement",
					},
				},
			},
		},
		{
			desc: "no fields set",
			config: &SubscriptionConfigToUpdate{
				Name: "projects/my-proj/locations/us-central1-c/subscriptions/my-subs",
			},
			want: &pb.UpdateSubscriptionRequest{
				Subscription: &pb.Subscription{
					Name:           "projects/my-proj/locations/us-central1-c/subscriptions/my-subs",
					DeliveryConfig: &pb.Subscription_DeliveryConfig{},
				},
				UpdateMask: &fmpb.FieldMask{},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			if got := tc.config.toUpdateRequest(); !proto.Equal(got, tc.want) {
				t.Errorf("SubscriptionConfigToUpdate: %v toUpdateRequest():\ngot: %v\nwant: %v", tc.config, got, tc.want)
			}
		})
	}
}
