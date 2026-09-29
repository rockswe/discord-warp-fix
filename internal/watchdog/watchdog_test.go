package watchdog

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rockswe/erisim/internal/config"
)

type fakeEnv struct {
	now       time.Time
	logErrs   int
	discord   *Discord
	healthy   bool
	rerollErr error
	nextPID   int32

	rerolls, launches, restarts int
	launchedProxy               []bool
	notes, logs                 []string
}

func (f *fakeEnv) Now() time.Time        { return f.now }
func (f *fakeEnv) PollLog() (int, error) { n := f.logErrs; f.logErrs = 0; return n, nil }
func (f *fakeEnv) Discord() *Discord     { return f.discord }
func (f *fakeEnv) Reroll() (string, string, error) {
	f.rerolls++
	if f.rerollErr != nil {
		return "1.1.1.1", "1.1.1.1", f.rerollErr
	}
	return "1.1.1.1", "2.2.2.2", nil
}
func (f *fakeEnv) ExitIP() string          { return "1.1.1.1" }
func (f *fakeEnv) ProxyHealthy() bool      { return f.healthy }
func (f *fakeEnv) ProxyExitIP() string     { return "9.9.9.9" }
func (f *fakeEnv) RestartTunnel()          { f.restarts++ }
func (f *fakeEnv) Notify(m string)         { f.notes = append(f.notes, m) }
func (f *fakeEnv) Logf(s string, a ...any) { f.logs = append(f.logs, fmt.Sprintf(s, a...)) }
func (f *fakeEnv) Launch(proxy bool) error {
	f.launches++
	f.launchedProxy = append(f.launchedProxy, proxy)
	f.nextPID++
	f.discord = &Discord{PID: 1000 + f.nextPID, Proxied: proxy}
	return nil
}

func (f *fakeEnv) advance(d time.Duration) { f.now = f.now.Add(d) }

func cfg(mode string) config.Config {
	c := config.Default()
	c.Mode = mode
	return c
}

func lastNote(f *fakeEnv) string {
	if len(f.notes) == 0 {
		return ""
	}
	return f.notes[len(f.notes)-1]
}

func TestRerollOnBurstWithCooldown(t *testing.T) {
	f := &fakeEnv{now: time.Unix(1e9, 0)}
	w := New(cfg(config.ModeReroll), f, time.Time{})

	f.logErrs = 4
	if w.Tick() || f.rerolls != 0 {
		t.Fatal("4 errors shouldn't trigger")
	}
	f.advance(5 * time.Second)
	f.logErrs = 1
	if !w.Tick() || f.rerolls != 1 || !strings.Contains(lastNote(f), "Moved to 2.2.2.2") {
		t.Fatalf("burst should reroll: rerolls=%d notes=%v", f.rerolls, f.notes)
	}
	f.advance(60 * time.Second)
	f.logErrs = 20
	if w.Tick() || f.rerolls != 1 {
		t.Fatal("cooldown must suppress the next reroll")
	}
	f.advance(300 * time.Second)
	f.logErrs = 5
	if !w.Tick() || f.rerolls != 2 {
		t.Fatal("after the cooldown a new burst rerolls again")
	}
}

func TestRerollFailureNotifies(t *testing.T) {
	f := &fakeEnv{now: time.Unix(1e9, 0), rerollErr: errors.New("stuck")}
	w := New(cfg(config.ModeReroll), f, time.Time{})
	f.logErrs = 5
	w.Tick()
	if !strings.Contains(lastNote(f), "no other exit was available") {
		t.Fatalf("notes: %v", f.notes)
	}
}

func TestCooldownSurvivesRestart(t *testing.T) {
	f := &fakeEnv{now: time.Unix(1e9, 0)}
	w := New(cfg(config.ModeReroll), f, f.now.Add(-time.Minute))
	f.logErrs = 10
	if w.Tick() {
		t.Fatal("a reroll one minute ago should still be cooling down")
	}
}

func TestRerollModeNeverTouchesDiscord(t *testing.T) {
	f := &fakeEnv{now: time.Unix(1e9, 0), discord: &Discord{PID: 1, Age: time.Second}, healthy: true}
	w := New(cfg(config.ModeReroll), f, time.Time{})
	w.Tick()
	if f.launches != 0 {
		t.Fatal("reroll mode must not relaunch Discord")
	}
}

