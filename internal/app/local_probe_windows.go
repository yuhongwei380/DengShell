//go:build windows

package app

import (
	"os/exec"
	"syscall"
)

// Probe helpers run as child processes of a windowsgui application; without
// HideWindow the console host (tracert) pops up a visible cmd window.
func hideProbeWindow(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
