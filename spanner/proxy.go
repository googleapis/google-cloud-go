/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

type rawCodec struct{}

func (rawCodec) Marshal(v any) ([]byte, error) {
	if b, ok := v.([]byte); ok {
		return b, nil
	}
	if pm, ok := v.(proto.Message); ok {
		return proto.Marshal(pm)
	}
	return nil, fmt.Errorf("rawCodec.Marshal: unexpected type %T", v)
}

func (rawCodec) Unmarshal(data []byte, v any) error {
	if b, ok := v.(*[]byte); ok {
		*b = data
		return nil
	}
	if pm, ok := v.(proto.Message); ok {
		return proto.Unmarshal(data, pm)
	}
	return fmt.Errorf("rawCodec.Unmarshal: unexpected type %T", v)
}

func (rawCodec) Name() string {
	return ""
}

// RawStream is a streaming client wrapper that yields raw bytes.
type RawStream interface {
	Recv() ([]byte, error)
	Header() (metadata.MD, error)
	Trailer() metadata.MD
	Context() context.Context
}

type rawStream struct {
	grpc.ClientStream
}

func (s *rawStream) Recv() ([]byte, error) {
	var b []byte
	err := s.ClientStream.RecvMsg(&b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (c *Client) getConn(hint int) (*grpc.ClientConn, error) {
	if c.sc == nil {
		return nil, fmt.Errorf("session client is not initialized")
	}

	conns := c.sc.connections
	numConns := len(conns)
	if numConns > 0 {
		idx := uint64(hint)
		return conns[idx % uint64(numConns)], nil
	}

	c.sc.mu.RLock()
	defer c.sc.mu.RUnlock()

	if len(c.sc.channelIDMap) == 0 {
		return c.sc.connPool.Conn(), nil
	}

	idx := uint64(hint)
	numChannels := uint64(len(c.sc.channelIDMap))
	targetID := (idx % numChannels) + 1

	for conn, id := range c.sc.channelIDMap {
		if id == targetID {
			return conn, nil
		}
	}

	return c.sc.connPool.Conn(), nil
}

// MakeUnaryCall executes a pass-through unary gRPC call using raw bytes.
func (c *Client) MakeUnaryCall(ctx context.Context, path string, requestBytes []byte, headers map[string]string, channelHint int) ([]byte, error) {
	conn, err := c.getConn(channelHint)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		ctx = metadata.AppendToOutgoingContext(ctx, k, v)
	}

	var responseBytes []byte
	err = conn.Invoke(ctx, path, requestBytes, &responseBytes, grpc.ForceCodec(rawCodec{}))
	if err != nil {
		return nil, err
	}
	return responseBytes, nil
}

// MakeStreamingCall executes a pass-through server-streaming gRPC call using raw bytes.
func (c *Client) MakeStreamingCall(ctx context.Context, path string, requestBytes []byte, headers map[string]string, channelHint int) (RawStream, error) {
	conn, err := c.getConn(channelHint)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		ctx = metadata.AppendToOutgoingContext(ctx, k, v)
	}

	desc := &grpc.StreamDesc{
		StreamName:    "MakeStreamingCall",
		ServerStreams: true,
		ClientStreams: false,
	}

	cs, err := conn.NewStream(ctx, desc, path, grpc.ForceCodec(rawCodec{}))
	if err != nil {
		return nil, err
	}

	if err := cs.SendMsg(requestBytes); err != nil {
		_ = cs.CloseSend()
		return nil, err
	}
	if err := cs.CloseSend(); err != nil {
		return nil, err
	}

	return &rawStream{ClientStream: cs}, nil
}

// RowDecoder wraps the internal partialResultSetDecoder for use by the native proxy addon.
type RowDecoder struct {
	dec partialResultSetDecoder
}

// NewRowDecoder creates a new RowDecoder.
func NewRowDecoder() *RowDecoder {
	return &RowDecoder{}
}

// Add decodes a PartialResultSet chunk and returns any newly completed Rows.
func (d *RowDecoder) Add(r []byte) ([]*Row, []byte, error) {
	var prs sppb.PartialResultSet
	if err := proto.Unmarshal(r, &prs); err != nil {
		return nil, nil, err
	}

	rows, metadata, err := d.dec.add(&prs)
	if err != nil {
		return nil, nil, err
	}

	var metadataBytes []byte
	if metadata != nil {
		prsMetadata := &sppb.PartialResultSet{
			Metadata: metadata,
		}
		metadataBytes, err = proto.Marshal(prsMetadata)
		if err != nil {
			return nil, nil, err
		}
	}

	return rows, metadataBytes, nil
}

// ResumableRawStream wraps the internal resumableStreamDecoder to handle automatic reconnection.
type ResumableRawStream struct {
	decoder *resumableStreamDecoder
	cancel  context.CancelFunc
	mt      builtinMetricsTracer
}

