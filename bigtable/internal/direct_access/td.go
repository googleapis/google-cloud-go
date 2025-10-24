package internal

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"time"

	v3clusterpb "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	v3corepb "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	v3endpointpb "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	v3listenerpb "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	v3clusterextpb "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/aggregate/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func openAdsStream(ctx context.Context, cfg *Config) (ADSStream, error) {
	var roots *x509.CertPool
	tlsCreds := credentials.NewTLS(&tls.Config{RootCAs: roots})
	// tlsCreds.OverrideServerName(cfg.TrafficDirectorHostname) // Not needed for oauth

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(tlsCreds),
		grpc.WithPerRPCCredentials(oauth.NewComputeEngine()),
	}
	if cfg.UserAgent != "" {
		opts = append(opts, grpc.WithUserAgent(cfg.UserAgent))
	}

	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	lbAddr := net.JoinHostPort(cfg.TrafficDirectorHostname, TrafficDirectorPort)
	InfoLog.Printf("Attempt to dial |%v| using TLS and VM's default service account", lbAddr)
	conn, err := grpc.DialContext(dialCtx, lbAddr, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create grpc connection to Traffic Director: %v", err)
	}
	lbClient := v3adsgrpc.NewAggregatedDiscoveryServiceClient(conn)
	stream, err := lbClient.StreamAggregatedResources(ctx)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open the stream to Traffic Director: %v", err)
	}
	return stream, nil
}

func sendXdsRequest(stream ADSStream, node *v3corepb.Node, typeURL, resourceName string, versionInfoMap, nonceMap map[string]string) (*v3discoverypb.DiscoveryResponse, error) {
	typeNameMap := map[string]string{
		V3ListenerURL:    "LDS",
		V3RouteConfigURL: "RDS",
		V3ClusterURL:     "CDS",
		V3EndpointsURL:   "EDS",
	}
	requestName := typeNameMap[typeURL]
	xdsReq := &v3discoverypb.DiscoveryRequest{
		VersionInfo:   versionInfoMap[typeURL],
		Node:          node,
		ResourceNames: []string{resourceName},
		TypeUrl:       typeURL,
		ResponseNonce: nonceMap[typeURL],
	}
	if err := stream.Send(xdsReq); err != nil {
		return nil, fmt.Errorf("failed to send %v request: %v", requestName, err)
	}
	xdsReply, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("failed to receive %v response: %v", requestName, err)
	}
	mm := protojson.MarshalOptions{Multiline: true, Indent: JSONIndent}
	if encodedReply, err := mm.Marshal(xdsReply); err == nil {
		InfoLog.Printf("Received %s response: %s", requestName, string(encodedReply))
	}

	versionInfoMap[typeURL] = xdsReply.GetVersionInfo()
	nonceMap[typeURL] = xdsReply.GetNonce()
	if err = ackXdsResponse(stream, node, typeURL, resourceName, versionInfoMap, nonceMap); err != nil {
		return nil, fmt.Errorf("failed to ack %v response: %v", requestName, err)
	}
	return xdsReply, nil
}

func ackXdsResponse(stream ADSStream, node *v3corepb.Node, typeURL, resourceName string, versionInfoMap, nonceMap map[string]string) error {
	ackReq := &v3discoverypb.DiscoveryRequest{
		VersionInfo:   versionInfoMap[typeURL],
		Node:          node,
		ResourceNames: []string{resourceName},
		TypeUrl:       typeURL,
		ResponseNonce: nonceMap[typeURL],
	}
	if err := stream.Send(ackReq); err != nil {
		return fmt.Errorf("failed to ack xDS response: %v", err)
	}
	return nil
}

