// Package httpx provides a hardened JSON-over-HTTP client for LLM, GitHub, Jira, and Confluence.
package httpx

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var Version = "dev"

var (
	prefixLinkLocalV4  = netip.MustParsePrefix("169.254.0.0/16")
	prefixLinkLocalV6  = netip.MustParsePrefix("fe80::/10")
	prefixCGNAT        = netip.MustParsePrefix("100.64.0.0/10")
	prefixBenchmarking = netip.MustParsePrefix("198.18.0.0/15")
	prefixDoc1         = netip.MustParsePrefix("192.0.2.0/24")
	prefixDoc2         = netip.MustParsePrefix("198.51.100.0/24")
	prefixDoc3         = netip.MustParsePrefix("203.0.113.0/24")
	prefixDocV6        = netip.MustParsePrefix("2001:db8::/32")

	reBearer = regexp.MustCompile(`(?i)(Bearer\s+)[^\s"'>]+`)
	reBasic  = regexp.MustCompile(`(?i)(Basic\s+)[^\s"'>]+`)
	reAPIKey = regexp.MustCompile(`(?i)(x-api-key\s*[:=]\s*)[^\s"'>]+`)
)

// Policy configures the transport and client hardening parameters.
type Policy struct {
	Secrets       []string            // values to scrub from any error text
	AllowPrivate  bool                // RFC1918/CGNAT/ULA/link-local targets (on-prem GHES, Jira DC) — default false
	AllowLoopback bool                // 127.0.0.0/8, ::1 — tests only (SCRIBEX_ALLOW_INSECURE_LOCAL=1)
	AllowHTTP     bool                // plain http — only honoured together with AllowLoopback
	Timeout       time.Duration       // whole attempt; default 30s
	MaxBody       int64               // response bytes; default 4 MiB
	MaxAttempts   int                 // default 3 (1 = no retry)
	RetryPOST     bool                // LLM only
	RootCAs       *x509.CertPool      // tests only
	Sleep         func(time.Duration) // injectable for tests; default time.Sleep
}

// Default returns a Policy reading flags from the environment.
func Default() Policy {
	insecureLocal := os.Getenv("SCRIBEX_ALLOW_INSECURE_LOCAL") == "1"
	return Policy{
		AllowPrivate:  os.Getenv("SCRIBEX_ALLOW_PRIVATE_HOSTS") == "1",
		AllowLoopback: insecureLocal,
		AllowHTTP:     insecureLocal,
		Timeout:       30 * time.Second,
		MaxBody:       4 << 20,
		MaxAttempts:   3,
	}
}

// Basic returns an Authorization header value.
func Basic(user, token string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+token))
}

// Client is a hardened HTTP client.
type Client struct {
	policy     Policy
	httpClient *http.Client
}

// BlockedIP reports whether a resolved IP address should be rejected, returning the reason or "".
func BlockedIP(a netip.Addr, p Policy) string {
	return blockedIP(a, p)
}

func blockedIP(a netip.Addr, p Policy) string {
	// Unmap IPv4-in-IPv6
	if a.Is4In6() {
		a = a.Unmap()
	}

	// 6to4 2002::/16
	b := a.As16()
	if b[0] == 0x20 && b[1] == 0x02 {
		a = netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]})
	}

	// NAT64 Well-Known Prefix 64:ff9b::/96
	if b[0] == 0 && b[1] == 0x64 && b[2] == 0xff && b[3] == 0x9b &&
		b[4] == 0 && b[5] == 0 && b[6] == 0 && b[7] == 0 &&
		b[8] == 0 && b[9] == 0 && b[10] == 0 && b[11] == 0 {
		a = netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]})
	}

	// Unspecified
	if a.IsUnspecified() {
		return "unspecified address"
	}

	// Multicast
	if a.IsMulticast() {
		return "multicast address"
	}

	// Link-local (never allowed even with AllowPrivate)
	if a.IsLinkLocalUnicast() || (a.Is4() && prefixLinkLocalV4.Contains(a)) || (a.Is6() && prefixLinkLocalV6.Contains(a)) {
		return "link-local address"
	}

	// Loopback
	if a.IsLoopback() {
		if !p.AllowLoopback {
			return "loopback address"
		}
		return ""
	}

	// Documentation ranges
	if (a.Is4() && (prefixDoc1.Contains(a) || prefixDoc2.Contains(a) || prefixDoc3.Contains(a))) ||
		(a.Is6() && prefixDocV6.Contains(a)) {
		if !p.AllowPrivate {
			return "documentation address"
		}
	}

	// Benchmarking
	if a.Is4() && prefixBenchmarking.Contains(a) {
		if !p.AllowPrivate {
			return "benchmarking address"
		}
	}

	// CGNAT
	if a.Is4() && prefixCGNAT.Contains(a) {
		if !p.AllowPrivate {
			return "CGNAT address"
		}
	}

	// Private RFC1918 / ULA
	if a.IsPrivate() {
		if !p.AllowPrivate {
			return "private network address"
		}
	}

	return ""
}

