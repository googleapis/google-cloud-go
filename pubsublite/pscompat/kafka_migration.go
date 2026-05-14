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
	"log"
	"sort"
	"strings"

	"google.golang.org/api/iterator"

	vkit "cloud.google.com/go/pubsublite/apiv1"
	pb "cloud.google.com/go/pubsublite/apiv1/pubsublitepb"
)

// MigrationStatus enumerates the outcome of migrating a single partition.
type MigrationStatus int

const (
	// MigrationSuccess indicates the offset was resolved and committed.
	MigrationSuccess MigrationStatus = iota
	// MigrationDryRun indicates the offset was resolved but not committed.
	MigrationDryRun
	// MigrationNoPSLOffset indicates no PSL cursor existed; reset to earliest
	// Kafka offset instead.
	MigrationNoPSLOffset
	// MigrationNoKafkaOffset indicates no Kafka offset at/after the target
	// timestamp.
	MigrationNoKafkaOffset
	// MigrationValidationFailed indicates post-commit validation saw an offset
	// different from what was committed.
	MigrationValidationFailed
	// MigrationError indicates execution failure. Inspect Message for details.
	MigrationError
)

// String returns the uppercased status name (matches Java enum names).
func (s MigrationStatus) String() string {
	switch s {
	case MigrationSuccess:
		return "SUCCESS"
	case MigrationDryRun:
		return "DRY_RUN"
	case MigrationNoPSLOffset:
		return "NO_PSL_OFFSET"
	case MigrationNoKafkaOffset:
		return "NO_KAFKA_OFFSET"
	case MigrationValidationFailed:
		return "VALIDATION_FAILED"
	case MigrationError:
		return "ERROR"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", int(s))
	}
}

// MigrationResult records the outcome of migrating one partition's offset.
type MigrationResult struct {
	Status      MigrationStatus
	PSLOffset   int64
	KafkaOffset int64 // -1 if unknown
	Message     string
	Validated   bool
}

// String renders the result in the same shape as the Java toString().
func (r MigrationResult) String() string {
	return fmt.Sprintf("MigrationResult{status=%s, pslOffset=%d, kafkaOffset=%d, validated=%t, message=%q}",
		r.Status, r.PSLOffset, r.KafkaOffset, r.Validated, r.Message)
}

// MigrationConfig captures the clients and identifiers needed to perform an
// offset migration from PSL to Managed Kafka.
type MigrationConfig struct {
	// PSL side (source).
	PSLCursorClient     *vkit.CursorClient
	PSLTopicStatsClient *vkit.TopicStatsClient
	PSLTopicPath        string // "projects/P/locations/L/topics/T"
	PSLSubscriptionPath string // "projects/P/locations/L/subscriptions/S"

	// Kafka side (destination).
	KafkaTopicStats *KafkaTopicStatsClient
	KafkaCursor     *KafkaCursorClient
	KafkaTopicName  string
	KafkaGroupID    string // consumer group (PSL subscription equivalent)
	Partitions      []int32

	// DryRun resolves offsets but skips the commit.
	DryRun bool
	// Validate re-reads committed offsets after commit to confirm accuracy.
	Validate bool
}

