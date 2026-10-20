package updater

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveTarget_DarwinWalksToApp(t *testing.T) {
	skipOnWindows(t)
	got, err := resolveTarget("darwin", "/Applications/SunriseTechCheck.app/Contents/MacOS/SunriseTechCheck")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/Applications/SunriseTechCheck.app" {
		t.Errorf("got %q", got)
	}
}

func TestResolveTarget_DarwinExeIsApp(t *testing.T) {
	skipOnWindows(t)
	got, err := resolveTarget("darwin", "/Applications/Foo.app")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/Applications/Foo.app" {
		t.Errorf("got %q", got)
	}
}

func TestResolveTarget_DarwinCaseInsensitive(t *testing.T) {
	skipOnWindows(t)
	got, err := resolveTarget("darwin", "/Applications/Foo.APP/Contents/MacOS/Foo")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/Applications/Foo.APP" {
		t.Errorf("got %q", got)
	}
}

func TestResolveTarget_DarwinNotInBundle(t *testing.T) {
	skipOnWindows(t)
	_, err := resolveTarget("darwin", "/usr/local/bin/techcheck")
	if err == nil {
		t.Fatal("expected error for non-.app path")
	}
}

func TestResolveTarget_WindowsReturnsExe(t *testing.T) {
	got, err := resolveTarget("windows", `C:\Program Files\SunriseTechCheck\SunriseTechCheck.exe`)
	if err != nil {
		t.Fatal(err)
	}
	if got != `C:\Program Files\SunriseTechCheck\SunriseTechCheck.exe` {
		t.Errorf("got %q", got)
	}
}

func TestResolveTarget_LinuxReturnsExe(t *testing.T) {
	got, err := resolveTarget("linux", "/opt/techcheck/techcheck")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/opt/techcheck/techcheck" {
		t.Errorf("got %q", got)
	}
}

func TestTargetWritable(t *testing.T) {
	dir := t.TempDir()
	if !TargetWritable(filepath.Join(dir, "SunriseTechCheck.app")) {
		t.Error("temp dir should be writable")
	}
	if TargetWritable(filepath.Join(dir, "nope", "deeper", "x.app")) {
		t.Error("nonexistent parent dir should not be writable")
	}
}

func TestCleanupLeftovers(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "SunriseTechCheck.app")
	if err := os.MkdirAll(target, 0o755); err != nil { // present, valid install
		t.Fatal(err)
	}
	if err := os.MkdirAll(target+".old", 0o755); err != nil {
		t.Fatal(err)
	}
	staged := stagedPath(dir)
	if err := os.MkdirAll(staged, 0o755); err != nil {
		t.Fatal(err)
	}
	CleanupLeftovers(target)
	if _, err := os.Stat(target + ".old"); !os.IsNotExist(err) {
		t.Error(".old not removed")
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Error("staged leftover not removed")
	}
	if _, err := os.Stat(target); err != nil {
		t.Error("target itself must not be removed")
	}
}

func TestCleanupLeftovers_NoopWhenTargetAbsent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "SunriseTechCheck.app") // never created
	old := target + ".old"
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	CleanupLeftovers(target)
	if _, err := os.Stat(old); err != nil {
		t.Error(".old must be preserved when target is absent")
	}
}

// skipOnWindows skips the darwin path tests: filepath uses backslashes there,
// and the darwin branch never runs on Windows.
func skipOnWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("darwin bundle paths use forward slashes")
	}
}
