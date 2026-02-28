package utils

import "net"

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
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 ULA
		"fe80::/10",      // IPv6 link-local
	} {
		if _, network, err := net.ParseCIDR(cidr); err == nil {
			privateRanges = append(privateRanges, network)
		}
	}
}

// PrivateRanges returns the list of blocked IP networks (used by the SSRF-safe dialer in upload.go).
func PrivateRanges() []*net.IPNet { return privateRanges }
