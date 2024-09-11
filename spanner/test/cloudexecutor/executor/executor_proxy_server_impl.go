// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package executor

// executor_proxy_server_impl.go contains the implementation of the executor proxy RPC.
// This RPC gets invoked through the gRPC stream exposed via proxy port by worker_proxy.go file.

import (
	"context"
	"sync"

	"cloud.google.com/go/spanner/executor/apiv1/executorpb"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/internal/inputstream"
	"google.golang.org/api/option"
)

const MAX_CLOUD_TRACE_CHECK_LIMIT = 20

// CloudProxyServer holds the cloud executor server.
type CloudProxyServer struct {
	// members below should be set by the caller
	serverContext      context.Context
	options            []option.ClientOption
	traceClientOptions []option.ClientOption
	// members below represent internal state
	mu                   sync.Mutex
	cloudTraceCheckCount int
}

// NewCloudProxyServer initializes and returns a new CloudProxyServer instance.
func NewCloudProxyServer(ctx context.Context, opts []option.ClientOption, traceClientOpts []option.ClientOption) (*CloudProxyServer, error) {
	return &CloudProxyServer{serverContext: ctx, options: opts, traceClientOptions: traceClientOpts}, nil
}

// ExecuteActionAsync is implementation of ExecuteActionAsync in SpannerExecutorProxyServer. It's a
// streaming method in which client and server exchange SpannerActions and SpannerActionOutcomes.
func (s *CloudProxyServer) ExecuteActionAsync(inputStream executorpb.SpannerExecutorProxy_ExecuteActionAsyncServer) error {
	handler := &inputstream.CloudStreamHandler{
		Stream:             inputStream,
		ServerContext:      s.serverContext,
		Options:            s.options,
		TraceClientOptions: s.traceClientOptions,
	}
	func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.cloudTraceCheckCount < MAX_CLOUD_TRACE_CHECK_LIMIT {
			handler.CloudTraceCheckAllowed = true
		}
	}()

	if err := handler.Execute(); err != nil {
		return err
	}
	if handler.IsServerSideTraceCheckDone() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.cloudTraceCheckCount++
	}
	return nil
}
