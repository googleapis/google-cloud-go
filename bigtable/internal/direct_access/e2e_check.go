package internal

import (
	"context"
	"errors"
	"net"
	"time"

	"google.golang.org/api/option"
)

// VerifyDirectPathAndBigtable runs prerequisite checks and then Bigtable connectivity.
func VerifyDirectPathAndBigtable(ctx context.Context, cfg *dpchecker.Config, opts ...option.ClientOption) (*dpchecker.Results, bool, error) {
	results := &dpchecker.Results{}
	var err error

	dpchecker.InfoLog.Println("Starting DirectPath environment checks...")

	// 1. GCP Environment
	if err = runSingleCheck("Running on GCP", dpchecker.IsRunningOnGCP); err != nil {
		return results, false, err
	}

	// 2. Metadata Server IP Fetch
	if !cfg.IPv4Only {
		results.IPv6FromMetadataServer, results.IPv6AllocatedCheck = dpchecker.FetchIPFromMetadataServer("IPv6")
		if err = runSingleCheck("IPv6 Address Allocated", func() error { return results.IPv6AllocatedCheck }); err != nil {
			return results, false, err
		}
	}
	if !cfg.IPv6Only {
		results.IPv4FromMetadataServer, results.IPv4AllocatedCheck = dpchecker.FetchIPFromMetadataServer("IPv4")
		if err = runSingleCheck("IPv4 Address Allocated", func() error { return results.IPv4AllocatedCheck }); err != nil {
			// Non-fatal for DP, continue
		}
	}

	// 3. Local Address Configuration
	if !cfg.IPv4Only {
		err = runSingleCheck("Local IPv6 Address Config", func() error {
			results.DirectPathIPv6Interface, results.LocalIPv6Check = dpchecker.CheckLocalIPv6Addresses(results.IPv6FromMetadataServer)
			return results.LocalIPv6Check
		})
		if err != nil {
			return results, false, err
		}

		err = runSingleCheck("Local IPv6 Loopback", dpchecker.CheckLocalIPv6LoopbackAddress)
		if err != nil {
			return results, false, err
		}
	}
	if !cfg.IPv6Only {
		err = runSingleCheck("Local IPv4 Address Config", func() error {
			results.DirectPathIPv4Interface, results.LocalIPv4Check = dpchecker.CheckLocalIPv4Addresses(results.IPv4FromMetadataServer)
			return results.LocalIPv4Check
		}) // Non-fatal

		err = runSingleCheck("Local IPv4 Loopback", dpchecker.CheckLocalIPv4LoopbackAddress)
		// Non-fatal
	}

	// 4. XDS Environment
	err = runSingleCheck("XDS Bootstrap Env", dpchecker.CheckXDSBootstrapEnv)
	if err != nil {
		return results, false, err
	}

	// 5. Traffic Director Backend Discovery
	var backends []dpchecker.XdsEndpoint
	if cfg.CheckXDS && cfg.BackendAddressOverride == "" {
		preference := dpchecker.Dualstack
		if cfg.IPv4Only {
			preference = dpchecker.IPv4
		} else if cfg.IPv6Only {
			preference = dpchecker.IPv6
		}
		backends, results.TDBackendDiscoveryCheck = dpchecker.FetchBackendAddrsFromTrafficDirector(ctx, cfg, preference)
		if err = runSingleCheck("TD Backend Discovery", func() error { return results.TDBackendDiscoveryCheck }); err != nil {
			return results, false, err
		}
		for _, be := range backends {
			if ip, _ := dpchecker.ParseAddress(be.PrimaryAddr); ip != nil && ip.To4() == nil {
				results.XDSIPv6BackendAddrs = append(results.XDSIPv6BackendAddrs, be.PrimaryAddr)
			} else if ip != nil {
				results.XDSIPv4BackendAddrs = append(results.XDSIPv4BackendAddrs, be.PrimaryAddr)
			}
			if be.SecondaryAddr != "" {
				results.XDSIPv4BackendAddrs = append(results.XDSIPv4BackendAddrs, be.SecondaryAddr)
			}
		}
	} else if cfg.BackendAddressOverride != "" {
		// Simplified handling for override
		ip, _ := dpchecker.ParseAddress(cfg.BackendAddressOverride)
		if ip.To4() == nil {
			results.XDSIPv6BackendAddrs = []string{cfg.BackendAddressOverride}
		} else {
			results.XDSIPv4BackendAddrs = []string{cfg.BackendAddressOverride}
		}
		dpchecker.InfoLog.Println("TD Backend Discovery: [SKIPPED: --backend_address_override set]")
	}

	// 6. Route Checks
	if !cfg.IPv4Only && len(results.XDSIPv6BackendAddrs) > 0 {
		if results.DirectPathIPv6Interface != nil {
			dpchecker.LogLocalRoutes(*results.DirectPathIPv6Interface, net.IPv6len)
		}
		err = runSingleCheck("Local IPv6 Routes", func() error {
			results.LocalIPv6RouteCheck = dpchecker.CheckLocalIPv6Routes(results.IPv6FromMetadataServer, results.XDSIPv6BackendAddrs[0])
			return results.LocalIPv6RouteCheck
		})
		if err != nil {
			return results, false, err
		}
	}
	if !cfg.IPv6Only && len(results.XDSIPv4BackendAddrs) > 0 {
		if results.DirectPathIPv4Interface != nil {
			dpchecker.LogLocalRoutes(*results.DirectPathIPv4Interface, net.IPv4len)
		}
		err = runSingleCheck("Local IPv4 Routes", func() error {
			results.LocalIPv4RouteCheck = dpchecker.CheckLocalIPv4Routes(results.IPv4FromMetadataServer, results.XDSIPv4BackendAddrs[0])
			return results.LocalIPv4RouteCheck
		}) // Non-fatal
	}

	// 7. TCP Connectivity
	if !cfg.IPv4Only && len(results.XDSIPv6BackendAddrs) > 0 {
		err = runSingleCheck("TCP to IPv6 Backend", func() error {
			results.TCPv6BackendCheck = dpchecker.CheckTCPConnectivity(results.XDSIPv6BackendAddrs[0], 5*time.Second)
			return results.TCPv6BackendCheck
		})
		if err != nil {
			return results, false, err
		}
	}
	if !cfg.IPv6Only && len(results.XDSIPv4BackendAddrs) > 0 {
		err = runSingleCheck("TCP to IPv4 Backend", func() error {
			results.TCPv4BackendCheck = dpchecker.CheckTCPConnectivity(results.XDSIPv4BackendAddrs[0], 5*time.Second)
			return results.TCPv4BackendCheck
		}) // Non-fatal
	}

	dpchecker.InfoLog.Println("DirectPath environment checks completed.")

	// 8. Bigtable Specific Check
	dpchecker.InfoLog.Println("Proceeding to Bigtable DirectAccess check...")
	isDirectPathUsed, err := CheckDirectAccessSupported(ctx, cfg.ProjectID, cfg.InstanceID, cfg.AppProfile, opts...)
	results.BigtableDirectAccessCheck = err
	if err != nil {
		dpchecker.InfoLog.Printf("Bigtable DirectAccess Check Failed: %v", err)
		return results, false, err
	}

	if !isDirectPathUsed {
		dpchecker.InfoLog.Println("Bigtable connection NOT using DirectPath.")
		return results, false, errors.New("bigtable connection not using DirectPath")
	}

	dpchecker.InfoLog.Println("Bigtable DirectAccess check passed and confirmed DirectPath usage.")
	return results, true, nil
}
