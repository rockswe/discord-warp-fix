// Package warp drives Cloudflare WARP through warp-cli.
package warp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ErrNotInstalled means warp-cli couldn't be found.
var ErrNotInstalled = errors.New("warp-cli not found. Is Cloudflare WARP installed?")

// Client runs warp-cli.
type Client struct{ Bin string }

// Find locates warp-cli on PATH or in its default install location.
func Find() (*Client, error) {
	if p, err := exec.LookPath("warp-cli"); err == nil {
		return &Client{Bin: p}, nil
	}
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{"/usr/local/bin/warp-cli", "/Applications/Cloudflare WARP.app/Contents/Resources/warp-cli"}
	case "linux":
		candidates = []string{"/usr/bin/warp-cli", "/usr/local/bin/warp-cli"}
	case "windows":
		pf := os.Getenv("ProgramFiles")
		if pf == "" {
			pf = `C:\Program Files`
		}
		candidates = []string{filepath.Join(pf, "Cloudflare", "Cloudflare WARP", "warp-cli.exe")}
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return &Client{Bin: c}, nil
		}
	}
	return nil, ErrNotInstalled
}

func (c *Client) run(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Bin, append([]string{"--accept-tos"}, args...)...)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func field(out, prefix string) string {
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), prefix); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// Status is warp-cli's connection state, e.g. "Connected".
func (c *Client) Status() string {
	out, _ := c.run("status")
	return field(out, "Status update:")
}

// Protocol is the tunnel protocol: "WireGuard" or "MASQUE".
func (c *Client) Protocol() string {
	out, _ := c.run("tunnel", "stats")
	p := field(out, "Tunnel Protocol:")
	if i := strings.IndexAny(p, " ("); i > 0 {
		p = p[:i]
	}
	return p
}

// SetProtocol switches the tunnel protocol, which reconnects WARP.
func (c *Client) SetProtocol(p string) error {
	out, err := c.run("tunnel", "protocol", "set", p)
	if err != nil {
		return fmt.Errorf("warp-cli refused to switch protocol: %s", strings.TrimSpace(out))
	}
	return nil
}

// Reroll flips the tunnel protocol until Cloudflare shows a different exit
// IP. WARP hands out one of a few shared exits at connection time, so a
// flip is the only lever a client has. exitIP must return "" while offline.
func Reroll(c *Client, exitIP func() string, attempts int, logf func(string, ...any)) (before, after string, err error) {
	before = exitIP()
	if before == "" {
		return "", "", errors.New("can't reach Cloudflare through WARP. Is it connected?")
	}
	for i := 1; i <= attempts; i++ {
		next := "WireGuard"
		if c.Protocol() == "WireGuard" {
			next = "MASQUE"
		}
		if err := c.SetProtocol(next); err != nil {
			return before, "", err
		}
		time.Sleep(4 * time.Second)
		after = ""
		for j := 0; j < 15 && after == ""; j++ {
			if after = exitIP(); after == "" {
				time.Sleep(2 * time.Second)
			}
		}
		logf("reroll attempt %d/%d: protocol=%s exit=%s", i, attempts, next, orNone(after))
		if after != "" && after != before {
			return before, after, nil
		}
	}
	return before, before, fmt.Errorf("exit IP is still %s after %d attempts", before, attempts)
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
