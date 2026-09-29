package discord

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/rockswe/discord-warp-fix/internal/paths"
	"github.com/shirou/gopsutil/v4/process"
)

// Covers the .deb/.tar.gz builds (/usr/share/discord/Discord), Flatpak
// (/app/discord/Discord) and Snap, which all run a binary named Discord.
func isMain(app string, pr *process.Process) bool { return belongs(app, pr) }

func belongs(app string, pr *process.Process) bool {
	name, err := pr.Name()
	return err == nil && name == flavorOf(app).binary
}

func gracefulQuit(_ string, p *Process) {
	if pr, err := process.NewProcess(p.PID); err == nil {
		_ = pr.Terminate()
	}
}

func launcher(app string) ([]string, error) {
	f := flavorOf(app)
	if p, err := exec.LookPath(f.command); err == nil {
		return []string{p}, nil
	}
	if fp, err := exec.LookPath("flatpak"); err == nil && exec.Command(fp, "info", f.flatpak).Run() == nil {
		return []string{fp, "run", f.flatpak}, nil
	}
	if _, err := os.Stat("/snap/bin/" + f.command); err == nil {
		return []string{"/snap/bin/" + f.command}, nil
	}
	return nil, errors.New("couldn't find how to start Discord (tried " + f.command + ", Flatpak and Snap)")
}

func launch(app, proxyURL string) error {
	argv, err := launcher(app)
	if err != nil {
		return err
	}
	if proxyURL != "" {
		argv = append(argv, "--proxy-server="+proxyURL)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// LogPath is Discord's renderer log: the native location, or the Flatpak
// one when only that exists.
func LogPath(app string) string {
	f := flavorOf(app)
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(paths.Home(), ".config")
	}
	native := filepath.Join(cfg, f.dataDir, "logs", "renderer_js.log")
	flat := filepath.Join(paths.Home(), ".var", "app", f.flatpak, "config", f.dataDir, "logs", "renderer_js.log")
	if _, err := os.Stat(native); err != nil {
		if _, err := os.Stat(flat); err == nil {
			return flat
		}
	}
	return native
}
