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

import (
	"context"

	executorpb "cloud.google.com/go/spanner/cloudexecutor/proto"
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
func (s *CloudProxyServer) ExecuteActionAsync(stream executorpb.SpannerExecutorProxy_ExecuteActionAsyncServer) error {
	handler := &cloudStreamHandler{
		cloudProxyServer: s,
		stream:           stream,
	}
	return handler.execute()
}
