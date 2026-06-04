package streaming

import "net"

// hostOnly strips the port from a "host:port" address, returning the bare host.
// It is bracket-aware via net.SplitHostPort, so IPv6 addresses survive intact
// ("[2001:db8::1]:54321" -> "2001:db8::1"). Inputs without a port are returned
// unchanged.
func hostOnly(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}
