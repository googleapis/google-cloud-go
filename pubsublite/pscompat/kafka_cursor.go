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

	"github.com/IBM/sarama"
)

// KafkaCursorClientConfig configures a KafkaCursorClient.
type KafkaCursorClientConfig struct {
	BootstrapServers string
	SaramaConfig     *sarama.Config
}

// KafkaCursorClient manages consumer group offsets on Managed Kafka. It
// provides PSL CursorClient-equivalent operations (commit, read, reset, seek
// to end), mapping PSL "subscription" to a Kafka consumer group ID.
//
// Consumers must not be actively joined to the group when calling Reset*
// methods; Kafka disallows external offset commits on groups with live
// members. For typical migration flows this is not a concern because the
// cutover sequence terminates PSL subscribers before invoking the migration
// tooling.
type KafkaCursorClient struct {
	client sarama.Client
	admin  sarama.ClusterAdmin
}

// NewKafkaCursorClient creates a cursor client connected to Managed Kafka.
func NewKafkaCursorClient(ctx context.Context, cfg *KafkaCursorClientConfig) (*KafkaCursorClient, error) {
	if cfg == nil || cfg.BootstrapServers == "" {
		return nil, fmt.Errorf("gmk: KafkaCursorClientConfig.BootstrapServers must not be empty")
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
	return &KafkaCursorClient{client: client, admin: admin}, nil
}

// Close releases the underlying connection.
func (c *KafkaCursorClient) Close() error {
	return c.admin.Close()
}

// CommitOffset commits a single partition's offset for the given consumer
// group.
func (c *KafkaCursorClient) CommitOffset(_ context.Context, groupID, topic string, partition int32, offset int64) error {
	return c.commitOffsetsInternal(groupID, map[string]map[int32]int64{topic: {partition: offset}})
}

// ResetOffsets commits the provided partition→offset map for the given group
// and topic in a single OffsetCommit RPC.
func (c *KafkaCursorClient) ResetOffsets(_ context.Context, groupID, topic string, offsets map[int32]int64) error {
	if len(offsets) == 0 {
		return nil
	}
	return c.commitOffsetsInternal(groupID, map[string]map[int32]int64{topic: offsets})
}

// ReadCommittedOffsets returns the currently committed offsets for the group
// on the given topic. Partitions with no committed offset are omitted from the
// result.
func (c *KafkaCursorClient) ReadCommittedOffsets(_ context.Context, groupID, topic string) (map[int32]int64, error) {
	// Discover partitions for the topic so we can query them explicitly.
	// Passing nil partitions to ListConsumerGroupOffsets returns an empty
	// response rather than "all partitions".
	partitions, err := c.client.Partitions(topic)
	if err != nil {
		return nil, fmt.Errorf("gmk: list partitions: %w", err)
	}
	resp, err := c.admin.ListConsumerGroupOffsets(groupID, map[string][]int32{topic: partitions})
	if err != nil {
		return nil, err
	}
	out := make(map[int32]int64)
	for p, block := range resp.Blocks[topic] {
		if block.Offset >= 0 {
			out[p] = block.Offset
		}
	}
	return out, nil
}

// GetCommittedOffset returns the committed offset for a single partition.
// Returns -1 (no offset) if the group has never committed on that partition.
func (c *KafkaCursorClient) GetCommittedOffset(_ context.Context, groupID, topic string, partition int32) (int64, error) {
	resp, err := c.admin.ListConsumerGroupOffsets(groupID, map[string][]int32{topic: {partition}})
	if err != nil {
		return -1, err
	}
	if block, ok := resp.Blocks[topic][partition]; ok {
		return block.Offset, nil
	}
	return -1, nil
}

// SeekToEnd resets the consumer group's committed offset to the latest offset
// on each of the given partitions. Returns the offsets that were committed.
func (c *KafkaCursorClient) SeekToEnd(ctx context.Context, groupID, topic string, partitionCount int) (map[int32]int64, error) {
	offsets := make(map[int32]int64, partitionCount)
	for p := int32(0); p < int32(partitionCount); p++ {
		off, err := c.client.GetOffset(topic, p, sarama.OffsetNewest)
		if err != nil {
			return nil, fmt.Errorf("gmk: latest offset topic=%q partition=%d: %w", topic, p, err)
		}
		offsets[p] = off
	}
	if err := c.ResetOffsets(ctx, groupID, topic, offsets); err != nil {
		return nil, err
	}
	return offsets, nil
}

// commitOffsetsInternal sends an OffsetCommit RPC directly to the group
// coordinator broker. This bypasses sarama.OffsetManager, which is designed
// for active consumers and was observed to silently drop multi-partition
// marks when used outside of a consumer-group session.
func (c *KafkaCursorClient) commitOffsetsInternal(groupID string, topicOffsets map[string]map[int32]int64) error {
	// Refresh the group coordinator location so subsequent retries/lookups are
	// against a live broker.
	if err := c.client.RefreshCoordinator(groupID); err != nil {
		return fmt.Errorf("gmk: refresh coordinator: %w", err)
	}
	coord, err := c.client.Coordinator(groupID)
	if err != nil {
		return fmt.Errorf("gmk: group coordinator: %w", err)
	}

	req := &sarama.OffsetCommitRequest{
		ConsumerGroup: groupID,
		// Version 2+ supports longer retention; version 2 is widely supported
		// and works with the offsets topic retention default.
		Version:                 2,
		ConsumerGroupGeneration: sarama.GroupGenerationUndefined,
		ConsumerID:              "",
		RetentionTime:           -1,
	}
	for topic, offs := range topicOffsets {
		for partition, offset := range offs {
			req.AddBlock(topic, partition, offset, 0, "")
		}
	}

	resp, err := coord.CommitOffset(req)
	if err != nil {
		return fmt.Errorf("gmk: offset commit rpc: %w", err)
	}
	for topic, parts := range resp.Errors {
		for partition, kafkaErr := range parts {
			if kafkaErr != sarama.ErrNoError {
				return fmt.Errorf("gmk: offset commit topic=%q partition=%d: %w", topic, partition, kafkaErr)
			}
		}
	}
	return nil
}
