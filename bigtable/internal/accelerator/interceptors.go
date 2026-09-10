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

package accelerator

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"io"
	"log"
	"strings"
	"sync"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// bigtableServerStub satisfies grpc's requirement that a typed server
// implementation be registered for the Bigtable service before it will
// accept RPCs. The proxy interceptors short-circuit every method before it
// reaches this stub — the embedded UnimplementedBigtableServer is only
// here to make the type satisfy btpb.BigtableServer.
type bigtableServerStub struct {
	btpb.UnimplementedBigtableServer
}

// newBigtableServerStub creates a new stub instance.
func newBigtableServerStub() *bigtableServerStub {
	return &bigtableServerStub{}
}

// authUnaryInterceptor validates the x-accelerator-token metadata header on
// every unary RPC. It is only chained into the server when a non-empty secret
// was read from stdin (see Server.Start); the shipped daemon always reads one,
// so auth is disabled only in test mode (WithStdinReader(nil)). Mismatches are
// rejected with codes.Internal (see checkAuthToken for why not Unauthenticated).
//
// "Every RPC" is literal, and deliberately so: unlike the proxy interceptors
// below, this one does not exempt locally-served services via isProxied, even
// though the health service is served locally. The two interceptors read the
// same predicate in opposite safety directions. There, a false means "do not
// forward this to Bigtable" and an unrecognised method falls through to gRPC's
// own Unimplemented — a loud, harmless failure. Here, a false would mean "serve
// this without a token", so an unrecognised method would be served unauthenticated.
// Since isProxied is a Bigtable-prefix test, every service registered later is
// unrecognised by construction, and would be un-gated by default rather than by
// decision.
//
// Nothing is lost by gating health, either: the token is not a Bigtable
// credential but a caller-identity check on the socket, and the only legitimate
// caller — the process that spawned this daemon — builds its health stub on the
// same connection as its data RPCs and so already attaches the token. Gating it
// also means a foreign daemon left on a reused socket path fails our probe
// closed, rather than answering SERVING for a process we are not using.
func authUnaryInterceptor(secret string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := checkAuthToken(ctx, secret); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// authStreamInterceptor is the streaming counterpart to authUnaryInterceptor.
func authStreamInterceptor(secret string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := checkAuthToken(ss.Context(), secret); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

// checkAuthToken pulls the x-accelerator-token from incoming metadata and
// constant-time-compares its SHA-256 digest against the secret's. Hashing first
// makes both ConstantTimeCompare inputs a fixed 32 bytes, so a mismatched-length
// token cannot leak the secret's length through the early-return timing side
// channel (ConstantTimeCompare returns immediately when its inputs differ in
// length).
//
// It returns nil when the token matches. The secret == "" guard is defense in
// depth: Server.Start only chains this interceptor for a non-empty secret, so
// the shipped daemon never hits it.
//
// On failure it returns codes.Internal with a generic message, NOT
// codes.Unauthenticated: the accelerator is a transparent proxy, so the only
// Unauthenticated a caller should ever see is a genuine Bigtable credential
// failure forwarded from the backend. A handshake-token mismatch is instead an
// accelerator-internal condition — in practice always a client/daemon wiring
// bug, since the spawning client attaches the correct token to every RPC.
// Surfacing it as Unauthenticated would send the user to debug their GCP
// credentials; Internal correctly says "not your fault." The specific reason is
// logged server-side (and never returned) so a same-UID probe gets no oracle.
func checkAuthToken(ctx context.Context, secret string) error {
	if secret == "" {
		return nil
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return authFailure("missing metadata")
	}
	vals := md.Get("x-accelerator-token")
	if len(vals) == 0 {
		return authFailure("missing x-accelerator-token")
	}
	secretHash := sha256.Sum256([]byte(secret))
	tokenHash := sha256.Sum256([]byte(vals[0]))
	if subtle.ConstantTimeCompare(tokenHash[:], secretHash[:]) != 1 {
		return authFailure("invalid x-accelerator-token")
	}
	return nil
}

// authRejectMsg is the single message used both for the server-side log line and
// for the codes.Internal error returned to the caller, so the two never drift
// apart. The specific reason (which check failed) is appended only to the log and
// is never returned, so a same-UID probe gets no oracle.
const authRejectMsg = "accelerator: rejecting RPC from mismatched token"

// authFailure logs the handshake rejection server-side (with the specific reason,
// which is never returned) and returns a codes.Internal error carrying the
// generic authRejectMsg to the caller.
func authFailure(reason string) error {
	log.Printf("%s (reason: %s)", authRejectMsg, reason)
	return status.Error(codes.Internal, authRejectMsg)
}

// bigtableMethodPrefix is the "/<service>/" prefix every RPC the daemon
// proxies to Bigtable carries. Derived from the generated service descriptor
// so it cannot drift from the registered service name.
var bigtableMethodPrefix = "/" + btpb.Bigtable_ServiceDesc.ServiceName + "/"

// isProxied reports whether fullMethod should be forwarded through the Channel
// to Bigtable, as opposed to being served locally by this process.
//
// The proxy interceptors are chained onto the whole server, so without this
// gate they would swallow every RPC of every registered service -- they never
// invoke the next handler. Any locally-served service (the health service, and
// anything added later) therefore has to be excluded here or its RPCs get
// forwarded to Bigtable, which has never heard of them.
//
// The test is deliberately "is this Bigtable?" rather than "is this one of the
// known-local services": a service we forgot to list then fails with gRPC's own
// Unimplemented instead of being silently shipped to the backend.
func isProxied(fullMethod string) bool {
	return strings.HasPrefix(fullMethod, bigtableMethodPrefix)
}

// proxyUnaryInterceptor intercepts every incoming unary RPC, allocates the
// correct response message via the global proto registry, and delegates to the
// channel's Invoke. The single switch on method name lives in
// Channel.Invoke; this interceptor is switch-free.
//
// The response type is registered in protoregistry.GlobalTypes as a side
// effect of importing btpb (the generated init() in the btpb package does the
// registration), so any RPC defined in the v2 Bigtable service is resolvable
// without per-method wiring here.
//
// Non-Bigtable RPCs are passed to their real handler -- see isProxied.
func proxyUnaryInterceptor(channel *Channel) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if !isProxied(info.FullMethod) {
			return handler(ctx, req)
		}
		_, output, err := resolveMethodIO(info.FullMethod)
		if err != nil {
			return nil, err
		}
		reply := output.New().Interface()
		if err := channel.Invoke(ctx, info.FullMethod, req, reply); err != nil {
			return nil, err
		}
		return reply, nil
	}
}

