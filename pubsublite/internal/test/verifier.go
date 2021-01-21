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

package test

import (
	"container/list"
	"fmt"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// blockWaitTimeout is the timeout for any wait operations to ensure no
	// deadlocks.
	blockWaitTimeout = 30 * time.Second
)

// Barrier is used to perform two-way synchronization betwen the server and
// client (test) to ensure tests are deterministic.
type Barrier struct {
	// Used to block until the server is ready to send the response.
	serverBlock chan struct{}
	// Used to block until the client wants the server to send the response.
	clientBlock chan struct{}
	err         error
}

func newBarrier() *Barrier {
	return &Barrier{
		serverBlock: make(chan struct{}),
		clientBlock: make(chan struct{}),
	}
}

// Release should be called by the test.
func (b *Barrier) Release() {
	// Wait for the server to reach the barrier.
	select {
	case <-time.After(blockWaitTimeout):
		// Note: avoid returning a retryable code to quickly terminate the test.
		b.err = status.Errorf(codes.FailedPrecondition, "mockserver: server did not reach barrier within %v", blockWaitTimeout)
	case <-b.serverBlock:
	}

	// Then close the client block.
	close(b.clientBlock)
}

func (b *Barrier) serverWait() error {
	if b.err != nil {
		return b.err
	}

	// Close the server block to signal the server reaching the point where it is
	// ready to send the response.
	close(b.serverBlock)

	// Wait for the test to release the client block.
	select {
	case <-time.After(blockWaitTimeout):
		// Note: avoid returning a retryable code to quickly terminate the test.
		return status.Errorf(codes.FailedPrecondition, "mockserver: test did not unblock response within %v", blockWaitTimeout)
	case <-b.clientBlock:
		return nil
	}
}

type rpcMetadata struct {
	wantRequest interface{}
	retResponse interface{}
	retErr      error
	barrier     *Barrier
}

// wait until the barrier is released by the test, or a timeout occurs.
// Returns immediately if there was no block.
func (r *rpcMetadata) wait() error {
	if r.barrier == nil {
		return nil
	}
	return r.barrier.serverWait()
}

// RPCVerifier stores an queue of requests expected from the client, and the
// corresponding response or error to return.
type RPCVerifier struct {
	t        *testing.T
	mu       sync.Mutex
	rpcs     *list.List // Value = *rpcMetadata
	numCalls int
}

// NewRPCVerifier creates a new verifier for requests received by the server.
func NewRPCVerifier(t *testing.T) *RPCVerifier {
	return &RPCVerifier{
		t:        t,
		rpcs:     list.New(),
		numCalls: -1,
	}
}

// Push appends a new {request, response, error} tuple.
//
// Valid combinations for unary and streaming RPCs:
// - {request, response, nil}
// - {request, nil, error}
//
// Additional combinations for streams only:
// - {nil, response, nil}: send a response without a request (e.g. messages).
// - {nil, nil, error}: break the stream without a request.
// - {request, nil, nil}: expect a request, but don't send any response.
func (v *RPCVerifier) Push(wantRequest interface{}, retResponse interface{}, retErr error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.rpcs.PushBack(&rpcMetadata{
		wantRequest: wantRequest,
		retResponse: retResponse,
		retErr:      retErr,
	})
}

// PushWithBarrier is like Push, but returns a barrier that the test should call
// Release when it would like the response to be sent to the client. This is
// useful for synchronizing with work that needs to be done on the client.
func (v *RPCVerifier) PushWithBarrier(wantRequest interface{}, retResponse interface{}, retErr error) *Barrier {
	v.mu.Lock()
	defer v.mu.Unlock()

	barrier := newBarrier()
	v.rpcs.PushBack(&rpcMetadata{
		wantRequest: wantRequest,
		retResponse: retResponse,
		retErr:      retErr,
		barrier:     barrier,
	})
	return barrier
}

// Pop validates the received request with the next {request, response, error}
// tuple.
func (v *RPCVerifier) Pop(gotRequest interface{}) (interface{}, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.numCalls++
	elem := v.rpcs.Front()
	if elem == nil {
		v.t.Errorf("call(%d): unexpected request:\n%v", v.numCalls, gotRequest)
		return nil, status.Error(codes.FailedPrecondition, "mockserver: got unexpected request")
	}

	rpc, _ := elem.Value.(*rpcMetadata)
	v.rpcs.Remove(elem)

	if !testutil.Equal(gotRequest, rpc.wantRequest) {
		v.t.Errorf("call(%d): got request: %v\nwant request: %v", v.numCalls, gotRequest, rpc.wantRequest)
	}
	if err := rpc.wait(); err != nil {
		return nil, err
	}
	return rpc.retResponse, rpc.retErr
}

// TryPop should be used only for streams. It checks whether the request in the
// next tuple is nil, in which case the response or error should be returned to
// the client without waiting for a request. Useful for streams where the server
// continuously sends data (e.g. subscribe stream).
func (v *RPCVerifier) TryPop() (bool, interface{}, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	elem := v.rpcs.Front()
	if elem == nil {
		return false, nil, nil
	}

	rpc, _ := elem.Value.(*rpcMetadata)
	if rpc.wantRequest != nil {
		return false, nil, nil
	}

	v.rpcs.Remove(elem)
	if err := rpc.wait(); err != nil {
		return true, nil, err
	}
	return true, rpc.retResponse, rpc.retErr
}

