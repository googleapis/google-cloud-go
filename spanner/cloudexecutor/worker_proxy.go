package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	"cloud.google.com/go/spanner/cloudexecutor/executor"
	executorpb "cloud.google.com/go/spanner/cloudexecutor/proto"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

var (
	proxyPort      = flag.String("proxy_port", "", "Proxy port to start worker proxy on.")
	spannerPort    = flag.String("spanner_port", "", "Port of Spanner Frontend to which to send requests.")
	cert           = flag.String("cert", "", "Certificate used to connect to Spanner GFE.")
	serviceKeyFile = flag.String("service_key_file", "", "Service key file used to set authentication.")
)

func main() {
	if d := os.Getenv("TEST_UNDECLARED_OUTPUTS_DIR"); d != "" {
		os.Args = append(os.Args, "--log_dir="+d)
	}

	flag.Parse()
	if *proxyPort == "" {
		log.Fatal("Proxy port need to be assigned in order to start worker proxy.")
	}
	if *spannerPort == "" {
		log.Fatal("Spanner proxyPort need to be assigned in order to start worker proxy.")
	}
	if *cert == "" {
		log.Fatalf("Certificate need to be assigned in order to start worker proxy.")
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", *proxyPort))
	if err != nil {
		log.Fatal(err)
	}

	// Create a new gRPC server
	grpcServer := grpc.NewServer()

	clientOptions := getClientOptionsForSysTests()
	// Create a new cloud proxy server
	cloudProxyServer, err := executor.NewCloudProxyServer(context.Background(), clientOptions)
	if err != nil {
		log.Fatalf("Creating Cloud Proxy Server failed: %v", err)
	}
	// Register cloudProxyServer service on the grpcServer
	executorpb.RegisterSpannerExecutorProxyServer(grpcServer, cloudProxyServer)

	// Create a new service health server
	healthServer := health.NewServer()
	// Register healthServer service on the grpcServer
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)

	log.Printf("Server started on proxyPort:%s\n", *proxyPort)
	err = grpcServer.Serve(lis)
	if err != nil {
		log.Printf("Failed to start server on proxyPort: %s\n", *proxyPort)
	}
}

// Constructs client options needed to run executor for systests
func getClientOptionsForSysTests() []option.ClientOption {
	var options []option.ClientOption

	endpoint := "127.0.0.1:" + *spannerPort
	log.Printf("endpoint for grpc dial:  %s", endpoint)
	options = append(options, option.WithEndpoint(endpoint))

	creds, err := credentials.NewClientTLSFromFile(*cert, "test-cert-2")
	if err != nil {
		log.Println(err)
	}
	options = append(options, option.WithGRPCDialOption(grpc.WithTransportCredentials(creds)))

	const (
		spannerAdminScope = "https://www.googleapis.com/auth/spanner.admin"
		spannerDataScope  = "https://www.googleapis.com/auth/spanner.data"
	)

	log.Println("Reading service key file in executor code")
	cloudSystestCredentialsJson, err := os.ReadFile(*serviceKeyFile)
	if err != nil {
		log.Fatal(err)
	}
	config, err := google.JWTConfigFromJSON([]byte(cloudSystestCredentialsJson), spannerAdminScope, spannerDataScope)
	if err != nil {
		log.Println(err)
	}
	options = append(options, option.WithTokenSource(config.TokenSource(context.Background())))
	options = append(options, option.WithCredentialsFile(*serviceKeyFile))

	return options
}

type fakeTokenSource struct{}

func (f *fakeTokenSource) Token() (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: "fake token for test"}, nil
}

// Constructs client options needed to run executor for unit tests
func getClientOptionsForUnitTests() []option.ClientOption {
	var options []option.ClientOption

	endpoint := "127.0.0.1:" + *spannerPort
	log.Printf("endpoint for grpc dial:  %s", endpoint)

	options = append(options, option.WithEndpoint(endpoint))
	creds, err := credentials.NewClientTLSFromFile(*cert, "test-cert-2")
	if err != nil {
		log.Println(err)
	}

	options = append(options, option.WithGRPCDialOption(grpc.WithTransportCredentials(creds)))
	options = append(options, option.WithTokenSource(&fakeTokenSource{}))

	return options
}

// Constructs client options needed to run executor on local machine
func getClientOptionsForLocalTest() []option.ClientOption {
	var options []option.ClientOption
	return options
}
