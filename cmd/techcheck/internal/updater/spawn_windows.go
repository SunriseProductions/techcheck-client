//go:build windows

package updater

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// detachSysProcAttr starts the helper detached and in a new process group so it
// survives the parent quitting.
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
}
