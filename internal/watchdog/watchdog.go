// Package watchdog is dwf's decision loop: it watches Discord's log for
// rate-limit bursts and keeps Discord on a working path.
//
// Everything it touches goes through Env, so the rules can be tested
// without WARP, Discord or a network.
package watchdog

import (
	"fmt"
	"time"

	"github.com/rockswe/discord-warp-fix/internal/burst"
	"github.com/rockswe/discord-warp-fix/internal/config"
)

// Discord is what the watchdog needs to know about the running app.
type Discord struct {
	PID     int32
	Proxied bool
	Age     time.Duration
}

// Env is the outside world.
type Env interface {
	Now() time.Time
	PollLog() (int, error)
	Discord() *Discord // nil when not running
	Reroll() (before, after string, err error)
	ExitIP() string
	ProxyHealthy() bool
	ProxyExitIP() string
	Launch(throughProxy bool) error
	RestartTunnel()
	Notify(msg string)
	Logf(format string, args ...any)
}

// Rules.
const (
	freshDiscord     = 180 * time.Second // a Discord this young was started by the OS or an update
	healthCheckEvery = 30 * time.Second
)

// Watchdog holds the loop's state between ticks.
type Watchdog struct {
	Cfg config.Config
	Env Env

	det            *burst.Detector
	relaunchedPID  int32
	lastHealth     time.Time
	proxyDownSince time.Time
	fellBack       bool
}

// New builds a watchdog. lastAction restores the cooldown after a restart.
func New(cfg config.Config, env Env, lastAction time.Time) *Watchdog {
	d := &burst.Detector{
		Threshold: cfg.Threshold,
		Window:    time.Duration(cfg.Window) * time.Second,
		Cooldown:  time.Duration(cfg.Cooldown) * time.Second,
	}
	d.SetLastAction(lastAction)
	return &Watchdog{Cfg: cfg, Env: env, det: d}
}

// Tick runs one pass. It returns true when it acted on a burst, so the
// caller can persist the time.
func (w *Watchdog) Tick() (acted bool) {
	now := w.Env.Now()
	n, err := w.Env.PollLog()
	if err != nil {
		w.Env.Logf("can't read Discord's log: %v", err)
	}
	w.det.Add(n, now)
	if w.det.Ready(now) {
		w.handleBurst(w.det.Count(now))
		w.det.Acted(now)
		acted = true
	}
	if w.Cfg.Mode != config.ModeReroll {
		w.checkProxyHealth(now)
		w.ensureProxiedLaunch()
	}
	return acted
}

func (w *Watchdog) handleBurst(count int) {
	w.Env.Logf("rate-limit burst: %d errors in %ds (mode=%s)", count, w.Cfg.Window, w.Cfg.Mode)
	switch w.Cfg.Mode {
	case config.ModeReroll:
		before, after, err := w.Env.Reroll()
		if err != nil {
			w.Env.Logf("reroll failed: %v", err)
			w.Env.Notify(fmt.Sprintf("Discord is rate-limited on WARP exit %s and no other exit was available. Will retry in %d min.", orUnknown(before), w.Cfg.Cooldown/60))
			return
		}
		w.Env.Logf("reroll success: %s -> %s", before, after)
		w.Env.Notify(fmt.Sprintf("Discord was rate-limited on WARP exit %s. Moved to %s.", before, after))
	default:
		d := w.Env.Discord()
		switch {
		case d != nil && !d.Proxied:
			w.Env.Notify("Discord is rate-limited because it isn't using your proxy. Run: dwf launch")
		case !w.Env.ProxyHealthy():
			w.Env.Logf("proxy unhealthy during burst")
			if w.Cfg.Mode == config.ModeTunnel {
				w.Env.RestartTunnel()
			}
			w.Env.Notify("Your Discord proxy isn't responding. Restarting the tunnel.")
		default:
			w.Env.Notify(fmt.Sprintf("Discord is rate-limited even through your proxy (%s). Check 'dwf status'.", orUnknown(w.Env.ProxyExitIP())))
		}
	}
}

// checkProxyHealth moves Discord to plain WARP when the proxy has been dead
// for FallbackAfter seconds, and says so when it comes back.
func (w *Watchdog) checkProxyHealth(now time.Time) {
	if now.Sub(w.lastHealth) < healthCheckEvery {
		return
	}
	w.lastHealth = now
	if w.Env.ProxyHealthy() {
		if !w.proxyDownSince.IsZero() {
			w.Env.Logf("proxy reaches Discord again")
			if w.fellBack {
				w.Env.Notify("Your Discord proxy is back. Run 'dwf launch' to use it again.")
			}
		}
		w.proxyDownSince, w.fellBack = time.Time{}, false
		return
	}
	d := w.Env.Discord()
	if d == nil || !d.Proxied {
		return
	}
	if w.proxyDownSince.IsZero() {
		w.proxyDownSince = now
		w.Env.Logf("proxy stopped reaching Discord")
		return
	}
	down := now.Sub(w.proxyDownSince)
	if w.fellBack || down < time.Duration(w.Cfg.FallbackAfter)*time.Second {
		return
	}
	w.fellBack = true
	if w.Cfg.FallbackAfter == 0 {
		w.Env.Notify("Your Discord proxy is down. Run 'dwf launch --direct' to use WARP directly.")
		return
	}
	w.Env.Logf("proxy down for %s, relaunching Discord directly through WARP", down.Round(time.Second))
	if err := w.Env.Launch(false); err != nil {
		w.Env.Logf("fallback relaunch failed: %v", err)
	}
	if nd := w.Env.Discord(); nd != nil {
		w.relaunchedPID = nd.PID // don't bounce it straight back onto the dead proxy
	}
	w.Env.Notify("Your Discord proxy has been down for a while, so Discord now runs directly through WARP.")
}

// ensureProxiedLaunch moves a freshly started Discord (login, self-update)
// onto the proxy, once per process.
func (w *Watchdog) ensureProxiedLaunch() {
	if !w.Cfg.AutoRelaunch {
		return
	}
	d := w.Env.Discord()
	if d == nil || d.Proxied || d.PID == w.relaunchedPID || d.Age > freshDiscord {
		return
	}
	w.relaunchedPID = d.PID
	if !w.Env.ProxyHealthy() {
		w.Env.Logf("fresh Discord started without proxy, but the proxy is down; leaving it alone")
		return
	}
	w.Env.Logf("fresh Discord (pid %d, %s old) started without proxy, relaunching", d.PID, d.Age.Round(time.Second))
	if err := w.Env.Launch(true); err != nil {
		w.Env.Logf("auto relaunch failed: %v", err)
		return
	}
	if nd := w.Env.Discord(); nd != nil {
		w.relaunchedPID = nd.PID
	}
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
