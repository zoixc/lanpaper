package utils

import "net"

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
