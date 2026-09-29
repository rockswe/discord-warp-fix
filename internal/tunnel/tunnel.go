// Package tunnel keeps an SSH connection to the user's server open and
// exposes it as a local SOCKS5 proxy, like `ssh -N -D`.
package tunnel

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Tunnel is an SSH connection plus the local SOCKS5 listener.
type Tunnel struct {
	Addr       string // server host:port
	User       string
	KeyPath    string
	KnownHosts string
	Listen     string // e.g. 127.0.0.1:1080
	Logf       func(string, ...any)

	mu       sync.Mutex
	client   *ssh.Client
	connects int
}

func (t *Tunnel) logf(f string, a ...any) {
	if t.Logf != nil {
		t.Logf(f, a...)
	}
}

// Connected reports whether the SSH connection is up.
func (t *Tunnel) Connected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.client != nil
}

// Connects counts successful connections since Run started.
func (t *Tunnel) Connects() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.connects
}

// Reconnect drops the current connection so Run makes a fresh one.
func (t *Tunnel) Reconnect() {
	t.mu.Lock()
	c := t.client
	t.mu.Unlock()
	if c != nil {
		c.Close()
	}
}

func (t *Tunnel) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	t.mu.Lock()
	c := t.client
	t.mu.Unlock()
	if c == nil {
		return nil, errNotConnected
	}
	return c.DialContext(ctx, network, addr)
}

// Run listens for SOCKS clients and keeps the SSH connection alive until
// ctx ends. The listener stays up while SSH reconnects, so Discord gets
// fast failures instead of hanging.
func (t *Tunnel) Run(ctx context.Context) error {
	signer, err := LoadKey(t.KeyPath)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", t.Listen)
	if err != nil {
		return fmt.Errorf("can't listen on %s: %w", t.Listen, err)
	}
	return t.RunWith(ctx, ln, signer)
}

// RunWith is Run with an existing listener and key.
func (t *Tunnel) RunWith(ctx context.Context, ln net.Listener, signer ssh.Signer) error {
	go func() {
		if err := ServeSOCKS5(ctx, ln, t.dial, nil); err != nil {
			t.logf("tunnel: SOCKS listener stopped: %v", err)
		}
	}()
	backoff := 2 * time.Second
	for ctx.Err() == nil {
		c, err := t.connect(ctx, signer)
		if err != nil {
			t.logf("tunnel: can't connect to %s: %v (retrying in %s)", t.Addr, err, backoff)
			select {
			case <-ctx.Done():
			case <-time.After(backoff):
			}
			if backoff *= 2; backoff > time.Minute {
				backoff = time.Minute
			}
			continue
		}
		backoff = 2 * time.Second
		t.mu.Lock()
		t.client = c
		t.connects++
		t.mu.Unlock()
		t.logf("tunnel: connected to %s@%s, SOCKS on %s", t.User, t.Addr, ln.Addr())

		done := make(chan struct{})
		go t.keepalive(c, done)
		go func() {
			select {
			case <-ctx.Done():
				c.Close()
			case <-done:
			}
		}()
		err = c.Wait()
		close(done)
		t.mu.Lock()
		t.client = nil
		t.mu.Unlock()
		if ctx.Err() == nil {
			t.logf("tunnel: connection lost: %v", err)
			select { // don't spin if the server keeps dropping us
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
		}
	}
	return nil
}

func (t *Tunnel) connect(ctx context.Context, signer ssh.Signer) (*ssh.Client, error) {
	cfg := &ssh.ClientConfig{
		User:            t.User,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: TOFU(t.KnownHosts, nil),
		Timeout:         15 * time.Second,
	}
	d := net.Dialer{Timeout: 15 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", t.Addr)
	if err != nil {
		return nil, err
	}
	sc, chans, reqs, err := ssh.NewClientConn(conn, t.Addr, cfg)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return ssh.NewClient(sc, chans, reqs), nil
}

// keepalive closes the connection when the server stops answering, the
// same job as ssh's ServerAliveInterval=15 ServerAliveCountMax=3.
func (t *Tunnel) keepalive(c *ssh.Client, done <-chan struct{}) {
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	misses := 0
	for {
		select {
		case <-done:
			return
		case <-tick.C:
		}
		res := make(chan error, 1)
		go func() {
			_, _, err := c.SendRequest("keepalive@openssh.com", true, nil)
			res <- err
		}()
		select {
		case err := <-res:
			if err != nil {
				c.Close()
				return
			}
			misses = 0
		case <-time.After(15 * time.Second):
			if misses++; misses >= 3 {
				t.logf("tunnel: server stopped answering keepalives")
				c.Close()
				return
			}
		}
	}
}

// LoadKey reads an unencrypted OpenSSH private key.
func LoadKey(path string) (ssh.Signer, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("can't read SSH key %s: %w", path, err)
	}
	s, err := ssh.ParsePrivateKey(b)
	var pp *ssh.PassphraseMissingError
	if errors.As(err, &pp) {
		return nil, fmt.Errorf("SSH key %s has a passphrase; dwf needs a key without one", path)
	}
	if err != nil {
		return nil, fmt.Errorf("can't parse SSH key %s: %w", path, err)
	}
	return s, nil
}

// GenerateKey writes a new ed25519 key pair (path and path.pub) and returns
// the public key line.
func GenerateKey(path, comment string) (string, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	block, err := ssh.MarshalPrivateKey(priv, comment)
	if err != nil {
		return "", err
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))) + " " + comment
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(path+".pub", []byte(line+"\n"), 0o644); err != nil {
		return "", err
	}
	return line, nil
}

// PublicKeyLine returns the authorized_keys line for a private key file,
// preferring the .pub file next to it so the comment is kept.
func PublicKeyLine(path string) (string, error) {
	if b, err := os.ReadFile(path + ".pub"); err == nil {
		return strings.TrimSpace(string(b)), nil
	}
	s, err := LoadKey(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(s.PublicKey()))), nil
}

// ConfirmFunc asks the user whether to trust an unknown host key.
type ConfirmFunc func(host string, remote net.Addr, key ssh.PublicKey) bool

// TOFU trusts a server the first time it's seen and pins its key in file,
// like ssh's StrictHostKeyChecking=accept-new. With confirm set, the user is
// asked first. A changed key is always refused.
func TOFU(file string, confirm ConfirmFunc) ssh.HostKeyCallback {
	return func(host string, remote net.Addr, key ssh.PublicKey) error {
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			return err
		}
		f, err := os.OpenFile(file, os.O_CREATE|os.O_RDONLY, 0o600)
		if err != nil {
			return err
		}
		f.Close()
		check, err := knownhosts.New(file)
		if err != nil {
			return err
		}
		err = check(host, remote, key)
		var ke *knownhosts.KeyError
		if !errors.As(err, &ke) {
			return err // nil (known and matching) or a real error
		}
		if len(ke.Want) > 0 {
			return fmt.Errorf("the host key for %s CHANGED. If you rebuilt the server, remove its line from %s", host, file)
		}
		if confirm != nil && !confirm(host, remote, key) {
			return errors.New("host key not accepted")
		}
		out, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = fmt.Fprintln(out, knownhosts.Line([]string{knownhosts.Normalize(host)}, key))
		return err
	}
}
