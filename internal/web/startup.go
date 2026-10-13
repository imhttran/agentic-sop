package web

import (
	"fmt"
	"net"
	"strings"
)

// StartupURLs is the set of access URLs advertised when the controller starts. Every
// URL here is safe to show: it carries no token, credential, or query string.
type StartupURLs struct {
	// Local is the loopback URL, when the listener is reachable locally.
	Local string
	// Network holds LAN URLs, one per usable interface address. It is empty when
	// network access is disabled, the listener is loopback-only, or no usable LAN
	// address exists.
	Network []string
	// Approvals is the /approvals link, reached via the primary URL.
	Approvals string
}

// ResolveStartupURLs computes the URLs to advertise for a bound listener. It never
// advertises an unspecified address (0.0.0.0 or ::); it advertises a LAN URL only when
// network access is enabled and the listener is bound to (or covers) a usable interface
// address. scheme is the URL scheme — "http", or "https" for a TLS listener.
func ResolveStartupURLs(bound net.Addr, scheme string, networkEnabled bool, lanIPs []net.IP) StartupURLs {
	if scheme == "" {
		scheme = "http"
	}
	host, port := splitHostPort(bound)

	var u StartupURLs
	switch {
	case host == "" || isLoopbackHost(host) || isUnspecifiedHost(host):
		u.Local = formatURL(scheme, "localhost", port, "")
	default:
		u.Local = formatURL(scheme, host, port, "")
	}

	if networkEnabled && !isLoopbackHost(host) {
		if isUnspecifiedHost(host) {
			for _, ip := range usableLANIPs(lanIPs) {
				u.Network = append(u.Network, formatURL(scheme, ip.String(), port, ""))
			}
		} else if ip := net.ParseIP(host); ip != nil && usableLANIP(ip) {
			u.Network = append(u.Network, formatURL(scheme, host, port, ""))
		}
	}

	// A listener bound to one specific address is both local and its own network
	// address; show it once rather than twice.
	network := u.Network[:0]
	for _, n := range u.Network {
		if n != u.Local {
			network = append(network, n)
		}
	}
	u.Network = network

	primary := u.Local
	if len(u.Network) > 0 {
		primary = u.Network[0]
	}
	if primary != "" {
		u.Approvals = primary + "/approvals"
	}
	return u
}

// StartupSummary renders the human-readable startup banner. authEnabled reports whether
// an access token is required; no token or other secret is ever printed.
func StartupSummary(u StartupURLs, authEnabled bool) string {
	var b strings.Builder
	b.WriteString("SOP Controller started\n\n")
	if u.Local != "" {
		fmt.Fprintf(&b, "Local:      %s\n", u.Local)
	}
	for _, n := range u.Network {
		fmt.Fprintf(&b, "Network:    %s\n", n)
	}
	if u.Approvals != "" {
		fmt.Fprintf(&b, "Approvals:  %s\n", u.Approvals)
	}
	b.WriteString("\n")
	if authEnabled {
		b.WriteString("Auth:       enabled\n")
	} else {
		b.WriteString("Auth:       disabled (loopback only)\n")
	}
	b.WriteString("Press Ctrl+C to stop\n")
	return b.String()
}

// LANAddresses returns the usable non-loopback interface addresses, IPv4 first, so the
// controller can advertise a LAN URL without hardcoding one.
func LANAddresses() []net.IP {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var ips []net.IP
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && usableLANIP(ipnet.IP) {
				ips = append(ips, ipnet.IP)
			}
		}
	}
	return usableLANIPs(ips)
}

// usableLANIPs keeps only usable addresses, deduplicated. It prefers IPv4: a phone on
// the LAN almost always reaches the IPv4 address, and a machine's many IPv6 privacy
// addresses would otherwise flood the summary.
func usableLANIPs(ips []net.IP) []net.IP {
	seen := map[string]bool{}
	var v4, v6 []net.IP
	for _, ip := range ips {
		if !usableLANIP(ip) {
			continue
		}
		key := ip.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		if ip.To4() != nil {
			v4 = append(v4, ip)
		} else {
			v6 = append(v6, ip)
		}
	}
	if len(v4) > 0 {
		return v4
	}
	return v6
}

// usableLANIP reports whether ip is a browser-reachable LAN address: not loopback, not
// unspecified, and not link-local.
func usableLANIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	return true
}

func splitHostPort(addr net.Addr) (host, port string) {
	if addr == nil {
		return "", ""
	}
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String(), ""
	}
	return host, port
}

// formatURL builds scheme://host[:port][/path], bracketing an IPv6 literal.
func formatURL(scheme, host, port, path string) string {
	if scheme == "" {
		scheme = "http"
	}
	h := host
	if strings.Contains(h, ":") && !strings.HasPrefix(h, "[") {
		h = "[" + h + "]"
	}
	if port != "" {
		h += ":" + port
	}
	url := scheme + "://" + h
	if path != "" {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		url += path
	}
	return url
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isUnspecifiedHost(host string) bool {
	if host == "" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsUnspecified()
}
