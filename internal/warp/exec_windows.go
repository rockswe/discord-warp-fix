//go:build windows

package warp

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// hideWindow stops a console window from flashing up for each warp-cli call.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}
}
