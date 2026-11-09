package evaltarget

import (
	"net"
	"net/url"
	"testing"
)

// TestLiveEndpointMatchesIsolationRule prevents the fixed endpoint URL from
// drifting away from the single-host:port Seatbelt rule. Seatbelt requires the
// "localhost" spelling; the decider dials the loopback address in the URL.
func TestLiveEndpointMatchesIsolationRule(t *testing.T) {
	u, err := url.Parse(LiveEndpointURL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "http" {
		t.Fatalf("live endpoint scheme = %q, want http", u.Scheme)
	}
	if u.Hostname() != "127.0.0.1" && u.Hostname() != "::1" && u.Hostname() != "localhost" {
		t.Fatalf("live endpoint host %q is not loopback", u.Hostname())
	}
	host, port, err := net.SplitHostPort(LiveEndpointHostPort)
	if err != nil {
		t.Fatal(err)
	}
	if host != "localhost" {
		t.Fatalf("isolation host = %q, want localhost (Seatbelt requires this spelling)", host)
	}
	if port != u.Port() {
		t.Fatalf("isolation port %q does not match live endpoint port %q", port, u.Port())
	}
}
