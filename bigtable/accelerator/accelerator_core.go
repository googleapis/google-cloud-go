package accelerator

import (
	"context"

	"accelerator/adapters"
	"accelerator/metrics"
	"accelerator/resourcemanager"
	v2pb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// SessionPool is a placeholder for the future per-(resource, method) session
// pool cached by the resource manager. Its ReadRow/MutateRow stubs return
// Unimplemented until the Jetstream transport is wired; the rest of the
// translation pipeline (dispatch, pool lookup) is built against this type
// so swapping in the real implementation is a localized change.
type SessionPool struct{}

// Close releases resources owned by the pool. No-op for the placeholder.
func (p *SessionPool) Close() error { return nil }

func (p *SessionPool) ReadRow(ctx context.Context, req *v2pb.SessionReadRowRequest) (*v2pb.SessionReadRowResponse, error) {
	return nil, status.Error(codes.Unimplemented, "SessionPool.ReadRow not wired")
}

func (p *SessionPool) MutateRow(ctx context.Context, req *v2pb.SessionMutateRowRequest) (*v2pb.SessionMutateRowResponse, error) {
	return nil, status.Error(codes.Unimplemented, "SessionPool.MutateRow not wired")
}

// Ensure AcceleratorChannel implements grpc.ClientConnInterface.
var _ grpc.ClientConnInterface = (*AcceleratorChannel)(nil)

// AcceleratorChannel is the single in-process channel used by both the native
// Go client and the daemon's gRPC service via grpc.ClientConnInterface.
type AcceleratorChannel struct {
	pools    *resourcemanager.PoolCache[*SessionPool]
	recorder *metrics.MetricsRecorder
}

// NewAcceleratorChannel constructs an AcceleratorChannel with its pool cache
// and metrics recorder wired internally. This is the public factory consumed
// by both the native Go client and the daemon binary.
func NewAcceleratorChannel() *AcceleratorChannel {
	factory := func(resource, method string) (*SessionPool, error) {
		return &SessionPool{}, nil
	}
	return &AcceleratorChannel{
		pools:    resourcemanager.NewPoolCache[*SessionPool](resourcemanager.DefaultPoolCacheSize, factory),
		recorder: &metrics.MetricsRecorder{},
	}
}

// Invoke implements grpc.ClientConnInterface by dispatching on the V2 method
// name to a per-method impl helper.
func (c *AcceleratorChannel) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	switch method {
	case v2pb.Bigtable_MutateRow_FullMethodName:
		return c.mutateRowImpl(ctx, args, reply)
	default:
		return status.Errorf(codes.Unimplemented, "method %s not implemented", method)
	}
}


func (c *AcceleratorChannel) mutateRowImpl(ctx context.Context, args interface{}, reply interface{}) error {
	reqV2, ok := args.(*v2pb.MutateRowRequest)
	if !ok {
		return status.Errorf(codes.Internal, "unexpected request type: %T", args)
	}
	respV2, ok := reply.(*v2pb.MutateRowResponse)
	if !ok {
		return status.Errorf(codes.Internal, "unexpected response type: %T", reply)
	}

	reqAdapter := &adapters.MutateRowRequestAdapter{}
	reqJS, err := reqAdapter.Adapt(reqV2)
	if err != nil {
		return err
	}
	resourceName, err := reqAdapter.ExtractResource(reqV2)
	if err != nil {
		return err
	}

	pool, release, err := c.pools.GetOrOpen(resourceName, "MutateRow")
	if err != nil {
		return err
	}
	defer release()
	respJS, err := pool.MutateRow(ctx, reqJS)
	if err != nil {
		return err
	}

	respAdapter := &adapters.MutateRowResponseAdapter{}
	adaptedResp, err := respAdapter.Adapt(respJS)
	if err != nil {
		return err
	}

	proto.Reset(respV2)
	proto.Merge(respV2, adaptedResp)
	return nil
}

func (c *AcceleratorChannel) readRowsImpl(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return &readRowsClientStream{ctx: ctx}, nil
}

// NewStream implements grpc.ClientConnInterface.
func (c *AcceleratorChannel) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	switch method {
	case v2pb.Bigtable_ReadRows_FullMethodName:
		return c.readRowsImpl(ctx, desc, method, opts...)
	default:
		return nil, status.Errorf(codes.Unimplemented, "method %s not implemented", method)
	}
}

type readRowsClientStream struct {
	ctx context.Context
}

func (s *readRowsClientStream) Header() (metadata.MD, error) { return nil, nil }
func (s *readRowsClientStream) Trailer() metadata.MD          { return nil }
func (s *readRowsClientStream) CloseSend() error               { return nil }
func (s *readRowsClientStream) Context() context.Context       { return s.ctx }
func (s *readRowsClientStream) SendMsg(m any) error            { return status.Error(codes.Unimplemented, "SendMsg not implemented") }
func (s *readRowsClientStream) RecvMsg(m any) error {
	return status.Error(codes.Unimplemented, "RecvMsg not implemented")
}

// Close releases resources held by the channel, draining any pools owned by
// the resource manager. Callers (notably AcceleratorServer.Stop) should
// invoke this after the gRPC server has finished draining in-flight RPCs so
// pools are not torn down while requests are still using them.
func (c *AcceleratorChannel) Close() error {
	if c.pools == nil {
		return nil
	}
	return c.pools.Close()
}
