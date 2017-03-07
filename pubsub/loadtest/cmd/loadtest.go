package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"strconv"

	"math/rand"

	"cloud.google.com/go/pubsub/loadtest"
	pb "cloud.google.com/go/pubsub/loadtest/pb"
	"google.golang.org/grpc"
)

func main() {
	port := flag.Uint("worker_port", 6000, "port to bind worker to")
	flag.Parse()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	serv := grpc.NewServer()
	pb.RegisterLoadtestWorkerServer(serv, &loadtest.Server{
		ID: strconv.Itoa(rand.Int()),
	})
	serv.Serve(lis)
}
