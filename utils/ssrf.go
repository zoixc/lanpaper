package utils

import (
	"context"
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
		"127.0.0.0/8",     // loopback
		"10.0.0.0/8",      // RFC 1918
		"172.16.0.0/12",   // RFC 1918
		"192.168.0.0/16",  // RFC 1918
		"169.254.0.0/16",  // link-local / AWS metadata
		"100.64.0.0/10",   // CGNAT
		"0.0.0.0/8",       // "this" network
		"192.0.0.0/24",    // IETF protocol assignments
		"198.18.0.0/15",   // benchmarking
		"224.0.0.0/4",     // IPv4 multicast
		"240.0.0.0/4",     // reserved / future use
		"::1/128",         // IPv6 loopback
		"fc00::/7",        // IPv6 ULA
		"fe80::/10",       // IPv6 link-local
		"ff00::/8",        // IPv6 multicast
		"::/128",          // unspecified
		"192.0.2.0/24",    // documentation
		"198.51.100.0/24", // documentation
		"203.0.113.0/24",  // documentation
		"64:ff9b::/96",    // NAT64 (can encode private IPv4 destinations)
		"64:ff9b:1::/48",  // local-use NAT64
		"2001::/32",       // Teredo (encapsulated IPv4)
		"2002::/16",       // 6to4 (encapsulated IPv4)
		"2001:db8::/32",   // documentation
	} {
		if _, network, err := net.ParseCIDR(cidr); err == nil {
			privateRanges = append(privateRanges, network)
		}
	}
}

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

// ResolvePublicURL checks the scheme, host and *all* DNS answers, then
// returns a vetted IP for the connection. The HTTP transport must dial this
// IP directly (including when using an HTTP/SOCKS proxy); checking DNS before
// connecting by hostname alone is vulnerable to DNS rebinding at the proxy.
func ResolvePublicURL(ctx context.Context, raw string) (net.IP, error) {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") ||
		u.Opaque != "" || u.User != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("invalid URL")
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") ||
		host == "metadata" || host == "metadata.google.internal" {
		return nil, fmt.Errorf("address is not allowed")
	}
	if ip := net.ParseIP(host); ip != nil {
		if IsBlockedIP(ip) {
			return nil, fmt.Errorf("address is not allowed")
		}
		return ip, nil
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("DNS resolution failed")
	}
	for _, addr := range ips {
		if IsBlockedIP(addr.IP) {
			return nil, fmt.Errorf("address is not allowed")
		}
	}
	return ips[0].IP, nil
}