func TestFreshDiscordMovedOntoProxyOnce(t *testing.T) {
	f := &fakeEnv{now: time.Unix(1e9, 0), discord: &Discord{PID: 1, Age: 8 * time.Second}, healthy: true}
	w := New(cfg(config.ModeTunnel), f, time.Time{})
	w.Tick()
	if f.launches != 1 || !f.launchedProxy[0] || !f.discord.Proxied {
		t.Fatalf("fresh Discord should be relaunched through the proxy: %+v", f)
	}
	f.advance(5 * time.Second)
	w.Tick()
	if f.launches != 1 {
		t.Fatal("must not relaunch again")
	}
}

func TestOldDiscordLeftAlone(t *testing.T) {
	f := &fakeEnv{now: time.Unix(1e9, 0), discord: &Discord{PID: 1, Age: time.Hour}, healthy: true}
	w := New(cfg(config.ModeTunnel), f, time.Time{})
	w.Tick()
	if f.launches != 0 {
		t.Fatal("a Discord you've been using for an hour must not be restarted")
	}
}

func TestFreshDiscordNotMovedOntoDeadProxy(t *testing.T) {
	f := &fakeEnv{now: time.Unix(1e9, 0), discord: &Discord{PID: 1, Age: time.Second}, healthy: false}
	w := New(cfg(config.ModeTunnel), f, time.Time{})
	w.Tick()
	if f.launches != 0 {
		t.Fatal("don't relaunch onto a dead proxy")
	}
}

func TestFallbackAndRecovery(t *testing.T) {
	f := &fakeEnv{now: time.Unix(1e9, 0), discord: &Discord{PID: 1, Proxied: true, Age: time.Hour}, healthy: false}
	w := New(cfg(config.ModeTunnel), f, time.Time{})

	w.Tick() // notices the outage
	for i := 0; i < 3; i++ {
		f.advance(30 * time.Second)
		w.Tick()
	}
	if f.launches != 0 {
		t.Fatal("90s is shorter than FALLBACK_AFTER=120")
	}
	f.advance(30 * time.Second)
	w.Tick()
	if f.launches != 1 || f.launchedProxy[0] || f.discord.Proxied {
		t.Fatalf("after 120s Discord should run directly: %+v", f)
	}
	if !strings.Contains(lastNote(f), "directly through WARP") {
		t.Fatalf("notes: %v", f.notes)
	}
	// the fresh direct Discord must not be bounced back onto the dead proxy
	f.advance(30 * time.Second)
	w.Tick()
	if f.launches != 1 {
		t.Fatal("bounced back onto the dead proxy")
	}
	// proxy comes back: tell the user, don't restart Discord by surprise
	f.healthy = true
	f.advance(30 * time.Second)
	w.Tick()
	if f.launches != 1 || !strings.Contains(lastNote(f), "proxy is back") {
		t.Fatalf("launches=%d notes=%v", f.launches, f.notes)
	}
}

func TestFallbackDisabled(t *testing.T) {
	c := cfg(config.ModeTunnel)
	c.FallbackAfter = 0
	f := &fakeEnv{now: time.Unix(1e9, 0), discord: &Discord{PID: 1, Proxied: true, Age: time.Hour}}
	w := New(c, f, time.Time{})
	w.Tick()
	f.advance(30 * time.Second)
	w.Tick()
	if f.launches != 0 || !strings.Contains(lastNote(f), "dwf launch --direct") {
		t.Fatalf("launches=%d notes=%v", f.launches, f.notes)
	}
}

func TestBurstInTunnelMode(t *testing.T) {
	// Discord running directly: tell the user to launch through the proxy
	f := &fakeEnv{now: time.Unix(1e9, 0), discord: &Discord{PID: 1, Age: time.Hour}, healthy: true}
	w := New(cfg(config.ModeTunnel), f, time.Time{})
	f.logErrs = 5
	w.Tick()
	if !strings.Contains(lastNote(f), "isn't using your proxy") {
		t.Fatalf("notes: %v", f.notes)
	}

	// proxied but the proxy is dead: restart the tunnel
	f = &fakeEnv{now: time.Unix(1e9, 0), discord: &Discord{PID: 1, Proxied: true, Age: time.Hour}}
	w = New(cfg(config.ModeTunnel), f, time.Time{})
	f.logErrs = 5
	w.Tick()
	if f.restarts != 1 {
		t.Fatal("tunnel should be restarted")
	}
}
