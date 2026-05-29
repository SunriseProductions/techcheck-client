package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// InstallTarget resolves the path that the swap will replace: the .app bundle
// on macOS, or the .exe on Windows/Linux. Returns an error if the running
// binary is not laid out like an installed app (e.g. `wails dev`).
func InstallTarget() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return resolveTarget(runtime.GOOS, exe)
}

// resolveTarget is the pure, testable core of InstallTarget.
func resolveTarget(goos, exe string) (string, error) {
	if goos == "darwin" {
		dir := exe
		for {
			if strings.HasSuffix(strings.ToLower(filepath.Base(dir)), ".app") {
				return dir, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return "", fmt.Errorf("updater: not running from a .app bundle (%s)", exe)
			}
			dir = parent
		}
	}
	return exe, nil
}

// TargetWritable reports whether the directory containing target can be written
// to — a precondition for the in-place swap. Advisory only: a real failure
// still surfaces later as an error and falls back to the browser path.
func TargetWritable(target string) bool {
	f, err := os.CreateTemp(filepath.Dir(target), ".techcheck-wtest")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// CleanupLeftovers removes `.old` and the staged sibling left next to target by
// a swap whose best-effort cleanup did not finish. It is a no-op when target is
// absent, so it can never delete the last surviving copy of the app. Best-effort.
func CleanupLeftovers(target string) {
	if _, err := os.Stat(target); err != nil {
		return // only sweep when a valid install is present
	}
	_ = os.RemoveAll(target + ".old")
	_ = os.RemoveAll(stagedPath(filepath.Dir(target)))
}
