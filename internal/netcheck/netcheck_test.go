package netcheck

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyHealthyAcceptsAny2xx(t *testing.T) {
	var gotProxy bool
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProxy = r.URL.IsAbs() // a proxied request carries the full URL
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxy.Close()
	t.Setenv("DWF_PROBE_URL", "http://discord.invalid/probe")
	if !ProxyHealthy(context.Background(), proxy.URL) || !gotProxy {
		t.Fatal("a 204 through the proxy should count as healthy")
	}
	if ProxyHealthy(context.Background(), "") {
		t.Fatal("no proxy is never healthy")
	}
}

func TestIPv4Only(t *testing.T) {
	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip("no IPv6 loopback")
	}
	defer ln.Close()
	go http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	url := "http://" + ln.Addr().String()
	if _, _, err := get(context.Background(), "", url, true); err == nil {
		t.Fatal("ipv4-only client must not connect over IPv6")
	}
	if code, _, err := get(context.Background(), "", url, false); err != nil || code != 200 {
		t.Fatalf("normal client should connect: %d %v", code, err)
	}
}
