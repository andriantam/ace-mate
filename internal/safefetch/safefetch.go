// Package safefetch reads a user-supplied job-posting URL without letting users reach
// internal infrastructure (SSRF). Rules: https on port 443 only, no IP literals or
// credentials, the resolved address is checked at connect time (which defeats DNS
// rebinding), redirects are re-validated, and size and time are capped.
package safefetch

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/andriantam/ace-mate/internal/version"
)

const (
	maxBytes     = 2 << 20
	maxText      = 60_000
	timeout      = 10 * time.Second
	maxRedirects = 3
)

// ErrUser marks errors whose message is safe to show to the user.
type ErrUser struct{ Msg string }

func (e *ErrUser) Error() string { return e.Msg }

// Retryable is false: the page refused or couldn't be read, and trying again won't change that.
func (e *ErrUser) Retryable() bool { return false }

func userErr(format string, a ...any) error { return &ErrUser{Msg: fmt.Sprintf(format, a...)} }

var blocked = mustPrefixes(
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
	"192.0.0.0/24", "192.0.2.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24",
	"224.0.0.0/4", "240.0.0.0/4",
	"::/128", "::1/128", "fc00::/7", "fe80::/10", "ff00::/8", "64:ff9b::/96", "2001:db8::/32",
)

func mustPrefixes(cidrs ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(cidrs))
	for i, c := range cidrs {
		out[i] = netip.MustParsePrefix(c)
	}
	return out
}

// IsBlocked reports whether an address must never be contacted.
func IsBlocked(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() {
		return true
	}
	for _, p := range blocked {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// Validate checks a URL before any network access.
func Validate(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return nil, userErr("That doesn't look like a valid link.")
	}
	if u.Scheme != "https" {
		return nil, userErr("Only https:// links are supported.")
	}
	if u.User != nil {
		return nil, userErr("Links with credentials are not allowed.")
	}
	if p := u.Port(); p != "" && p != "443" {
		return nil, userErr("Non-standard ports are not allowed.")
	}
	host := strings.ToLower(u.Hostname())
	if _, err := netip.ParseAddr(host); err == nil ||
		host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".internal") ||
		strings.HasSuffix(host, ".local") || !strings.Contains(host, ".") {
		return nil, userErr("That host is not allowed.")
	}
	return u, nil
}

func newClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		// Control runs after DNS resolution, once per connection attempt, so the check
		// applies to the exact address being dialled.
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return errors.New("blocked")
			}
			ip, err := netip.ParseAddr(host)
			if err != nil || IsBlocked(ip) {
				return errors.New("blocked address")
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:                  nil, // never route through environment proxies
			DialContext:            dialer.DialContext,
			TLSHandshakeTimeout:    5 * time.Second,
			ResponseHeaderTimeout:  timeout,
			MaxResponseHeaderBytes: 64 << 10,
			DisableKeepAlives:      true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return userErr("Too many redirects.")
			}
			if _, err := Validate(req.URL.String()); err != nil {
				return err
			}
			return nil
		},
	}
}

var (
	dropRe   = regexp.MustCompile(`(?is)<(script|style|noscript|svg|iframe|template)\b.*?</(script|style|noscript|svg|iframe|template)>`)
	blockRe  = regexp.MustCompile(`(?i)</(p|div|li|h[1-6]|tr|section|article|ul|ol)>|<br\s*/?>`)
	tagRe    = regexp.MustCompile(`<[^>]+>`)
	spaceRe  = regexp.MustCompile(`[ \t]+`)
	blanksRe = regexp.MustCompile(`\n\s*\n+`)
)

// HTMLToText strips markup, scripts and styles.
func HTMLToText(s string) string {
	s = dropRe.ReplaceAllString(s, " ")
	s = blockRe.ReplaceAllString(s, "\n")
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = spaceRe.ReplaceAllString(s, " ")
	s = blanksRe.ReplaceAllString(s, "\n\n")
	s = strings.TrimSpace(s)
	if len(s) > maxText {
		s = s[:maxText]
	}
	return s
}

// Fetch returns the posting as plain text, or an *ErrUser that explains what to do.
func Fetch(ctx context.Context, raw string) (string, error) {
	u, err := Validate(raw)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", userErr("That doesn't look like a valid link.")
	}
	req.Header.Set("User-Agent", "AceMate/"+version.Version+" (job-posting-reader)")
	req.Header.Set("Accept", "text/html,text/plain;q=0.9")

	resp, err := newClient().Do(req)
	if err != nil {
		var eu *ErrUser
		if errors.As(err, &eu) {
			return "", eu
		}
		return "", userErr("Couldn't fetch that page. Paste the job description text instead.")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", userErr("The job site returned %d. Paste the job description text instead.", resp.StatusCode)
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if !strings.Contains(ct, "text/html") && !strings.Contains(ct, "text/plain") {
		return "", userErr("That link isn't a web page.")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil || len(body) > maxBytes {
		return "", userErr("That page is too large. Paste the job description text instead.")
	}
	text := string(body)
	if strings.Contains(ct, "html") {
		text = HTMLToText(text)
	} else if len(text) > maxText {
		text = text[:maxText]
	}
	if len(text) < 300 {
		return "", userErr("The page had too little text (it may need JavaScript). Paste the job description text instead.")
	}
	return text, nil
}

// Get fetches a URL under the same SSRF rules as Fetch and returns the raw body and its
// content type. Errors are *ErrUser with generic wording.
func Get(ctx context.Context, raw, accept string) ([]byte, string, error) {
	u, err := Validate(raw)
	if err != nil {
		return nil, "", err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", userErr("That doesn't look like a valid link.")
	}
	req.Header.Set("User-Agent", "AceMate/"+version.Version+" (profile-reader)")
	req.Header.Set("Accept", accept)
	resp, err := newClient().Do(req)
	if err != nil {
		var eu *ErrUser
		if errors.As(err, &eu) {
			return nil, "", eu
		}
		return nil, "", userErr("Couldn't reach that site.")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, "", userErr("That page doesn't exist (404).")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", userErr("The site returned %d.", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil || len(body) > maxBytes {
		return nil, "", userErr("That page is too large.")
	}
	return body, strings.ToLower(resp.Header.Get("Content-Type")), nil
}
