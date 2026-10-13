package web

import (
	"net"
	"strings"
	"testing"
)

func tcp(host string, port int) net.Addr {
	return &net.TCPAddr{IP: net.ParseIP(host), Port: port}
}

// A listener on a loopback address advertises only the local URL, never a LAN URL —
// even when network access is enabled, because loopback is not reachable on a LAN.
func TestResolveStartupURLsLoopbackOnly(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		u := ResolveStartupURLs(tcp("127.0.0.1", 8090), "http", enabled, []net.IP{net.ParseIP("192.168.86.249")})
		if u.Local != "http://localhost:8090" {
			t.Errorf("network=%t: local = %q, want http://localhost:8090", enabled, u.Local)
		}
		if len(u.Network) != 0 {
			t.Errorf("network=%t: loopback must not advertise a LAN URL, got %v", enabled, u.Network)
		}
		if u.Approvals != "http://localhost:8090/approvals" {
			t.Errorf("network=%t: approvals = %q", enabled, u.Approvals)
		}
	}
}

// An unspecified bind with network access advertises the detected LAN address and never
// the unspecified 0.0.0.0.
func TestResolveStartupURLsLANEnabled(t *testing.T) {
	u := ResolveStartupURLs(tcp("0.0.0.0", 8090), "http", true, []net.IP{net.ParseIP("192.168.86.249")})
	if u.Local != "http://localhost:8090" {
		t.Errorf("local = %q, want http://localhost:8090", u.Local)
	}
	if len(u.Network) != 1 || u.Network[0] != "http://192.168.86.249:8090" {
		t.Errorf("network = %v, want [http://192.168.86.249:8090]", u.Network)
	}
	if u.Approvals != "http://192.168.86.249:8090/approvals" {
		t.Errorf("approvals = %q", u.Approvals)
	}
	if strings.Contains(u.Local+strings.Join(u.Network, " "), "0.0.0.0") {
		t.Errorf("0.0.0.0 must never be advertised as a browser URL")
	}
}

// A dynamically allocated port is reflected verbatim.
func TestResolveStartupURLsDynamicPort(t *testing.T) {
	u := ResolveStartupURLs(tcp("127.0.0.1", 54321), "http", false, nil)
	if u.Local != "http://localhost:54321" {
		t.Errorf("local = %q, want the dynamically bound port", u.Local)
	}
}

// IPv6 literals are bracketed, loopback IPv6 maps to localhost, and link-local IPv6 is
// not advertised.
func TestResolveStartupURLsIPv6(t *testing.T) {
	if got := ResolveStartupURLs(tcp("::1", 8090), "http", true, nil).Local; got != "http://localhost:8090" {
		t.Errorf("ipv6 loopback local = %q, want http://localhost:8090", got)
	}
	if got := formatURL("https", "2001:db8::5", "443", ""); got != "https://[2001:db8::5]:443" {
		t.Errorf("formatURL ipv6 = %q", got)
	}
	// IPv6-only: bracketed, link-local excluded.
	u := ResolveStartupURLs(tcp("::", 8090), "http", true, []net.IP{
		net.ParseIP("fe80::1"), net.ParseIP("2001:db8::5"),
	})
	if len(u.Network) != 1 || u.Network[0] != "http://[2001:db8::5]:8090" {
		t.Errorf("ipv6-only network = %v, want [http://[2001:db8::5]:8090]", u.Network)
	}
	// When an IPv4 address exists it is preferred over IPv6.
	u = ResolveStartupURLs(tcp("::", 8090), "http", true, []net.IP{
		net.ParseIP("2001:db8::5"), net.ParseIP("192.168.1.2"),
	})
	if len(u.Network) != 1 || u.Network[0] != "http://192.168.1.2:8090" {
		t.Errorf("IPv4 should be preferred over IPv6: %v", u.Network)
	}
}

// Network access disabled yields no LAN URL even for an unspecified bind.
func TestResolveStartupURLsNoNetworkWhenDisabled(t *testing.T) {
	u := ResolveStartupURLs(tcp("0.0.0.0", 8090), "http", false, []net.IP{net.ParseIP("192.168.86.249")})
	if len(u.Network) != 0 {
		t.Errorf("network access disabled must advertise no LAN URL, got %v", u.Network)
	}
	if strings.Contains(StartupSummary(u, false), "192.168.86.249") {
		t.Errorf("summary leaked a LAN URL while network access was disabled")
	}
}

// A bind to one specific address is shown once, not duplicated as both local and network.
func TestResolveStartupURLsSpecificBind(t *testing.T) {
	u := ResolveStartupURLs(tcp("192.168.1.5", 8090), "http", true, nil)
	if u.Local != "http://192.168.1.5:8090" {
		t.Errorf("local = %q", u.Local)
	}
	if len(u.Network) != 0 {
		t.Errorf("specific bind should not duplicate the address, got %v", u.Network)
	}
}

// An HTTPS scheme is honored rather than assumed to be HTTP.
func TestResolveStartupURLsScheme(t *testing.T) {
	u := ResolveStartupURLs(tcp("0.0.0.0", 443), "https", true, []net.IP{net.ParseIP("10.0.0.5")})
	if !strings.HasPrefix(u.Local, "https://") || !strings.HasPrefix(u.Network[0], "https://") {
		t.Errorf("scheme not honored: local=%q network=%v", u.Local, u.Network)
	}
}

// The summary never contains a secret, and reports the auth state.
func TestStartupSummaryHasNoSecrets(t *testing.T) {
	u := ResolveStartupURLs(tcp("0.0.0.0", 8090), "http", true, []net.IP{net.ParseIP("192.168.86.249")})
	out := StartupSummary(u, true)
	for _, want := range []string{
		"SOP Controller started", "Local:      http://localhost:8090",
		"Network:    http://192.168.86.249:8090",
		"Approvals:  http://192.168.86.249:8090/approvals",
		"Auth:       enabled", "Press Ctrl+C to stop",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
	for _, forbidden := range []string{"token", "secret", "password", "cookie", "@", "?token="} {
		if strings.Contains(strings.ToLower(out), forbidden) {
			t.Errorf("summary must not contain %q:\n%s", forbidden, out)
		}
	}
	if !strings.Contains(StartupSummary(u, false), "Auth:       disabled") {
		t.Errorf("summary should report disabled auth when no token is required")
	}
}

// usableLANIP keeps routable addresses and drops loopback, unspecified, and link-local.
func TestUsableLANIP(t *testing.T) {
	cases := map[string]bool{
		"192.168.1.2": true,
		"10.0.0.5":    true,
		"2001:db8::5": true,
		"127.0.0.1":   false,
		"::1":         false,
		"0.0.0.0":     false,
		"::":          false,
		"169.254.1.1": false,
		"fe80::1":     false,
	}
	for addr, want := range cases {
		if got := usableLANIP(net.ParseIP(addr)); got != want {
			t.Errorf("usableLANIP(%s) = %t, want %t", addr, got, want)
		}
	}
}
