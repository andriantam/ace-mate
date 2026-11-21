package safefetch

import (
	"net/netip"
	"strings"
	"testing"
)

func TestIsBlocked(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "172.31.255.255", "192.168.1.1", "169.254.169.254", "100.64.0.1", "0.0.0.0", "::1", "fd00::1", "fe80::1", "::ffff:127.0.0.1", "::ffff:169.254.169.254"} {
		if !IsBlocked(netip.MustParseAddr(ip)) {
			t.Errorf("%s should be blocked", ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "172.32.0.1", "104.16.0.1", "2606:4700::1111"} {
		if IsBlocked(netip.MustParseAddr(ip)) {
			t.Errorf("%s should be allowed", ip)
		}
	}
}

func TestValidate(t *testing.T) {
	bad := []string{"http://example.com/job", "https://127.0.0.1/", "https://[::1]/", "https://localhost/", "https://example.com:8443/",
		"https://user:pw@example.com/", "file:///etc/passwd", "https://metadata.google.internal/", "https://intranet/", "not a url", "https://printer.local/"}
	for _, u := range bad {
		if _, err := Validate(u); err == nil {
			t.Errorf("%q should be rejected", u)
		}
	}
	if u, err := Validate("https://jobs.example.com/job/123"); err != nil || u.Hostname() != "jobs.example.com" {
		t.Errorf("normal link rejected: %v", err)
	}
}

func TestHTMLToText(t *testing.T) {
	got := HTMLToText("<p>Role</p><script>alert(1)</script><li>Go &amp; Java</li><style>x{}</style>")
	if !strings.Contains(got, "Go & Java") || strings.Contains(got, "alert") || strings.Contains(got, "x{}") {
		t.Errorf("got %q", got)
	}
}

// The dialer must refuse private targets even when the URL looks public. Uses a hostname
// that resolves to loopback without touching the network beyond local DNS.
func TestFetchRefusesLoopbackHostname(t *testing.T) {
	_, err := Fetch(t.Context(), "https://localtest.me/")
	if err == nil {
		t.Fatal("expected the fetch to be blocked")
	}
}
