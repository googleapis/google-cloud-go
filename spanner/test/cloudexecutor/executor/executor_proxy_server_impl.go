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

	"cloud.google.com/go/spanner/executor/apiv1/executorpb"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/internal/inputstream"
	"google.golang.org/api/option"
)

// CloudProxyServer holds the cloud executor server.
type CloudProxyServer struct {
	serverContext context.Context
	options       []option.ClientOption
}

// NewCloudProxyServer initializes and returns a new CloudProxyServer instance.
func NewCloudProxyServer(ctx context.Context, opts []option.ClientOption) (*CloudProxyServer, error) {
	return &CloudProxyServer{serverContext: ctx, options: opts}, nil
}

// ExecuteActionAsync is implementation of ExecuteActionAsync in SpannerExecutorProxyServer. It's a
// streaming method in which client and server exchange SpannerActions and SpannerActionOutcomes.
func (s *CloudProxyServer) ExecuteActionAsync(inputStream executorpb.SpannerExecutorProxy_ExecuteActionAsyncServer) error {
	handler := &inputstream.CloudStreamHandler{
		Stream:        inputStream,
		ServerContext: s.serverContext,
		Options:       s.options,
	}
	return handler.Execute()
}
