//go:build !windows

package sysinfo

import "os/exec"

// hideWindow is a no-op on non-Windows platforms. POSIX shell-outs from a GUI
// process don't spawn visible terminal windows, so nothing to hide.
func hideWindow(cmd *exec.Cmd) {}
