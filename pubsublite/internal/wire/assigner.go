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
	"sort"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	vkit "cloud.google.com/go/pubsublite/apiv1"
	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

// PartitionSet is a set of partition numbers.
type PartitionSet map[int]struct{}

// NewPartitionSet creates a partition set initialized from the given partition
// numbers.
func NewPartitionSet(partitions []int) PartitionSet {
	var void struct{}
	partitionSet := make(map[int]struct{})
	for _, p := range partitions {
		partitionSet[p] = void
	}
	return partitionSet
}

func newPartitionSet(assignmentpb *pb.PartitionAssignment) PartitionSet {
	var partitions []int
	for _, p := range assignmentpb.GetPartitions() {
		partitions = append(partitions, int(p))
	}
	return NewPartitionSet(partitions)
}

// Ints returns the partitions contained in this set as an unsorted slice.
func (ps PartitionSet) Ints() (partitions []int) {
	for p := range ps {
		partitions = append(partitions, p)
	}
	return
}

// SortedInts returns the partitions contained in this set as a sorted slice.
func (ps PartitionSet) SortedInts() (partitions []int) {
	partitions = ps.Ints()
	sort.Ints(partitions)
	return
}

// Contains returns true if this set contains the specified partition.
func (ps PartitionSet) Contains(partition int) bool {
	_, exists := ps[partition]
	return exists
}

// A function that generates a 16-byte UUID.
type generateUUIDFunc func() (uuid.UUID, error)

// partitionAssignmentReceiver must enact the received partition assignment from
// the server, or otherwise return an error, which will break the stream.
type partitionAssignmentReceiver func(PartitionSet) error

// assigner wraps the partition assignment stream and notifies a receiver when
// the server sends a new set of partition assignments for a subscriber.
type assigner struct {
	// Immutable after creation.
	assignmentClient  *vkit.PartitionAssignmentClient
	subscription      string
	initialReq        *pb.PartitionAssignmentRequest
	receiveAssignment partitionAssignmentReceiver
	metadata          pubsubMetadata

	// Fields below must be guarded with mu.
	stream *retryableStream

	abstractService
}

func newAssigner(ctx context.Context, assignmentClient *vkit.PartitionAssignmentClient, genUUID generateUUIDFunc, settings ReceiveSettings, subscriptionPath string, receiver partitionAssignmentReceiver) (*assigner, error) {
	clientID, err := genUUID()
	if err != nil {
		return nil, fmt.Errorf("pubsublite: failed to generate client UUID: %v", err)
	}

	a := &assigner{
		assignmentClient: assignmentClient,
		subscription:     subscriptionPath,
		initialReq: &pb.PartitionAssignmentRequest{
			Request: &pb.PartitionAssignmentRequest_Initial{
				Initial: &pb.InitialPartitionAssignmentRequest{
					Subscription: subscriptionPath,
					ClientId:     clientID[:],
				},
			},
		},
		receiveAssignment: receiver,
		metadata:          newPubsubMetadata(),
	}
	a.stream = newRetryableStream(ctx, a, settings.Timeout, 10*time.Minute, reflect.TypeOf(pb.PartitionAssignment{}))
	a.metadata.AddClientInfo(settings.Framework)
	return a, nil
}

func (a *assigner) Start() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.unsafeUpdateStatus(serviceStarting, nil) {
		a.stream.Start()
	}
}

func (a *assigner) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.unsafeInitiateShutdown(serviceTerminating, nil)
}

func (a *assigner) newStream(ctx context.Context) (grpc.ClientStream, error) {
	return a.assignmentClient.AssignPartitions(a.metadata.AddToContext(ctx))
}

func (a *assigner) initialRequest() (interface{}, initialResponseRequired) {
	return a.initialReq, initialResponseRequired(false)
}

func (a *assigner) validateInitialResponse(_ interface{}) error {
	// Should not be called as initialResponseRequired=false above.
	return errors.New("pubsublite: unexpected initial response")
}

func (a *assigner) onStreamStatusChange(status streamStatus) {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch status {
	case streamConnected:
		a.unsafeUpdateStatus(serviceActive, nil)
	case streamTerminated:
		a.unsafeInitiateShutdown(serviceTerminated, a.stream.Error())
	}
}

func (a *assigner) onResponse(response interface{}) {
	assignment, _ := response.(*pb.PartitionAssignment)
	err := a.receiveAssignment(newPartitionSet(assignment))

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.status >= serviceTerminating {
		return
	}
	if err != nil {
		a.unsafeInitiateShutdown(serviceTerminated, err)
		return
	}
	a.stream.Send(&pb.PartitionAssignmentRequest{
		Request: &pb.PartitionAssignmentRequest_Ack{
			Ack: &pb.PartitionAssignmentAck{},
		},
	})
}

func (a *assigner) unsafeInitiateShutdown(targetStatus serviceStatus, err error) {
	if !a.unsafeUpdateStatus(targetStatus, wrapError("assigner", a.subscription, err)) {
		return
	}
	// No data to send. Immediately terminate the stream.
	a.stream.Stop()
}
