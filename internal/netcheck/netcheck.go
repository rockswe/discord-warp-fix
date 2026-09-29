// Package netcheck answers "which IP does the internet see?" and "does this
// proxy reach Discord?".
package netcheck

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Endpoints. DWF_PROBE_URL overrides the Discord probe, which CI uses.
const (
	TraceURL  = "https://www.cloudflare.com/cdn-cgi/trace"
	IPEchoURL = "https://api.ipify.org"
)

// ProbeURL is fetched through the proxy to prove it reaches Discord.
func ProbeURL() string {
	if v := os.Getenv("DWF_PROBE_URL"); v != "" {
		return v
	}
	return "https://discord.com/api/v9/experiments"
}

func client(proxy string, ipv4 bool) (*http.Client, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	if ipv4 {
		// Discord only has IPv4 addresses, so the IPv4 exit is the one that
		// gets rate-limited. Without this Go would happily report WARP's
		// IPv6 exit instead.
		d := &net.Dialer{Timeout: 10 * time.Second}
		tr.DialContext = func(ctx context.Context, _, addr string) (net.Conn, error) {
			return d.DialContext(ctx, "tcp4", addr)
		}
	}
	if proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil {
			return nil, err
		}
		tr.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Transport: tr, Timeout: 10 * time.Second}, nil
}

func get(ctx context.Context, proxy, target string, ipv4 bool) (int, string, error) {
	c, err := client(proxy, ipv4)
	if err != nil {
		return 0, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("User-Agent", "discord-warp-fix")
	resp, err := c.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	return resp.StatusCode, string(body), nil
}

// ExitIP is the IPv4 address Cloudflare sees for this machine (WARP's exit
// when WARP is on). Empty when unreachable.
func ExitIP(ctx context.Context) string {
	_, body, err := get(ctx, "", TraceURL, true)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(body, "\n") {
		if v, ok := strings.CutPrefix(line, "ip="); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ProxyHealthy reports whether a request through proxy reaches Discord.
func ProxyHealthy(ctx context.Context, proxy string) bool {
	if proxy == "" {
		return false
	}
	code, _, err := get(ctx, proxy, ProbeURL(), false)
	return err == nil && code >= 200 && code < 300
}

// ProxyExitIP is the address sites see for traffic through proxy.
func ProxyExitIP(ctx context.Context, proxy string) string {
	code, body, err := get(ctx, proxy, IPEchoURL, false)
	if err != nil || code != http.StatusOK {
		return ""
	}
	return strings.TrimSpace(body)
}
