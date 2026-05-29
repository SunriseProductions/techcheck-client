package updater

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func writeDir(t *testing.T, path, marker string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "marker"), []byte(marker), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readMarker(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(path, "marker"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRenameSwapDir_Swaps(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "App.app")
	staged := filepath.Join(dir, ".App.app.new")
	writeDir(t, target, "old")
	writeDir(t, staged, "new")

	if err := renameSwapDir(staged, target); err != nil {
		t.Fatal(err)
	}
	if got := readMarker(t, target); got != "new" {
		t.Errorf("target marker = %q, want new", got)
	}
	if _, err := os.Stat(target + ".old"); !os.IsNotExist(err) {
		t.Error(".old should be cleaned up")
	}
}

func TestRenameSwapDir_RollsBackWhenStagedMissing(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "App.app")
	staged := filepath.Join(dir, ".App.app.new") // never created
	writeDir(t, target, "old")

	if err := renameSwapDir(staged, target); err == nil {
		t.Fatal("expected error when staged path is missing")
	}
	if got := readMarker(t, target); got != "old" {
		t.Errorf("target marker = %q after rollback, want old", got)
	}
}

func TestWaitForExit_ReturnsAfterChildDies(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses unix sleep")
	}
	cmd := exec.Command("sleep", "0.3")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	go func() { _ = cmd.Wait() }()

	start := time.Now()
	if err := waitForExit(pid, 5*time.Second); err != nil {
		t.Fatalf("waitForExit: %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Error("waitForExit took too long")
	}
}
