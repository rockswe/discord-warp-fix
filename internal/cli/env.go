package cli

import (
	"context"
	"time"

	"github.com/rockswe/discord-warp-fix/internal/config"
	"github.com/rockswe/discord-warp-fix/internal/discord"
	"github.com/rockswe/discord-warp-fix/internal/logtail"
	"github.com/rockswe/discord-warp-fix/internal/logx"
	"github.com/rockswe/discord-warp-fix/internal/netcheck"
	"github.com/rockswe/discord-warp-fix/internal/notify"
	"github.com/rockswe/discord-warp-fix/internal/tunnel"
	"github.com/rockswe/discord-warp-fix/internal/warp"
	"github.com/rockswe/discord-warp-fix/internal/watchdog"
)

// realEnv connects the watchdog to the actual machine.
type realEnv struct {
	cfg  config.Config
	tail *logtail.Tailer
	tun  *tunnel.Tunnel // nil unless this process runs the tunnel
}

func ctx10() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

func (e *realEnv) Now() time.Time        { return time.Now() }
func (e *realEnv) PollLog() (int, error) { return e.tail.Poll() }

func (e *realEnv) Discord() *watchdog.Discord {
	p, err := discord.Find(e.cfg.DiscordApp)
	if err != nil || p == nil {
		return nil
	}
	return &watchdog.Discord{PID: p.PID, Proxied: p.Proxied(), Age: p.Age(time.Now())}
}

func (e *realEnv) Reroll() (string, string, error) {
	c, err := warp.Find()
	if err != nil {
		return "", "", err
	}
	return warp.Reroll(c, e.ExitIP, e.cfg.RerollAttempts, logx.Printf)
}

func (e *realEnv) ExitIP() string {
	ctx, cancel := ctx10()
	defer cancel()
	return netcheck.ExitIP(ctx)
}

func (e *realEnv) ProxyHealthy() bool {
	ctx, cancel := ctx10()
	defer cancel()
	return netcheck.ProxyHealthy(ctx, e.cfg.ProxyFor())
}

func (e *realEnv) ProxyExitIP() string {
	ctx, cancel := ctx10()
	defer cancel()
	return netcheck.ProxyExitIP(ctx, e.cfg.ProxyFor())
}

func (e *realEnv) Launch(throughProxy bool) error {
	url := ""
	if throughProxy {
		url = e.cfg.ProxyFor()
	}
	return discord.Relaunch(e.cfg.DiscordApp, url)
}

func (e *realEnv) RestartTunnel() {
	if e.tun != nil {
		e.tun.Reconnect()
	}
}

func (e *realEnv) Notify(msg string) {
	logx.Printf("notify: %s", msg)
	if e.cfg.Notify {
		notify.Send(msg)
	}
}

func (e *realEnv) Logf(format string, args ...any) { logx.Printf(format, args...) }
