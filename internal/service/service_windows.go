package service

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// UNTESTED on real Windows. See CONTRIBUTING.md.
//
// The HKCU Run key starts dwf at login without admin rights. Task Scheduler
// would restart it on crash, but logon triggers need admin.

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

const (
	detachedProcess = 0x00000008
	createNoWindow  = 0x08000000
)

// Command is what the Run key executes.
func Command(exe string) string { return `"` + exe + `" run --background` }

// Install registers dwf to start at login and starts it now.
func Install(exe string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetStringValue(Name, Command(exe)); err != nil {
		return err
	}
	StopRunning()
	cmd := exec.Command(exe, "run", "--background")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | createNoWindow, HideWindow: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// Uninstall removes the Run entry and stops dwf.
func Uninstall() error {
	if k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE); err == nil {
		_ = k.DeleteValue(Name)
		k.Close()
	}
	StopRunning()
	return nil
}

// Installed reports whether the Run entry exists.
func Installed() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(Name)
	return err == nil
}

// Describe names the mechanism for status output.
func Describe() string { return `HKCU\` + runKey + `\` + Name }

var freeConsole = windows.NewLazySystemDLL("kernel32.dll").NewProc("FreeConsole")

// Detach drops the console window a Run-key start opens.
func Detach() { _, _, _ = freeConsole.Call() }
