/*
Copyright 2025 Google LLC

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

package bigtable

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	internal "cloud.google.com/go/bigtable/internal/transport"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/google"
	"google.golang.org/grpc/peer"
)

const (
	directPathIPV6Prefix = "[2001:4860:8040"
	directPathIPV4Prefix = "34.126"
)

// CheckDirectAccessSupported attempts to establish a connection to the Bigtable instance
// using Direct Access by enforcing internal gRPC options. It then checks if the underlying
// gRPC connection is indeed using a DirectPath IP address.
//
// Prerequisites for successful Direct Access connectivity:
//  1. The code must be running in a Google Cloud environment (e.g., GCE VM, GKE)
//     that is properly configured for Direct Access. This includes ensuring
//     that your routes and firewall rules allow egress traffic to the
//     Direct Access IP ranges: 34.126.0.0/18 and 2001:4860:8040::/42.
//  2. The service account must have the necessary IAM permissions.
//
// Parameters:
//   - ctx: The context for the operation.
//   - project: The Google Cloud project ID.
//   - instance: The Cloud Bigtable instance ID.
//   - appProfile: The application profile ID to use for the connection. Defaults to "default" if empty.
//   - opts: Additional option.ClientOption to configure the Bigtable client. These are
//           appended to the options used to force DirectPath.
//
// Returns:
//   - bool: True if DirectPath is successfully used for the connection, False otherwise.
//   - error: An error if the check could not be completed. Specific error causes include:
//            - Failure to create the Bigtable client (e.g., invalid project/instance).
//            - Failure during the PingAndWarm call (e.g., network issue, permissions).
//

// CheckDirectAccessSupported verifies if Direct Access connectivity is enabled, configured,
// and actively being used for the given Cloud Bigtable instance.
func CheckDirectAccessSupported(ctx context.Context, project, instance, appProfile string, opts ...option.ClientOption) (bool, error) {
	isDirectPathUsed := false
	// Define the unary client interceptor
	interceptor := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, callOpts ...grpc.CallOption) error {
		// Create a new context with a peer to be captured by FromContext
		peerInfo := &peer.Peer{}
		allCallOpts := append(callOpts, grpc.Peer(peerInfo))

		// Invoke the original RPC call
		err := invoker(ctx, method, req, reply, cc, allCallOpts...)
		if err != nil {
			return err
		}

		// After the call, store the captured peer address
		if peerInfo.Addr != nil {
			remoteIP := peerInfo.Addr.String()
			if strings.HasPrefix(remoteIP, directPathIPV4Prefix) || strings.HasPrefix(remoteIP, directPathIPV6Prefix) {
				isDirectPathUsed = true
			}
		}

		return nil
	}

	// register the interceptor
	allOpts := append([]option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptor)),
	}, opts...)

	// Force DirectPath and ALTS using internal options
	allOpts = append(allOpts,
		internaloption.EnableDirectPath(true),
		internaloption.EnableDirectPathXds(),
		internaloption.AllowHardBoundTokens("ALTS"),
		internaloption.AllowNonDefaultServiceAccount(true),
	)

	config := ClientConfig{
		AppProfile:      appProfile,
		MetricsProvider: NoopMetricsProvider{},
	}

	client, err := NewClientWithConfig(ctx, project, instance, config, allOpts...)
	if err != nil {
		return false, fmt.Errorf("CheckDirectConnectivitySupported: failed to create Bigtable client for checking DirectAccess %w", err)
	}
	defer client.Close()

	// Call the  PingAndWarm method
	err = client.PingAndWarm(ctx)
	if err != nil {
		return false, fmt.Errorf("CheckDirectConnectivitySupported: PingAndWarm failed: %w", err)
	}

	return isDirectPathUsed, nil
}

// CallSingleChannel connects to Bigtable using the DirectPath C2P scheme,
// and continuously maintains 200 in-flight Prime requests on the same channel
// until the provided context is canceled.
func CallSingleChannel(ctx context.Context, project, instance, appProfile, target string, requestsInFlight int) error {
	fullInstanceName := fmt.Sprintf("projects/%s/instances/%s", project, instance)

	log.Printf("Creating single channel to %s", target)

	// 1. Create the gRPC Client
	conn, err := grpc.NewClient(target,
		grpc.WithCredentialsBundle(google.NewDefaultCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to create client for C2P target %s: %w", target, err)
	}
	defer conn.Close()

	// 2. Wrap it in the internal BigtableConn
	btc := internal.NewBigtableConn(conn)

	// 3. Maintain 200 in-flight requests continuously
	log.Printf("Starting %d workers to maintain continuous in-flight Prime() requests...", requestsInFlight)

	var wg sync.WaitGroup

	for i := 0; i < requestsInFlight; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			// Infinite loop to keep firing requests
			for {
				// Exit the loop if the parent context is canceled
				select {
				case <-ctx.Done():
					return
				default:
				}

				// Execute a single request with its own 10s timeout
				primeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				ffMd := createFeatureFlagsMD(true, false, true)
				err := btc.Prime(primeCtx, fullInstanceName, appProfile, ffMd)
				cancel() // Always cancel to prevent context leaks

				if err != nil {
					log.Printf("[Worker %d] Prime() failed: %v", workerID, err)
					// Tiny backoff to prevent CPU thrashing if the connection fully drops
				}
			}
		}(i)
	}

	// 4. Block until the context is canceled and all workers exit
	wg.Wait()

	log.Println("Stopped maintaining requests. Exiting CallSingleChannel.")
	return ctx.Err()
}
