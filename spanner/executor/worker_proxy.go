package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	"cloud.google.com/go/spanner/executor/apiv1/executorpb"
	"cloud.google.com/go/spanner/executor/executor"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

var (
	proxyPort               = flag.String("proxy_port", "", "Proxy port to start worker proxy on.")
	spannerPort             = flag.String("spanner_port", "", "Port of Spanner Frontend to which to send requests.")
	cert                    = flag.String("cert", "", "Certificate used to connect to Spanner GFE.")
	serviceKeyFile          = flag.String("service_key_file", "", "Service key file used to set authentication.")
	usePlainTextChannel     = flag.String("use_plain_text_channel", "", "Use a plain text gRPC channel (intended for the Cloud Spanner Emulator).")
	enableGrpcFaultInjector = flag.String("enable_grpc_fault_injector", "", "Enable grpc fault injector in cloud client executor")
)

func main() {
	if d := os.Getenv("TEST_UNDECLARED_OUTPUTS_DIR"); d != "" {
		os.Args = append(os.Args, "--log_dir="+d)
	}

	flag.Parse()
	// Print "port:<number>" to STDOUT for the systest worker.
	if *proxyPort == "" {
		log.Fatalf("usage: %s --proxy_port=8081", os.Args[0])
	}
	fmt.Printf("Running on port:%s\n", *proxyPort)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", *proxyPort))
	if err != nil {
		log.Fatal(err)
	}

	i, err := executor.NewCloudProxyServer(context.Background(), getClientOptions())
	if err != nil {
		log.Fatalf("NewCloudProxyServer failed: %v", err)
	}

	s := grpc.NewServer()
	executorpb.RegisterSpannerExecutorProxyServer(s, i)

	log.Fatal(s.Serve(lis))
}

func getClientOptions() []option.ClientOption {
	var options []option.ClientOption

	if *spannerPort != "" {
		endpoint := "https://localhost:" + *spannerPort
		options = append(options, option.WithEndpoint(endpoint))
	}
	if *serviceKeyFile != "" {
		options = append(options, option.WithCredentialsFile(*serviceKeyFile))
	}
	return options
}
