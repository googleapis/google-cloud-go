package internal

import (
	"fmt"
	"log"
	"net"
)

func skipLoopback(iface net.Interface) error {
	if iface.Flags&net.FlagLoopback != 0 {
		return fmt.Errorf("interface has loopback flag")
	}
	if iface.Flags&net.FlagUp != net.FlagUp {
		return fmt.Errorf("interface is not marked up")
	}
	return nil
}

func onlyLoopback(iface net.Interface) error {
	if iface.Flags&net.FlagLoopback == 0 {
		return fmt.Errorf("interface does not have loopback flag")
	}
	if iface.Flags&net.FlagUp != net.FlagUp {
		return fmt.Errorf("interface is not marked up")
	}
	return nil
}

func findLocalAddress(ipMatches func(net.IP) bool, ifaceFilter func(iface net.Interface) error) (*net.Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var match *net.Interface
	foundMatch := false
	for _, iface := range ifaces {
		currentIface := iface // Capture range variable
		if err := ifaceFilter(currentIface); err != nil {
			continue
		}
		ifaddrs, err := currentIface.Addrs()
		if err != nil {
			continue
		}
		for _, ifaddr := range ifaddrs {
			ip := ifaddr.(*net.IPNet).IP
			if ipMatches(ip) {
				if foundMatch {
					log.Printf("Warning: Found multiple interfaces with matching IP. Using first one: %s", match.Name)
				} else {
					match = &currentIface
					foundMatch = true
				}
			}
		}
	}
	if !foundMatch {
		return nil, fmt.Errorf("failed to find matching address on any interface")
	}
	log.Printf("Found matching IP on interface %s", match.Name)
	return match, nil
}

// CheckLocalIPv6Addresses verifies that the IPv6 address assigned to the VM by the metadata server
// is actually configured on a local network interface (excluding loopback).
// This is crucial for DirectPath/IPv6, as the VM needs to have the correct Google-internal
// IPv6 address properly bound to an interface to route traffic over the DirectPath network.
func CheckLocalIPv6Addresses(ipv6FromMetadataServer *net.IP) (*net.Interface, error) {
	if ipv6FromMetadataServer == nil {
		return nil, fmt.Errorf("no IPv6 address from metadata server to check")
	}
	log.Println("Checking for local IPv6 address interface...")
	return findLocalAddress(func(ip net.IP) bool { return ip.To4() == nil && ip.Equal(*ipv6FromMetadataServer) }, skipLoopback)
}

// CheckLocalIPv6LoopbackAddress checks for the presence of the IPv6 loopback address (::1)
// on a local network interface. This can affect gRPC's ability to use IPv6.
func CheckLocalIPv6LoopbackAddress() error {
	log.Println("Checking for local IPv6 loopback address (::1)...")
	ipv6Loopback := net.ParseIP("::1")
	_, err := findLocalAddress(func(ip net.IP) bool { return ip.Equal(ipv6Loopback) }, onlyLoopback)
	return err
}

// CheckLocalIPv4Addresses verifies that the IPv4 address assigned to the VM by the metadata server
// is actually configured on a local network interface (excluding loopback).
// Important for dual-stack or IPv4 DirectPath scenarios.
func CheckLocalIPv4Addresses(ipv4FromMetadataServer *net.IP) (*net.Interface, error) {
	if ipv4FromMetadataServer == nil {
		return nil, fmt.Errorf("no IPv4 address from metadata server to check")
	}
	log.Println("Checking for local IPv4 address interface...")
	return findLocalAddress(func(ip net.IP) bool { return ip.To4() != nil && ip.Equal(*ipv4FromMetadataServer) }, skipLoopback)
}

// CheckLocalIPv4LoopbackAddress checks for the presence of the IPv4 loopback address (127.0.0.1).
func CheckLocalIPv4LoopbackAddress() error {
	log.Println("Checking for local IPv4 loopback address (127.0.0.1)...")
	ipv4Loopback := net.ParseIP("127.0.0.1")
	_, err := findLocalAddress(func(ip net.IP) bool { return ip.Equal(ipv4Loopback) }, onlyLoopback)
	return err
}
