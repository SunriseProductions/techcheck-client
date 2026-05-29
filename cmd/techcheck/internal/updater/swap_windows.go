//go:build windows

package updater

import "golang.org/x/sys/windows"

// applySwap on Windows atomically replaces the single .exe. Go's os.Rename does
// not replace an existing file on Windows, so MoveFileEx is used directly.
func applySwap(staged, target string) error {
	from, err := windows.UTF16PtrFromString(staged)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
