package executor

import (
	executorpb "cloud.google.com/go/spanner/cloudexecutor/proto"
)

type cloudStreamHandler struct {
	cloudProxyServer *CloudProxyServer
	stream           executorpb.SpannerExecutorProxy_ExecuteActionAsyncServer
}

func (h *cloudStreamHandler) execute() error {
	return nil
}
