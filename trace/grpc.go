// Copyright 2017 Google Inc. All Rights Reserved.
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

package trace

import (
	"encoding/hex"
	"fmt"

	"cloud.google.com/go/internal/tracecontext"
	"golang.org/x/net/context"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const grpcMetadataKey = "grpc-trace-bin"

// GRPCClientInterceptor returns a grpc.UnaryClientInterceptor that traces all outgoing requests from a gRPC client.
// The calling context should already have a *trace.Span; a child span will be
// created for the outgoing gRPC call. If the calling context doesn't have a span,
// the call will not be traced.
//
// The functionality in gRPC that this feature relies on is currently experimental.
func GRPCClientInterceptor() grpc.UnaryClientInterceptor {
	return grpc.UnaryClientInterceptor(grpcUnaryInterceptor)
}

func grpcUnaryInterceptor(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	// TODO: also intercept streams.
	span := FromContext(ctx).NewChild(method)
	defer span.Finish()

	if span != nil {
		tc := make([]byte, tracecontext.Len)
		// traceID is a hex-encoded 128-bit value.
		// TODO(jbd): Decode trace IDs upon arrival and
		// represent trace IDs with 16 bytes internally.
		tid, err := hex.DecodeString(span.trace.traceID)
		if err != nil {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		tracecontext.Encode(tc, tid, span.span.SpanId, byte(span.trace.globalOptions))
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.Pairs(grpcMetadataKey, string(tc))
		} else {
			md = md.Copy() // metadata is immutable, copy.
			md[grpcMetadataKey] = []string{string(tc)}
		}
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	err := invoker(ctx, method, req, reply, cc, opts...)
	if err != nil {
		// TODO: standardize gRPC label names?
		span.SetLabel("error", err.Error())
	}
	return err
}

// GRPCServerInterceptor returns a grpc.UnaryServerInterceptor that enables the tracing of the incoming
// gRPC calls. Incoming call's context can be used to extract the span on servers that enabled this option:
//
//	span := trace.FromContext(ctx)
//
// The functionality in gRPC that this feature relies on is currently experimental.
func GRPCServerInterceptor(tc *Client) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		md, _ := metadata.FromIncomingContext(ctx)
		if header, ok := md[grpcMetadataKey]; ok {
			traceID, spanID, opts, ok := tracecontext.Decode([]byte(header[0]))
			if !ok {
				return handler(ctx, req)
			}
			// TODO(jbd): Generate a span directly from string(traceID), spanID and opts.
			header := fmt.Sprintf("%x/%d;o=%d", traceID, spanID, opts)
			span := tc.SpanFromHeader("", header)

			defer span.Finish()
			ctx = NewContext(ctx, span)
		}
		return handler(ctx, req)
	}
}

// EnableGRPCTracing automatically traces all outgoing gRPC calls from cloud.google.com/go clients.
//
// The functionality in gRPC that this relies on is currently experimental.
//
// Deprecated: Use option.WithGRPCDialOption(grpc.WithUnaryInterceptor(GRPCClientInterceptor())) instead.
var EnableGRPCTracing option.ClientOption = option.WithGRPCDialOption(grpc.WithUnaryInterceptor(GRPCClientInterceptor()))