func processLdsResponse(ldsReply *v3discoverypb.DiscoveryResponse, requestedResource string, service string) (string, error) {
	if len(ldsReply.GetResources()) == 0 {
		return "", fmt.Errorf("no listener resource received in LDS response")
	}
	resource := ldsReply.GetResources()[0]
	lis := &v3listenerpb.Listener{}
	if err := proto.Unmarshal(resource.GetValue(), lis); err != nil {
		return "", fmt.Errorf("failed to unmarshal listener resource from LDS response: %v", err)
	}
	if lis.GetName() != requestedResource {
		return "", fmt.Errorf("listener resource name |%v| does not match |%v|", lis.GetName(), service)
	}
	apiLis := &v3httppb.HttpConnectionManager{}
	if err := proto.Unmarshal(lis.GetApiListener().GetApiListener().GetValue(), apiLis); err != nil {
		return "", fmt.Errorf("failed to unmarshal api_listener resource from LDS response: %v", err)
	}
	switch apiLis.RouteSpecifier.(type) {
	case *v3httppb.HttpConnectionManager_RouteConfig:
		for _, vh := range apiLis.GetRouteConfig().GetVirtualHosts() {
			if len(vh.GetDomains()) == 0 || (vh.GetDomains()[0] != "*" && vh.GetDomains()[0] != service) {
				continue
			}
			if len(vh.GetRoutes()) == 0 {
				continue
			}
			route := vh.GetRoutes()[len(vh.GetRoutes())-1]
			if match := route.GetMatch(); match == nil || match.GetPrefix() != "" {
				continue
			}
			return route.GetRoute().GetCluster(), nil
		}
	}
	return "", fmt.Errorf("no matching cluster name found in LDS response for service %s", service)
}

func getCluster(cdsReply *v3discoverypb.DiscoveryResponse, expectedClusterName string) (*v3clusterpb.Cluster, error) {
	if len(cdsReply.GetResources()) == 0 {
		return nil, fmt.Errorf("no cluster resource received in CDS response")
	}
	resource := cdsReply.GetResources()[0]
	cluster := &v3clusterpb.Cluster{}
	if err := proto.Unmarshal(resource.GetValue(), cluster); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cluster resource from CDS response: %v", err)
	}
	if cluster.GetName() != expectedClusterName {
		return nil, fmt.Errorf("cluster resource name |%v| does not match |%v|", cluster.GetName(), expectedClusterName)
	}
	return cluster, nil
}

func processAggregateClusterResponse(cdsReply *v3discoverypb.DiscoveryResponse, clusterName string) ([]string, error) {
	aggregateCluster, err := getCluster(cdsReply, clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to get aggregate cluster from CDS response: %v", err)
	}
	if aggregateCluster.GetClusterType() == nil || aggregateCluster.GetClusterType().GetName() != "envoy.clusters.aggregate" {
		return nil, fmt.Errorf("failed to receive an aggregate cluster from Traffic Director")
	}
	clusterConfig := &v3clusterextpb.ClusterConfig{}
	if err := proto.Unmarshal(aggregateCluster.GetClusterType().GetTypedConfig().GetValue(), clusterConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cluster_config resource from CDS response: %v", err)
	}
	if len(clusterConfig.GetClusters()) < 1 {
		return nil, fmt.Errorf("expected to receive at least 1 cluster in aggregate cluster, but received %v", len(clusterConfig.GetClusters()))
	}
	return clusterConfig.GetClusters(), nil
}

func processEdsClusterResponse(cdsReply *v3discoverypb.DiscoveryResponse, clusterName string) (string, error) {
	edsCluster, err := getCluster(cdsReply, clusterName)
	if err != nil {
		return "", fmt.Errorf("failed to get EDS cluster from CDS response: %v", err)
	}
	if edsCluster.GetType() != v3clusterpb.Cluster_EDS {
		return "", fmt.Errorf("the cluster type is expected to be EDS, but it is: %v", edsCluster.GetType())
	}
	if serviceName := edsCluster.GetEdsClusterConfig().GetServiceName(); serviceName != "" {
		return serviceName, nil
	}
	return clusterName, nil
}

func processDNSClusterResponse(cdsReply *v3discoverypb.DiscoveryResponse, clusterName string, service string) error {
	dnsCluster, err := getCluster(cdsReply, clusterName)
	if err != nil {
		return fmt.Errorf("failed to get DNS cluster from CDS response: %v", err)
	}
	if dnsCluster.GetType() != v3clusterpb.Cluster_LOGICAL_DNS {
		return fmt.Errorf("the cluster type is expected to be LOGICAL_DNS, but it is: %v", dnsCluster.GetType())
	}
	if len(dnsCluster.GetLoadAssignment().GetEndpoints()) != 1 {
		return fmt.Errorf("the DNS cluster must have exactly 1 locality, but it has %v", len(dnsCluster.GetLoadAssignment().GetEndpoints()))
	}
	locality := dnsCluster.GetLoadAssignment().GetEndpoints()[0]
	if len(locality.GetLbEndpoints()) != 1 {
		return fmt.Errorf("the DNS cluster must exactly has 1 endpoint, but it has %v", len(locality.GetLbEndpoints()))
	}
	socketAddress := locality.GetLbEndpoints()[0].GetEndpoint().GetAddress().GetSocketAddress()
	if socketAddress.GetAddress() != service {
		return fmt.Errorf("the address field must be service name |%v|, but it is |%v|", service, socketAddress.GetAddress())
	}
	if socketAddress.GetPortValue() != 443 {
		return fmt.Errorf("the port_value field must be CFE port 443, but it is: %v", socketAddress.GetPortValue())
	}
	return nil
}

