//go:build !windows

package updater

// applySwap on non-Windows replaces a .app directory via the two-step rename.
func applySwap(staged, target string) error {
	return renameSwapDir(staged, target)
}
