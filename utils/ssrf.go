package utils

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

func mustPrefixes(cidrs ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(cidrs))
	for i, c := range cidrs {
		out[i] = netip.MustParsePrefix(c)
	}
	return out
}

// blockedPrefixes lists networks that must never be contacted via
// user-supplied URLs (SSRF prevention): private, internal and special-purpose
// ranges, plus transition mechanisms that can embed such IPv4 addresses.
// Loopback, link-local, multicast and unspecified addresses are additionally
// rejected by IsBlockedIP.
var blockedPrefixes = mustPrefixes(
	"0.0.0.0/8",       // "this" network
	"10.0.0.0/8",      // RFC 1918
	"100.64.0.0/10",   // CGNAT
	"127.0.0.0/8",     // loopback
	"169.254.0.0/16",  // link-local / cloud metadata
	"172.16.0.0/12",   // RFC 1918
	"192.0.0.0/24",    // IETF protocol assignments
	"192.0.2.0/24",    // documentation
	"192.88.99.0/24",  // deprecated 6to4 relay anycast
	"192.168.0.0/16",  // RFC 1918
	"198.18.0.0/15",   // benchmarking
	"198.51.100.0/24", // documentation
	"203.0.113.0/24",  // documentation
	"224.0.0.0/4",     // IPv4 multicast
	"240.0.0.0/4",     // reserved / broadcast
	"::/96",           // unspecified, loopback, deprecated IPv4-compatible
	"64:ff9b::/96",    // NAT64 (can encode private IPv4 destinations)
	"64:ff9b:1::/48",  // local-use NAT64
	"100::/64",        // discard-only
	"2001::/23",       // IETF protocol assignments, incl. Teredo
	"2001:db8::/32",   // documentation
	"2002::/16",       // 6to4 (encapsulated IPv4)
	"fc00::/7",        // IPv6 unique local
	"fe80::/10",       // IPv6 link-local
	"fec0::/10",       // deprecated IPv6 site-local
	"ff00::/8",        // IPv6 multicast
)

// localPrefixes complements netip's loopback, link-local and private checks
// for the "local" access level.
var localPrefixes = mustPrefixes(
	"100.64.0.0/10", // CGNAT (also used by overlay VPNs such as Tailscale)
)

// toAddr converts a net.IP to its canonical netip form (IPv4-mapped IPv6
// addresses become plain IPv4).
func toAddr(ip net.IP) (netip.Addr, bool) {
	addr, ok := netip.AddrFromSlice(ip)
	return addr.Unmap(), ok
}

func inPrefixes(addr netip.Addr, prefixes []netip.Prefix) bool {
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// IsBlockedIP reports whether ip is in a range that must not be contacted
// via user-supplied URLs (loopback, RFC1918, link-local, metadata, etc.).
func IsBlockedIP(ip net.IP) bool {
	addr, ok := toAddr(ip)
	if !ok || !addr.IsGlobalUnicast() {
		return true
	}
	return inPrefixes(addr, blockedPrefixes)
}

// IsPrivateOrLocalIP reports whether ip belongs to a private, loopback,
// link-local or CGNAT network. Used for the "local" access level.
func IsPrivateOrLocalIP(ip net.IP) bool {
	addr, ok := toAddr(ip)
	if !ok {
		return false
	}
	return addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsPrivate() ||
		inPrefixes(addr, localPrefixes)
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
