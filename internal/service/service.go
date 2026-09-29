// Package service installs dwf as a background service that starts at login
// (launchd on macOS, a systemd user unit on Linux, the Run key on Windows)
// and tracks the running instance with a pid file.
package service

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rockswe/discord-warp-fix/internal/paths"
	"github.com/shirou/gopsutil/v4/process"
)

// Name identifies the service on every platform.
const Name = "discord-warp-fix"

// PIDFile holds the pid of the running `dwf run`.
func PIDFile() string { return filepath.Join(paths.StateDir(), "dwf.pid") }

// Running returns the pid of a live `dwf run`, if any.
func Running() (int, bool) {
	b, err := os.ReadFile(PIDFile())
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return 0, false
	}
	name, _ := p.Name()
	if !strings.Contains(strings.ToLower(name), "dwf") {
		return 0, false // stale file, pid reused by something else
	}
	return pid, true
}

// ErrAlreadyRunning means another `dwf run` owns the pid file.
var ErrAlreadyRunning = errors.New("dwf is already running")

// Claim writes this process's pid, unless another instance is alive.
func Claim() error {
	if pid, ok := Running(); ok && pid != os.Getpid() {
		return ErrAlreadyRunning
	}
	if err := os.MkdirAll(paths.StateDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(PIDFile(), []byte(strconv.Itoa(os.Getpid())), 0o644)
}

// Release removes the pid file if it's ours.
func Release() {
	if b, err := os.ReadFile(PIDFile()); err == nil && strings.TrimSpace(string(b)) == strconv.Itoa(os.Getpid()) {
		_ = os.Remove(PIDFile())
	}
}

// StopRunning terminates a running instance and waits for it to exit.
func StopRunning() {
	pid, ok := Running()
	if !ok {
		return
	}
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return
	}
	_ = p.Terminate()
	for i := 0; i < 20; i++ {
		if alive, _ := process.PidExists(int32(pid)); !alive {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	_ = p.Kill()
}

// Executable is the absolute, symlink-free path of the running binary.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return exe, nil
}
