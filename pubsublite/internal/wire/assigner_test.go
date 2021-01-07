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
	"sort"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/pubsublite/internal/test"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

func TestPartitionSet(t *testing.T) {
	partitions := newPartitionSet(&pb.PartitionAssignment{
		Partitions: []int64{8, 5, 8, 1},
	})

	wantPartitions := []int{1, 5, 8}
	for _, partition := range wantPartitions {
		if !partitions.Contains(partition) {
			t.Errorf("Contains(%d) got false, want true", partition)
		}
	}
	for _, partition := range []int{2, 3, 4, 6, 7} {
		if partitions.Contains(partition) {
			t.Errorf("Contains(%d) got true, want false", partition)
		}
	}

	gotPartitions := partitions.Ints()
	sort.Ints(gotPartitions)
	if !testutil.Equal(gotPartitions, wantPartitions) {
		t.Errorf("Ints() got %v, want %v", gotPartitions, wantPartitions)
	}
}

var fakeUUID = [16]byte{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '0', '1', '2', '3', '4', '5'}

func fakeGenerateUUID() (uuid.UUID, error) {
	return fakeUUID, nil
}

// testAssigner wraps an assigner for ease of testing.
type testAssigner struct {
	// Fake error to simulate receiver unable to handle assignment.
	recvError error
	mu        sync.Mutex

	t          *testing.T
	asn        *assigner
	partitions chan []int

	serviceTestProxy
}

func newTestAssigner(t *testing.T, subscription string) *testAssigner {
	ctx := context.Background()
	assignmentClient, err := newPartitionAssignmentClient(ctx, "ignored", testClientOpts...)
	if err != nil {
		t.Fatal(err)
	}

	ta := &testAssigner{
		t:          t,
		partitions: make(chan []int, 1),
	}
	asn, err := newAssigner(ctx, assignmentClient, fakeGenerateUUID, testReceiveSettings(), subscription, ta.receiveAssignment)
	if err != nil {
		t.Fatal(err)
	}
	ta.asn = asn
	ta.initAndStart(t, ta.asn, "Assigner")
	return ta
}

func (ta *testAssigner) receiveAssignment(partitions partitionSet) error {
	p := partitions.Ints()
	sort.Ints(p)
	ta.partitions <- p

	ta.mu.Lock()
	defer ta.mu.Unlock()
	if ta.recvError != nil {
		return ta.recvError
	}
	return nil
}

func (ta *testAssigner) SetReceiveError(err error) {
	ta.mu.Lock()
	defer ta.mu.Unlock()
	ta.recvError = err
}

func (ta *testAssigner) NextPartitions() []int {
	select {
	case <-time.After(serviceTestWaitTimeout):
		ta.t.Errorf("%s partitions not received within %v", ta.name, serviceTestWaitTimeout)
		return nil
	case p := <-ta.partitions:
		return p
	}
}

func TestAssignerNoInitialResponse(t *testing.T) {
	subscription := "projects/123456/locations/us-central1-b/subscriptions/my-subs"

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	barrier := stream.PushWithBarrier(initAssignmentReq(subscription, fakeUUID[:]), nil, nil)
	verifiers.AddAssignmentStream(subscription, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	asn := newTestAssigner(t, subscription)

	// Assigner starts even though no initial response was received from the
	// server.
	if gotErr := asn.StartError(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}
	// To ensure test is deterministic, i.e. server must receive initial request
	// before stopping the client.
	barrier.Release()
	asn.StopVerifyNoError()
}

func TestAssignerReconnect(t *testing.T) {
	subscription := "projects/123456/locations/us-central1-b/subscriptions/my-subs"
	permanentErr := status.Error(codes.FailedPrecondition, "failed")

	verifiers := test.NewVerifiers(t)

	// Simulate a transient error that results in a reconnect.
	stream1 := test.NewRPCVerifier(t)
	stream1.Push(initAssignmentReq(subscription, fakeUUID[:]), nil, status.Error(codes.Unavailable, "server unavailable"))
	verifiers.AddAssignmentStream(subscription, stream1)

	// Send 2 partition assignments before terminating with permanent error.
	stream2 := test.NewRPCVerifier(t)
	stream2.Push(initAssignmentReq(subscription, fakeUUID[:]), assignmentResp([]int64{3, 2, 4}), nil)
	stream2.Push(assignmentAckReq(), assignmentResp([]int64{0, 3, 3}), nil)
	stream2.Push(assignmentAckReq(), nil, permanentErr)
	verifiers.AddAssignmentStream(subscription, stream2)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	asn := newTestAssigner(t, subscription)

	if gotErr := asn.StartError(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}
	if got, want := asn.NextPartitions(), []int{2, 3, 4}; !testutil.Equal(got, want) {
		t.Errorf("Partition assignment #1: got %v, want %v", got, want)
	}
	if got, want := asn.NextPartitions(), []int{0, 3}; !testutil.Equal(got, want) {
		t.Errorf("Partition assignment #2: got %v, want %v", got, want)
	}
	if gotErr, wantErr := asn.FinalError(), permanentErr; !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("Final err: (%v), want: (%v)", gotErr, wantErr)
	}
}

func TestAssignerHandlePartitionFailure(t *testing.T) {
	subscription := "projects/123456/locations/us-central1-b/subscriptions/my-subs"

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initAssignmentReq(subscription, fakeUUID[:]), assignmentResp([]int64{1, 2}), nil)
	verifiers.AddAssignmentStream(subscription, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	asn := newTestAssigner(t, subscription)
	// Simulates the assigningSubscriber discarding assignments.
	wantErr := errors.New("subscriber shutting down")
	asn.SetReceiveError(wantErr)

	if gotErr := asn.StartError(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}
	if got, want := asn.NextPartitions(), []int{1, 2}; !testutil.Equal(got, want) {
		t.Errorf("Partition assignments: got %v, want %v", got, want)
	}
	if gotErr := asn.FinalError(); !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("Final err: (%v), want: (%v)", gotErr, wantErr)
	}
}
