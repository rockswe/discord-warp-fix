package service

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/rockswe/discord-warp-fix/internal/paths"
)

const label = "com.discord-warp-fix"

// Labels used by the Bash version, removed on install.
var legacyLabels = []string{"com.discord-warp-fix.watch", "com.discord-warp-fix.tunnel"}

func agentDir() string {
	if v := os.Getenv("DWF_AGENT_DIR"); v != "" {
		return v
	}
	return filepath.Join(paths.Home(), "Library", "LaunchAgents")
}

func plistPath(l string) string { return filepath.Join(agentDir(), l+".plist") }

func domain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// Plist renders the launchd agent.
func Plist(exe, logFile string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>run</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ThrottleInterval</key><integer>10</integer>
  <key>ProcessType</key><string>Background</string>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, label, esc(exe), esc(logFile), esc(logFile))
}

func unload(l string) {
	_ = exec.Command("launchctl", "bootout", domain()+"/"+l).Run()
	_ = os.Remove(plistPath(l))
}

// Install writes and loads the launchd agent, replacing older installs.
func Install(exe string) error {
	for _, l := range legacyLabels {
		unload(l)
	}
	_ = exec.Command("launchctl", "bootout", domain()+"/"+label).Run()
	StopRunning()
	if err := os.MkdirAll(agentDir(), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(paths.LogFile()), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(plistPath(label), []byte(Plist(exe, paths.LogFile())), 0o644); err != nil {
		return err
	}
	if out, err := exec.Command("plutil", "-lint", plistPath(label)).CombinedOutput(); err != nil {
		return fmt.Errorf("invalid plist: %s", out)
	}
	if out, err := exec.Command("launchctl", "bootstrap", domain(), plistPath(label)).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl couldn't load the agent: %s", bytes.TrimSpace(out))
	}
	return nil
}

// Uninstall stops and removes the agent.
func Uninstall() error {
	unload(label)
	for _, l := range legacyLabels {
		unload(l)
	}
	StopRunning()
	return nil
}

// Installed reports whether the agent is loaded.
func Installed() bool {
	return exec.Command("launchctl", "print", domain()+"/"+label).Run() == nil
}

// Describe names the mechanism for status output.
func Describe() string { return "launchd agent " + label }

// Detach is a no-op on macOS; launchd owns the process.
func Detach() {}
