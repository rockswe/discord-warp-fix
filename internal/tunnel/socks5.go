package tunnel

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

// DialFunc opens an outbound connection, e.g. through SSH.
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// ServeSOCKS5 accepts SOCKS5 CONNECT requests on ln (no authentication; bind
// it to loopback only) and relays them through dial. Host names are passed
// through unresolved, so DNS happens on the far side.
func ServeSOCKS5(ctx context.Context, ln net.Listener, dial DialFunc, logf func(string, ...any)) error {
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go func() {
			if err := handleSOCKS(ctx, c, dial); err != nil && logf != nil {
				logf("socks: %v", err)
			}
		}()
	}
}

const (
	socksVersion   = 5
	cmdConnect     = 1
	atypIPv4       = 1
	atypDomain     = 3
	atypIPv6       = 4
	repSuccess     = 0
	repFailure     = 1
	repCmdNotSupp  = 7
	repAddrNotSupp = 8
)

func reply(c net.Conn, code byte) error {
	_, err := c.Write([]byte{socksVersion, code, 0, atypIPv4, 0, 0, 0, 0, 0, 0})
	return err
}

func handleSOCKS(ctx context.Context, c net.Conn, dial DialFunc) error {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(30 * time.Second))
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(c, hdr); err != nil {
		return err
	}
	if hdr[0] != socksVersion {
		return fmt.Errorf("not SOCKS5 (version %d)", hdr[0])
	}
	methods := make([]byte, hdr[1])
	if _, err := io.ReadFull(c, methods); err != nil {
		return err
	}
	if _, err := c.Write([]byte{socksVersion, 0}); err != nil { // no auth
		return err
	}
	req := make([]byte, 4)
	if _, err := io.ReadFull(c, req); err != nil {
		return err
	}
	if req[1] != cmdConnect {
		reply(c, repCmdNotSupp)
		return fmt.Errorf("unsupported command %d", req[1])
	}
	var host string
	switch req[3] {
	case atypIPv4, atypIPv6:
		n := 4
		if req[3] == atypIPv6 {
			n = 16
		}
		ip := make([]byte, n)
		if _, err := io.ReadFull(c, ip); err != nil {
			return err
		}
		host = net.IP(ip).String()
	case atypDomain:
		l := make([]byte, 1)
		if _, err := io.ReadFull(c, l); err != nil {
			return err
		}
		name := make([]byte, l[0])
		if _, err := io.ReadFull(c, name); err != nil {
			return err
		}
		host = string(name)
	default:
		reply(c, repAddrNotSupp)
		return fmt.Errorf("unsupported address type %d", req[3])
	}
	pb := make([]byte, 2)
	if _, err := io.ReadFull(c, pb); err != nil {
		return err
	}
	addr := net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(pb))))

	dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	up, err := dial(dctx, "tcp", addr)
	cancel()
	if err != nil {
		reply(c, repFailure)
		return fmt.Errorf("connect %s: %w", addr, err)
	}
	defer up.Close()
	if err := reply(c, repSuccess); err != nil {
		return err
	}
	_ = c.SetDeadline(time.Time{})
	pipe(c, up)
	return nil
}

func pipe(a, b net.Conn) {
	var wg sync.WaitGroup
	cp := func(dst, src net.Conn) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		} else {
			_ = dst.Close()
		}
	}
	wg.Add(2)
	go cp(a, b)
	go cp(b, a)
	wg.Wait()
}

// errNotConnected is returned while the SSH connection is down.
var errNotConnected = errors.New("SSH tunnel isn't connected")
