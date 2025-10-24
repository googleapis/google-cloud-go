package internal

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
)

const (
	MetadataIPURL   = "http://metadata.google.internal/computeMetadata/v1/instance/network-interfaces/0/ip"
	MetadataIPv6URL = "http://metadata.google.internal/computeMetadata/v1/instance/network-interfaces/0/ipv6s"
)

// FetchIPFromMetadataServer fetches the IP address (IPv4 or IPv6) from the metadata server.
// This confirms the VM has been *assigned* the necessary IPs by GCP.
func FetchIPFromMetadataServer(addrFamilyStr string) (*net.IP, error) {
	var metadataServerURL string
	switch addrFamilyStr {
	case "IPv4":
		metadataServerURL = MetadataIPURL
	case "IPv6":
		metadataServerURL = MetadataIPv6URL
	default:
		return nil, fmt.Errorf("invalid address family %v is not IPv4 or IPv6", addrFamilyStr)
	}

	log.Printf("Fetching %s address from metadata server: %s", addrFamilyStr, metadataServerURL)
	req, err := http.NewRequest("GET", metadataServerURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Metadata-Flavor", "Google")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 200 {
		address := net.ParseIP(strings.TrimSuffix(string(body), "\n"))
		if address == nil {
			return nil, fmt.Errorf("failed to parse IP address from metadata server response: %s", string(body))
		}
		log.Printf("Received %s address %s from metadata server", addrFamilyStr, address)
		return &address, nil
	}
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("this VM doesn't have a %s address allocated to its primary network interface", addrFamilyStr)
	}
	return nil, fmt.Errorf("received status code %d in response to metadata server GET request to URL: %s", resp.StatusCode, metadataServerURL)
}
