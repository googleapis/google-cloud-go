// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ExecuteVRpc executes a virtual RPC sequentially and synchronously, multiplexing over the stream.
func (s *Session) ExecuteVRpc(ctx context.Context, desc VRpcDescriptor, req interface{}) (resp interface{}, clusterInfo *spb.ClusterInformation, err error) {
	if err := s.vrpcSem.Acquire(ctx, sessionConcurrencyLimit); err != nil {
		return nil, nil, err
	}
	defer s.vrpcSem.Release(sessionConcurrencyLimit)

	startTime := time.Now()
	defer func() {
		s.tracer.recordOperation(ctx, startTime, desc.Method(), err)
	}()

	s.mu.Lock()
	if s.state != StateActive {
		s.mu.Unlock()
		return nil, nil, status.Errorf(codes.Unavailable, "session is not active (state: %v)", s.state)
	}
	s.mu.Unlock()

	reqBytes, err := desc.Encode(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode request: %w", err)
	}

	rpcID := atomic.AddInt64(&s.nextRPCID, 1)

	resultChan := make(chan vrpcResult, 1)
	rpcImpl := &VRPCImpl{
		id:         rpcID,
		method:     desc.Method(),
		resultChan: resultChan,
	}

	s.mu.Lock()
	s.activeRPCs[rpcID] = rpcImpl
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.activeRPCs, rpcID)
		s.mu.Unlock()
	}()

	vrpcReq := &spb.VirtualRpcRequest{
		RpcId:   rpcID,
		Payload: reqBytes,
	}

	sessionReq := &spb.SessionRequest{
		Payload: &spb.SessionRequest_VirtualRpc{
			VirtualRpc: vrpcReq,
		},
	}

	s.resetHeartbeatDeadline() // Reset deadline when sending active request!

	if err := s.Send(sessionReq); err != nil {
		return nil, nil, fmt.Errorf("failed to send vRPC request: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case res := <-resultChan:
		if res.err != nil {
			return nil, res.clusterInfo, res.err
		}
		if res.resp.RpcId != rpcID {
			return nil, nil, fmt.Errorf("internal error: response RpcId %d does not match request RpcId %d", res.resp.RpcId, rpcID)
		}
		respMsg, err := desc.Decode(res.resp.Payload)
		if err != nil {
			return nil, res.clusterInfo, fmt.Errorf("failed to decode response: %w", err)
		}
		// if res.clusterInfo != nil {
		// 	fmt.Printf(">>> ExecuteVRpc response served by Cluster: Id=%s, Zone=%s <<<\n", res.clusterInfo.ClusterId, res.clusterInfo.ZoneId)
		// }
		// fmt.Printf(">>> ExecuteVRpc response decoded: Type=%T <<<\n", respMsg)
		return respMsg, res.clusterInfo, nil
	}
}

func (s *Session) handleVRPCResponse(resp *spb.VirtualRpcResponse) {
	// fmt.Printf(">>> Session handleVRPCResponse: RpcId=%d, PayloadLength=%d <<<\n", resp.RpcId, len(resp.Payload))
	s.mu.Lock()
	rpc, ok := s.activeRPCs[resp.RpcId]
	if ok {
		s.hasOkRpcs = true
	}
	s.mu.Unlock()

	if !ok {
		return
	}

	select {
	case rpc.resultChan <- vrpcResult{resp: resp, clusterInfo: resp.ClusterInfo}:
	default:
	}
}

func (s *Session) handleVRPCErrorResponse(errResp *spb.ErrorResponse) {
	s.mu.Lock()
	rpc, ok := s.activeRPCs[errResp.RpcId]
	if ok {
		s.hasErrorRpcs = true
	}
	s.mu.Unlock()

	if !ok {
		return
	}

	var goErr error
	if errResp.Status != nil {
		goErr = status.FromProto(errResp.Status).Err()
	} else {
		goErr = fmt.Errorf("unknown vRPC error")
	}

	select {
	case rpc.resultChan <- vrpcResult{err: goErr, clusterInfo: errResp.ClusterInfo}:
	default:
	}
}