// MigrateOffsets translates the PSL committed cursor for each partition to a
// Kafka offset via publish-time timestamps and commits the resolved offsets
// to the Kafka consumer group in a single batch. The implementation mirrors
// Java's OffsetMigrationHelper and runs in five steps:
//   1. List PSL committed offsets for the subscription.
//   2. For each partition, look up the message at that PSL offset and read its
//      publish timestamp.
//   3. Resolve the corresponding Kafka offset by timestamp lookup on the
//      Kafka topic.
//   4. Commit the resolved offsets to the Kafka consumer group in one batch.
//   5. Optionally re-read the committed offsets to confirm.
func MigrateOffsets(ctx context.Context, cfg *MigrationConfig) (map[int32]MigrationResult, error) {
	if err := validateMigrationConfig(cfg); err != nil {
		return nil, err
	}

	results := make(map[int32]MigrationResult)

	// Step 1: read all PSL committed offsets for the subscription.
	pslOffsets, err := listPSLOffsets(ctx, cfg.PSLCursorClient, cfg.PSLSubscriptionPath)
	if err != nil {
		return nil, fmt.Errorf("gmk: list PSL cursors: %w", err)
	}
	log.Printf("gmk migration: PSL committed offsets: %v", pslOffsets)

	kafkaOffsetsToReset := make(map[int32]int64)

	for _, partition := range cfg.Partitions {
		pslOffset, ok := pslOffsets[partition]
		if !ok || pslOffset == 0 {
			earliest, err := cfg.KafkaTopicStats.GetEarliestOffset(ctx, cfg.KafkaTopicName, partition)
			if err != nil {
				results[partition] = errorResult(partition, err)
				continue
			}
			kafkaOffsetsToReset[partition] = earliest
			results[partition] = MigrationResult{
				Status:      pickNoPSLStatus(cfg.DryRun),
				KafkaOffset: earliest,
				Message:     fmt.Sprintf("no PSL committed offset; reset to earliest Kafka offset %d", earliest),
			}
			continue
		}

		// Step 2: resolve publish_time for the PSL offset via ComputeMessageStats.
		// We query a 1-offset range ending at pslOffset; the response's
		// MinimumPublishTime is our lookup key.
		rangeStart := pslOffset - 1
		if rangeStart < 0 {
			rangeStart = 0
		}
		stats, err := cfg.PSLTopicStatsClient.ComputeMessageStats(ctx, &pb.ComputeMessageStatsRequest{
			Topic:       cfg.PSLTopicPath,
			Partition:   int64(partition),
			StartCursor: &pb.Cursor{Offset: rangeStart},
			EndCursor:   &pb.Cursor{Offset: pslOffset},
		})
		if err != nil {
			results[partition] = errorResult(partition, fmt.Errorf("compute message stats: %w", err))
			continue
		}
		pub := stats.GetMinimumPublishTime()
		if pub == nil || (pub.GetSeconds() == 0 && pub.GetNanos() == 0) {
			// Fallback: earliest Kafka offset.
			earliest, err := cfg.KafkaTopicStats.GetEarliestOffset(ctx, cfg.KafkaTopicName, partition)
			if err != nil {
				results[partition] = errorResult(partition, err)
				continue
			}
			kafkaOffsetsToReset[partition] = earliest
			results[partition] = MigrationResult{
				Status:      pickNoPSLStatus(cfg.DryRun),
				PSLOffset:   pslOffset,
				KafkaOffset: earliest,
				Message:     fmt.Sprintf("could not determine publish_time for PSL offset %d; reset to earliest Kafka offset %d", pslOffset, earliest),
			}
			continue
		}
		timestampMs := pub.GetSeconds()*1000 + int64(pub.GetNanos())/1_000_000

		// Step 3: look up Kafka offset >= timestamp.
		kOff, found, err := cfg.KafkaTopicStats.GetOffsetForTimestamp(ctx, cfg.KafkaTopicName, partition, timestampMs)
		if err != nil {
			results[partition] = errorResult(partition, fmt.Errorf("offset for timestamp: %w", err))
			continue
		}
		if !found {
			latest, err := cfg.KafkaTopicStats.GetLatestOffset(ctx, cfg.KafkaTopicName, partition)
			if err != nil {
				results[partition] = errorResult(partition, err)
				continue
			}
			kafkaOffsetsToReset[partition] = latest
			results[partition] = MigrationResult{
				Status:      MigrationNoKafkaOffset,
				PSLOffset:   pslOffset,
				KafkaOffset: -1,
				Message:     fmt.Sprintf("no Kafka offset at/after timestamp %dms; would reset to latest %d", timestampMs, latest),
			}
			continue
		}

		kafkaOffsetsToReset[partition] = kOff
		status := MigrationSuccess
		msg := fmt.Sprintf("migrated PSL offset %d -> Kafka offset %d", pslOffset, kOff)
		if cfg.DryRun {
			status = MigrationDryRun
			msg = fmt.Sprintf("DRY RUN: would migrate PSL offset %d -> Kafka offset %d", pslOffset, kOff)
		}
		results[partition] = MigrationResult{
			Status:      status,
			PSLOffset:   pslOffset,
			KafkaOffset: kOff,
			Message:     msg,
		}
	}

	if cfg.DryRun {
		log.Printf("gmk migration: DRY RUN; would commit %d offsets", len(kafkaOffsetsToReset))
		return results, nil
	}

	// Step 4: batch commit.
	if len(kafkaOffsetsToReset) > 0 {
		if err := cfg.KafkaCursor.ResetOffsets(ctx, cfg.KafkaGroupID, cfg.KafkaTopicName, kafkaOffsetsToReset); err != nil {
			return nil, fmt.Errorf("gmk: reset Kafka offsets: %w", err)
		}

		// Step 5 (optional): read-back validation.
		if cfg.Validate {
			results = validateCommittedOffsets(ctx, cfg, kafkaOffsetsToReset, results)
		}
	}

	return results, nil
}

