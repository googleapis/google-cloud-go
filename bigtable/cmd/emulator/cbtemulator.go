// Copyright 2016 Google LLC
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

/*
cbtemulator launches the in-memory Cloud Bigtable server on the given address.
*/
package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"cloud.google.com/go/bigtable/bttest"
	"google.golang.org/grpc"
)

var (
	host                   = flag.String("host", "localhost", "the address to bind to on the local machine")
	port                   = flag.Int("port", 9000, "the port number to bind to on the local machine")
	introduce_grpc_latency = flag.Bool("introduce_grpc_latency", false, "Introduce gRPC latency for testing")
	grpc_read_p50          = flag.Duration("grpc_p50_read", time.Duration(0), "Target gRPC latency for ReadRows method at p50. Valid if introduce_grpc_latency is set.")
	grpc_read_p99          = flag.Duration("grpc_p99_read", time.Duration(0), "Target gRPC latency for ReadRows method at p99. Valid if introduce_grpc_latency is set.")
	grpc_write_p50         = flag.Duration("grpc_p50_write", time.Duration(0), "Target gRPC latency for MutateRows method at p50. Valid if introduce_grpc_latency is set.")
	grpc_write_p99         = flag.Duration("grpc_p99_write", time.Duration(0), "Target gRPC latency for MutateRows method at p99. Valid if introduce_grpc_latency is set.")
)

const (
	maxMsgSize = 256 * 1024 * 1024 // 256 MiB
)

func introduceStreamLatency(srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler) error {

	start := time.Now()

	expectedLatency := time.Duration(0)
	if strings.HasSuffix(info.FullMethod, "ReadRows") {
		if rand.Int31n(100) >= 99 {
			expectedLatency = *grpc_read_p99
		} else {
			expectedLatency = *grpc_read_p50
		}
	} else if strings.HasSuffix(info.FullMethod, "MutateRows") {
		if rand.Int31n(100) >= 99 {
			expectedLatency = *grpc_write_p99
		} else {
			expectedLatency = *grpc_write_p50
		}
	}

	err := handler(srv, ss)

	time.Sleep(time.Until(start.Add(expectedLatency)))

	return err
}

func main() {
	grpc.EnableTracing = false
	flag.Parse()
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(maxMsgSize),
		grpc.MaxSendMsgSize(maxMsgSize),
	}
	if *introduce_grpc_latency {
		opts = append(opts, grpc.StreamInterceptor(introduceStreamLatency))
	}

	srv, err := bttest.NewServer(fmt.Sprintf("%s:%d", *host, *port), opts...)
	if err != nil {
		log.Fatalf("Failed to start emulator: %v", err)
	}

	fmt.Printf("Cloud Bigtable emulator running on %s\n", srv.Addr)
	select {}
}
