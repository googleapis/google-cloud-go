package directaccess

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const metadataBaseURL = "http://metadata.google.internal/computeMetadata/v1/"
const metadataIPURL = "http://metadata.google.internal/computeMetadata/v1/instance/network-interfaces/0/ip"
const metadataIPv6URL = "http://metadata.google.internal/computeMetadata/v1/instance/network-interfaces/0/ipv6s"

// CheckMetadataServerReachability performs a basic connectivity check to the GCE metadata server.
func CheckMetadataServerReachability() error {
	req, err := http.NewRequest("GET", metadataBaseURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create metadata request: %w", err)
	}
	req.Header.Add("Metadata-Flavor", "Google")

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to GCE Metadata Server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("reachable but returned status code: %d", resp.StatusCode)
	}
	return nil
}

// FetchIPFromMetadataServer fetches the assigned IP address from the metadata server.
func FetchIPFromMetadataServer(addrFamilyStr string) (*net.IP, error) {
	var metadataServerURL string
	switch addrFamilyStr {
	case "IPv4":
		metadataServerURL = metadataIPURL
	case "IPv6":
		metadataServerURL = metadataIPv6URL
	default:
		return nil, fmt.Errorf("invalid address family %v", addrFamilyStr)
	}

	req, err := http.NewRequest("GET", metadataServerURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Metadata-Flavor", "Google")

	client := &http.Client{Timeout: 5 * time.Second}
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
			return nil, fmt.Errorf("failed to parse IP: %s", string(body))
		}
		return &address, nil
	}
	return nil, fmt.Errorf("received status code %d", resp.StatusCode)
}
