// Package cli implements the dwf commands.
package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rockswe/discord-warp-fix/internal/config"
	"github.com/rockswe/discord-warp-fix/internal/discord"
	"github.com/rockswe/discord-warp-fix/internal/logtail"
	"github.com/rockswe/discord-warp-fix/internal/logx"
	"github.com/rockswe/discord-warp-fix/internal/netcheck"
	"github.com/rockswe/discord-warp-fix/internal/paths"
	"github.com/rockswe/discord-warp-fix/internal/serversetup"
	"github.com/rockswe/discord-warp-fix/internal/service"
	"github.com/rockswe/discord-warp-fix/internal/tunnel"
	"github.com/rockswe/discord-warp-fix/internal/warp"
	"github.com/rockswe/discord-warp-fix/internal/watchdog"
	"github.com/rockswe/discord-warp-fix/server"
	"golang.org/x/term"
)

type app struct {
	cfg     config.Config
	version string
	in      *bufio.Reader
	out     io.Writer
	tty     bool
}

// Main runs dwf with args (without the program name) and returns the exit code.
func Main(args []string, version string) int {
	logx.Init(paths.LogFile(), false)
	a := &app{version: version, in: bufio.NewReader(os.Stdin), out: os.Stdout,
		tty: term.IsTerminal(int(os.Stdin.Fd()))}
	cmd := "help"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	var err error
	if cmd != "help" && cmd != "-h" && cmd != "--help" && cmd != "version" && cmd != "--version" {
		if a.cfg, err = config.Load(paths.ConfigFile()); err != nil {
			fmt.Fprintln(os.Stderr, "dwf:", err)
			return 1
		}
	}
	switch cmd {
	case "setup":
		err = a.setup(args)
	case "install":
		err = a.install()
	case "uninstall":
		err = a.uninstall(args)
	case "status":
		err = a.status()
	case "check":
		err = a.check()
	case "launch":
		err = a.launch(args)
	case "reroll":
		err = a.reroll(args)
	case "logs":
		err = a.logs(args)
	case "run":
		err = a.run(args)
	case "watch":
		err = a.watchOnly()
	case "tunnel":
		err = a.tunnelOnly()
	case "version", "--version":
		fmt.Println(version)
	case "help", "-h", "--help":
		a.usage()
	default:
		a.usage()
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "dwf:", err)
		return 1
	}
	return 0
}

func (a *app) say(format string, args ...any) { fmt.Fprintf(a.out, format+"\n", args...) }

func (a *app) usage() {
	a.say(`discord-warp-fix %s: keep Discord working behind Cloudflare WARP's shared exit IPs

Usage: dwf <command>

  setup [--mode M ...]  choose a mode and write the config (interactive)
  install               start dwf in the background, now and at every login
  uninstall [--purge]   stop and remove it (and the config with --purge)
  status                show WARP, Discord, proxy and service state
  check                 test the configured mode end to end
  launch [--direct]     relaunch Discord through the proxy (or without it)
  reroll [N]            move WARP to a different exit IP now
  logs [N]              show the last N lines of dwf's log
  run                   the background service itself (used by install)
  watch | tunnel        run only the watchdog or only the tunnel, in the foreground
  version

Config: %s`, a.version, paths.ConfigFile())
}

func (a *app) logPath() string {
	if a.cfg.DiscordLog != "" {
		return a.cfg.DiscordLog
	}
	return discord.LogPath(a.cfg.DiscordApp)
}

func (a *app) ask(question, def string) string {
	if !a.tty {
		return def
	}
	fmt.Fprintf(a.out, "%s [%s]: ", question, def)
	line, _ := a.in.ReadString('\n')
	if line = strings.TrimSpace(line); line != "" {
		return line
	}
	return def
}

func readPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return string(b), err
}

// ---------------------------------------------------------------- setup ---

