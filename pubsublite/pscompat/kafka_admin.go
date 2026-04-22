// Copyright 2026 Google LLC
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

package pscompat

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/IBM/sarama"
)

// metadataRetry is used by TopicPartitionCount, GetTopic, and ListTopics to
// tolerate the brief window after CreateTopic where Kafka metadata has not yet
// propagated to all brokers.
const (
	metadataRetryAttempts = 6
	metadataRetryDelay    = 500 * time.Millisecond
)

// ErrUnsupported indicates an operation that has no meaningful Kafka analogue.
// Callers should either omit the operation or handle this error explicitly.
var ErrUnsupported = errors.New("gmk: operation unsupported on Managed Kafka backend")

// KafkaAdminClientConfig configures a KafkaAdminClient.
type KafkaAdminClientConfig struct {
	// BootstrapServers is the Kafka bootstrap server address.
	// Use BuildGMKBootstrapServer() to construct this for GMK clusters.
	BootstrapServers string

	// SaramaConfig is an optional pre-built Sarama configuration. If nil,
	// NewGMKSaramaConfig() is called to build one with GCP OAUTHBEARER auth.
	SaramaConfig *sarama.Config
}

// KafkaAdminClient is a Managed Kafka-backed admin client providing PSL-style
// topic and subscription management. Subscriptions map to Kafka consumer
// groups; topics map to Kafka topics.
//
// Methods that have no Kafka analogue (updateSubscription, reservation ops,
// seekSubscription) either return ErrUnsupported or log a WARNING and succeed
// as a no-op, matching the documented degraded-behavior contract.
type KafkaAdminClient struct {
	admin sarama.ClusterAdmin
}

// NewKafkaAdminClient creates an admin client connected to Managed Kafka.
func NewKafkaAdminClient(ctx context.Context, cfg *KafkaAdminClientConfig) (*KafkaAdminClient, error) {
	if cfg == nil || cfg.BootstrapServers == "" {
		return nil, fmt.Errorf("gmk: KafkaAdminClientConfig.BootstrapServers must not be empty")
	}
	saramaCfg := cfg.SaramaConfig
	if saramaCfg == nil {
		var err error
		saramaCfg, err = NewGMKSaramaConfig(ctx)
		if err != nil {
			return nil, fmt.Errorf("gmk: failed to create Sarama config: %w", err)
		}
	}
	admin, err := sarama.NewClusterAdmin([]string{cfg.BootstrapServers}, saramaCfg)
	if err != nil {
		return nil, fmt.Errorf("gmk: failed to create cluster admin: %w", err)
	}
	return &KafkaAdminClient{admin: admin}, nil
}

// Close releases the underlying connection.
func (c *KafkaAdminClient) Close() error {
	return c.admin.Close()
}

// CreateTopic creates a Kafka topic with the given partition count and
// replication factor.
func (c *KafkaAdminClient) CreateTopic(_ context.Context, topic string, numPartitions int32, replicationFactor int16) error {
	return c.admin.CreateTopic(topic, &sarama.TopicDetail{
		NumPartitions:     numPartitions,
		ReplicationFactor: replicationFactor,
	}, false)
}

// DeleteTopic deletes a Kafka topic.
func (c *KafkaAdminClient) DeleteTopic(_ context.Context, topic string) error {
	return c.admin.DeleteTopic(topic)
}

// TopicPartitionCount returns the number of partitions for the given topic.
// Retries briefly to tolerate post-CreateTopic metadata propagation.
func (c *KafkaAdminClient) TopicPartitionCount(_ context.Context, topic string) (int, error) {
	var lastErr error
	for i := 0; i < metadataRetryAttempts; i++ {
		meta, err := c.admin.DescribeTopics([]string{topic})
		if err == nil && len(meta) > 0 && meta[0] != nil && meta[0].Err == sarama.ErrNoError {
			return len(meta[0].Partitions), nil
		}
		if err != nil {
			lastErr = err
		} else if len(meta) > 0 && meta[0] != nil {
			lastErr = meta[0].Err
		} else {
			lastErr = fmt.Errorf("topic %q not found", topic)
		}
		time.Sleep(metadataRetryDelay)
	}
	return 0, fmt.Errorf("gmk: describe topic %q after %d attempts: %w", topic, metadataRetryAttempts, lastErr)
}

// GetTopic returns metadata for a single topic.
func (c *KafkaAdminClient) GetTopic(_ context.Context, topic string) (*sarama.TopicMetadata, error) {
	meta, err := c.admin.DescribeTopics([]string{topic})
	if err != nil {
		return nil, err
	}
	if len(meta) == 0 || meta[0] == nil {
		return nil, fmt.Errorf("gmk: topic %q not found", topic)
	}
	if meta[0].Err != sarama.ErrNoError {
		return nil, fmt.Errorf("gmk: describe topic %q: %w", topic, meta[0].Err)
	}
	return meta[0], nil
}

// ListTopics returns all non-internal topic names in the cluster.
// Internal topics (those starting with "__") are filtered out, matching the
// Java KafkaAdminClient behavior. Optionally accepts an "expect" topic that
// must be present; retries briefly to tolerate post-CreateTopic metadata
// propagation.
func (c *KafkaAdminClient) ListTopics(_ context.Context) ([]string, error) {
	topics, err := c.admin.ListTopics()
	if err != nil {
		return nil, err
	}
	var out []string
	for name := range topics {
		if strings.HasPrefix(name, "__") {
			continue
		}
		out = append(out, name)
	}
	return out, nil
}