func validateCommittedOffsets(ctx context.Context, cfg *MigrationConfig, expected map[int32]int64, current map[int32]MigrationResult) map[int32]MigrationResult {
	committed, err := cfg.KafkaCursor.ReadCommittedOffsets(ctx, cfg.KafkaGroupID, cfg.KafkaTopicName)
	if err != nil {
		log.Printf("gmk migration: post-reset validation failed: %v", err)
		return current
	}
	updated := make(map[int32]MigrationResult, len(current))
	for p, r := range current {
		updated[p] = r
	}
	for p, expOff := range expected {
		orig := current[p]
		actual, ok := committed[p]
		if ok && actual == expOff {
			orig.Status = MigrationSuccess
			orig.Validated = true
			orig.Message = fmt.Sprintf("%s (validated)", orig.Message)
			updated[p] = orig
		} else {
			orig.Status = MigrationValidationFailed
			orig.Message = fmt.Sprintf("validation failed: expected Kafka offset %d but found %d", expOff, actual)
			updated[p] = orig
		}
	}
	return updated
}

func validateMigrationConfig(cfg *MigrationConfig) error {
	if cfg == nil {
		return fmt.Errorf("gmk: MigrationConfig is nil")
	}
	if cfg.PSLCursorClient == nil {
		return fmt.Errorf("gmk: MigrationConfig.PSLCursorClient must not be nil")
	}
	if cfg.PSLTopicStatsClient == nil {
		return fmt.Errorf("gmk: MigrationConfig.PSLTopicStatsClient must not be nil")
	}
	if cfg.KafkaTopicStats == nil {
		return fmt.Errorf("gmk: MigrationConfig.KafkaTopicStats must not be nil")
	}
	if cfg.KafkaCursor == nil {
		return fmt.Errorf("gmk: MigrationConfig.KafkaCursor must not be nil")
	}
	if cfg.PSLTopicPath == "" || cfg.PSLSubscriptionPath == "" {
		return fmt.Errorf("gmk: MigrationConfig PSL path fields must be set")
	}
	if cfg.KafkaTopicName == "" || cfg.KafkaGroupID == "" {
		return fmt.Errorf("gmk: MigrationConfig KafkaTopicName and KafkaGroupID must be set")
	}
	if len(cfg.Partitions) == 0 {
		return fmt.Errorf("gmk: MigrationConfig.Partitions must not be empty")
	}
	return nil
}

func listPSLOffsets(ctx context.Context, c *vkit.CursorClient, subscriptionPath string) (map[int32]int64, error) {
	it := c.ListPartitionCursors(ctx, &pb.ListPartitionCursorsRequest{Parent: subscriptionPath})
	out := make(map[int32]int64)
	for {
		pc, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		if pc == nil || pc.GetCursor() == nil {
			continue
		}
		out[int32(pc.GetPartition())] = pc.GetCursor().GetOffset()
	}
	return out, nil
}

func errorResult(_ int32, err error) MigrationResult {
	return MigrationResult{
		Status:      MigrationError,
		KafkaOffset: -1,
		Message:     err.Error(),
	}
}

