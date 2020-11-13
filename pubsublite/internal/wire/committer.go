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
	"errors"
	"fmt"
	"reflect"
	"time"

	"google.golang.org/grpc"

	vkit "cloud.google.com/go/pubsublite/apiv1"
	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

var (
	errInvalidInitialCommitResponse = errors.New("pubsublite: first response from server was not an initial response for streaming commit")
	errInvalidCommitResponse        = errors.New("pubsublite: received invalid commit response from server")
)

// The frequency of batched cursor commits.
const commitCursorPeriod = 50 * time.Millisecond

// committer wraps a commit cursor stream for a subscription and partition.
// A background task periodically effectively reads the latest desired cursor
// offset from the `ackTracker` and sends a commit request to the stream if the
// cursor needs to be updated. The `commitCursorTracker` is used to manage
// in-flight commit requests.
type committer struct {
	// Immutable after creation.
	cursorClient *vkit.CursorClient
	initialReq   *pb.StreamingCommitCursorRequest

	// Fields below must be guarded with mutex.
	stream        *retryableStream
	acks          *ackTracker
	cursorTracker *commitCursorTracker
	pollCommits   *periodicTask

	abstractService
}

func newCommitter(ctx context.Context, cursor *vkit.CursorClient, settings ReceiveSettings,
	subscription subscriptionPartition, acks *ackTracker, disableTasks bool) *committer {

	c := &committer{
		cursorClient: cursor,
		initialReq: &pb.StreamingCommitCursorRequest{
			Request: &pb.StreamingCommitCursorRequest_Initial{
				Initial: &pb.InitialCommitCursorRequest{
					Subscription: subscription.Path,
					Partition:    int64(subscription.Partition),
				},
			},
		},
		acks:          acks,
		cursorTracker: newCommitCursorTracker(acks),
	}
	c.stream = newRetryableStream(ctx, c, settings.Timeout, reflect.TypeOf(pb.StreamingCommitCursorResponse{}))

	backgroundTask := c.commitOffsetToStream
	if disableTasks {
		backgroundTask = func() {}
	}
	c.pollCommits = newPeriodicTask(commitCursorPeriod, backgroundTask)
	return c
}

// Start attempts to establish a streaming commit cursor connection.
func (c *committer) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.unsafeUpdateStatus(serviceStarting, nil) {
		c.stream.Start()
		c.pollCommits.Start()
	}
}

// Stop initiates shutdown of the committer. The commit stream remains open to
// process all outstanding acks and send the final commit offset.
func (c *committer) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.unsafeInitiateShutdown(serviceTerminating, nil)
}

func (c *committer) newStream(ctx context.Context) (grpc.ClientStream, error) {
	return c.cursorClient.StreamingCommitCursor(ctx)
}

func (c *committer) initialRequest() (req interface{}, needsResp bool) {
	req = c.initialReq
	needsResp = true
	return
}

func (c *committer) validateInitialResponse(response interface{}) error {
	commitResponse, _ := response.(*pb.StreamingCommitCursorResponse)
	if commitResponse.GetInitial() == nil {
		return errInvalidInitialCommitResponse
	}
	return nil
}

func (c *committer) onStreamStatusChange(status streamStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch status {
	case streamConnected:
		c.unsafeUpdateStatus(serviceActive, nil)
		// Once the stream connects, clear unconfirmed commits and immediately send
		// the latest desired commit offset.
		c.cursorTracker.ClearPending()
		c.unsafeCommitOffsetToStream()
		c.pollCommits.Start()

	case streamReconnecting:
		c.pollCommits.Stop()

	case streamTerminated:
		c.unsafeInitiateShutdown(serviceTerminated, c.stream.Error())
	}
}

func (c *committer) onResponse(response interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If an inconsistency is detected in the server's responses, immediately
	// terminate the committer, as correct processing of commits cannot be
	// guaranteed.
	processResponse := func() error {
		commitResponse, _ := response.(*pb.StreamingCommitCursorResponse)
		if commitResponse.GetCommit() == nil {
			return errInvalidCommitResponse
		}
		numAcked := commitResponse.GetCommit().GetAcknowledgedCommits()
		if numAcked <= 0 {
			return fmt.Errorf("pubsublite: server acknowledged an invalid commit count: %d", numAcked)
		}
		if err := c.cursorTracker.ConfirmOffsets(numAcked); err != nil {
			return err
		}
		c.unsafeCheckDone()
		return nil
	}
	if err := processResponse(); err != nil {
		c.unsafeInitiateShutdown(serviceTerminated, err)
	}
}

// commitOffsetToStream is called by the periodic background task.
func (c *committer) commitOffsetToStream() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.unsafeCommitOffsetToStream()
}

func (c *committer) unsafeCommitOffsetToStream() {
	nextOffset := c.cursorTracker.NextOffset()
	if nextOffset == nilCursorOffset {
		return
	}

	req := &pb.StreamingCommitCursorRequest{
		Request: &pb.StreamingCommitCursorRequest_Commit{
			Commit: &pb.SequencedCommitCursorRequest{
				Cursor: &pb.Cursor{Offset: nextOffset},
			},
		},
	}
	if c.stream.Send(req) {
		c.cursorTracker.AddPending(nextOffset)
	}
}

func (c *committer) unsafeInitiateShutdown(targetStatus serviceStatus, err error) {
	if !c.unsafeUpdateStatus(targetStatus, err) {
		return
	}

	// If it's a graceful shutdown, expedite sending final commits to the stream.
	if targetStatus == serviceTerminating {
		c.unsafeCommitOffsetToStream()
		c.unsafeCheckDone()
		return
	}
	// Otherwise immediately terminate the stream.
	c.unsafeTerminate()
}

func (c *committer) unsafeCheckDone() {
	// If the user stops the subscriber, they will no longer receive messages, but
	// the commit stream remains open to process acks for outstanding messages.
	if c.status == serviceTerminating && c.cursorTracker.Done() && c.acks.Empty() {
		c.unsafeTerminate()
	}
}

func (c *committer) unsafeTerminate() {
	c.acks.Release()
	c.pollCommits.Stop()
	c.stream.Stop()
}