type rawStreamingReceiver struct {
	stream RawStream
}

func (w *rawStreamingReceiver) Recv() (*sppb.PartialResultSet, error) {
	bytes, err := w.stream.Recv()
	if err != nil {
		return nil, err
	}
	prs := &sppb.PartialResultSet{}
	if err := proto.Unmarshal(bytes, prs); err != nil {
		return nil, err
	}
	return prs, nil
}

func (w *rawStreamingReceiver) Context() context.Context {
	return w.stream.Context()
}

type dummyRequestIDHeaderProvider struct {
	gsc *grpcSpannerClient
}

func (p *dummyRequestIDHeaderProvider) requestIDHeaderInjector(ctx context.Context) (*requestIDWrap, error) {
	md := new(metadata.MD)
	return &requestIDWrap{
		md:         md,
		nthRequest: p.gsc.nextNthRequest(),
		gsc:        p.gsc,
	}, nil
}

// NewResumableRawStream creates a new ResumableRawStream that handles auto-resumptions for streaming queries.
func (c *Client) NewResumableRawStream(
	ctx context.Context,
	path string,
	requestBytes []byte,
	headers map[string]string,
	channelHint int,
) (*ResumableRawStream, error) {
	ctx, cancel := context.WithCancel(ctx)

	dummyGSC := &grpcSpannerClient{
		id:         999,
		channelID:  999,
		nthRequest: new(atomic.Uint32),
	}
	reqIDProvider := &dummyRequestIDHeaderProvider{gsc: dummyGSC}

	rpcFactory := func(ct context.Context, restartToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
		req := &sppb.ExecuteSqlRequest{}
		if err := proto.Unmarshal(requestBytes, req); err != nil {
			return nil, err
		}
		if len(restartToken) > 0 {
			req.ResumeToken = restartToken
		}
		newReqBytes, err := proto.Marshal(req)
		if err != nil {
			return nil, err
		}

		rawStr, err := c.MakeStreamingCall(ct, path, newReqBytes, headers, channelHint)
		if err != nil {
			return nil, err
		}

		return &rawStreamingReceiver{stream: rawStr}, nil
	}

	decoder := newResumableStreamDecoder(
		ctx,
		func() { cancel() },
		nil, // logger
		rpcFactory,
		reqIDProvider,
		true, // retryResourceExhausted
		true, // allowRetryResourceExhaustedWithoutDelay
	)

	mt := builtinMetricsTracer{currOp: &opTracer{currAttempt: &attemptTracer{}}}

	return &ResumableRawStream{
		decoder: decoder,
		cancel:  cancel,
		mt:      mt,
	}, nil
}

// Recv returns the next raw bytes from the resumable stream, or error (e.g. io.EOF).
func (s *ResumableRawStream) Recv() ([]byte, error) {
	if !s.decoder.next(&s.mt) {
		if err := s.decoder.lastErr(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}

	prs := s.decoder.get()
	if prs == nil {
		return nil, io.EOF
	}

	return proto.Marshal(prs)
}

// Close cancels the underlying stream context.
func (s *ResumableRawStream) Close() {
	s.cancel()
}

// NewResumableRowIterator creates a new RowIterator that handles auto-resumptions for streaming queries.
func (c *Client) NewResumableRowIterator(
	ctx context.Context,
	path string,
	requestBytes []byte,
	headers map[string]string,
	channelHint int,
) (*RowIterator, error) {
	ctx, cancel := context.WithCancel(ctx)

	dummyGSC := &grpcSpannerClient{
		id:         999,
		channelID:  999,
		nthRequest: new(atomic.Uint32),
	}
	reqIDProvider := &dummyRequestIDHeaderProvider{gsc: dummyGSC}

	rpcFactory := func(ct context.Context, restartToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
		req := &sppb.ExecuteSqlRequest{}
		if err := proto.Unmarshal(requestBytes, req); err != nil {
			return nil, err
		}
		if len(restartToken) > 0 {
			req.ResumeToken = restartToken
		}
		newReqBytes, err := proto.Marshal(req)
		if err != nil {
			return nil, err
		}

		rawStr, err := c.MakeStreamingCall(ct, path, newReqBytes, headers, channelHint)
		if err != nil {
			return nil, err
		}

		return &rawStreamingReceiver{stream: rawStr}, nil
	}

	mtFactory := &builtinMetricsTracerFactory{}

	iter := &RowIterator{
		meterTracerFactory: mtFactory,
		streamd:            newResumableStreamDecoder(ctx, cancel, nil, rpcFactory, reqIDProvider, true, true),
		rowd:               &partialResultSetDecoder{},
		cancel:             cancel,
		ctx:                ctx,
	}
	iter.release = func(err error) {}
	iter.updateTxState = func(err error) error { return err }

	return iter, nil
}


