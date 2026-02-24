package utils

import (
	"fmt"
	"net"
)

// privateRanges holds all IP networks that must never be contacted via
// user-supplied URLs (SSRF prevention).
var privateRanges_ []*net.IPNet

func init() {
	for _, cidr := range []string{
		"127.0.0.0/8",    // loopback
		"10.0.0.0/8",     // RFC 1918
		"172.16.0.0/12",  // RFC 1918
		"192.168.0.0/16", // RFC 1918
		"169.254.0.0/16", // link-local / AWS metadata
		"100.64.0.0/10",  // CGNAT
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 ULA
		"fe80::/10",      // IPv6 link-local
	} {
		if _, network, err := net.ParseCIDR(cidr); err == nil {
			privateRanges_ = append(privateRanges_, network)
		}
	}
}

// PrivateRanges returns the list of blocked IP networks (used by the SSRF-safe dialer).
func PrivateRanges() []*net.IPNet { return privateRanges_ }

// IsBlockedIP reports whether host resolves to a private or reserved address.
// Returns true on DNS failure to fail safe.
func IsBlockedIP(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return true
	}
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

// ValidateRemoteURL returns an error if host resolves to a private/internal address.
func ValidateRemoteURL(host string) error {
	if IsBlockedIP(host) {
		return fmt.Errorf("access to internal or reserved addresses is not allowed")
	}
	return nil
}