func (a *app) setup(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	mode := fs.String("mode", "", "reroll, tunnel or proxy")
	host := fs.String("host", "", "tunnel server address")
	user := fs.String("user", "", "tunnel user on the server")
	port := fs.Int("port", 0, "tunnel server SSH port")
	key := fs.String("key", "", "SSH private key for the tunnel")
	socks := fs.Int("socks-port", 0, "local SOCKS port")
	proxy := fs.String("proxy", "", "proxy URL for proxy mode")
	admin := fs.String("admin", "", "admin login used to prepare the server")
	yes := fs.Bool("yes", false, "don't ask questions")
	fs.BoolVar(yes, "y", false, "don't ask questions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c := a.cfg
	set := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	set(&c.Mode, *mode)
	set(&c.SSHHost, *host)
	set(&c.SSHUser, *user)
	set(&c.SSHKey, *key)
	set(&c.ProxyURL, *proxy)
	if *port > 0 {
		c.SSHPort = *port
	}
	if *socks > 0 {
		c.SocksPort = *socks
	}
	interactive := a.tty && !*yes
	if !interactive {
		a.tty = false
	}

	if interactive {
		a.say("Modes:")
		a.say("  reroll  free. Moves WARP to another shared exit IP when Discord gets rate-limited.")
		a.say("  tunnel  complete fix. Sends Discord through an SSH tunnel to a server you control.")
		a.say("  proxy   sends Discord through a SOCKS5/HTTP proxy you already have.")
		c.Mode = a.ask("Mode", c.Mode)
	}
	switch c.Mode {
	case config.ModeReroll:
	case config.ModeProxy:
		c.ProxyURL = a.ask("Proxy URL (socks5://host:port or http://host:port)", c.ProxyURL)
		if c.ProxyURL == "" {
			return errors.New("proxy mode needs a proxy URL")
		}
	case config.ModeTunnel:
		c.SSHHost = a.ask("Server address (IP or hostname)", c.SSHHost)
		if p, err := strconv.Atoi(a.ask("Server SSH port", strconv.Itoa(c.SSHPort))); err == nil {
			c.SSHPort = p
		}
		c.SSHUser = a.ask("Tunnel user on the server", c.SSHUser)
		if c.SSHHost == "" {
			return errors.New("tunnel mode needs a server address")
		}
		if err := a.prepareServer(c, *admin, interactive); err != nil {
			return err
		}
	default:
		return fmt.Errorf("mode must be reroll, tunnel or proxy, not %q", c.Mode)
	}
	if err := c.Validate(); err != nil {
		return err
	}
	if err := c.Save(paths.ConfigFile()); err != nil {
		return err
	}
	a.cfg = c
	a.say("saved %s", paths.ConfigFile())
	if err := a.check(); err != nil {
		a.say("%v", err)
	}
	a.say("")
	a.say("Next: dwf install")
	if c.Mode != config.ModeReroll {
		a.say("Then: dwf launch   (relaunches Discord through the proxy)")
	}
	return nil
}

func (a *app) prepareServer(c config.Config, admin string, interactive bool) error {
	pub, err := tunnel.PublicKeyLine(c.SSHKey)
	if err != nil {
		host, _ := os.Hostname()
		if pub, err = tunnel.GenerateKey(c.SSHKey, "discord-warp-fix@"+strings.Split(host, ".")[0]); err != nil {
			return fmt.Errorf("couldn't create an SSH key: %w", err)
		}
		a.say("created SSH key %s", c.SSHKey)
	}
	if admin == "" && interactive {
		admin = a.ask("Admin login on the server to prepare it now (e.g. root), or leave empty to do it yourself", "")
	}
	if admin == "" {
		script := filepath.Join(paths.ConfigDir(), "setup-server.sh")
		if err := os.MkdirAll(paths.ConfigDir(), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(script, server.Script, 0o755); err != nil {
			return err
		}
		a.say("")
		a.say("Prepare the server yourself: copy %s to it and run as root:", script)
		a.say("  sudo env DWF_USER=%s bash setup-server.sh '%s'", c.SSHUser, pub)
		a.say("")
		return nil
	}
	a.say("preparing %s as %s...", c.SSHHost, admin)
	return serversetup.Prepare(serversetup.Options{
		Host: c.SSHHost, Port: c.SSHPort, Admin: admin, TunnelUser: c.SSHUser, PubKey: pub,
		KnownHosts: paths.KnownHostsFile(), Interactive: interactive, In: a.in, Out: a.out,
		ReadPassword: readPassword,
	})
}

// ---------------------------------------------------------- install etc ---

func (a *app) install() error {
	if err := a.cfg.Validate(); err != nil {
		return err
	}
	if _, err := os.Stat(paths.ConfigFile()); errors.Is(err, os.ErrNotExist) {
		if err := a.cfg.Save(paths.ConfigFile()); err != nil {
			return err
		}
	}
	exe, err := service.Executable()
	if err != nil {
		return err
	}
	if err := service.Install(exe); err != nil {
		return err
	}
	logx.Printf("installed %s (mode=%s)", service.Describe(), a.cfg.Mode)
	a.say("installed and started (mode: %s). dwf now runs in the background and starts at login.", a.cfg.Mode)
	return nil
}

func (a *app) uninstall(args []string) error {
	if err := service.Uninstall(); err != nil {
		return err
	}
	a.say("dwf stopped and removed from login items.")
	if len(args) > 0 && args[0] == "--purge" {
		_ = os.RemoveAll(paths.ConfigDir())
		_ = os.RemoveAll(paths.StateDir())
		a.say("config and state removed.")
	}
	return nil
}

func (a *app) logs(args []string) error {
	n := 40
	if len(args) > 0 {
		if v, err := strconv.Atoi(args[0]); err == nil && v > 0 {
			n = v
		}
	}
	b, err := os.ReadFile(paths.LogFile())
	if err != nil {
		a.say("no log yet at %s", paths.LogFile())
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	a.say("%s", strings.Join(lines, "\n"))
	return nil
}

// --------------------------------------------------------------- status ---

var stampRe = regexp.MustCompile(`^\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})`)

// recentErrors counts rate-limit lines Discord logged in the last d.
func recentErrors(path string, d time.Duration) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil && fi.Size() > 4<<20 {
		_, _ = f.Seek(fi.Size()-4<<20, io.SeekStart)
	}
	since := time.Now().Add(-d)
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		m := stampRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		t, err := time.ParseInLocation("2006-01-02 15:04:05", m[1], time.Local)
		if err == nil && t.After(since) && logtail.DefaultPattern.MatchString(line) {
			n++
		}
	}
	return n
}

func (a *app) status() error {
	env := &realEnv{cfg: a.cfg}
	a.say("discord-warp-fix %s  (mode: %s, config: %s)", a.version, a.cfg.Mode, paths.ConfigFile())
	a.say("")
	if w, err := warp.Find(); err == nil {
		ip := env.ExitIP()
		if ip == "" {
			ip = "unreachable"
		}
		a.say("WARP        %s, protocol %s, exit IP %s", orUnknown(w.Status()), orUnknown(w.Protocol()), ip)
	} else {
		a.say("WARP        %v", err)
	}
	switch p, _ := discord.Find(a.cfg.DiscordApp); {
	case p == nil:
		a.say("Discord     not running")
	case p.Proxied():
		a.say("Discord     running through %s", p.ProxyArg())
	default:
		a.say("Discord     running directly (no proxy)")
	}
	a.say("Errors      %d rate-limit errors in Discord's log in the last 10 min", recentErrors(a.logPath(), 10*time.Minute))
	if a.cfg.Mode != config.ModeReroll {
		if env.ProxyHealthy() {
			a.say("Proxy       %s reaches Discord, exit IP %s", a.cfg.ProxyFor(), orUnknown(env.ProxyExitIP()))
		} else {
			a.say("Proxy       %s is NOT reaching Discord", a.cfg.ProxyFor())
		}
	}
	switch pid, ok := service.Running(); {
	case ok:
		a.say("Service     running (pid %d)", pid)
	case service.Installed():
		a.say("Service     installed but not running. Check 'dwf logs'")
	default:
		a.say("Service     not installed. Run: dwf install")
	}
	a.say("Log         %s", paths.LogFile())
	return nil
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

// ---------------------------------------------------------------- check ---

func (a *app) check() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	warpIP := netcheck.ExitIP(ctx)
	switch a.cfg.Mode {
	case config.ModeReroll:
		if warpIP == "" {
			return errors.New("can't reach Cloudflare. Is WARP connected?")
		}
		a.say("WARP exit IP: %s", warpIP)
		return nil
	case config.ModeProxy:
		if !netcheck.ProxyHealthy(ctx, a.cfg.ProxyURL) {
			return fmt.Errorf("proxy %s does NOT reach Discord", a.cfg.ProxyURL)
		}
		a.say("proxy reaches Discord, exit IP %s", orUnknown(netcheck.ProxyExitIP(ctx, a.cfg.ProxyURL)))
		return nil
	}
	proxy := a.cfg.ProxyFor()
	if _, running := service.Running(); !running || !netcheck.ProxyHealthy(ctx, proxy) {
		var err error
		if proxy, err = a.tempTunnel(ctx); err != nil {
			return fmt.Errorf("tunnel test FAILED: %v", err)
		}
	}
	via := netcheck.ProxyExitIP(ctx, proxy)
	a.say("tunnel works. Discord would leave from %s (WARP's shared exit is %s).", orUnknown(via), orUnknown(warpIP))
	if via != "" && via == warpIP {
		a.say("warning: that's the same IP as WARP's shared exit, so this won't avoid the rate limit. Use a server outside this network.")
	}
	return nil
}

// tempTunnel starts a short-lived tunnel on a free port and returns its
// proxy URL once it reaches Discord.
func (a *app) tempTunnel(ctx context.Context) (string, error) {
	a.say("testing SSH tunnel to %s@%s:%d ...", a.cfg.SSHUser, a.cfg.SSHHost, a.cfg.SSHPort)
	signer, err := tunnel.LoadKey(a.cfg.SSHKey)
	if err != nil {
		return "", err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	lastErr := errors.New("no connection yet")
	t := &tunnel.Tunnel{
		Addr: net.JoinHostPort(a.cfg.SSHHost, strconv.Itoa(a.cfg.SSHPort)), User: a.cfg.SSHUser,
		KnownHosts: paths.KnownHostsFile(),
		Logf: func(f string, args ...any) {
			if strings.Contains(f, "can't connect") {
				lastErr = fmt.Errorf("%v", args[1])
			}
		},
	}
	go t.RunWith(ctx, ln, signer)
	proxy := "socks5://" + ln.Addr().String()
	for ctx.Err() == nil {
		if t.Connected() && netcheck.ProxyHealthy(ctx, proxy) {
			return proxy, nil
		}
		if t.Connected() {
			lastErr = errors.New("connected, but the server can't reach Discord")
		}
		time.Sleep(500 * time.Millisecond)
	}
	return "", lastErr
}

// --------------------------------------------------------------- launch ---

func (a *app) launch(args []string) error {
	fs := flag.NewFlagSet("launch", flag.ContinueOnError)
	direct := fs.Bool("direct", false, "relaunch without a proxy")
	force := fs.Bool("force", false, "don't wait for the proxy to work")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *direct {
		if err := discord.Relaunch(a.cfg.DiscordApp, ""); err != nil {
			return err
		}
		logx.Printf("launched %s directly", a.cfg.DiscordApp)
		a.say("Discord relaunched without a proxy.")
		return nil
	}
	url := a.cfg.ProxyFor()
	if url == "" {
		return fmt.Errorf("launch needs tunnel or proxy mode (current mode: %s). Use --direct for a plain relaunch", a.cfg.Mode)
	}
	if !*force {
		ok := false
		for i := 0; i < 15 && !ok; i++ {
			ctx, cancel := ctx10()
			ok = netcheck.ProxyHealthy(ctx, url)
			cancel()
			if !ok {
				time.Sleep(2 * time.Second)
			}
		}
		if !ok {
			return fmt.Errorf("the proxy at %s isn't reaching Discord. Check 'dwf status', or use --force", url)
		}
	}
	if err := discord.Relaunch(a.cfg.DiscordApp, url); err != nil {
		return err
	}
	logx.Printf("launched %s via %s", a.cfg.DiscordApp, url)
	a.say("Discord relaunched through %s", url)
	return nil
}

func (a *app) reroll(args []string) error {
	n := a.cfg.RerollAttempts
	if len(args) > 0 {
		if v, err := strconv.Atoi(args[0]); err == nil && v > 0 {
			n = v
		}
	}
	w, err := warp.Find()
	if err != nil {
		return err
	}
	env := &realEnv{cfg: a.cfg}
	logx.Init(paths.LogFile(), true)
	before, after, err := warp.Reroll(w, env.ExitIP, n, logx.Printf)
	if err != nil {
		return err
	}
	logx.Printf("reroll success: %s -> %s", before, after)
	return nil
}

// ------------------------------------------------------------- services ---

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func (a *app) startTunnel(ctx context.Context) (*tunnel.Tunnel, error) {
	t := &tunnel.Tunnel{
		Addr: net.JoinHostPort(a.cfg.SSHHost, strconv.Itoa(a.cfg.SSHPort)), User: a.cfg.SSHUser,
		KeyPath: a.cfg.SSHKey, KnownHosts: paths.KnownHostsFile(),
		Listen: net.JoinHostPort("127.0.0.1", strconv.Itoa(a.cfg.SocksPort)), Logf: logx.Printf,
	}
	signer, err := tunnel.LoadKey(t.KeyPath)
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", t.Listen)
	if err != nil {
		return nil, fmt.Errorf("can't listen on %s (is another dwf or proxy using it?): %w", t.Listen, err)
	}
	go func() { _ = t.RunWith(ctx, ln, signer) }()
	return t, nil
}

func lastActionFile() string { return filepath.Join(paths.StateDir(), "last_action") }

func (a *app) watchLoop(ctx context.Context, tun *tunnel.Tunnel) {
	var last time.Time
	if b, err := os.ReadFile(lastActionFile()); err == nil {
		if s, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); err == nil {
			last = time.Unix(s, 0)
		}
	}
	env := &realEnv{cfg: a.cfg, tail: &logtail.Tailer{Path: a.logPath()}, tun: tun}
	w := watchdog.New(a.cfg, env, last)
	logx.Printf("watchdog %s started (mode=%s threshold=%d/%ds cooldown=%ds log=%s)",
		a.version, a.cfg.Mode, a.cfg.Threshold, a.cfg.Window, a.cfg.Cooldown, a.logPath())
	tick := time.NewTicker(time.Duration(a.cfg.Poll) * time.Second)
	defer tick.Stop()
	for {
		if w.Tick() {
			_ = os.MkdirAll(paths.StateDir(), 0o755)
			_ = os.WriteFile(lastActionFile(), []byte(strconv.FormatInt(time.Now().Unix(), 10)), 0o644)
		}
		select {
		case <-ctx.Done():
			logx.Printf("watchdog stopped")
			return
		case <-tick.C:
		}
	}
}

// run is the background service: tunnel (in tunnel mode) plus watchdog.
func (a *app) run(args []string) error {
	if len(args) > 0 && args[0] == "--background" {
		service.Detach()
	}
	if err := a.cfg.Validate(); err != nil {
		logx.Printf("can't start: %v", err)
		return err
	}
	ctx, cancel := signalContext()
	defer cancel()
	for {
		err := service.Claim()
		if err == nil {
			break
		}
		if !errors.Is(err, service.ErrAlreadyRunning) {
			return err
		}
		select { // another instance owns the job; take over if it exits
		case <-ctx.Done():
			return nil
		case <-time.After(30 * time.Second):
		}
	}
	defer service.Release()
	var tun *tunnel.Tunnel
	if a.cfg.Mode == config.ModeTunnel {
		var err error
		if tun, err = a.startTunnel(ctx); err != nil {
			logx.Printf("tunnel: %v", err)
			return err
		}
	}
	a.watchLoop(ctx, tun)
	return nil
}

func (a *app) watchOnly() error {
	logx.Init(paths.LogFile(), true)
	ctx, cancel := signalContext()
	defer cancel()
	a.watchLoop(ctx, nil)
	return nil
}

func (a *app) tunnelOnly() error {
	if a.cfg.Mode != config.ModeTunnel {
		return fmt.Errorf("tunnel only runs in tunnel mode (current mode: %s)", a.cfg.Mode)
	}
	if err := a.cfg.Validate(); err != nil {
		return err
	}
	logx.Init(paths.LogFile(), true)
	ctx, cancel := signalContext()
	defer cancel()
	if _, err := a.startTunnel(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}
