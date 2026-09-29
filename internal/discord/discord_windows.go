package discord

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/rockswe/discord-warp-fix/internal/paths"
	"github.com/shirou/gopsutil/v4/process"
)

// UNTESTED on real Windows. See CONTRIBUTING.md.

func isMain(app string, pr *process.Process) bool { return belongs(app, pr) }

func belongs(app string, pr *process.Process) bool {
	name, err := pr.Name()
	return err == nil && strings.EqualFold(name, flavorOf(app).binary+".exe")
}

// Closing Discord's window only hides it to the tray, so there's no
// gentle way to quit. Quit falls through to terminating the processes.
func gracefulQuit(string, *Process) {}

const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
)

// Discord installs through Squirrel: Update.exe starts the newest app-x.y.z
// folder and forwards --process-start-args to Discord.exe.
func launch(app, proxyURL string) error {
	f := flavorOf(app)
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		local = filepath.Join(paths.Home(), "AppData", "Local")
	}
	update := filepath.Join(local, f.appDir, "Update.exe")
	args := []string{"--processStart", f.binary + ".exe"}
	if proxyURL != "" {
		args = append(args, "--process-start-args", "--proxy-server="+proxyURL)
	}
	cmd := exec.Command(update, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | createNewProcessGroup}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// LogPath is Discord's renderer log under %APPDATA%.
func LogPath(app string) string {
	roaming := os.Getenv("APPDATA")
	if roaming == "" {
		roaming = filepath.Join(paths.Home(), "AppData", "Roaming")
	}
	return filepath.Join(roaming, flavorOf(app).dataDir, "logs", "renderer_js.log")
}