// proxyStreamInterceptor mirrors proxyUnaryInterceptor for server-streaming
// RPCs. For each incoming stream it resolves the method's input/output types
// via protoreflect, opens a client stream against the Channel,
// forwards the single request from the server side, then pumps responses
// back until io.EOF. Backpressure flows naturally: each ss.SendMsg blocks on
// HTTP/2 flow control when the client is slow, which holds back the next
// clientStream.RecvMsg.
//
// Only server-streaming RPCs are handled. Client-streaming and bidi RPCs are
// rejected with Unimplemented because no Channel method shape
// supports them today.
//
// Non-Bigtable streams are passed to their real handler -- see isProxied. That
// check comes before the client-streaming rejection below, which is scoped to
// methods this proxy is actually responsible for: grpc.health.v1.Health/Watch
// is a legitimate server-streaming method served locally, and a locally-served
// client-streaming method would be the local handler's business, not ours.
func proxyStreamInterceptor(channel *Channel) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if !isProxied(info.FullMethod) {
			return handler(srv, ss)
		}
		if info.IsClientStream {
			return status.Errorf(codes.Unimplemented, "accelerator: client-streaming method %s not supported", info.FullMethod)
		}
		input, output, err := resolveMethodIO(info.FullMethod)
		if err != nil {
			return err
		}

		req := input.New().Interface()
		if err := ss.RecvMsg(req); err != nil {
			// A server-streaming RPC must carry exactly one request. An io.EOF
			// here means the client half-closed without sending it; surface that
			// as InvalidArgument rather than returning a raw io.EOF, which gRPC
			// would report to the caller as an opaque codes.Unknown.
			if err == io.EOF {
				return status.Errorf(codes.InvalidArgument, "accelerator: %s expects exactly one request, got none", info.FullMethod)
			}
			return err
		}

		// Bind the upstream stream's lifetime to this handler. Deferring cancel
		// guarantees the Channel stream is torn down on every exit path —
		// including a mid-pump ss.SendMsg failure — rather than relying solely on
		// the server context being cancelled after the handler returns.
		ctx, cancel := context.WithCancel(ss.Context())
		defer cancel()

		desc := &grpc.StreamDesc{
			StreamName:    info.FullMethod,
			ServerStreams: true,
		}
		clientStream, err := channel.NewStream(ctx, desc, info.FullMethod)
		if err != nil {
			return err
		}
		if err := clientStream.SendMsg(req); err != nil {
			return err
		}
		if err := clientStream.CloseSend(); err != nil {
			return err
		}

		// Allocate the response message once and reuse it across iterations
		// instead of allocating per streamed row. gRPC marshals the message
		// synchronously inside SendMsg before returning, so resetting and
		// refilling the same message on the next RecvMsg is safe.
		resp := output.New().Interface()
		for {
			proto.Reset(resp)
			err := clientStream.RecvMsg(resp)
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			if err := ss.SendMsg(resp); err != nil {
				return err
			}
		}
	}
}