func pickNoPSLStatus(dryRun bool) MigrationStatus {
	if dryRun {
		return MigrationDryRun
	}
	return MigrationNoPSLOffset
}

// MigrationOrchestrator coordinates a full migration lifecycle with dry-run
// gating, result summary, and a post-flight backlog check.
type MigrationOrchestrator struct {
	Config *MigrationConfig
}

// MigrationSummary aggregates per-partition results.
type MigrationSummary struct {
	PartitionResults map[int32]MigrationResult
	Successful       bool
	// Summary is a formatted multi-line report suitable for direct logging.
	Summary string
}

// NewMigrationOrchestrator creates a new orchestrator wrapping the given
// config.
func NewMigrationOrchestrator(cfg *MigrationConfig) *MigrationOrchestrator {
	return &MigrationOrchestrator{Config: cfg}
}

// Execute runs the full migration and returns a summary of per-partition
// outcomes.
func (o *MigrationOrchestrator) Execute(ctx context.Context) (*MigrationSummary, error) {
	// Pre-flight: confirm Kafka topic has some data so NO_KAFKA_OFFSET results
	// are informative rather than a sign of a misconfigured pipeline.
	for _, p := range o.Config.Partitions {
		latest, err := o.Config.KafkaTopicStats.GetLatestOffset(ctx, o.Config.KafkaTopicName, p)
		if err != nil {
			return nil, fmt.Errorf("gmk: preflight latest offset partition=%d: %w", p, err)
		}
		earliest, err := o.Config.KafkaTopicStats.GetEarliestOffset(ctx, o.Config.KafkaTopicName, p)
		if err != nil {
			return nil, fmt.Errorf("gmk: preflight earliest offset partition=%d: %w", p, err)
		}
		if latest == 0 && earliest == 0 {
			log.Printf("WARNING: gmk: Kafka partition %d appears empty (earliest=latest=0); NO_KAFKA_OFFSET results may reflect backlog, not lag", p)
		}
	}

	results, err := MigrateOffsets(ctx, o.Config)
	if err != nil {
		return nil, err
	}

	summary := summarizeResults(results, o.Config.Partitions, o.Config.DryRun)
	successful := true
	for _, r := range results {
		switch r.Status {
		case MigrationSuccess, MigrationDryRun, MigrationNoPSLOffset:
			// OK or acceptable outcomes.
		default:
			successful = false
		}
	}

	return &MigrationSummary{
		PartitionResults: results,
		Successful:       successful,
		Summary:          summary,
	}, nil
}

// CheckBacklog reports, per partition, how many messages are between the
// (now-migrated) committed offset and the current head.
func (o *MigrationOrchestrator) CheckBacklog(ctx context.Context) (map[int32]int64, error) {
	backlog := make(map[int32]int64, len(o.Config.Partitions))
	for _, p := range o.Config.Partitions {
		info, err := o.Config.KafkaTopicStats.ComputeBacklogBytes(ctx, o.Config.KafkaTopicName, p, o.Config.KafkaGroupID, 0)
		if err != nil {
			return nil, fmt.Errorf("gmk: backlog partition=%d: %w", p, err)
		}
		backlog[p] = info.MessageCount
	}
	return backlog, nil
}

func summarizeResults(results map[int32]MigrationResult, partitions []int32, dryRun bool) string {
	var sb strings.Builder
	sb.WriteString("=== Migration Summary ")
	if dryRun {
		sb.WriteString("(DRY RUN) ")
	}
	sb.WriteString("===\n")
	parts := append([]int32(nil), partitions...)
	sort.Slice(parts, func(i, j int) bool { return parts[i] < parts[j] })
	for _, p := range parts {
		r := results[p]
		sb.WriteString(fmt.Sprintf("  partition=%d status=%s pslOffset=%d kafkaOffset=%d validated=%t msg=%q\n",
			p, r.Status, r.PSLOffset, r.KafkaOffset, r.Validated, r.Message))
	}
	return sb.String()
}
