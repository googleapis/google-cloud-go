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
