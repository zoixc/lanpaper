package utils

import (
	"fmt"
	"net"
	"strings"
)

// privateRanges contains all private, loopback, link-local, and other
// non-routable IP ranges that should be blocked for SSRF protection.
var privateRanges []*net.IPNet

func init() {
	// RFC 1918 - Private IPv4 ranges
	_, ipv4Private10, _ := net.ParseCIDR("10.0.0.0/8")
	_, ipv4Private172, _ := net.ParseCIDR("172.16.0.0/12")
	_, ipv4Private192, _ := net.ParseCIDR("192.168.0.0/16")

	// RFC 5735 - Special Use IPv4 Addresses
	_, ipv4Loopback, _ := net.ParseCIDR("127.0.0.0/8")
	_, ipv4LinkLocal, _ := net.ParseCIDR("169.254.0.0/16")
	_, ipv4Multicast, _ := net.ParseCIDR("224.0.0.0/4")
	_, ipv4Broadcast, _ := net.ParseCIDR("255.255.255.255/32")
	_, ipv4ZeroConf, _ := net.ParseCIDR("0.0.0.0/8")
	_, ipv4CGNAT, _ := net.ParseCIDR("100.64.0.0/10")
	_, ipv4TestNet1, _ := net.ParseCIDR("192.0.2.0/24")
	_, ipv4TestNet2, _ := net.ParseCIDR("198.51.100.0/24")
	_, ipv4TestNet3, _ := net.ParseCIDR("203.0.113.0/24")
	_, ipv4Benchmark, _ := net.ParseCIDR("198.18.0.0/15")

	// RFC 4193 - Unique Local IPv6 Unicast Addresses
	_, ipv6Private, _ := net.ParseCIDR("fc00::/7")

	// RFC 4291 - IPv6 Special Addresses
	_, ipv6Loopback, _ := net.ParseCIDR("::1/128")
	_, ipv6LinkLocal, _ := net.ParseCIDR("fe80::/10")
	_, ipv6Multicast, _ := net.ParseCIDR("ff00::/8")

	privateRanges = []*net.IPNet{
		ipv4Private10,
		ipv4Private172,
		ipv4Private192,
		ipv4Loopback,
		ipv4LinkLocal,
		ipv4Multicast,
		ipv4Broadcast,
		ipv4ZeroConf,
		ipv4CGNAT,
		ipv4TestNet1,
		ipv4TestNet2,
		ipv4TestNet3,
		ipv4Benchmark,
		ipv6Private,
		ipv6Loopback,
		ipv6LinkLocal,
		ipv6Multicast,
	}
}

// PrivateRanges returns all private and non-routable IP ranges for SSRF protection.
func PrivateRanges() []*net.IPNet {
	return privateRanges
}

// IsPrivateOrLocalIP checks if an IP address is private, loopback, or link-local.
// This prevents SSRF attacks by blocking requests to internal infrastructure.
func IsPrivateOrLocalIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	
	for _, cidr := range privateRanges {
		if cidr.Contains(ip) {
			return true
		}
	}
	
	return false
}

// ValidateURLHost checks if a hostname resolves to a safe (non-private) IP.
// This prevents DNS rebinding attacks and SSRF via private IPs.
func ValidateURLHost(host string) error {
	// Strip port if present
	hostOnly := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostOnly = h
	}
	
	// Check if host is directly an IP
	if ip := net.ParseIP(hostOnly); ip != nil {
		if IsPrivateOrLocalIP(ip) {
			return fmt.Errorf("private or local IP addresses are not allowed")
		}
		return nil
	}
	
	// Blocked hostnames (cloud metadata services, etc.)
	blockedHosts := []string{
		"localhost",
		"metadata.google.internal", // GCP metadata
		"169.254.169.254",           // AWS/Azure metadata IP
	}
	for _, blocked := range blockedHosts {
		if strings.EqualFold(hostOnly, blocked) {
			return fmt.Errorf("hostname %q is not allowed", hostOnly)
		}
	}
	
	// Resolve hostname and check all IPs
	ips, err := net.LookupIP(hostOnly)
	if err != nil {
		return fmt.Errorf("failed to resolve hostname: %w", err)
	}
	
	if len(ips) == 0 {
		return fmt.Errorf("hostname did not resolve to any IP addresses")
	}
	
	// Check that ALL resolved IPs are safe (prevents DNS rebinding)
	for _, ip := range ips {
		if IsPrivateOrLocalIP(ip) {
			return fmt.Errorf("hostname resolves to private or local IP: %s", ip.String())
		}
	}
	
	return nil
}
