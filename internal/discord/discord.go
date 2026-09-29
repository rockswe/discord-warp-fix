// Package discord finds, launches and quits the Discord desktop app.
//
// Only the main process matters: it's the one Discord was launched as, and
// it carries --proxy-server when dwf launched it. Electron's helper
// processes have a --type= flag and are ignored.
package discord

import (
	"errors"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// Process is Discord's main process.
type Process struct {
	PID     int32
	Cmdline string
	Started time.Time
}

// Proxied reports whether Discord was started with a proxy.
func (p *Process) Proxied() bool { return p.ProxyArg() != "" }

// ProxyArg is the value of --proxy-server, or "".
func (p *Process) ProxyArg() string {
	for _, f := range strings.Fields(p.Cmdline) {
		if v, ok := strings.CutPrefix(strings.Trim(f, `"'`), "--proxy-server="); ok {
			return v
		}
	}
	return ""
}

// Age is how long the process has been running.
func (p *Process) Age(now time.Time) time.Duration { return now.Sub(p.Started) }

// Find returns Discord's main process, or nil when it isn't running.
func Find(app string) (*Process, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}
	var best *Process
	for _, pr := range procs {
		if !isMain(app, pr) {
			continue
		}
		cmd, _ := pr.Cmdline()
		if strings.Contains(cmd, "--type=") {
			continue
		}
		ms, _ := pr.CreateTime()
		p := &Process{PID: pr.Pid, Cmdline: cmd, Started: time.UnixMilli(ms)}
		if best == nil || p.Started.Before(best.Started) {
			best = p // the oldest match is the parent
		}
	}
	return best, nil
}

// all returns every Discord process, helpers included.
func all(app string) []*process.Process {
	procs, _ := process.Processes()
	var out []*process.Process
	for _, pr := range procs {
		if belongs(app, pr) {
			out = append(out, pr)
		}
	}
	return out
}

// Quit closes Discord, gently first.
func Quit(app string) error {
	p, _ := Find(app)
	if p == nil {
		return nil
	}
	gracefulQuit(app, p)
	if waitGone(app, 20*time.Second) {
		return nil
	}
	for _, pr := range all(app) {
		_ = pr.Terminate()
	}
	if waitGone(app, 5*time.Second) {
		return nil
	}
	for _, pr := range all(app) {
		_ = pr.Kill()
	}
	if waitGone(app, 5*time.Second) {
		return nil
	}
	return errors.New("Discord didn't quit")
}

func waitGone(app string, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if p, _ := Find(app); p == nil {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// Relaunch quits Discord and starts it again, through proxyURL when set.
func Relaunch(app, proxyURL string) error {
	if err := Quit(app); err != nil {
		return err
	}
	return launch(app, proxyURL)
}

// flavor maps the app name to its per-OS variants.
type flavor struct {
	binary  string // Linux/Windows executable name without .exe
	dataDir string // settings folder name
	appDir  string // Windows install folder under %LOCALAPPDATA%
	flatpak string // Linux Flatpak app id
	command string // Linux launcher on PATH
}

func flavorOf(app string) flavor {
	switch strings.ToLower(strings.ReplaceAll(app, " ", "")) {
	case "discordptb":
		return flavor{"DiscordPTB", "discordptb", "DiscordPTB", "com.discordapp.DiscordPTB", "discord-ptb"}
	case "discordcanary":
		return flavor{"DiscordCanary", "discordcanary", "DiscordCanary", "com.discordapp.DiscordCanary", "discord-canary"}
	default:
		return flavor{"Discord", "discord", "Discord", "com.discordapp.Discord", "discord"}
	}
}