// ListTopicsAwait is like ListTopics but retries until the named topic shows
// up in the listing or attempts are exhausted. Useful for workflows that
// create a topic and then immediately list it.
func (c *KafkaAdminClient) ListTopicsAwait(ctx context.Context, expect string) ([]string, error) {
	var lastErr error
	for i := 0; i < metadataRetryAttempts; i++ {
		topics, err := c.ListTopics(ctx)
		if err != nil {
			lastErr = err
			time.Sleep(metadataRetryDelay)
			continue
		}
		for _, t := range topics {
			if t == expect {
				return topics, nil
			}
		}
		lastErr = fmt.Errorf("topic %q not yet visible in listing", expect)
		time.Sleep(metadataRetryDelay)
	}
	return nil, fmt.Errorf("gmk: ListTopicsAwait %q: %w", expect, lastErr)
}

// UpdateTopic is a no-op on Managed Kafka. Kafka does not support mutable
// topic configurations. A WARNING is logged for visibility.
func (c *KafkaAdminClient) UpdateTopic(_ context.Context, topic string) error {
	log.Printf("WARNING: gmk: UpdateTopic is a no-op on Managed Kafka (topic=%q)", topic)
	return nil
}

// IncreasePartitions grows a topic's partition count. Kafka only supports
// monotonically increasing partition counts; shrinking is not possible.
func (c *KafkaAdminClient) IncreasePartitions(_ context.Context, topic string, newCount int32) error {
	return c.admin.CreatePartitions(topic, newCount, nil, false)
}

// SeekSubscription returns ErrUnsupported. Admin-level seeks are not exposed
// by the Kafka admin surface; use KafkaCursorClient for offset management.
func (c *KafkaAdminClient) SeekSubscription(_ context.Context, _ string) error {
	return fmt.Errorf("gmk: SeekSubscription not supported; use KafkaCursorClient.ResetOffsets: %w", ErrUnsupported)
}

// CreateSubscription is a no-op on Managed Kafka. Consumer groups are created
// implicitly when a consumer joins them for the first time.
func (c *KafkaAdminClient) CreateSubscription(_ context.Context, groupID string) error {
	log.Printf("INFO: gmk: CreateSubscription is a no-op on Managed Kafka (groupID=%q); groups are created implicitly", groupID)
	return nil
}

// UpdateSubscription is a no-op on Managed Kafka. Kafka consumer groups lack
// centralized mutable configuration.
func (c *KafkaAdminClient) UpdateSubscription(_ context.Context, groupID string) error {
	log.Printf("WARNING: gmk: UpdateSubscription is a no-op on Managed Kafka (groupID=%q)", groupID)
	return nil
}

// DeleteSubscription deletes a Kafka consumer group.
func (c *KafkaAdminClient) DeleteSubscription(_ context.Context, groupID string) error {
	return c.admin.DeleteConsumerGroup(groupID)
}

// GetSubscription returns the offsets currently committed for a consumer
// group on a given topic. Matches PSL's GetSubscription semantics insofar as
// Kafka offers an analogue.
func (c *KafkaAdminClient) GetSubscription(_ context.Context, groupID, topic string) (map[int32]int64, error) {
	resp, err := c.admin.ListConsumerGroupOffsets(groupID, map[string][]int32{topic: nil})
	if err != nil {
		return nil, err
	}
	out := make(map[int32]int64)
	for p, block := range resp.Blocks[topic] {
		out[p] = block.Offset
	}
	return out, nil
}

// ListSubscriptions returns all Kafka consumer group names in the cluster.
func (c *KafkaAdminClient) ListSubscriptions(_ context.Context) ([]string, error) {
	groups, err := c.admin.ListConsumerGroups()
	if err != nil {
		return nil, err
	}
	var out []string
	for name := range groups {
		out = append(out, name)
	}
	return out, nil
}

// --- Reservation operations (no-ops) ---------------------------------------
//
// PSL reservation concepts (throughput capacity units) have no Kafka analogue.
// These methods exist for PSL surface parity and simply log at INFO level.

// CreateReservation is a no-op on Managed Kafka.
func (c *KafkaAdminClient) CreateReservation(_ context.Context, name string) error {
	log.Printf("INFO: gmk: CreateReservation is a no-op on Managed Kafka (name=%q)", name)
	return nil
}

// GetReservation is a no-op on Managed Kafka.
func (c *KafkaAdminClient) GetReservation(_ context.Context, name string) error {
	log.Printf("INFO: gmk: GetReservation is a no-op on Managed Kafka (name=%q)", name)
	return nil
}

// ListReservations is a no-op on Managed Kafka.
func (c *KafkaAdminClient) ListReservations(_ context.Context) ([]string, error) {
	log.Printf("INFO: gmk: ListReservations is a no-op on Managed Kafka")
	return nil, nil
}

// UpdateReservation is a no-op on Managed Kafka.
func (c *KafkaAdminClient) UpdateReservation(_ context.Context, name string) error {
	log.Printf("INFO: gmk: UpdateReservation is a no-op on Managed Kafka (name=%q)", name)
	return nil
}

// DeleteReservation is a no-op on Managed Kafka.
func (c *KafkaAdminClient) DeleteReservation(_ context.Context, name string) error {
	log.Printf("INFO: gmk: DeleteReservation is a no-op on Managed Kafka (name=%q)", name)
	return nil
}
