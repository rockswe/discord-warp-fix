package tunnel

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// testServer is a minimal SSH server that, like a server prepared by
// setup-server.sh, only allows direct-tcpip (outbound) channels.
type testServer struct {
	addr     string
	ln       net.Listener
	mu       sync.Mutex
	conns    []net.Conn
	sessions int
}

func startServer(t *testing.T, clientPub ssh.PublicKey) (*testServer, ssh.Signer) {
	t.Helper()
	_, hk, _ := ed25519.GenerateKey(rand.Reader)
	hostSigner, _ := ssh.NewSignerFromKey(hk)
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
			if string(k.Marshal()) == string(clientPub.Marshal()) {
				return nil, nil
			}
			return nil, io.EOF
		},
	}
	cfg.AddHostKey(hostSigner)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &testServer{addr: ln.Addr().String(), ln: ln}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.conns = append(s.conns, c)
			s.mu.Unlock()
			go s.serve(c, cfg)
		}
	}()
	t.Cleanup(func() { ln.Close(); s.dropAll() })
	return s, hostSigner
}

func (s *testServer) serve(c net.Conn, cfg *ssh.ServerConfig) {
	_, chans, reqs, err := ssh.NewServerConn(c, cfg)
	if err != nil {
		return
	}
	go func() {
		for r := range reqs {
			r.Reply(r.Type == "keepalive@openssh.com", nil)
		}
	}()
	for nc := range chans {
		if nc.ChannelType() != "direct-tcpip" {
			s.mu.Lock()
			s.sessions++
			s.mu.Unlock()
			nc.Reject(ssh.Prohibited, "only forwarding")
			continue
		}
		var p struct {
			Host  string
			Port  uint32
			OHost string
			OPort uint32
		}
		ssh.Unmarshal(nc.ExtraData(), &p)
		target := net.JoinHostPort(p.Host, itoa(p.Port))
		up, err := net.Dial("tcp", target)
		if err != nil {
			nc.Reject(ssh.ConnectionFailed, err.Error())
			continue
		}
		ch, creqs, _ := nc.Accept()
		go ssh.DiscardRequests(creqs)
		go func() { io.Copy(ch, up); ch.CloseWrite() }()
		go func() { io.Copy(up, ch); up.Close() }()
	}
}

func (s *testServer) dropAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.conns {
		c.Close()
	}
	s.conns = nil
}

func itoa(n uint32) string { return strconv.FormatUint(uint64(n), 10) }

func httpViaSOCKS(t *testing.T, socks, target string) (int, string) {
	t.Helper()
	u, _ := url.Parse("socks5://" + socks)
	c := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(u)}, Timeout: 5 * time.Second}
	resp, err := c.Get(target)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestTunnelEndToEnd(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id")
	pubLine, err := GenerateKey(keyPath, "test@dwf")
	if err != nil {
		t.Fatal(err)
	}
	signer, err := LoadKey(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := PublicKeyLine(keyPath); got != pubLine {
		t.Fatalf("PublicKeyLine = %q, want %q", got, pubLine)
	}
	if fi, _ := os.Stat(keyPath); os.PathSeparator == '/' && fi.Mode().Perm() != 0o600 {
		t.Fatalf("private key mode %v", fi.Mode().Perm())
	}

	srv, _ := startServer(t, signer.PublicKey())
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "hello from "+r.Host)
	}))
	defer web.Close()

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	known := filepath.Join(dir, "known_hosts")
	tun := &Tunnel{Addr: srv.addr, User: "dwf", KnownHosts: known, Logf: t.Logf}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tun.RunWith(ctx, ln, signer)
	waitFor(t, "connection", tun.Connected)

	code, body := httpViaSOCKS(t, ln.Addr().String(), web.URL)
	if code != 200 || !strings.HasPrefix(body, "hello from") {
		t.Fatalf("through tunnel: %d %q", code, body)
	}
	if b, _ := os.ReadFile(known); !strings.Contains(string(b), "ssh-ed25519") {
		t.Fatal("host key wasn't pinned")
	}
	if srv.sessions != 0 {
		t.Fatal("the tunnel must never open a shell session")
	}

	// the server drops us: the tunnel must come back by itself
	srv.dropAll()
	waitFor(t, "reconnect", func() bool { return tun.Connects() >= 2 && tun.Connected() })
	if code, _ = httpViaSOCKS(t, ln.Addr().String(), web.URL); code != 200 {
		t.Fatal("tunnel didn't recover")
	}

	// the server goes away for good: requests must fail fast, not hang
	srv.ln.Close()
	srv.dropAll()
	waitFor(t, "disconnect", func() bool { return !tun.Connected() })
	start := time.Now()
	if code, _ = httpViaSOCKS(t, ln.Addr().String(), web.URL); code != 0 {
		t.Fatal("requests should fail while disconnected")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("failure took %s; should be immediate", time.Since(start))
	}
}

func TestChangedHostKeyIsRefused(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id")
	GenerateKey(keyPath, "t")
	signer, _ := LoadKey(keyPath)
	known := filepath.Join(dir, "known_hosts")

	srv1, _ := startServer(t, signer.PublicKey())
	tun := &Tunnel{Addr: srv1.addr, User: "dwf", KnownHosts: known}
	c, err := tun.connect(context.Background(), signer)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	srv1.ln.Close()

	// a different server (new host key) on the same address
	srv2, _ := startServerAt(t, srv1.addr, signer.PublicKey())
	_ = srv2
	if _, err := tun.connect(context.Background(), signer); err == nil || !strings.Contains(err.Error(), "CHANGED") {
		t.Fatalf("expected a changed-key error, got %v", err)
	}
}

func startServerAt(t *testing.T, addr string, clientPub ssh.PublicKey) (*testServer, ssh.Signer) {
	t.Helper()
	var s *testServer
	var sg ssh.Signer
	deadline := time.Now().Add(5 * time.Second)
	for {
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			ln.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Skip("port not reusable on this OS")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// reuse startServer's logic on a fixed address
	_, hk, _ := ed25519.GenerateKey(rand.Reader)
	sg, _ = ssh.NewSignerFromKey(hk)
	cfg := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) { return nil, nil }}
	cfg.AddHostKey(sg)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	s = &testServer{addr: addr, ln: ln}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(c, cfg)
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return s, sg
}

func TestPassphraseKeyIsRejected(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	block, _ := ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte("secret"))
	p := filepath.Join(t.TempDir(), "k")
	os.WriteFile(p, pemEncode(block), 0o600)
	if _, err := LoadKey(p); err == nil || !strings.Contains(err.Error(), "passphrase") {
		t.Fatalf("got %v", err)
	}
}
