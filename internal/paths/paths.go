// Package paths decides where dwf keeps its config, state and log on each OS.
// Every location can be overridden with an environment variable, which the
// tests use to stay out of the real home directory.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

const appName = "discord-warp-fix"

func home() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ConfigDir holds the config file and the tunnel's known_hosts.
// macOS and Linux keep the location the Bash version used.
func ConfigDir() string {
	if v := os.Getenv("DWF_CONFIG_DIR"); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(envOr("APPDATA", filepath.Join(home(), "AppData", "Roaming")), appName)
	}
	return filepath.Join(envOr("XDG_CONFIG_HOME", filepath.Join(home(), ".config")), appName)
}

// ConfigFile is the KEY=value config file.
func ConfigFile() string { return filepath.Join(ConfigDir(), "config") }

// KnownHostsFile pins the tunnel server's host key.
func KnownHostsFile() string { return filepath.Join(ConfigDir(), "known_hosts") }

// StateDir holds the pid file and the watchdog's last-action timestamp.
func StateDir() string {
	if v := os.Getenv("DWF_STATE_DIR"); v != "" {
		return v
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home(), "Library", "Application Support", appName)
	case "windows":
		return filepath.Join(envOr("LOCALAPPDATA", filepath.Join(home(), "AppData", "Local")), appName)
	default:
		return filepath.Join(envOr("XDG_STATE_HOME", filepath.Join(home(), ".local", "state")), appName)
	}
}

// LogFile is where the background service writes what it does.
func LogFile() string {
	if v := os.Getenv("DWF_LOG_FILE"); v != "" {
		return v
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home(), "Library", "Logs", appName+".log")
	}
	return filepath.Join(StateDir(), "dwf.log")
}

// DefaultSSHKey is the private key used for the tunnel.
func DefaultSSHKey() string { return filepath.Join(home(), ".ssh", appName+"_ed25519") }

// Home returns the user's home directory.
func Home() string { return home() }
