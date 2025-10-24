package internal

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func cmd(command string) (string, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}
	head := parts[0]
	parts = parts[1:]
	out, err := exec.Command(head, parts...).Output()
	return string(out), err
}

// CheckLocalIPv6Routes checks if the kernel can route traffic to the given IPv6 backend address
// using the VM's DirectPath IPv6 address.
func CheckLocalIPv6Routes(localAddress *net.IP, backendAddress string) error {
	if localAddress == nil {
		return fmt.Errorf("skipping IPv6 route check, no local IPv6 address available")
	}
	if backendAddress == "" {
		return fmt.Errorf("skipping IPv6 route check, no backend address provided")
	}
	destIPStr, destPortStr, err := net.SplitHostPort(backendAddress)
	if err != nil {
		return fmt.Errorf("failed to split backend address: %v into host and port components: %w", backendAddress, err)
	}
	destIP := net.ParseIP(destIPStr)
	if destIP == nil || destIP.To4() != nil {
		return fmt.Errorf("backend address %s is not a valid IPv6 address", backendAddress)
	}

	destPort, err := strconv.Atoi(destPortStr)
	if err != nil {
		return fmt.Errorf("failed to parse port from backend address: %v: %w", backendAddress, err)
	}

	sourceStr := net.JoinHostPort(localAddress.String(), "0")
	log.Printf("Checking kernel routability for IPv6: %s -> %s", sourceStr, backendAddress)
	fd, err := syscall.Socket(syscall.AF_INET6, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP)
	if err != nil {
		return fmt.Errorf("error creating IPv6/UDP socket: %w", err)
	}
	defer syscall.Close(fd)

	source := &syscall.SockaddrInet6{Port: 0}
	copy(source.Addr[:], (*localAddress))
	if err := syscall.Bind(fd, source); err != nil {
		return fmt.Errorf("error binding UDP/IPV6 socket to %s: %w", sourceStr, err)
	}

	dest := &syscall.SockaddrInet6{Port: destPort}
	copy(dest.Addr[:], destIP)
	if err := syscall.Connect(fd, dest); err != nil {
		return fmt.Errorf("failed to connect UDP socket (source: %s) to dest: %s, err: %w. This indicates the DirectPath/IPv6 backends aren't routable", sourceStr, backendAddress, err)
	}
	return nil
}

// CheckLocalIPv4Routes checks if the kernel can route traffic to the given IPv4 backend address
// using the VM's primary IPv4 address.
func CheckLocalIPv4Routes(localAddress *net.IP, backendAddress string) error {
	if localAddress == nil {
		return fmt.Errorf("skipping IPv4 route check, no local IPv4 address available")
	}
	if backendAddress == "" {
		return fmt.Errorf("skipping IPv4 route check, no backend address provided")
	}
	destIPStr, destPortStr, err := net.SplitHostPort(backendAddress)
	if err != nil {
		return fmt.Errorf("failed to split backend address: %v into host and port components: %w", backendAddress, err)
	}
	destIP := net.ParseIP(destIPStr)
	if destIP == nil || destIP.To4() == nil {
		return fmt.Errorf("backend address %s is not a valid IPv4 address", backendAddress)
	}
	destPort, err := strconv.Atoi(destPortStr)
	if err != nil {
		return fmt.Errorf("failed to parse port from backend address: %v: %w", backendAddress, err)
	}

	sourceStr := net.JoinHostPort(localAddress.String(), "0")
	log.Printf("Checking kernel routability for IPv4: %s -> %s", sourceStr, backendAddress)
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP)
	if err != nil {
		return fmt.Errorf("error creating IPv4/UDP socket: %w", err)
	}
	defer syscall.Close(fd)

	source := &syscall.SockaddrInet4{Port: 0}
	copy(source.Addr[:], (*localAddress).To4())
	if err := syscall.Bind(fd, source); err != nil {
		return fmt.Errorf("error binding UDP/IPV4 socket to %s: %w", sourceStr, err)
	}

	dest := &syscall.SockaddrInet4{Port: destPort}
	copy(dest.Addr[:], destIP.To4())
	if err := syscall.Connect(fd, dest); err != nil {
		return fmt.Errorf("failed to connect UDP socket (source: %s) to dest: %s, err: %w. This indicates the DirectPath/IPv4 backends aren't routable", sourceStr, backendAddress, err)
	}
	return nil
}

// LogLocalRoutes logs the routing table for the given interface.
func LogLocalRoutes(iface net.Interface, addrLen int) {
	var cmdStr string
	if addrLen == net.IPv6len {
		cmdStr = fmt.Sprintf("ip -6 route show dev %s", iface.Name)
	} else {
		cmdStr = fmt.Sprintf("ip route show dev %s", iface.Name)
	}
	log.Printf("Logging routes for interface %s using command: %s", iface.Name, cmdStr)
	out, err := cmd(cmdStr)
	if err != nil {
		log.Printf("Failed to get routes for interface %s: %v", iface.Name, err)
		return
	}
	log.Printf("Routes for %s:\n%s", iface.Name, out)
}
