//go:build !windows

package app

import "os/exec"

func hideProbeWindow(_ *exec.Cmd) {}
