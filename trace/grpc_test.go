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
	"log"
	"net"
	"testing"

	pb "cloud.google.com/go/trace/testdata/helloworld"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

func TestGRPCInterceptors(t *testing.T) {
	tc := newTestClient(&noopTransport{})

	incomingCh := make(chan *Span, 1)
	addrCh := make(chan net.Addr, 1)
	go func() {
		lis, err := net.Listen("tcp", "")
		if err != nil {
			t.Fatalf("Failed to listen: %v", err)
		}
		addrCh <- lis.Addr()

		s := grpc.NewServer(grpc.UnaryInterceptor(GRPCServerInterceptor(tc)))
		pb.RegisterGreeterServer(s, &grpcServer{
			fn: func(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
				incomingCh <- FromContext(ctx)
				return &pb.HelloReply{}, nil
			},
		})
		if err := s.Serve(lis); err != nil {
			t.Fatalf("Failed to serve: %v", err)
		}
	}()

	addr := <-addrCh
	conn, err := grpc.Dial(addr.String(), grpc.WithInsecure(), grpc.WithUnaryInterceptor(GRPCClientInterceptor()))
	if err != nil {
		log.Fatalf("Did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewGreeterClient(conn)

	span := tc.NewSpan("parent")
	outgoingCtx := NewContext(context.Background(), span)
	_, err = c.SayHello(outgoingCtx, &pb.HelloRequest{})
	if err != nil {
		log.Fatalf("Could not SayHello: %v", err)
	}

	incomingSpan := <-incomingCh
	if incomingSpan == nil {
		t.Fatalf("missing span in the incoming context")
	}
	if got, want := incomingSpan.TraceID(), span.TraceID(); got != want {
		t.Errorf("incoming call is not tracing the outgoing trace; TraceID = %q; want %q", got, want)
	}
}

type grpcServer struct {
	fn func(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error)
}

func (s *grpcServer) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	return s.fn(ctx, in)
}