func isLoopbackHost(h string) bool {
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// New constructs a hardened Client enforcing the given Policy.
func New(p Policy) *Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			addr, err := netip.ParseAddr(host)
			if err != nil {
				return err
			}
			if reason := blockedIP(addr, p); reason != "" {
				return fmt.Errorf("blocked address %s: %s", addr, reason)
			}
			return nil
		},
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ResponseHeaderTimeout: 20 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		DisableCompression:    true,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    p.RootCAs,
		},
	}

	httpClient := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("stopped after 3 redirects")
			}
			orig := via[0]
			if req.URL.Scheme != orig.URL.Scheme {
				return fmt.Errorf("redirect scheme changed from %s to %s", orig.URL.Scheme, req.URL.Scheme)
			}
			if !strings.EqualFold(req.URL.Host, orig.URL.Host) {
				return fmt.Errorf("cross-host redirect from %s to %s refused", orig.URL.Host, req.URL.Host)
			}
			return nil
		},
	}

	return &Client{
		policy:     p,
		httpClient: httpClient,
	}
}

func cleanURLString(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.RawQuery = ""
	u.User = nil
	u.Fragment = ""
	return u.String()
}

func scrub(text string, secrets []string) string {
	for _, s := range secrets {
		s = strings.TrimSpace(s)
		if len(s) >= 6 {
			text = strings.ReplaceAll(text, s, "[redacted]")
		}
	}
	text = reBearer.ReplaceAllString(text, "$1[redacted]")
	text = reBasic.ReplaceAllString(text, "$1[redacted]")
	text = reAPIKey.ReplaceAllString(text, "$1[redacted]")
	return text
}

func (c *Client) sanitizeError(err error) error {
	if err == nil {
		return nil
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		clean := cleanURLString(urlErr.URL)
		err = fmt.Errorf("%s %s: %v", urlErr.Op, clean, urlErr.Err)
	}
	msg := scrub(err.Error(), c.policy.Secrets)
	return errors.New(msg)
}