func processEdsResponse(edsReply *v3discoverypb.DiscoveryResponse, enableDualstackEndpoints bool) ([]XdsEndpoint, error) {
	if len(edsReply.GetResources()) != 1 {
		return nil, fmt.Errorf("expect to receive only 1 cluster_load_assignment resource in EDS response, but received %v", len(edsReply.GetResources()))
	}
	resource := edsReply.GetResources()[0]
	clusterLoadAssignment := &v3endpointpb.ClusterLoadAssignment{}
	if err := proto.Unmarshal(resource.GetValue(), clusterLoadAssignment); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cluster_load_assigement resource from EDS response: %v", err)
	}
	var results []XdsEndpoint
	for _, endpoint := range clusterLoadAssignment.GetEndpoints() {
		if endpoint.GetPriority() == 0 {
			for _, lbendpoint := range endpoint.GetLbEndpoints() {
				if lbendpoint.GetHealthStatus() == v3corepb.HealthStatus_HEALTHY {
					primaryAddr := lbendpoint.GetEndpoint().GetAddress().GetSocketAddress()
					var secondaryAddrStr string
					if enableDualstackEndpoints && len(lbendpoint.GetEndpoint().GetAdditionalAddresses()) == 1 {
						secondaryAddr := lbendpoint.GetEndpoint().GetAdditionalAddresses()[0].GetAddress().GetSocketAddress()
						secondaryAddrStr = net.JoinHostPort(secondaryAddr.GetAddress(), fmt.Sprint(secondaryAddr.GetPortValue()))
					}
					results = append(results, XdsEndpoint{
						PrimaryAddr:   net.JoinHostPort(primaryAddr.GetAddress(), fmt.Sprint(primaryAddr.GetPortValue())),
						SecondaryAddr: secondaryAddrStr,
						HealthStatus:  lbendpoint.GetHealthStatus().String(),
					})
				}
			}
		}
	}
	if len(results) == 0 {
		return nil, errors.New("no healthy primary endpoints received in EDS response")
	}
	return results, nil
}

func checkLDS(stream ADSStream, node *v3corepb.Node, versionInfoMap, nonceMap map[string]string, service string) (string, error) {
	resourceName := fmt.Sprintf("xdstp://traffic-director-c2p.xds.googleapis.com/envoy.config.listener.v3.Listener/%s", service)
	ldsReply, err := sendXdsRequest(stream, node, V3ListenerURL, resourceName, versionInfoMap, nonceMap)
	if err != nil {
		return "", fmt.Errorf("fail to send LDS request: %v", err)
	}
	clusterName, err := processLdsResponse(ldsReply, resourceName, service)
	if err != nil {
		return "", fmt.Errorf("fail to process LDS response: %v", err)
	}
	InfoLog.Printf("Successfully extract cluster_name from LDS response: |%+v|", clusterName)
	return clusterName, nil
}

