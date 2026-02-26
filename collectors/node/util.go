package node

import (
	"net"
	"strings"
)

// internalNetworks lists non-routable IPv4/IPv6 ranges.
var internalNetworks []*net.IPNet

func init() {
	cidrs := []string{
		// IPv4 private
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",

		// IPv4 special-use
		"127.0.0.0/8",        // loopback
		"169.254.0.0/16",     // link-local
		"100.64.0.0/10",      // carrier-grade NAT
		"198.18.0.0/15",      // benchmarking
		"0.0.0.0/8",          // unspecified
		"255.255.255.255/32", // broadcast

		// IPv6 special-use
		"fc00::/7",  // unique local
		"fe80::/10", // link-local
		"::1/128",   // loopback
		"::/128",    // unspecified
	}

	for _, cidr := range cidrs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(err)
		}
		internalNetworks = append(internalNetworks, n)
	}
}

// IsInternalIP returns true if the IP is from a non-routable/internal range.
func IsInternalIP(ip string) bool {
	var nip net.IP
	var err error

	if strings.Contains(ip, "/") {
		nip, _, err = net.ParseCIDR(ip)
		if err != nil {
			return false
		}
	} else {
		nip = net.ParseIP(ip)
	}

	if nip == nil {
		return false
	}

	for _, n := range internalNetworks {
		if n.Contains(nip) {
			return true
		}
	}

	return false
}