// Flush logs an error for any remaining {request, response, error} tuples, in
// case the client terminated early.
func (v *RPCVerifier) Flush() {
	v.mu.Lock()
	defer v.mu.Unlock()

	for elem := v.rpcs.Front(); elem != nil; elem = elem.Next() {
		v.numCalls++
		rpc, _ := elem.Value.(*rpcMetadata)
		if rpc.wantRequest != nil {
			v.t.Errorf("call(%d): did not receive expected request:\n%v", v.numCalls, rpc.wantRequest)
		} else {
			v.t.Errorf("call(%d): unsent response:\n%v, err = (%v)", v.numCalls, rpc.retResponse, rpc.retErr)
		}
	}
	v.rpcs.Init()
}

// streamVerifiers stores a queue of verifiers for unique stream connections.
type streamVerifiers struct {
	t          *testing.T
	verifiers  *list.List // Value = *RPCVerifier
	numStreams int
}

func newStreamVerifiers(t *testing.T) *streamVerifiers {
	return &streamVerifiers{
		t:          t,
		verifiers:  list.New(),
		numStreams: -1,
	}
}

func (sv *streamVerifiers) Push(v *RPCVerifier) {
	sv.verifiers.PushBack(v)
}

func (sv *streamVerifiers) Pop() (*RPCVerifier, error) {
	sv.numStreams++
	elem := sv.verifiers.Front()
	if elem == nil {
		sv.t.Errorf("stream(%d): unexpected connection with no verifiers", sv.numStreams)
		return nil, status.Error(codes.FailedPrecondition, "mockserver: got unexpected stream connection")
	}

	v, _ := elem.Value.(*RPCVerifier)
	sv.verifiers.Remove(elem)
	return v, nil
}

func (sv *streamVerifiers) Flush() {
	for elem := sv.verifiers.Front(); elem != nil; elem = elem.Next() {
		v, _ := elem.Value.(*RPCVerifier)
		v.Flush()
	}
}

// keyedStreamVerifiers stores indexed streamVerifiers. Examples of keys:
// "streamType:topic_path:partition".
type keyedStreamVerifiers struct {
	verifiers map[string]*streamVerifiers
}

func newKeyedStreamVerifiers() *keyedStreamVerifiers {
	return &keyedStreamVerifiers{verifiers: make(map[string]*streamVerifiers)}
}

func (kv *keyedStreamVerifiers) Push(key string, v *RPCVerifier) {
	sv, ok := kv.verifiers[key]
	if !ok {
		sv = newStreamVerifiers(v.t)
		kv.verifiers[key] = sv
	}
	sv.Push(v)
}

func (kv *keyedStreamVerifiers) Pop(key string) (*RPCVerifier, error) {
	sv, ok := kv.verifiers[key]
	if !ok {
		return nil, status.Error(codes.FailedPrecondition, "mockserver: unexpected connection with no configured responses")
	}
	return sv.Pop()
}

func (kv *keyedStreamVerifiers) Flush() {
	for _, sv := range kv.verifiers {
		sv.Flush()
	}
}

// Verifiers contains RPCVerifiers for unary RPCs and streaming RPCs.
type Verifiers struct {
	t  *testing.T
	mu sync.Mutex

	// Global list of verifiers for all unary RPCs.
	GlobalVerifier *RPCVerifier
	// Stream verifiers by key.
	streamVerifiers       *keyedStreamVerifiers
	activeStreamVerifiers []*RPCVerifier
}

// NewVerifiers creates a new instance of Verifiers for a test.
func NewVerifiers(t *testing.T) *Verifiers {
	return &Verifiers{
		t:               t,
		GlobalVerifier:  NewRPCVerifier(t),
		streamVerifiers: newKeyedStreamVerifiers(),
	}
}

// streamType is used as a key prefix for keyedStreamVerifiers.
type streamType string

const (
	publishStreamType    streamType = "publish"
	subscribeStreamType  streamType = "subscribe"
	commitStreamType     streamType = "commit"
	assignmentStreamType streamType = "assignment"
)

func keyPartition(st streamType, path string, partition int) string {
	return fmt.Sprintf("%s:%s:%d", st, path, partition)
}

func key(st streamType, path string) string {
	return fmt.Sprintf("%s:%s", st, path)
}

// AddPublishStream adds verifiers for a publish stream.
func (tv *Verifiers) AddPublishStream(topic string, partition int, streamVerifier *RPCVerifier) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	tv.streamVerifiers.Push(keyPartition(publishStreamType, topic, partition), streamVerifier)
}

// AddSubscribeStream adds verifiers for a subscribe stream.
func (tv *Verifiers) AddSubscribeStream(subscription string, partition int, streamVerifier *RPCVerifier) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	tv.streamVerifiers.Push(keyPartition(subscribeStreamType, subscription, partition), streamVerifier)
}

// AddCommitStream adds verifiers for a commit stream.
func (tv *Verifiers) AddCommitStream(subscription string, partition int, streamVerifier *RPCVerifier) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	tv.streamVerifiers.Push(keyPartition(commitStreamType, subscription, partition), streamVerifier)
}

// AddAssignmentStream adds verifiers for an assignment stream.
func (tv *Verifiers) AddAssignmentStream(subscription string, streamVerifier *RPCVerifier) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	tv.streamVerifiers.Push(key(assignmentStreamType, subscription), streamVerifier)
}

func (tv *Verifiers) popStreamVerifier(key string) (*RPCVerifier, error) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	v, err := tv.streamVerifiers.Pop(key)
	if v != nil {
		tv.activeStreamVerifiers = append(tv.activeStreamVerifiers, v)
	}
	return v, err
}

func (tv *Verifiers) flush() {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	tv.GlobalVerifier.Flush()
	tv.streamVerifiers.Flush()
	for _, v := range tv.activeStreamVerifiers {
		v.Flush()
	}
}
