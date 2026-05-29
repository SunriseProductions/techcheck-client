//go:build !windows

package updater

import "syscall"

// detachSysProcAttr starts the helper in its own session so it survives the
// parent quitting.
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
