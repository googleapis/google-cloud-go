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
	"fmt"
	"time"

	"github.com/IBM/sarama"
)

// defaultAvgMessageBytes is the per-message byte estimate used when the caller
// does not provide one. Matches Java KafkaTopicStatsClient's 1 KB default.
const defaultAvgMessageBytes = 1024

// KafkaTopicStatsClientConfig configures a KafkaTopicStatsClient.
type KafkaTopicStatsClientConfig struct {
	BootstrapServers string
	SaramaConfig     *sarama.Config
}

// KafkaTopicStatsClient exposes PSL-style offset and stats APIs backed by
// Kafka's ListOffsets protocol.
//
// Degraded semantics (documented in design doc):
//   - ComputeMessageStats does not populate min_publish_time (would require a
//     message read).
//   - Byte estimates default to 1 KB/message since Kafka provides no low-latency
//     per-offset byte count.
//   - ComputeCursorForEventTime is aliased to ComputeCursorForPublishTime
//     (Kafka lacks a native event-time primitive).
type KafkaTopicStatsClient struct {
	client sarama.Client
	admin  sarama.ClusterAdmin
}

// NewKafkaTopicStatsClient creates a TopicStats client connected to Managed
// Kafka.
func NewKafkaTopicStatsClient(ctx context.Context, cfg *KafkaTopicStatsClientConfig) (*KafkaTopicStatsClient, error) {
	if cfg == nil || cfg.BootstrapServers == "" {
		return nil, fmt.Errorf("gmk: KafkaTopicStatsClientConfig.BootstrapServers must not be empty")
	}
	saramaCfg := cfg.SaramaConfig
	if saramaCfg == nil {
		var err error
		saramaCfg, err = NewGMKSaramaConfig(ctx)
		if err != nil {
			return nil, fmt.Errorf("gmk: failed to create Sarama config: %w", err)
		}
	}
	client, err := sarama.NewClient([]string{cfg.BootstrapServers}, saramaCfg)
	if err != nil {
		return nil, fmt.Errorf("gmk: failed to create client: %w", err)
	}
	admin, err := sarama.NewClusterAdminFromClient(client)
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("gmk: failed to create admin: %w", err)
	}
	return &KafkaTopicStatsClient{client: client, admin: admin}, nil
}

// Close releases the underlying connection.
func (c *KafkaTopicStatsClient) Close() error {
	// admin.Close() closes the shared client.
	return c.admin.Close()
}

// GetEarliestOffset returns the earliest (oldest) offset available for the
// given partition.
func (c *KafkaTopicStatsClient) GetEarliestOffset(_ context.Context, topic string, partition int32) (int64, error) {
	return c.client.GetOffset(topic, partition, sarama.OffsetOldest)
}

// GetLatestOffset returns the latest (head) offset for the given partition.
// This is the offset of the next message to be produced, not the last message.
func (c *KafkaTopicStatsClient) GetLatestOffset(_ context.Context, topic string, partition int32) (int64, error) {
	return c.client.GetOffset(topic, partition, sarama.OffsetNewest)
}

// GetEarliestOffsets returns earliest offsets for the first partitionCount
// partitions of the topic.
func (c *KafkaTopicStatsClient) GetEarliestOffsets(ctx context.Context, topic string, partitionCount int) (map[int32]int64, error) {
	return c.offsetsPerPartition(ctx, topic, partitionCount, sarama.OffsetOldest)
}

// GetLatestOffsets returns latest offsets for the first partitionCount
// partitions of the topic.
func (c *KafkaTopicStatsClient) GetLatestOffsets(ctx context.Context, topic string, partitionCount int) (map[int32]int64, error) {
	return c.offsetsPerPartition(ctx, topic, partitionCount, sarama.OffsetNewest)
}

func (c *KafkaTopicStatsClient) offsetsPerPartition(_ context.Context, topic string, partitionCount int, spec int64) (map[int32]int64, error) {
	out := make(map[int32]int64, partitionCount)
	for p := int32(0); p < int32(partitionCount); p++ {
		off, err := c.client.GetOffset(topic, p, spec)
		if err != nil {
			return nil, fmt.Errorf("gmk: GetOffset topic=%q partition=%d: %w", topic, p, err)
		}
		out[p] = off
	}
	return out, nil
}

