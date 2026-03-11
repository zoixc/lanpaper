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
	cidrs := []string{
		// RFC 1918 — Private IPv4
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		// RFC 5735 — Special-use IPv4
		"127.0.0.0/8",   // loopback
		"169.254.0.0/16", // link-local
		"224.0.0.0/4",   // multicast
		"255.255.255.255/32", // broadcast
		"0.0.0.0/8",     // "this" network
		"100.64.0.0/10", // CGNAT
		"192.0.2.0/24",  // TEST-NET-1
		"198.51.100.0/24", // TEST-NET-2
		"203.0.113.0/24",  // TEST-NET-3
		"198.18.0.0/15",   // benchmarking
		// RFC 4193 / 4291 — IPv6
		"fc00::/7",   // unique local
		"::1/128",    // loopback
		"fe80::/10",  // link-local
		"ff00::/8",   // multicast
	}
	privateRanges = make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, network, _ := net.ParseCIDR(cidr)
		privateRanges = append(privateRanges, network)
	}
}

// PrivateRanges returns all private and non-routable IP ranges for SSRF protection.
func PrivateRanges() []*net.IPNet {
	return privateRanges
}

// IsPrivateOrLocalIP reports whether ip is private, loopback, or link-local.
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

// blockedHostnames is the set of hostnames always blocked regardless of DNS resolution.
var blockedHostnames = map[string]bool{
	"localhost":                true,
	"metadata.google.internal": true, // GCP metadata
	"169.254.169.254":          true, // AWS/Azure metadata IP
}

// ValidateURLHost checks if a hostname resolves to a safe (non-private) IP.
// This prevents DNS rebinding attacks and SSRF via private IPs.
func ValidateURLHost(host string) error {
	// Strip port if present.
	hostOnly := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostOnly = h
	}

	// Direct IP check.
	if ip := net.ParseIP(hostOnly); ip != nil {
		if IsPrivateOrLocalIP(ip) {
			return fmt.Errorf("private or local IP addresses are not allowed")
		}
		return nil
	}

	// Blocked hostname check (O(1) map lookup).
	if blockedHostnames[strings.ToLower(hostOnly)] {
		return fmt.Errorf("hostname %q is not allowed", hostOnly)
	}

	// Resolve and verify all IPs to prevent DNS rebinding.
	ips, err := net.LookupIP(hostOnly)
	if err != nil {
		return fmt.Errorf("failed to resolve hostname: %w", err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("hostname did not resolve to any IP addresses")
	}
	for _, ip := range ips {
		if IsPrivateOrLocalIP(ip) {
			return fmt.Errorf("hostname resolves to private or local IP: %s", ip)
		}
	}
	return nil
}
