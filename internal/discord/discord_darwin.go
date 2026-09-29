package discord

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rockswe/erisim/internal/paths"
	"github.com/shirou/gopsutil/v4/process"
)

func mainExeSuffix(app string) string { return "/" + app + ".app/Contents/MacOS/" + app }

func isMain(app string, pr *process.Process) bool {
	exe, err := pr.Exe()
	return err == nil && strings.HasSuffix(exe, mainExeSuffix(app))
}

func belongs(app string, pr *process.Process) bool {
	exe, err := pr.Exe()
	return err == nil && strings.Contains(exe, "/"+app+".app/")
}

func gracefulQuit(app string, _ *Process) {
	_ = exec.Command("osascript", "-e", `quit app "`+app+`"`).Run()
}

func launch(app, proxyURL string) error {
	args := []string{"-a", app}
	if proxyURL != "" {
		args = append(args, "--args", "--proxy-server="+proxyURL)
	}
	return exec.Command("open", args...).Run()
}

// LogPath is Discord's renderer log.
func LogPath(app string) string {
	return filepath.Join(paths.Home(), "Library", "Application Support", flavorOf(app).dataDir, "logs", "renderer_js.log")
}