// GetOffsetForTimestamp returns the earliest offset whose record timestamp is
// greater than or equal to timestampMs. The bool return is false when no such
// offset exists (e.g. timestamp is past the head).
func (c *KafkaTopicStatsClient) GetOffsetForTimestamp(_ context.Context, topic string, partition int32, timestampMs int64) (int64, bool, error) {
	off, err := c.client.GetOffset(topic, partition, timestampMs)
	if err != nil {
		return 0, false, err
	}
	// Sarama returns -1 when the broker has no record at/after the timestamp.
	if off < 0 {
		return 0, false, nil
	}
	return off, true, nil
}

// MessageStats mirrors the subset of PSL ComputeMessageStatsResponse that
// Kafka can cheaply populate. MinPublishTime is left unset since Kafka lacks a
// low-latency mechanism to retrieve it without reading messages.
type MessageStats struct {
	// MessageCount is the number of messages in the offset range.
	MessageCount int64
	// MessageBytes is the estimated byte count of messages in the range
	// (MessageCount * avgBytesPerMessage).
	MessageBytes int64
}

// ComputeMessageStats returns approximate stats for the offset range
// [startOffset, endOffset). Uses a default estimate of 1 KB per message for
// byte sizing; pass a non-zero avgBytesPerMessage to override.
func (c *KafkaTopicStatsClient) ComputeMessageStats(_ context.Context, topic string, partition int32, startOffset, endOffset int64, avgBytesPerMessage int64) (*MessageStats, error) {
	_ = topic
	_ = partition
	if endOffset < startOffset {
		return nil, fmt.Errorf("gmk: endOffset %d < startOffset %d", endOffset, startOffset)
	}
	if avgBytesPerMessage <= 0 {
		avgBytesPerMessage = defaultAvgMessageBytes
	}
	count := endOffset - startOffset
	return &MessageStats{
		MessageCount: count,
		MessageBytes: count * avgBytesPerMessage,
	}, nil
}

// BacklogInfo reports how far behind head a consumer group is on a given
// partition.
type BacklogInfo struct {
	// MessageCount is the number of messages between the committed offset and
	// the latest (head) offset.
	MessageCount int64
	// ByteCount is the estimated byte size of that backlog.
	ByteCount int64
}

// ComputeBacklogBytes returns backlog stats for a consumer group on a
// partition. If avgBytesPerMessage <= 0, a 1 KB default is used.
func (c *KafkaTopicStatsClient) ComputeBacklogBytes(_ context.Context, topic string, partition int32, groupID string, avgBytesPerMessage int64) (*BacklogInfo, error) {
	if avgBytesPerMessage <= 0 {
		avgBytesPerMessage = defaultAvgMessageBytes
	}
	latest, err := c.client.GetOffset(topic, partition, sarama.OffsetNewest)
	if err != nil {
		return nil, fmt.Errorf("gmk: latest offset: %w", err)
	}
	resp, err := c.admin.ListConsumerGroupOffsets(groupID, map[string][]int32{topic: {partition}})
	if err != nil {
		return nil, fmt.Errorf("gmk: list consumer group offsets: %w", err)
	}
	var committed int64
	if block, ok := resp.Blocks[topic][partition]; ok && block.Offset >= 0 {
		committed = block.Offset
	} else {
		committed = 0
	}
	count := latest - committed
	if count < 0 {
		count = 0
	}
	return &BacklogInfo{
		MessageCount: count,
		ByteCount:    count * avgBytesPerMessage,
	}, nil
}

// ComputeCursorForPublishTime returns the earliest offset whose record
// timestamp is >= t. Equivalent to GetOffsetForTimestamp with a time.Time
// parameter for caller ergonomics.
func (c *KafkaTopicStatsClient) ComputeCursorForPublishTime(ctx context.Context, topic string, partition int32, t time.Time) (int64, bool, error) {
	return c.GetOffsetForTimestamp(ctx, topic, partition, t.UnixMilli())
}

// ComputeCursorForEventTime aliases to ComputeCursorForPublishTime since
// Kafka lacks a native event-time primitive. All record timestamps are
// treated as publish time.
func (c *KafkaTopicStatsClient) ComputeCursorForEventTime(ctx context.Context, topic string, partition int32, t time.Time) (int64, bool, error) {
	return c.ComputeCursorForPublishTime(ctx, topic, partition, t)
}