// methodIO holds the resolved input and output message types for a gRPC
// method, cached in methodCache so the registry lookup runs at most once per
// method name.
type methodIO struct {
	input  protoreflect.MessageType
	output protoreflect.MessageType
}

// methodCache memoizes resolveMethodIO results keyed by full method name. The
// Bigtable service surface is static, so a successful resolution never changes
// and can be reused for the life of the process.
var methodCache sync.Map // map[string]methodIO

// resolveMethodIO looks up a gRPC method by its full name (e.g.
// "/google.bigtable.v2.Bigtable/MutateRow") in the global proto registry and
// returns the message types for its input and output. Successful resolutions
// are cached so subsequent calls on the RPC hot path are O(1) lookups with no
// registry traversal or string parsing. Errors are not cached — they only
// arise from unimplemented/malformed methods, which are not on the hot path.
func resolveMethodIO(fullMethod string) (protoreflect.MessageType, protoreflect.MessageType, error) {
	if v, ok := methodCache.Load(fullMethod); ok {
		io := v.(methodIO)
		return io.input, io.output, nil
	}

	serviceName, methodName, ok := strings.Cut(strings.TrimPrefix(fullMethod, "/"), "/")
	if !ok {
		return nil, nil, status.Errorf(codes.Internal, "invalid method name: %s", fullMethod)
	}
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(serviceName))
	if err != nil {
		return nil, nil, status.Errorf(codes.Unimplemented, "service %s not found: %v", serviceName, err)
	}
	svc, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, nil, status.Errorf(codes.Internal, "descriptor %s is not a service", serviceName)
	}
	method := svc.Methods().ByName(protoreflect.Name(methodName))
	if method == nil {
		return nil, nil, status.Errorf(codes.Unimplemented, "method %s not found on service %s", methodName, serviceName)
	}
	inputType, err := protoregistry.GlobalTypes.FindMessageByName(method.Input().FullName())
	if err != nil {
		return nil, nil, status.Errorf(codes.Internal, "input type %s not registered: %v", method.Input().FullName(), err)
	}
	outputType, err := protoregistry.GlobalTypes.FindMessageByName(method.Output().FullName())
	if err != nil {
		return nil, nil, status.Errorf(codes.Internal, "output type %s not registered: %v", method.Output().FullName(), err)
	}

	methodCache.Store(fullMethod, methodIO{input: inputType, output: outputType})
	return inputType, outputType, nil
}
