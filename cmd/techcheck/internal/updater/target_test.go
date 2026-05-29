package updater

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTarget_DarwinWalksToApp(t *testing.T) {
	got, err := resolveTarget("darwin", "/Applications/SunriseTechCheck.app/Contents/MacOS/SunriseTechCheck")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/Applications/SunriseTechCheck.app" {
		t.Errorf("got %q", got)
	}
}

func TestResolveTarget_DarwinExeIsApp(t *testing.T) {
	got, err := resolveTarget("darwin", "/Applications/Foo.app")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/Applications/Foo.app" {
		t.Errorf("got %q", got)
	}
}

func TestResolveTarget_DarwinCaseInsensitive(t *testing.T) {
	got, err := resolveTarget("darwin", "/Applications/Foo.APP/Contents/MacOS/Foo")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/Applications/Foo.APP" {
		t.Errorf("got %q", got)
	}
}

func TestResolveTarget_DarwinNotInBundle(t *testing.T) {
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
	if err := os.MkdirAll(target+".old", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target+".new", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	CleanupLeftovers(target)
	if _, err := os.Stat(target + ".old"); !os.IsNotExist(err) {
		t.Error(".old not removed")
	}
	if _, err := os.Stat(target + ".new"); !os.IsNotExist(err) {
		t.Error(".new not removed")
	}
}
