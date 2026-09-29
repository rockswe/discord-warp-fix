// Package serversetup logs in to the user's server as an admin and runs
// setup-server.sh, which creates the forwarding-only tunnel user.
package serversetup

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rockswe/discord-warp-fix/internal/paths"
	"github.com/rockswe/discord-warp-fix/internal/tunnel"
	"github.com/rockswe/discord-warp-fix/server"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// Options for Prepare.
type Options struct {
	Host        string
	Port        int
	Admin       string // login with root or sudo rights
	TunnelUser  string
	PubKey      string // authorized_keys line for the tunnel user
	KnownHosts  string
	Interactive bool
	In          *bufio.Reader
	Out         io.Writer
	// ReadPassword reads a secret without echoing it. Nil disables
	// password logins.
	ReadPassword func(prompt string) (string, error)
}

var userRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// ShellQuote wraps s in single quotes for a POSIX shell.
func ShellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Prepare runs setup-server.sh on the server.
func Prepare(o Options) error {
	if !userRe.MatchString(o.TunnelUser) {
		return fmt.Errorf("tunnel user %q may only contain a-z, 0-9, _ and -", o.TunnelUser)
	}
	if !strings.HasPrefix(o.PubKey, "ssh-") && !strings.HasPrefix(o.PubKey, "ecdsa-") {
		return errors.New("public key looks wrong")
	}
	addr := net.JoinHostPort(o.Host, strconv.Itoa(o.Port))
	var password string
	auth := authMethods(o, &password)
	cfg := &ssh.ClientConfig{
		User:            o.Admin,
		Auth:            auth,
		HostKeyCallback: tunnel.TOFU(o.KnownHosts, confirmHost(o)),
		Timeout:         20 * time.Second,
	}
	c, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return fmt.Errorf("can't log in to %s as %s: %w", addr, o.Admin, err)
	}
	defer c.Close()

	path, err := output(c, `f=$(mktemp /tmp/dwf-setup.XXXXXX) && cat > "$f" && echo "$f"`, bytes.NewReader(server.Script))
	if err != nil {
		return fmt.Errorf("couldn't upload the setup script: %w", err)
	}
	path = strings.TrimSpace(path)
	uid, err := output(c, "id -u", nil)
	if err != nil {
		return err
	}
	script := fmt.Sprintf("env DWF_USER=%s bash %s %s", ShellQuote(o.TunnelUser), ShellQuote(path), ShellQuote(o.PubKey))
	var stdin io.Reader
	switch {
	case strings.TrimSpace(uid) == "0":
	case run(c, "sudo -n true", nil, io.Discard) == nil:
		script = "sudo -n " + script
	default:
		if password == "" {
			if o.ReadPassword == nil || !o.Interactive {
				return fmt.Errorf("%s needs a sudo password; run dwf setup interactively or log in as root", o.Admin)
			}
			if password, err = o.ReadPassword(fmt.Sprintf("[sudo] password for %s: ", o.Admin)); err != nil {
				return err
			}
		}
		script = "sudo -S -p '' " + script
		stdin = strings.NewReader(password + "\n")
	}
	full := fmt.Sprintf("%s; rc=$?; rm -f %s; exit $rc", script, ShellQuote(path))
	if err := run(c, full, stdin, o.Out); err != nil {
		return fmt.Errorf("server setup failed: %w", err)
	}
	return nil
}

func authMethods(o Options, password *string) []ssh.AuthMethod {
	var methods []ssh.AuthMethod
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
		}
	}
	var signers []ssh.Signer
	for _, name := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
		if b, err := os.ReadFile(filepath.Join(paths.Home(), ".ssh", name)); err == nil {
			if s, err := ssh.ParsePrivateKey(b); err == nil {
				signers = append(signers, s)
			}
		}
	}
	if len(signers) > 0 {
		methods = append(methods, ssh.PublicKeys(signers...))
	}
	if o.Interactive && o.ReadPassword != nil {
		ask := func() (string, error) {
			if *password != "" {
				return *password, nil
			}
			p, err := o.ReadPassword(fmt.Sprintf("%s@%s's password: ", o.Admin, o.Host))
			*password = p
			return p, err
		}
		methods = append(methods, ssh.PasswordCallback(ask),
			ssh.KeyboardInteractive(func(_, _ string, qs []string, _ []bool) ([]string, error) {
				ans := make([]string, len(qs))
				for i := range qs {
					p, err := ask()
					if err != nil {
						return nil, err
					}
					ans[i] = p
				}
				return ans, nil
			}))
	}
	return methods
}

// confirmHost asks like OpenSSH does on first contact.
func confirmHost(o Options) tunnel.ConfirmFunc {
	return func(host string, remote net.Addr, key ssh.PublicKey) bool {
		fp := ssh.FingerprintSHA256(key)
		if !o.Interactive {
			fmt.Fprintf(o.Out, "trusting %s on first use (%s key %s)\n", host, key.Type(), fp)
			return true
		}
		fmt.Fprintf(o.Out, "The authenticity of host '%s (%s)' can't be established.\n%s key fingerprint is %s.\n",
			host, remote, strings.ToUpper(strings.TrimPrefix(key.Type(), "ssh-")), fp)
		for {
			fmt.Fprint(o.Out, "Are you sure you want to continue connecting (yes/no/[fingerprint])? ")
			line, err := o.In.ReadString('\n')
			ans := strings.TrimSpace(line)
			switch {
			case ans == "yes" || ans == fp:
				return true
			case ans == "no" || err != nil:
				return false
			}
			fmt.Fprintln(o.Out, "Please type 'yes', 'no' or the fingerprint.")
		}
	}
}

func run(c *ssh.Client, cmd string, stdin io.Reader, out io.Writer) error {
	s, err := c.NewSession()
	if err != nil {
		return err
	}
	defer s.Close()
	s.Stdin, s.Stdout, s.Stderr = stdin, out, out
	return s.Run(cmd)
}

func output(c *ssh.Client, cmd string, stdin io.Reader) (string, error) {
	var b bytes.Buffer
	err := run(c, cmd, stdin, &b)
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(b.String()))
	}
	return b.String(), nil
}