func checkCDS(stream ADSStream, node *v3corepb.Node, clusterName string, versionInfoMap, nonceMap map[string]string, cfg *Config) (string, error) {
	cdsReply, err := sendXdsRequest(stream, node, V3ClusterURL, clusterName, versionInfoMap, nonceMap)
	if err != nil {
		return "", fmt.Errorf("fail to send CDS request: %v", err)
	}
	var edsClusterName string
	var edsClusterReply *v3discoverypb.DiscoveryResponse
	if cfg.XDSExpectFallbackConfigured {
		clusters, err := processAggregateClusterResponse(cdsReply, clusterName)
		if err != nil {
			return "", fmt.Errorf("fail to process aggregate cluster response: %v", err)
		}
		edsClusterName = clusters[0]
		edsClusterReply, err = sendXdsRequest(stream, node, V3ClusterURL, edsClusterName, versionInfoMap, nonceMap)
		if err != nil {
			return "", fmt.Errorf("fail to send EDS cluster request: %v", err)
		}
		dnsClusterReply, err := sendXdsRequest(stream, node, V3ClusterURL, clusters[1], versionInfoMap, nonceMap)
		if err != nil {
			return "", fmt.Errorf("fail to send DNS cluster request: %v", err)
		}
		if err = processDNSClusterResponse(dnsClusterReply, clusters[1], cfg.Service); err != nil {
			return "", fmt.Errorf("fail to process DNS cluster response: %v", err)
		}
	} else {
		edsClusterName = clusterName
		edsClusterReply = cdsReply
	}
	serviceName, err := processEdsClusterResponse(edsClusterReply, edsClusterName)
	if err != nil {
		return "", fmt.Errorf("fail to process EDS cluster response: %v", err)
	}
	InfoLog.Printf("Successfully extract service_name from CDS response: |%v|", serviceName)
	return serviceName, nil
}

func checkEDS(stream ADSStream, node *v3corepb.Node, serviceName string, versionInfoMap, nonceMap map[string]string, cfg *Config) ([]XdsEndpoint, error) {
	edsReply, err := sendXdsRequest(stream, node, V3EndpointsURL, serviceName, versionInfoMap, nonceMap)
	if err != nil {
		return nil, fmt.Errorf("fail to send EDS request: %v", err)
	}
	return processEdsResponse(edsReply, cfg.EnableDualstackEndpoints)
}

// FetchBackendAddrsFromTrafficDirector fetches backend addresses from Traffic Director
// based on the service configuration and address preference.
func FetchBackendAddrsFromTrafficDirector(ctx context.Context, cfg *Config, preference addressPreference) ([]XdsEndpoint, error) {
	stream, err := openAdsStream(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to open stream to Traffic Director: %v", err)
	}
	defer stream.CloseSend()

	zone, err := GetZone(10 * time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to get zone from metadata server: %v", err)
	}

	ipv6Capable := false
	if cfg.IPv6CapableNodeMetadataOverride == "true" {
		ipv6Capable = true
	} else if cfg.IPv6CapableNodeMetadataOverride == "" {
		if preference == IPv6 || preference == Dualstack {
			ipv6Capable = true
		}
	}

	node := NewNode(zone, ipv6Capable)
	versionInfoMap := make(map[string]string)
	nonceMap := make(map[string]string)

	clusterName, err := checkLDS(stream, node, versionInfoMap, nonceMap, cfg.Service)
	if err != nil {
		return nil, fmt.Errorf("LDS failed: %v", err)
	}
	serviceName, err := checkCDS(stream, node, clusterName, versionInfoMap, nonceMap, cfg)
	if err != nil {
		return nil, fmt.Errorf("CDS failed: %v", err)
	}
	xdsBackendAddrs, err := checkEDS(stream, node, serviceName, versionInfoMap, nonceMap, cfg)
	if err != nil {
		return nil, fmt.Errorf("EDS failed: %v", err)
	}

	var filteredAddrs []XdsEndpoint
	for _, backend := range xdsBackendAddrs {
		valid := false
		pIP, err := ParseAddress(backend.PrimaryAddr)
		if err != nil {
			continue
		}

		switch preference {
		case IPv4:
			if pIP.To4() != nil && backend.SecondaryAddr == "" {
				valid = true
			}
		case IPv6:
			if pIP.To4() == nil && backend.SecondaryAddr == "" {
				valid = true
			}
		case Dualstack:
			if pIP.To4() == nil { // Primary must be IPv6
				if backend.SecondaryAddr == "" {
					valid = true // IPv6 only is OK for Dualstack preference
				} else {
					sIP, err := ParseAddress(backend.SecondaryAddr)
					if err == nil && sIP.To4() != nil { // Secondary must be IPv4
						valid = true
					}
				}
			}
		}
		if valid {
			filteredAddrs = append(filteredAddrs, backend)
		}
	}

	if len(filteredAddrs) == 0 {
		return nil, fmt.Errorf("no suitable %s backends found from Traffic Director", preference)
	}
	return filteredAddrs, nil
}