func calcBackoff(attempt int, retryAfterHeader string) time.Duration {
	if retryAfterHeader != "" {
		if sec, err := strconv.Atoi(strings.TrimSpace(retryAfterHeader)); err == nil && sec > 0 {
			d := time.Duration(sec) * time.Second
			if d > 30*time.Second {
				d = 30 * time.Second
			}
			return d
		}
		if t, err := http.ParseTime(retryAfterHeader); err == nil {
			d := time.Until(t)
			if d < time.Second {
				d = time.Second
			}
			if d > 30*time.Second {
				d = 30 * time.Second
			}
			return d
		}
	}

	base := time.Second * (1 << (attempt - 1))
	jitter := time.Duration(rand.Intn(250)) * time.Millisecond
	d := base + jitter
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

// Do sends in as JSON (if non-nil) and decodes the JSON reply into out (if non-nil).
func (c *Client) Do(method, rawURL string, hdr map[string]string, in, out any) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return c.sanitizeError(fmt.Errorf("invalid URL: %w", err))
	}
	if u.User != nil {
		return c.sanitizeError(fmt.Errorf("userinfo in URL is not allowed"))
	}
	if u.Scheme == "http" {
		if !(c.policy.AllowHTTP && c.policy.AllowLoopback && isLoopbackHost(u.Hostname())) {
			return c.sanitizeError(fmt.Errorf("http scheme not allowed"))
		}
	} else if u.Scheme != "https" {
		return c.sanitizeError(fmt.Errorf("scheme %q not allowed; only https is permitted", u.Scheme))
	}

	// Pre-flight check if proxy applies
	reqForProxyCheck, _ := http.NewRequest(method, rawURL, nil)
	if reqForProxyCheck != nil {
		if proxyURL, _ := http.ProxyFromEnvironment(reqForProxyCheck); proxyURL != nil {
			// Best-effort pre-flight DNS lookup on target host when proxy is used
			targetHost := u.Hostname()
			if ips, err := net.LookupIP(targetHost); err == nil {
				for _, ip := range ips {
					if addr, ok := netip.AddrFromSlice(ip); ok {
						if reason := blockedIP(addr, c.policy); reason != "" {
							return c.sanitizeError(fmt.Errorf("pre-flight proxy check blocked target %s: %s", addr, reason))
						}
					}
				}
			}
		}
	}

	maxAttempts := c.policy.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	timeout := c.policy.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	maxBody := c.policy.MaxBody
	if maxBody <= 0 {
		maxBody = 4 << 20
	}
	sleep := c.policy.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}

	methodUpper := strings.ToUpper(method)
	isEligibleMethod := methodUpper == "GET" || methodUpper == "HEAD" || methodUpper == "PUT" ||
		methodUpper == "DELETE" || methodUpper == "PATCH" || (methodUpper == "POST" && c.policy.RetryPOST)

	var bodyBytes []byte
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return c.sanitizeError(err)
		}
		bodyBytes = b
	}

	var totalWait time.Duration

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var r io.Reader
		if bodyBytes != nil {
			r = bytes.NewReader(bodyBytes)
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		req, err := http.NewRequestWithContext(ctx, method, rawURL, r)
		if err != nil {
			cancel()
			return c.sanitizeError(err)
		}

		req.Header.Set("User-Agent", "scribex/"+Version)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		for k, v := range hdr {
			req.Header.Set(k, v)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			cancel()
			if attempt < maxAttempts && isEligibleMethod {
				wait := calcBackoff(attempt, "")
				totalWait += wait
				if totalWait <= 60*time.Second {
					sleep(wait)
					continue
				}
			}
			return c.sanitizeError(err)
		}

		// Status handling
		status := resp.StatusCode
		if status == 429 || status == 502 || status == 503 || status == 504 {
			if attempt < maxAttempts && isEligibleMethod {
				retryAfter := resp.Header.Get("Retry-After")
				resp.Body.Close()
				cancel()
				wait := calcBackoff(attempt, retryAfter)
				totalWait += wait
				if totalWait <= 60*time.Second {
					sleep(wait)
					continue
				}
			}
		}

		if status >= 300 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			cancel()
			clean := cleanURLString(rawURL)
			return c.sanitizeError(fmt.Errorf("%s %s: %s: %s", method, clean, resp.Status, b))
		}

		// Success response
		b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
		resp.Body.Close()
		cancel()
		if err != nil {
			return c.sanitizeError(err)
		}
		if int64(len(b)) > maxBody {
			return c.sanitizeError(fmt.Errorf("response exceeds %d bytes", maxBody))
		}
		if out != nil {
			if err := json.Unmarshal(b, out); err != nil {
				return c.sanitizeError(err)
			}
		}
		return nil
	}

	return c.sanitizeError(fmt.Errorf("request failed after %d attempts", maxAttempts))
}

// Policy returns a copy of the client's Policy.
func (c *Client) Policy() Policy {
	return c.policy
}

// Package-level Do maintains backwards compatibility and uses New(Default()).
var Do = func(method, url string, hdr map[string]string, in, out any) error {
	return New(Default()).Do(method, url, hdr, in, out)
}
