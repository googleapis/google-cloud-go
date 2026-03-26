package directaccess

import (
	"fmt"
	"net"
)

// CheckLoopbackInterfaceUp verifies that at least one loopback interface is UP.
func CheckLoopbackInterfaceUp() error {
	ifaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("failed to list network interfaces: %w", err)
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 && iface.Flags&net.FlagUp != 0 {
			return nil
		}
	}
	return fmt.Errorf("no loopback interface found in UP state")
}

func skipLoopback(iface net.Interface) error {
	if iface.Flags&net.FlagLoopback != 0 {
		return fmt.Errorf("is loopback")
	}
	if iface.Flags&net.FlagUp != net.FlagUp {
		return fmt.Errorf("not up")
	}
	return nil
}

func onlyLoopback(iface net.Interface) error {
	if iface.Flags&net.FlagLoopback == 0 {
		return fmt.Errorf("not loopback")
	}
	if iface.Flags&net.FlagUp != net.FlagUp {
		return fmt.Errorf("not up")
	}
	return nil
}

func CheckLocalIPv6Addresses(ip *net.IP) (*net.Interface, error) {
	return findLocalAddress(func(i net.IP) bool { return i.To4() == nil && i.Equal(*ip) }, skipLoopback)
}

func CheckLocalIPv4Addresses(ip *net.IP) (*net.Interface, error) {
	return findLocalAddress(func(i net.IP) bool { return i.To4() != nil && i.Equal(*ip) }, skipLoopback)
}

func CheckLocalIPv6LoopbackAddress() error {
	_, err := findLocalAddress(func(i net.IP) bool { return i.Equal(net.ParseIP("::1")) }, onlyLoopback)
	return err
}

func CheckLocalIPv4LoopbackAddress() error {
	_, err := findLocalAddress(func(i net.IP) bool { return i.Equal(net.ParseIP("127.0.0.1")) }, onlyLoopback)
	return err
}

func findLocalAddress(ipMatches func(net.IP) bool, ifaceFilter func(net.Interface) error) (*net.Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range ifaces {
		if err := ifaceFilter(iface); err != nil {
			continue
		}
		ifaddrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, ifaddr := range ifaddrs {
			if ip, ok := ifaddr.(*net.IPNet); ok && ipMatches(ip.IP) {
				return &iface, nil
			}
		}
	}
	return nil, fmt.Errorf("address not found on any interface")
}
