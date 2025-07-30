/*
Copyright 2017 Google LLC

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
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	itestutil "cloud.google.com/go/internal/testutil"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/GoogleCloudPlatform/grpc-gcp-go/grpcgcp"
	"github.com/GoogleCloudPlatform/grpc-gcp-go/grpcgcp/grpc_gcp"
	"github.com/GoogleCloudPlatform/grpc-gcp-go/grpcgcp/multiendpoint"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/googleapis/gax-go/v2"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	structpb "google.golang.org/protobuf/types/known/structpb"

	vkit "cloud.google.com/go/spanner/apiv1"
	. "cloud.google.com/go/spanner/internal/testutil"
)

var useGRPCgcp = strings.ToLower(os.Getenv("GCLOUD_TESTS_GOLANG_USE_GRPC_GCP")) == "true"

func setupMockedTestServer(t *testing.T) (server *MockedSpannerInMemTestServer, client *Client, teardown func()) {
	l := log.Default()
	l.SetOutput(io.Discard)
	return setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, Logger: l})
}

func setupMockedTestServerWithConfig(t *testing.T, config ClientConfig) (server *MockedSpannerInMemTestServer, client *Client, teardown func()) {
	return setupMockedTestServerWithConfigAndClientOptions(t, config, []option.ClientOption{})
}

func setupMockedTestServerWithConfigAndClientOptions(t *testing.T, config ClientConfig, clientOptions []option.ClientOption) (server *MockedSpannerInMemTestServer, client *Client, teardown func()) {
	return setupMockedTestServerWithConfigAndGCPMultiendpointPool(t, config, clientOptions, nil)
}

func setupMockedTestServerWithConfigAndGCPMultiendpointPool(t *testing.T, config ClientConfig, clientOptions []option.ClientOption, poolCfg *grpc_gcp.ChannelPoolConfig) (server *MockedSpannerInMemTestServer, client *Client, teardown func()) {
	grpcHeaderChecker := &itestutil.HeadersEnforcer{
		OnFailure: t.Fatalf,
		Checkers: []*itestutil.HeaderChecker{
			{
				Key: "x-goog-api-client",
				ValuesValidator: func(token ...string) error {
					if len(token) != 1 {
						return status.Errorf(codes.Internal, "unexpected number of api client token headers: %v", len(token))
					}
					if !strings.Contains(token[0], "gl-go/") {
						return status.Errorf(codes.Internal, "unexpected api client token: %v", token[0])
					}
					if !strings.Contains(token[0], "gccl/") {
						return status.Errorf(codes.Internal, "unexpected api client token: %v", token[0])
					}
					return nil
				},
			},
		},
	}
	expectedResourceHeaderFormat := regexp.MustCompile("projects/.+/instances/.+/databases/.+.*")
	grpcHeaderChecker.Checkers = append(grpcHeaderChecker.Checkers, &itestutil.HeaderChecker{
		Key: resourcePrefixHeader,
		ValuesValidator: func(token ...string) error {
			if len(token) != 1 {
				return status.Errorf(codes.Internal, "unexpected number of resource headers: %v", len(token))
			}
			if !expectedResourceHeaderFormat.MatchString(token[0]) {
				return status.Errorf(codes.Internal, "invalid resource header value: %v", token[0])
			}
			return nil
		},
	})
	if config.Compression == gzip.Name {
		grpcHeaderChecker.Checkers = append(grpcHeaderChecker.Checkers, &itestutil.HeaderChecker{
			Key: "x-response-encoding",
			ValuesValidator: func(token ...string) error {
				if len(token) != 1 {
					return status.Errorf(codes.Internal, "unexpected number of compression headers: %v", len(token))
				}
				if token[0] != gzip.Name {
					return status.Errorf(codes.Internal, "unexpected compression: %v", token[0])
				}
				return nil
			},
		})
	}
	clientOptions = append(clientOptions, grpcHeaderChecker.CallOptions()...)
	server, opts, serverTeardown := NewMockedSpannerInMemTestServer(t)
	opts = append(opts, clientOptions...)
	ctx := context.Background()
	formattedDatabase := fmt.Sprintf("projects/%s/instances/%s/databases/%s", "[PROJECT]", "[INSTANCE]", "[DATABASE]")
	var err error
	if useGRPCgcp {
		gmeCfg := &grpcgcp.GCPMultiEndpointOptions{
			GRPCgcpConfig: &grpc_gcp.ApiConfig{
				ChannelPool: poolCfg,
			},
			MultiEndpoints: map[string]*multiendpoint.MultiEndpointOptions{
				"default": {
					Endpoints: []string{server.ServerAddress},
				},
			},
			Default: "default",
		}
		client, _, err = NewMultiEndpointClientWithConfig(ctx, formattedDatabase, config, gmeCfg, opts...)
	} else {
		client, err = NewClientWithConfig(ctx, formattedDatabase, config, opts...)
	}
	if err != nil {
		t.Fatal(err)
	}
	if isMultiplexEnabled || config.enableMultiplexSession {
		waitFor(t, func() error {
			client.idleSessions.mu.Lock()
			defer client.idleSessions.mu.Unlock()
			if client.idleSessions.multiplexedSession == nil {
				return errInvalidSessionPool
			}
			return nil
		})
	}
	return server, client, func() {
		client.Close()
		serverTeardown()
	}
}

func setupMockedTestServerWithoutWaitingForMultiplexedSessionInit(t *testing.T) (server *MockedSpannerInMemTestServer, client *Client, teardown func()) {
	config := ClientConfig{DisableNativeMetrics: true}
	clientOptions := []option.ClientOption{}
	var poolCfg *grpc_gcp.ChannelPoolConfig
	grpcHeaderChecker := &itestutil.HeadersEnforcer{
		OnFailure: t.Fatalf,
		Checkers: []*itestutil.HeaderChecker{
			{
				Key: "x-goog-api-client",
				ValuesValidator: func(token ...string) error {
					if len(token) != 1 {
						return status.Errorf(codes.Internal, "unexpected number of api client token headers: %v", len(token))
					}
					if !strings.HasPrefix(token[0], "gl-go/") {
						return status.Errorf(codes.Internal, "unexpected api client token: %v", token[0])
					}
					if !strings.Contains(token[0], "gccl/") {
						return status.Errorf(codes.Internal, "unexpected api client token: %v", token[0])
					}
					return nil
				},
			},
		},
	}
	if config.Compression == gzip.Name {
		grpcHeaderChecker.Checkers = append(grpcHeaderChecker.Checkers, &itestutil.HeaderChecker{
			Key: "x-response-encoding",
			ValuesValidator: func(token ...string) error {
				if len(token) != 1 {
					return status.Errorf(codes.Internal, "unexpected number of compression headers: %v", len(token))
				}
				if token[0] != gzip.Name {
					return status.Errorf(codes.Internal, "unexpected compression: %v", token[0])
				}
				return nil
			},
		})
	}
	clientOptions = append(clientOptions, grpcHeaderChecker.CallOptions()...)
	server, opts, serverTeardown := NewMockedSpannerInMemTestServer(t)
	opts = append(opts, clientOptions...)
	ctx := context.Background()
	formattedDatabase := fmt.Sprintf("projects/%s/instances/%s/databases/%s", "[PROJECT]", "[INSTANCE]", "[DATABASE]")
	var err error
	if useGRPCgcp {
		gmeCfg := &grpcgcp.GCPMultiEndpointOptions{
			GRPCgcpConfig: &grpc_gcp.ApiConfig{
				ChannelPool: poolCfg,
			},
			MultiEndpoints: map[string]*multiendpoint.MultiEndpointOptions{
				"default": {
					Endpoints: []string{server.ServerAddress},
				},
			},
			Default: "default",
		}
		client, _, err = NewMultiEndpointClientWithConfig(ctx, formattedDatabase, config, gmeCfg, opts...)
	} else {
		client, err = NewClientWithConfig(ctx, formattedDatabase, config, opts...)
	}
	if err != nil {
		t.Fatal(err)
	}
	return server, client, func() {
		client.Close()
		serverTeardown()
	}
}

func makeClient(ctx context.Context, database string, target string, opts ...option.ClientOption) (*Client, error) {
	if !useGRPCgcp {
		return NewClient(ctx, database, opts...)
	}
	c, _, err := NewMultiEndpointClient(
		ctx,
		database,
		&grpcgcp.GCPMultiEndpointOptions{
			MultiEndpoints: map[string]*multiendpoint.MultiEndpointOptions{
				"default": {
					Endpoints: []string{target},
				},
			},
			Default: "default",
		},
		opts...,
	)
	return c, err
}

func makeClientWithConfig(ctx context.Context, database string, config ClientConfig, target string, opts ...option.ClientOption) (*Client, error) {
	if !useGRPCgcp {
		return NewClientWithConfig(ctx, database, config, opts...)
	}
	c, _, err := NewMultiEndpointClientWithConfig(
		ctx,
		database,
		config,
		&grpcgcp.GCPMultiEndpointOptions{
			MultiEndpoints: map[string]*multiendpoint.MultiEndpointOptions{
				"default": {
					Endpoints: []string{target},
				},
			},
			Default: "default",
		},
		opts...,
	)
	return c, err
}

// Test validDatabaseName()
func TestValidDatabaseName(t *testing.T) {
	validDbURI := "projects/spanner-cloud-test/instances/foo/databases/foodb"
	invalidDbUris := []string{
		// Completely wrong DB URI.
		"foobarDB",
		// Project ID contains "/".
		"projects/spanner-cloud/test/instances/foo/databases/foodb",
		// No instance ID.
		"projects/spanner-cloud-test/instances//databases/foodb",
	}
	if err := validDatabaseName(validDbURI); err != nil {
		t.Errorf("validateDatabaseName(%q) = %v, want nil", validDbURI, err)
	}
	for _, d := range invalidDbUris {
		if err, wantErr := validDatabaseName(d), "should conform to pattern"; !strings.Contains(err.Error(), wantErr) {
			t.Errorf("validateDatabaseName(%q) = %q, want error pattern %q", validDbURI, err, wantErr)
		}
	}
}

func TestReadOnlyTransactionClose(t *testing.T) {
	// Closing a ReadOnlyTransaction shouldn't panic.
	c := &Client{}
	tx := c.ReadOnlyTransaction()
	tx.Close()
}

func TestClient_MultiEndpoint(t *testing.T) {
	if !useGRPCgcp {
		t.Skip("gRPC-GCP only test")
	}
	t.Parallel()

	server, opts, serverTeardown := NewMockedSpannerInMemTestServerWithAddr(t, "localhost:0")
	defer serverTeardown()

	mirrorAvailable := true
	connCount := uint32(0)

	makeMirror := func(enable *bool) string {
		lis, err := net.Listen("tcp", "localhost:0")
		if err != nil {
			t.Fatal(err)
		}

		proxy := func(connA, connB net.Conn) {
			buf := make([]byte, 1024)
			for {
				n, err := connA.Read(buf)
				if !*enable || err == io.EOF {
					connA.Close()
					return
				}
				if err != nil {
					t.Logf("error reading from conn: %v", err)
					return
				}
				_, err = connB.Write(buf[:n])
				if err != nil {
					t.Logf("error writing to conn: %v", err)
					return
				}
			}
		}

		handleConn := func(c net.Conn) {
			if !*enable {
				c.Close()
				return
			}

			// Open connection to the mocked server.
			conn, err := net.Dial("tcp", server.ServerAddress)
			if err != nil {
				t.Logf("cannot open connection: %v", err)
				return
			}

			// Close connections when mirror is disabled.
			go func() {
				for *enable {
					time.Sleep(time.Millisecond * 5)
				}
				c.Close()
				conn.Close()
			}()

			go proxy(c, conn)
			go proxy(conn, c)

			atomic.AddUint32(&connCount, 1)
		}

		// Serve.
		go func() {
			for {
				c, err := lis.Accept()
				if err != nil {
					t.Logf("cannot accept connection: %v", err)
					return
				}
				go handleConn(c)
			}
		}()

		return lis.Addr().String()
	}

	mirrorAddress := makeMirror(&mirrorAvailable)

	stable := true
	stableMirrorAddress := makeMirror(&stable)

	// Configuring MultiEndpoint with two endpoints.
	gmeCfg := &grpcgcp.GCPMultiEndpointOptions{
		MultiEndpoints: map[string]*multiendpoint.MultiEndpointOptions{
			"default": {
				Endpoints: []string{
					mirrorAddress,
					stableMirrorAddress,
				},
			},
		},
		Default: "default",
	}

	ctx := context.Background()
	formattedDatabase := fmt.Sprintf("projects/%s/instances/%s/databases/%s", "[PROJECT]", "[INSTANCE]", "[DATABASE]")
	client, gme, err := NewMultiEndpointClient(ctx, formattedDatabase, gmeCfg, opts...)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Let both endpoints connect.
	for atomic.LoadUint32(&connCount) < numChannels*2 {
		time.Sleep(time.Millisecond * 5)
	}

	// Works via mirror.
	err = executeSingerQueryWithTimeout(ctx, client.Single(), time.Second)
	if err != nil {
		t.Fatal(err)
	}

	// Breaking the mirror.
	mirrorAvailable = false

	// Let some time to detect breakage.
	time.Sleep(time.Millisecond * 20)

	// Should work via stable mirror.
	err = executeSingerQueryWithTimeout(ctx, client.Single(), time.Second)
	if err != nil {
		t.Fatal(err)
	}

	// Reversing the order of endpoints.
	gmeCfg = &grpcgcp.GCPMultiEndpointOptions{
		MultiEndpoints: map[string]*multiendpoint.MultiEndpointOptions{
			"default": {
				Endpoints: []string{
					stableMirrorAddress,
					mirrorAddress,
				},
			},
		},
		Default: "default",
	}
	if err := gme.UpdateMultiEndpoints(gmeCfg); err != nil {
		t.Fatal(err)
	}

	// Should work in reverse order.
	err = executeSingerQueryWithTimeout(ctx, client.Single(), time.Second)
	if err != nil {
		t.Fatal(err)
	}

	// Moving the stable endpoint to a different MultiEndpoint.
	gmeCfg = &grpcgcp.GCPMultiEndpointOptions{
		MultiEndpoints: map[string]*multiendpoint.MultiEndpointOptions{
			"default": {
				Endpoints: []string{
					mirrorAddress,
				},
			},
			"stable": {
				Endpoints: []string{
					stableMirrorAddress,
				},
			},
		},
		Default: "default",
	}
	if err := gme.UpdateMultiEndpoints(gmeCfg); err != nil {
		t.Fatal(err)
	}

	// Should fail as the mirror is the only endpoint and it is broken.
	err = executeSingerQueryWithTimeout(ctx, client.Single(), time.Millisecond*100)
	if err == nil {
		t.Fatalf("deadline exceeded error expected, got: %v", err)
	}

	// Should work via stable MultiEndpoint.
	stableCtx := grpcgcp.NewMEContext(ctx, "stable")
	err = executeSingerQueryWithTimeout(stableCtx, client.Single(), time.Second)
	if err != nil {
		t.Fatal(err)
	}

	// Restoring the mirror.
	mirrorAvailable = true

	// Should work via the mirror again by default.
	err = executeSingerQueryWithTimeout(ctx, client.Single(), time.Second*3)
	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_MultiplexedSession(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name     string
		test     func(client *Client) error
		validate func(server InMemSpannerServer)
		wantErr  error
	}{
		{
			name: "Given if multiplexed session is enabled, When executing single use R/O transactions, should use multiplexed session",
			test: func(client *Client) error {
				ctx := context.Background()
				// Test the single use read-only transaction
				_, err := client.Single().ReadRow(ctx, "Albums", Key{"foo"}, []string{"SingerId", "AlbumId", "AlbumTitle"})
				return err
			},
			validate: func(server InMemSpannerServer) {
				// Validate the multiplexed session is used
				expectedSessionCount := uint(1)
				if !isMultiplexEnabled {
					expectedSessionCount = uint(25) // BatchCreateSession request from regular session pool
				}
				if !testEqual(expectedSessionCount, server.TotalSessionsCreated()) {
					t.Errorf("TestClient_MultiplexedSession expected session creation with multiplexed=%s should be=%v, got: %v", strconv.FormatBool(isMultiplexEnabled), expectedSessionCount, server.TotalSessionsCreated())
				}
				reqs := drainRequestsFromServer(server)
				for _, s := range reqs {
					switch req := s.(type) {
					case *sppb.ReadRequest:
						// Validate the session is multiplexed
						if !testEqual(isMultiplexEnabled, strings.Contains(req.Session, "multiplexed")) {
							t.Errorf("TestClient_MultiplexedSession expected multiplexed session to be used, got: %v", req.Session)
						}

					}
				}
			},
		},
		{
			name: "Given if multiplexed session is enabled, When executing multi use R/O transactions, should use multiplexed session",
			test: func(client *Client) error {
				ctx := context.Background()
				// Test the multi use read-only transaction
				roTxn := client.ReadOnlyTransaction()
				defer roTxn.Close()
				iter := roTxn.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
				if err := iter.Do(func(row *Row) error {
					return nil
				}); err != nil {
					return err
				}
				iter = roTxn.Read(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"})
				return iter.Do(func(row *Row) error {
					return nil
				})
			},
			validate: func(server InMemSpannerServer) {
				// Validate the multiplexed session is used
				expectedSessionCount := uint(1)
				if !isMultiplexEnabled {
					expectedSessionCount = uint(25) // BatchCreateSession request from regular session pool
				}
				if !testEqual(expectedSessionCount, server.TotalSessionsCreated()) {
					t.Errorf("TestClient_MultiplexedSession expected session creation with multiplexed=%s should be=%v, got: %v", strconv.FormatBool(isMultiplexEnabled), expectedSessionCount, server.TotalSessionsCreated())
				}
				reqs := drainRequestsFromServer(server)
				for _, s := range reqs {
					switch req := s.(type) {
					case *sppb.ReadRequest:
						// Validate the session is multiplexed
						if !testEqual(isMultiplexEnabled, strings.Contains(req.Session, "multiplexed")) {
							t.Errorf("TestClient_MultiplexedSession expected multiplexed session to be used, got: %v", req.Session)
						}
					case *sppb.ExecuteSqlRequest:
						// Validate the session is multiplexed
						if !testEqual(isMultiplexEnabled, strings.Contains(req.Session, "multiplexed")) {
							t.Errorf("TestClient_MultiplexedSession expected multiplexed session to be used, got: %v", req.Session)
						}
					}

				}
			},
		},
		{
			name: "Given if multiplexed session is enabled, When executing R/W transactions, should always use regular session",
			test: func(client *Client) error {
				ctx := context.Background()
				// Test the read-write transaction
				_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *ReadWriteTransaction) error {
					iter := txn.Read(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"})
					return iter.Do(func(r *Row) error {
						return nil
					})
				})
				return err
			},
			validate: func(server InMemSpannerServer) {
				// Validate the regular session is used, toatl session created should be 25
				expectedSessionCount := uint(26)
				if !isMultiplexEnabled {
					expectedSessionCount = uint(25) // BatchCreateSession request from regular session pool
				}
				if !testEqual(expectedSessionCount, server.TotalSessionsCreated()) {
					t.Errorf("TestClient_MultiplexedSession expected session creation with multiplexed=%s should be=%v, got: %v", strconv.FormatBool(isMultiplexEnabled), expectedSessionCount, server.TotalSessionsCreated())
				}
				reqs := drainRequestsFromServer(server)
				for _, s := range reqs {
					switch req := s.(type) {
					case *sppb.ReadRequest:
						// Validate the session is not multiplexed
						if !testEqual(false, strings.Contains(req.Session, "multiplexed")) {
							t.Errorf("TestClient_MultiplexedSession expected multiplexed session to be used, got: %v", req.Session)
						}
					}
				}
			},
		},
		{
			name: "Given if multiplexed session is enabled, Only one multiplex session should be created for multiple read only transactions",
			test: func(client *Client) error {
				// Test the parallel single use read-only transaction
				g := new(errgroup.Group)
				for i := 0; i < 25; i++ {
					g.Go(func() error {
						ctx := context.Background()
						// Test the single use read-only transaction
						_, err := client.Single().ReadRow(ctx, "Albums", Key{"foo"}, []string{"SingerId", "AlbumId", "AlbumTitle"})
						return err
					})
				}
				return g.Wait()
			},
			validate: func(server InMemSpannerServer) {
				// Validate the multiplexed session is used
				expectedSessionCount := uint(1)
				if !isMultiplexEnabled {
					expectedSessionCount = uint(25) // BatchCreateSession request from regular session pool
				}
				if !testEqual(expectedSessionCount, server.TotalSessionsCreated()) {
					t.Errorf("TestClient_MultiplexedSession expected session creation with multiplexed=%s should be=%v, got: %v", strconv.FormatBool(isMultiplexEnabled), expectedSessionCount, server.TotalSessionsCreated())
				}
				reqs := drainRequestsFromServer(server)
				for _, s := range reqs {
					switch req := s.(type) {
					case *sppb.ReadRequest:
						// Verify that a multiplexed session is used when that is enabled.
						if !testEqual(isMultiplexEnabled, strings.Contains(req.Session, "multiplexed")) {
							t.Errorf("TestClient_MultiplexedSession expected multiplexed session to be used, got: %v", req.Session)
						}
					}
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, client, teardown := setupMockedTestServer(t)
			defer teardown()
			gotErr := tt.test(client)
			if !testEqual(gotErr, tt.wantErr) {
				t.Errorf("TestClient_MultiplexedSession error=%v, wantErr: %v", gotErr, tt.wantErr)
			} else {
				tt.validate(server.TestSpanner)
			}
		})
	}
}

func TestClient_Single(t *testing.T) {
	t.Parallel()
	err := testSingleQuery(t, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_NonConformingHeader(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	md := metadata.Pairs(resourcePrefixHeader, "projects/foo/documents/bar")
	ctx = metadata.NewOutgoingContext(ctx, md)
	err := testSingleQueryWithContext(ctx, t, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Verify that even though the request is sent without the non-conforming header,
	// it is still present in the original context.
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		header := md.Get(resourcePrefixHeader)
		if g, w := len(header), 1; g != w {
			t.Fatalf("header length mismatch\n Got: %v\nWant: %v", g, w)
		}
		if g, w := header[0], "projects/foo/documents/bar"; g != w {
			t.Fatalf("header mismatch\n Got: %v\nWant: %v", g, w)
		}
	} else {
		t.Fatal("could not get metadata from context")
	}
}

func TestClient_Single_Unavailable(t *testing.T) {
	t.Parallel()
	err := testSingleQuery(t, serverErrorWithMinimalRetryDelay(codes.Unavailable, "Temporary unavailable"))
	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_Single_ResourceExhausted(t *testing.T) {
	t.Parallel()
	err := testSingleQuery(t, serverErrorWithMinimalRetryDelay(codes.ResourceExhausted, "Temporary server overload"))
	if err != nil {
		t.Fatal(err)
	}
}

func serverErrorWithMinimalRetryDelay(code codes.Code, msg string) error {
	st := status.New(code, msg)
	retry := &errdetails.RetryInfo{
		RetryDelay: durationpb.New(time.Nanosecond),
	}
	st, _ = st.WithDetails(retry)
	return st.Err()
}

func TestClient_Single_InvalidArgument(t *testing.T) {
	t.Parallel()
	err := testSingleQuery(t, status.Error(codes.InvalidArgument, "Invalid argument"))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("got: %v, want: %v", err, codes.InvalidArgument)
	}
}

func TestClient_Single_SessionNotFound(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodExecuteStreamingSql,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)
	ctx := context.Background()
	iter := client.Single().Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	defer iter.Stop()
	rowCount := int64(0)
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		rowCount++
	}
	if rowCount != SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount {
		t.Fatalf("row count mismatch\nGot: %v\nWant: %v", rowCount, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
	}
}

func TestClient_Single_Read_SessionNotFound(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodStreamingRead,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)
	ctx := context.Background()
	iter := client.Single().Read(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"})
	defer iter.Stop()
	rowCount := int64(0)
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		rowCount++
	}
	if rowCount != SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount {
		t.Fatalf("row count mismatch\nGot: %v\nWant: %v", rowCount, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
	}
}

func TestClient_Single_WhenInactiveTransactionsAndSessionIsNotFoundOnBackend_RemoveSessionFromPool(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened: 1,
			MaxOpened: 1,
			InactiveTransactionRemovalOptions: InactiveTransactionRemovalOptions{
				ActionOnInactiveTransaction: WarnAndClose,
			},
		},
	})
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodExecuteStreamingSql,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)
	ctx := context.Background()
	single := client.Single()
	iter := single.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	p := client.idleSessions
	sh := single.sh
	// simulate session to be last used before 60 mins
	sh.mu.Lock()
	sh.lastUseTime = time.Now().Add(-time.Hour)
	sh.mu.Unlock()

	// force run task to clean up unexpected long-running sessions
	p.removeLongRunningSessions()
	rowCount := int64(0)
	for {
		// Backend throws SessionNotFoundError. Session gets replaced with new session
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		rowCount++
	}
	// New session returns back to pool
	iter.Stop()

	p.mu.Lock()
	defer p.mu.Unlock()
	if g, w := p.idleList.Len(), 1; g != w {
		t.Fatalf("Idle Sessions in pool, count mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	if g, w := p.numInUse, uint64(0); g != w {
		t.Fatalf("Number of sessions currently in use mismatch\nGot: %d\nWant: %d\n", g, w)
	}
	if g, w := p.numOpened, uint64(1); g != w {
		t.Fatalf("Session pool size mismatch\nGot: %d\nWant: %d\n", g, w)
	}

	sh.mu.Lock()
	defer sh.mu.Unlock()
	if g, w := sh.eligibleForLongRunning, false; g != w {
		t.Fatalf("isLongRunningTransaction mismatch\nGot: %v\nWant: %v\n", g, w)
	}
	expectedLeakedSessions := uint64(1)
	if isMultiplexEnabled {
		expectedLeakedSessions = 0
	}
	if g, w := p.numOfLeakedSessionsRemoved, expectedLeakedSessions; g != w {
		t.Fatalf("Number of leaked sessions removed mismatch\nGot: %d\nWant: %d\n", g, w)
	}
}

func TestClient_Single_ReadRow_SessionNotFound(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodStreamingRead,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)
	ctx := context.Background()
	row, err := client.Single().ReadRow(ctx, "Albums", Key{"foo"}, []string{"SingerId", "AlbumId", "AlbumTitle"})
	if err != nil {
		t.Fatalf("Unexpected error for read row: %v", err)
	}
	if row == nil {
		t.Fatal("ReadRow did not return a row")
	}
}

func TestClient_Single_RetryableErrorOnPartialResultSet(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	// Add two errors that will be returned by the mock server when the client
	// is trying to fetch a partial result set. Both errors are retryable.
	// The errors are not 'sticky' on the mocked server, i.e. once the error
	// has been returned once, the next call for the same partial result set
	// will succeed.

	// When the client is fetching the partial result set with resume token 2,
	// the mock server will respond with an internal error with the message
	// 'stream terminated by RST_STREAM'. The client will retry the call to get
	// this partial result set.
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(2),
			Err:         status.Errorf(codes.Internal, "stream terminated by RST_STREAM"),
		},
	)
	// When the client is fetching the partial result set with resume token 3,
	// the mock server will respond with a 'Unavailable' error. The client will
	// retry the call to get this partial result set.
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(3),
			Err:         status.Errorf(codes.Unavailable, "server is unavailable"),
		},
	)
	ctx := context.Background()
	if err := executeSingerQuery(ctx, client.Single()); err != nil {
		t.Fatal(err)
	}
}

func TestClient_Single_NonRetryableErrorOnPartialResultSet(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	// Add two errors that will be returned by the mock server when the client
	// is trying to fetch a partial result set. The first error is retryable,
	// the second is not.

	// This error will automatically be retried.
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(2),
			Err:         status.Errorf(codes.Internal, "stream terminated by RST_STREAM"),
		},
	)
	// 'Session not found' is not retryable and the error will be returned to
	// the user.
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(3),
			Err:         newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s"),
		},
	)
	ctx := context.Background()
	err := executeSingerQuery(ctx, client.Single())
	if status.Code(err) != codes.NotFound {
		t.Fatalf("Error mismatch:\ngot: %v\nwant: %v", err, codes.NotFound)
	}
}

func TestClient_Single_NonRetryableInternalErrors(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(2),
			Err:         status.Errorf(codes.Internal, "grpc: error while marshaling: string field contains invalid UTF-8"),
		},
	)
	ctx := context.Background()
	err := executeSingerQuery(ctx, client.Single())
	if status.Code(err) != codes.Internal {
		t.Fatalf("Error mismatch:\ngot: %v\nwant: %v", err, codes.Internal)
	}
}

func TestClient_Single_DeadlineExceeded_NoErrors(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql,
		SimulatedExecutionTime{
			MinimumExecutionTime: 50 * time.Millisecond,
		})
	ctx := context.Background()
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(5*time.Millisecond))
	defer cancel()
	err := executeSingerQuery(ctx, client.Single())
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("Error mismatch:\ngot: %v\nwant: %v", err, codes.DeadlineExceeded)
	}
}

func TestClient_Single_DeadlineExceeded_WithErrors(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(2),
			Err:         status.Errorf(codes.Internal, "stream terminated by RST_STREAM"),
		},
	)
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken:   EncodeResumeToken(3),
			Err:           status.Errorf(codes.Unavailable, "server is unavailable"),
			ExecutionTime: 50 * time.Millisecond,
		},
	)
	ctx := context.Background()
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(25*time.Millisecond))
	defer cancel()
	err := executeSingerQuery(ctx, client.Single())
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("got unexpected error %v, expected DeadlineExceeded", err)
	}
}

func TestClient_Single_ContextCanceled_noDeclaredServerErrors(t *testing.T) {
	t.Parallel()
	_, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	err := executeSingerQuery(ctx, client.Single())
	if status.Code(err) != codes.Canceled {
		t.Fatalf("got unexpected error %v, expected Canceled", err)
	}
}

func TestClient_Single_ContextCanceled_withDeclaredServerErrors(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(2),
			Err:         status.Errorf(codes.Internal, "stream terminated by RST_STREAM"),
		},
	)
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(3),
			Err:         status.Errorf(codes.Unavailable, "server is unavailable"),
		},
	)
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	f := func(rowCount int64) error {
		if rowCount == 2 {
			cancel()
		}
		return nil
	}
	iter := client.Single().Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	defer iter.Stop()
	err := executeSingerQueryWithRowFunc(ctx, client.Single(), f)
	if status.Code(err) != codes.Canceled {
		t.Fatalf("got unexpected error %v, expected Canceled", err)
	}
}

func TestClient_Single_QueryOptions(t *testing.T) {
	for _, tt := range queryOptionsTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env.Options != nil {
				unset := setQueryOptionsEnvVars(tt.env.Options)
				defer unset()
			}

			ctx := context.Background()
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, QueryOptions: tt.client})
			defer teardown()

			var iter *RowIterator
			if tt.query.Options == nil {
				iter = client.Single().Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
			} else {
				iter = client.Single().QueryWithOptions(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), tt.query)
			}
			testQueryOptions(t, iter, server.TestSpanner, tt.want)
		})
	}
}

func TestClient_Single_ReadOptions(t *testing.T) {
	for _, tt := range readOptionsTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, ReadOptions: *tt.client})
			defer teardown()

			var iter *RowIterator
			if tt.read == nil {
				iter = client.Single().Read(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"})
			} else {
				iter = client.Single().ReadWithOptions(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"}, tt.read)
			}
			testReadOptions(t, iter, server.TestSpanner, *tt.want)
		})
	}
}

func TestClient_ReturnDatabaseName(t *testing.T) {
	t.Parallel()

	_, client, teardown := setupMockedTestServer(t)
	defer teardown()

	got := client.DatabaseName()
	want := "projects/[PROJECT]/instances/[INSTANCE]/databases/[DATABASE]"
	if got != want {
		t.Fatalf("Incorrect database name returned, got: %s, want: %s", got, want)
	}
}

func testQueryOptions(t *testing.T, iter *RowIterator, server InMemSpannerServer, qo QueryOptions) {
	defer iter.Stop()

	_, err := iter.Next()
	if err != nil {
		t.Fatalf("Failed to read from the iterator: %v", err)
	}

	checkReqsForQueryOptions(t, server, qo)
}

func checkReqsForQueryOptions(t *testing.T, server InMemSpannerServer, qo QueryOptions) {
	reqs := drainRequestsFromServer(server)
	sqlReqs := []*sppb.ExecuteSqlRequest{}

	for _, req := range reqs {
		if sqlReq, ok := req.(*sppb.ExecuteSqlRequest); ok {
			sqlReqs = append(sqlReqs, sqlReq)
		}
	}

	if got, want := len(sqlReqs), 1; got != want {
		t.Fatalf("Length mismatch, got %v, want %v", got, want)
	}

	sqlReq := sqlReqs[0]
	reqQueryOptions := sqlReq.QueryOptions
	if got, want := reqQueryOptions.OptimizerVersion, qo.Options.OptimizerVersion; got != want {
		t.Fatalf("Optimizer version mismatch, got %v, want %v", got, want)
	}
	if got, want := reqQueryOptions.OptimizerStatisticsPackage, qo.Options.OptimizerStatisticsPackage; got != want {
		t.Fatalf("Optimizer statistics package mismatch, got %v, want %v", got, want)
	}
	if got, want := sqlReq.DirectedReadOptions, qo.DirectedReadOptions; got.String() != want.String() {
		t.Fatalf("Directed Read Options mismatch, got %v, want %v", got, want)
	}
}

func testReadOptions(t *testing.T, iter *RowIterator, server InMemSpannerServer, ro ReadOptions) {
	defer iter.Stop()

	_, err := iter.Next()
	if err != nil {
		t.Fatalf("Failed to read from the iterator: %v", err)
	}

	checkReqsForReadOptions(t, server, ro)
}

func checkReqsForReadOptions(t *testing.T, server InMemSpannerServer, ro ReadOptions) {
	reqs := drainRequestsFromServer(server)
	sqlReqs := []*sppb.ReadRequest{}

	for _, req := range reqs {
		if sqlReq, ok := req.(*sppb.ReadRequest); ok {
			sqlReqs = append(sqlReqs, sqlReq)
		}
	}

	if got, want := len(sqlReqs), 1; got != want {
		t.Fatalf("Length mismatch, got %v, want %v", got, want)
	}

	sqlReq := sqlReqs[0]
	if got, want := sqlReq.Index, ro.Index; got != want {
		t.Fatalf("Index mismatch, got %v, want %v", got, want)
	}
	if got, want := sqlReq.Limit, ro.Limit; got != int64(want) {
		t.Fatalf("Limit mismatch, got %v, want %v", got, want)
	}

	reqRequestOptions := sqlReq.RequestOptions
	if got, want := reqRequestOptions.Priority, ro.Priority; got != want {
		t.Fatalf("Priority mismatch, got %v, want %v", got, want)
	}
	if got, want := reqRequestOptions.RequestTag, ro.RequestTag; got != want {
		t.Fatalf("Request tag mismatch, got %v, want %v", got, want)
	}
	if got, want := sqlReq.DirectedReadOptions, ro.DirectedReadOptions; got.String() != want.String() {
		t.Fatalf("Directed Read Options mismatch, got %v, want %v", got, want)
	}
	if got, want := sqlReq.OrderBy, ro.OrderBy; got != want {
		t.Fatalf("OrderBy mismatch, got %v, want %v", got, want)
	}
	if got, want := sqlReq.LockHint, ro.LockHint; got != want {
		t.Fatalf("LockHint mismatch, got %v, want %v", got, want)
	}
}

func checkReqsForTransactionOptions(t *testing.T, server InMemSpannerServer, txo TransactionOptions) {
	reqs := drainRequestsFromServer(server)
	sqlReqs := []*sppb.CommitRequest{}

	for _, req := range reqs {
		if sqlReq, ok := req.(*sppb.CommitRequest); ok {
			sqlReqs = append(sqlReqs, sqlReq)
		}
	}

	if got, want := len(sqlReqs), 1; got != want {
		t.Fatalf("Length mismatch, got %v, want %v", got, want)
	}

	sqlReq := sqlReqs[0]
	if got, want := sqlReq.ReturnCommitStats, txo.CommitOptions.ReturnCommitStats; got != want {
		t.Fatalf("Return commit stats mismatch, got %v, want %v", got, want)
	}

	reqRequestOptions := sqlReq.RequestOptions
	if got, want := reqRequestOptions.Priority, txo.CommitPriority; got != want {
		t.Fatalf("Commit priority mismatch, got %v, want %v", got, want)
	}
	if got, want := reqRequestOptions.TransactionTag, txo.TransactionTag; got != want {
		t.Fatalf("Transaction tag mismatch, got %v, want %v", got, want)
	}
}

func testSingleQuery(t *testing.T, serverError error) error {
	return testSingleQueryWithContext(context.Background(), t, serverError)
}

func testSingleQueryWithContext(ctx context.Context, t *testing.T, serverError error) error {
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, SessionPoolConfig: SessionPoolConfig{MinOpened: 1}})
	defer teardown()
	// Wait until all sessions have been created, so we know that those requests will not interfere with the test.
	sp := client.idleSessions
	waitFor(t, func() error {
		sp.mu.Lock()
		defer sp.mu.Unlock()
		if uint64(sp.idleList.Len()) != sp.MinOpened {
			return fmt.Errorf("num open sessions mismatch.\nGot: %d\nWant: %d", sp.numOpened, sp.MinOpened)
		}
		return nil
	})

	if serverError != nil {
		server.TestSpanner.SetError(serverError)
	}
	return executeSingerQuery(ctx, client.Single())
}

func executeSingerQuery(ctx context.Context, tx *ReadOnlyTransaction) error {
	return executeSingerQueryWithRowFunc(ctx, tx, nil)
}

func executeSingerQueryWithTimeout(ctx context.Context, tx *ReadOnlyTransaction, to time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	return executeSingerQueryWithRowFunc(ctx, tx, nil)
}

func executeSingerQueryWithRowFunc(ctx context.Context, tx *ReadOnlyTransaction, f func(rowCount int64) error) error {
	iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	defer iter.Stop()
	rowCount := int64(0)
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		var singerID, albumID int64
		var albumTitle string
		if err := row.Columns(&singerID, &albumID, &albumTitle); err != nil {
			return err
		}
		rowCount++
		if f != nil {
			if err := f(rowCount); err != nil {
				return err
			}
		}
	}
	if rowCount != SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount {
		return status.Errorf(codes.Internal, "Row count mismatch, got %v, expected %v", rowCount, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
	}
	return nil
}

func createSimulatedExecutionTimeWithTwoUnavailableErrors(method string) map[string]SimulatedExecutionTime {
	errors := make([]error, 2)
	errors[0] = status.Error(codes.Unavailable, "Temporary unavailable")
	errors[1] = status.Error(codes.Unavailable, "Temporary unavailable")
	executionTimes := make(map[string]SimulatedExecutionTime)
	executionTimes[method] = SimulatedExecutionTime{
		Errors: errors,
	}
	return executionTimes
}

func TestClient_ReadOnlyTransaction(t *testing.T) {
	t.Parallel()
	if err := testReadOnlyTransaction(t, make(map[string]SimulatedExecutionTime)); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadOnlyTransaction_UnavailableOnSessionCreate(t *testing.T) {
	t.Parallel()
	if err := testReadOnlyTransaction(t, createSimulatedExecutionTimeWithTwoUnavailableErrors(MethodCreateSession)); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadOnlyTransaction_UnavailableOnBeginTransaction(t *testing.T) {
	t.Parallel()
	if err := testReadOnlyTransaction(t, createSimulatedExecutionTimeWithTwoUnavailableErrors(MethodBeginTransaction)); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadOnlyTransaction_UnavailableOnExecuteStreamingSql(t *testing.T) {
	t.Parallel()
	if err := testReadOnlyTransaction(t, createSimulatedExecutionTimeWithTwoUnavailableErrors(MethodExecuteStreamingSql)); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadOnlyTransaction_SessionNotFoundOnExecuteStreamingSql(t *testing.T) {
	t.Parallel()
	// Session not found is not retryable for a query on a multi-use read-only
	// transaction, as we would need to start a new transaction on a new
	// session.
	err := testReadOnlyTransaction(t, map[string]SimulatedExecutionTime{
		MethodExecuteStreamingSql: {Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	})
	want := ToSpannerError(newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s"))
	if err == nil {
		t.Fatalf("missing expected error\nGot: nil\nWant: %v", want)
	}
	if status.Code(err) != status.Code(want) || !strings.Contains(err.Error(), want.Error()) {
		t.Fatalf("error mismatch\nGot: %v\nWant: %v", err, want)
	}
}

func TestClient_ReadOnlyTransaction_UnavailableOnCreateSessionAndBeginTransaction(t *testing.T) {
	t.Parallel()
	exec := map[string]SimulatedExecutionTime{
		MethodCreateSession:    {Errors: []error{status.Error(codes.Unavailable, "Temporary unavailable")}},
		MethodBeginTransaction: {Errors: []error{status.Error(codes.Unavailable, "Temporary unavailable")}},
	}
	if err := testReadOnlyTransaction(t, exec); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadOnlyTransaction_UnavailableOnCreateSessionAndInvalidArgumentOnBeginTransaction(t *testing.T) {
	t.Parallel()
	exec := map[string]SimulatedExecutionTime{
		MethodCreateSession:    {Errors: []error{status.Error(codes.Unavailable, "Temporary unavailable")}},
		MethodBeginTransaction: {Errors: []error{status.Error(codes.InvalidArgument, "Invalid argument")}},
	}
	if err := testReadOnlyTransaction(t, exec); err == nil {
		t.Fatalf("Missing expected exception")
	} else if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Got unexpected exception: %v", err)
	}
}

func TestClient_ReadOnlyTransaction_SessionNotFoundOnBeginTransaction(t *testing.T) {
	t.Parallel()
	if err := testReadOnlyTransaction(
		t,
		map[string]SimulatedExecutionTime{
			MethodBeginTransaction: {Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
		},
	); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadOnlyTransaction_SessionNotFoundOnBeginTransaction_WithMaxOneSession(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(
		t,
		ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig: SessionPoolConfig{
				MinOpened: 0,
				MaxOpened: 1,
			},
		})
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodBeginTransaction,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)
	tx := client.ReadOnlyTransaction()
	defer tx.Close()
	ctx := context.Background()
	if err := executeSingerQuery(ctx, tx); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_SessionNotFoundForFirstStatement(t *testing.T) {
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodExecuteStreamingSql,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)

	expectedAttempts := 2
	var attempts int
	_, err := client.ReadWriteTransaction(
		ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
			attempts++
			iter := tx.Query(ctx, NewStatement(SelectFooFromBar))
			defer iter.Stop()
			for {
				_, err := iter.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					return err
				}
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if expectedAttempts != attempts {
		t.Fatalf("unexpected number of attempts: %d, expected %d", attempts, expectedAttempts)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{},
	}, requests); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_SessionNotFoundForFirstStatement_AndThenSessionNotFoundForBeginTransaction(t *testing.T) {
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodExecuteStreamingSql,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)
	server.TestSpanner.PutExecutionTime(
		MethodBeginTransaction,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)

	expectedAttempts := 2
	var attempts int
	_, err := client.ReadWriteTransaction(
		ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
			attempts++
			iter := tx.Query(ctx, NewStatement(SelectFooFromBar))
			defer iter.Stop()
			for {
				_, err := iter.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					return err
				}
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if expectedAttempts != attempts {
		t.Fatalf("unexpected number of attempts: %d, expected %d", attempts, expectedAttempts)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{},
	}, requests); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_AbortedForFirstStatement_AndThenSessionNotFoundForBeginTransaction(t *testing.T) {
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodExecuteStreamingSql,
		SimulatedExecutionTime{Errors: []error{status.Error(codes.Aborted, "Transaction aborted")}},
	)
	server.TestSpanner.PutExecutionTime(
		MethodBeginTransaction,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)

	expectedAttempts := 2
	var attempts int
	_, err := client.ReadWriteTransaction(
		ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
			attempts++
			iter := tx.Query(ctx, NewStatement(SelectFooFromBar))
			defer iter.Stop()
			for {
				_, err := iter.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					return err
				}
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if expectedAttempts != attempts {
		t.Fatalf("unexpected number of attempts: %d, expected %d", attempts, expectedAttempts)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{},
	}, requests); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_SessionNotFoundForFirstStatement_DoesNotLeakSession(t *testing.T) {
	ctx := context.Background()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened: 1,
			MaxOpened: 1,
		},
	})
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodExecuteStreamingSql,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)

	expectedAttempts := 2
	var attempts int
	_, err := client.ReadWriteTransaction(
		ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
			attempts++
			iter := tx.Query(ctx, NewStatement(SelectFooFromBar))
			defer iter.Stop()
			for {
				_, err := iter.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					return err
				}
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if expectedAttempts != attempts {
		t.Fatalf("unexpected number of attempts: %d, expected %d", attempts, expectedAttempts)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.BatchCreateSessionsRequest{}, // We need to create more sessions, as the one used first was destroyed.
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{},
	}, requests); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadOnlyTransaction_QueryOptions(t *testing.T) {
	for _, tt := range queryOptionsTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env.Options != nil {
				unset := setQueryOptionsEnvVars(tt.env.Options)
				defer unset()
			}

			ctx := context.Background()
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, QueryOptions: tt.client})
			defer teardown()

			tx := client.ReadOnlyTransaction()
			defer tx.Close()

			var iter *RowIterator
			if tt.query.Options == nil {
				iter = tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
			} else {
				iter = tx.QueryWithOptions(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), tt.query)
			}
			testQueryOptions(t, iter, server.TestSpanner, tt.want)
		})
	}
}

func TestClient_ReadOnlyTransaction_ReadOptions(t *testing.T) {
	for _, tt := range readOptionsTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, ReadOptions: *tt.client})
			defer teardown()

			tx := client.ReadOnlyTransaction()
			defer tx.Close()

			var iter *RowIterator
			if tt.read == nil {
				iter = tx.Read(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"})
			} else {
				iter = tx.ReadWithOptions(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"}, tt.read)
			}
			testReadOptions(t, iter, server.TestSpanner, *tt.want)
		})
	}
}

func TestClient_DirectedReadOptions(t *testing.T) {
	directedReadOptions := &sppb.DirectedReadOptions{
		Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
			IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{
				ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
					{
						Location: "us-west1",
						Type:     sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY,
					},
				},
				AutoFailoverDisabled: true,
			},
		},
	}

	readOptionsTestCases := []ReadOptionsTestCase{
		{
			name:      "Client level",
			clientDRO: directedReadOptions,
			want:      &ReadOptions{DirectedReadOptions: directedReadOptions},
		},
		{
			name: "Read level",
			read: &ReadOptions{DirectedReadOptions: directedReadOptions},
			want: &ReadOptions{DirectedReadOptions: directedReadOptions},
		},
		{
			name:      "Read level has precedence than client level",
			clientDRO: &sppb.DirectedReadOptions{},
			read:      &ReadOptions{DirectedReadOptions: directedReadOptions},
			want:      &ReadOptions{DirectedReadOptions: directedReadOptions},
		},
	}

	queryOptionsTestCases := []QueryOptionsTestCase{
		{
			name:      "Client level",
			clientDRO: directedReadOptions,
			want:      QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{}, DirectedReadOptions: directedReadOptions},
		},
		{
			name:  "Query level",
			query: QueryOptions{DirectedReadOptions: directedReadOptions},
			want:  QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{}, DirectedReadOptions: directedReadOptions},
		},
		{
			name:      "Query level has precedence than client level",
			clientDRO: &sppb.DirectedReadOptions{},
			query:     QueryOptions{DirectedReadOptions: directedReadOptions},
			want:      QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{}, DirectedReadOptions: directedReadOptions},
		},
	}

	for _, tt := range readOptionsTestCases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, DirectedReadOptions: tt.clientDRO})
			defer teardown()

			tx := client.ReadOnlyTransaction()
			defer tx.Close()

			var iter *RowIterator
			if tt.read == nil {
				iter = tx.Read(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"})
			} else {
				iter = tx.ReadWithOptions(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"}, tt.read)
			}
			testReadOptions(t, iter, server.TestSpanner, *tt.want)
		})
	}

	for _, tt := range queryOptionsTestCases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, DirectedReadOptions: tt.clientDRO})
			defer teardown()

			var iter *RowIterator
			if tt.query.DirectedReadOptions == nil {
				iter = client.Single().Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
			} else {
				iter = client.Single().QueryWithOptions(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), tt.query)
			}
			testQueryOptions(t, iter, server.TestSpanner, tt.want)
		})
	}

	ctx := context.Background()
	directedReadOptionsForRW := &sppb.DirectedReadOptions{
		Replicas: &sppb.DirectedReadOptions_ExcludeReplicas_{
			ExcludeReplicas: &sppb.DirectedReadOptions_ExcludeReplicas{
				ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
					{
						Location: "us-west1",
						Type:     sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY,
					},
				},
			},
		},
	}
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, DirectedReadOptions: directedReadOptionsForRW})
	defer teardown()

	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *ReadWriteTransaction) error {
		iter := txn.ReadWithOptions(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"}, &ReadOptions{DirectedReadOptions: directedReadOptions})
		testReadOptions(t, iter, server.TestSpanner, ReadOptions{DirectedReadOptions: directedReadOptions})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *ReadWriteTransaction) error {
		iter := txn.QueryWithOptions(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), QueryOptions{DirectedReadOptions: directedReadOptions})
		testQueryOptions(t, iter, server.TestSpanner, QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{}, DirectedReadOptions: directedReadOptions})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadOnlyTransaction_WhenMultipleOperations_SessionLastUseTimeShouldBeUpdated(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened: 1,
			MaxOpened: 1,
			InactiveTransactionRemovalOptions: InactiveTransactionRemovalOptions{
				ActionOnInactiveTransaction: WarnAndClose,
				idleTimeThreshold:           300 * time.Millisecond,
			},
		},
	})
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql,
		SimulatedExecutionTime{
			MinimumExecutionTime: 200 * time.Millisecond,
		})
	server.TestSpanner.PutExecutionTime(MethodStreamingRead,
		SimulatedExecutionTime{
			MinimumExecutionTime: 200 * time.Millisecond,
		})
	ctx := context.Background()
	p := client.idleSessions

	roTxn := client.ReadOnlyTransaction()
	defer roTxn.Close()
	iter := roTxn.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	iter.Next()
	iter.Stop()

	// Get the session last use time.
	roTxn.sh.mu.Lock()
	sessionPrevLastUseTime := roTxn.sh.lastUseTime
	roTxn.sh.mu.Unlock()

	iter = roTxn.Read(ctx, "FOO", AllKeys(), []string{"BAR"})
	iter.Next()
	iter.Stop()

	// Get the latest session last use time
	roTxn.sh.mu.Lock()
	sessionLatestLastUseTime := roTxn.sh.lastUseTime
	sessionCheckoutTime := roTxn.sh.checkoutTime
	roTxn.sh.mu.Unlock()

	// sessionLatestLastUseTime should not be equal to sessionPrevLastUseTime.
	// This is because session lastUse time should be updated whenever a new operation is being executed on the transaction.
	if (sessionLatestLastUseTime.Sub(sessionPrevLastUseTime)).Milliseconds() <= 0 {
		t.Fatalf("Session lastUseTime times should not be equal")
	}

	if time.Since(sessionPrevLastUseTime).Milliseconds() < 400 {
		t.Fatalf("Expected session to be checkedout for more than 400 milliseconds")
	}
	if time.Since(sessionCheckoutTime).Milliseconds() < 400 {
		t.Fatalf("Expected session to be checkedout for more than 400 milliseconds")
	}
	// force run task to clean up unexpected long-running sessions whose lastUseTime >= 3sec.
	// The session should not be cleaned since the latest operation on the transaction has updated the lastUseTime.
	p.removeLongRunningSessions()
	if p.numOfLeakedSessionsRemoved > 0 {
		t.Fatalf("Expected session to not get cleaned by background maintainer")
	}
}

func setQueryOptionsEnvVars(opts *sppb.ExecuteSqlRequest_QueryOptions) func() {
	os.Setenv("SPANNER_OPTIMIZER_VERSION", opts.OptimizerVersion)
	os.Setenv("SPANNER_OPTIMIZER_STATISTICS_PACKAGE", opts.OptimizerStatisticsPackage)
	return func() {
		defer os.Setenv("SPANNER_OPTIMIZER_VERSION", "")
		defer os.Setenv("SPANNER_OPTIMIZER_STATISTICS_PACKAGE", "")
	}
}

func testReadOnlyTransaction(t *testing.T, executionTimes map[string]SimulatedExecutionTime) error {
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	for method, exec := range executionTimes {
		server.TestSpanner.PutExecutionTime(method, exec)
	}
	tx := client.ReadOnlyTransaction()
	defer tx.Close()
	ctx := context.Background()
	return executeSingerQuery(ctx, tx)
}

func TestClient_ReadWriteTransaction(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, make(map[string]SimulatedExecutionTime), 1); err != nil {
		t.Fatal(err)
	}
}

func validateIsolationLevelForRWTransactions(t *testing.T, server *MockedSpannerInMemTestServer, expected sppb.TransactionOptions_IsolationLevel, beginTransactionOption BeginTransactionOption) {
	found := false
	requests := drainRequestsFromServer(server.TestSpanner)
	for _, req := range requests {
		switch sqlReq := req.(type) {
		case *sppb.ExecuteSqlRequest:
			if beginTransactionOption == ExplicitBeginTransaction {
				t.Fatalf("got TransactionOptions on ExecuteSqlRequest in combination with ExplicitBeginTransaction")
			}
			found = true
			if sqlReq.GetTransaction().GetBegin().GetIsolationLevel() != expected {
				t.Fatalf("Invalid IsolationLevel\n Expected: %v\n Got: %v\n", expected, sqlReq.GetTransaction().GetBegin().GetIsolationLevel())
			}
			break
		case *sppb.BeginTransactionRequest:
			if beginTransactionOption == InlinedBeginTransaction {
				t.Fatalf("got BeginTransaction RPC in combination with InlinedBeginTransaction")
			}
			found = true
			if sqlReq.GetOptions().GetIsolationLevel() != expected {
				t.Fatalf("Invalid IsolationLevel\n Expected: %v\n Got: %v\n", expected, sqlReq.GetOptions().GetIsolationLevel())
			}
			break
		case *sppb.CommitRequest:
			found = true
			if sqlReq.GetSingleUseTransaction().GetIsolationLevel() != expected {
				t.Fatalf("Invalid IsolationLevel\n Expected: %v\n Got: %v\n", expected, sqlReq.GetSingleUseTransaction().GetIsolationLevel())
			}
			break
		default:
			continue
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatal("Request is not received")
	}
}

func TestClient_ReadWriteTransactionWithNoIsolationLevelForRWTransactionAtClientConfig(t *testing.T) {
	t.Parallel()
	server, teardown, err := testReadWriteTransactionWithConfig(t, ClientConfig{DisableNativeMetrics: true, SessionPoolConfig: DefaultSessionPoolConfig}, make(map[string]SimulatedExecutionTime), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer teardown()
	validateIsolationLevelForRWTransactions(t, server, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, InlinedBeginTransaction)
}

func TestClient_ReadWriteTransactionWithIsolationLevelForRWTransactionAtClientConfig(t *testing.T) {
	t.Parallel()
	server, teardown, err := testReadWriteTransactionWithConfig(t, ClientConfig{DisableNativeMetrics: true, SessionPoolConfig: DefaultSessionPoolConfig, TransactionOptions: TransactionOptions{IsolationLevel: sppb.TransactionOptions_REPEATABLE_READ}}, make(map[string]SimulatedExecutionTime), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer teardown()
	validateIsolationLevelForRWTransactions(t, server, sppb.TransactionOptions_REPEATABLE_READ, InlinedBeginTransaction)
}

func TestClient_ReadWriteTransactionWithIsolationLevelForRWTransactionAtTransactionLevel(t *testing.T) {
	t.Parallel()
	server, teardown, err := testReadWriteTransactionWithConfigWithTransactionOptions(t, ClientConfig{DisableNativeMetrics: true, SessionPoolConfig: DefaultSessionPoolConfig, TransactionOptions: TransactionOptions{IsolationLevel: sppb.TransactionOptions_SERIALIZABLE}}, TransactionOptions{IsolationLevel: sppb.TransactionOptions_REPEATABLE_READ}, make(map[string]SimulatedExecutionTime), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer teardown()
	validateIsolationLevelForRWTransactions(t, server, sppb.TransactionOptions_REPEATABLE_READ, InlinedBeginTransaction)
}

func TestClient_ReadWriteTransactionWithIsolationLevelForRWTransactionAtTransactionLevelWithAbort(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql, SimulatedExecutionTime{Errors: []error{status.Error(codes.Aborted, "Transaction aborted")}})

	_, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		defer iter.Stop()
		_, _ = iter.Next()
		return nil
	}, TransactionOptions{IsolationLevel: sppb.TransactionOptions_REPEATABLE_READ})
	if err != nil {
		t.Fatal(err)
	}
	validateIsolationLevelForRWTransactions(t, server, sppb.TransactionOptions_REPEATABLE_READ, InlinedBeginTransaction)
}

func TestClient_ApplyMutationsWithAtLeastOnceIsolationLevel(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}
	_, err := client.Apply(context.Background(), ms, ApplyAtLeastOnce(), IsolationLevel(sppb.TransactionOptions_REPEATABLE_READ))
	if err != nil {
		t.Fatal(err)
	}
	validateIsolationLevelForRWTransactions(t, server, sppb.TransactionOptions_REPEATABLE_READ, ExplicitBeginTransaction)
}

func TestClient_ApplyMutationsWithIsolationLevel(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}
	_, err := client.Apply(context.Background(), ms, IsolationLevel(sppb.TransactionOptions_SERIALIZABLE))
	if err != nil {
		t.Fatal(err)
	}
	validateIsolationLevelForRWTransactions(t, server, sppb.TransactionOptions_SERIALIZABLE, ExplicitBeginTransaction)
}

func consumeIterator(iter *RowIterator) error {
	defer iter.Stop()
	for {
		_, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func TestClient_ReadWriteStmtBasedTransactionWithIsolationLevelAtTransactionLevelWithExplicitBegin(t *testing.T) {
	t.Parallel()
	testClientReadWriteStmtBasedTransactionWithIsolationLevelAtTransactionLevel(t, ExplicitBeginTransaction)
}

func TestClient_ReadWriteStmtBasedTransactionWithIsolationLevelAtTransactionLevelWithInlineBegin(t *testing.T) {
	t.Parallel()
	testClientReadWriteStmtBasedTransactionWithIsolationLevelAtTransactionLevel(t, InlinedBeginTransaction)
}

func testClientReadWriteStmtBasedTransactionWithIsolationLevelAtTransactionLevel(t *testing.T, beginTransactionOption BeginTransactionOption) {
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()
	tx, err := NewReadWriteStmtBasedTransactionWithOptions(
		ctx,
		client,
		TransactionOptions{IsolationLevel: sppb.TransactionOptions_REPEATABLE_READ, BeginTransactionOption: beginTransactionOption})
	if err != nil {
		t.Fatalf("Unexpected error when creating transaction: %v", err)
	}

	iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	if err := consumeIterator(iter); err != nil {
		t.Fatal(err)
	}

	validateIsolationLevelForRWTransactions(t, server, sppb.TransactionOptions_REPEATABLE_READ, beginTransactionOption)
}

func TestClient_ReadWriteStmtBasedTransactionWithIsolationLevelAtClientConfigLevelWithExplicitBegin(t *testing.T) {
	t.Parallel()
	testClientReadWriteStmtBasedTransactionWithIsolationLevelAtClientConfigLevel(t, ExplicitBeginTransaction)
}

func TestClient_ReadWriteStmtBasedTransactionWithIsolationLevelAtClientConfigLevelWithInlineBegin(t *testing.T) {
	t.Parallel()
	testClientReadWriteStmtBasedTransactionWithIsolationLevelAtClientConfigLevel(t, InlinedBeginTransaction)
}

func testClientReadWriteStmtBasedTransactionWithIsolationLevelAtClientConfigLevel(t *testing.T, beginTransactionOption BeginTransactionOption) {
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, TransactionOptions: TransactionOptions{IsolationLevel: sppb.TransactionOptions_SERIALIZABLE}})
	defer teardown()
	ctx := context.Background()
	tx, err := NewReadWriteStmtBasedTransactionWithOptions(
		ctx,
		client,
		TransactionOptions{BeginTransactionOption: beginTransactionOption})
	if err != nil {
		t.Fatalf("Unexpected error when creating transaction: %v", err)
	}

	iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	if err := consumeIterator(iter); err != nil {
		t.Fatal(err)
	}

	validateIsolationLevelForRWTransactions(t, server, sppb.TransactionOptions_SERIALIZABLE, beginTransactionOption)
}

func TestClient_ReadWriteStmtBasedTransactionWithNoIsolationLevelWithExplicitBegin(t *testing.T) {
	t.Parallel()
	testClientReadWriteStmtBasedTransactionWithNoIsolationLevel(t, ExplicitBeginTransaction)
}

func TestClient_ReadWriteStmtBasedTransactionWithNoIsolationLevelWithInlineBegin(t *testing.T) {
	t.Parallel()
	testClientReadWriteStmtBasedTransactionWithNoIsolationLevel(t, InlinedBeginTransaction)
}

func testClientReadWriteStmtBasedTransactionWithNoIsolationLevel(t *testing.T, beginTransactionOption BeginTransactionOption) {
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, TransactionOptions: TransactionOptions{}})
	defer teardown()
	ctx := context.Background()
	tx, err := NewReadWriteStmtBasedTransactionWithOptions(
		ctx,
		client,
		TransactionOptions{BeginTransactionOption: beginTransactionOption})
	if err != nil {
		t.Fatalf("Unexpected error when creating transaction: %v", err)
	}

	iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	if err := consumeIterator(iter); err != nil {
		t.Fatal(err)
	}

	validateIsolationLevelForRWTransactions(t, server, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, beginTransactionOption)
}

func TestClient_ReadWriteTransactionCommitAborted(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodCommitTransaction: {Errors: []error{status.Error(codes.Aborted, "Transaction aborted")}},
	}, 2); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_BufferedWriteBeforeAbortedFirstSqlStatement(t *testing.T) {
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodExecuteSql, SimulatedExecutionTime{Errors: []error{status.Error(codes.Aborted, "Transaction aborted")}})

	var attempts int
	expectedAttempts := 2
	_, err := client.ReadWriteTransaction(
		ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
			attempts++
			// Buffer mutations before executing a SQL statement.
			if err := tx.BufferWrite([]*Mutation{
				Insert("foo", []string{"col1"}, []interface{}{"key1"}),
			}); err != nil {
				return err
			}
			// Then execute a SQL statement that will return Aborted from the backend.
			// This will force a retry of the transaction with an explicit BeginTransaction RPC.
			c, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo))
			if err != nil {
				return err
			}
			if g, w := c, int64(UpdateBarSetFooRowCount); g != w {
				return fmt.Errorf("update count mismatch\nGot:  %v\nWant: %v", g, w)
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if expectedAttempts != attempts {
		t.Fatalf("unexpected number of attempts: %d, expected %d", attempts, expectedAttempts)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{},
	}, requests); err != nil {
		t.Fatal(err)
	}
	commit := requests[len(requests)-1].(*sppb.CommitRequest)
	g, w := len(commit.Mutations), 1
	if g != w {
		t.Fatalf("mutations count mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestClient_ReadWriteTransaction_BufferedWriteBeforeAbortedFirstSqlStatementTwice(t *testing.T) {
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodExecuteSql, SimulatedExecutionTime{Errors: []error{
		status.Error(codes.Aborted, "Transaction aborted"),
		status.Error(codes.Aborted, "Transaction aborted"),
	}})

	var attempts int
	expectedAttempts := 3
	_, err := client.ReadWriteTransaction(
		ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
			attempts++
			// Buffer mutations before executing a SQL statement.
			if err := tx.BufferWrite([]*Mutation{
				Insert("foo", []string{"col1"}, []interface{}{"key1"}),
			}); err != nil {
				return err
			}
			// Then execute a SQL statement that will return Aborted from the backend.
			// This will force a retry of the transaction with an explicit BeginTransaction RPC.
			c, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo))
			if err != nil {
				return err
			}
			if g, w := c, int64(UpdateBarSetFooRowCount); g != w {
				return fmt.Errorf("update count mismatch\nGot:  %v\nWant: %v", g, w)
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if expectedAttempts != attempts {
		t.Fatalf("unexpected number of attempts: %d, expected %d", attempts, expectedAttempts)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{},
	}, requests); err != nil {
		t.Fatal(err)
	}
	commit := requests[len(requests)-1].(*sppb.CommitRequest)
	g, w := len(commit.Mutations), 1
	if g != w {
		t.Fatalf("mutations count mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestClient_ReadWriteTransaction_BufferedWriteBeforeSqlStatementWithError(t *testing.T) {
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodExecuteSql, SimulatedExecutionTime{Errors: []error{
		status.Error(codes.InvalidArgument, "Invalid"),
		status.Error(codes.InvalidArgument, "Invalid"),
	}})

	var attempts int
	expectedAttempts := 2
	_, err := client.ReadWriteTransaction(
		ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
			attempts++
			// Buffer mutations before executing a SQL statement.
			if err := tx.BufferWrite([]*Mutation{
				Insert("foo", []string{"col1"}, []interface{}{"key1"}),
			}); err != nil {
				return err
			}
			// Then execute a SQL statement that will return InvalidArgument from the backend.
			// This will initially force a retry of the transaction with an explicit BeginTransaction RPC.
			// We ignore the error and proceed to commit the transaction.
			_, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo))
			if err == nil {
				return errors.New("missing expected InvalidArgument error")
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if expectedAttempts != attempts {
		t.Fatalf("unexpected number of attempts: %d, expected %d", attempts, expectedAttempts)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{},
	}, requests); err != nil {
		t.Fatal(err)
	}
	commit := requests[len(requests)-1].(*sppb.CommitRequest)
	g, w := len(commit.Mutations), 1
	if g != w {
		t.Fatalf("mutations count mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestClient_ReadWriteTransaction_BufferedWriteBeforeSqlStatementWithErrorThatGoesAway(t *testing.T) {
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodExecuteSql, SimulatedExecutionTime{Errors: []error{
		status.Error(codes.AlreadyExists, "Row already exists"),
	}})

	var attempts int
	expectedAttempts := 2
	_, err := client.ReadWriteTransaction(
		ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
			attempts++
			// Buffer mutations before executing a SQL statement.
			if err := tx.BufferWrite([]*Mutation{
				Insert("foo", []string{"col1"}, []interface{}{"key1"}),
			}); err != nil {
				return err
			}
			// Then execute a SQL statement that will return InvalidArgument from the backend.
			// This will initially force a retry of the transaction with an explicit BeginTransaction RPC.
			// The error does not occur during the retry and the transaction is allowed to continue.
			_, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo))
			if err != nil {
				return err
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if expectedAttempts != attempts {
		t.Fatalf("unexpected number of attempts: %d, expected %d", attempts, expectedAttempts)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{},
	}, requests); err != nil {
		t.Fatal(err)
	}
	commit := requests[len(requests)-1].(*sppb.CommitRequest)
	g, w := len(commit.Mutations), 1
	if g != w {
		t.Fatalf("mutations count mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestClient_ReadWriteTransaction_OnlyBufferWritesDuringInitialAttempt(t *testing.T) {
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodExecuteSql, SimulatedExecutionTime{Errors: []error{
		status.Error(codes.AlreadyExists, "Row already exists"),
	}})

	expectedAttempts := 2
	var attempts int
	_, err := client.ReadWriteTransaction(
		ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
			attempts++
			if attempts == 1 {
				// Only do a blind write if it is not a retry of the transaction.
				if err := tx.BufferWrite([]*Mutation{
					Delete("foo", AllKeys()),
				}); err != nil {
					return err
				}
			}
			// Then execute a SQL statement that will return InvalidArgument from the backend.
			// This will initially force a retry of the transaction with an explicit BeginTransaction RPC.
			// The error does not occur during the retry and the transaction is allowed to continue.
			_, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo))
			if err != nil {
				return err
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if expectedAttempts != attempts {
		t.Fatalf("unexpected number of attempts: %d, expected %d", attempts, expectedAttempts)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{},
	}, requests); err != nil {
		t.Fatal(err)
	}
	commit := requests[len(requests)-1].(*sppb.CommitRequest)
	g, w := len(commit.Mutations), 0
	if g != w {
		t.Fatalf("mutations count mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestClient_ReadWriteTransaction_BlindWriteWithAbortedCommit(t *testing.T) {
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction, SimulatedExecutionTime{Errors: []error{status.Error(codes.Aborted, "Transaction aborted")}})

	var attempts int
	expectedAttempts := 2
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		// Do a blind write and then commit. The CommitRequest will be aborted and cause the transaction to retry.
		if err := tx.BufferWrite([]*Mutation{Insert("foo", []string{"col1"}, []interface{}{"key1"})}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if expectedAttempts != attempts {
		t.Fatalf("unexpected number of attempts: %d, expected %d", attempts, expectedAttempts)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{},
	}, requests); err != nil {
		t.Fatal(err)
	}
	commit := requests[len(requests)-1].(*sppb.CommitRequest)
	// TODO: Update to 1 when the bug is fixed
	g, w := len(commit.Mutations), 1
	if g != w {
		t.Fatalf("mutations count mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestClient_ReadWriteTransaction_SessionNotFoundOnCommit(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodCommitTransaction: {Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	}, 2); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_SessionNotFoundOnBeginTransaction(t *testing.T) {
	t.Parallel()
	// We expect only 1 attempt, as the 'Session not found' error is already
	//handled in the session pool where the session is prepared.
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodBeginTransaction: {Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	}, 1); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_SessionNotFoundOnExecuteStreamingSql(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodExecuteStreamingSql: {Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	}, 2); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_SessionNotFoundOnExecuteUpdate(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodExecuteSql,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)
	ctx := context.Background()
	var attempts int
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		rowCount, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo))
		if err != nil {
			return err
		}
		if g, w := rowCount, int64(UpdateBarSetFooRowCount); g != w {
			return status.Errorf(codes.FailedPrecondition, "Row count mismatch\nGot: %v\nWant: %v", g, w)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 2; g != w {
		t.Fatalf("number of attempts mismatch:\nGot%d\nWant:%d", g, w)
	}
}

func TestClient_ReadWriteTransaction_WhenLongRunningSessionCleaned_TransactionShouldFail(t *testing.T) {
	t.Parallel()
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened: 1,
			MaxOpened: 1,
			InactiveTransactionRemovalOptions: InactiveTransactionRemovalOptions{
				ActionOnInactiveTransaction: WarnAndClose,
			},
		},
	})
	defer teardown()
	ctx := context.Background()
	p := client.idleSessions
	msg := "session is already recycled / destroyed"
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		rowCount, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo))
		if err != nil {
			return err
		}
		if g, w := rowCount, int64(UpdateBarSetFooRowCount); g != w {
			return status.Errorf(codes.FailedPrecondition, "Row count mismatch\nGot: %v\nWant: %v", g, w)
		}

		// Simulate the session to be last used before 60 mins.
		// The background task cleans up this long-running session.
		tx.sh.mu.Lock()
		tx.sh.lastUseTime = time.Now().Add(-time.Hour)
		if g, w := tx.sh.eligibleForLongRunning, false; g != w {
			tx.sh.mu.Unlock()
			return status.Errorf(codes.FailedPrecondition, "isLongRunningTransaction value mismatch\nGot: %v\nWant: %v", g, w)
		}
		tx.sh.mu.Unlock()

		// force run task to clean up unexpected long-running sessions
		p.removeLongRunningSessions()

		// The session associated with this transaction tx has been destroyed. So the below call should fail.
		// Eventually this means the entire transaction should not succeed.
		_, err = tx.Update(ctx, NewStatement("UPDATE FOO SET BAR='value' WHERE ID=1"))
		if err != nil {
			return err
		}
		return nil
	})
	if err == nil {
		t.Fatalf("Missing expected exception")
	}
	if status.Code(err) != codes.FailedPrecondition || !strings.Contains(err.Error(), msg) {
		t.Fatalf("error mismatch\nGot: %v\nWant: %v", err, msg)
	}
}

func TestClient_ReadWriteTransaction_WhenMultipleOperations_SessionLastUseTimeShouldBeUpdated(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened: 1,
			MaxOpened: 1,
			InactiveTransactionRemovalOptions: InactiveTransactionRemovalOptions{
				ActionOnInactiveTransaction: WarnAndClose,
				idleTimeThreshold:           300 * time.Millisecond,
			},
		},
	})
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodExecuteSql,
		SimulatedExecutionTime{
			MinimumExecutionTime: 200 * time.Millisecond,
		})
	ctx := context.Background()
	p := client.idleSessions
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		// Execute first operation on the transaction
		_, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo))
		if err != nil {
			return err
		}

		// Get the session last use time.
		tx.sh.mu.Lock()
		sessionPrevLastUseTime := tx.sh.lastUseTime
		tx.sh.mu.Unlock()

		// Execute second operation on the transaction
		_, err = tx.Update(ctx, NewStatement(UpdateBarSetFoo))
		if err != nil {
			return err
		}
		// Get the latest session last use time
		tx.sh.mu.Lock()
		sessionLatestLastUseTime := tx.sh.lastUseTime
		sessionCheckoutTime := tx.sh.checkoutTime
		tx.sh.mu.Unlock()

		// sessionLatestLastUseTime should not be equal to sessionPrevLastUseTime.
		// This is because session lastUse time should be updated whenever a new operation is being executed on the transaction.
		if (sessionLatestLastUseTime.Sub(sessionPrevLastUseTime)).Milliseconds() <= 0 {
			t.Fatalf("Session lastUseTime times should not be equal")
		}

		if time.Since(sessionPrevLastUseTime).Milliseconds() < 400 {
			t.Fatalf("Expected session to be checkedout for more than 400 milliseconds")
		}
		if time.Since(sessionCheckoutTime).Milliseconds() < 400 {
			t.Fatalf("Expected session to be checkedout for more than 400 milliseconds")
		}
		// force run task to clean up unexpected long-running sessions whose lastUseTime >= 3sec.
		// The session should not be cleaned since the latest operation on the transaction has updated the lastUseTime.
		p.removeLongRunningSessions()
		if p.numOfLeakedSessionsRemoved > 0 {
			t.Fatalf("Expected session to not get cleaned by background maintainer")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_SessionNotFoundOnExecuteBatchUpdate(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodExecuteBatchDml,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)
	ctx := context.Background()
	var attempts int
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		rowCounts, err := tx.BatchUpdate(ctx, []Statement{NewStatement(UpdateBarSetFoo)})
		if err != nil {
			return err
		}
		if g, w := len(rowCounts), 1; g != w {
			return status.Errorf(codes.FailedPrecondition, "Row counts length mismatch\nGot: %v\nWant: %v", g, w)
		}
		if g, w := rowCounts[0], int64(UpdateBarSetFooRowCount); g != w {
			return status.Errorf(codes.FailedPrecondition, "Row count mismatch\nGot: %v\nWant: %v", g, w)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 2; g != w {
		t.Fatalf("number of attempts mismatch:\nGot%d\nWant:%d", g, w)
	}
}

func TestClient_ReadWriteTransaction_WhenLongRunningExecuteBatchUpdate_TakeNoAction(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened: 1,
			MaxOpened: 1,
			InactiveTransactionRemovalOptions: InactiveTransactionRemovalOptions{
				ActionOnInactiveTransaction: WarnAndClose,
			},
		},
	})
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodExecuteBatchDml,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)
	ctx := context.Background()
	p := client.idleSessions
	var attempts int
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		if attempts == 2 {
			// Simulate the session to be long-running. The background task should not clean up this long-running session.
			tx.sh.mu.Lock()
			tx.sh.lastUseTime = time.Now().Add(-time.Hour)
			if g, w := tx.sh.eligibleForLongRunning, true; g != w {
				tx.sh.mu.Unlock()
				return status.Errorf(codes.FailedPrecondition, "isLongRunningTransaction value mismatch\nGot: %v\nWant: %v", g, w)
			}
			tx.sh.mu.Unlock()

			// force run task to clean up unexpected long-running sessions
			p.removeLongRunningSessions()
		}
		rowCounts, err := tx.BatchUpdate(ctx, []Statement{NewStatement(UpdateBarSetFoo)})
		if err != nil {
			return err
		}
		if g, w := len(rowCounts), 1; g != w {
			return status.Errorf(codes.FailedPrecondition, "Row counts length mismatch\nGot: %v\nWant: %v", g, w)
		}
		if g, w := rowCounts[0], int64(UpdateBarSetFooRowCount); g != w {
			return status.Errorf(codes.FailedPrecondition, "Row count mismatch\nGot: %v\nWant: %v", g, w)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 2; g != w {
		t.Fatalf("number of attempts mismatch:\nGot%d\nWant:%d", g, w)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if g, w := p.numOfLeakedSessionsRemoved, uint64(0); g != w {
		t.Fatalf("Number of leaked sessions removed mismatch\nGot: %d\nWant: %d\n", g, w)
	}
}

func TestClient_ReadWriteTransaction_Query_QueryOptions(t *testing.T) {
	for _, tt := range queryOptionsTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env.Options != nil {
				unset := setQueryOptionsEnvVars(tt.env.Options)
				defer unset()
			}

			ctx := context.Background()
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, QueryOptions: tt.client})
			defer teardown()

			_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
				var iter *RowIterator
				if tt.query.Options == nil {
					iter = tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
				} else {
					iter = tx.QueryWithOptions(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), tt.query)
				}
				testQueryOptions(t, iter, server.TestSpanner, tt.want)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClient_ReadWriteTransaction_LockHintOptions(t *testing.T) {
	readOptionsTestCases := []ReadOptionsTestCase{
		{
			name:   "Client level",
			client: &ReadOptions{LockHint: sppb.ReadRequest_LOCK_HINT_EXCLUSIVE},
			want:   &ReadOptions{LockHint: sppb.ReadRequest_LOCK_HINT_EXCLUSIVE},
		},
		{
			name:   "Read level",
			client: &ReadOptions{},
			read:   &ReadOptions{LockHint: sppb.ReadRequest_LOCK_HINT_EXCLUSIVE},
			want:   &ReadOptions{LockHint: sppb.ReadRequest_LOCK_HINT_EXCLUSIVE},
		},
		{
			name:   "Read level has precedence than client level",
			client: &ReadOptions{LockHint: sppb.ReadRequest_LOCK_HINT_SHARED},
			read:   &ReadOptions{LockHint: sppb.ReadRequest_LOCK_HINT_EXCLUSIVE},
			want:   &ReadOptions{LockHint: sppb.ReadRequest_LOCK_HINT_EXCLUSIVE},
		},
		{
			name:   "Client level has precendence when LOCK_HINT_UNSPECIFIED at read level",
			client: &ReadOptions{LockHint: sppb.ReadRequest_LOCK_HINT_EXCLUSIVE},
			read:   &ReadOptions{},
			want:   &ReadOptions{LockHint: sppb.ReadRequest_LOCK_HINT_EXCLUSIVE},
		},
	}

	for _, tt := range readOptionsTestCases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, ReadOptions: *tt.client})
			defer teardown()

			_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
				var iter *RowIterator
				if tt.read == nil {
					iter = tx.Read(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"})
				} else {
					iter = tx.ReadWithOptions(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"}, tt.read)
				}
				testReadOptions(t, iter, server.TestSpanner, *tt.want)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClient_ReadOnlyTransaction_LockHintOptions(t *testing.T) {
	readOptionsTestCases := []ReadOptionsTestCase{
		{
			name:   "Client level Lock hint overiden in request level",
			client: &ReadOptions{LockHint: sppb.ReadRequest_LOCK_HINT_EXCLUSIVE},
			read:   &ReadOptions{},
			want:   &ReadOptions{LockHint: sppb.ReadRequest_LOCK_HINT_UNSPECIFIED},
		},
		{
			name:   "Request level",
			client: &ReadOptions{LockHint: sppb.ReadRequest_LOCK_HINT_EXCLUSIVE},
			read:   &ReadOptions{LockHint: sppb.ReadRequest_LOCK_HINT_SHARED},
			want:   &ReadOptions{LockHint: sppb.ReadRequest_LOCK_HINT_SHARED},
		},
	}

	for _, tt := range readOptionsTestCases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, ReadOptions: *tt.client})
			defer teardown()

			for _, tx := range []*ReadOnlyTransaction{
				client.Single(),
				client.ReadOnlyTransaction(),
			} {
				iter := tx.ReadWithOptions(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"}, tt.read)
				testReadOptions(t, iter, server.TestSpanner, *tt.want)
			}

		})
	}
}
func TestClient_ReadWriteTransaction_Query_ReadOptions(t *testing.T) {
	for _, tt := range readOptionsTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, ReadOptions: *tt.client})
			defer teardown()

			_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
				var iter *RowIterator
				if tt.read == nil {
					iter = tx.Read(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"})
				} else {
					iter = tx.ReadWithOptions(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"}, tt.read)
				}
				testReadOptions(t, iter, server.TestSpanner, *tt.want)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClient_ReadWriteTransaction_Update_QueryOptions(t *testing.T) {
	for _, tt := range queryOptionsTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env.Options != nil {
				unset := setQueryOptionsEnvVars(tt.env.Options)
				defer unset()
			}

			ctx := context.Background()
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, QueryOptions: tt.client})
			defer teardown()

			_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
				var rowCount int64
				var err error
				if tt.query.Options == nil {
					rowCount, err = tx.Update(ctx, NewStatement(UpdateBarSetFoo))
				} else {
					rowCount, err = tx.UpdateWithOptions(ctx, NewStatement(UpdateBarSetFoo), tt.query)
				}
				if got, want := rowCount, int64(5); got != want {
					t.Fatalf("Incorrect updated row count: got %v, want %v", got, want)
				}
				return err
			})
			if err != nil {
				t.Fatalf("Failed to update rows: %v", err)
			}
			checkReqsForQueryOptions(t, server.TestSpanner, tt.want)
		})
	}
}

func TestClient_ReadWriteTransaction_TransactionOptions(t *testing.T) {
	for _, tt := range transactionOptionsTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, TransactionOptions: *tt.client})
			defer teardown()

			var err error
			if tt.write == nil {
				_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
					return nil
				})
			} else {
				_, err = client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
					return nil
				}, *tt.write)
			}

			if err != nil {
				t.Fatalf("Failed executing a read-write transaction: %v", err)
			}
			checkReqsForTransactionOptions(t, server.TestSpanner, *tt.want)
		})
	}
}

func TestClient_ReadWriteTransactionWithOptions(t *testing.T) {
	_, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()
	resp, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		defer iter.Stop()
		rowCount := int64(0)
		for {
			row, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return err
			}
			var singerID, albumID int64
			var albumTitle string
			if err := row.Columns(&singerID, &albumID, &albumTitle); err != nil {
				return err
			}
			rowCount++
		}
		if rowCount != SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount {
			return status.Errorf(codes.FailedPrecondition, "Row count mismatch, got %v, expected %v", rowCount, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
		}
		return nil
	}, TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true}})
	if err != nil {
		t.Fatalf("Failed to execute the transaction: %s", err)
	}
	if got, want := resp.CommitStats.MutationCount, int64(1); got != want {
		t.Fatalf("Mismatch mutation count - got: %d, want: %d", got, want)
	}
}

func TestClient_ReadWriteTransactionWithOptimisticLockMode_ExecuteSqlRequest(t *testing.T) {
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql,
		SimulatedExecutionTime{
			Errors: []error{status.Error(codes.Unavailable, "Temporary unavailable"), status.Error(codes.Aborted, "Transaction aborted")},
		})
	_, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		defer iter.Stop()
		_, err := iter.Next()
		return err
	}, TransactionOptions{ReadLockMode: sppb.TransactionOptions_ReadWrite_OPTIMISTIC})
	if err != nil {
		t.Fatalf("Failed to execute the transaction: %s", err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{}}, requests); err != nil {
		t.Fatal(err)
	}
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}
	if requests[1+muxCreateBuffer].(*sppb.ExecuteSqlRequest).GetTransaction().GetBegin().GetReadWrite().GetReadLockMode() != sppb.TransactionOptions_ReadWrite_OPTIMISTIC {
		t.Fatal("Transaction is not set to optimistic")
	}
	if requests[2+muxCreateBuffer].(*sppb.ExecuteSqlRequest).GetTransaction().GetBegin().GetReadWrite().GetReadLockMode() != sppb.TransactionOptions_ReadWrite_OPTIMISTIC {
		t.Fatal("Transaction is not set to optimistic")
	}
	if requests[3+muxCreateBuffer].(*sppb.BeginTransactionRequest).GetOptions().GetReadWrite().GetReadLockMode() != sppb.TransactionOptions_ReadWrite_OPTIMISTIC {
		t.Fatal("Begin Transaction is not set to optimistic")
	}
	if _, ok := requests[4+muxCreateBuffer].(*sppb.ExecuteSqlRequest).Transaction.GetSelector().(*sppb.TransactionSelector_Id); !ok {
		t.Fatal("expected streaming query to use transactionID from explicit begin transaction")
	}
}

func TestClient_ReadWriteTransactionWithOptimisticLockMode_ReadRequest(t *testing.T) {
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()
	server.TestSpanner.PutExecutionTime(MethodStreamingRead,
		SimulatedExecutionTime{
			Errors: []error{status.Error(codes.Unavailable, "Temporary unavailable"), status.Error(codes.Aborted, "Transaction aborted")},
		})
	_, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		iter := tx.Read(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"})
		defer iter.Stop()
		_, err := iter.Next()
		return err
	}, TransactionOptions{ReadLockMode: sppb.TransactionOptions_ReadWrite_OPTIMISTIC})
	if err != nil {
		t.Fatalf("Failed to execute the transaction: %s", err)
	}

	requests := drainRequestsFromServer(server.TestSpanner)

	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ReadRequest{},
		&sppb.ReadRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ReadRequest{},
		&sppb.CommitRequest{}}, requests); err != nil {
		t.Fatal(err)
	}
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}
	if requests[1+muxCreateBuffer].(*sppb.ReadRequest).GetTransaction().GetBegin().GetReadWrite().GetReadLockMode() != sppb.TransactionOptions_ReadWrite_OPTIMISTIC {
		t.Fatal("Transaction is not set to optimistic")
	}
	if requests[2+muxCreateBuffer].(*sppb.ReadRequest).GetTransaction().GetBegin().GetReadWrite().GetReadLockMode() != sppb.TransactionOptions_ReadWrite_OPTIMISTIC {
		t.Fatal("Transaction is not set to optimistic")
	}
	if requests[3+muxCreateBuffer].(*sppb.BeginTransactionRequest).GetOptions().GetReadWrite().GetReadLockMode() != sppb.TransactionOptions_ReadWrite_OPTIMISTIC {
		t.Fatal("Begin Transaction is not set to optimistic")
	}
	if _, ok := requests[4+muxCreateBuffer].(*sppb.ReadRequest).Transaction.GetSelector().(*sppb.TransactionSelector_Id); !ok {
		t.Fatal("expected streaming read to use transactionID from explicit begin transaction")
	}
}

func TestClient_ReadWriteStmtBasedTransaction_TransactionOptions(t *testing.T) {
	for _, tt := range transactionOptionsTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, TransactionOptions: *tt.client})
			defer teardown()

			var tx *ReadWriteStmtBasedTransaction
			var err error
			if tt.write == nil {
				tx, err = NewReadWriteStmtBasedTransaction(ctx, client)
			} else {
				tx, err = NewReadWriteStmtBasedTransactionWithOptions(ctx, client, *tt.write)
			}

			if err != nil {
				t.Fatalf("Failed initializing a read-write stmt based transaction: %v", err)
			}

			if got, want := tx.txOpts, *tt.want; got != want {
				t.Fatalf("Transaction options mismatch, got %v, want %v", got, want)
			}
		})
	}
}

func TestClient_ReadWriteStmtBasedTransactionWithOptions(t *testing.T) {
	_, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()
	tx, err := NewReadWriteStmtBasedTransactionWithOptions(
		ctx,
		client,
		TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true}})
	if err != nil {
		t.Fatalf("Unexpected error when creating transaction: %v", err)
	}

	iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	defer iter.Stop()
	rowCount := int64(0)
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatalf("Unexpected error when fetching query results: %v", err)
		}
		var singerID, albumID int64
		var albumTitle string
		if err := row.Columns(&singerID, &albumID, &albumTitle); err != nil {
			t.Fatalf("Unexpected error when getting query data: %v", err)
		}
		rowCount++
	}
	resp, err := tx.CommitWithReturnResp(ctx)
	if err != nil {
		t.Fatalf("Unexpected error when committing transaction: %v", err)
	}
	if rowCount != SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount {
		t.Errorf("Row count mismatch, got %v, expected %v", rowCount, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
	}
	if got, want := resp.CommitStats.MutationCount, int64(1); got != want {
		t.Fatalf("Mismatch mutation count - got: %d, want: %d", got, want)
	}
}

func TestClient_ReadWriteTransaction_DoNotLeakSessionOnPanic(t *testing.T) {
	// Make sure that there is always only one session in the pool.
	sc := SessionPoolConfig{
		MinOpened: 1,
		MaxOpened: 1,
	}
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, SessionPoolConfig: sc})
	defer teardown()
	ctx := context.Background()

	// If a panic occurs during a transaction, the session will not leak.
	func() {
		defer func() { recover() }()

		_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
			panic("cause panic")
		})
		if err != nil {
			t.Fatalf("Unexpected error during transaction: %v", err)
		}
	}()

	if g, w := client.idleSessions.idleList.Len(), 1; g != w {
		t.Fatalf("idle session count mismatch.\nGot: %v\nWant: %v", g, w)
	}
}

func TestClient_SessionContainsDatabaseRole(t *testing.T) {
	// Make sure that there is always only one session in the pool.
	sc := SessionPoolConfig{
		MinOpened: 1,
		MaxOpened: 1,
	}
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, SessionPoolConfig: sc, DatabaseRole: "test"})
	defer teardown()

	// Wait until all sessions have been created, so we know that those requests will not interfere with the test.
	sp := client.idleSessions
	waitFor(t, func() error {
		sp.mu.Lock()
		defer sp.mu.Unlock()
		if uint64(sp.idleList.Len()) != 1 {
			return fmt.Errorf("num open sessions mismatch.\nGot: %d\nWant: %d", sp.numOpened, sp.MinOpened)
		}
		return nil
	})

	resp, err := server.TestSpanner.GetSession(context.Background(), &sppb.GetSessionRequest{Name: client.idleSessions.idleList.Front().Value.(*session).id})
	if err != nil {
		t.Fatalf("Failed to get session unexpectedly: %v", err)
	}
	if g, w := resp.CreatorRole, "test"; g != w {
		t.Fatalf("database role mismatch.\nGot: %v\nWant: %v", g, w)
	}
}

func TestClient_SessionNotFound(t *testing.T) {
	// Ensure we always have at least one session in the pool.
	sc := SessionPoolConfig{
		MinOpened: 1,
	}
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, SessionPoolConfig: sc})
	defer teardown()
	ctx := context.Background()
	for {
		client.idleSessions.mu.Lock()
		numSessions := client.idleSessions.idleList.Len()
		client.idleSessions.mu.Unlock()
		if numSessions > 0 {
			break
		}
		time.After(time.Millisecond)
	}
	// Remove the session from the server without the pool knowing it.
	_, err := server.TestSpanner.DeleteSession(ctx, &sppb.DeleteSessionRequest{Name: client.idleSessions.idleList.Front().Value.(*session).id})
	if err != nil {
		t.Fatalf("Failed to delete session unexpectedly: %v", err)
	}

	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		defer iter.Stop()
		rowCount := int64(0)
		for {
			row, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return err
			}
			var singerID, albumID int64
			var albumTitle string
			if err := row.Columns(&singerID, &albumID, &albumTitle); err != nil {
				return err
			}
			rowCount++
		}
		if rowCount != SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount {
			return spannerErrorf(codes.FailedPrecondition, "Row count mismatch, got %v, expected %v", rowCount, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Unexpected error during transaction: %v", err)
	}
}

func TestClient_ReadWriteTransactionExecuteStreamingSqlAborted(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodExecuteStreamingSql: {Errors: []error{status.Error(codes.Aborted, "Transaction aborted")}},
	}, 2); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_UnavailableOnBeginTransaction(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodBeginTransaction: {Errors: []error{status.Error(codes.Unavailable, "Unavailable")}},
	}, 1); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_UnavailableOnBeginAndAbortOnCommit(t *testing.T) {
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodBeginTransaction:  {Errors: []error{status.Error(codes.Unavailable, "Unavailable")}},
		MethodCommitTransaction: {Errors: []error{status.Error(codes.Aborted, "Aborted")}},
	}, 2); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_UnavailableOnExecuteStreamingSql(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodExecuteStreamingSql: {Errors: []error{status.Error(codes.Unavailable, "Unavailable")}},
	}, 1); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_UnavailableOnBeginAndExecuteStreamingSqlAndTwiceAbortOnCommit(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodBeginTransaction:    {Errors: []error{status.Error(codes.Unavailable, "Unavailable")}},
		MethodExecuteStreamingSql: {Errors: []error{status.Error(codes.Unavailable, "Unavailable")}},
		MethodCommitTransaction:   {Errors: []error{status.Error(codes.Aborted, "Aborted"), status.Error(codes.Aborted, "Aborted")}},
	}, 3); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_CommitAborted(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction, SimulatedExecutionTime{
		Errors: []error{status.Error(codes.Aborted, "Aborted")},
	})
	defer teardown()
	ctx := context.Background()
	attempts := 0
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		_, err := tx.Update(ctx, Statement{SQL: UpdateBarSetFoo})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 2; g != w {
		t.Fatalf("attempt count mismatch:\nWant: %v\nGot: %v", w, g)
	}
}

func TestClient_ReadWriteTransaction_DMLAborted(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	server.TestSpanner.PutExecutionTime(MethodExecuteSql, SimulatedExecutionTime{
		Errors: []error{status.Error(codes.Aborted, "Aborted")},
	})
	defer teardown()
	ctx := context.Background()
	attempts := 0
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		_, err := tx.Update(ctx, Statement{SQL: UpdateBarSetFoo})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 2; g != w {
		t.Fatalf("attempt count mismatch:\nWant: %v\nGot: %v", w, g)
	}
}

func TestClient_ReadWriteTransaction_BatchDMLAborted(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	server.TestSpanner.PutExecutionTime(MethodExecuteBatchDml, SimulatedExecutionTime{
		Errors: []error{status.Error(codes.Aborted, "Aborted")},
	})
	defer teardown()
	ctx := context.Background()
	attempts := 0
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		_, err := tx.BatchUpdate(ctx, []Statement{{SQL: UpdateBarSetFoo}})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 2; g != w {
		t.Fatalf("attempt count mismatch:\nWant: %v\nGot: %v", w, g)
	}
}

func TestClient_ReadWriteTransaction_BatchDMLAbortedHalfway(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	abortedStatement := "UPDATE FOO_ABORTED SET BAR=1 WHERE ID=2"
	server.TestSpanner.PutStatementResult(
		abortedStatement,
		&StatementResult{
			Type: StatementResultError,
			Err:  status.Error(codes.Aborted, "Statement was aborted"),
		},
	)
	ctx := context.Background()
	var updateCounts []int64
	attempts := 0
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		if attempts > 1 {
			// Replace the aborted result with a real result to prevent the
			// transaction from aborting indefinitely.
			server.TestSpanner.PutStatementResult(
				abortedStatement,
				&StatementResult{
					Type:        StatementResultUpdateCount,
					UpdateCount: 3,
				},
			)
		}
		var err error
		updateCounts, err = tx.BatchUpdate(ctx, []Statement{{SQL: abortedStatement}, {SQL: UpdateBarSetFoo}})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 2; g != w {
		t.Fatalf("attempt count mismatch:\nWant: %v\nGot: %v", w, g)
	}
	if g, w := updateCounts, []int64{3, UpdateBarSetFooRowCount}; !testEqual(w, g) {
		t.Fatalf("update count mismatch\nWant: %v\nGot: %v", w, g)
	}
}

func TestClient_ReadWriteTransaction_QueryAborted(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql, SimulatedExecutionTime{
		Errors: []error{status.Error(codes.Aborted, "Aborted")},
	})
	defer teardown()
	ctx := context.Background()
	attempts := 0
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		iter := tx.Query(ctx, Statement{SQL: SelectFooFromBar})
		defer iter.Stop()
		for {
			_, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 2; g != w {
		t.Fatalf("attempt count mismatch:\nWant: %v\nGot: %v", w, g)
	}
}

func TestClient_ReadWriteTransaction_AbortedOnExecuteStreamingSqlAndCommit(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodExecuteStreamingSql: {Errors: []error{status.Error(codes.Aborted, "Aborted")}},
		MethodCommitTransaction:   {Errors: []error{status.Error(codes.Aborted, "Aborted"), status.Error(codes.Aborted, "Aborted")}},
	}, 4); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransactionCommitAbortedAndUnavailable(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodCommitTransaction: {
			Errors: []error{
				status.Error(codes.Aborted, "Transaction aborted"),
				status.Error(codes.Unavailable, "Unavailable"),
			},
		},
	}, 2); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransactionCommitAlreadyExists(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodCommitTransaction: {Errors: []error{status.Error(codes.AlreadyExists, "A row with this key already exists")}},
	}, 1); err != nil {
		if status.Code(err) != codes.AlreadyExists {
			t.Fatalf("Got unexpected error %v, expected %v", err, codes.AlreadyExists)
		}
	} else {
		t.Fatalf("Missing expected exception")
	}
}

func TestClient_ReadWriteTransactionConcurrentQueries(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	var (
		ctx                 = context.Background()
		wg                  = sync.WaitGroup{}
		firstTransactionID  transactionID
		secondTransactionID transactionID
	)
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		query := func(id *transactionID) {
			defer func() {
				if tx.tx != nil {
					*id = tx.tx
				}
				wg.Done()
			}()
			iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
			defer iter.Stop()
			rowCount := int64(0)
			for {
				row, err := iter.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					return
				}
				var singerID, albumID int64
				var albumTitle string
				if err := row.Columns(&singerID, &albumID, &albumTitle); err != nil {
					return
				}
				rowCount++
			}
		}
		wg.Add(2)
		go query(&firstTransactionID)
		go query(&secondTransactionID)
		wg.Wait()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if firstTransactionID == nil || secondTransactionID == nil || string(firstTransactionID) != string(secondTransactionID) {
		t.Fatalf("transactionID mismatch:\nfirst: %v\nsecong: %v", firstTransactionID, secondTransactionID)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{}}, requests); err != nil {
		t.Fatal(err)
	}
	var (
		callsWithBeginSelector int32
		callsWithTransactionID int32
	)
	for _, req := range requests {
		if sql, ok := req.(*sppb.ExecuteSqlRequest); ok {
			if _, ok := sql.Transaction.GetSelector().(*sppb.TransactionSelector_Begin); ok {
				callsWithBeginSelector++
			}
			if _, ok := sql.Transaction.GetSelector().(*sppb.TransactionSelector_Id); ok {
				callsWithTransactionID++
			}
		}
	}
	if callsWithBeginSelector != 1 || callsWithTransactionID != 1 {
		t.Fatal("first statement in concurrent read/write transaction should use TransactionSelector::Begin "+
			"and others should use transactionID returned from first statement", firstTransactionID, secondTransactionID)
	}
}

// Given a transaction, When the first call to ExecuteStreamingSql/StreamingRead returns an UNAVAILABLE error
// and retry returns Aborted, then the transaction should be retried with an explicit BeginTransaction rpc.
func TestClient_ReadWriteTransaction_FirstStatementAsQueryReturnsUnavailableRetryReturnsAborted(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql,
		SimulatedExecutionTime{
			Errors: []error{status.Error(codes.Unavailable, "Temporary unavailable"), status.Error(codes.Aborted, "Transaction aborted")},
		})
	ctx := context.Background()
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		defer iter.Stop()
		for {
			_, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{}}, requests); err != nil {
		t.Fatal(err)
	}
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}
	if _, ok := requests[1+muxCreateBuffer].(*sppb.ExecuteSqlRequest).Transaction.GetSelector().(*sppb.TransactionSelector_Begin); !ok {
		t.Fatal("expected streaming query to use TransactionSelector::Begin")
	}
	if _, ok := requests[2+muxCreateBuffer].(*sppb.ExecuteSqlRequest).Transaction.GetSelector().(*sppb.TransactionSelector_Begin); !ok {
		t.Fatal("expected streaming query to use TransactionSelector::Begin")
	}
	if _, ok := requests[4+muxCreateBuffer].(*sppb.ExecuteSqlRequest).Transaction.GetSelector().(*sppb.TransactionSelector_Id); !ok {
		t.Fatal("expected streaming query to use transactionID from explicit begin transaction")
	}
}

// Given a transaction, When the StreamingRead fails halfway and stream is restarted with a resume token,
// Then the transaction ID should be used from the first PartialResultSet.
func TestClient_ReadWriteTransaction_FirstStatementAsReadFailsHalfway(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(2),
			Err:         status.Errorf(codes.Internal, "stream terminated by RST_STREAM"),
		},
	)
	_, err := client.ReadWriteTransaction(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
		iter := tx.Read(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"})
		defer iter.Stop()
		for {
			_, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ReadRequest{},
		&sppb.ReadRequest{},
		&sppb.CommitRequest{}}, requests); err != nil {
		t.Fatal(err)
	}
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}
	if _, ok := requests[1+muxCreateBuffer].(*sppb.ReadRequest).Transaction.GetSelector().(*sppb.TransactionSelector_Begin); !ok {
		t.Fatal("expected streaming read to use TransactionSelector::Begin")
	}
	if _, ok := requests[2+muxCreateBuffer].(*sppb.ReadRequest).Transaction.GetSelector().(*sppb.TransactionSelector_Id); !ok {
		t.Fatal("expected streaming read to use transactionID from previous success request")
	}
	if requests[2+muxCreateBuffer].(*sppb.ReadRequest).ResumeToken == nil {
		t.Fatal("expected streaming read to include resume token")
	}
}

func TestClient_ReadWriteTransaction_BatchDmlWithErrorOnFirstStatement(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	invalidStatement := "UPDATE FOO_ABORTED SET BAR=1 WHERE ID=2"
	server.TestSpanner.PutStatementResult(
		invalidStatement,
		&StatementResult{
			Type: StatementResultError,
			Err:  status.Error(codes.InvalidArgument, "Statement was invalid"),
		},
	)
	ctx := context.Background()
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		_, err := tx.BatchUpdate(ctx, []Statement{{SQL: invalidStatement}, {SQL: UpdateBarSetFoo}})
		if err != nil {
			// We know that this statement can fail, but it is acceptable for this transaction,
			// so we just continue with the next statement.
		}
		if _, err := tx.Update(ctx, Statement{SQL: UpdateBarSetFoo}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteBatchDmlRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteBatchDmlRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{}}, requests); err != nil {
		t.Fatal(err)
	}
	// The first statement will fail and not return a transaction id. This will trigger a retry of
	// the entire transaction, and the retry will do an explicit BeginTransaction RPC.
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}
	if _, ok := requests[1+muxCreateBuffer].(*sppb.ExecuteBatchDmlRequest).Transaction.GetSelector().(*sppb.TransactionSelector_Begin); !ok {
		t.Fatal("expected first BatchUpdate to use TransactionSelector::Begin")
	}
	if _, ok := requests[3+muxCreateBuffer].(*sppb.ExecuteBatchDmlRequest).Transaction.GetSelector().(*sppb.TransactionSelector_Id); !ok {
		t.Fatal("expected second BatchUpdate to use transactionID from explicit begin")
	}
	if _, ok := requests[4+muxCreateBuffer].(*sppb.ExecuteSqlRequest).Transaction.GetSelector().(*sppb.TransactionSelector_Id); !ok {
		t.Fatal("expected second ExecuteSqlRequest to use transactionID from explicit begin")
	}
}

func TestClient_ReadWriteTransaction_BatchDmlWithErrorOnSecondStatement(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	invalidStatement := "UPDATE FOO_ABORTED SET BAR=1 WHERE ID=2"
	server.TestSpanner.PutStatementResult(
		invalidStatement,
		&StatementResult{
			Type: StatementResultError,
			Err:  status.Error(codes.InvalidArgument, "Statement was invalid"),
		},
	)
	ctx := context.Background()
	var updateCounts []int64
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		updateCounts, _ = tx.BatchUpdate(ctx, []Statement{{SQL: UpdateBarSetFoo}, {SQL: invalidStatement}})
		if _, err := tx.Update(ctx, Statement{SQL: UpdateBarSetFoo}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := updateCounts, []int64{UpdateBarSetFooRowCount}; !testEqual(w, g) {
		t.Fatalf("update count mismatch\nWant: %v\nGot: %v", w, g)
	}

	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteBatchDmlRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{}}, requests); err != nil {
		t.Fatal(err)
	}
	// Although the batch DML returned an error, that error was for the second statement. That
	// means that the transaction was started by the first statement.
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}
	if _, ok := requests[1+muxCreateBuffer].(*sppb.ExecuteBatchDmlRequest).Transaction.GetSelector().(*sppb.TransactionSelector_Begin); !ok {
		t.Fatal("expected BatchUpdate to use TransactionSelector::Begin")
	}
	if _, ok := requests[2+muxCreateBuffer].(*sppb.ExecuteSqlRequest).Transaction.GetSelector().(*sppb.TransactionSelector_Id); !ok {
		t.Fatal("expected ExecuteSqlRequest use transactionID from BatchUpdate request")
	}
}

func TestClient_ReadWriteTransaction_MultipleReadsWithoutNext(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(2),
			Err:         status.Errorf(codes.Internal, "stream terminated by RST_STREAM"),
		},
	)

	_, err := client.ReadWriteTransaction(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
		iter := tx.Read(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"})
		iter.Stop()
		iter = tx.Read(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"})
		iter.Stop()
		iter = tx.Read(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"})
		iter.Stop()
		iter = tx.Read(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"})
		iter.Stop()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{}}, requests); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_WithCancelledContext(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(2),
			Err:         status.Errorf(codes.Internal, "stream terminated by RST_STREAM"),
		},
	)
	ctx, cancel := context.WithCancel(context.Background())
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		iter := tx.Read(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"})
		if _, err := iter.Next(); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	cancel()
	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		iter := tx.Read(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"})
		if _, err := iter.Next(); err != nil {
			return err
		}
		return nil
	})
	if status.Code(err) != codes.Canceled {
		t.Fatal(err)
	}
}

func testReadWriteTransaction(t *testing.T, executionTimes map[string]SimulatedExecutionTime, expectedAttempts int) error {
	_, teardown, err := testReadWriteTransactionWithConfig(t, ClientConfig{DisableNativeMetrics: true, SessionPoolConfig: DefaultSessionPoolConfig}, executionTimes, expectedAttempts)
	defer teardown()
	return err
}

func testReadWriteTransactionWithConfig(t *testing.T, config ClientConfig, executionTimes map[string]SimulatedExecutionTime, expectedAttempts int) (*MockedSpannerInMemTestServer, func(), error) {
	return testReadWriteTransactionWithConfigWithTransactionOptions(t, config, TransactionOptions{}, executionTimes, expectedAttempts)
}

func testReadWriteTransactionWithConfigWithTransactionOptions(t *testing.T, config ClientConfig, transactionOptions TransactionOptions, executionTimes map[string]SimulatedExecutionTime, expectedAttempts int) (*MockedSpannerInMemTestServer, func(), error) {
	server, client, teardown := setupMockedTestServerWithConfig(t, config)
	for method, exec := range executionTimes {
		server.TestSpanner.PutExecutionTime(method, exec)
	}
	ctx := context.Background()
	var attempts int
	_, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		defer iter.Stop()
		rowCount := int64(0)
		for {
			row, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return err
			}
			var singerID, albumID int64
			var albumTitle string
			if err := row.Columns(&singerID, &albumID, &albumTitle); err != nil {
				return err
			}
			rowCount++
		}
		if rowCount != SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount {
			return status.Errorf(codes.FailedPrecondition, "Row count mismatch, got %v, expected %v", rowCount, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
		}
		return nil
	}, transactionOptions)
	if err != nil {
		return server, teardown, err
	}
	if expectedAttempts != attempts {
		t.Fatalf("unexpected number of attempts: %d, expected %d", attempts, expectedAttempts)
	}
	return server, teardown, nil
}

func TestClient_ApplyAtLeastOnce(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{
			Errors: []error{status.Error(codes.Aborted, "Transaction aborted")},
		})
	_, err := client.Apply(context.Background(), ms, ApplyAtLeastOnce())
	if err != nil {
		t.Fatal(err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	for _, req := range requests {
		if r, ok := req.(*sppb.CommitRequest); ok {
			if r.MaxCommitDelay != nil {
				t.Fatalf("unexpected MaxCommitDelay: %v", r.MaxCommitDelay)
			}
		}
	}

	// Using Max commit delay
	duration := 1 * time.Millisecond
	_, err = client.Apply(context.Background(), ms, ApplyAtLeastOnce(), ApplyCommitOptions(CommitOptions{MaxCommitDelay: &duration}))
	if err != nil {
		t.Fatal(err)
	}
	requests = drainRequestsFromServer(server.TestSpanner)
	for _, req := range requests {
		if r, ok := req.(*sppb.CommitRequest); ok {
			if r.MaxCommitDelay.GetNanos() != durationpb.New(duration).GetNanos() {
				t.Fatalf("unexpected MaxCommitDelay: %v", r.MaxCommitDelay)
			}
		}
	}
}

func TestClient_ApplyAtLeastOnceReuseSession(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:           0,
			WriteSessions:       0.0,
			TrackSessionHandles: true,
		},
	})
	defer teardown()
	sp := client.idleSessions
	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}
	for i := 0; i < 10; i++ {
		_, err := client.Apply(context.Background(), ms, ApplyAtLeastOnce())
		if err != nil {
			t.Fatal(err)
		}
		expectedIdleSesions := sp.incStep
		if isMultiplexEnabled {
			expectedIdleSesions = 0
		}
		sp.mu.Lock()
		if g, w := uint64(sp.idleList.Len())+sp.createReqs, expectedIdleSesions; g != w {
			sp.mu.Unlock()
			t.Fatalf("idle session count mismatch:\nGot: %v\nWant: %v", g, w)
		}
		expectedSessions := expectedIdleSesions
		if isMultiplexEnabled {
			expectedSessions++
		}
		if g, w := uint64(len(server.TestSpanner.DumpSessions())), expectedSessions; g != w {
			sp.mu.Unlock()
			t.Fatalf("server session count mismatch:\nGot: %v\nWant: %v", g, w)
		}
		sp.mu.Unlock()
	}
	// There should be no sessions marked as checked out.
	sp.mu.Lock()
	g, w := sp.trackedSessionHandles.Len(), 0
	sp.mu.Unlock()
	if g != w {
		t.Fatalf("checked out sessions count mismatch:\nGot: %v\nWant: %v", g, w)
	}
}

func TestClient_ApplyAtLeastOnceInvalidArgument(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:           0,
			WriteSessions:       0.0,
			TrackSessionHandles: true,
		},
	})
	defer teardown()
	sp := client.idleSessions
	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}
	for i := 0; i < 10; i++ {
		server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
			SimulatedExecutionTime{
				Errors: []error{status.Error(codes.InvalidArgument, "Invalid data")},
			})
		_, err := client.Apply(context.Background(), ms, ApplyAtLeastOnce())
		if status.Code(err) != codes.InvalidArgument {
			t.Fatal(err)
		}
		sp.mu.Lock()
		expectedIdleSesions := sp.incStep
		if isMultiplexEnabled {
			expectedIdleSesions = 0
		}
		if g, w := uint64(sp.idleList.Len())+sp.createReqs, expectedIdleSesions; g != w {
			sp.mu.Unlock()
			t.Fatalf("idle session count mismatch:\nGot: %v\nWant: %v", g, w)
		}
		var countMuxSess uint64
		if isMultiplexEnabled {
			countMuxSess = 1
		}
		if g, w := uint64(len(server.TestSpanner.DumpSessions())), expectedIdleSesions+countMuxSess; g != w {
			sp.mu.Unlock()
			t.Fatalf("server session count mismatch:\nGot: %v\nWant: %v", g, w)
		}
		sp.mu.Unlock()
	}
	// There should be no sessions marked as checked out.
	client.idleSessions.mu.Lock()
	g, w := client.idleSessions.trackedSessionHandles.Len(), 0
	client.idleSessions.mu.Unlock()
	if g != w {
		t.Fatalf("checked out sessions count mismatch:\nGot: %v\nWant: %v", g, w)
	}
}

func TestClient_ApplyAtLeastOnce_NonRetryableInternalErrors(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{
			Errors: []error{status.Errorf(codes.Internal, "grpc: error while marshaling: string field contains invalid UTF-8")},
		})
	_, err := client.Apply(context.Background(), ms, ApplyAtLeastOnce())
	if status.Code(err) != codes.Internal {
		t.Fatalf("Error mismatch:\ngot: %v\nwant: %v", err, codes.Internal)
	}
}

func TestClient_Apply_ApplyOptions(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name               string
		client             []ApplyOption
		apply              []ApplyOption
		wantTransactionTag string
		wantPriority       sppb.RequestOptions_Priority
	}{
		{
			name:               "At least once & client level",
			client:             []ApplyOption{ApplyAtLeastOnce(), TransactionTag("testTransactionTag"), Priority(sppb.RequestOptions_PRIORITY_LOW)},
			wantTransactionTag: "testTransactionTag",
			wantPriority:       sppb.RequestOptions_PRIORITY_LOW,
		},
		{
			name:               "Not at least once & client level",
			client:             []ApplyOption{TransactionTag("testTransactionTag"), Priority(sppb.RequestOptions_PRIORITY_LOW)},
			wantTransactionTag: "testTransactionTag",
			wantPriority:       sppb.RequestOptions_PRIORITY_LOW,
		},
		{
			name:               "At least once & apply level",
			apply:              []ApplyOption{ApplyAtLeastOnce(), TransactionTag("testTransactionTag"), Priority(sppb.RequestOptions_PRIORITY_LOW)},
			wantTransactionTag: "testTransactionTag",
			wantPriority:       sppb.RequestOptions_PRIORITY_LOW,
		},
		{
			name:               "Not at least once & apply level",
			apply:              []ApplyOption{TransactionTag("testTransactionTag"), Priority(sppb.RequestOptions_PRIORITY_LOW)},
			wantTransactionTag: "testTransactionTag",
			wantPriority:       sppb.RequestOptions_PRIORITY_LOW,
		},
		{
			name:               "At least once & query level has precedence than client level",
			client:             []ApplyOption{ApplyAtLeastOnce(), TransactionTag("clientTransactionTag"), Priority(sppb.RequestOptions_PRIORITY_LOW)},
			apply:              []ApplyOption{ApplyAtLeastOnce(), TransactionTag("applyTransactionTag"), Priority(sppb.RequestOptions_PRIORITY_MEDIUM)},
			wantTransactionTag: "applyTransactionTag",
			wantPriority:       sppb.RequestOptions_PRIORITY_MEDIUM,
		},
		{
			name:               "Not at least once & apply level",
			client:             []ApplyOption{TransactionTag("clientTransactionTag"), Priority(sppb.RequestOptions_PRIORITY_LOW)},
			apply:              []ApplyOption{TransactionTag("applyTransactionTag"), Priority(sppb.RequestOptions_PRIORITY_MEDIUM)},
			wantTransactionTag: "applyTransactionTag",
			wantPriority:       sppb.RequestOptions_PRIORITY_MEDIUM,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, ApplyOptions: tt.client})
			defer teardown()

			_, err := client.Apply(context.Background(), []*Mutation{Insert("foo", []string{"col1"}, []interface{}{"val1"})}, tt.apply...)
			if err != nil {
				t.Fatalf("failed applying mutations: %v", err)
			}
			checkCommitForExpectedRequestOptions(t, server.TestSpanner, &sppb.RequestOptions{Priority: tt.wantPriority, TransactionTag: tt.wantTransactionTag})
		})
	}
}

func TestReadWriteTransaction_ErrUnexpectedEOF(t *testing.T) {
	t.Parallel()
	_, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()
	var attempts int
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		defer iter.Stop()
		for {
			row, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return err
			}
			var singerID, albumID int64
			var albumTitle string
			if err := row.Columns(&singerID, &albumID, &albumTitle); err != nil {
				return err
			}
		}
		return io.ErrUnexpectedEOF
	})
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("Missing expected error %v, got %v", io.ErrUnexpectedEOF, err)
	}
	if attempts != 1 {
		t.Fatalf("unexpected number of attempts: %d, expected %d", attempts, 1)
	}
}

func TestReadWriteTransaction_WrapError(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	// Abort the transaction on both the query as well as commit.
	// The first abort error will be wrapped. The client will unwrap the cause
	// of the error and retry the transaction. The aborted error on commit
	// will not be wrapped, but will also be recognized by the client as an
	// abort that should be retried.
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql,
		SimulatedExecutionTime{
			Errors: []error{status.Error(codes.Aborted, "Transaction aborted")},
		})
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{
			Errors: []error{status.Error(codes.Aborted, "Transaction aborted")},
		})
	msg := "query failed"
	numAttempts := 0
	ctx := context.Background()
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		numAttempts++
		iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		defer iter.Stop()
		for {
			_, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				// Wrap the error in another error that implements the
				// (xerrors|errors).Wrapper interface.
				return &wrappedTestError{err, msg}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Unexpected error\nGot: %v\nWant: nil", err)
	}
	if g, w := numAttempts, 3; g != w {
		t.Fatalf("Number of transaction attempts mismatch\nGot: %d\nWant: %d", w, w)
	}

	// Execute a transaction that returns a non-retryable error that is
	// wrapped in a custom error. The transaction should return the custom
	// error.
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql,
		SimulatedExecutionTime{
			Errors: []error{status.Error(codes.NotFound, "Table not found")},
		})
	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		numAttempts++
		iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		defer iter.Stop()
		for {
			_, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				// Wrap the error in another error that implements the
				// (xerrors|errors).Wrapper interface.
				return &wrappedTestError{err, msg}
			}
		}
		return nil
	})
	if err == nil || err.Error() != msg {
		t.Fatalf("Unexpected error\nGot: %v\nWant: %v", err, msg)
	}
}

func TestReadWriteTransaction_WrapSessionNotFoundError(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql,
		SimulatedExecutionTime{
			Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")},
		})
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{
			Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")},
		})
	msg := "query failed"
	numAttempts := 0
	ctx := context.Background()
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		numAttempts++
		iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		defer iter.Stop()
		for {
			_, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				// Wrap the error in another error that implements the
				// (xerrors|errors).Wrapper interface.
				return &wrappedTestError{err, msg}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Unexpected error\nGot: %v\nWant: nil", err)
	}
	// We want 3 attempts. The 'Session not found' error on BeginTransaction
	// will not retry the entire transaction, which means that we will have two
	// failed attempts and then a successful attempt.
	if g, w := numAttempts, 3; g != w {
		t.Fatalf("Number of transaction attempts mismatch\nGot: %d\nWant: %d", g, w)
	}
}

func TestStmtBasedReadWriteTransaction_SessionNotFoundError_shouldNotPanic(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodBeginTransaction,
		SimulatedExecutionTime{
			Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")},
		})
	ctx := context.Background()
	tx, _ := NewReadWriteStmtBasedTransaction(ctx, client)
	_ = tx.BufferWrite([]*Mutation{Update("my_table", []string{"key", "value"}, []interface{}{int64(1), "my-value"})})
	// This would panic, as it could not refresh the session.
	_, _ = tx.Commit(ctx)
}

func TestClient_WriteStructWithPointers(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	type T struct {
		ID    int64
		Col1  *string
		Col2  []*string
		Col3  *bool
		Col4  []*bool
		Col5  *int64
		Col6  []*int64
		Col7  *float64
		Col8  []*float64
		Col9  *time.Time
		Col10 []*time.Time
		Col11 *civil.Date
		Col12 []*civil.Date
	}
	t1 := T{
		ID:    1,
		Col2:  []*string{nil},
		Col4:  []*bool{nil},
		Col6:  []*int64{nil},
		Col8:  []*float64{nil},
		Col10: []*time.Time{nil},
		Col12: []*civil.Date{nil},
	}
	s := "foo"
	b := true
	i := int64(100)
	f := 3.14
	tm := time.Now()
	d := civil.DateOf(time.Now())
	t2 := T{
		ID:    2,
		Col1:  &s,
		Col2:  []*string{&s},
		Col3:  &b,
		Col4:  []*bool{&b},
		Col5:  &i,
		Col6:  []*int64{&i},
		Col7:  &f,
		Col8:  []*float64{&f},
		Col9:  &tm,
		Col10: []*time.Time{&tm},
		Col11: &d,
		Col12: []*civil.Date{&d},
	}
	m1, err := InsertStruct("Tab", &t1)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := InsertStruct("Tab", &t2)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Apply(context.Background(), []*Mutation{m1, m2})
	if err != nil {
		t.Fatal(err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	for _, req := range requests {
		if commit, ok := req.(*sppb.CommitRequest); ok {
			if g, w := len(commit.Mutations), 2; w != g {
				t.Fatalf("mutation count mismatch\nGot: %v\nWant: %v", g, w)
			}
			insert := commit.Mutations[0].GetInsert()
			// The first insert should contain NULL values and arrays
			// containing exactly one NULL element.
			for i := 1; i < len(insert.Values[0].Values); i += 2 {
				// The non-array columns should contain NULL values.
				g, w := insert.Values[0].Values[i].GetKind(), &structpb.Value_NullValue{}
				if _, ok := g.(*structpb.Value_NullValue); !ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: %v", g, w)
				}
				// The array columns should not be NULL.
				g, wList := insert.Values[0].Values[i+1].GetKind(), &structpb.Value_ListValue{}
				if _, ok := g.(*structpb.Value_ListValue); !ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: %v", g, wList)
				}
				// The array should contain 1 NULL value.
				if gLength, wLength := len(insert.Values[0].Values[i+1].GetListValue().Values), 1; gLength != wLength {
					t.Fatalf("list value length mismatch\nGot: %v\nWant: %v", gLength, wLength)
				}
				g, w = insert.Values[0].Values[i+1].GetListValue().Values[0].GetKind(), &structpb.Value_NullValue{}
				if _, ok := g.(*structpb.Value_NullValue); !ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: %v", g, w)
				}
			}

			// The second insert should contain all non-NULL values.
			insert = commit.Mutations[1].GetInsert()
			for i := 1; i < len(insert.Values[0].Values); i += 2 {
				// The non-array columns should contain non-NULL values.
				g := insert.Values[0].Values[i].GetKind()
				if _, ok := g.(*structpb.Value_NullValue); ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: non-NULL value", g)
				}
				// The array columns should also be non-NULL.
				g, wList := insert.Values[0].Values[i+1].GetKind(), &structpb.Value_ListValue{}
				if _, ok := g.(*structpb.Value_ListValue); !ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: %v", g, wList)
				}
				// The array should contain exactly 1 non-NULL value.
				if gLength, wLength := len(insert.Values[0].Values[i+1].GetListValue().Values), 1; gLength != wLength {
					t.Fatalf("list value length mismatch\nGot: %v\nWant: %v", gLength, wLength)
				}
				g = insert.Values[0].Values[i+1].GetListValue().Values[0].GetKind()
				if _, ok := g.(*structpb.Value_NullValue); ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: non-NULL value", g)
				}
			}
		}
	}
}

func TestClient_WriteStructWithCustomTypes(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	type CustomString string
	type CustomBool bool
	type CustomInt64 int64
	type CustomFloat64 float64
	type CustomTime time.Time
	type CustomDate civil.Date
	type T struct {
		ID    int64
		Col1  CustomString
		Col2  []CustomString
		Col3  CustomBool
		Col4  []CustomBool
		Col5  CustomInt64
		Col6  []CustomInt64
		Col7  CustomFloat64
		Col8  []CustomFloat64
		Col9  CustomTime
		Col10 []CustomTime
		Col11 CustomDate
		Col12 []CustomDate
	}
	t1 := T{
		ID:    1,
		Col2:  []CustomString{},
		Col4:  []CustomBool{},
		Col6:  []CustomInt64{},
		Col8:  []CustomFloat64{},
		Col10: []CustomTime{},
		Col12: []CustomDate{},
	}
	t2 := T{
		ID:    2,
		Col1:  "foo",
		Col2:  []CustomString{"foo"},
		Col3:  true,
		Col4:  []CustomBool{true},
		Col5:  100,
		Col6:  []CustomInt64{100},
		Col7:  3.14,
		Col8:  []CustomFloat64{3.14},
		Col9:  CustomTime(time.Now()),
		Col10: []CustomTime{CustomTime(time.Now())},
		Col11: CustomDate(civil.DateOf(time.Now())),
		Col12: []CustomDate{CustomDate(civil.DateOf(time.Now()))},
	}
	m1, err := InsertStruct("Tab", &t1)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := InsertStruct("Tab", &t2)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Apply(context.Background(), []*Mutation{m1, m2})
	if err != nil {
		t.Fatal(err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	for _, req := range requests {
		if commit, ok := req.(*sppb.CommitRequest); ok {
			if g, w := len(commit.Mutations), 2; w != g {
				t.Fatalf("mutation count mismatch\nGot: %v\nWant: %v", g, w)
			}
			insert1 := commit.Mutations[0].GetInsert()
			row1 := insert1.Values[0]
			// The first insert should contain empty values and empty arrays
			for i := 1; i < len(row1.Values); i += 2 {
				// The non-array columns should contain empty values.
				g := row1.Values[i].GetKind()
				if _, ok := g.(*structpb.Value_NullValue); ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: non-NULL value", g)
				}
				// The array columns should not be NULL.
				g, wList := row1.Values[i+1].GetKind(), &structpb.Value_ListValue{}
				if _, ok := g.(*structpb.Value_ListValue); !ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: %v", g, wList)
				}
			}

			// The second insert should contain all non-NULL values.
			insert2 := commit.Mutations[1].GetInsert()
			row2 := insert2.Values[0]
			for i := 1; i < len(row2.Values); i += 2 {
				// The non-array columns should contain non-NULL values.
				g := row2.Values[i].GetKind()
				if _, ok := g.(*structpb.Value_NullValue); ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: non-NULL value", g)
				}
				// The array columns should also be non-NULL.
				g, wList := row2.Values[i+1].GetKind(), &structpb.Value_ListValue{}
				if _, ok := g.(*structpb.Value_ListValue); !ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: %v", g, wList)
				}
				// The array should contain exactly 1 non-NULL value.
				if gLength, wLength := len(row2.Values[i+1].GetListValue().Values), 1; gLength != wLength {
					t.Fatalf("list value length mismatch\nGot: %v\nWant: %v", gLength, wLength)
				}
				g = row2.Values[i+1].GetListValue().Values[0].GetKind()
				if _, ok := g.(*structpb.Value_NullValue); ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: non-NULL value", g)
				}
			}
		}
	}
}

func TestReadWriteTransaction_ContextTimeoutDuringCommit(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     1,
			WriteSessions: 0,
		},
	})
	defer teardown()

	// Wait until session creation has seized so that
	// context timeout won't happen while a session is being created.
	waitFor(t, func() error {
		sp := client.idleSessions
		sp.mu.Lock()
		defer sp.mu.Unlock()
		if sp.createReqs != 0 {
			return fmt.Errorf("%d sessions are still in creation", sp.createReqs)
		}
		return nil
	})

	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{
			MinimumExecutionTime: time.Minute,
		})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		tx.BufferWrite([]*Mutation{Insert("FOO", []string{"ID", "NAME"}, []interface{}{int64(1), "bar"})})
		return nil
	})

	errContext, cancel := context.WithTimeout(context.Background(), -time.Second)
	defer cancel()

	w := toSpannerErrorWithCommitInfo(errContext.Err(), true).(*Error)
	var se *Error
	if !errors.As(err, &se) {
		t.Fatalf("Error mismatch\nGot: %v\nWant: %v", err, w)
	}
	if se.GRPCStatus().Code() != w.GRPCStatus().Code() {
		t.Fatalf("Error status mismatch:\nGot: %v\nWant: %v", se.GRPCStatus(), w.GRPCStatus())
	}
	// Check that the error code is DeadlineExceeded
	if se.GRPCStatus().Code() != codes.DeadlineExceeded {
		t.Fatalf("Expected error code DeadlineExceeded, got: %v", se.GRPCStatus().Code())
	}

	// Check that the error message contains the essential information
	errMsg := se.Error()
	if !strings.Contains(errMsg, "DeadlineExceeded") {
		t.Errorf("Error message should contain 'DeadlineExceeded', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "transaction outcome unknown") {
		t.Errorf("Error message should contain 'transaction outcome unknown', got: %s", errMsg)
	}

	// Check that the error wraps a TransactionOutcomeUnknownError
	var outcome *TransactionOutcomeUnknownError
	if !errors.As(err, &outcome) {
		t.Fatalf("Missing wrapped TransactionOutcomeUnknownError error")
	}

	if w.RequestID != "" {
		t.Fatal("Missing .RequestID")
	}
}

func TestFailedCommit_NoRollback(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     0,
			MaxOpened:     1,
			WriteSessions: 0,
		},
	})
	defer teardown()

	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{
			Errors: []error{status.Errorf(codes.InvalidArgument, "Invalid mutations")},
		})
	_, err := client.Apply(context.Background(), []*Mutation{
		Insert("FOO", []string{"ID", "BAR"}, []interface{}{1, "value"}),
	})
	if got, want := status.Convert(err).Code(), codes.InvalidArgument; got != want {
		t.Fatalf("Error mismatch\nGot: %v\nWant: %v", got, want)
	}
	// The failed commit should not trigger a rollback after the commit.
	if _, err := shouldHaveReceived(server.TestSpanner, []interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{},
	}); err != nil {
		t.Fatalf("Received RPCs mismatch: %v", err)
	}
}

func TestFailedUpdate_ShouldRollback(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     0,
			MaxOpened:     1,
			WriteSessions: 0,
		},
	})
	defer teardown()

	server.TestSpanner.PutExecutionTime(MethodExecuteSql,
		SimulatedExecutionTime{
			Errors: []error{status.Errorf(codes.InvalidArgument, "Invalid update"), status.Errorf(codes.InvalidArgument, "Invalid update")},
		})
	_, err := client.ReadWriteTransaction(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
		_, err := tx.Update(ctx, NewStatement("UPDATE FOO SET BAR='value' WHERE ID=1"))
		return err
	})
	if got, want := status.Convert(err).Code(), codes.InvalidArgument; got != want {
		t.Fatalf("Error mismatch\nGot: %v\nWant: %v", got, want)
	}
	// The failed update should trigger a rollback.
	if _, err := shouldHaveReceived(server.TestSpanner, []interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
		//	first failure should trigger an explicit BeginTransaction.
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.RollbackRequest{},
	}); err != nil {
		t.Fatalf("Received RPCs mismatch: %v", err)
	}
}

func TestClient_NumChannels(t *testing.T) {
	t.Parallel()

	configuredNumChannels := 8
	gcpPoolNumChannels := 5
	var client *Client
	var teardown func()
	if useGRPCgcp {
		_, client, teardown = setupMockedTestServerWithConfigAndGCPMultiendpointPool(
			t,
			ClientConfig{NumChannels: configuredNumChannels},
			[]option.ClientOption{},
			&grpc_gcp.ChannelPoolConfig{
				MinSize: uint32(gcpPoolNumChannels),
				MaxSize: uint32(gcpPoolNumChannels),
			},
		)
	} else {
		_, client, teardown = setupMockedTestServerWithConfig(
			t,
			ClientConfig{DisableNativeMetrics: true, NumChannels: configuredNumChannels},
		)
	}
	defer teardown()
	w := configuredNumChannels
	if useGRPCgcp {
		w = gcpPoolNumChannels
	}
	if g := client.sc.connPool.Num(); g != w {
		t.Fatalf("NumChannels mismatch\nGot: %v\nWant: %v", g, w)
	}
}

func TestClient_WithGRPCConnectionPool(t *testing.T) {
	t.Parallel()

	configuredConnPool := 8
	gcpPoolNumChannels := 5
	var client *Client
	var teardown func()
	if useGRPCgcp {
		_, client, teardown = setupMockedTestServerWithConfigAndGCPMultiendpointPool(
			t,
			ClientConfig{DisableNativeMetrics: true},
			[]option.ClientOption{option.WithGRPCConnectionPool(configuredConnPool)},
			&grpc_gcp.ChannelPoolConfig{
				MinSize: uint32(gcpPoolNumChannels),
				MaxSize: uint32(gcpPoolNumChannels),
			},
		)
	} else {
		_, client, teardown = setupMockedTestServerWithConfigAndClientOptions(
			t,
			ClientConfig{DisableNativeMetrics: true},
			[]option.ClientOption{option.WithGRPCConnectionPool(configuredConnPool)},
		)
	}
	defer teardown()
	w := configuredConnPool
	if useGRPCgcp {
		w = gcpPoolNumChannels
	}
	if g := client.sc.connPool.Num(); g != w {
		t.Fatalf("NumChannels mismatch\nGot: %v\nWant: %v", g, w)
	}
}

func TestClient_WithGRPCConnectionPoolAndNumChannels(t *testing.T) {
	t.Parallel()

	configuredNumChannels := 8
	configuredConnPool := 8
	gcpPoolNumChannels := 5
	var client *Client
	var teardown func()
	if useGRPCgcp {
		_, client, teardown = setupMockedTestServerWithConfigAndGCPMultiendpointPool(
			t,
			ClientConfig{NumChannels: configuredNumChannels, DisableNativeMetrics: true},
			[]option.ClientOption{option.WithGRPCConnectionPool(configuredConnPool)},
			&grpc_gcp.ChannelPoolConfig{
				MaxSize: uint32(gcpPoolNumChannels),
				MinSize: uint32(gcpPoolNumChannels),
			},
		)
	} else {
		_, client, teardown = setupMockedTestServerWithConfigAndClientOptions(
			t,
			ClientConfig{NumChannels: configuredNumChannels, DisableNativeMetrics: true},
			[]option.ClientOption{option.WithGRPCConnectionPool(configuredConnPool)},
		)
	}
	defer teardown()
	w := configuredConnPool
	if useGRPCgcp {
		w = gcpPoolNumChannels
	}
	if g := client.sc.connPool.Num(); g != w {
		t.Fatalf("NumChannels mismatch\nGot: %v\nWant: %v", g, w)
	}
}

func TestClient_WithGRPCConnectionPoolAndNumChannels_Misconfigured(t *testing.T) {
	t.Parallel()

	// Deliberately misconfigure NumChannels and ConnPool.
	configuredNumChannels := 8
	configuredConnPool := 16

	server, opts, serverTeardown := NewMockedSpannerInMemTestServer(t)
	defer serverTeardown()
	opts = append(opts, option.WithGRPCConnectionPool(configuredConnPool))

	config := ClientConfig{NumChannels: configuredNumChannels, DisableNativeMetrics: true}
	_, err := makeClientWithConfig(context.Background(), "projects/p/instances/i/databases/d", config, server.ServerAddress, opts...)
	if useGRPCgcp {
		// GCPMultiEndpoint channel pool config is preceeding default pool config.
		// I.e., pool config is ignored when GCPMultiEndpoint is used.
		if err != nil {
			t.Fatalf("Error mismatch\nGot: %v\nWant: nil", err)
		}
		return
	}

	msg := "Connection pool mismatch:"
	if err == nil {
		t.Fatalf("Error mismatch\nGot: nil\nWant: %s", msg)
	}
	var se *Error
	if ok := errors.As(err, &se); !ok {
		t.Fatalf("Error mismatch\nGot: %v\nWant: An instance of a Spanner error", err)
	}
	if g, w := se.GRPCStatus().Code(), codes.InvalidArgument; g != w {
		t.Fatalf("Error code mismatch\nGot: %v\nWant: %v", g, w)
	}
	if !strings.Contains(se.Error(), msg) {
		t.Fatalf("Error message mismatch\nGot: %s\nWant: %s", se.Error(), msg)
	}
}

func TestClient_EndToEndTracingHeader(t *testing.T) {
	tests := []struct {
		name                  string
		endToEndTracingEnv    string
		enableEndToEndTracing bool
		wantEndToEndTracing   bool
	}{
		{
			name:                  "when end-to-end tracing is enabled via config",
			enableEndToEndTracing: true,
			wantEndToEndTracing:   true,
			endToEndTracingEnv:    "false",
		},
		{
			name:                  "when end-to-end tracing is enabled via env",
			enableEndToEndTracing: false,
			wantEndToEndTracing:   true,
			endToEndTracingEnv:    "true",
		},
		{
			name:                  "when end-to-end tracing is disabled",
			enableEndToEndTracing: false,
			wantEndToEndTracing:   false,
			endToEndTracingEnv:    "false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SPANNER_ENABLE_END_TO_END_TRACING", tt.endToEndTracingEnv)

			server, opts, teardown := NewMockedSpannerInMemTestServer(t)
			defer teardown()
			config := ClientConfig{}
			if tt.enableEndToEndTracing {
				config.EnableEndToEndTracing = true
			}

			client, err := makeClientWithConfig(context.Background(), "projects/p/instances/i/databases/d", config, server.ServerAddress, opts...)
			if err != nil {
				t.Fatalf("failed to get a client: %v", err)
			}

			gotEndToEndTracing := false
			for _, val := range client.sc.md.Get(endToEndTracingHeader) {
				if val == "true" {
					gotEndToEndTracing = true
				}
			}

			if gotEndToEndTracing != tt.wantEndToEndTracing {
				t.Fatalf("mismatch in client configuration for property EnableEndToEndTracing: got %v, want %v", gotEndToEndTracing, tt.wantEndToEndTracing)
			}
		})
	}
}

func TestClient_WithCustomBatchTimeout(t *testing.T) {
	t.Parallel()

	server, opts, serverTeardown := NewMockedSpannerInMemTestServer(t)
	defer serverTeardown()

	wantBatchTimeout := time.Second * 42
	config := ClientConfig{BatchTimeout: wantBatchTimeout}
	client, err := makeClientWithConfig(context.Background(), "projects/p/instances/i/databases/d", config, server.ServerAddress, opts...)
	if err != nil {
		t.Fatalf("failed to get a client: %v", err)
	}
	if wantBatchTimeout != client.sc.batchTimeout {
		t.Fatalf("mismatch in client configuration for property BatchTimeout: got %v, want %v", client.sc.batchTimeout, wantBatchTimeout)
	}
}

var makeMockServer = NewMockedSpannerInMemTestServer

func TestClient_WithoutCustomBatchTimeout(t *testing.T) {
	t.Parallel()

	server, opts, serverTeardown := NewMockedSpannerInMemTestServer(t)
	defer serverTeardown()

	wantBatchTimeout := time.Minute
	client, err := makeClient(context.Background(), "projects/p/instances/i/databases/d", server.ServerAddress, opts...)
	if err != nil {
		t.Fatalf("failed to get a client: %v", err)
	}
	if wantBatchTimeout != client.sc.batchTimeout {
		t.Fatalf("mismatch in client configuration for property BatchTimeout: got %v, want %v", client.sc.batchTimeout, wantBatchTimeout)
	}
}

func TestClient_CallOptions(t *testing.T) {
	t.Parallel()
	co := &vkit.CallOptions{
		CreateSession: []gax.CallOption{
			gax.WithRetry(func() gax.Retryer {
				return gax.OnCodes([]codes.Code{
					codes.Unavailable, codes.DeadlineExceeded,
				}, gax.Backoff{
					Initial:    200 * time.Millisecond,
					Max:        30000 * time.Millisecond,
					Multiplier: 1.25,
				})
			}),
		},
	}

	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, CallOptions: co})
	defer teardown()

	c, err := client.sc.nextClient()
	if err != nil {
		t.Fatalf("failed to get a session client: %v", err)
	}

	cs := &gax.CallSettings{}
	// This is the default retry setting.
	c.CallOptions().CreateSession[1].Resolve(cs)
	if got, want := fmt.Sprintf("%v", cs.Retry()), "&{{250000000 32000000000 1.3 0} [14 8]}"; got != want {
		t.Fatalf("merged CallOptions is incorrect: got %v, want %v", got, want)
	}

	// This is the custom retry setting.
	c.CallOptions().CreateSession[2].Resolve(cs)
	if got, want := fmt.Sprintf("%v", cs.Retry()), "&{{200000000 30000000000 1.25 0} [14 4]}"; got != want {
		t.Fatalf("merged CallOptions is incorrect: got %v, want %v", got, want)
	}
}

func TestClient_QueryWithCallOptions(t *testing.T) {
	t.Parallel()
	co := &vkit.CallOptions{
		ExecuteSql: []gax.CallOption{
			gax.WithRetry(func() gax.Retryer {
				return gax.OnCodes([]codes.Code{
					codes.DeadlineExceeded,
				}, gax.Backoff{
					Initial:    200 * time.Millisecond,
					Max:        30000 * time.Millisecond,
					Multiplier: 1.25,
				})
			}),
		},
	}
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, CallOptions: co})
	server.TestSpanner.PutExecutionTime(MethodExecuteSql, SimulatedExecutionTime{
		Errors: []error{status.Error(codes.DeadlineExceeded, "Deadline exceeded")},
	})
	defer teardown()
	ctx := context.Background()
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		_, err := tx.Update(ctx, Statement{SQL: UpdateBarSetFoo})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_ShouldReceiveMetadataForEmptyResultSet(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	// This creates an empty result set.
	res := server.CreateSingleRowSingersResult(SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
	sql := "SELECT SingerId, AlbumId, AlbumTitle FROM Albums WHERE 1=2"
	server.TestSpanner.PutStatementResult(sql, res)
	defer teardown()
	ctx := context.Background()
	iter := client.Single().Query(ctx, NewStatement(sql))
	defer iter.Stop()
	row, err := iter.Next()
	if err != iterator.Done {
		t.Errorf("Query result mismatch:\nGot: %v\nWant: <no rows>", row)
	}
	metadata := iter.Metadata
	if metadata == nil {
		t.Fatalf("Missing ResultSet Metadata")
	}
	if metadata.RowType == nil {
		t.Fatalf("Missing ResultSet RowType")
	}
	if metadata.RowType.Fields == nil {
		t.Fatalf("Missing ResultSet Fields")
	}
	if g, w := len(metadata.RowType.Fields), 3; g != w {
		t.Fatalf("Field count mismatch\nGot: %v\nWant: %v", g, w)
	}
	wantFieldNames := []string{"SingerId", "AlbumId", "AlbumTitle"}
	for i, w := range wantFieldNames {
		g := metadata.RowType.Fields[i].Name
		if g != w {
			t.Fatalf("Field[%v] name mismatch\nGot: %v\nWant: %v", i, g, w)
		}
	}
	wantFieldTypes := []sppb.TypeCode{sppb.TypeCode_INT64, sppb.TypeCode_INT64, sppb.TypeCode_STRING}
	for i, w := range wantFieldTypes {
		g := metadata.RowType.Fields[i].Type.Code
		if g != w {
			t.Fatalf("Field[%v] type mismatch\nGot: %v\nWant: %v", i, g, w)
		}
	}
}

func TestClient_EncodeCustomFieldType(t *testing.T) {
	t.Parallel()

	type typesTable struct {
		Int    customStructToInt    `spanner:"Int"`
		String customStructToString `spanner:"String"`
		Float  customStructToFloat  `spanner:"Float"`
		Bool   customStructToBool   `spanner:"Bool"`
		Time   customStructToTime   `spanner:"Time"`
		Date   customStructToDate   `spanner:"Date"`
	}

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()

	d := typesTable{
		Int:    customStructToInt{1, 23},
		String: customStructToString{"A", "B"},
		Float:  customStructToFloat{1.23, 12.3},
		Bool:   customStructToBool{true, false},
		Time:   customStructToTime{"A", "B"},
		Date:   customStructToDate{"A", "B"},
	}

	m, err := InsertStruct("Types", &d)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	ms := []*Mutation{m}
	_, err = client.Apply(ctx, ms)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	reqs := drainRequestsFromServer(server.TestSpanner)

	for _, req := range reqs {
		if commitReq, ok := req.(*sppb.CommitRequest); ok {
			val := commitReq.Mutations[0].GetInsert().Values[0]

			if got, want := val.Values[0].GetStringValue(), "123"; got != want {
				t.Fatalf("value mismatch: got %v (kind %T), want %v", got, val.Values[0].GetKind(), want)
			}
			if got, want := val.Values[1].GetStringValue(), "A-B"; got != want {
				t.Fatalf("value mismatch: got %v (kind %T), want %v", got, val.Values[1].GetKind(), want)
			}
			if got, want := val.Values[2].GetNumberValue(), float64(123.123); got != want {
				t.Fatalf("value mismatch: got %v (kind %T), want %v", got, val.Values[2].GetKind(), want)
			}
			if got, want := val.Values[3].GetBoolValue(), true; got != want {
				t.Fatalf("value mismatch: got %v (kind %T), want %v", got, val.Values[3].GetKind(), want)
			}
			if got, want := val.Values[4].GetStringValue(), "2016-11-15T15:04:05.999999999Z"; got != want {
				t.Fatalf("value mismatch: got %v (kind %T), want %v", got, val.Values[4].GetKind(), want)
			}
			if got, want := val.Values[5].GetStringValue(), "2016-11-15"; got != want {
				t.Fatalf("value mismatch: got %v (kind %T), want %v", got, val.Values[5].GetKind(), want)
			}
		}
	}
}

func setupDecodeCustomFieldResult(server *MockedSpannerInMemTestServer, stmt string) error {
	metadata := &sppb.ResultSetMetadata{
		RowType: &sppb.StructType{
			Fields: []*sppb.StructType_Field{
				{Name: "Int", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
				{Name: "String", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
				{Name: "Float", Type: &sppb.Type{Code: sppb.TypeCode_FLOAT64}},
				{Name: "Bool", Type: &sppb.Type{Code: sppb.TypeCode_BOOL}},
				{Name: "Time", Type: &sppb.Type{Code: sppb.TypeCode_TIMESTAMP}},
				{Name: "Date", Type: &sppb.Type{Code: sppb.TypeCode_DATE}},
			},
		},
	}
	rowValues := []*structpb.Value{
		{Kind: &structpb.Value_StringValue{StringValue: "123"}},
		{Kind: &structpb.Value_StringValue{StringValue: "A-B"}},
		{Kind: &structpb.Value_NumberValue{NumberValue: float64(123.123)}},
		{Kind: &structpb.Value_BoolValue{BoolValue: true}},
		{Kind: &structpb.Value_StringValue{StringValue: "2016-11-15T15:04:05.999999999Z"}},
		{Kind: &structpb.Value_StringValue{StringValue: "2016-11-15"}},
	}
	rows := []*structpb.ListValue{
		{Values: rowValues},
	}
	resultSet := &sppb.ResultSet{
		Metadata: metadata,
		Rows:     rows,
	}
	result := &StatementResult{
		Type:      StatementResultResultSet,
		ResultSet: resultSet,
	}
	return server.TestSpanner.PutStatementResult(stmt, result)
}

func TestClient_DecodeCustomFieldType(t *testing.T) {
	t.Parallel()

	type typesTable struct {
		Int    customStructToInt    `spanner:"Int"`
		String customStructToString `spanner:"String"`
		Float  customStructToFloat  `spanner:"Float"`
		Bool   customStructToBool   `spanner:"Bool"`
		Time   customStructToTime   `spanner:"Time"`
		Date   customStructToDate   `spanner:"Date"`
	}

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	query := "SELECT * FROM Types"
	setupDecodeCustomFieldResult(server, query)

	ctx := context.Background()
	stmt := Statement{SQL: query}
	iter := client.Single().Query(ctx, stmt)
	defer iter.Stop()

	var results []typesTable
	var lenientResults []typesTable
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatalf("failed to get next: %v", err)
		}

		var d typesTable
		if err := row.ToStruct(&d); err != nil {
			t.Fatalf("failed to convert a row to a struct: %v", err)
		}
		results = append(results, d)

		var d2 typesTable
		if err := row.ToStructLenient(&d2); err != nil {
			t.Fatalf("failed to convert a row to a struct: %v", err)
		}
		lenientResults = append(lenientResults, d2)
	}

	if len(results) > 1 || len(lenientResults) > 1 {
		t.Fatalf("mismatch length of array: got %v, want 1", results)
	}

	want := typesTable{
		Int:    customStructToInt{1, 23},
		String: customStructToString{"A", "B"},
		Float:  customStructToFloat{1.23, 12.3},
		Bool:   customStructToBool{true, false},
		Time:   customStructToTime{"A", "B"},
		Date:   customStructToDate{"A", "B"},
	}
	got := results[0]
	if !testEqual(got, want) {
		t.Fatalf("mismatch result from ToStruct: got %v, want %v", got, want)
	}
	got = lenientResults[0]
	if !testEqual(got, want) {
		t.Fatalf("mismatch result from ToStructLenient: got %v, want %v", got, want)
	}
}

func TestClient_EmulatorWithCredentialsFile(t *testing.T) {
	old := os.Getenv("SPANNER_EMULATOR_HOST")
	defer os.Setenv("SPANNER_EMULATOR_HOST", old)

	os.Setenv("SPANNER_EMULATOR_HOST", "localhost:1234")

	opts := []option.ClientOption{
		option.WithCredentialsFile("/path/to/key.json"),
	}
	client, err := makeClient(
		context.Background(),
		"projects/p/instances/i/databases/d",
		"localhost:1234",
		opts...,
	)
	if err != nil {
		t.Fatalf("Failed to create a client with credentials file when running against an emulator: %v", err)
	}
	defer client.Close()
}

func TestBatchReadOnlyTransaction_QueryOptions(t *testing.T) {
	ctx := context.Background()
	qo := QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{
		OptimizerVersion:           "1",
		OptimizerStatisticsPackage: "latest",
	}}
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, QueryOptions: qo})
	defer teardown()

	txn, err := client.BatchReadOnlyTransaction(ctx, StrongRead())
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Cleanup(ctx)

	if txn.qo != qo {
		t.Fatalf("Query options are mismatched: got %v, want %v", txn.qo, qo)
	}
}

func TestBatchReadOnlyTransactionFromID_QueryOptions(t *testing.T) {
	qo := QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{
		OptimizerVersion:           "1",
		OptimizerStatisticsPackage: "latest",
	}}
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, QueryOptions: qo})
	defer teardown()

	txn := client.BatchReadOnlyTransactionFromID(BatchReadOnlyTransactionID{})

	if txn.qo != qo {
		t.Fatalf("Query options are mismatched: got %v, want %v", txn.qo, qo)
	}
}

func TestBatchReadOnlyTransaction_ReadOptions(t *testing.T) {
	ctx := context.Background()
	ro := ReadOptions{
		Index:      "testIndex",
		Limit:      100,
		Priority:   sppb.RequestOptions_PRIORITY_LOW,
		RequestTag: "testRequestTag",
	}
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, ReadOptions: ro})
	defer teardown()

	txn, err := client.BatchReadOnlyTransaction(ctx, StrongRead())
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Cleanup(ctx)

	if txn.ro != ro {
		t.Fatalf("Read options are mismatched: got %v, want %v", txn.ro, ro)
	}
}

func TestBatchReadOnlyTransactionFromID_ReadOptions(t *testing.T) {
	ro := ReadOptions{
		Index:      "testIndex",
		Limit:      100,
		Priority:   sppb.RequestOptions_PRIORITY_LOW,
		RequestTag: "testRequestTag",
	}
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, ReadOptions: ro})
	defer teardown()

	txn := client.BatchReadOnlyTransactionFromID(BatchReadOnlyTransactionID{})

	if txn.ro != ro {
		t.Fatalf("Read options are mismatched: got %v, want %v", txn.ro, ro)
	}
}

type QueryOptionsTestCase struct {
	name      string
	client    QueryOptions
	clientDRO *sppb.DirectedReadOptions
	env       QueryOptions
	query     QueryOptions
	want      QueryOptions
}

func queryOptionsTestCases() []QueryOptionsTestCase {
	statsPkg := "latest"
	return []QueryOptionsTestCase{
		{
			name:   "Client level",
			client: QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1", OptimizerStatisticsPackage: statsPkg}},
			env:    QueryOptions{Options: nil},
			query:  QueryOptions{Options: nil},
			want:   QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1", OptimizerStatisticsPackage: statsPkg}},
		},
		{
			name:   "Environment level",
			client: QueryOptions{Options: nil},
			env:    QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1", OptimizerStatisticsPackage: statsPkg}},
			query:  QueryOptions{Options: nil},
			want:   QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1", OptimizerStatisticsPackage: statsPkg}},
		},
		{
			name:   "Query level",
			client: QueryOptions{Options: nil},
			env:    QueryOptions{Options: nil},
			query:  QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1", OptimizerStatisticsPackage: statsPkg}},
			want:   QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1", OptimizerStatisticsPackage: statsPkg}},
		},
		{
			name:   "Environment level has precedence",
			client: QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1", OptimizerStatisticsPackage: statsPkg}},
			env:    QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "2", OptimizerStatisticsPackage: statsPkg}},
			query:  QueryOptions{Options: nil},
			want:   QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "2", OptimizerStatisticsPackage: statsPkg}},
		},
		{
			name:   "Query level has precedence than client level",
			client: QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1", OptimizerStatisticsPackage: statsPkg}},
			env:    QueryOptions{Options: nil},
			query:  QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "3", OptimizerStatisticsPackage: statsPkg}},
			want:   QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "3", OptimizerStatisticsPackage: statsPkg}},
		},
		{
			name:   "Query level has highest precedence",
			client: QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1", OptimizerStatisticsPackage: statsPkg}},
			env:    QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "2", OptimizerStatisticsPackage: statsPkg}},
			query:  QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "3", OptimizerStatisticsPackage: statsPkg}},
			want:   QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "3", OptimizerStatisticsPackage: statsPkg}},
		},
	}
}

type ReadOptionsTestCase struct {
	name      string
	client    *ReadOptions
	clientDRO *sppb.DirectedReadOptions
	read      *ReadOptions
	want      *ReadOptions
}

func readOptionsTestCases() []ReadOptionsTestCase {
	return []ReadOptionsTestCase{
		{
			name:   "Client level",
			client: &ReadOptions{Index: "testIndex", Limit: 100, Priority: sppb.RequestOptions_PRIORITY_LOW, RequestTag: "testRequestTag", OrderBy: sppb.ReadRequest_ORDER_BY_NO_ORDER},
			want:   &ReadOptions{Index: "testIndex", Limit: 100, Priority: sppb.RequestOptions_PRIORITY_LOW, RequestTag: "testRequestTag", OrderBy: sppb.ReadRequest_ORDER_BY_NO_ORDER},
		},
		{
			name:   "Client level has precendence when ORDER_BY_UNSPECIFIED at read level",
			client: &ReadOptions{Index: "testIndex", Limit: 100, Priority: sppb.RequestOptions_PRIORITY_LOW, RequestTag: "testRequestTag", OrderBy: sppb.ReadRequest_ORDER_BY_NO_ORDER},
			read:   &ReadOptions{Index: "testIndex", Limit: 100, Priority: sppb.RequestOptions_PRIORITY_LOW, RequestTag: "testRequestTag"},
			want:   &ReadOptions{Index: "testIndex", Limit: 100, Priority: sppb.RequestOptions_PRIORITY_LOW, RequestTag: "testRequestTag", OrderBy: sppb.ReadRequest_ORDER_BY_NO_ORDER},
		},
		{
			name:   "Read level",
			client: &ReadOptions{},
			read:   &ReadOptions{Index: "testIndex", Limit: 100, Priority: sppb.RequestOptions_PRIORITY_LOW, RequestTag: "testRequestTag", OrderBy: sppb.ReadRequest_ORDER_BY_NO_ORDER},
			want:   &ReadOptions{Index: "testIndex", Limit: 100, Priority: sppb.RequestOptions_PRIORITY_LOW, RequestTag: "testRequestTag", OrderBy: sppb.ReadRequest_ORDER_BY_NO_ORDER},
		},
		{
			name:   "Read level has precedence than client level",
			client: &ReadOptions{Index: "clientIndex", Limit: 10, Priority: sppb.RequestOptions_PRIORITY_LOW, RequestTag: "clientRequestTag", OrderBy: sppb.ReadRequest_ORDER_BY_NO_ORDER},
			read:   &ReadOptions{Index: "readIndex", Limit: 20, Priority: sppb.RequestOptions_PRIORITY_MEDIUM, RequestTag: "readRequestTag", OrderBy: sppb.ReadRequest_ORDER_BY_PRIMARY_KEY},
			want:   &ReadOptions{Index: "readIndex", Limit: 20, Priority: sppb.RequestOptions_PRIORITY_MEDIUM, RequestTag: "readRequestTag", OrderBy: sppb.ReadRequest_ORDER_BY_PRIMARY_KEY},
		},
	}
}

type TransactionOptionsTestCase struct {
	name   string
	client *TransactionOptions
	write  *TransactionOptions
	want   *TransactionOptions
}

func transactionOptionsTestCases() []TransactionOptionsTestCase {
	duration, _ := time.ParseDuration("100ms")
	otherDuration, _ := time.ParseDuration("50ms")

	return []TransactionOptionsTestCase{
		{
			name:   "Client level",
			client: &TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true}, TransactionTag: "testTransactionTag", CommitPriority: sppb.RequestOptions_PRIORITY_LOW},
			want:   &TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true}, TransactionTag: "testTransactionTag", CommitPriority: sppb.RequestOptions_PRIORITY_LOW},
		},
		{
			name:   "Client level with MaxCommitDelay",
			client: &TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true, MaxCommitDelay: &duration}, TransactionTag: "testTransactionTag", CommitPriority: sppb.RequestOptions_PRIORITY_LOW},
			want:   &TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true, MaxCommitDelay: &duration}, TransactionTag: "testTransactionTag", CommitPriority: sppb.RequestOptions_PRIORITY_LOW},
		},
		{
			name:   "Write level",
			client: &TransactionOptions{},
			write:  &TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true}, TransactionTag: "testTransactionTag", CommitPriority: sppb.RequestOptions_PRIORITY_LOW},
			want:   &TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true}, TransactionTag: "testTransactionTag", CommitPriority: sppb.RequestOptions_PRIORITY_LOW},
		},
		{
			name:   "Write level with MaxCommitDelay",
			client: &TransactionOptions{},
			write:  &TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true, MaxCommitDelay: &duration}, TransactionTag: "testTransactionTag", CommitPriority: sppb.RequestOptions_PRIORITY_LOW},
			want:   &TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true, MaxCommitDelay: &duration}, TransactionTag: "testTransactionTag", CommitPriority: sppb.RequestOptions_PRIORITY_LOW},
		},
		{
			name:   "Write level has precedence than client level",
			client: &TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: false}, TransactionTag: "clientTransactionTag", CommitPriority: sppb.RequestOptions_PRIORITY_LOW},
			write:  &TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true}, TransactionTag: "writeTransactionTag", CommitPriority: sppb.RequestOptions_PRIORITY_MEDIUM},
			want:   &TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true}, TransactionTag: "writeTransactionTag", CommitPriority: sppb.RequestOptions_PRIORITY_MEDIUM},
		},
		{
			name:   "Write level nil MaxCommitDelay does not unset client level MaxCommitDelay",
			client: &TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: false, MaxCommitDelay: &duration}, TransactionTag: "clientTransactionTag", CommitPriority: sppb.RequestOptions_PRIORITY_LOW},
			write:  &TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true}, TransactionTag: "writeTransactionTag", CommitPriority: sppb.RequestOptions_PRIORITY_MEDIUM},
			want:   &TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true, MaxCommitDelay: &duration}, TransactionTag: "writeTransactionTag", CommitPriority: sppb.RequestOptions_PRIORITY_MEDIUM},
		},
		{
			name:   "Write level has precedence than client level MaxCommitDelay",
			client: &TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: false, MaxCommitDelay: &duration}, TransactionTag: "clientTransactionTag", CommitPriority: sppb.RequestOptions_PRIORITY_LOW},
			write:  &TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true, MaxCommitDelay: &otherDuration}, TransactionTag: "writeTransactionTag", CommitPriority: sppb.RequestOptions_PRIORITY_MEDIUM},
			want:   &TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true, MaxCommitDelay: &otherDuration}, TransactionTag: "writeTransactionTag", CommitPriority: sppb.RequestOptions_PRIORITY_MEDIUM},
		},
		{
			name:   "Read lock mode is optimistic",
			client: &TransactionOptions{ReadLockMode: sppb.TransactionOptions_ReadWrite_OPTIMISTIC},
			write:  &TransactionOptions{},
			want:   &TransactionOptions{},
		},
	}
}

func TestClient_DoForEachRow_ShouldNotEndSpanWithIteratorDoneError(t *testing.T) {
	t.Skip("open census spans are no longer exported by gapics")
	// This test cannot be parallel, as the TestExporter does not support that.
	te := NewTestExporter()
	defer te.Unregister()
	minOpened := uint64(1)
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     minOpened,
			WriteSessions: 0,
		},
	})
	defer teardown()

	// Wait until all sessions have been created, so we know that those requests will not interfere with the test.
	sp := client.idleSessions
	waitFor(t, func() error {
		sp.mu.Lock()
		defer sp.mu.Unlock()
		if uint64(sp.idleList.Len()) != minOpened {
			return fmt.Errorf("num open sessions mismatch\nWant: %d\nGot: %d", sp.MinOpened, sp.numOpened)
		}
		return nil
	})

	iter := client.Single().Query(context.Background(), NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	iter.Do(func(r *Row) error {
		return nil
	})
	select {
	case <-te.Stats:
	case <-time.After(1 * time.Second):
		t.Fatal("No stats were exported before timeout")
	}
	spans := te.Spans()
	if len(spans) == 0 {
		t.Fatal("No spans were exported")
	}
	s := spans[len(spans)-1].Status
	if s.Code != int32(codes.OK) {
		t.Errorf("Span status mismatch\nGot: %v\nWant: %v", s.Code, codes.OK)
	}
}

func TestClient_DoForEachRow_ShouldEndSpanWithQueryError(t *testing.T) {
	t.Skip("open census spans are no longer exported by gapics")
	// This test cannot be parallel, as the TestExporter does not support that.
	te := NewTestExporter()
	defer te.Unregister()
	minOpened := uint64(1)
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     minOpened,
			WriteSessions: 0,
		},
	})
	defer teardown()

	// Wait until all sessions have been created, so we know that those requests will not interfere with the test.
	sp := client.idleSessions
	waitFor(t, func() error {
		sp.mu.Lock()
		defer sp.mu.Unlock()
		if uint64(sp.idleList.Len()) != minOpened {
			return fmt.Errorf("num open sessions mismatch\nWant: %d\nGot: %d", sp.MinOpened, sp.numOpened)
		}
		return nil
	})

	sql := "SELECT * FROM"
	server.TestSpanner.PutStatementResult(sql, &StatementResult{
		Type: StatementResultError,
		Err:  status.Error(codes.InvalidArgument, "Invalid query"),
	})

	iter := client.Single().Query(context.Background(), NewStatement(sql))
	iter.Do(func(r *Row) error {
		return nil
	})
	select {
	case <-te.Stats:
	case <-time.After(1 * time.Second):
		t.Fatal("No stats were exported before timeout")
	}
	spans := te.Spans()
	if len(spans) == 0 {
		t.Fatal("No spans were exported")
	}
	s := spans[len(spans)-1].Status
	if s.Code != int32(codes.InvalidArgument) {
		t.Errorf("Span status mismatch\nGot: %v\nWant: %v", s.Code, codes.InvalidArgument)
	}
}

func TestClient_ReadOnlyTransaction_Priority(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	for _, qo := range []QueryOptions{
		{},
		{Priority: sppb.RequestOptions_PRIORITY_HIGH},
	} {
		for _, tx := range []*ReadOnlyTransaction{
			client.Single(),
			client.ReadOnlyTransaction(),
		} {
			iter := tx.QueryWithOptions(context.Background(), NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), qo)
			iter.Next()
			iter.Stop()

			if tx.singleUse {
				tx = client.Single()
			}
			iter = tx.ReadWithOptions(context.Background(), "FOO", AllKeys(), []string{"BAR"}, &ReadOptions{Priority: qo.Priority})
			iter.Next()
			iter.Stop()

			checkRequestsForExpectedRequestOptions(t, server.TestSpanner, 2, &sppb.RequestOptions{Priority: qo.Priority})
			tx.Close()
		}
	}
}

func TestClient_ReadWriteTransaction_Priority(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	for _, to := range []TransactionOptions{
		{},
		{CommitPriority: sppb.RequestOptions_PRIORITY_MEDIUM},
	} {
		for _, qo := range []QueryOptions{
			{},
			{Priority: sppb.RequestOptions_PRIORITY_MEDIUM},
		} {
			client.ReadWriteTransactionWithOptions(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
				iter := tx.QueryWithOptions(context.Background(), NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), qo)
				iter.Next()
				iter.Stop()

				iter = tx.ReadWithOptions(context.Background(), "FOO", AllKeys(), []string{"BAR"}, &ReadOptions{Priority: qo.Priority})
				iter.Next()
				iter.Stop()

				tx.UpdateWithOptions(context.Background(), NewStatement(UpdateBarSetFoo), qo)
				tx.BatchUpdateWithOptions(context.Background(), []Statement{
					NewStatement(UpdateBarSetFoo),
				}, qo)
				checkRequestsForExpectedRequestOptions(t, server.TestSpanner, 4, &sppb.RequestOptions{Priority: qo.Priority})

				return nil
			}, to)
			checkCommitForExpectedRequestOptions(t, server.TestSpanner, &sppb.RequestOptions{Priority: to.CommitPriority})
		}
	}
}

func TestClient_StmtBasedReadWriteTransaction_Priority(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	for _, to := range []TransactionOptions{
		{},
		{CommitPriority: sppb.RequestOptions_PRIORITY_LOW},
	} {
		for _, qo := range []QueryOptions{
			{},
			{Priority: sppb.RequestOptions_PRIORITY_LOW},
		} {
			tx, _ := NewReadWriteStmtBasedTransactionWithOptions(context.Background(), client, to)
			iter := tx.QueryWithOptions(context.Background(), NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), qo)
			iter.Next()
			iter.Stop()

			iter = tx.ReadWithOptions(context.Background(), "FOO", AllKeys(), []string{"BAR"}, &ReadOptions{Priority: qo.Priority})
			iter.Next()
			iter.Stop()

			tx.UpdateWithOptions(context.Background(), NewStatement(UpdateBarSetFoo), qo)
			tx.BatchUpdateWithOptions(context.Background(), []Statement{
				NewStatement(UpdateBarSetFoo),
			}, qo)

			checkRequestsForExpectedRequestOptions(t, server.TestSpanner, 4, &sppb.RequestOptions{Priority: qo.Priority})
			tx.Commit(context.Background())
			checkCommitForExpectedRequestOptions(t, server.TestSpanner, &sppb.RequestOptions{Priority: to.CommitPriority})
		}
	}
}

func TestClient_PDML_Priority(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	for _, qo := range []QueryOptions{
		{},
		{Priority: sppb.RequestOptions_PRIORITY_HIGH},
	} {
		client.PartitionedUpdateWithOptions(context.Background(), NewStatement(UpdateBarSetFoo), qo)
		checkRequestsForExpectedRequestOptions(t, server.TestSpanner, 1, &sppb.RequestOptions{Priority: qo.Priority})
	}
}

func TestClient_WhenLongRunningPartitionedUpdateRequest_TakeNoAction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:                 1,
			MaxOpened:                 1,
			healthCheckSampleInterval: 10 * time.Millisecond, // maintainer runs every 10ms
			InactiveTransactionRemovalOptions: InactiveTransactionRemovalOptions{
				ActionOnInactiveTransaction: WarnAndClose,
				executionFrequency:          15 * time.Millisecond, // check long-running sessions every 15ms
			},
		},
	})
	defer teardown()
	// delay the rpc by 30ms. The background task runs to clean long-running sessions.
	server.TestSpanner.PutExecutionTime(MethodExecuteSql,
		SimulatedExecutionTime{
			MinimumExecutionTime: 30 * time.Millisecond,
		})

	stmt := NewStatement(UpdateBarSetFoo)
	// This transaction is eligible to be long-running, so the background task should not clean its session.
	rowCount, err := client.PartitionedUpdate(ctx, stmt)
	if err != nil {
		t.Fatal(err)
	}
	if g, w := rowCount, int64(UpdateBarSetFooRowCount); g != w {
		t.Errorf("Row count mismatch\nGot: %v\nWant: %v", g, w)
	}
	p := client.idleSessions
	p.mu.Lock()
	defer p.mu.Unlock()
	if g, w := p.numOfLeakedSessionsRemoved, uint64(0); g != w {
		t.Fatalf("Number of leaked sessions removed mismatch\nGot: %d\nWant: %d\n", g, w)
	}
}

func TestClient_Apply_Priority(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	client.Apply(context.Background(), []*Mutation{Insert("foo", []string{"col1"}, []interface{}{"val1"})})
	checkCommitForExpectedRequestOptions(t, server.TestSpanner, &sppb.RequestOptions{})

	client.Apply(context.Background(), []*Mutation{Insert("foo", []string{"col1"}, []interface{}{"val1"})}, Priority(sppb.RequestOptions_PRIORITY_HIGH))
	checkCommitForExpectedRequestOptions(t, server.TestSpanner, &sppb.RequestOptions{Priority: sppb.RequestOptions_PRIORITY_HIGH})

	client.Apply(context.Background(), []*Mutation{Insert("foo", []string{"col1"}, []interface{}{"val1"})}, ApplyAtLeastOnce())
	checkCommitForExpectedRequestOptions(t, server.TestSpanner, &sppb.RequestOptions{})

	client.Apply(context.Background(), []*Mutation{Insert("foo", []string{"col1"}, []interface{}{"val1"})}, ApplyAtLeastOnce(), Priority(sppb.RequestOptions_PRIORITY_MEDIUM))
	checkCommitForExpectedRequestOptions(t, server.TestSpanner, &sppb.RequestOptions{Priority: sppb.RequestOptions_PRIORITY_MEDIUM})
}

func TestClient_ReadOnlyTransaction_Tag(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	for _, qo := range []QueryOptions{
		{},
		{RequestTag: "tag-1"},
	} {
		for _, tx := range []*ReadOnlyTransaction{
			client.Single(),
			client.ReadOnlyTransaction(),
		} {
			iter := tx.QueryWithOptions(context.Background(), NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), qo)
			iter.Next()
			iter.Stop()

			if tx.singleUse {
				tx = client.Single()
			}
			iter = tx.ReadWithOptions(context.Background(), "FOO", AllKeys(), []string{"BAR"}, &ReadOptions{RequestTag: qo.RequestTag})
			iter.Next()
			iter.Stop()

			checkRequestsForExpectedRequestOptions(t, server.TestSpanner, 2, &sppb.RequestOptions{RequestTag: qo.RequestTag})
			tx.Close()
		}
	}
}

func TestClient_ReadWriteTransaction_Tag(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	for _, to := range []TransactionOptions{
		{},
		{TransactionTag: "tx-tag-1"},
	} {
		for _, qo := range []QueryOptions{
			{},
			{RequestTag: "request-tag-1"},
		} {
			client.ReadWriteTransactionWithOptions(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
				iter := tx.QueryWithOptions(context.Background(), NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), qo)
				iter.Next()
				iter.Stop()

				iter = tx.ReadWithOptions(context.Background(), "FOO", AllKeys(), []string{"BAR"}, &ReadOptions{RequestTag: qo.RequestTag})
				iter.Next()
				iter.Stop()

				tx.UpdateWithOptions(context.Background(), NewStatement(UpdateBarSetFoo), qo)
				tx.BatchUpdateWithOptions(context.Background(), []Statement{
					NewStatement(UpdateBarSetFoo),
				}, qo)

				// Check for SQL requests inside the transaction to prevent the check to
				// drain the commit request from the server.
				checkRequestsForExpectedRequestOptions(t, server.TestSpanner, 4, &sppb.RequestOptions{RequestTag: qo.RequestTag, TransactionTag: to.TransactionTag})
				return nil
			}, to)
			checkCommitForExpectedRequestOptions(t, server.TestSpanner, &sppb.RequestOptions{TransactionTag: to.TransactionTag})
		}
	}
}

func TestClient_StmtBasedReadWriteTransaction_Tag(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	for _, to := range []TransactionOptions{
		{},
		{TransactionTag: "tx-tag-1"},
	} {
		for _, qo := range []QueryOptions{
			{},
			{RequestTag: "request-tag-1"},
		} {
			tx, _ := NewReadWriteStmtBasedTransactionWithOptions(context.Background(), client, to)
			iter := tx.QueryWithOptions(context.Background(), NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), qo)
			iter.Next()
			iter.Stop()

			iter = tx.ReadWithOptions(context.Background(), "FOO", AllKeys(), []string{"BAR"}, &ReadOptions{RequestTag: qo.RequestTag})
			iter.Next()
			iter.Stop()

			tx.UpdateWithOptions(context.Background(), NewStatement(UpdateBarSetFoo), qo)
			tx.BatchUpdateWithOptions(context.Background(), []Statement{
				NewStatement(UpdateBarSetFoo),
			}, qo)
			checkRequestsForExpectedRequestOptions(t, server.TestSpanner, 4, &sppb.RequestOptions{RequestTag: qo.RequestTag, TransactionTag: to.TransactionTag})

			tx.Commit(context.Background())
			checkCommitForExpectedRequestOptions(t, server.TestSpanner, &sppb.RequestOptions{TransactionTag: to.TransactionTag})
		}
	}
}

func TestClient_PDML_Tag(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	for _, qo := range []QueryOptions{
		{},
		{RequestTag: "request-tag-1"},
	} {
		client.PartitionedUpdateWithOptions(context.Background(), NewStatement(UpdateBarSetFoo), qo)
		checkRequestsForExpectedRequestOptions(t, server.TestSpanner, 1, &sppb.RequestOptions{RequestTag: qo.RequestTag})
	}
}

func TestClient_Apply_Tagging(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	duration := time.Millisecond
	client.Apply(context.Background(), []*Mutation{Insert("foo", []string{"col1"}, []interface{}{"val1"})}, ApplyCommitOptions(CommitOptions{MaxCommitDelay: &duration}))
	checkCommitForExpectedRequestOptions(t, server.TestSpanner, &sppb.RequestOptions{})
	for _, req := range drainRequestsFromServer(server.TestSpanner) {
		if commitReq, ok := req.(*sppb.CommitRequest); ok {
			if commitReq.MaxCommitDelay.GetNanos() != durationpb.New(duration).GetNanos() {
				t.Fatalf("Missing MaxCommitDelay in commit request")
			}
		}
	}

	client.Apply(context.Background(), []*Mutation{Insert("foo", []string{"col1"}, []interface{}{"val1"})}, TransactionTag("tx-tag"))
	checkCommitForExpectedRequestOptions(t, server.TestSpanner, &sppb.RequestOptions{TransactionTag: "tx-tag"})

	client.Apply(context.Background(), []*Mutation{Insert("foo", []string{"col1"}, []interface{}{"val1"})}, ApplyAtLeastOnce())
	checkCommitForExpectedRequestOptions(t, server.TestSpanner, &sppb.RequestOptions{})

	client.Apply(context.Background(), []*Mutation{Insert("foo", []string{"col1"}, []interface{}{"val1"})}, ApplyAtLeastOnce(), TransactionTag("tx-tag"))
	checkCommitForExpectedRequestOptions(t, server.TestSpanner, &sppb.RequestOptions{TransactionTag: "tx-tag"})

	client.Apply(context.Background(), []*Mutation{Insert("foo", []string{"col1"}, []interface{}{"val1"})}, ApplyAtLeastOnce(), TransactionTag("tx-tag"))
	checkCommitForExpectedRequestOptions(t, server.TestSpanner, &sppb.RequestOptions{TransactionTag: "tx-tag"})
}

func TestClient_PartitionQuery_RequestOptions(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	for _, qo := range []QueryOptions{
		{},
		{Priority: sppb.RequestOptions_PRIORITY_LOW},
		{RequestTag: "batch-query-tag"},
		{Priority: sppb.RequestOptions_PRIORITY_MEDIUM, RequestTag: "batch-query-with-medium-prio"},
	} {
		ctx := context.Background()
		txn, _ := client.BatchReadOnlyTransaction(ctx, StrongRead())
		partitions, _ := txn.PartitionQueryWithOptions(ctx, NewStatement(SelectFooFromBar), PartitionOptions{MaxPartitions: 10}, qo)
		for _, p := range partitions {
			iter := txn.Execute(ctx, p)
			iter.Next()
			iter.Stop()
		}
		checkRequestsForExpectedRequestOptions(t, server.TestSpanner, len(partitions), &sppb.RequestOptions{RequestTag: qo.RequestTag, Priority: qo.Priority})
	}
}

func TestClient_PartitionRead_RequestOptions(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	for _, ro := range []ReadOptions{
		{},
		{Priority: sppb.RequestOptions_PRIORITY_LOW},
		{RequestTag: "batch-read-tag"},
		{Priority: sppb.RequestOptions_PRIORITY_MEDIUM, RequestTag: "batch-read-with-medium-prio"},
	} {
		ctx := context.Background()
		txn, _ := client.BatchReadOnlyTransaction(ctx, StrongRead())
		partitions, _ := txn.PartitionReadWithOptions(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"}, PartitionOptions{MaxPartitions: 10}, ro)
		for _, p := range partitions {
			iter := txn.Execute(ctx, p)
			iter.Next()
			iter.Stop()
		}
		checkRequestsForExpectedRequestOptions(t, server.TestSpanner, len(partitions), &sppb.RequestOptions{RequestTag: ro.RequestTag, Priority: ro.Priority})
	}
}

func checkRequestsForExpectedRequestOptions(t *testing.T, server InMemSpannerServer, reqCount int, ro *sppb.RequestOptions) {
	reqs := drainRequestsFromServer(server)
	reqOptions := []*sppb.RequestOptions{}

	for _, req := range reqs {
		if sqlReq, ok := req.(*sppb.ExecuteSqlRequest); ok {
			reqOptions = append(reqOptions, sqlReq.RequestOptions)
		}
		if batchReq, ok := req.(*sppb.ExecuteBatchDmlRequest); ok {
			reqOptions = append(reqOptions, batchReq.RequestOptions)
		}
		if readReq, ok := req.(*sppb.ReadRequest); ok {
			reqOptions = append(reqOptions, readReq.RequestOptions)
		}
	}

	if got, want := len(reqOptions), reqCount; got != want {
		t.Fatalf("Requests length mismatch\nGot: %v\nWant: %v", got, want)
	}

	for _, opts := range reqOptions {
		if opts == nil {
			opts = &sppb.RequestOptions{}
		}
		if got, want := opts.Priority, ro.Priority; got != want {
			t.Fatalf("Request priority mismatch\nGot: %v\nWant: %v", got, want)
		}
		if got, want := opts.RequestTag, ro.RequestTag; got != want {
			t.Fatalf("Request tag mismatch\nGot: %v\nWant: %v", got, want)
		}
		if got, want := opts.TransactionTag, ro.TransactionTag; got != want {
			t.Fatalf("Transaction tag mismatch\nGot: %v\nWant: %v", got, want)
		}
	}
}

func checkCommitForExpectedRequestOptions(t *testing.T, server InMemSpannerServer, ro *sppb.RequestOptions) {
	reqs := drainRequestsFromServer(server)
	var commit *sppb.CommitRequest
	var ok bool

	for _, req := range reqs {
		if commit, ok = req.(*sppb.CommitRequest); ok {
			break
		}
	}

	if commit == nil {
		t.Fatalf("Missing commit request")
	}

	var got sppb.RequestOptions_Priority
	if commit.RequestOptions != nil {
		got = commit.RequestOptions.Priority
	}
	want := ro.Priority
	if got != want {
		t.Fatalf("Commit priority mismatch\nGot: %v\nWant: %v", got, want)
	}

	var requestTag string
	var transactionTag string
	if commit.RequestOptions != nil {
		requestTag = commit.RequestOptions.RequestTag
		transactionTag = commit.RequestOptions.TransactionTag
	}
	if got, want := requestTag, ro.RequestTag; got != want {
		t.Fatalf("Commit request tag mismatch\nGot: %v\nWant: %v", got, want)
	}
	if got, want := transactionTag, ro.TransactionTag; got != want {
		t.Fatalf("Commit transaction tag mismatch\nGot: %v\nWant: %v", got, want)
	}
}

func TestClient_Single_Read_WithNumericKey(t *testing.T) {
	t.Parallel()

	_, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()
	iter := client.Single().Read(ctx, "Albums", KeySets(Key{*big.NewRat(1, 1)}), []string{"SingerId", "AlbumId", "AlbumTitle"})
	defer iter.Stop()
	rowCount := int64(0)
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		rowCount++
	}
	if rowCount != SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount {
		t.Fatalf("row count mismatch\nGot: %v\nWant: %v", rowCount, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
	}
}

func TestClient_Single_ReadRowWithOptions(t *testing.T) {
	t.Parallel()

	_, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()
	row, err := client.Single().ReadRowWithOptions(ctx, "Albums", Key{"foo"}, []string{"SingerId", "AlbumId", "AlbumTitle"}, &ReadOptions{RequestTag: "foo/bar"})
	if err != nil {
		t.Fatalf("Unexpected error for read row with options: %v", err)
	}
	if row == nil {
		t.Fatal("ReadRowWithOptions did not return a row")
	}
}

func TestClient_CloseWithUnresponsiveBackend(t *testing.T) {
	t.Parallel()

	minOpened := uint64(5)
	server, client, teardown := setupMockedTestServerWithConfig(t,
		ClientConfig{
			DisableNativeMetrics: true,
			SessionPoolConfig: SessionPoolConfig{
				MinOpened: minOpened,
			},
		})
	defer teardown()
	sp := client.idleSessions

	waitFor(t, func() error {
		sp.mu.Lock()
		defer sp.mu.Unlock()
		if uint64(sp.idleList.Len()) != minOpened {
			return fmt.Errorf("num open sessions mismatch\nWant: %d\nGot: %d", sp.MinOpened, sp.numOpened)
		}
		return nil
	})
	server.TestSpanner.Freeze()
	defer server.TestSpanner.Unfreeze()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	sp.close(ctx)

	// session pool close does not trigger any request to backend
	if ctx.Err() != nil {
		t.Fatalf("context error mismatch\nWant: nil\nGot: %v", ctx.Err())
	}
}

func TestClient_CustomRetryAndTimeoutSettings(t *testing.T) {
	co := &vkit.CallOptions{
		ExecuteSql: []gax.CallOption{
			gax.WithRetry(func() gax.Retryer {
				return gax.OnCodes([]codes.Code{
					codes.Unavailable,
				}, gax.Backoff{
					Initial:    500 * time.Millisecond,
					Max:        64000 * time.Millisecond,
					Multiplier: 1.5,
				})
			}),
		},
	}
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, CallOptions: co})
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodExecuteSql,
		SimulatedExecutionTime{MinimumExecutionTime: time.Second},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	_, err := client.ReadWriteTransaction(
		ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
			c, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo))
			if err != nil {
				return err
			}
			if g, w := c, int64(UpdateBarSetFooRowCount); g != w {
				return fmt.Errorf("update count mismatch\n Got: %v\nWant: %v", g, w)
			}
			return nil
		})
	if err == nil {
		t.Fatal("missing expected error")
	}
	se := ToSpannerError(err)
	if g, w := ErrCode(se), codes.DeadlineExceeded; g != w {
		t.Fatalf("error code mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func TestClient_BatchWrite(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	mutationGroups := []*MutationGroup{
		{[]*Mutation{
			{opInsertOrUpdate, "t_test", nil, []string{"key", "val"}, []interface{}{"foo1", 1}, nil},
		}},
	}
	iter := client.BatchWrite(context.Background(), mutationGroups)
	responseCount := 0
	doFunc := func(r *sppb.BatchWriteResponse) error {
		responseCount++
		return nil
	}
	if err := iter.Do(doFunc); err != nil {
		t.Fatal(err)
	}
	if responseCount != len(mutationGroups) {
		t.Fatalf("Response count mismatch.\nGot: %v\nWant:%v", responseCount, len(mutationGroups))
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BatchWriteRequest{},
	}, requests); err != nil {
		t.Fatal(err)
	}
}

func TestClient_BatchWrite_SessionNotFound(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodBatchWrite,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)
	mutationGroups := []*MutationGroup{
		{[]*Mutation{
			{opInsertOrUpdate, "t_test", nil, []string{"key", "val"}, []interface{}{"foo1", 1}, nil},
		}},
	}
	iter := client.BatchWrite(context.Background(), mutationGroups)
	responseCount := 0
	doFunc := func(r *sppb.BatchWriteResponse) error {
		responseCount++
		return nil
	}
	if err := iter.Do(doFunc); err != nil {
		t.Fatal(err)
	}
	if responseCount != len(mutationGroups) {
		t.Fatalf("Response count mismatch.\nGot: %v\nWant:%v", responseCount, len(mutationGroups))
	}

	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BatchWriteRequest{},
		&sppb.BatchWriteRequest{},
	}, requests); err != nil {
		t.Fatal(err)
	}
}

func TestClient_BatchWrite_Error(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	injectedErr := status.Error(codes.InvalidArgument, "Invalid argument")
	server.TestSpanner.PutExecutionTime(
		MethodBatchWrite,
		SimulatedExecutionTime{Errors: []error{injectedErr}},
	)
	mutationGroups := []*MutationGroup{
		{[]*Mutation{
			{opInsertOrUpdate, "t_test", nil, []string{"key", "val"}, []interface{}{"foo1", 1}, nil},
		}},
	}
	iter := client.BatchWrite(context.Background(), mutationGroups)
	responseCount := 0
	doFunc := func(r *sppb.BatchWriteResponse) error {
		responseCount++
		return nil
	}
	if err := iter.Do(doFunc); status.Code(err) != status.Code(injectedErr) {
		t.Fatalf("Error mismatch.\nGot:%v\nExpected:%v\n", err, injectedErr)
	}
	if responseCount != 0 {
		t.Fatalf("Do function unexpectedly called %v times", responseCount)
	}
}

func checkBatchWriteForExpectedRequestOptions(t *testing.T, server InMemSpannerServer, want *sppb.RequestOptions) {
	reqs := drainRequestsFromServer(server)
	var got *sppb.RequestOptions

	for _, req := range reqs {
		if request, ok := req.(*sppb.BatchWriteRequest); ok {
			got = request.RequestOptions
			break
		}
	}

	if got == nil {
		t.Fatalf("Missing BatchWrite RequestOptions")
	}

	if diff := itestutil.Diff(got, want, cmpopts.IgnoreUnexported(sppb.RequestOptions{})); diff != "" {
		t.Fatalf("RequestOptions mismatch. (+Got, -Want):%v", diff)
	}
}

func TestClient_BatchWrite_Options(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name               string
		client             BatchWriteOptions
		write              BatchWriteOptions
		wantTransactionTag string
		wantPriority       sppb.RequestOptions_Priority
	}{
		{
			name:               "Client level",
			client:             BatchWriteOptions{TransactionTag: "testTransactionTag", Priority: sppb.RequestOptions_PRIORITY_LOW},
			wantTransactionTag: "testTransactionTag",
			wantPriority:       sppb.RequestOptions_PRIORITY_LOW,
		},
		{
			name:               "Write level",
			write:              BatchWriteOptions{TransactionTag: "testTransactionTag", Priority: sppb.RequestOptions_PRIORITY_LOW},
			wantTransactionTag: "testTransactionTag",
			wantPriority:       sppb.RequestOptions_PRIORITY_LOW,
		},
		{
			name:               "Write level has precedence over client level",
			client:             BatchWriteOptions{TransactionTag: "clientTransactionTag", Priority: sppb.RequestOptions_PRIORITY_LOW},
			write:              BatchWriteOptions{TransactionTag: "writeTransactionTag", Priority: sppb.RequestOptions_PRIORITY_MEDIUM},
			wantTransactionTag: "writeTransactionTag",
			wantPriority:       sppb.RequestOptions_PRIORITY_MEDIUM,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, BatchWriteOptions: tt.client})
			defer teardown()

			mutationGroups := []*MutationGroup{
				{[]*Mutation{
					{opInsertOrUpdate, "t_test", nil, []string{"key", "val"}, []interface{}{"foo1", 1}, nil},
				}},
			}
			iter := client.BatchWriteWithOptions(context.Background(), mutationGroups, tt.write)
			doFunc := func(r *sppb.BatchWriteResponse) error {
				return nil
			}
			if err := iter.Do(doFunc); err != nil {
				t.Fatal(err)
			}
			checkBatchWriteForExpectedRequestOptions(t, server.TestSpanner, &sppb.RequestOptions{Priority: tt.wantPriority, TransactionTag: tt.wantTransactionTag})
		})
	}
}

func checkBatchWriteSpan(t *testing.T, errors []error, code codes.Code) {
	// This test cannot be parallel, as the TestExporter does not support that.
	te := NewTestExporter()
	defer te.Unregister()
	minOpened := uint64(1)
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     minOpened,
			WriteSessions: 0,
		},
	})
	defer teardown()

	// Wait until all sessions have been created, so we know that those requests will not interfere with the test.
	sp := client.idleSessions
	waitFor(t, func() error {
		sp.mu.Lock()
		defer sp.mu.Unlock()
		if uint64(sp.idleList.Len()) != minOpened {
			return fmt.Errorf("num open sessions mismatch\nWant: %d\nGot: %d", sp.MinOpened, sp.numOpened)
		}
		return nil
	})

	server.TestSpanner.PutExecutionTime(
		MethodBatchWrite,
		SimulatedExecutionTime{Errors: errors},
	)
	mutationGroups := []*MutationGroup{
		{[]*Mutation{
			{opInsertOrUpdate, "t_test", nil, []string{"key", "val"}, []interface{}{"foo1", 1}, nil},
		}},
	}
	iter := client.BatchWrite(context.Background(), mutationGroups)
	iter.Do(func(r *sppb.BatchWriteResponse) error {
		return nil
	})
	select {
	case <-te.Stats:
	case <-time.After(1 * time.Second):
		t.Fatal("No stats were exported before timeout")
	}
	spans := te.Spans()
	if len(spans) == 0 {
		t.Fatal("No spans were exported")
	}
	s := spans[len(spans)-1].Status
	if s.Code != int32(code) {
		t.Errorf("Span status mismatch\nGot: %v\nWant: %v", s.Code, code)
	}
}
func TestClient_BatchWrite_SpanExported(t *testing.T) {
	t.Skip("open census spans are no longer exported by gapics")
	testcases := []struct {
		name   string
		code   codes.Code
		errors []error
	}{
		{
			name:   "Success",
			code:   codes.OK,
			errors: []error{},
		},
		{
			name:   "Error",
			code:   codes.InvalidArgument,
			errors: []error{status.Error(codes.InvalidArgument, "Invalid argument")},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			checkBatchWriteSpan(t, tt.errors, tt.code)
		})
	}
}

func TestClient_ReadWriteTransactionWithTag_AbortedOnce(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{Errors: []error{status.Error(codes.Aborted, "Transaction aborted")}})

	var attempts int
	_, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		return tx.BufferWrite([]*Mutation{Update("my_table", []string{"key", "value"}, []interface{}{int64(1), "my-value"})})
	}, TransactionOptions{TransactionTag: "test-tag1"})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 2; g != w {
		t.Fatalf("attempts mismatch\nGot:  %v\nWant: %v", g, w)
	}

	attempts = 0
	_, err = client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		return tx.BufferWrite([]*Mutation{Update("my_table", []string{"key", "value"}, []interface{}{int64(1), "my-value"})})
	}, TransactionOptions{TransactionTag: "test-tag2"})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 1; g != w {
		t.Fatalf("attempts mismatch\nGot:  %v\nWant: %v", g, w)
	}

	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{},
	}, requests); err != nil {
		t.Fatal(err)
	}
	commit := requests[len(requests)-1].(*sppb.CommitRequest)
	g, w := len(commit.Mutations), 1
	if g != w {
		t.Fatalf("mutations count mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestClient_ReadWriteTransactionWithTag_SessionNotFound(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	ctx := context.Background()
	server.TestSpanner.PutExecutionTime(MethodBeginTransaction,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}})

	var attempts int
	_, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		return tx.BufferWrite([]*Mutation{Update("my_table", []string{"key", "value"}, []interface{}{int64(1), "my-value"})})
	}, TransactionOptions{TransactionTag: "test-tag1"})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 1; g != w {
		t.Fatalf("attempts mismatch\nGot:  %v\nWant: %v", g, w)
	}

	attempts = 0
	_, err = client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		return tx.BufferWrite([]*Mutation{Update("my_table", []string{"key", "value"}, []interface{}{int64(1), "my-value"})})
	}, TransactionOptions{TransactionTag: "test-tag2"})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 1; g != w {
		t.Fatalf("attempts mismatch\nGot:  %v\nWant: %v", g, w)
	}

	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{},
	}, requests); err != nil {
		t.Fatal(err)
	}
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}
	if g, w := requests[3+muxCreateBuffer].(*sppb.CommitRequest).RequestOptions.TransactionTag, "test-tag1"; g != w {
		t.Fatalf("transaction tag mismatch\nGot:  %s\nWant: %s", g, w)
	}
	if g, w := requests[5+muxCreateBuffer].(*sppb.CommitRequest).RequestOptions.TransactionTag, "test-tag2"; g != w {
		t.Fatalf("transaction tag mismatch\nGot:  %s\nWant: %s", g, w)
	}
}

func TestClient_NestedReadWriteTransactionWithTag_AbortedOnce(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{Errors: []error{status.Error(codes.Aborted, "Transaction aborted")}})

	var attempts int
	_, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		if _, err := client.ReadWriteTransactionWithOptions(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
			if _, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo)); err != nil {
				return err
			}
			return nil
		}, TransactionOptions{TransactionTag: "test-tag2"}); err != nil {
			return err
		}
		return tx.BufferWrite([]*Mutation{Update("my_table", []string{"key", "value"}, []interface{}{int64(1), "my-value"})})
	}, TransactionOptions{TransactionTag: "test-tag1"})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 1; g != w {
		t.Fatalf("attempts mismatch\nGot:  %v\nWant: %v", g, w)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{},
	}, requests); err != nil {
		t.Fatal(err)
	}
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}
	if g, w := requests[1+muxCreateBuffer].(*sppb.ExecuteSqlRequest).RequestOptions.TransactionTag, "test-tag2"; g != w {
		t.Fatalf("transaction tag mismatch\nGot:  %s\nWant: %s", g, w)
	}
	if g, w := requests[2+muxCreateBuffer].(*sppb.CommitRequest).RequestOptions.TransactionTag, "test-tag2"; g != w {
		t.Fatalf("transaction tag mismatch\nGot:  %s\nWant: %s", g, w)
	}
	if g, w := requests[3+muxCreateBuffer].(*sppb.ExecuteSqlRequest).RequestOptions.TransactionTag, "test-tag2"; g != w {
		t.Fatalf("transaction tag mismatch\nGot:  %s\nWant: %s", g, w)
	}
	if g, w := requests[4+muxCreateBuffer].(*sppb.CommitRequest).RequestOptions.TransactionTag, "test-tag2"; g != w {
		t.Fatalf("transaction tag mismatch\nGot:  %s\nWant: %s", g, w)
	}
	if g, w := requests[6+muxCreateBuffer].(*sppb.CommitRequest).RequestOptions.TransactionTag, "test-tag1"; g != w {
		t.Fatalf("transaction tag mismatch\nGot:  %s\nWant: %s", g, w)
	}
}

func TestClient_NestedReadWriteTransactionWithTag_OuterAbortedOnce(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	ctx := context.Background()
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{Errors: []error{nil, status.Error(codes.Aborted, "Transaction aborted")}})

	var attempts int
	_, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		if _, err := client.ReadWriteTransactionWithOptions(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
			if _, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo)); err != nil {
				return err
			}
			return nil
		}, TransactionOptions{TransactionTag: "test-tag2"}); err != nil {
			return err
		}
		return tx.BufferWrite([]*Mutation{Update("my_table", []string{"key", "value"}, []interface{}{int64(1), "my-value"})})
	}, TransactionOptions{TransactionTag: "test-tag1"})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 2; g != w {
		t.Fatalf("attempts mismatch\nGot:  %v\nWant: %v", g, w)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{},
	}, requests); err != nil {
		t.Fatal(err)
	}
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}
	if g, w := requests[1+muxCreateBuffer].(*sppb.ExecuteSqlRequest).RequestOptions.TransactionTag, "test-tag2"; g != w {
		t.Fatalf("transaction tag mismatch\nGot:  %s\nWant: %s", g, w)
	}
	if g, w := requests[2+muxCreateBuffer].(*sppb.CommitRequest).RequestOptions.TransactionTag, "test-tag2"; g != w {
		t.Fatalf("transaction tag mismatch\nGot:  %s\nWant: %s", g, w)
	}
	if g, w := requests[4+muxCreateBuffer].(*sppb.CommitRequest).RequestOptions.TransactionTag, "test-tag1"; g != w {
		t.Fatalf("transaction tag mismatch\nGot:  %s\nWant: %s", g, w)
	}
	if g, w := requests[5+muxCreateBuffer].(*sppb.ExecuteSqlRequest).RequestOptions.TransactionTag, "test-tag2"; g != w {
		t.Fatalf("transaction tag mismatch\nGot:  %s\nWant: %s", g, w)
	}
	if g, w := requests[6+muxCreateBuffer].(*sppb.CommitRequest).RequestOptions.TransactionTag, "test-tag2"; g != w {
		t.Fatalf("transaction tag mismatch\nGot:  %s\nWant: %s", g, w)
	}
	if g, w := requests[8+muxCreateBuffer].(*sppb.CommitRequest).RequestOptions.TransactionTag, "test-tag1"; g != w {
		t.Fatalf("transaction tag mismatch\nGot:  %s\nWant: %s", g, w)
	}
}

func TestClient_NestedReadWriteTransactionWithTag_InnerBlindWrite(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{Errors: []error{nil, status.Error(codes.Aborted, "Transaction aborted")}})

	var attempts int
	_, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		if _, err := client.ReadWriteTransactionWithOptions(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
			return tx.BufferWrite([]*Mutation{Update("my_table", []string{"key", "value"}, []interface{}{int64(1), "my-value"})})
		}, TransactionOptions{TransactionTag: "test-tag2"}); err != nil {
			return err
		}
		if _, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo)); err != nil {
			return err
		}
		return nil
	}, TransactionOptions{TransactionTag: "test-tag1"})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 2; g != w {
		t.Fatalf("attempts mismatch\nGot:  %v\nWant: %v", g, w)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{},
	}, requests); err != nil {
		t.Fatal(err)
	}
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}

	if g, w := requests[2+muxCreateBuffer].(*sppb.CommitRequest).RequestOptions.TransactionTag, "test-tag2"; g != w {
		t.Fatalf("transaction tag mismatch\nGot:  %s\nWant: %s", g, w)
	}
	if g, w := requests[3+muxCreateBuffer].(*sppb.ExecuteSqlRequest).RequestOptions.TransactionTag, "test-tag1"; g != w {
		t.Fatalf("transaction tag mismatch\nGot:  %s\nWant: %s", g, w)
	}
	if g, w := requests[4+muxCreateBuffer].(*sppb.CommitRequest).RequestOptions.TransactionTag, "test-tag1"; g != w {
		t.Fatalf("transaction tag mismatch\nGot:  %s\nWant: %s", g, w)
	}
	if g, w := requests[6+muxCreateBuffer].(*sppb.CommitRequest).RequestOptions.TransactionTag, "test-tag2"; g != w {
		t.Fatalf("transaction tag mismatch\nGot:  %s\nWant: %s", g, w)
	}
	if g, w := requests[7+muxCreateBuffer].(*sppb.ExecuteSqlRequest).RequestOptions.TransactionTag, "test-tag1"; g != w {
		t.Fatalf("transaction tag mismatch\nGot:  %s\nWant: %s", g, w)
	}
	if g, w := requests[8+muxCreateBuffer].(*sppb.CommitRequest).RequestOptions.TransactionTag, "test-tag1"; g != w {
		t.Fatalf("transaction tag mismatch\nGot:  %s\nWant: %s", g, w)
	}
}

func TestClient_ReadWriteTransactionWithExcludeTxnFromChangeStreams_ExecuteSqlRequest(t *testing.T) {
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()

	_, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		_, err := tx.Update(ctx, Statement{SQL: UpdateBarSetFoo})
		if err != nil {
			return err
		}
		return nil
	}, TransactionOptions{ExcludeTxnFromChangeStreams: true})
	if err != nil {
		t.Fatalf("Failed to execute the transaction: %s", err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{}}, requests); err != nil {
		t.Fatal(err)
	}
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}
	if !requests[1+muxCreateBuffer].(*sppb.ExecuteSqlRequest).Transaction.GetBegin().ExcludeTxnFromChangeStreams {
		t.Fatal("Transaction is not set to be excluded from change streams")
	}
}

func TestClient_ReadWriteTransactionWithExcludeTxnFromChangeStreams_BufferWrite(t *testing.T) {
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()

	_, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		if err := tx.BufferWrite([]*Mutation{
			Insert("foo", []string{"col1"}, []interface{}{"key1"}),
		}); err != nil {
			return err
		}
		return nil
	}, TransactionOptions{ExcludeTxnFromChangeStreams: true})
	if err != nil {
		t.Fatalf("Failed to execute the transaction: %s", err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{}}, requests); err != nil {
		t.Fatal(err)
	}
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}
	if !requests[1+muxCreateBuffer].(*sppb.BeginTransactionRequest).Options.ExcludeTxnFromChangeStreams {
		t.Fatal("Transaction is not set to be excluded from change streams")
	}
}

func TestClient_ReadWriteTransactionWithExcludeTxnFromChangeStreams_BatchUpdate(t *testing.T) {
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()

	_, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		_, err := tx.BatchUpdate(ctx, []Statement{NewStatement(UpdateBarSetFoo)})
		if err != nil {
			return err
		}
		return nil
	}, TransactionOptions{ExcludeTxnFromChangeStreams: true})
	if err != nil {
		t.Fatalf("Failed to execute the transaction: %s", err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteBatchDmlRequest{},
		&sppb.CommitRequest{}}, requests); err != nil {
		t.Fatal(err)
	}
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}
	if !requests[1+muxCreateBuffer].(*sppb.ExecuteBatchDmlRequest).Transaction.GetBegin().ExcludeTxnFromChangeStreams {
		t.Fatal("Transaction is not set to be excluded from change streams")
	}
}

func TestClient_RequestLevelDMLWithExcludeTxnFromChangeStreams_Failed(t *testing.T) {
	_, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()

	// Test normal DML
	_, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		_, err := tx.UpdateWithOptions(ctx, Statement{SQL: UpdateBarSetFoo}, QueryOptions{ExcludeTxnFromChangeStreams: true})
		if err != nil {
			return err
		}
		return nil
	}, TransactionOptions{ExcludeTxnFromChangeStreams: true})
	if err == nil {
		t.Fatalf("Missing expected exception")
	}
	msg := "cannot set exclude transaction from change streams for a request-level DML statement."
	if status.Code(err) != codes.InvalidArgument || !strings.Contains(err.Error(), msg) {
		t.Fatalf("error mismatch\nGot: %v\nWant: %v", err, msg)
	}

	// Test batch DML
	_, err = client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		_, err := tx.UpdateWithOptions(ctx, Statement{SQL: UpdateBarSetFoo}, QueryOptions{ExcludeTxnFromChangeStreams: true})
		if err != nil {
			return err
		}
		return nil
	}, TransactionOptions{ExcludeTxnFromChangeStreams: true})
	if err == nil {
		t.Fatalf("Missing expected exception")
	}
	if status.Code(err) != codes.InvalidArgument || !strings.Contains(err.Error(), msg) {
		t.Fatalf("error mismatch\nGot: %v\nWant: %v", err, msg)
	}
}

func TestClient_ApplyExcludeTxnFromChangeStreams(t *testing.T) {
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}

	_, err := client.Apply(context.Background(), ms, ExcludeTxnFromChangeStreams())
	if err != nil {
		t.Fatal(err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{}}, requests); err != nil {
		t.Fatal(err)
	}
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}
	if !requests[1+muxCreateBuffer].(*sppb.BeginTransactionRequest).Options.ExcludeTxnFromChangeStreams {
		t.Fatal("Transaction is not set to be excluded from change streams")
	}
}

func TestClient_ApplyAtLeastOnceExcludeTxnFromChangeStreams(t *testing.T) {
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}

	_, err := client.Apply(context.Background(), ms, []ApplyOption{ExcludeTxnFromChangeStreams(), ApplyAtLeastOnce()}...)
	if err != nil {
		t.Fatal(err)
	}

	expectedReqs := []interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.CommitRequest{},
	}
	if isMultiplexEnabled {
		expectedReqs = []interface{}{
			&sppb.CreateSessionRequest{},
			&sppb.CommitRequest{},
		}
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests(expectedReqs, requests); err != nil {
		t.Fatal(err)
	}
	for _, req := range requests {
		if request, ok := req.(*sppb.CommitRequest); ok {
			if !request.Transaction.(*sppb.CommitRequest_SingleUseTransaction).SingleUseTransaction.ExcludeTxnFromChangeStreams {
				t.Fatal("Transaction is not set to be excluded from change streams")
			}
			if !testEqual(isMultiplexEnabled, strings.Contains(request.GetSession(), "multiplexed")) {
				t.Errorf("TestClient_ApplyAtLeastOnceExcludeTxnFromChangeStreams expected multiplexed session to be used, got: %v", request.GetSession())
			}
		}
	}
}

func TestClient_BatchWriteExcludeTxnFromChangeStreams(t *testing.T) {
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	mutationGroups := []*MutationGroup{
		{[]*Mutation{
			{opInsertOrUpdate, "t_test", nil, []string{"key", "val"}, []interface{}{"foo1", 1}, nil},
		}},
	}
	iter := client.BatchWriteWithOptions(context.Background(), mutationGroups, BatchWriteOptions{ExcludeTxnFromChangeStreams: true})
	responseCount := 0
	doFunc := func(r *sppb.BatchWriteResponse) error {
		responseCount++
		return nil
	}
	if err := iter.Do(doFunc); err != nil {
		t.Fatal(err)
	}
	if responseCount != len(mutationGroups) {
		t.Fatalf("Response count mismatch.\nGot: %v\nWant:%v", responseCount, len(mutationGroups))
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BatchWriteRequest{}}, requests); err != nil {
		t.Fatal(err)
	}
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}
	if !requests[1+muxCreateBuffer].(*sppb.BatchWriteRequest).ExcludeTxnFromChangeStreams {
		t.Fatal("Transaction is not set to be excluded from change streams")
	}
}

func TestParseServerTimingHeader(t *testing.T) {
	tests := []struct {
		name     string
		header   metadata.MD
		expected map[string]time.Duration
	}{
		{
			name:     "empty metadata",
			header:   metadata.New(map[string]string{}),
			expected: map[string]time.Duration{},
		},
		{
			name:     "no server-timing header",
			header:   metadata.New(map[string]string{"other-header": "value"}),
			expected: map[string]time.Duration{},
		},
		{
			name:     "integer duration",
			header:   metadata.New(map[string]string{"server-timing": "gfet4t7; dur=123"}),
			expected: map[string]time.Duration{"gfet4t7": 123 * time.Millisecond},
		},
		{
			name:     "float duration",
			header:   metadata.New(map[string]string{"server-timing": "gfet4t7; dur=123.45"}),
			expected: map[string]time.Duration{"gfet4t7": 123*time.Millisecond + 450*time.Microsecond},
		},
		{
			name:   "multiple metrics",
			header: metadata.New(map[string]string{"server-timing": "gfet4t7; dur=123, afe; dur=456.789"}),
			expected: map[string]time.Duration{
				"gfet4t7": 123 * time.Millisecond,
				"afe":     456*time.Millisecond + 789*time.Microsecond,
			},
		},
		{
			name:     "invalid duration format",
			header:   metadata.New(map[string]string{"server-timing": "gfet4t7; dur=invalid"}),
			expected: map[string]time.Duration{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseServerTimingHeader(tt.header)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("parseServerTimingHeader() = %v, want %v", got, tt.expected)
			}
		})
	}
}
