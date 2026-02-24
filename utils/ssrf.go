package utils

import (
	"fmt"
	"net"
)

// privateRanges contains all IP ranges that must never be contacted via
// user-supplied URLs (SSRF prevention).
var privateRanges_ []*net.IPNet

func init() {
	blocked := []string{
		"127.0.0.0/8",    // loopback
		"10.0.0.0/8",     // RFC 1918
		"172.16.0.0/12",  // RFC 1918
		"192.168.0.0/16", // RFC 1918
		"169.254.0.0/16", // link-local / AWS metadata
		"100.64.0.0/10",  // CGNAT
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 ULA
		"fe80::/10",      // IPv6 link-local
	}
	for _, cidr := range blocked {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil {
			privateRanges_ = append(privateRanges_, network)
		}
	}
}

// PrivateRanges returns the list of blocked IP networks (used by the dialer).
func PrivateRanges() []*net.IPNet {
	return privateRanges_
}

// IsBlockedIP returns true when the given hostname/IP resolves to a private or
// reserved address. Always returns true on DNS resolution failure to fail-safe.
func IsBlockedIP(host string) bool {
	// Strip brackets around IPv6 literals
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		// Fail safe: block unresolvable hosts
		return true
	}

	// Every resolved IP must be public — block if any is private
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return true
		}
		for _, network := range privateRanges_ {
			if network.Contains(ip) {
				return true
			}
		}
	}
	return false
}

// ValidateRemoteURL checks that the URL host is not a private/internal IP.
// Returns an error if the URL should be blocked.
func ValidateRemoteURL(host string) error {
	if IsBlockedIP(host) {
		return fmt.Errorf("access to internal or reserved addresses is not allowed")
	}
	return nil
}
