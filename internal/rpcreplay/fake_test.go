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

package rpcreplay

import (
	"log"
	"net"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	pb "cloud.google.com/go/internal/rpcreplay/proto/intstore"
)

// intStoreServer is an in-memory implementation of IntStore.
type intStoreServer struct {
	pb.IntStoreServer

	Addr string
	l    net.Listener
	gsrv *grpc.Server

	items map[string]int32
}

func newIntStoreServer() *intStoreServer {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	s := &intStoreServer{
		Addr: l.Addr().String(),
		l:    l,
		gsrv: grpc.NewServer(),
	}
	pb.RegisterIntStoreServer(s.gsrv, s)
	go s.gsrv.Serve(s.l)
	return s
}

func (s *intStoreServer) stop() {
	s.gsrv.Stop()
	s.l.Close()
}

func (s *intStoreServer) Set(_ context.Context, item *pb.Item) (*pb.SetResponse, error) {
	old := s.setItem(item)
	return &pb.SetResponse{PrevValue: old}, nil
}

func (s *intStoreServer) setItem(item *pb.Item) int32 {
	if s.items == nil {
		s.items = map[string]int32{}
	}
	old := s.items[item.Name]
	s.items[item.Name] = item.Value
	return old
}

func (s *intStoreServer) Get(_ context.Context, req *pb.GetRequest) (*pb.Item, error) {
	val, ok := s.items[req.Name]
	if !ok {
		return nil, grpc.Errorf(codes.NotFound, "%q", req.Name)
	}
	return &pb.Item{Name: req.Name, Value: val}, nil
}
