package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rockswe/discord-warp-fix/internal/paths"
)

const unitName = Name + ".service"

func unitPath() string {
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(paths.Home(), ".config")
	}
	return filepath.Join(cfg, "systemd", "user", unitName)
}

// Environment the service needs to show notifications and start Discord
// in the desktop session, captured at install time.
var sessionVars = []string{"DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS", "XDG_CURRENT_DESKTOP", "PATH"}

func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "%", "%%").Replace(s) + `"`
}

// Unit renders the systemd user unit.
func Unit(exe string, env map[string]string) string {
	var b strings.Builder
	b.WriteString("[Unit]\nDescription=discord-warp-fix: keeps Discord working behind Cloudflare WARP\nAfter=network-online.target\n\n[Service]\n")
	fmt.Fprintf(&b, "ExecStart=%s run\n", quote(exe))
	for _, k := range sessionVars {
		if v, ok := env[k]; ok && v != "" {
			fmt.Fprintf(&b, "Environment=%s\n", quote(k+"="+v))
		}
	}
	b.WriteString("Restart=always\nRestartSec=10\n\n[Install]\nWantedBy=default.target\n")
	return b.String()
}

func systemctl(args ...string) error {
	out, err := exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl --user %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}

// Install writes, enables and starts the systemd user unit.
func Install(exe string) error {
	if err := systemctl("show-environment"); err != nil {
		return fmt.Errorf("no systemd user session available (%v). Start `dwf run` from your desktop's autostart instead", err)
	}
	env := map[string]string{}
	for _, k := range sessionVars {
		env[k] = os.Getenv(k)
	}
	StopRunning()
	if err := os.MkdirAll(filepath.Dir(unitPath()), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(unitPath(), []byte(Unit(exe, env)), 0o644); err != nil {
		return err
	}
	if err := systemctl("daemon-reload"); err != nil {
		return err
	}
	if err := systemctl("enable", unitName); err != nil {
		return err
	}
	return systemctl("restart", unitName)
}

// Uninstall stops, disables and removes the unit.
func Uninstall() error {
	_ = systemctl("disable", "--now", unitName)
	_ = os.Remove(unitPath())
	_ = systemctl("daemon-reload")
	StopRunning()
	return nil
}

// Installed reports whether the unit file exists.
func Installed() bool {
	_, err := os.Stat(unitPath())
	return err == nil
}

// Describe names the mechanism for status output.
func Describe() string { return "systemd user unit " + unitName }

// Detach is a no-op on Linux; systemd owns the process.
func Detach() {}
