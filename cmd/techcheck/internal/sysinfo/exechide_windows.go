//go:build windows

package sysinfo

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW tells Windows not to create a console window for a child
// console process. Wails apps have no console of their own, so without this
// flag every child process flashes a visible cmd window.
const createNoWindow = 0x08000000

// hideWindow configures cmd so its console window stays hidden. Must be
// called before the command runs.
func hideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
