//go:build windows

package updater

import "golang.org/x/sys/windows"

const stillActive = 259 // STILL_ACTIVE

// processAlive reports whether pid is still running on Windows.
func processAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}
