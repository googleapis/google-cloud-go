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

type rpcMetadata struct {
	wantRequest   interface{}
	retResponse   interface{}
	retErr        error
	blockResponse chan struct{}
}

// wait until the `blockResponse` is released by the test, or a timeout occurs.
// Returns immediately if there was no block.
func (r *rpcMetadata) wait() error {
	if r.blockResponse == nil {
		return nil
	}
	select {
	case <-time.After(blockWaitTimeout):
		// Note: avoid returning a retryable code to quickly terminate the test.
		return status.Errorf(codes.FailedPrecondition, "mockserver: test did not unblock response within %v", blockWaitTimeout)
	case <-r.blockResponse:
		return nil
	}
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
func (v *RPCVerifier) Push(wantRequest interface{}, retResponse interface{}, retErr error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.rpcs.PushBack(&rpcMetadata{
		wantRequest: wantRequest,
		retResponse: retResponse,
		retErr:      retErr,
	})
}

// PushWithBlock is like Push, but returns a channel that the test should close
// when it would like the response to be sent to the client. This is useful for
// synchronizing with work that needs to be done on the client.
func (v *RPCVerifier) PushWithBlock(wantRequest interface{}, retResponse interface{}, retErr error) chan struct{} {
	v.mu.Lock()
	defer v.mu.Unlock()

	block := make(chan struct{})
	v.rpcs.PushBack(&rpcMetadata{
		wantRequest:   wantRequest,
		retResponse:   retResponse,
		retErr:        retErr,
		blockResponse: block,
	})
	return block
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
		v.t.Errorf("call(%d): did not receive expected request:\n%v", v.numCalls, rpc.wantRequest)
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

// keyedStreamVerifiers stores indexed streamVerifiers.
type keyedStreamVerifiers struct {
	verifiers map[string]*streamVerifiers
}

func newKeyedStreamVerifiers() *keyedStreamVerifiers {
	return &keyedStreamVerifiers{verifiers: make(map[string]*streamVerifiers)}
}

func (kv *keyedStreamVerifiers) Reset() {
	kv.verifiers = make(map[string]*streamVerifiers)
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
