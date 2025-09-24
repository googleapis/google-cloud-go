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
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
)

const (
	directPathIPV6Prefix = "[2001:4860:8040"
	directPathIPV4Prefix = "34.126"
)

// CheckDirectConnectivitySupported verifies if DirectPath connectivity is enabled, configured,
// and actively being used for the given Cloud Bigtable instance.
//
// This function attempts to establish a connection to the Bigtable instance using
// settings that force the use of DirectPath. It then checks if the underlying
// gRPC connection is indeed using a DirectPath IP address.
//
// Prerequisites for successful DirectPath connectivity:
// 1. The environment variable `CBT_ENABLE_DIRECTPATH` must be set to "true".
// 2. The code must be running in a Google Cloud environment (e.g., GCE VM, GKE)
//    that is properly configured for DirectPath. This includes:
//    - Subnet enabled for Private Google Access with the "Direct Google Access" option.
//    - The VM/Pod has an IPv6 address.
//    - Appropriate network routes and firewall rules are in place.
//    - See the Cloud Bigtable Direct Google Access User Guide for detailed setup (go/cbt-directpath-alpha-user-guide).
// 3. The necessary IAM permissions must be granted to the service account.
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
//   - error: An error if the check could not be completed, or if DirectPath is not
//            enabled/configured. Specific error causes include:
//            - "CBT_ENABLE_DIRECTPATH=true is not set in env var": The required environment variable is missing.
//            - Failure to create the Bigtable client (e.g., invalid project/instance).
//            - Failure during the PingAndWarm call (e.g., network issue, permissions).
//
// Example Usage:
//
//   ctx := context.Background()
//   projectID := "my-project"
//   instanceID := "my-instance"
//   appProfileID := "default"
//
//   // Set the environment variable if not already set
//   os.Setenv("CBT_ENABLE_DIRECTPATH", "true")
//
//   isDirectPath, err := bigtable.CheckDirectConnectivitySupported(ctx, projectID, instanceID, appProfileID)
//   if err != nil {
//     log.Fatalf("DirectPath check failed: %v", err)
//   }
//
//   if isDirectPath {
//     log.Printf("DirectPath connectivity is active for %s/%s", projectID, instanceID)
//   } else {
//     log.Printf("DirectPath connectivity is NOT active for %s/%s", projectID, instanceID)
//   }
//

func CheckDirectConnectivitySupported(ctx context.Context, project, instance, appProfile string, opts ...option.ClientOption) (bool, error) {
	// Check if env variable is set to true
	// Inside the function
	envVal := os.Getenv("CBT_ENABLE_DIRECTPATH")
	if envVal == "" {
		return false, errors.New("CBT_ENABLE_DIRECTPATH environment variable is not set")
	}
	isEnvEnabled, err := strconv.ParseBool(envVal)
	if err != nil {
		return false, fmt.Errorf("invalid value for CBT_ENABLE_DIRECTPATH: %s, must be true or false: %w", envVal, err)
	}
	if !isEnvEnabled {
		return false, errors.New("CBT_ENABLE_DIRECTPATH is not set to true")
	}
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
