//go:build !windows

package warp

import "os/exec"

func hideWindow(*exec.Cmd) {}
