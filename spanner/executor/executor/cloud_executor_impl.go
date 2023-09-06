package executor

import (
	"context"

	"cloud.google.com/go/spanner/executor/apiv1/executorpb"
	"google.golang.org/api/option"
)

// CloudProxyServer holds the cloud go server.
type CloudProxyServer struct {
	options []option.ClientOption
}

// NewCloudProxyServer initializes and returns a new CloudProxyServer instance.
func NewCloudProxyServer(ctx context.Context, opts []option.ClientOption) (*CloudProxyServer, error) {
	return &CloudProxyServer{options: opts}, nil
}

// ExecuteActionAsync is implementation of ExecuteActionAsync in AsyncSpannerExecutorProxy. It's a
// streaming method in which client and server exchange SpannerActions and SpannerActionOutcomes.
func (s *CloudProxyServer) ExecuteActionAsync(stream executorpb.SpannerExecutorProxy_ExecuteActionAsyncServer) error {
	h := &cloudStreamHandler{
		cloudProxyServer: s,
		stream:           stream,
	}
	return h.execute()
}
