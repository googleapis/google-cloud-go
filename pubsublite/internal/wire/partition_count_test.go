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
	"testing"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/pubsublite/internal/test"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type testPartitionCountWatcher struct {
	t                  *testing.T
	watcher            *partitionCountWatcher
	gotPartitionCounts []int

	serviceTestProxy
}

func (tw *testPartitionCountWatcher) onCountChanged(partitionCount int) {
	tw.gotPartitionCounts = append(tw.gotPartitionCounts, partitionCount)
}

func (tw *testPartitionCountWatcher) VerifyCounts(want []int) {
	if !testutil.Equal(tw.gotPartitionCounts, want) {
		tw.t.Errorf("partition counts: got %v, want %v", tw.gotPartitionCounts, want)
	}
}

func (tw *testPartitionCountWatcher) UpdatePartitionCount() {
	tw.watcher.updatePartitionCount()
}

func newTestPartitionCountWatcher(t *testing.T, topicPath string, settings PublishSettings) *testPartitionCountWatcher {
	ctx := context.Background()
	adminClient, err := NewAdminClient(ctx, "ignored", testClientOpts...)
	if err != nil {
		t.Fatal(err)
	}
	tw := &testPartitionCountWatcher{
		t: t,
	}
	tw.watcher = newPartitionCountWatcher(ctx, adminClient, testPublishSettings(), topicPath, tw.onCountChanged)
	tw.initAndStart(t, tw.watcher, "PartitionCountWatcher")
	return tw
}

func TestPartitionCountWatcherRetries(t *testing.T) {
	const topic = "projects/123456/locations/us-central1-b/topics/my-topic"
	wantPartitionCount := 2

	verifiers := test.NewVerifiers(t)
	verifiers.GlobalVerifier.Push(topicPartitionsReq(topic), nil, status.Error(codes.Unavailable, "retryable"))
	verifiers.GlobalVerifier.Push(topicPartitionsReq(topic), nil, status.Error(codes.ResourceExhausted, "retryable"))
	verifiers.GlobalVerifier.Push(topicPartitionsReq(topic), topicPartitionsResp(wantPartitionCount), nil)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	watcher := newTestPartitionCountWatcher(t, topic, testPublishSettings())
	if gotErr := watcher.StartError(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}
	watcher.VerifyCounts([]int{wantPartitionCount})
	watcher.StopVerifyNoError()
}

func TestPartitionCountWatcherZeroPartitionCountFails(t *testing.T) {
	const topic = "projects/123456/locations/us-central1-b/topics/my-topic"

	verifiers := test.NewVerifiers(t)
	verifiers.GlobalVerifier.Push(topicPartitionsReq(topic), topicPartitionsResp(0), nil)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	watcher := newTestPartitionCountWatcher(t, topic, testPublishSettings())
	if gotErr, wantMsg := watcher.StartError(), "invalid number of partitions 0"; !test.ErrorHasMsg(gotErr, wantMsg) {
		t.Errorf("Start() got err: (%v), want msg: (%q)", gotErr, wantMsg)
	}
	watcher.VerifyCounts(nil)
}

func TestPartitionCountWatcherPartitionCountUnchanged(t *testing.T) {
	const topic = "projects/123456/locations/us-central1-b/topics/my-topic"
	wantPartitionCount1 := 4
	wantPartitionCount2 := 6

	verifiers := test.NewVerifiers(t)
	verifiers.GlobalVerifier.Push(topicPartitionsReq(topic), topicPartitionsResp(wantPartitionCount1), nil)
	verifiers.GlobalVerifier.Push(topicPartitionsReq(topic), topicPartitionsResp(wantPartitionCount1), nil)
	verifiers.GlobalVerifier.Push(topicPartitionsReq(topic), topicPartitionsResp(wantPartitionCount2), nil)
	verifiers.GlobalVerifier.Push(topicPartitionsReq(topic), topicPartitionsResp(wantPartitionCount2), nil)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	watcher := newTestPartitionCountWatcher(t, topic, testPublishSettings())
	if gotErr := watcher.StartError(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}
	watcher.VerifyCounts([]int{wantPartitionCount1}) // Initial count

	// Simulate 3 background updates.
	watcher.UpdatePartitionCount()
	watcher.UpdatePartitionCount()
	watcher.UpdatePartitionCount()
	watcher.VerifyCounts([]int{wantPartitionCount1, wantPartitionCount2})
	watcher.StopVerifyNoError()
}
