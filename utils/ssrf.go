package utils

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)


// privateRanges holds all IP networks that must never be contacted via
// user-supplied URLs (SSRF prevention).
var privateRanges []*net.IPNet

func init() {
	for _, cidr := range []string{
		"127.0.0.0/8",    // loopback
		"10.0.0.0/8",     // RFC 1918
		"172.16.0.0/12",  // RFC 1918
		"192.168.0.0/16", // RFC 1918
		"169.254.0.0/16", // link-local / AWS metadata
		"100.64.0.0/10",  // CGNAT
		"0.0.0.0/8",      // "this" network
		"192.0.0.0/24",   // IETF protocol assignments
		"198.18.0.0/15",  // benchmarking
		"224.0.0.0/4",    // IPv4 multicast
		"240.0.0.0/4",    // reserved / future use
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 ULA
		"fe80::/10",      // IPv6 link-local
		"ff00::/8",       // IPv6 multicast
		"::/128",         // unspecified
		"2001:db8::/32",  // documentation
	} {
		if _, network, err := net.ParseCIDR(cidr); err == nil {
			privateRanges = append(privateRanges, network)
		}
	}
}

// PrivateRanges returns the list of blocked IP networks (used by the SSRF-safe dialer in upload.go).
func PrivateRanges() []*net.IPNet { return privateRanges }

// IsBlockedIP reports whether ip is in a range that must not be contacted
// via user-supplied URLs (loopback, RFC1918, link-local, metadata, etc.).
func IsBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() || ip.IsPrivate() {
		return true
	}
	for _, cidr := range privateRanges {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// localAccessCIDRs are networks that count as "local" for the local access level.
// Subset of privateRanges: RFC1918, loopback, link-local, CGNAT, IPv6 ULA/link-local.
var localAccessCIDRs []*net.IPNet

func init() {
	for _, cidr := range []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"100.64.0.0/10",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	} {
		if _, network, err := net.ParseCIDR(cidr); err == nil {
			localAccessCIDRs = append(localAccessCIDRs, network)
		}
	}
}

// IsPrivateOrLocalIP reports whether ip belongs to a private, loopback, or
// link-local network. Used for the "local" access level on public links.
func IsPrivateOrLocalIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate() {
		return true
	}
	for _, cidr := range localAccessCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// ValidateRemoteURL parses urlStr and ensures the host resolves only to
// public (non-blocked) addresses. Call this BEFORE issuing the request so
// HTTP proxies cannot be abused to reach internal networks (the dialer only
// sees the proxy address, not the target).
func ValidateRemoteURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() {
		return fmt.Errorf("invalid URL")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported URL scheme")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("missing host")
	}
	// Block obvious local hostnames without waiting for DNS.
	switch strings.ToLower(host) {
	case "localhost", "localhost.localdomain", "metadata", "metadata.google.internal":
		return fmt.Errorf("address is not allowed")
	}
	// If host is already an IP literal, check it directly.
	if ip := net.ParseIP(host); ip != nil {
		if IsBlockedIP(ip) {
			return fmt.Errorf("address is not allowed")
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("DNS resolution failed for %s", host)
	}
	// Require at least one public IP; reject if ANY resolved IP is blocked
	// to prevent DNS round-robin rebinding tricks that mix public+private.
	public := 0
	for _, ip := range ips {
		if IsBlockedIP(ip) {
			return fmt.Errorf("address is not allowed")
		}
		public++
	}
	if public == 0 {
		return fmt.Errorf("address is not allowed")
	}
	return nil
}
