//go:build !windows

package updater

import "syscall"

// processAlive reports whether pid is still running. On Unix, signal 0 probes
// without delivering: nil or EPERM means alive, ESRCH means gone.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
