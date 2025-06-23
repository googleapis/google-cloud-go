package smoketests

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	bigquery "cloud.google.com/go/bigquery/v2/apiv2"
	"cloud.google.com/go/bigquery/v2/apiv2_client"

	"cloud.google.com/go/internal/testutil"
	"google.golang.org/api/option"
)

var testClients map[string]*apiv2_client.Client
var testProjectID string
var defaultTestTimeout = 30 * time.Second

func TestMain(m *testing.M) {
	err := setup(context.Background())
	if err != nil {
		log.Printf("failure setting up test environment, skipping test execution: %v", err)
		os.Exit(1)
	}
	code := m.Run()
	shutdown()
	os.Exit(code)
}

func setup(ctx context.Context) error {
	projID := testutil.ProjID()
	if projID == "" {
		log.Fatal("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	testProjectID = projID
	ts := testutil.TokenSource(ctx, bigquery.DefaultAuthScopes()...)
	if ts == nil {
		log.Fatal("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	var opts []option.ClientOption
	opts = append(opts, option.WithTokenSource(ts))
	testClients = make(map[string]*apiv2_client.Client)
	var err error

	testClients["GRPC"], err = apiv2_client.NewClient(ctx, opts...)
	if err != nil {
		return err
	}
	//opts = append(opts, option.WithHTTPClient(&c))
	testClients["REST"], err = apiv2_client.NewRESTClient(ctx, opts...)
	if err != nil {
		testClients["GRPC"].Close()
		return err
	}
	return nil
}

func shutdown() {
	for k, v := range testClients {
		if err := v.Close(); err != nil {
			log.Printf("closing client %q had error: %v", k, err)
		}
	}
}
